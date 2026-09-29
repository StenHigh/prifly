'use strict';
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const labels = {running:'В работе',completed:'Завершён',failed:'Ошибка',cancelled:'Отменён',uncertain:'Неопределённость',waiting:'Ожидание',pending:'Ожидает',ready:'Готов',succeeded:'Выполнен',rejected:'Отклонён',stopping:'Останавливается',awaiting_host:'Ожидает агента',reported:'Результат получен',disconnected:'Связь потеряна',settled:'Завершено',future:'Ещё не выполнен',unchosen:'Ветвь не выбрана',not_taken:'Не выполнен в этом исполнении',workflow_invocation:'WorkflowInvocation',stage_activation:'StageActivation',step_instance:'StepInstance',attempt:'Attempt',run:'Run',check_execution:'Проверка',step:'Шаг',choice:'Развилка',parallel:'Параллельные ветви',call:'Вложенный workflow',repeat:'Повтор',map:'Обработка элементов',wait:'Ожидание события',finish:'Завершение',elapsed:'Общее время',executor:'Работа исполнителя',queue:'Ожидание допуска',active:'Активное время',quality:'Качество измерения',measured:'Измерено',estimated:'Оценка',partial:'Частично',unknown:'Неизвестно',not_applicable:'Не применимо',unavailable:'Недоступно',incomparable_clock_domains:'Разные сессии часов',calendar_suspend_coverage_unqualified:'Сон машины не учтён достоверно',authority_wall_estimate:'Оценка по календарным часам authority',executor_time:'Работа исполнителя',executor_sum:'Сумма времени исполнителей',executor_active_union:'Интервалы активности исполнителей',check_executor_sum:'Сумма времени проверок',check_executor_active_union:'Интервалы активности проверок',ready_queue:'Ожидание в очереди',restricted_time:'Время ограничений',dispatch_latency:'Задержка передачи',result_to_acceptance:'От результата до приёмки',post_execution_settlement:'Завершение после исполнения',output_contracts:'Требования к результату',subject:'Задача',desired_outcome:'Ожидаемый результат',in_scope:'Входит в задачу',out_of_scope:'За пределами задачи',completion_criteria:'Критерии завершения',source_refs:'Источники',assumptions:'Предположения',confirmation:'Подтверждение',summary:'Описание результата',verdict:'Вердикт',outputs:'Выходы',inputs:'Входы',instructions:'Инструкции',context:'Контекст',description:'Описание',status:'Состояние',reason:'Причина',message:'Сообщение',code:'Код',severity:'Важность',phase:'Фаза',observed:'Зафиксировано',actor:'Автор',actor_id:'Автор',amount:'Сумма',currency:'Валюта',source:'Источник',reported_costs:'Заявленная стоимость',permitted_effects:'Разрешённые действия',workspace:'Рабочая папка',skill_refs:'Навыки',host_state:'Состояние агента',deadline:'Срок',process_outcome:'Итог процесса',error:'Ошибка',evidence_refs:'Свидетельства',effect_receipt_refs:'Подтверждения эффектов'};
Object.assign(labels,{host_take_not_recorded:'агент не отметил, когда взял задание (session take)',decision_request_time_not_recorded:'момент вопроса не записан в этом Run',host_work_needs_session_and_wait:'нужны время сессии и ожидание ответа',difference_of_intervals:'разность интервалов',executor_start_not_observed:'старт исполнителя не наблюдался'});
Object.assign(labels,{ready:'Подготовлен к выдаче Attempt',pending:'Attempt передан агенту',running:'Исполнение начато',future:'Может быть выполнен',skipped:'Будет пропущен',reused:'Из исходного Run',revalidated:'Перепроверен без запуска',no_work:'Нет работы'});
const label = s => labels[s] || s;
const badge = s => `<span class="badge ${/^[a-z_]+$/.test(s || '') ? s : ''}">${esc(label(s) || 'Не записано')}</span>`;
const parseMaybe = value => { if(typeof value !== 'string') return value; try{return JSON.parse(value);}catch{return value;} };
const values = o => Object.values(o || {}).filter(Boolean);
const short = s => String(s || '').replace(/^[a-z_]+:/,'').slice(0,12);
function executionRole(object,stage) {
 if(object?.session)return {kind:'Внешний host',detail:object.session.principal_id || 'host не записан',state:object.started?'Исполнение начато':'Выдано · ожидается отчёт host'};
 if(object?.process)return {kind:'Локальная программа',detail:object.process.executable || 'программа не записана',state:object.started?'Исполнение начато':'Процесс ещё не начал работу'};
 if(stage?.kind==='call')return {kind:'Вложенный workflow',detail:'Не является fork Run',state:label(object?.status || 'future')};
 if(stage?.kind)return {kind:'Управляющий узел workflow',detail:label(stage.kind),state:label(object?.status || 'future')};
 return {kind:'Роль не записана',detail:'Нельзя определить по сохранённым фактам',state:''};
}
function activityRows(run) {const rows=[];for(const a of values(run.attempts).filter(a=>!a.settled)){const r=executionRole(a);rows.push({node:a.id,title:r.kind,detail:r.detail+' · '+r.state});}for(const a of values(run.activations).filter(a=>a.status==='ready'))rows.push({node:a.id,title:'Готово к выдаче',detail:a.stage_id});if(run.pending_decision)rows.push({node:run.pending_decision.attempt_id,title:'Ожидается решение',detail:run.pending_decision.decision_id});for(const w of values(run.wait_registrations).filter(w=>w.status==='active'))rows.push({node:w.activation_id,title:'Ожидается сигнал',detail:w.target_stage_id});if(run.recovery)rows.push({run:run.recovery.source_run_id,title:'Восстановлен от Run',detail:`${run.recovery.reused.length} узлов взято из источника · ${run.recovery.frontier_stage_id}: ${run.recovery.frontier_action==='revalidate'?'перепроверен без запуска':'новое исполнение'}`});else if(run.fork)rows.push({run:run.fork.source_run_id,title:run.fork.reason==='project continuation'?'Продолжение от Run':'Run создан как fork',detail:run.fork.source_run_id});return rows;}
const ms = n => n >= 60000 ? `${(n/60000).toFixed(1)} мин` : n >= 1000 ? `${(n/1000).toFixed(1)} с` : `${n} мс`;
function spreadPorts(edges,key,port) {
 const groups=new Map();for(const edge of edges){const group=groups.get(edge[key])||[];group.push(edge);groups.set(edge[key],group);}
 for(const group of groups.values())group.forEach((edge,index)=>edge[port]=(index-(group.length-1)/2)*14);
}
function duration(d,withReasons=false) {
 if(!d) return 'Не записано';
 const text = d.value_ms != null ? ms(d.value_ms) : d.estimate_ms != null ? `около ${ms(d.estimate_ms)}` : d.known_ms != null ? `не менее ${ms(d.known_ms)}` : label(d.quality || 'unknown');
 return `${text}${d.is_open ? ' · интервал открыт' : ''}${d.quality && d.quality !== 'measured' && (d.value_ms!=null || d.known_ms!=null || d.estimate_ms!=null) ? ' · '+label(d.quality) : ''}${withReasons && d.reasons?.length ? ' · '+d.reasons.map(label).join(', ') : ''}`;
}
const date = s => s ? new Date(s).toLocaleString('ru-RU') : 'Не записано';
function graphData(workflow, run, invocation, choices=[], inputValues={}) {
 const stages = workflow?.definition?.stages || {}, nodes = [], edges = [];
 const inv = run.invocations?.[invocation];
 const activations = values(run.activations).filter(a => (a.workflow_invocation_id || '') === (invocation || ''));
 for(const [id,stage] of Object.entries(stages)) {
  const actual = activations.filter(a=>a.stage_id===id);
  const recovery=invocation===run.root_workflow_invocation_id && run.recovery;
  const inherited=recovery && !!recovery.root_output_refs?.[id];
  const state = actual.at(-1)?.status || (inherited?(recovery.frontier_action==='revalidate' && id===recovery.frontier_stage_id?'revalidated':'reused'):invocation==='__definition__' ? 'future' : ((inv?.ready_stages || run.ready_stages || []).includes(id) ? 'ready' : (inv?.settled || (!inv && run.settled)) ? 'not_taken' : 'future'));
  nodes.push({id,title:id,kind:stage.kind,state,actual,stage,visual:'future',reason:''});
  const add = (to,reason) => {if(typeof to==='string' && stages[to]) edges.push({from:id,to,label:reason});};
  for(const key of (stage.kind==='parallel' && stage.branches?.length ? [] : ['on','on_complete'])) for(const [reason,to] of Object.entries(stage[key] || {})) add(to,reason);
  for(const key of ['on_error','on_limit','on_event','on_timeout','default','on_unknown']) add(stage[key],key);
  for(const branch of stage.branches || []) {
   if(branch.next) add(branch.next,branch.id);
   if(branch.workflow_ref) {
    const branchID=id+' / '+branch.id;
    const children=values(run.invocations).filter(i=>i.branch_id===branch.id && actual.some(a=>i.caller_stage_activation_id===a.id));
    nodes.push({id:branchID,title:branch.id,kind:'call',state:children.at(-1)?.status || 'future',ref:branch.workflow_ref,invocation:children.at(-1)?.id,stage:branch});
    edges.push({from:id,to:branchID,label:'параллельно'});
    for(const [reason,to] of Object.entries(stage.on || stage.on_complete || {})) if(stages[to]) edges.push({from:branchID,to,label:'join · '+reason});
   }
  }
 }
 const decisions=new Map(choices.filter(c=>(c.workflow_invocation_id || '')===(invocation || '')).map(c=>[c.stage_id,c]));
 for(const [id,stage] of Object.entries(stages))if(stage.kind==='choice'&&!decisions.has(id)){const branches=stage.branches||[],known=branches.every(b=>{const p=b.predicate,v=p?.left?.ref;return p?.op==='eq'&&v?.from==='workflow_input'&&v.port&&p?.right?.kind==='literal'&&Object.hasOwn(inputValues,v.port);}),branch=branches.find(b=>{const p=b.predicate,v=p?.left?.ref;if(p?.op!=='eq'||v?.from!=='workflow_input'||!v.port||p?.right?.kind!=='literal')return false;try{return JSON.stringify(JSON.parse(inputValues[v.port]))===JSON.stringify(p.right.value);}catch{return false;}});if(branch)decisions.set(id,{stage_id:id,route:'branch',branch_id:branch.id,next_stage_id:branch.next});else if(known&&stage.default)decisions.set(id,{stage_id:id,route:'default',branch_id:'default',next_stage_id:stage.default});}
 for(const edge of edges) {const decision=decisions.get(edge.from);if(!decision)continue;const selected=decision.route==='branch'?decision.branch_id:decision.route;edge.unchosen=edge.to!==decision.next_stage_id || edge.label!==selected;if(edge.unchosen)edge.reason=`Выбран маршрут ${selected}${decision.next_stage_id?` → ${decision.next_stage_id}`:''}`;}
 const reachable=filtered=>{const found=new Set([workflow?.definition?.entry]),pending=[workflow?.definition?.entry];for(let i=0;i<pending.length;i++)for(const e of edges)if(e.from===pending[i] && (!filtered || !e.unchosen) && !found.has(e.to)){found.add(e.to);pending.push(e.to);}return found;};
 const all=reachable(false),chosen=reachable(true),byID=new Map(nodes.map(n=>[n.id,n])),skipped=new Map();
 for(const edge of edges)if(edge.unchosen)skipped.set(edge.to,edge.reason);
 for(const [id,reason] of skipped)for(const edge of edges)if(edge.from===id&&!skipped.has(edge.to))skipped.set(edge.to,reason);
 for(const node of nodes) {
  if(node.state==='reused' || node.state==='revalidated') {node.visual='completed';node.reason=node.state==='reused'?'Принято в исходном Run; повторно не исполнялось':'Сохранённые bytes прошли контракт новой версии без запуска процесса';}
  else if((node.actual||[]).length) node.visual=['completed','failed','cancelled'].includes(node.state)?'completed':'current';
  else if(all.has(node.id)&&!chosen.has(node.id)) {node.visual='skipped';node.reason=skipped.get(node.id)||'Причина пропуска не записана';node.state='skipped';}
  else if(node.state==='ready') {node.visual='reachable';node.reason='Подготовлен к выдаче Attempt';}
  else if(node.state==='not_taken') {node.visual='unknown';node.reason='Причина отсутствия исполнения не записана';}
 }
 // ponytail: a breadth-first layout handles cycles without a graph dependency; dense graphs remain scrollable.
 const rank = new Map(), queue = [];
 if(stages[workflow?.definition?.entry]) {rank.set(workflow.definition.entry,0);queue.push(workflow.definition.entry);}
 for(let i=0;i<queue.length;i++)for(const edge of edges.filter(e=>e.from===queue[i]&&!e.unchosen))if(!rank.has(edge.to)){rank.set(edge.to,rank.get(edge.from)+1);queue.push(edge.to);}
 const columns=new Map();for(const node of nodes){const column=rank.get(node.id)??0,group=columns.get(column)||[];group.push(node);columns.set(column,group);}
 let maxRank=0,maxY=0;
 for(const [column,group] of columns) {maxRank=Math.max(maxRank,column);group.sort((a,b)=>['completed','current','reachable','future','unknown','skipped'].indexOf(a.visual)-['completed','current','reachable','future','unknown','skipped'].indexOf(b.visual)||a.id.localeCompare(b.id));let main=0,side=0,terminal=0;for(const node of group){node.x=30+column*300;if(node.kind==='finish'||node.visual==='skipped')node.y=360+terminal++*100;else if(['completed','current','reachable'].includes(node.visual))node.y=105+main++*100;else node.y=245+side++*100;maxY=Math.max(maxY,node.y);}}
 for(const edge of edges) {const from=byID.get(edge.from),to=byID.get(edge.to);edge.path=!edge.unchosen&&['completed','current','reachable'].includes(from?.visual)&&['completed','current','reachable'].includes(to?.visual);edge.muted=!!edge.unchosen||['future','unknown','skipped'].includes(to?.visual);edge.route=edge.unchosen?'skipped':(rank.get(edge.to)??0)<=(rank.get(edge.from)??0)?'return':to?.kind==='finish'?'terminal':'forward';}
 spreadPorts(edges,'from','fromPort');spreadPorts(edges,'to','toPort');
 return {nodes,edges,width:Math.max(760,(maxRank+1)*300+50),height:Math.max(480,maxY+120)};
}
function definitionInvocation(run,id) {
 const definition=(run.definitions||[]).find(d=>parseMaybe(d.bytes)?.id===id),matches=values(run.invocations).filter(i=>i.workflow_ref?.digest===definition?.ref?.digest);
 return matches.length===1?matches[0].id:'__definition__';
}
function fileChanges(before,after) {
 if(!before || !after) return null;
 const left = new Map(before.files.map(f=>[f.path,f.ref?.digest])), right = new Map(after.files.map(f=>[f.path,f.ref?.digest]));
 return [...new Set([...left.keys(),...right.keys()])].sort().flatMap(path => !left.has(path) ? [{path,change:'Добавлен'}] : !right.has(path) ? [{path,change:'Удалён'}] : left.get(path)!==right.get(path) ? [{path,change:'Изменён'}] : []);
}
function nodeCard(graph,id,object,run) {
 const node=graph.nodes.find(n=>n.id===id);if(!node)return '';
 const list=(title,edges)=>`<div><small>${title}</small>${edges.length?`<ul>${edges.map(e=>`<li>${esc(e.label||'переход')} · ${esc(e.from===id?e.to:e.from)}${e.reason?` · ${esc(e.reason)}`:''}</li>`).join('')}</ul>`:'<p class="muted">Нет закреплённых переходов</p>'}</div>`;
 const inputs=Object.keys(object?.context?.inputs||node.stage?.input_bindings||{}),outputs=Object.keys(object?.accepted?.outputs||node.stage?.output_bindings||{});
 const incoming=graph.edges.filter(e=>e.to===id),attemptFor=e=>{const source=graph.nodes.find(n=>n.id===e.from);return values(run?.attempts).find(a=>a.step_instance_id===source?.actual?.at(-1)?.step_instance_id);};
 const arrived=incoming.find(e=>{const attempt=attemptFor(e);return attempt && (attempt.accepted?.verdict||attempt.candidate?.verdict)===e.label;})||incoming.find(e=>graph.nodes.find(n=>n.id===e.from)?.actual?.length),source=graph.nodes.find(n=>n.id===arrived?.from),attempt=arrived&&attemptFor(arrived),summary=attempt?.accepted?.summary||attempt?.candidate?.summary;
 const reason=arrived?`<div><small>Почему выбран этот переход</small><p>После ${esc(arrived.from)}: ${esc(label(arrived.label))}${summary?` — ${esc(summary)}`:''}</p></div>`:'';
 return `<section class="node-card"><div><small>Закреплённый узел</small><p>${esc(node.title)} · ${esc(label(node.kind))}</p></div><p>${badge(node.state)}${node.reason?` <span class="muted">${esc(node.reason)}</span>`:''}</p>${reason}${list('Входящие переходы',graph.edges.filter(e=>e.to===id))}${list('Исходящие переходы',graph.edges.filter(e=>e.from===id))}<div><small>Данные</small><p>${inputs.length?`Входы: ${esc(inputs.join(', '))}`:'Входы не записаны'}${outputs.length?` · Выходы: ${esc(outputs.join(', '))}`:''}</p></div></section>`;
}
const runKey = run => run.source+'|'+run.run_id;
function relatedTitle(run,depth) {
 const relation=run.fork_source_run_id?(run.fork_reason==='project continuation'?'Продолжение':run.fork_reason==='recover_failed_stage'?'Восстановление':run.fork_reason==='resume_stopped_run'?'Возобновление':'Производный Run'):'';
 const missing=run.missing_parent?`<span class="missing-parent">Исходный Run недоступен: ${esc(run.fork_source_run_id)}</span>`:'';
 const context=run.context?'<span class="run-context">Контекст поиска</span>':'';
 const count=run.children?`${run.descendants} связанных Run · ${run.matches} совпадений в ветви`:'';
 return `<div class="run-lineage ${depth?'is-child':''}" style="--run-depth:${depth}">${run.children?`<button type="button" class="run-expand" data-expand="${esc(runKey(run))}" data-focus="toggle:${esc(runKey(run))}" aria-expanded="${expandedRuns.has(runKey(run))}" aria-label="${expandedRuns.has(runKey(run))?'Свернуть':'Развернуть'} связанные Run (${run.descendants})">${expandedRuns.has(runKey(run))?'▾':'▸'}</button>`:'<span class="run-expand-spacer" aria-hidden="true"></span>'}<div>${relation?`<span class="run-relation">${relation}</span>`:''}${context}${missing}<a data-focus="${esc(run.source+run.run_id)}" href="#${esc(new URLSearchParams({source:run.source,run:run.run_id}))}">${esc(run.subject || 'Название задачи не записано')}</a>${count?`<small>${esc(count)}</small>`:''}<small>${esc(run.run_id)}</small>${run.error?`<p class="error">${esc(run.error)}</p>`:''}</div></div>`;
}
function runStatus(run) {
 const tip=run.latest_descendant;
 const own=run.status==='running' && run.active_attempts>0 && run.active_attempts===run.awaiting_hosts?'awaiting_host':run.status;
 const current=`${tip?'<small>Этот Run</small>':''}${badge(own)}${run.outcome?`<small>Outcome: ${esc(run.outcome)}</small>`:''}`;
 if(!tip)return current;
 const hash=new URLSearchParams({source:run.source,run:tip.run_id});
 return `${current}<div class="branch-tip"><small>Последний производный Run · ${esc(date(tip.last_observed))}</small><a href="#${esc(hash)}">${badge(tip.outcome || tip.status)} <span>${esc(short(tip.run_id))}</span></a><small>Состояние: ${esc(label(tip.status))}</small></div>`;
}
const expandedRuns = new Set();
// What a step was handed and what is expected of it, read only from the stored
// Run. Nothing is inferred from the current project, profile or catalog: a fact
// the Run does not hold is returned as null and shown as not recorded.
const answerSources={actor:'ответ владельца при запуске',project_default:'значение проекта по умолчанию',package_default:'значение пакета по умолчанию',autonomous_policy:'автономная политика',unanswered:'ещё без ответа',launch:'указано при запуске'};
const questionBases={decision:'объявленное решение Run',input:'вход шага',instructions:'инструкции шага',person:'человек в сессии',judgement:'решение агента'};
const sameValue=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
function definitionBytes(run,ref) {return parseMaybe((run.definitions||[]).find(d=>ref && d.ref?.digest===ref.digest)?.bytes);}
function activationWorkflow(run,activation) {const ref=run.invocations?.[activation?.workflow_invocation_id]?.workflow_ref||run.workflow_ref;return definitionBytes(run,ref)||parseMaybe(run.workflow);}
function catalogDecision(run,id) {return (run.decision_catalog?.decisions||[]).find(d=>d.id===id);}
function choiceTitle(definition,value) {return (definition?.choices||[]).find(c=>sameValue(c.value,value))?.title||null;}
function stateNumber(run) {return Number(/^core-state\/(\d+)$/.exec(run?.schema_version||'')?.[1]||0);}
function stepReceipt(run,attempt) {
 const activation=run.activations?.[attempt.stage_activation_id],stageID=activation?.stage_id||'';
 const stage=activationWorkflow(run,activation)?.definition?.stages?.[stageID]||{};
 const step=run.steps?.[attempt.step_instance_id],definition=definitionBytes(run,step?.definition_ref)||{};
 const inputs=Object.keys({...(definition.inputs||{}),...(attempt.context?.inputs||{})}).sort().map(port=>{
  const given=attempt.context?.inputs?.[port],binding=stage.input_bindings?.[port];
  const origin=binding?.from==='workflow_input'?{kind:'workflow_input',port:binding.port}:binding?.from==='stage_output'?{kind:'stage_output',stage:binding.stage_id,port:binding.port}:binding?{kind:binding.from}:null;
  return {port,ref:given?.ref||null,origin};
 });
 const session=attempt.session||null;
 let decisions=null;
 if(session?.decision_context){
  const catalog=run.decision_catalog?.decisions||[];
  decisions=Object.entries(session.decision_context).map(([name,value])=>{
   const d=name==='core:package_profile'?catalog.find(x=>x.destination?.kind==='package_profile'):catalog.find(x=>x.destination?.kind==='session_context'&&x.destination?.name===name);
   const record=[...(run.decision_ledger||[])].reverse().find(r=>r.definition_id===d?.id&&(!r.attempt_id||r.attempt_id===attempt.id));
   return {name,id:d?.id||name,title:d?.title||name,phase:d?.phase||'',value,choice:choiceTitle(d,value),source:name==='core:package_profile'?run.decision_sheet?.profile_source:record?.source||null,during:!!record?.attempt_id};
  });
 }
 const refs=session?.skill_refs||[definition.instructions_ref,...(definition.context_refs||[])].filter(Boolean);
 const texts=refs.map(ref=>{const resource=(run.context_resources||[]).find(x=>x.ref?.digest===ref.digest);return {ref,role:ref.digest===definition.instructions_ref?.digest?'instructions':'context',text:resource?.bytes??null};});
 const expects={outputs:Object.entries(definition.outputs||{}).map(([port,o])=>({port,schema:o.schema_ref?.id||o.format||'',requiredFor:o.required_for||[]})),verdicts:Object.keys(stage.on||{}),effect:definition.effects||null,externalWrite:definition.external_write||null,trees:(session?.workspace_trees||[]).map(t=>t.capture?.path||t.output_port).filter(Boolean)};
 const asked=(run.decision_ledger||[]).filter(r=>r.attempt_id===attempt.id).map(r=>{const d=catalogDecision(run,r.definition_id);return {id:r.definition_id,title:d?.title||r.definition_id,options:(d?.choices||[]).map(c=>c.title),value:r.value,choice:choiceTitle(d,r.value),status:r.status,source:r.source,requested:r.requested?.utc||'',answered:r.status==='pending'?'':r.observed?.utc||''};});
 const pending=run.pending_decision?.attempt_id===attempt.id?run.pending_decision.decision_id:'';
 // A report is owed from state 41; before it no list was kept, which is not
 // the same as a host saying there were no questions.
 const reported=attempt.question_report?attempt.question_report.questions:null;
 return {stage:stageID,program:!session,inputs,decisions,texts,expects,asked,pending,reported,reportsKept:stateNumber(run)>=41};
}
// Launch inputs and every decision of the sealed sheet, with who gave each
// answer. A decision the profile excludes is named as such, not left out.
function launchInputs(run,inputValues={}) {
 const configured=run.effective_configuration?.inputs||{};
 const inputs=Object.entries(inputValues||{}).map(([port,value])=>({port,value,source:configured[port]?.source||'launch'}));
 const sheet=run.decision_sheet,records=sheet?.records||[];
 const decisions=(run.decision_catalog?.decisions||[]).map(d=>{
  const record=records.find(r=>r.definition_id===d.id)||[...(run.decision_ledger||[])].reverse().find(r=>r.definition_id===d.id&&r.status!=='pending');
  const profiles=d.when?.profiles,answers=d.when?.answers||{};
  const excluded=!record&&((profiles&&!profiles.includes(sheet?.package_profile))||Object.entries(answers).some(([id,want])=>!records.some(r=>r.definition_id===id&&sameValue(r.value,want))));
  const value=d.destination?.kind==='package_profile'&&!record?sheet?.package_profile:record?.value;
  return {id:d.id,title:d.title,phase:d.phase,answered:!!record||d.destination?.kind==='package_profile'&&!!sheet,value,choice:choiceTitle(d,value),source:record?.source||(d.destination?.kind==='package_profile'?sheet?.profile_source:null),excluded};
 });
 return {inputs,decisions,profile:sheet?.package_profile||'',profileSource:sheet?.profile_source||'',policy:sheet?.decision_policy||''};
}
// Where the Run's time went, from the timing tree the engine computed. Nothing
// here subtracts observations: every number is one the calculator published.
const amount=d=>d?.value_ms??d?.estimate_ms??d?.known_ms??null;
function timeBreakdown(timing) {
 const root=timing?.root,metrics=root?.metrics||{};
 if(!metrics.idle)return null;
 const categories=[['host_work_sum','Работа агентов'],['host_pickup_sum','Ожидание, пока агент возьмёт задание'],['decision_wait_sum','Ожидание ответа на вопрос'],['executor_sum','Работа программ'],['check_executor_sum','Проверки'],['idle','Ничего не выполнялось']]
  .map(([key,title])=>({key,title,d:metrics[key]})).filter(c=>c.d&&c.d.quality!=='not_applicable');
 const stages=new Map();
 for(const invocation of root.children||[])for(const node of invocation.children||[]){
  if(node.kind!=='stage_activation'||node.stage_kind==='finish')continue;
  const row=stages.get(node.stage_id)||{stage:node.stage_id,count:0,ms:0,agentMs:0,unknown:false};
  row.count++;const value=amount(node.metrics?.elapsed);if(value==null)row.unknown=true;else row.ms+=value;
  const agent=amount(node.metrics?.host_work_sum);if(agent!=null)row.agentMs+=agent;
  stages.set(node.stage_id,row);
 }
 return {elapsed:metrics.elapsed,categories,stages:[...stages.values()].sort((a,b)=>b.ms-a.ms)};
}
function attemptPhases(node,assisted) {
 const m={...(node?.metrics||{})};
 // An assisted attempt has no process, so its executor time is not a phase of it.
 if(assisted)delete m.executor_time;
 return [['host_pickup','Ждал, пока агент возьмёт задание'],['host_work','Агент работал'],['decision_wait','Ждал ответа на вопрос'],['executor_time','Программа работала'],['result_to_acceptance','Приёмка результата']]
  .map(([key,title])=>({key,title,d:m[key]})).filter(p=>p.d&&p.d.quality!=='not_applicable');
}
if(typeof module !== 'undefined') module.exports = {esc,label,duration,graphData,definitionInvocation,fileChanges,nodeCard,executionRole,activityRows,relatedTitle,expandedRuns,runStatus,stepReceipt,launchInputs,timeBreakdown,attemptPhases};
if(typeof document !== 'undefined') {
const $ = id => document.getElementById(id);
let selected='',source='',generation=0,page=1,pages=1,state=null,nodeID='',invocation='',definition='',tab='workflow',stamp='',eventsCursor=0,eventsMore=false,busy=false,queued=false,filterTimer,revealed='',graphFocus='';
let listScroll=0,maintenanceBusy=false,storageBusy=false;
let relatedInitialized=false,expandSearch=false;
const childPages=new Map(),childCache=new Map();
const artifactCache=new Map(),fileCache=new Map();
const fileKey=a=>source+selected+nodeID+JSON.stringify(a?.accepted?.outputs || {});
const form=$('filters');
const query = () => new URLSearchParams(new FormData(form));
const route = () => new URLSearchParams(location.hash.slice(1));
async function get(path,params={}) {
 const response=await fetch(path+'?'+new URLSearchParams(params),{signal:AbortSignal.timeout(15000)});
 if(!response.ok) throw new Error((await response.text()).slice(0,1200));
 return response.json();
}
const lastMarkup=new WeakMap();
function preserve(box,html) {
 if(lastMarkup.get(box)===html)return;
 lastMarkup.set(box,html);
 const open=new Map([...box.querySelectorAll('details[data-key]')].map(d=>[d.dataset.key,d.open]));
 const scroll=new Map([...box.querySelectorAll('[data-scroll]')].map(e=>[e.dataset.scroll,[e.scrollLeft,e.scrollTop]]));
 const focus=document.activeElement?.dataset.focus;
 box.innerHTML=html;
 for(const d of box.querySelectorAll('details[data-key]')) if(open.has(d.dataset.key)) d.open=open.get(d.dataset.key);
 hydrate(box);
 for(const e of box.querySelectorAll('[data-scroll]')) {const old=scroll.get(e.dataset.scroll);if(old){e.scrollLeft=old[0];e.scrollTop=old[1];}}
 if(focus) [...box.querySelectorAll('[data-focus]')].find(e=>e.dataset.focus===focus)?.focus({preventScroll:true});
}

function plainValue(value) {return typeof value==='string'?value:JSON.stringify(value,null,2);}
function shownValue(value,key) {
 const text=plainValue(value);
 if(text==='')return '<span class="muted">(пусто)</span>';
 if(text.length<=140&&!text.includes('\n'))return `<code>${esc(text)}</code>`;
 return `<details class="long-value" data-key="${esc(key)}"><summary>${esc(text.replace(/\s+/g,' ').slice(0,110))}…</summary><pre class="raw" data-scroll="${esc(key)}">${esc(text)}</pre></details>`;
}
function launchValue(value,key) {
 if(value&&typeof value==='object'&&typeof value.title==='string')return `<b>${esc(value.title)}</b>${value.reference?` <a href="${esc(value.reference)}" target="_blank" rel="noopener">${esc(value.reference)}</a>`:''}${value.description?shownValue(value.description,key+':description'):''}`;
 return shownValue(value,key);
}
function stageLink(run,stage) {const activation=values(run.activations).filter(a=>a.stage_id===stage).at(-1);return activation?`<button type="button" data-node="${esc(activation.id)}">${esc(stage)}</button>`:`<b>${esc(stage)}</b>`;}
function receiptHTML(run,attempt,inputValues,phases,note='') {
 const r=stepReceipt(run,attempt),key='receipt:'+attempt.id,row=(title,body)=>`<div class="receipt-row"><div class="receipt-key">${title}</div><div>${body}</div></div>`;
 let html=`<section class="receipt"><h4>Что получил шаг${r.stage?` · ${esc(r.stage)}`:''}</h4>${note?`<p class="muted">${esc(note)}</p>`:''}`;
 if(phases.length)html+=`<h5>Время шага</h5>`+phases.map(p=>row(esc(p.title),esc(amount(p.d)==null?duration(p.d,true):duration(p.d))+(p.key==='host_work'&&p.d.reasons?.includes('host_take_not_recorded')?'<br><small class="muted">включает время до того, как агент взял задание</small>':''))).join('');
 html+='<h5>Входы</h5>'+(r.inputs.length?r.inputs.map(i=>{
  const from=i.origin?.kind==='workflow_input'?`вход запуска <code>${esc(i.origin.port)}</code>${inputValues&&Object.hasOwn(inputValues,i.origin.port)?launchValue(inputValues[i.origin.port],key+':in:'+i.port):''}`:i.origin?.kind==='stage_output'?`результат шага ${stageLink(run,i.origin.stage)}, выход <code>${esc(i.origin.port)}</code>`:i.origin?`источник ${esc(i.origin.kind)}`:'<span class="muted">происхождение в закреплённом workflow не найдено</span>';
  return row(`<code>${esc(i.port)}</code>`,from+(i.ref?artifact(i.ref,'Содержимое, переданное шагу'):'<p class="muted">Артефакт не передан</p>'));
 }).join(''):'<p class="muted">У шага нет входов.</p>');
 if(r.program)html+='<h5>Решения и инструкции</h5><p class="muted">Шаг исполняет программа: ответы решений и тексты инструкций такому шагу не передаются.</p>';
 else{
  html+='<h5>Решения, переданные агенту</h5>'+(r.decisions===null?'<p class="muted">В этом Run не записано, какие ответы переданы агенту.</p>':r.decisions.length?r.decisions.map(d=>row(`${esc(d.title)}<br><small class="muted">${esc(d.id)}${d.phase==='runtime'?' · вопрос навыка':''}</small>`,`${d.choice?`<b>${esc(d.choice)}</b> `:''}${shownValue(d.value,key+':d:'+d.name)}<br><small class="muted">${esc(answerSources[d.source]||d.source||'источник не записан')}${d.phase==='runtime'?' · ответ дан заранее':''}</small>`)).join(''):'<p class="muted">Агенту не передано ни одного ответа.</p>');
  html+='<h5>Инструкции и навыки</h5>'+(r.texts.length?r.texts.map(t=>row(`${t.role==='instructions'?'Инструкции шага':'Контекст'}<br><small class="muted">${esc(t.ref.id)}</small>`,t.text==null?'<p class="muted">Текст не закреплён в Run</p>':`<details class="long-value" data-key="${esc(key+':t:'+t.ref.digest)}"><summary>${esc(Math.round(t.text.length/1024))} КБ текста</summary><pre class="raw" data-scroll="${esc(key+':t:'+t.ref.digest)}">${esc(t.text)}</pre></details>`)).join(''):'<p class="muted">Текстов не передано.</p>');
  html+='<h5>Вопросы шага</h5>';
  if(r.pending)html+=row('Ждёт ответа',`<b>${esc(catalogDecision(run,r.pending)?.title||r.pending)}</b> <code>${esc(r.pending)}</code>`);
  html+=r.asked.map(q=>row(`${esc(q.title)}<br><small class="muted">${esc(q.id)} · задан через Pri-Fly</small>`,q.status==='pending'?'<span class="muted">ответа ещё нет</span>':`${q.choice?`<b>${esc(q.choice)}</b> `:''}${shownValue(q.value,key+':q:'+q.id)}<br><small class="muted">${esc(answerSources[q.source]||q.source)}${q.answered?' · '+esc(date(q.answered)):''}${q.options.length?' · варианты: '+esc(q.options.join(' / ')):''}</small>`)).join('');
  if(r.reported===null)html+=`<p class="muted">${r.reportsKept?'Агент ещё не отчитался о вопросах этого шага.':'В этом Run вопросы, на которые агент ответил сам, не записывались.'}</p>`;
  else if(!r.reported.length)html+='<p class="muted">Агент заявил: вопросов не было.</p>';
  else html+='<p class="muted">Заявление агента в отчёте; Pri-Fly проверяет форму и ссылки, но не правдивость.</p>'+r.reported.map((q,index)=>row(`${esc(q.question)}${q.asked_by?`<br><small class="muted">спросил: ${esc(q.asked_by)}</small>`:''}`,`${shownValue(q.answer,key+':r:'+index)}<br><small class="muted">основание: ${esc(questionBases[q.basis]||q.basis)}${q.decision_id?` <code>${esc(q.decision_id)}</code>`:''}${q.port?` <code>${esc(q.port)}</code>`:''}${q.options?.length?' · варианты: '+esc(q.options.join(' / ')):''}</small>`)).join('');
 }
 const e=r.expects;
 html+='<h5>Что шаг должен вернуть</h5>'+row('Выходы',e.outputs.length?e.outputs.map(o=>`<code>${esc(o.port)}</code> <small class="muted">${esc(o.schema)}${o.requiredFor.length?' · обязателен при '+esc(o.requiredFor.join(', ')):''}</small>`).join('<br>'):'<span class="muted">нет</span>')
  +row('Вердикты с продолжением',e.verdicts.length?e.verdicts.map(v=>`<code>${esc(v)}</code>`).join(' '):'<span class="muted">не объявлены</span>')
  +(e.effect?row('Что шаг может менять',`<code>${esc(e.effect.class)}</code> · повтор: <code>${esc(e.effect.retry_class)}</code>${e.externalWrite?` · ${esc(e.externalWrite.system)}: ${esc((e.externalWrite.operations||[]).join(', '))} → ${esc(e.externalWrite.target)}`:''}`):'')
  +(e.trees.length?row('Забирается из рабочей копии',e.trees.map(t=>`<code>${esc(t)}</code>`).join(' ')):'');
 return html+'</section>';
}
function launchHTML(run,inputValues) {
 const l=launchInputs(run,inputValues);
 if(!l.inputs.length&&!l.decisions.length)return '';
 const row=(title,body)=>`<div class="receipt-row"><div class="receipt-key">${title}</div><div>${body}</div></div>`;
 let html='<details class="facts-section receipt" data-key="launch" open><summary>Входы и решения запуска</summary><div class="facts-body">';
 if(l.profile)html+=row('Профиль пакета',`<b>${esc(l.profile)}</b> <small class="muted">${esc(answerSources[l.profileSource]||l.profileSource)}${l.policy?' · политика: '+esc(l.policy):''}</small>`);
 html+=l.inputs.map(i=>row(`Вход <code>${esc(i.port)}</code>`,`${launchValue(i.value,'launch:in:'+i.port)}<br><small class="muted">${esc(answerSources[i.source]||i.source)}</small>`)).join('');
 html+=l.decisions.map(d=>row(`${esc(d.title)}<br><small class="muted">${esc(d.id)}${d.phase==='runtime'?' · вопрос во время работы':''}</small>`,d.excluded?'<span class="muted">не применяется к этому профилю</span>':!d.answered?'<span class="muted">не отвечено до запуска</span>':`${d.choice?`<b>${esc(d.choice)}</b> `:''}${shownValue(d.value,'launch:d:'+d.id)}<br><small class="muted">${esc(answerSources[d.source]||d.source||'источник не записан')}</small>`)).join('');
 return html+'</div></details>';
}
function timeHTML(timing) {
 const t=timeBreakdown(timing);
 if(!t)return '';
 const total=amount(t.elapsed)||0,share=d=>total&&amount(d)!=null?Math.min(100,Math.round(amount(d)*100/total)):0;
 let html=`<details class="facts-section" data-key="time" open><summary>Куда ушло время</summary><div class="facts-body"><p>Всего: <b>${esc(duration(t.elapsed))}</b></p><div class="time-bars">`;
 html+=t.categories.map(c=>`<div class="time-bar"><span>${esc(c.title)}</span><progress max="100" value="${share(c.d)}" aria-label="${esc(c.title)}"></progress><span>${esc(amount(c.d)==null?duration(c.d,true):duration(c.d))}</span></div>`).join('');
 html+='</div><p class="muted">Суммы по попыткам; параллельные ветви пересекаются, поэтому сумма может быть больше общего времени. «Оценка» — по часам authority между разными процессами.</p>';
 if(t.stages.length)html+='<table class="time-stages"><thead><tr><th>Стадия</th><th>Раз</th><th>Время</th><th>Из него агент</th></tr></thead><tbody>'+t.stages.map(s=>`<tr><td>${esc(s.stage)}</td><td>${s.count>1?'×'+s.count:'1'}</td><td>${s.unknown?'не менее ':''}${esc(ms(s.ms))}</td><td>${s.agentMs?esc(ms(s.agentMs)):'—'}</td></tr>`).join('')+'</tbody></table>';
 return html+'</div></details>';
}
function raw(value,key) {return `<details class="facts-section" data-key="raw:${esc(key)}"><summary>Исходные сохранённые данные</summary><pre class="raw" data-scroll="${esc(key)}">${esc(JSON.stringify(value,null,2))}</pre></details>`;}
function artifact(ref,title) {return `<details class="artifact" data-key="artifact:${esc(ref.digest)}" data-ref="${esc(JSON.stringify(ref))}"><summary>${esc(title || ref.artifact_id)} <small>Ревизия ${esc(ref.revision)} · ${esc(short(ref.digest))}</small></summary><div class="artifact-content">Откройте для чтения содержимого</div></details>`;}
function readable(value,key='',depth=0) {
 if(value==null) return '<span class="muted">Не записано</span>';
 if(value?.artifact_id && value?.digest && value?.revision) return artifact(value,'Артефакт '+short(value.artifact_id));
 if(typeof value!=='object') return `<span class="prose">${esc(typeof value==='boolean' ? (value?'Да':'Нет') : value)}</span>`;
 if(depth>8) return raw(value,key);
 if(Array.isArray(value)) return value.length ? '<ol>'+value.map((v,i)=>`<li>${readable(v,key+':'+i,depth+1)}</li>`).join('')+'</ol>' : '<span class="muted">Нет записей</span>';
 if(value.utc) return esc(date(value.utc));
 if(value.byte_encoding && typeof value.bytes==='string') {let text;try{text=new TextDecoder().decode(Uint8Array.from(atob(value.bytes),c=>c.charCodeAt(0)));}catch{text='Не удалось декодировать сохранённый текст';}return `<p>${esc(value.ref?.id || '')}</p><pre class="raw">${esc(text)}</pre>`;}
 return '<dl class="kv">'+Object.entries(value).map(([k,v])=>`<dt>${esc(label(k))}</dt><dd>${readable(v,key+':'+k,depth+1)}</dd>`).join('')+'</dl>';
}
function section(title,value,key,open=false) {return `<details class="facts-section" data-key="${esc(key)}" ${open?'open':''}><summary>${esc(title)}</summary><div class="facts-body">${readable(value,key)}</div></details>`;}
function artifactBody(a) {
 const parsed=parseMaybe(a.content);
 const files=parsed?.schema_version==='workspace-tree-manifest/1' && Array.isArray(parsed.files) ? `<h4>Файлы сохранённого дерева</h4><p class="muted">Состав этого снимка. Для списка изменений нужны входной и выходной снимки.</p>${readable(parsed.files)}` : '';
 return `<p class="muted">${esc(a.bytes)} байт · ${esc(a.artifact?.media_type || a.artifact?.format || '')}${a.truncated ? ' · Показан только первый 1 MiB' : ''}</p>${files || (typeof parsed==='object' ? readable(parsed) : `<pre class="raw" data-scroll="blob:${esc(a.artifact?.digest || '')}">${esc(a.content)}</pre>`)}${typeof parsed==='object'?`<details class="facts-section"><summary>Исходный текст артефакта</summary><pre class="raw" data-scroll="raw-blob:${esc(a.artifact?.digest || '')}">${esc(a.content)}</pre></details>`:''}${a.truncated?'<button type="button" data-full>Загрузить текст целиком</button>':''}`;
}
function hydrate(box) {
 for(const details of box.querySelectorAll('details[data-ref]')) {
  const ref=JSON.parse(details.dataset.ref),cached=artifactCache.get(source+':'+ref.digest);
  if(cached && details.open) details.querySelector('.artifact-content').innerHTML=artifactBody(cached);
  else if(details.open && !cached) loadArtifact(details);
 }
}
async function loadArtifact(details,full=false) {
 const ref=JSON.parse(details.dataset.ref),key=source+':'+ref.digest,g=source+selected;
 const box=details.querySelector('.artifact-content');box.textContent='Загрузка…';
 try {const value=await get('/api/artifact',{source,...ref,...(full?{full:'1'}:{})});if(g!==source+selected || !details.isConnected)return;artifactCache.set(key,value);box.innerHTML=artifactBody(value);}
 catch(err){if(g===source+selected && details.isConnected)box.textContent='Не удалось прочитать: '+err.message;}
}
document.addEventListener('toggle',event=>{const d=event.target;if(d.matches?.('details[data-ref]') && d.open && !artifactCache.has(source+':'+JSON.parse(d.dataset.ref).digest))loadArtifact(d);else if(d.matches?.('details[data-ref]') && d.open){const a=artifactCache.get(source+':'+JSON.parse(d.dataset.ref).digest);if(a)d.querySelector('.artifact-content').innerHTML=artifactBody(a);}},true);
document.addEventListener('click',event=>{if(event.target.closest('[data-full]'))loadArtifact(event.target.closest('details[data-ref]'),true);});
function options(name,items,all) {
 const el=form.elements[name],old=el.value;
 if(old && !items.includes(old))items=[...items,old];
 const html=`<option value="">${all}</option>`+items.map(v=>`<option value="${esc(v)}">${esc(name==='status'?label(v):v)}</option>`).join('');
 if(el.innerHTML!==html){el.innerHTML=html;if(items.includes(old))el.value=old;}
}
const sizeText = n => {if(n==null)return 'Недоступно';const units=['Б','КиБ','МиБ','ГиБ','ТиБ'];let i=0;while(n>=1024 && i<units.length-1){n/=1024;i++;}return n.toLocaleString('ru-RU',{maximumFractionDigits:1})+' '+units[i];};
async function refreshStorage(){
 if(storageBusy)return;storageBusy=true;
 try {const data=await get('/api/storage');$('storage-title').textContent=data.measured?'Учтено в найденных хранилищах: '+sizeText(data.bytes)+(data.sources.some(s=>s.errors.length)?' · подсчёт неполный':''):'Размер хранилищ: измерение…';
 preserve($('storage-details'),`<p class="muted">Выделенное место по данным файловой системы. Общие файлы учтены один раз. Клоны и снимки APFS могут использовать одни блоки, поэтому размер приблизительный. Внешние рабочие репозитории не включены. ${data.scanning?'Измерение продолжается.':''} ${data.measured?'Измерено '+esc(date(data.measured)):''}</p>${(data.sources||[]).map(s=>`<div class="storage-source"><b>${esc(s.root)}</b><p>${s.errors.length?'Учтено (неполно)':'Занято'}: ${sizeText(s.measured?s.bytes:null)} · артефакты: ${sizeText(s.measured?s.artifacts:null)} · доступно на диске: ${sizeText(s.available)}</p>${s.errors.length?'':`<button type="button" data-clean-source="${esc(s.id)}">Очистить завершённые Run…</button>`}${s.errors.map(e=>`<p class="error">${esc(e)}</p>`).join('')}</div>`).join('')}`);
 }catch(err){$('storage-title').textContent='Размер хранилищ недоступен: '+err.message;}finally{storageBusy=false;}
}
function renderList(data) {
 pages=data.pages;page=data.page;
 form.elements.sort.disabled=form.elements.view.value!=='flat';
 form.elements.sort.title=form.elements.sort.disabled?'Порядок связанных Run выбран в поле «Вид»':'';
 $('totals').innerHTML=`<span><b>${data.total}</b> прогонов</span><span><b>${data.active}</b> с активными попытками</span>`;
 const d=data.discovery,errors=data.sources.filter(s=>s.error),pending=data.sources.filter(s=>!s.indexed && !s.error).length;
 $('coverage-title').textContent=`${data.sources.length} хранилищ · ${d.scanning?'поиск продолжается':'обход завершён'} · ${d.directories} каталогов${d.unreadable?' · недоступно: '+d.unreadable:''}${errors.length?' · ошибок чтения: '+errors.length:''}${pending?' · индексируется: '+pending:''}`;
 preserve($('sources'),`<p class="muted">Область поиска: ${esc(d.roots.join(', '))}. Исключены системные/сетевые каталоги: ${d.excluded}. Символические ссылки при обходе не раскрываются; физические каталоги читаются по исходному пути. Полнота относится к доступной проверенной области.</p>${d.completed?`<p>Последний обход: ${esc(date(d.completed))}</p>`:''}${data.sources.map(s=>`<div class="source"><b>${esc(s.project || 'Хранилище')}</b><small>${esc(s.root)}</small>${s.conflict?'<p class="error">Конфликт: одинаковая идентичность у нескольких хранилищ. Их Run показаны отдельно.</p>':''}<p class="${s.error?'error':'muted'}">${esc(s.error || (s.indexed?'Прочитано '+date(s.updated):'Индексируется…'))}</p></div>`).join('')}${d.errors.length?section('Недоступные пути (примеры)',d.errors,'scan-errors'):''}`);
 options('project',data.filters.projects,'Все проекты');options('status',data.filters.statuses,'Все статусы');options('executor',data.filters.executors,'Все исполнители');
 const listed=[];
 const append=(run,depth)=>{
  listed.push({run,depth});
  if(form.elements.view.value==='flat' || !expandedRuns.has(runKey(run)))return;
  const loaded=childCache.get(runKey(run));
  for(const child of loaded?.runs||[])append(child,depth+1);
  if(loaded && loaded.loadedPages<loaded.pages)listed.push({more:run,depth:depth+1});
 };
 for(const run of data.runs)append(run,0);
 const anchor=[...$('runs').querySelectorAll('tr[data-run-key]')].find(row=>row.getBoundingClientRect().bottom>0);
 const anchorTop=anchor?.getBoundingClientRect().top,anchorKey=anchor?.dataset.runKey;
 preserve($('runs'),listed.length?listed.map(({run,depth,more})=>{
  if(more)return `<tr><td colspan="5" class="run-more" style="--run-depth:${depth}"><button type="button" data-more="${esc(runKey(more))}">Показать ещё дочерние Run →</button></td></tr>`;
  const hash=new URLSearchParams({source:run.source,run:run.run_id});
  const task=run.subject || 'Название задачи не записано',project=run.project_title || run.project_fallback || 'Локальное имя не найдено',fallback=!run.project_title;
  return `<tr data-run-key="${esc(runKey(run))}" class="${run.source===source && run.run_id===selected?'selected':''} ${run.context?'run-context-row':''}"><td>${form.elements.view.value!=='flat'?relatedTitle(run,depth):`<a data-focus="${esc(run.source+run.run_id)}" href="#${esc(hash)}">${esc(task)}</a><small>${esc(run.run_id)}</small>${run.error?`<p class="error">${esc(run.error)}</p>`:''}`}</td><td>${esc(project)}${fallback?'<small>Локальное имя</small>':''}<small>${esc(run.project)}</small><small>${esc(run.workflow_id)}</small><small>${esc(run.root)}</small></td><td>${runStatus(run)}</td><td>${esc(run.step_instances ?? '—')} / ${esc(run.attempts ?? '—')}<small>${run.settled_attempts ?? '—'} попыток завершено</small></td><td>${esc(date(run.created))}<small>v${esc(run.run_version)} · e${esc(run.event_sequence)}</small></td></tr>`;
 }).join(''):'<tr><td colspan="5" class="empty">Прогоны не найдены в прочитанной области. Измените фильтры или проверьте состояние поиска выше.</td></tr>');
 if(!selected && anchorKey && anchorTop!=null){const current=[...$('runs').querySelectorAll('tr[data-run-key]')].find(row=>row.dataset.runKey===anchorKey);if(current)window.scrollBy(0,current.getBoundingClientRect().top-anchorTop);}
 $('matches').textContent=`Найдено: ${data.filtered}`;$('page').textContent=`${page} / ${pages}`;$('previous').disabled=page<=1;$('next').disabled=page>=pages;
}
async function loadRelated(rows,params,g,autoExpand) {
 const seen=new Set();
 const load=async run=>{
  const key=runKey(run);
  if(!run.children || !expandedRuns.has(key) || seen.has(key))return;
  seen.add(key);
  const requested=childPages.get(key)||1,children=[];
  let pages=1;
  for(let next=1;next<=requested && next<=pages;next++) {
   const query=new URLSearchParams(params);query.set('parent',run.run_id);query.set('source',run.source);query.set('page',next);
   const data=await get('/api/runs',query);if(g!==generation)return;
   pages=data.pages;children.push(...data.runs);
  }
  childCache.set(key,{runs:children,pages,loadedPages:Math.min(requested,pages)});
  if(autoExpand)for(const child of children)if(child.context && child.children)expandedRuns.add(runKey(child));
  await Promise.all(children.map(load));
 };
 await Promise.all(rows.map(load));
}
function definitions(run) {return (run.definitions || []).map(d=>parseMaybe(d.bytes)).filter(d=>d?.definition?.stages);}
function currentWorkflow(run) {
 if(definition) return definitions(run).find(d=>d.id===definition) || run.workflow;
 const ref=run.invocations?.[invocation]?.workflow_ref || run.workflow_ref;
 return (run.definitions || []).find(d=>d.ref?.digest===ref?.digest)?.bytes || run.workflow;
}
function renderGraph() {
 if(!state)return;const run=state.run,workflow=parseMaybe(currentWorkflow(run));
 const selectedActivation=run.attempts?.[nodeID]?.stage_activation_id || run.steps?.[nodeID]?.stage_activation_id || nodeID;
 const graphInvocation=definition?definitionInvocation(run,definition):invocation,data=graphData(workflow,run,graphInvocation,state.choices,state.input_values),byID=new Map(data.nodes.map(n=>[n.id,n]));
 const svg=`<svg width="${data.width}" height="${data.height}" viewBox="0 0 ${data.width} ${data.height}" role="group" aria-label="Связи закреплённых стадий workflow"><defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z"/></marker></defs>${data.edges.map((e,index)=>{const a=byID.get(e.from),b=byID.get(e.to);if(!a||!b)return '';const x=a.x+230,y=a.y+39+e.fromPort,tx=b.x,ty=b.y+39+e.toPort,channel=e.route==='return'?42+index%3*18:data.height-42-index%3*18,route=e.route==='forward'?`M ${x} ${y} H ${tx-22} V ${ty} H ${tx}`:`M ${x} ${y} H ${x+22} V ${channel} H ${tx-22} V ${ty} H ${tx}`,classes=[`route-${e.route}`,e.path?'active-path':'',e.muted?'muted-path':''].filter(Boolean).join(' ');return `<path class="${classes}" d="${route}" marker-end="url(#arrow)"/><text class="edge-label ${e.muted?'muted-path':''}" x="${(x+tx)/2}" y="${e.route==='forward'?(y+ty)/2-8:channel-6}" text-anchor="middle">${esc(e.label)}</text>`;}).join('')}${data.nodes.map(n=>`<g tabindex="0" role="button" aria-label="${esc(n.title+', '+label(n.state)+(n.reason?': '+n.reason:''))}" data-stage="${esc(n.id)}" class="node-${n.visual} ${(n.id===nodeID || n.actual?.some(a=>a.id===selectedActivation))?'selected':''}"><rect x="${n.x}" y="${n.y}" width="230" height="78" rx="9"/><text x="${n.x+12}" y="${n.y+22}">${esc(n.title.length>26?n.title.slice(0,24)+'…':n.title)}</text><text x="${n.x+12}" y="${n.y+43}">${esc(label(n.kind))}</text><text x="${n.x+12}" y="${n.y+64}">${esc(n.state==='reused'||n.state==='revalidated'?label(n.state):n.visual==='skipped'?label('skipped'):(n.visual==='reachable'?'Далее':label(n.state)))}</text>${n.reason?`<title>${esc(n.reason)}</title>`:''}</g>`).join('')}</svg>`;
 const left=$('graph').scrollLeft,top=$('graph').scrollTop;$('graph').innerHTML=svg;$('graph').scrollLeft=left;$('graph').scrollTop=top;
 // The active stages are centred once per change of what is active, so polling never fights a person scrolling.
 const box=$('graph'),active=data.nodes.filter(n=>n.visual==='current'),focusKey=[source,selected,invocation,definition,...active.map(n=>n.id)].join('\n');
 if(active.length && focusKey!==graphFocus && box.clientHeight){const pad=parseFloat(getComputedStyle(box).paddingTop)||0,ys=active.map(n=>n.y),xs=active.map(n=>n.x);box.scrollTop=pad+(Math.min(...ys)+Math.max(...ys)+78)/2-box.clientHeight/2;box.scrollLeft=pad+(Math.min(...xs)+Math.max(...xs)+230)/2-box.clientWidth/2;graphFocus=focusKey;}
 $('graph').onclick=event=>{const target=event.target.closest('[data-stage]');if(!target)return;const n=byID.get(target.dataset.stage);if(n.invocation){navigateNode('',n.invocation);return;}if(n.ref){definition=n.ref.id;invocation='';renderDetail();return;}navigateNode(n.actual?.at(-1)?.id || n.id,invocation);};
 $('graph').onkeydown=event=>{if(event.key==='Enter'||event.key===' '){const n=event.target.closest('[data-stage]');if(n){event.preventDefault();n.dispatchEvent(new MouseEvent('click',{bubbles:true}));}}};
}
function treeNode(n,depth=0) {
 return `<details class="node" data-key="tree:${esc(n.kind+':'+n.id)}" ${depth<2?'open':''}><summary><button data-node="${esc(n.id)}" data-focus="${esc(n.id)}" aria-pressed="${n.id===nodeID}">${esc(n.stage_id || label(n.kind))} · ${esc(short(n.id))}</button> ${badge(n.status)} <small>${esc(duration(n.metrics?.elapsed))}</small></summary>${(n.children||[]).map(c=>treeNode(c,depth+1)).join('')}</details>`;
}
function timingNode(node,id) {if(!node)return null;if(node.id===id)return node;for(const child of node.children||[]){const found=timingNode(child,id);if(found)return found;}return null;}
function nodeObject(run,id) {for(const pool of [run.attempts,run.steps,run.activations,run.invocations,run.check_executions])if(pool?.[id])return pool[id];return id===run.id?run:null;}
function renderNode() {
 const run=state.run,object=nodeObject(run,nodeID),timing=timingNode(state.timing?.root,nodeID);
 const activation=run.activations?.[object?.stage_activation_id||object?.activation_id||nodeID],stageID=activation?.stage_id||object?.stage_id||nodeID,graph=graphData(parseMaybe(currentWorkflow(run)),run,definition?definitionInvocation(run,definition):invocation,state.choices,state.input_values);
 let html=nodeCard(graph,stageID,object,run),role=executionRole(object,parseMaybe(currentWorkflow(run))?.definition?.stages?.[stageID]);
 const received=run.attempts?.[nodeID]||(object?values(run.steps).filter(s=>s.stage_activation_id===object.id).flatMap(s=>s.attempt_ids||[]).map(id=>run.attempts?.[id]).filter(Boolean).at(-1):null);
 if(received)html+=receiptHTML(run,received,state.input_values,attemptPhases(timingNode(state.timing?.root,received.id),!!received.session),received.id===nodeID?'':'Последнее исполнение этой стадии: '+short(received.id));
 if(!object) {
  const stage=parseMaybe(currentWorkflow(run))?.definition?.stages?.[nodeID];
  $('node-title').textContent=stage ? nodeID+' · '+label(stage.kind) : 'Выберите шаг или попытку';
  if(stage){html+='<p class="muted">Закреплённое определение стадии.</p>'+readable(stage,nodeID);const actual=values(run.activations).filter(a=>a.stage_id===nodeID && (a.workflow_invocation_id || '')===invocation);html+=actual.map(a=>`<p><button data-node="${esc(a.id)}">Открыть исполнение ${esc(short(a.id))}</button></p>`).join('');}
  preserve($('node'),html);return;
 }
 $('node-title').textContent=(timing?.stage_id || label(timing?.kind) || 'Исполнение')+' · '+short(nodeID);html+=`<h4>Роль исполнения</h4><p><b>${esc(role.kind)}</b><br><span class="muted">${esc(role.detail)}${role.state?' · '+esc(role.state):''}</span></p>`;
 html+=`<p>${badge(object.status)} ${esc(object.verdict || object.outcome || '')}</p>`;
 if(timing){const metrics=Object.entries(timing.metrics||{}),shown=metrics.filter(([,v])=>v.quality!=='not_applicable');html+=section('Время',Object.fromEntries(shown.map(([k,v])=>[label(k),duration(v,true)])),nodeID+':time',true);const other=metrics.filter(([,v])=>v.quality==='not_applicable');if(other.length)html+=section('Неприменимые метрики',Object.fromEntries(other.map(([k,v])=>[label(k),duration(v,true)])),nodeID+':other-time');}
 if(run.attempts?.[nodeID]) {
  const context=object.context || {},envelope=parseMaybe(object.envelope),step=run.steps?.[object.step_instance_id],def=(run.definitions||[]).find(d=>d.ref?.digest===step?.definition_ref?.digest)?.bytes;
  const task=parseMaybe(def),instruction=(run.context_resources||[]).find(r=>r.ref?.digest===task?.instructions_ref?.digest);
  if(instruction)html+=section('Инструкции агенту',instruction,nodeID+':instructions',true);
  html+=section('Что отправлено агенту',{inputs:envelope?.input_artifacts,output_contracts:envelope?.output_contracts,context:envelope?.context_manifest_ref},nodeID+':sent',true)+section('Конверт исполнения',envelope,nodeID+':envelope');
  if(def)html+=section('Закреплённое задание шага',def,nodeID+':definition');
  if(context.rendering?.ref)html+=artifact(context.rendering.ref,'Текст контекста');
  html+=section('Входы и источники контекста',context,nodeID+':context');
  html+=section('Полученный результат (кандидат)',object.candidate,nodeID+':candidate',true)+section('Принятый результат',object.accepted,nodeID+':accepted',true);
  html+=section('Исполнитель и процесс',object.session || {process:object.process,process_outcome:object.process_outcome,workspace:object.workspace},nodeID+':executor');
  html+=section('Заявленная стоимость',object.reported_costs,nodeID+':cost');
  html+='<p class="muted">Модель, reasoning и токены: отдельные метрики не записаны. Суммы разных источников не складываются.</p><button type="button" id="load-files">Список файлов из сохранённых результатов</button><div id="file-list">'+(fileCache.get(fileKey(object)) || '')+'</div>';
 } else html+=section('Данные исполнения',object,nodeID+':facts',true);
 if(timing?.children?.length)html+='<h4>Вложенные исполнения</h4>'+timing.children.map(n=>`<p><button data-node="${esc(n.id)}">${esc(label(n.kind))} · ${esc(n.stage_id || short(n.id))}</button> ${badge(n.status)}</p>`).join('');
 html+=raw(object,nodeID);
 preserve($('node'),html);
}
async function loadFiles() {
 const g=source+selected,id=nodeID,a=state.run.attempts[id],box=$('file-list');box.textContent='Читаем сохранённые manifests…';
 try {
  const readTrees=async refs=>{const result=[];for(const ref of refs){const v=await get('/api/artifact',{source,...ref,full:'1'}),p=parseMaybe(v.content);if(p?.schema_version==='workspace-tree-manifest/1' && Array.isArray(p.files))result.push(p);}return result;};
  const before=await readTrees(values(a.context?.inputs).map(v=>v.ref)),after=await readTrees(values(a.accepted?.outputs));
  if(g!==source+selected || id!==nodeID || !box.isConnected)return;
  if(before.length===1 && after.length===1){const rows=fileChanges(before[0],after[0]);box.innerHTML='<p>Изменения между сохранёнными входным и выходным деревьями:</p>'+ (rows.length?readable(rows):'<p>Изменений файлов нет.</p>');}
  else box.innerHTML='<p class="muted">Точный список изменений не установлен: нужна однозначная пара сохранённых входного и выходного деревьев.</p>'+(after.length?after.map(tree=>readable(tree.files)).join(''):'<p>Выходное дерево файлов не записано.</p>');
  fileCache.set(fileKey(a),box.innerHTML);
 } catch(err){if(g===source+selected && box.isConnected)box.textContent='Не удалось прочитать файлы: '+err.message;}
}
function renderDetail() {
 const scrollX=window.scrollX,scrollY=window.scrollY;
 const run=state.run;$('delete-run').disabled=maintenanceBusy || !run.settled || !['completed','failed','cancelled'].includes(run.status);$('detail-title').textContent=parseMaybe(run.workflow)?.title || run.workflow_ref?.id || 'Прогон';
 $('detail-id').textContent=run.project_id+' · '+run.id;$('permalink').href=location.hash;
 const active=values(run.attempts).filter(a=>!a.settled),waiting=active.filter(a=>a.session?.host_state==='awaiting_host'),activity=activityRows(run);
 preserve($('overview'),`<div class="cards"><div class="card"><small>Состояние исполнения</small><strong>${badge(run.status)}</strong>${run.outcome?`<p>Outcome: ${esc(run.outcome)}</p>`:''}</div><div class="card"><small>Общее время · по данным Pri-Fly</small><strong>${esc(duration(state.timing?.root?.metrics?.elapsed))}</strong></div><div class="card"><small>Попытки / ожидают агента</small><strong>${values(run.attempts).length} / ${waiting.length}</strong><small>${active.length} незавершённых</small></div><div class="card"><small>Чтение</small><strong>v${state.run_version} · e${state.event_sequence}</strong><small>${state.driver_live?'Драйвер активен':'Активность драйвера не подтверждена'} · ${esc(date(run.last_observed?.utc))}</small></div></div><h3>Сейчас</h3>${activity.length?activity.map(a=>`<p>${a.node?`<button data-node="${esc(a.node)}">${esc(a.title)}</button>`:a.run?`<a href="#${esc(new URLSearchParams({source,run:a.run}))}">${esc(a.title)}</a>`:`<b>${esc(a.title)}</b>`} <span class="muted">${esc(a.detail)}</span></p>`).join(''):'<p class="muted">Активная работа и ожидания не записаны.</p>'}${timeHTML(state.timing)}${launchHTML(run,state.input_values)}${run.brief_ref?artifact(run.brief_ref,'Задача и критерии завершения'):''}${run.pending_decision?section('Ожидается решение',run.pending_decision,'pending',true):''}${run.stops?.length?section('Причины остановки',run.stops,'stops',true):''}${run.gaps?.length?section('Разрывы наблюдения',run.gaps,'gaps',true):''}`);
 const invs=values(run.invocations);if(!invocation && !definition)invocation=run.root_workflow_invocation_id || '';
 $('invocation').innerHTML=invs.map(i=>`<option value="${esc(i.id)}">${esc(i.workflow_ref.id)} · ${esc(i.branch_id || '')}${i.iteration?' · итерация '+i.iteration:''} · ${esc(short(i.id))}</option>`).join('')+(!invs.length?'<option value="">Корневой workflow</option>':'')+definitions(run).map(d=>`<option value="def:${esc(d.id)}">План: ${esc(d.title || d.id)}</option>`).join('');
 $('invocation').value=definition?'def:'+definition:invocation;
 renderGraph();preserve($('tree'),state.timing?.root?treeNode(state.timing.root):'<p>Дерево времени не записано.</p>');renderNode();
 const revealKey=source+selected+nodeID+invocation;if(revealed!==revealKey){const target=[...$('tree').querySelectorAll('[data-node]')].find(n=>n.dataset.node===(nodeID || invocation));for(let parent=target?.parentElement;parent && parent!==$('tree');parent=parent.parentElement)if(parent.tagName==='DETAILS')parent.open=true;revealed=revealKey;}
 const costs=values(run.attempts).flatMap(a=>(a.reported_costs||[]).map(c=>({attempt_id:a.id,...c})));
 preserve($('facts'),section('Выходные артефакты',run.output_artifacts,'outputs',true)+section('Решения',run.decision_ledger,'decisions',true)+section('Проверки',run.check_executions,'checks')+section('Ожидающая приёмка',run.pending_acceptance,'acceptance')+section('Диагностика',run.diagnostics,'diagnostics',true)+section('Отклонения качества',run.waivers,'waivers')+section('Публикации артефактов',run.artifact_publications,'publications')+section('Заявленная стоимость по попыткам и источникам',costs,'costs')+section('Входные артефакты',run.input_artifacts,'inputs')+section('Закреплённые ресурсы контекста',run.context_resources,'resources')+section('Ожидания, сигналы и ограничения',{waits:run.wait_registrations,inbox:run.inbox,guards:run.guards},'control')+raw(run,'run'));
 window.scrollTo(scrollX,scrollY);
}
function navigateNode(id,inv=invocation) {const h=new URLSearchParams({source,run:selected});if(id)h.set('node',id);if(inv)h.set('invocation',inv);location.hash=h;}
function useRoute() {
 const h=route(),next=h.get('run')||'',nextSource=h.get('source')||'';
 const changed=next!==selected || nextSource!==source;if(changed && !selected)listScroll=window.scrollY;
 if(next!==selected || nextSource!==source) {selected=next;source=nextSource;generation++;stamp='';state=null;eventsCursor=0;artifactCache.clear();fileCache.clear();$('events').innerHTML='';$('overview').textContent='Загрузка прогона…';$('tree').innerHTML='';$('node').innerHTML='';$('graph').innerHTML='';$('facts').innerHTML='';for(const name of ['overview','tree','node','graph','facts'])lastMarkup.delete($(name));$('detail-title').textContent='Загрузка прогона…';$('detail-id').textContent=selected;$('detail-error').hidden=true;}
 nodeID=h.get('node')||'';invocation=h.get('invocation')||'';definition='';
 $('run-list').hidden=!!selected;$('detail').hidden=!selected;$('empty-detail').hidden=!!selected;
 if(changed){window.scrollTo(0,selected?0:listScroll);if(selected)$('back-to-runs').focus({preventScroll:true});}
 if(state){renderDetail();const target=[...$('tree').querySelectorAll('[data-node]')].find(n=>n.dataset.node===(nodeID || invocation));for(let parent=target?.parentElement;parent && parent!==$('tree');parent=parent.parentElement)if(parent.tagName==='DETAILS')parent.open=true;}schedule();
}
async function refreshEvents(g) {
 const data=await get('/api/events',{source,id:selected,after:eventsCursor});if(g!==generation)return;
 for(const e of data.events){if(e.seq<=eventsCursor)continue;eventsCursor=e.seq;const row=document.createElement('details');row.className='event';row.innerHTML=`<summary><small>#${e.seq} · v${e.run_version} · ${esc(e.actor)}</small>${esc(e.type)}</summary>${readable(e.data,'event:'+e.seq)}`;$('events').appendChild(row);}
 eventsMore=data.more;$('more-events').hidden=!eventsMore;
}
async function tick() {
 if(busy){queued=true;return;}busy=true;queued=false;const g=generation;
 try {
  const params=query();params.set('page',page);const data=await get('/api/runs',params);if(g!==generation)return;
  if(form.elements.view.value!=='flat'){
   if(!relatedInitialized){for(const run of data.runs.filter(r=>r.children).slice(0,10))expandedRuns.add(runKey(run));relatedInitialized=true;}
   if(expandSearch)for(const run of data.runs)if(run.context && run.children)expandedRuns.add(runKey(run));
   await loadRelated(data.runs,params,g,expandSearch);if(g!==generation)return;
   expandSearch=false;
  }
  renderList(data);$('connection').textContent='Обновлено '+new Date().toLocaleTimeString('ru-RU');
  if(selected) {
   try {const next=await get('/api/run',{source,id:selected});if(g!==generation)return;
    $('detail-error').hidden=true;
    const version=source+selected+':'+next.run_version+':'+next.event_sequence;
    state=next;if(stamp!==version){stamp=version;renderDetail();}
    if(tab==='journal')await refreshEvents(g);
   }catch(err){if(g===generation){$('detail-error').hidden=false;$('detail-error').textContent='Нет актуального чтения. Последние показанные данные могут устареть: '+err.message;}}
  }
 }catch(err){if(g===generation)$('connection').textContent='Нет связи: '+err.message;}
 finally{busy=false;if(queued)setTimeout(tick,0);}
}
function schedule(){if(busy)queued=true;else tick();}
form.onsubmit=e=>e.preventDefault();form.oninput=()=>{clearTimeout(filterTimer);filterTimer=setTimeout(()=>{generation++;page=1;expandSearch=true;schedule();},200);};
$('runs').addEventListener('click',event=>{const expand=event.target.closest('[data-expand]');if(expand){const key=expand.dataset.expand;if(expandedRuns.has(key))expandedRuns.delete(key);else expandedRuns.add(key);schedule();}const more=event.target.closest('[data-more]');if(more){childPages.set(more.dataset.more,(childPages.get(more.dataset.more)||1)+1);schedule();}});
$('previous').onclick=()=>{page--;generation++;schedule();};$('next').onclick=()=>{page++;generation++;schedule();};
$('invocation').onchange=e=>{if(e.target.value.startsWith('def:')){definition=e.target.value.slice(4);invocation='';nodeID='';renderDetail();}else navigateNode('',e.target.value);};
$('detail').addEventListener('click',e=>{const target=e.target.closest('[data-node]');if(target){const obj=nodeObject(state.run,target.dataset.node);navigateNode(target.dataset.node,(state.run.invocations?.[obj?.id] ? obj.id : obj?.workflow_invocation_id || state.run.activations?.[obj?.stage_activation_id]?.workflow_invocation_id || invocation));}if(e.target.closest('#load-files'))loadFiles();});
for(const button of document.querySelectorAll('[data-tab]'))button.onclick=()=>{tab=button.dataset.tab;for(const other of document.querySelectorAll('[data-tab]')){const active=other===button;other.setAttribute('aria-pressed',active);$(other.dataset.tab).hidden=!active;}schedule();};
async function cleanStorage(targetSource,run='') {
 if(maintenanceBusy)return;maintenanceBusy=true;$('delete-run').disabled=true;
 const status=$('maintenance-status');status.textContent='Подготовка очистки: проверяем Run и ссылки на артефакты…';
 try {
  const {token}=await get('/api/maintenance-token');
  const request=async(action,digest='')=>{const response=await fetch('/api/maintenance',{method:'POST',headers:{'Content-Type':'application/json','X-PriFly-Maintenance':token},body:JSON.stringify({source:targetSource,run,action,digest}),signal:AbortSignal.timeout(120000)});const body=await response.json();if(!response.ok)throw new Error(({storage_busy:'Хранилище используется другим процессом. Дождитесь завершения операции и повторите.',run_not_deletable:'Run не завершён окончательно или защищён зависимостями.',cleanup_plan_changed:'Данные изменились после предпросмотра. Откройте очистку заново.'})[body.code]||body.message||body.detail||JSON.stringify(body));return body;};
  const plan=await request('preview');const ids=Object.keys(plan.runs),protectedCount=Object.keys(plan.protected).length;
  if(!ids.length && !plan.files){status.textContent='Удалять нечего. Защищённых Run: '+protectedCount;return;}
  const dialog=$('cleanup-dialog');$('cleanup-preview').innerHTML=`<p class="prose">${esc(plan.root)}</p><p>Будет удалено Run: <b>${ids.length}</b>. Файлов и каталогов: <b>${plan.files}</b> (${sizeText(plan.bytes)} содержимого, без оценки освобождения SQLite).</p><details open><summary>Удаляемые Run</summary><ul>${ids.map(id=>`<li>${esc(id)}</li>`).join('')}</ul></details>${protectedCount?`<details><summary>Защищённые Run: ${protectedCount}</summary><ul>${Object.entries(plan.protected).map(([id,reason])=>`<li>${esc(id)} — ${esc(reason)}</li>`).join('')}</ul></details>`:''}<p>Настройки, пакеты, импортированные и общие артефакты, audit/receipts и внешние рабочие папки сохранятся. Удаляются только служебные рабочие каталоги выбранных Run. Уплотнение базы может потребовать дополнительного свободного места.</p>`;
  dialog.returnValue='';const confirmation=new Promise(resolve=>dialog.addEventListener('close',()=>resolve(dialog.returnValue==='delete'),{once:true}));dialog.showModal();
  if(!await confirmation){status.textContent='Очистка отменена.';return;}
  status.textContent='Очистка выполняется…';const result=await request('delete',plan.digest);
  status.textContent=`Удалено Run: ${result.deleted_runs}; файлов и каталогов: ${result.deleted_files}, содержимого: ${sizeText(result.deleted_bytes)}. ${result.warnings.join(' ')} Размеры обновятся после следующего фонового измерения (интервал 30 секунд).`;
  if(run && source===targetSource && selected===run)location.hash='';schedule();refreshStorage();
 }catch(err){status.textContent='Очистка не завершена: '+err.message;}finally{maintenanceBusy=false;if(state)renderDetail();}
}
$('delete-run').onclick=()=>cleanStorage(source,selected);
$('storage-details').addEventListener('click',event=>{const target=event.target.closest('[data-clean-source]');if(target)cleanStorage(target.dataset.cleanSource);});
$('more-events').onclick=()=>schedule();
window.addEventListener('hashchange',useRoute);useRoute();setInterval(schedule,1500);refreshStorage();setInterval(refreshStorage,5000);
}
