#!/usr/bin/env python3
"""Observe existing exact-SHA verify/qualify batches; never dispatch CI."""
import argparse
import json
from pathlib import Path
import re
import sys
import time
from urllib.parse import quote
from release_checks import GitHub, select


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--sha',required=True);p.add_argument('--tag',required=True);p.add_argument('--output',required=True,type=Path)
    a=p.parse_args()
    if not re.fullmatch('[0-9a-f]{40}',a.sha): p.error('--sha requires exact 40-hex commit SHA')
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?',a.tag): p.error('--tag requires an explicit version tag')
    if a.output.exists() or a.output.is_symlink(): p.error('--output must be new')
    report=dict(schema_version='schema-migration-release-gates/1',repository='StenHigh/prifly',sha=a.sha,release_tag=a.tag,
                status='blocked',collected_at_epoch=time.time(),checks={},read_only=True)
    try:
        client=GitHub('StenHigh/prifly')
        if client.ref_sha(a.tag)!=a.sha: raise ValueError('release tag resolves to a different SHA')
        release=client.api('releases/tags/'+quote(a.tag,safe=''))
        if release.get('draft') or not release.get('published_at'): raise ValueError('release is not published')
        report['release']=dict(id=release['id'],url=release['html_url'],published_at=release['published_at'],prerelease=release['prerelease'])
        report['checks']={name:select(client,name,a.sha) for name in ['verify','qualify']}
        if all(x['state']=='success' for x in report['checks'].values()): report['status']='passed'
    except Exception as err:
        report['error']=dict(type=type(err).__name__,message=str(err))
    a.output.parent.mkdir(parents=True,exist_ok=True)
    with a.output.open('x') as f: json.dump(report,f,sort_keys=True,indent=2);f.write('\n')
    print(json.dumps(dict(status=report['status'],evidence=str(a.output))))
    return 0 if report['status']=='passed' else 1


if __name__=='__main__': sys.exit(main())
