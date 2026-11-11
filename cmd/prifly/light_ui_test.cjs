const assert = require('node:assert/strict');
const {lightExplanation, lightReasonRun} = require('./light.js');

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
