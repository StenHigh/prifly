const assert = require('node:assert/strict');
const monitorJSSource=require('node:fs').readFileSync(require('node:path').join(__dirname,'monitor.js'),'utf8');
const {esc,label,duration,graphData,definitionInvocation,fileChanges,nodeCard,executionRole,activityRows,partialCause,nowHTML,executionCounts,relatedTitle,expandedRuns,runStatus,stepReceipt,launchInputs,timeBreakdown,attemptPhases,resourceRows}=require('./monitor.js');
assert.equal(esc(`<img src='x' onerror="evil()">&`),'&lt;img src=&#39;x&#39; onerror=&quot;evil()&quot;&gt;&amp;');
assert.equal(label('ready'),'Подготовлен к выдаче Attempt');
assert.equal(label('pending'),'Attempt передан агенту');
assert.equal(label('running'),'Исполнение начато');
assert.equal(label('skipped'),'Будет пропущен');
assert.equal(label('future'),'Может быть выполнен');
assert.equal(label('no_work'),'Нет работы');
assert.equal(definitionInvocation({definitions:[{ref:{digest:'workflow'},bytes:'{"id":"plan"}'}],invocations:{one:{id:'actual',workflow_ref:{digest:'workflow'}}}},'plan'),'actual');
assert.equal(definitionInvocation({definitions:[{ref:{digest:'workflow'},bytes:'{"id":"plan"}'}],invocations:{one:{id:'first',workflow_ref:{digest:'workflow'}},two:{id:'second',workflow_ref:{digest:'workflow'}}}},'plan'),'__definition__');
assert.equal(duration({quality:'unknown'}),'Неизвестно');
assert.match(duration({quality:'partial',known_ms:2500}),/не менее 2.5 с/);
assert.match(duration({quality:'measured',value_ms:0}),/^0 мс/);
assert.match(duration({quality:'partial',known_ms:0,estimate_ms:300000}),/^около 5.0 мин/);
const workflow={definition:{entry:'choose',stages:{choose:{kind:'choice',branches:[{id:'yes',next:'fan'},{id:'no',next:'end'}]},fan:{kind:'parallel',branches:[{id:'left',workflow_ref:{id:'child'}},{id:'right',workflow_ref:{id:'child'}}],on:{ok:'repeat'}},repeat:{kind:'repeat',body_workflow_ref:{id:'child'},on_complete:{ok:'end'},on_limit:'end'},end:{kind:'finish'}}}};
const run={invocations:{root:{ready_stages:['fan']},child:{id:'child',branch_id:'left',caller_stage_activation_id:'activation',status:'running'}},activations:{choose:{id:'c',stage_id:'choose',workflow_invocation_id:'root',status:'completed'},fan:{id:'activation',stage_id:'fan',workflow_invocation_id:'root',status:'running'},other:{stage_id:'end',workflow_invocation_id:'other',status:'completed'}}};
const graph=graphData(workflow,run,'root');
assert.equal(graph.nodes.find(n=>n.id==='end').state,'future');
assert.equal(graph.nodes.find(n=>n.id==='fan / left').invocation,'child');
assert.equal(graph.nodes.find(n=>n.id==='fan / right').state,'future');
assert.equal(graph.nodes.filter(n=>n.kind==='repeat').length,1);
assert.ok(graph.edges.some(e=>e.from==='choose' && e.to==='fan'));
assert.ok(graph.edges.some(e=>e.from==='choose' && e.to==='fan' && e.path));
assert.ok(graph.edges.some(e=>e.from==='fan / right' && e.to==='repeat'));
const choiceGraph=graphData({definition:{entry:'choose',stages:{choose:{kind:'choice',branches:[{id:'yes',next:'yes'},{id:'no',next:'no'}]},yes:{kind:'step'},no:{kind:'step'}}}}, {}, '',[{stage_id:'choose',next_stage_id:'yes',route:'branch',branch_id:'yes'}]);
assert.equal(choiceGraph.nodes.find(n=>n.id==='no').state,'skipped');
assert.equal(choiceGraph.nodes.find(n=>n.id==='no').visual,'skipped');
assert.match(choiceGraph.nodes.find(n=>n.id==='no').reason,/Выбран маршрут yes → yes/);
assert.equal(choiceGraph.nodes.find(n=>n.id==='yes').state,'future');
const choiceEdges=choiceGraph.edges.filter(e=>e.from==='choose');
assert.equal(new Set(choiceEdges.map(e=>e.fromPort)).size,choiceEdges.length);
assert.ok(choiceEdges.find(e=>e.to==='no').muted);
const excludedGraph=graphData({definition:{entry:'choose-security',stages:{'choose-security':{kind:'choice',branches:[{id:'enabled',next:'security',predicate:{op:'eq',left:{ref:{from:'workflow_input',port:'security_enabled'}},right:{kind:'literal',value:true}}}],default:'review'},security:{kind:'step'},review:{kind:'step'}}}}, {}, '', [], {security_enabled:'false'});
assert.equal(excludedGraph.nodes.find(n=>n.id==='security').state,'skipped');
assert.equal(excludedGraph.nodes.find(n=>n.id==='review').state,'future');
const skippedCard=nodeCard(choiceGraph,'no',{context:{inputs:{brief:{}}},accepted:{outputs:{result:{}}}});
assert.match(skippedCard,/Закреплённый узел/);
assert.match(skippedCard,/Входящие переходы/);
assert.match(skippedCard,/Исходящие переходы/);
assert.match(skippedCard,/Выбран маршрут yes/);
assert.match(skippedCard,/Входы: brief/);
const arrivalGraph=graphData({definition:{entry:'plan',stages:{plan:{kind:'step',on:{no_work:'abandoned'}},abandoned:{kind:'finish'}}}},{activations:{plan:{stage_id:'plan',workflow_invocation_id:'root',status:'completed',step_instance_id:'step-plan'}}},'root');
const arrivedCard=nodeCard(arrivalGraph,'abandoned',{}, {attempts:{a:{step_instance_id:'step-plan',accepted:{summary:'План не требует работы'}}}});
assert.match(arrivedCard,/Почему выбран этот переход/);
assert.match(arrivedCard,/Нет работы.*План не требует работы/);
const readyGraph=graphData({definition:{entry:'ready',stages:{ready:{kind:'step'}}}},{ready_stages:['ready']},'');
assert.equal(readyGraph.nodes[0].visual,'reachable');
const unknownGraph=graphData({definition:{entry:'missing',stages:{missing:{kind:'step'}}}},{settled:true},'');
assert.equal(unknownGraph.nodes[0].visual,'unknown');
assert.match(unknownGraph.nodes[0].reason,/не записана/);
assert.deepEqual([...new Set(choiceEdges.map(e=>e.route))],['forward','skipped']);
const cycle=graphData({definition:{entry:'a',stages:{a:{on:{go:'b'}},b:{on:{again:'a'}}}}},{},'');
assert.equal(cycle.nodes.length,2);assert.ok(Number.isFinite(cycle.width));
assert.equal(fileChanges(null,{files:[]}),null);
assert.deepEqual(fileChanges({files:[{path:'a',ref:{digest:'old'}},{path:'deleted',ref:{digest:'d'}}]},{files:[{path:'a',ref:{digest:'new'}},{path:'new',ref:{digest:'n'}}]}),[{path:'a',change:'Изменён'},{path:'deleted',change:'Удалён'},{path:'new',change:'Добавлен'}]);
assert.deepEqual(executionRole({session:{principal_id:'host:codex'},started:null}),{kind:'Внешний host',detail:'host:codex',state:'Выдано · ожидается отчёт host'});
assert.equal(executionRole({process:{executable:'go'},started:{}},null).kind,'Локальная программа');
assert.equal(executionRole(null,{kind:'call'}).detail,'Не является fork Run');
assert.match(executionRole(null,null).kind,/не записана/);
const activity=activityRows({attempts:{one:{id:'attempt:one',session:{principal_id:'host:one'}}},activations:{ready:{id:'activation:ready',stage_id:'plan',status:'ready'}},pending_decision:{attempt_id:'attempt:one',decision_id:'decision:go'},wait_registrations:{wait:{activation_id:'activation:wait',target_stage_id:'signal',status:'active'}},fork:{source_run_id:'run:source'}});
assert.equal(activity.length,5);assert.match(activity[0].detail,/ожидается отчёт/);
assert.equal(activityRows({fork:{source_run_id:'run:source',reason:'project continuation'}})[0].title,'Продолжение от Run');
const lineage={source:'authority',run_id:'run:child',subject:'#154',fork_source_run_id:'run:parent',fork_reason:'recover_failed_stage',children:2,descendants:4,matches:3,context:true};
let row=relatedTitle(lineage,2);
assert.match(row,/--run-depth:2/);assert.match(row,/Восстановление/);assert.match(row,/Контекст поиска/);
assert.match(row,/aria-expanded="false"/);assert.match(row,/data-expand=/);assert.match(row,/href="#source=authority&amp;run=run%3Achild"/);
expandedRuns.add('authority|run:child');row=relatedTitle(lineage,2);assert.match(row,/aria-expanded="true"/);
assert.match(relatedTitle({...lineage,children:0,missing_parent:true,fork_source_run_id:'run:deleted'},0),/Исходный Run недоступен: run:deleted/);
assert.match(relatedTitle({...lineage,fork_source_run_id:'',children:0},0),/#154/);
assert.match(relatedTitle({...lineage,children:0},3),/run-lineage is-child/);
expandedRuns.clear();
const branchStatus=runStatus({source:'authority',status:'failed',latest_descendant:{run_id:'run:recovery',status:'completed',outcome:'succeeded',last_observed:'2026-09-23T00:03:20Z'}});
assert.match(branchStatus,/Этот Run/);assert.match(branchStatus,/Ошибка/);
assert.match(branchStatus,/Последний производный Run/);assert.match(branchStatus,/Выполнен/);
assert.match(branchStatus,/href="#source=authority&amp;run=run%3Arecovery"/);
const recovered={root_workflow_invocation_id:'root',recovery:{source_run_id:'run:source',frontier_stage_id:'tests',reused:[{stage_id:'verify'}],root_output_refs:{verify:{implementation:{artifact_id:'artifact:old'}}}}};
assert.equal(graphData({definition:{entry:'verify',stages:{verify:{kind:'call',on:{succeeded:'tests'}},tests:{kind:'step'}}}},recovered,'root').nodes[0].state,'reused');
assert.match(activityRows(recovered)[0].detail,/1 узлов взято/);

// What a step received: every fact comes from the stored Run, and one it does
// not hold reads as not recorded rather than as an empty answer.
{
 const catalog={decisions:[
  {id:'plan_profile',title:'Planning depth',phase:'preflight',choices:[{title:'Fast',value:'fast'}],destination:{kind:'package_profile'}},
  {id:'improve_apply',title:'Apply improvements',phase:'runtime',choices:[{title:'All',value:'all'},{title:'None',value:'none'}],destination:{kind:'session_context',name:'improve_apply'}},
  {id:'plan_docs',title:'Docs checkpoint',phase:'preflight',choices:[{title:'Require',value:true}],destination:{kind:'session_context',name:'plan_docs'},when:{profiles:['full']}},
 ]};
 const step={id:'aif:step/plan',instructions_ref:{id:'bridge',digest:'d:bridge'},context_refs:[{id:'skill',digest:'d:skill'}],inputs:{task:{},handoff:{}},outputs:{plan:{schema_ref:{id:'plan-schema'},required_for:['pass']}},effects:{class:'workspace_write',retry_class:'never'}};
 const workflowDef={definition:{stages:{plan:{kind:'step',on:{pass:'done',fail:'done'},input_bindings:{task:{from:'workflow_input',port:'task'},handoff:{from:'stage_output',stage_id:'warmup',port:'handoff'}}}}}};
 const run={schema_version:'core-state/41',workflow_ref:{digest:'d:wf'},definitions:[{ref:{digest:'d:wf'},bytes:JSON.stringify(workflowDef)},{ref:{digest:'d:step'},bytes:JSON.stringify(step)}],
  context_resources:[{ref:{digest:'d:bridge'},bytes:'Bridge text'},{ref:{digest:'d:skill'},bytes:'Skill text'}],
  decision_catalog:catalog,decision_sheet:{package_profile:'fast',profile_source:'actor',decision_policy:'autonomous',records:[{definition_id:'improve_apply',source:'project_default',value:'all'}]},
  decision_ledger:[{definition_id:'improve_apply',source:'project_default',status:'answered',value:'all'}],
  activations:{act:{id:'act',stage_id:'plan'},warm:{id:'warm',stage_id:'warmup'}},steps:{s:{id:'s',definition_ref:{digest:'d:step'},stage_activation_id:'act',attempt_ids:['a']}},
  attempts:{a:{id:'a',stage_activation_id:'act',step_instance_id:'s',context:{inputs:{task:{ref:{artifact_id:'artifact:task',revision:1,digest:'d:task'}},handoff:{ref:{artifact_id:'artifact:h',revision:1,digest:'d:h'}}}},
   session:{decision_context:{'core:package_profile':'fast',improve_apply:'all'},skill_refs:[{id:'bridge',digest:'d:bridge'},{id:'skill',digest:'d:skill'}],workspace_trees:[{capture:{path:'.ai-factory/PLAN.md'}}]},
   question_report:{questions:[{question:'Which runner?',answer:'phpunit',basis:'instructions'},{question:'Apply?',answer:'all',basis:'decision',decision_id:'improve_apply'}]}}}};
 const r=stepReceipt(run,run.attempts.a);
 assert.deepEqual(r.inputs.map(i=>[i.port,i.origin.kind,i.origin.stage||i.origin.port]),[['handoff','stage_output','warmup'],['task','workflow_input','task']]);
 assert.deepEqual(r.decisions.map(d=>[d.id,d.choice,d.source]),[['plan_profile','Fast','actor'],['improve_apply','All','project_default']]);
 assert.deepEqual(r.texts.map(t=>[t.role,t.text]),[['instructions','Bridge text'],['context','Skill text']]);
 assert.deepEqual(r.expects.verdicts,['pass','fail']);
 assert.deepEqual(r.expects.trees,['.ai-factory/PLAN.md']);
 assert.equal(r.reported.length,2);
 assert.equal(r.program,false);
 // A Run written before 41 kept no list and no delivered answers: both read
 // as not recorded, never as "none".
 const older=JSON.parse(JSON.stringify(run));older.schema_version='core-state/40';delete older.attempts.a.question_report;delete older.attempts.a.session.decision_context;
 const o=stepReceipt(older,older.attempts.a);
 assert.equal(o.reported,null);assert.equal(o.reportsKept,false);assert.equal(o.decisions,null);
 // An empty report is the host's statement that there were none.
 const empty=JSON.parse(JSON.stringify(run));empty.attempts.a.question_report={questions:[]};
 assert.deepEqual(stepReceipt(empty,empty.attempts.a).reported,[]);
 // A program step is handed no answers and no instructions.
 const program=JSON.parse(JSON.stringify(run));delete program.attempts.a.session;
 assert.equal(stepReceipt(program,program.attempts.a).program,true);
 // A question asked through Pri-Fly during the attempt, and one still waiting.
 const asked=JSON.parse(JSON.stringify(run));asked.decision_ledger.push({definition_id:'improve_apply',attempt_id:'a',status:'answered',source:'actor',value:'none',requested:{utc:'2026-09-30T10:00:00Z'},observed:{utc:'2026-09-30T10:05:00Z'}});asked.pending_decision={attempt_id:'a',decision_id:'improve_apply'};
 const q=stepReceipt(asked,asked.attempts.a);
 assert.deepEqual(q.asked.map(x=>[x.id,x.choice,x.source]),[['improve_apply','None','actor']]);
 assert.equal(q.pending,'improve_apply');
 const launch=launchInputs(run,{task:{title:'Split files'},security_enabled:false});
 assert.deepEqual(launch.decisions.map(d=>[d.id,d.answered,d.excluded,d.source]),[['plan_profile',true,false,'actor'],['improve_apply',true,false,'project_default'],['plan_docs',false,true,null]]);
 assert.deepEqual(launch.inputs.map(i=>[i.port,i.source]),[['task','launch'],['security_enabled','launch']]);
}
// Where time went: only numbers the engine published, sorted by stage.
{
 const d=(ms,q='measured')=>q==='measured'?{quality:q,value_ms:ms}:{quality:q,estimate_ms:ms};
 const timing={root:{metrics:{elapsed:d(10000,'estimated'),host_work_sum:d(6000,'estimated'),idle:d(1000,'estimated'),executor_sum:{quality:'not_applicable'}},children:[{kind:'workflow_invocation',children:[
  {kind:'stage_activation',stage_id:'verify',metrics:{elapsed:d(2000),host_work_sum:d(1500)}},
  {kind:'stage_activation',stage_id:'verify',metrics:{elapsed:d(1000),host_work_sum:d(800)}},
  {kind:'stage_activation',stage_id:'implement',metrics:{elapsed:d(5000),host_work_sum:d(4000)}},
  {kind:'stage_activation',stage_id:'done',stage_kind:'finish',metrics:{elapsed:d(0)}},
 ]}]}};
 const t=timeBreakdown(timing);
 assert.deepEqual(t.categories.map(c=>c.key),['host_work_sum','idle']);
 assert.deepEqual(t.stages.map(s=>[s.stage,s.count,s.ms,s.agentMs]),[['implement',1,5000,4000],['verify',2,3000,2300]]);
 assert.equal(timeBreakdown({root:{metrics:{elapsed:d(1)}}}),null);
 assert.deepEqual(attemptPhases({metrics:{host_work:d(1),executor_time:{quality:'unavailable'}}},true).map(p=>p.key),['host_work']);
}
// Exclusive resources: what this Run sealed, and who holds each now.
{
 const run={stage_resources:{tests:['heavy'],verify:['heavy','db']},resource_limits:{heavy:1,db:2},attempts:{'attempt:mine':{}}};
 const rows=resourceRows(run,{held:{'attempt:mine':'run:this','attempt:other':'run:other'},resources:{heavy:['attempt:other']}});
 assert.deepEqual(rows.map(r=>[r.name,r.limit,r.stages.join(',')]),[['db',2,'verify'],['heavy',1,'tests,verify']]);
 assert.deepEqual(rows[1].holders,[{attempt:'attempt:other',run:'run:other',mine:false}]);
 assert.deepEqual(rows[0].holders,[]);
 assert.equal(resourceRows(run,undefined)[0].holders,null);
 assert.deepEqual(resourceRows({},{}),[]);
}
// Execution counts are two labelled numbers, never a fraction or a progress.
{
 assert.deepEqual(executionCounts({step_instances:4,attempts:4,settled_attempts:3,awaiting_hosts:1}),{steps:4,attempts:4,settled:3,open:1,hosts:1});
 assert.deepEqual(executionCounts({step_instances:1,attempts:2,settled_attempts:2,awaiting_hosts:0}),{steps:1,attempts:2,settled:2,open:0,hosts:0});
 assert.deepEqual(executionCounts({step_instances:2,attempts:1,settled_attempts:3}),{steps:2,attempts:1,settled:null,open:null,hosts:null},'more settled than made is unknown, not negative');
 assert.deepEqual(executionCounts({}),{steps:null,attempts:null,settled:null,open:null,hosts:null},'an absent count is unknown, not zero');
 assert.doesNotMatch(monitorJSSource,/step_instances \?\? '—'\)\} \/ /,'the list cell went back to a fraction');
}
// A terminal Run says it is over; a partial one names only a proven cause.
{
 const workflow=JSON.stringify({definition:{stages:{review:{kind:'step',on:{pass:'done',needs_revision:'stop'}},done:{kind:'finish',outcome:'succeeded'},stop:{kind:'finish',outcome:'partial'}}}});
 const base={id:'run:p',status:'completed',outcome:'partial',workflow,root_workflow_invocation_id:'inv:root',
  activations:{'act:review':{id:'act:review',stage_id:'review',kind:'step',status:'completed',workflow_invocation_id:'inv:root',step_instance_id:'step:review'},'act:stop':{id:'act:stop',stage_id:'stop',kind:'finish',status:'completed',workflow_invocation_id:'inv:root'}},
  steps:{'step:review':{id:'step:review',verdict:'needs_revision',attempt_ids:['attempt:r'],outputs:{report:{}}}},
  attempts:{'attempt:r':{id:'attempt:r',settled:{},accepted:{summary:'Two blocking findings'}}}};
 assert.deepEqual(partialCause(base),{kind:'verdict',finish:'stop',description:'',stage:'review',verdict:'needs_revision',node:'attempt:r',summary:'Two blocking findings',outputs:['report']});
 const html=nowHTML(base,activityRows(base));
 assert.match(html,/ничего не исполняется, и сам он не продолжится/);assert.match(html,/data-node="attempt:r"/);assert.match(html,/Two blocking findings/);
 assert.doesNotMatch(html,/не записаны|Ожидает агента/);
 // Two steps route to the same finish: no single one is blamed.
 const twice={...base,activations:{...base.activations,'act:again':{id:'act:again',stage_id:'review',kind:'step',status:'completed',workflow_invocation_id:'inv:root',step_instance_id:'step:again'}},steps:{...base.steps,'step:again':{id:'step:again',verdict:'needs_revision',attempt_ids:[]}}};
 assert.equal(partialCause(twice).kind,'unknown');assert.match(nowHTML(twice,[]),/Причина не установлена/);
 assert.equal(partialCause({...base,activations:{}}).kind,'unknown','no finish reached');
 // A waiver lowered a finish declared succeeded.
 const waived={...base,waiver_applied:true,waivers:[{reason:'flaky e2e',approver_id:'owner'}],activations:{...base.activations,'act:stop':{...base.activations['act:stop'],stage_id:'done'}}};
 assert.equal(partialCause(waived).kind,'waiver');assert.match(nowHTML(waived,[]),/flaky e2e/);
 // A call whose child ended partial routes to the finish; the cause is followed into the child.
 const child=JSON.stringify({definition:{stages:{check:{kind:'step',on:{blocked:'halt'}},halt:{kind:'finish',outcome:'partial'}}}});
 const nested={id:'run:n',status:'completed',outcome:'partial',root_workflow_invocation_id:'inv:root',
  workflow:{definition:{stages:{verify:{kind:'call',on:{partial:'fix',succeeded:'done'}},fix:{kind:'finish',outcome:'partial'},done:{kind:'finish',outcome:'succeeded'}}}},
  definitions:[{ref:{digest:'sha256:child'},bytes:child}],
  invocations:{'inv:root':{id:'inv:root',outcome:'partial'},'inv:child':{id:'inv:child',parent_invocation_id:'inv:root',caller_stage_activation_id:'act:verify',workflow_ref:{digest:'sha256:child'},outcome:'partial'}},
  activations:{'act:verify':{id:'act:verify',stage_id:'verify',kind:'call',status:'completed',workflow_invocation_id:'inv:root'},'act:fix':{id:'act:fix',stage_id:'fix',kind:'finish',status:'completed',workflow_invocation_id:'inv:root'},
   'act:check':{id:'act:check',stage_id:'check',kind:'step',status:'completed',workflow_invocation_id:'inv:child',step_instance_id:'step:check'},'act:halt':{id:'act:halt',stage_id:'halt',kind:'finish',status:'completed',workflow_invocation_id:'inv:child'}},
  steps:{'step:check':{id:'step:check',verdict:'blocked',attempt_ids:['attempt:c']}},attempts:{'attempt:c':{id:'attempt:c',accepted:{summary:'port busy'}}}};
 const found=partialCause(nested);
 assert.deepEqual([found.kind,found.stage,found.outcome,found.node,found.inner.kind,found.inner.stage,found.inner.verdict,found.inner.summary],['call','verify','partial','inv:child','verdict','check','blocked','port busy']);
 assert.match(nowHTML(nested,[]),/вложенный workflow[^]*port busy/);
 // A recorded choice that routes to the finish is the cause; the finish's own description is shown.
 const chosen={...base,workflow:JSON.stringify({definition:{stages:{decide:{kind:'choice'},owner:{kind:'finish',outcome:'partial',description:'The developer decides'}}}}),
  activations:{'act:decide':{id:'act:decide',stage_id:'decide',kind:'choice',status:'completed',workflow_invocation_id:'inv:root'},'act:owner':{id:'act:owner',stage_id:'owner',kind:'finish',status:'completed',workflow_invocation_id:'inv:root'}}};
 const choices=[{stage_activation_id:'act:decide',branch_id:'owner',next_stage_id:'owner'}];
 assert.deepEqual(partialCause(chosen,choices),{kind:'choice',finish:'owner',description:'The developer decides',stage:'decide',branch:'owner',node:'act:decide'});
 assert.equal(partialCause(chosen).kind,'unknown','a choice without its recorded decision is not guessed');
 assert.match(nowHTML(chosen,[],choices),/выбрала ветвь <b>owner<\/b>[^]*The developer decides/);
 // Other terminal outcomes get no partial explanation.
 const rejected=nowHTML({...base,outcome:'rejected'},[]);
 assert.match(rejected,/не продолжится/);assert.doesNotMatch(rejected,/partial|Причина/);
 // A live Run with nothing recorded still says so; a stray unsettled row on a terminal one is not shown as work.
 assert.match(nowHTML({status:'running'},[]),/не записаны/);
 assert.doesNotMatch(nowHTML({...base,outcome:'succeeded'},[{node:'attempt:x',title:'Внешний host',detail:'host:x'}]),/host:x/);
}
// Roles and the admission queue.
{
 assert.equal(executionRole({session:{principal_id:'host:a',host_state:'waiting_admission'}}).state,'Ожидает допуска: занята capacity authority');
 assert.equal(executionRole({session:{principal_id:'host:a',host_state:'waiting_decision'}}).state,'Ожидает решения оператора');
 const two=activityRows({attempts:{a:{id:'attempt:a',session:{principal_id:'host:a'}},b:{id:'attempt:b',session:{principal_id:'host:b'}}}});
 assert.deepEqual(two.map(r=>[r.node,r.detail]),[['attempt:a','host:a · Выдано · ожидается отчёт host'],['attempt:b','host:b · Выдано · ожидается отчёт host']]);
 const queued=activityRows({id:'run:b'},{capacity:1,waiting:{'run:c':9,'run:b':7,'run:a':3}});
 assert.deepEqual(queued,[{title:'Ожидает допуска',detail:'место 2 из 3 в очереди authority · capacity 1'}]);
 assert.deepEqual(activityRows({id:'run:z'},{capacity:1,waiting:{'run:a':1}}),[]);
 const decided=activityRows({pending_decision:{attempt_id:'attempt:q',decision_id:'decision:go'}});
 assert.deepEqual(decided,[{node:'attempt:q',title:'Ожидается решение',detail:'decision:go'}]);
}
console.log('monitor UI: status wording, escaping, timing quality, graph paths/ports/parallel/repeat/cycles, file evidence passed');

