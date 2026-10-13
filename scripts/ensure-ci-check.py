#!/usr/bin/env python3
"""Run the full local gate, or reuse its evidence for identical audited inputs.

The caller names its execution context; an unnamed context is execute-only.
This is cooperative local evidence, never release qualification or authority.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import shlex
import signal
import stat
import subprocess
import sys
import time
import uuid

POLICY = 1
MAX_AGE = 24 * 60 * 60
# Build artifacts are not source. Ignored inputs everywhere else are included.
OUTPUT_ROOTS = ('.cache/', 'bin/', 'dist/', '.idea/', 'node_modules/')


def run(root, *args):
    return subprocess.check_output(args, cwd=root).decode().strip()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def file_record(path, root=None):
    if not path.exists() and not path.is_symlink():
        return ['missing']
    info = path.lstat()
    if path.is_symlink():
        target = path.resolve(strict=True)
        if not target.is_file() or (root and not target.is_relative_to(root)):
            raise ValueError('unsupported symlink input')
        return ['link', stat.S_IMODE(info.st_mode), os.readlink(path), digest(target.read_bytes())]
    if not path.is_file():
        raise ValueError('unsupported non-file input')
    return ['file', stat.S_IMODE(info.st_mode), digest(path.read_bytes())]


def tool_tree(root):
    root=Path(root).resolve(strict=True);records={}
    def unreadable(error): raise error
    for current,dirs,files in os.walk(root,onerror=unreadable,followlinks=False):
        for name in sorted(dirs+files):
            path=Path(current)/name;key=str(path.relative_to(root))
            if path.is_symlink() and path.resolve(strict=True).is_dir():
                if not path.resolve().is_relative_to(root):raise ValueError('external tool directory symlink; execute-only')
                # Its real directory is inside this same complete tree.
                records[key]=['directory-link',stat.S_IMODE(path.lstat().st_mode),os.readlink(path)]
            elif path.is_dir():records[key]=['directory',stat.S_IMODE(path.stat().st_mode)]
            else:records[key]=file_record(path,root)
    return digest(json.dumps(records,sort_keys=True).encode())


def source_snapshot(root):
    # Include dirty bytes, deletions, ignored inputs and untracked additions.
    tracked = set(os.fsdecode(x) for x in subprocess.check_output(['git','ls-files','-z','--cached'],cwd=root).split(b'\0') if x)
    names = set()
    for flags in (['--cached'], ['--others','--exclude-standard'], ['--others','--ignored','--exclude-standard']):
        out = subprocess.check_output(['git','ls-files','-z',*flags],cwd=root)
        names.update(os.fsdecode(x) for x in out.split(b'\0') if x)
    records = {}
    for name in sorted(names):
        if name.startswith(OUTPUT_ROOTS) or name.startswith('.tools/') or name.endswith(('.pyc','.pyo')) or '/__pycache__/' in name or name == '.DS_Store':
            # A tracked file is always an input, even under an output path.
            if name not in tracked:
                continue
        records[name] = file_record(root/name,root)
    return digest(json.dumps(records,sort_keys=True).encode())


def go_settings(settings):
    settings=dict(settings)
    # go env creates a fresh work dir for this debug-only map on every invocation.
    settings['GOGCCFLAGS']=re.sub(r'(-ffile-prefix-map=)\S*/go-build[0-9]+(=/tmp/go-build)(?=\s|$)',r'\1<go-workdir>\2',settings.get('GOGCCFLAGS',''))
    return settings


def tool_context(root, go, context, env, salt):
    if not context:
        raise ValueError('execution context is unnamed; execute-only')
    prefixes=('PRIFLY_','ACCEPTANCE_','PYTHONPATH','PYTHONHOME','NODE_OPTIONS','NODE_PATH','DYLD_','LD_','GIT_CONFIG_','CPATH','C_INCLUDE_PATH','CPLUS_INCLUDE_PATH','LIBRARY_PATH','PKG_CONFIG_','BASH_ENV','ENV')
    if any(v and k.startswith(prefixes) for k,v in env.items()):
        raise ValueError('unregistered external/test environment override; execute-only')
    tools = {}
    for name in ('make','git','python3','sh','bash','gcc','clang','cc','clang++','g++','ld','ar','node','curl','tar','shasum','sha256sum','perl','grep','find','wc','tr','rm','mktemp','mkdir','chmod','sed','awk','uname','head','basename','dirname','cp','mv','cat','sort','uniq','touch','env','ps','pgrep','kill','cut'):
        path = shutil.which(name, path=env.get('PATH'))
        tools[name] = [path, file_record(Path(path).resolve())] if path else ['absent']
    go = str(Path(go).resolve(strict=True))
    settings = go_settings(json.loads(run(root,go,'env','-json')))
    if settings.get('GOWORK') not in ('','off') or settings.get('GOFLAGS'):
        raise ValueError('external workspace/GOFLAGS require fresh execution')
    if Path(settings['GOMOD']).resolve()!=root/'go.mod':
        raise ValueError('external module input requires fresh execution')
    cc=shlex.split(settings.get('CC',''))
    if len(cc)!=1 or cc[0] not in ('gcc','clang','cc'):
        raise ValueError('custom compiler requires fresh execution')
    compiler=shutil.which(cc[0],path=env.get('PATH'))
    if not compiler: raise ValueError('compiler identity unavailable')
    if sys.platform=='darwin':
        compiler=run(root,'xcrun','--find',cc[0])
    tools['compiler']=[compiler,file_record(Path(compiler).resolve()),run(root,compiler,'--version')]
    for key in ('CGO_CFLAGS','CGO_CPPFLAGS','CGO_CXXFLAGS','CGO_LDFLAGS'):
        if env.get(key):raise ValueError('custom compiler flags require fresh execution')
    tools['go'] = file_record(Path(go))
    goroot = Path(settings['GOROOT'])
    # Hash compiler/stdlib source too: a version label alone misses local edits.
    parts = {}
    for folder in ('bin','src','pkg/tool','pkg/include'):
        for p in sorted((goroot/folder).rglob('*')):
            if p.is_file() or p.is_symlink():
                parts[str(p.relative_to(goroot))] = file_record(p)
    tools['goroot'] = digest(json.dumps(parts,sort_keys=True).encode())
    headers=(Path(run(root,'xcrun','--show-sdk-path')) if sys.platform=='darwin' else Path('/usr/include')).resolve()
    tools['headers']=tool_tree(headers)
    resource=run(root,compiler,'-print-resource-dir') if cc[0] in ('clang','cc') and sys.platform=='darwin' else run(root,compiler,'-print-file-name=include')
    tools['compiler_resources']=tool_tree(Path(resource))
    if sys.platform=='darwin':
        linker=Path(run(root,'xcrun','--find','ld')).resolve(strict=True)
        tools['native_linker']=file_record(linker)
    # Existing .tools may contain additional ignored inputs; never assume it is only Go.
    extra = {}
    for p in sorted((root/'.tools').rglob('*')):
        if p.is_file() or p.is_symlink():
            extra[str(p.relative_to(root))] = file_record(p)
    tools['local_tools'] = digest(json.dumps(extra,sort_keys=True).encode())
    # Hash the complete inherited environment; omit only proven volatile shell/make bookkeeping.
    ignored = {'_','SHLVL','PWD','OLDPWD','MAKEFLAGS','MFLAGS','MAKELEVEL','MAKEOVERRIDES'}
    opaque = {k:digest(salt+v.encode()) for k,v in env.items() if k not in ignored}
    tools['git_configuration']=digest(salt+run(root,'git','config','--list','--show-origin').encode())
    return {'policy':POLICY,'context':context,'platform':platform.platform(),'uid':os.getuid(),
            'tools':tools,'go_env':settings,'environment':opaque,'commands':[['make','ci-check','GO='+go],['make','vuln-check','GO='+go]]}


def fingerprint(root, go, context, env, salt):
    data = {'root':str(root),'git_dir':run(root,'git','rev-parse','--absolute-git-dir'),
            'head':run(root,'git','rev-parse','HEAD'),'source':source_snapshot(root),
            'execution':tool_context(root,go,context,env,salt)}
    return digest(json.dumps(data,sort_keys=True).encode())


def staged_matches(root):
    paths = subprocess.check_output(['git','diff','--cached','--name-only','-z','--diff-filter=ACDMRT'],cwd=root).split(b'\0')
    for raw in paths:
        if not raw: continue
        name=os.fsdecode(raw)
        # git diff compares index bytes and modes, including symlinks/deletions.
        if subprocess.run(['git','diff','--quiet','--',name],cwd=root).returncode:
            return False
    return True


def valid_evidence(record, key, cache, now):
    try:
        age=now-record['finished']
        log=cache/record['log']
        return (record['policy']==POLICY and record['fingerprint']==key and record['exit_code']==0
                and 0<=age<MAX_AGE and log.parent==cache and log.is_file()
                and digest(log.read_bytes())==record['log_digest'])
    except (KeyError,TypeError,OSError):
        return False


def execute(root, go, target, log, env):
    child=env.copy()
    for key in ('MAKEFLAGS','MFLAGS','MAKEOVERRIDES','MAKELEVEL'):
        child.pop(key,None)  # -n/-i must never manufacture successful evidence.
    with log.open('wb') as out:
        p=subprocess.Popen(['make',target,'GO='+str(Path(go).resolve())],cwd=root,env=child,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,start_new_session=True)
        try:
            for data in iter(lambda:p.stdout.read1(4096),b''):
                out.write(data);out.flush()
                sink=getattr(sys.stdout,'buffer',None)
                if sink is not None:sink.write(data);sink.flush()
                else:sys.stdout.write(data.decode(errors='replace'));sys.stdout.flush()
            return p.wait()
        except BaseException:
            os.killpg(p.pid,signal.SIGTERM)
            try:p.wait(timeout=5)
            except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
            raise
        finally:
            p.stdout.close()


def ensure(root, go, context, force=False, for_commit=False, env=None):
    root=Path(root).resolve();env=dict(os.environ if env is None else env)
    for control in ('CHECK_CONTEXT','CHECK_FORCE','CHECK_FOR_COMMIT'):env.pop(control,None)
    env.update(GOCACHE=str(root/'.cache/go-build'),GOMODCACHE=str(root/'.cache/go-mod'),GOTOOLCHAIN='local',GOTELEMETRY='off')
    cache=root/'.cache/verification';cache.mkdir(parents=True,exist_ok=True)
    with (cache/'lock').open('a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX)
        saltfile=cache/'salt'
        if not saltfile.exists():saltfile.write_bytes(os.urandom(32));saltfile.chmod(0o600)
        salt=saltfile.read_bytes();receipt=cache/'success.json'
        if for_commit and not staged_matches(root):
            print('ci-check: staged bytes differ from tested working bytes',file=sys.stderr);return 2
        reason='no applicable evidence';key=None
        try:key=fingerprint(root,go,context,env,salt)
        except (OSError,ValueError,subprocess.SubprocessError) as err:reason='execute-only: '+str(err)
        try:record=json.loads(receipt.read_text())
        except (OSError,ValueError):record={}
        reuse=bool(key and not force and valid_evidence(record,key,cache,time.time()))
        if receipt.exists():receipt.unlink() # a later failure invalidates the earlier success
        before_source=source_snapshot(root)
        log=cache/(uuid.uuid4().hex+'.log')
        print('ci-check: '+('reuse '+key+'; fresh vuln-check; source '+record['log'] if reuse else 'execute '+(key or reason)),flush=True)
        try:
            status=execute(root,go,'vuln-check' if reuse else 'ci-check',log,env)
            if not status:
                subprocess.check_call([str(Path(go).resolve()),'mod','verify'],cwd=root,env=env)
            after=fingerprint(root,go,context,env,salt) if key else None
            stable=before_source==source_snapshot(root) and (key is None or key==after)
            if status or not stable:
                if not status:print('ci-check: inputs changed during check; no reusable evidence',file=sys.stderr)
                return status or 2
            if key is None:
                print('ci-check: executed; no reusable evidence ('+reason+')',flush=True)
                return 0
            if reuse:
                # The fresh scan must pass but cannot extend the original batch's lifetime.
                new=record
            else:
                new={'policy':POLICY,'fingerprint':key,'finished':time.time(),'exit_code':0,'log':log.name,'log_digest':digest(log.read_bytes())}
            temporary=cache/'success.tmp';temporary.write_text(json.dumps(new,sort_keys=True)+'\n');os.replace(temporary,receipt)
            print('ci-check: '+('reused' if reuse else 'executed')+' '+key+'; evidence '+str(receipt),flush=True)
            return 0
        except BaseException:
            if receipt.exists():receipt.unlink()
            raise


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--go',required=True);p.add_argument('--context',default=os.environ.get('CHECK_CONTEXT',''))
    p.add_argument('--force',action='store_true');p.add_argument('--for-commit',action='store_true')
    a=p.parse_args();root=Path(run(Path.cwd(),'git','rev-parse','--show-toplevel'))
    return ensure(root,a.go,a.context,a.force,a.for_commit)


if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,subprocess.SubprocessError) as err:sys.exit('ci-check: '+str(err))
