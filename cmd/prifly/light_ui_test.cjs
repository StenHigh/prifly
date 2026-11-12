const assert = require('node:assert/strict');
const {lightExplanation, lightReasonRun, observedLightRun, lightRunReason, lightRunTarget} = require('./light.js');

const expired={code:'attempt_deadline_expired',text:'Диагностика: attempt_deadline_expired',attempt_id:'attempt:a',action:'journal'};
const other={code:'resource_busy',text:'Ресурс <test> занят',stage_activation_id:'act:b',action:'facts'};
const runs=[
 {run_id:'run:a',board:{reasons:[expired],stages:[]}},
 {run_id:'run:b',board:{reasons:[other],stages:[]}}
];
const mixed={source:'source:one',key:'task:one',status:'mixed',reasons:[expired,other],current_runs:runs,current_count:2};
const detail=(source,key)=>'#'+new URLSearchParams({source,key});
const full=(source,run,tab)=>'/full/#'+new URLSearchParams({source,run,tab});
assert.equal(lightReasonRun(mixed,other),'run:b');
const html=lightExplanation(mixed,detail,full);
assert.match(html,/Ресурс &lt;test&gt; занят/);
assert.match(html,/Истёк срок попытки.*attempt_deadline_expired/);
assert.match(html,/run=run%3Ab&amp;tab=facts/);
assert.match(html,/Другие причины · 1/);
assert.doesNotMatch(html,/<test>/);
const ambiguous={...mixed,current_runs:[...runs,{run_id:'run:c',board:{reasons:[other]}}]};
assert.equal(lightReasonRun(ambiguous,other),null);
assert.match(lightExplanation(ambiguous,detail,full),/#source=source%3Aone&amp;key=task%3Aone/);
assert.equal(lightReasonRun(mixed,{code:'review_gate_blocked',action:'command'}),null);
assert.match(lightExplanation({source:'s',key:'k',status:'failed'},detail,full),/Причина текущего состояния не записана/);

const pin={source:mixed.source,key:mixed.key,run:'run:a'};
const compact={...runs[0],status:'running'};
const detailed={...runs[0],status:'failed',board:{reasons:[expired],stages:[{
 stage_activation_id:'act:a',workflow_invocation_id:'inv:child',attempts:[{attempt_id:'attempt:a'}]
}]}};
const data={source:{source:pin.source},card:{...mixed,current_runs:[compact,runs[1]]},runs:[detailed]};
assert.equal(observedLightRun(data,pin),detailed);
assert.equal(observedLightRun({...data,runs:[]},pin),compact);
assert.equal(observedLightRun(data,{...pin,source:'source:other'}),null);
assert.equal(observedLightRun(data,{...pin,run:'run:missing'}),null);
assert.deepEqual(lightRunTarget(detailed,lightRunReason(detailed)),{
 tab:'workflow',node:'attempt:a',invocation:'inv:child',label:'Открыть попытку в Full'
});
assert.deepEqual(lightRunTarget(runs[1],other),{tab:'facts',label:'Результаты и решения в Full'});