// Exercise the actual pre-paint script, including browsers that deny storage.
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const monitorHTML=fs.readFileSync(path.join(__dirname,'monitor.html'),'utf8');
const monitorCSS=fs.readFileSync(path.join(__dirname,'monitor.css'),'utf8');
const monitorJS=fs.readFileSync(path.join(__dirname,'monitor.js'),'utf8');
assert.match(monitorHTML,/<select name="view"[^>]*><option value="related">/);
assert.match(monitorHTML,/<option value="related-newest">Связанные Run · ранние снизу/);
assert.match(monitorHTML,/<option value="flat">Плоский список/);
assert.match(monitorCSS,/\.run-lineage\.is-child::before/);
assert.match(monitorCSS,/@media\(max-width:1100px\).*\.run-table\{min-width:820px\}/);
// The execution column's meaning is visible text before the scrolling table, not a hover title.
assert.match(monitorHTML,/<p class="muted table-note" id="execution-note">[^<]*не доля сценария\.<\/p><div class="table-wrap"><table class="run-table" aria-describedby="execution-note">/);
assert.match(monitorHTML,/<th>Исполнение<\/th>/);
assert.doesNotMatch(monitorJS,/function executionCell[^\n]*badge/,'a settled count must not be coloured as success');
assert.match(monitorJS,/runStatus\(run\)/);assert.match(monitorJS,/run\.outcome/);
assert.match(monitorJS,/window\.scrollBy\(0,current\.getBoundingClientRect\(\)\.top-anchorTop\)/);
assert.match(monitorJS,/if\(child\.context && child\.children\)expandedRuns\.add/);
const themeSource = fs.readFileSync(require('node:path').join(__dirname,'monitor-theme.js'),'utf8');
function themeSession(saved, dark, blocked=false) {
 const callbacks={},media={matches:dark,addEventListener:(_,fn)=>callbacks.system=fn};
 const select={value:'',addEventListener:(_,fn)=>callbacks.select=fn};
 const document={documentElement:{dataset:{}},addEventListener:(_,fn)=>callbacks.ready=fn,getElementById:()=>select};
 const localStorage={getItem:()=>{if(blocked)throw Error('denied');return saved;},setItem:(_,value)=>{if(blocked)throw Error('denied');saved=value;}};
 vm.runInNewContext(themeSource,{window:{matchMedia:()=>media},document,localStorage});
 const firstPaint=document.documentElement.dataset.theme;
 callbacks.ready();
 return {document,firstPaint,select,change(value){select.value=value;callbacks.select();},system(value){media.matches=value;callbacks.system();},saved:()=>saved};
}
const automatic=themeSession(null,true);
assert.equal(automatic.firstPaint,'dark');assert.equal(automatic.select.value,'system');
automatic.system(false);assert.equal(automatic.document.documentElement.dataset.theme,'light');
automatic.change('dark');automatic.system(false);
assert.equal(automatic.document.documentElement.dataset.theme,'dark');
assert.equal(themeSession(automatic.saved(),false).firstPaint,'dark');
automatic.change('system');assert.equal(automatic.document.documentElement.dataset.theme,'light');
assert.equal(themeSession('invalid',true).firstPaint,'dark');
const denied=themeSession(null,false,true);denied.change('dark');
assert.equal(denied.document.documentElement.dataset.theme,'dark');
console.log('monitor themes: pre-paint, persistence, system changes and denied storage passed');
