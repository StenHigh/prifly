#!/usr/bin/env python3
"""Prepare exact-SHA CI checks without creating a tag or publishing a release."""
import argparse
import json
import subprocess
import sys
import time
from release_checks import GitHub, REQUIRED, select


def prepare(client,sha,ref,force=False,wait=False,timeout=60,clock=time.monotonic,pause=time.sleep):
    start=clock();client.deadline=start+timeout;dispatched=set();last={};forced=set();previous={}
    while True:
        try:states={name:select(client,name,sha) for name in REQUIRED}
        except (TimeoutError,subprocess.TimeoutExpired):
            print(json.dumps({'sha':sha,'state':'pending','reason':'observation timeout; no success inferred'}),flush=True)
            return 3,last
        for name,record in states.items():
            action=record['state']
            identity=(record.get('run_id'),record.get('attempt'))
            if force and name in dispatched and action in ('success','failed') and identity==previous.get(name):
                states[name]={'state':'wait','reason':'forced batch not visible yet; old result is not its result'}
                continue
            if action=='failed' and not force:continue
            needs=(action=='missing' or action=='failed' or (force and action=='success' and name not in forced))
            if needs and name not in dispatched:
                if client.ref_sha(ref)!=sha:raise ValueError('ref moved; no dispatch for a different candidate')
                client.dispatch(name,ref,sha,force);dispatched.add(name);forced.add(name);previous[name]=identity
                record={'state':'wait','reason':'dispatch requested; await GitHub run visibility'};states[name]=record
        # Never dispatch again on a visibility gap or after a failure in this invocation.
        changed={name:r for name,r in states.items() if last.get(name)!=r}
        if changed:print(json.dumps({'sha':sha,'checks':changed},sort_keys=True),flush=True)
        last=states
        if all(r['state']=='success' for r in states.values()) and (not force or all(n in forced for n in REQUIRED)):
            return 0,states
        if any(r['state']=='failed' for r in states.values()):return 1,states
        if not wait or clock()-start>=timeout:return 3,states
        pause(min(15,max(0,timeout-(clock()-start))))


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--sha',required=True);p.add_argument('--ref',required=True)
    p.add_argument('--wait',action='store_true');p.add_argument('--force',action='store_true');p.add_argument('--timeout',type=int,default=60)
    a=p.parse_args()
    if not 0<a.timeout<=3600:p.error('timeout must be 1..3600 seconds')
    return prepare(GitHub(),a.sha,a.ref,a.force,a.wait,a.timeout)[0]


if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,KeyError,TypeError,subprocess.SubprocessError) as err:sys.exit('release checks: '+str(err))
