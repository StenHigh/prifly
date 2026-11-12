'use strict';
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const labels = {running:'В работе',completed:'Завершён',failed:'Ошибка',cancelled:'Отменён',uncertain:'Неопределённость',waiting:'Ожидание',pending:'Ожидает',ready:'Готов',succeeded:'Выполнен',rejected:'Отклонён',stopping:'Останавливается',awaiting_host:'Ожидает агента',reported:'Результат получен',disconnected:'Связь потеряна',settled:'Завершено',future:'Ещё не выполнен',unchosen:'Ветвь не выбрана',not_taken:'Не выполнен в этом исполнении',workflow_invocation:'WorkflowInvocation',stage_activation:'StageActivation',step_instance:'StepInstance',attempt:'Attempt',run:'Run',check_execution:'Проверка',step:'Шаг',choice:'Развилка',parallel:'Параллельные ветви',call:'Вложенный workflow',repeat:'Повтор',map:'Обработка элементов',wait:'Ожидание события',finish:'Завершение',elapsed:'Общее время',executor:'Работа исполнителя',queue:'Ожидание допуска',active:'Активное время',quality:'Качество измерения',measured:'Измерено',estimated:'Оценка',partial:'Частично',unknown:'Неизвестно',not_applicable:'Не применимо',unavailable:'Недоступно',incomparable_clock_domains:'Разные сессии часов',calendar_suspend_coverage_unqualified:'Сон машины не учтён достоверно',authority_wall_estimate:'Оценка по календарным часам authority',executor_time:'Работа исполнителя',executor_sum:'Сумма времени исполнителей',executor_active_union:'Интервалы активности исполнителей',check_executor_sum:'Сумма времени проверок',check_executor_active_union:'Интервалы активности проверок',ready_queue:'Ожидание в очереди',restricted_time:'Время ограничений',dispatch_latency:'Задержка передачи',result_to_acceptance:'От результата до приёмки',post_execution_settlement:'Завершение после исполнения',output_contracts:'Требования к результату',subject:'Задача',desired_outcome:'Ожидаемый результат',in_scope:'Входит в задачу',out_of_scope:'За пределами задачи',completion_criteria:'Критерии завершения',source_refs:'Источники',assumptions:'Предположения',confirmation:'Подтверждение',summary:'Описание результата',verdict:'Вердикт',outputs:'Выходы',inputs:'Входы',instructions:'Инструкции',context:'Контекст',description:'Описание',status:'Состояние',reason:'Причина',message:'Сообщение',code:'Код',severity:'Важность',phase:'Фаза',observed:'Зафиксировано',actor:'Автор',actor_id:'Автор',amount:'Сумма',currency:'Валюта',source:'Источник',reported_costs:'Заявленная стоимость',permitted_effects:'Разрешённые действия',workspace:'Рабочая папка',skill_refs:'Навыки',host_state:'Состояние агента',deadline:'Срок',process_outcome:'Итог процесса',error:'Ошибка',evidence_refs:'Свидетельства',effect_receipt_refs:'Подтверждения эффектов'};
Object.assign(labels,{host_take_not_recorded:'агент не отметил, когда взял задание (session take)',decision_request_time_not_recorded:'момент вопроса не записан в этом Run',host_work_needs_session_and_wait:'нужны время сессии и ожидание ответа',difference_of_intervals:'разность интервалов',executor_start_not_observed:'старт исполнителя не наблюдался'});
Object.assign(labels,{ready:'Подготовлен к выдаче Attempt',pending:'Attempt передан агенту',running:'Исполнение начато',future:'Может быть выполнен',skipped:'Будет пропущен',reused:'Из исходного Run',revalidated:'Перепроверен без запуска',no_work:'Нет работы'});
const label = s => labels[s] || s;
const badge = (s,text=label(s)) => `<span class="badge ${/^[a-z_]+$/.test(s || '') ? s : ''}">${esc(text || 'Не записано')}</span>`;
const runBadge = status => status==='running'?badge(status,'В работе'):badge(status);
const parseMaybe = value => { if(typeof value !== 'string') return value; try{return JSON.parse(value);}catch{return value;} };
const values = o => Object.values(o || {}).filter(Boolean);
const short = s => String(s || '').replace(/^[a-z_]+:/,'').slice(0,12);
function executionRole(object,stage) {
 if(object?.session){
  const states={
   awaiting_host:object.session.taken?'Принятие записано. Ожидается отчёт хоста':'Задание выдано. Принятие хостом не записано',
   disconnected:'Связь с хостом потеряна; исход требует проверки',
   waiting_admission:'Ожидает повторного допуска',
   waiting_decision:'Ожидает решения оператора',
   reported:'Отчёт получен; ожидается обработка'
  };
  return {kind:'Внешний host',detail:object.session.principal_id || 'host не записан',
   state:states[object.session.host_state] || (object.started?'Исполнение начато':'Выдано · ожидается отчёт host')};
 }
 if(object?.process)return {kind:'Локальная программа',detail:object.process.executable || 'программа не записана',state:object.started?'Исполнение начато':'Процесс ещё не начал работу'};
 if(stage?.kind==='call')return {kind:'Вложенный workflow',detail:'Не является fork Run',state:label(object?.status || 'future')};
 if(stage?.kind)return {kind:'Управляющий узел workflow',detail:label(stage.kind),state:label(object?.status || 'future')};
 return {kind:'Роль не записана',detail:'Нельзя определить по сохранённым фактам',state:''};
}
// A Run in the admission queue is the authority's fact, not the Run's: it is
// read from the capacity answer, and only when that answer was fetched.
function activityRows(run,capacity,progress) {const rows=[];const queue=Object.entries(capacity?.waiting||{}).sort((a,b)=>a[1]-b[1]||a[0].localeCompare(b[0])),place=queue.findIndex(([id])=>id===run.id);if(place>=0)rows.push({title:'Ожидает допуска',detail:`место ${place+1} из ${queue.length} в очереди authority · capacity ${capacity.capacity}`});for(const a of values(run.attempts).filter(a=>!a.settled)){const r=executionRole(a),p=progressView(progress?.attempts?.find(x=>x.attempt_id===a.id),progress?.as_of?.utc);rows.push({node:a.id,title:r.kind,detail:r.detail+' · '+r.state+(p?.phase?` · ${p.lead==='Сейчас'?'фаза':'последняя фаза, не текущая:'} ${p.phase}${p.counted?` ${p.counted.current}${p.counted.total?'/'+p.counted.total:''}`:''}`:'')});}for(const a of values(run.activations).filter(a=>a.status==='ready'))rows.push({node:a.id,title:'Готово к выдаче',detail:a.stage_id});if(run.pending_decision)rows.push({node:run.pending_decision.attempt_id,title:'Ожидается решение',detail:run.pending_decision.decision_id});for(const w of values(run.wait_registrations).filter(w=>w.status==='active'))rows.push({node:w.activation_id,title:'Ожидается сигнал',detail:w.target_stage_id});if(run.recovery)rows.push({run:run.recovery.source_run_id,title:'Восстановлен от Run',detail:`${run.recovery.reused.length} узлов взято из источника · ${run.recovery.frontier_stage_id}: ${run.recovery.frontier_action==='revalidate'?'перепроверен без запуска':'новое исполнение'}`});else if(run.fork)rows.push({run:run.fork.source_run_id,title:run.fork.reason==='project continuation'?'Продолжение от Run':'Run создан как fork',detail:run.fork.source_run_id});return rows;}
const terminalStatuses=['completed','failed','cancelled'];
// Why a completed Run ended partial, only as far as the record proves it: the
// one finish an invocation reached, then either a waiver that changed that
// finish's declared outcome, or the one settled step whose verdict, call whose
// child outcome, or repeat or choice whose recorded decision routes straight to
// it; a call or a repeat is followed into the body that decided. Anything else
// (a join, two candidates, no finish) is unknown, not "the latest step": parallel and nested branches make that a
// guess.
function invocationWorkflow(run,id) {
 const inv=run.invocations?.[id];
 if(!inv?.parent_invocation_id)return parseMaybe(run.workflow);
 return parseMaybe((run.definitions||[]).find(d=>d.ref?.digest===inv.workflow_ref?.digest)?.bytes);
}
function partialCause(run,choices=[],id=run.root_workflow_invocation_id||'') {
 const stages=invocationWorkflow(run,id)?.definition?.stages||{};
 const here=values(run.activations).filter(a=>(a.workflow_invocation_id||'')===id);
 const finishes=here.filter(a=>a.kind==='finish'&&a.status==='completed');
 if(finishes.length!==1)return {kind:'unknown'};
 const finish=finishes[0].stage_id,description=stages[finish]?.description||'',declared=stages[finish]?.outcome,actual=run.invocations?.[id]?.outcome||run.outcome;
 if(values(run.waivers).length&&declared&&actual&&declared!==actual)return {kind:'waiver',finish,description,waivers:values(run.waivers)};
 const routes=here.flatMap(a=>{
  const on=stages[a.stage_id]?.on||{};
  if(a.kind==='choice'){const d=choices.find(c=>c.stage_activation_id===a.id);return d?.next_stage_id===finish?[{a,choice:d}]:[];}
  if(a.kind==='repeat'){const d=a.repeat?.last_decision;return d?.next_stage_id===finish?[{a,repeat:d}]:[];}
  if(a.kind==='call'){const child=values(run.invocations).find(i=>i.caller_stage_activation_id===a.id);return child?.outcome&&on[child.outcome]===finish?[{a,child}]:[];}
  const step=run.steps?.[a.step_instance_id];return step?.verdict&&on[step.verdict]===finish?[{a,step}]:[];
 });
 if(routes.length!==1)return {kind:'unknown',finish,description};
 const {a,step,child,repeat,choice}=routes[0];
 if(choice)return {kind:'choice',finish,description,stage:a.stage_id,branch:choice.branch_id,node:a.id};
 if(repeat)return {kind:'repeat',finish,description,stage:a.stage_id,route:repeat.route,iteration:repeat.iteration,outcome:repeat.body_outcome,node:repeat.body_workflow_invocation_id,inner:partialCause(run,choices,repeat.body_workflow_invocation_id)};
 if(child)return {kind:'call',finish,description,stage:a.stage_id,outcome:child.outcome,node:child.id,inner:partialCause(run,choices,child.id)};
 const attempt=run.attempts?.[step.attempt_ids?.at(-1)];
 return {kind:'verdict',finish,description,stage:a.stage_id,verdict:step.verdict,node:attempt?.id||step.id,summary:attempt?.accepted?.summary||'',outputs:Object.keys(step.outputs||{}).sort()};
}
// The finish's description is the author's, from the sealed definition.
const finishName = c => `<b>${esc(c.finish)}</b>${c.description?` <span class="muted">— ${esc(c.description.replace(/[.\s]+$/,''))}</span>`:''}`;
function causeHTML(cause) {
 if(cause.kind==='choice')return `<p>Развилка <button data-node="${esc(cause.node)}">${esc(cause.stage)}</button> выбрала ветвь <b>${esc(cause.branch)}</b>, и она ведёт к завершению ${finishName(cause)}.</p>`;
 if(cause.kind==='call')return `<p>Стадия <button data-node="${esc(cause.node)}">${esc(cause.stage)}</button> (вложенный workflow) завершилась с исходом <b>${esc(cause.outcome)}</b>, и маршрут по нему ведёт к завершению ${finishName(cause)}. Внутри неё:</p>`+causeHTML(cause.inner);
 if(cause.kind==='repeat')return `<p>Стадия <button data-node="${esc(cause.node)}">${esc(cause.stage)}</button> (повтор) после итерации ${esc(cause.iteration)} ${cause.route==='on_limit'?'исчерпала лимит повторов':`получила исход <b>${esc(cause.outcome)}</b> (маршрут ${esc(cause.route)})`}, и это ведёт к завершению ${finishName(cause)}. В последней итерации:</p>`+causeHTML(cause.inner);
 if(cause.kind==='verdict')return `<p>Стадия <button data-node="${esc(cause.node)}">${esc(cause.stage)}</button> вернула вердикт <b>${esc(cause.verdict)}</b>, и маршрут по нему ведёт к завершению ${finishName(cause)}.</p>${cause.summary?`<blockquote><small>Описание результата, записанное исполнителем</small><p>${esc(cause.summary)}</p></blockquote>`:''}${cause.outputs.length?`<p class="muted">Выходы этой стадии: ${esc(cause.outputs.join(', '))}</p>`:''}`;
 if(cause.kind==='waiver')return `<p>Завершение ${finishName(cause)} опирается на проверку, снятую waiver, поэтому объявленный исход изменён.</p>${cause.waivers.map(w=>`<p class="muted">${esc(w.reason)} · ${esc(w.approver_id)}</p>`).join('')}`;
 return '<p class="muted">Причина не установлена по записи Run: завершающий маршрут нельзя однозначно связать с одним результатом.</p>';
}
// The "now" section. A terminal Run says it is over instead of "nothing
// recorded", which read as a pause; a live one lists its recorded activity.
function nowHTML(run,activity,choices=[]) {
 const rows=list=>list.map(a=>`<p>${a.node?`<button data-node="${esc(a.node)}">${esc(a.title)}</button>`:a.run?`<a href="#${esc(new URLSearchParams({source:run.source||'',run:a.run}))}">${esc(a.title)}</a>`:`<b>${esc(a.title)}</b>`} <span class="muted">${esc(a.detail)}</span></p>`).join('');
 if(!terminalStatuses.includes(run.status))return activity.length?rows(activity):'<p class="muted">Активная работа и ожидания не записаны.</p>';
 let html=`<p><b>${esc(label(run.status))}.</b> Внутри этого Run ничего не исполняется, и сам он не продолжится. Работа хоста вне его попыток к нему не относится.</p>`+rows(activity.filter(a=>a.run));
 if(run.outcome!=='partial')return html;
 html+='<p>Outcome <b>partial</b>: сценарий дошёл до завершения, но записал результат как частичный. Причина по записи Run:</p>'+causeHTML(partialCause(run,choices));
 return html;
}
// The list's execution cell: created StepInstances and all Attempts are two
// separate counts, never a fraction -- a retry raises only the second one, and
// neither counts future or unchosen stages. A missing or inconsistent value is
// unknown, never zero or a negative difference.
function executionCounts(run) {
 const count=v=>Number.isInteger(v)&&v>=0?v:null,steps=count(run.step_instances),attempts=count(run.attempts),settled=count(run.settled_attempts),hosts=count(run.awaiting_hosts);
 const consistent=attempts!==null&&settled!==null&&settled<=attempts;
 return {steps,attempts,settled:consistent?settled:null,open:consistent?attempts-settled:null,hosts};
}
const known = v => v===null ? 'неизвестно' : String(v);
// What a program said about itself over fd 4, as a reader may take it: the
// phase and count it gave, how old that is, and whether it is now, its last
// word, or unknowable. Never a percentage it did not give, never a success.
function progressView(row,asOf) {
 if(!row)return null;
 const age=row.observed&&asOf?Math.max(0,Math.round((Date.parse(asOf)-Date.parse(row.observed))/1000)):null;
 const counted=Number.isInteger(row.current)?(Number.isInteger(row.total)&&row.total>0&&row.current<=row.total?{current:row.current,total:row.total}:{current:row.current,total:null}):null;
 const lead={reported:row.settled?'Последняя фаза до завершения попытки':'Сейчас',stale:'Последний отчёт — не текущий',read_stale:'Последний отчёт — чтение прервано',not_reported:'Прогресс не сообщается',unavailable:'Прогресс недоступен в этом хранилище'}[row.state]||'Прогресс неизвестен';
 const notes=[];
 if(row.state==='stale')notes.push('драйвер попытки не наблюдается или отчёт от прежнего запуска');
 if(row.settled&&row.phase)notes.push(`попытка ${label(row.attempt_status)}: фаза не говорит об итоге`);
 if(row.rejected)notes.push(`${row.rejected} отчётов отклонено как неверные`);
 if(row.truncated)notes.push('часть отчётов отброшена сверх предела');
 return {lead,phase:row.phase||'',message:row.message||'',counted,age,notes};
}
// A program's own output, as text that cannot act: every control character
// but a line break or tab is shown as an escape, then HTML-escaped.
const inertText = text => String(text||'').replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g,c=>'\\x'+c.charCodeAt(0).toString(16).padStart(2,'0'));
function outputHTML(view) {
 if(!view?.disclosed)return '';
 const lead={reported:view.settled?'Вывод до завершения попытки':'Вывод работающей программы',stale:'Последний сохранённый вывод — не текущий',unavailable:'Вывод недоступен в этом хранилище',not_reported:'Программа ещё ничего не напечатала'}[view.state]||'Вывод неизвестен';
 let html=`<h4>Вывод программы</h4><p><b>${esc(lead)}</b> <span class="muted">· последние 64 КиБ каждого потока, не весь вывод · предел программы ${esc(view.output_limit_bytes)} байт на поток${view.settled?` · попытка ${esc(label(view.attempt_status))}: вывод не говорит об итоге`:''}</span></p>`;
 for(const stream of view.streams||[]){
  const notes=[`байты ${stream.start_offset}–${stream.end_offset}`];
  if(stream.start_offset>0)notes.push(`первые ${stream.start_offset} байт не сохранены`);
  if(stream.gap)notes.push('есть пропуски: часть не поместилась');
  html+=`<p><b>${esc(stream.stream)}</b> <span class="muted">${esc(notes.join(' · '))}</span></p><pre class="program-output">${esc(inertText(stream.text))}</pre>`;
 }
 return html;
}
function nodeObject(run,id) {for(const pool of [run.attempts,run.steps,run.activations,run.invocations,run.check_executions])if(pool?.[id])return pool[id];return id===run.id?run:null;}
function receivedAttempt(run,id) {
 const object=nodeObject(run,id);
 return run.attempts?.[id]||(object?values(run.steps).filter(s=>s.stage_activation_id===object.id).flatMap(s=>s.attempt_ids||[]).map(a=>run.attempts?.[a]).filter(Boolean).at(-1):null);
}
function progressHTML(row,asOf,interrupted=false) {
 if(interrupted && row && !row.settled)row={...row,state:'read_stale'};
 const v=progressView(row,asOf);if(!v)return '';
 return `<h4>Прогресс программы</h4><p><b>${esc(v.lead)}</b>${v.phase?`: ${esc(v.phase)}`:''}${v.counted?` · ${esc(v.counted.current)}${v.counted.total?' / '+esc(v.counted.total):''}`:''}${v.age!==null?` <span class="muted">· ${esc(v.age)} с на момент чтения</span>`:''}</p>${v.counted?.total?`<progress max="${esc(v.counted.total)}" value="${esc(v.counted.current)}"></progress>`:''}${v.message?`<p class="muted">${esc(v.message)}</p>`:''}${v.notes.map(n=>`<p class="muted">${esc(n)}</p>`).join('')}`;
}
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
const runKey = run => JSON.stringify([run.source,run.run_id]);
const runHash = (source,run,params={}) => '#'+new URLSearchParams({source,run,...params});
const runTabs = ['overview','workflow','facts','journal'];
const catalogueViews = ['related-newest','related','flat','board'];
const catalogueStatuses = ['ready','running','stopping','completed','failed','cancelled','uncertain'];
const catalogueFields = ['q','project','status','executor','from','to','view','sort'];
function catalogueAddress(search) {
 const input=new URLSearchParams(search), params=new URLSearchParams();
 for(const name of catalogueFields){
  const value=input.get(name);
  if(!value)continue;
  if(name==='view' && !catalogueViews.includes(value))continue;
  if(name==='status' && !catalogueStatuses.includes(value))continue;
  if(name==='sort' && !['created','oldest','status'].includes(value))continue;
  if((name==='from' || name==='to') && (!/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isFinite(Date.parse(value)) || new Date(value).toISOString().slice(0,10)!==value))continue;
  params.set(name,value);
 }
 if(params.get('from') && params.get('to') && params.get('from')>params.get('to'))params.delete('to');
 const page=/^\d+$/.test(input.get('page')||'')?Number(input.get('page')):1;
 if(Number.isSafeInteger(page) && page>1 && page<=100000000)params.set('page',page);
 return params;
}
function currentRuns(rows) {
 const latest=new Map();
 for(const run of rows){
  const key=runKey(run), previous=latest.get(key);
  if(!previous || run.run_version>previous.run_version ||
   (run.run_version===previous.run_version && run.event_sequence>previous.event_sequence))latest.set(key,run);
 }
 return [...latest.values()].sort((a,b)=>(Date.parse(b.last_observed||'')||0)-(Date.parse(a.last_observed||'')||0) ||
  a.source.localeCompare(b.source) || a.run_id.localeCompare(b.run_id)).slice(0,2);
}
const programsIn = run => values(run?.attempts).filter(a=>!a.session && a.process);
function matchingProgress(progress,programs) {
 if(!progress)return null;
 const known=new Map(programs.map(a=>[a.id,a]));
 return {...progress,attempts:(progress.attempts||[]).flatMap(row=>{
  const attempt=known.get(row.attempt_id);
  if(!attempt)return [];
  if(!attempt.settled || row.settled)return [row];
  return [{...row,settled:true,attempt_status:attempt.status}];
 })};
}
function observedProgress(progress,programs,stale=false) {
 const matched=matchingProgress(progress,programs);
 if(!matched)return '';
 const running=new Set(programs.filter(a=>!a.settled).map(a=>a.id));
 const recent=matched.attempts.filter(a=>!running.size || running.has(a.attempt_id));
 const rows=running.size?recent:recent.filter(a=>a.phase).sort((a,b)=>String(b.observed||'').localeCompare(String(a.observed||''))).slice(0,1);
 return rows.map(row=>`<section class="observed-progress">${rows.length>1?`<small>Attempt ${esc(short(row.attempt_id))}</small>`:''}${progressHTML(row,matched.as_of?.utc,stale)}</section>`).join('');
}
function relatedTitle(run,depth) {
 const relation=run.fork_source_run_id?(run.fork_reason==='project continuation'?'Продолжение':run.fork_reason==='recover_failed_stage'?'Восстановление':run.fork_reason==='resume_stopped_run'?'Возобновление':'Производный Run'):'';
 const missing=run.missing_parent?`<span class="missing-parent">Исходный Run недоступен: ${esc(run.fork_source_run_id)}</span>`:'';
 const context=run.context?'<span class="run-context">Контекст поиска</span>':'';
 const count=run.children?`${run.descendants} связанных Run · ${run.matches} совпадений в ветви`:'';
 return `<div class="run-lineage ${depth?'is-child':''}" data-run-depth="${depth}">${run.children?`<button type="button" class="run-expand" data-expand="${esc(runKey(run))}" data-focus="toggle:${esc(runKey(run))}" aria-expanded="${expandedRuns.has(runKey(run))}" aria-label="${expandedRuns.has(runKey(run))?'Свернуть':'Развернуть'} связанные Run (${run.descendants})">${expandedRuns.has(runKey(run))?'▾':'▸'}</button>`:'<span class="run-expand-spacer" aria-hidden="true"></span>'}<div>${relation?`<span class="run-relation">${relation}</span>`:''}${context}${missing}<a data-focus="${esc(runKey(run))}" href="${esc(runHash(run.source,run.run_id))}">${esc(run.subject || 'Название задачи не записано')}</a>${count?`<small>${esc(count)}</small>`:''}<small>${esc(run.run_id)}</small>${run.error?`<p class="error">${esc(run.error)}</p>`:''}</div></div>`;
}
function runStatus(run) {
 const tip=run.latest_descendant;
 const handed=run.status==='running' && run.active_attempts>0 && run.active_attempts===run.awaiting_hosts;
 const current=`${tip?'<small>Этот Run</small>':''}${runBadge(run.status)}${handed?'<small>Задания выданы хосту</small>':''}${run.outcome?`<small>Outcome: ${esc(run.outcome)}</small>`:''}`;
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
// Use the same saved title sources as the Run catalog.
function runTaskTitle(inputValues,brief){
 const subject=brief?.subject;
 if(typeof subject==='string' && subject)return subject;
 const task=inputValues?.task?.title;
 return (typeof task==='string' && task.trim()) || 'Название задачи не записано';
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
// The exclusive resources this Run sealed, with who holds each now. Holders
// come from the authority's capacity read and name attempts of any Run.
function resourceRows(run,capacity) {
 const byName=new Map();
 for(const [stage,names] of Object.entries(run.stage_resources||{}))for(const name of names){const r=byName.get(name)||{name,limit:run.resource_limits?.[name]??null,stages:[]};r.stages.push(stage);byName.set(name,r);}
 return [...byName.values()].sort((a,b)=>a.name.localeCompare(b.name)).map(r=>({...r,stages:r.stages.sort(),holders:capacity?.resources?(capacity.resources[r.name]||[]).map(attempt=>({attempt,run:capacity.held?.[attempt]||'',mine:!!run.attempts?.[attempt]})):null}));
}
function attemptPhases(node,assisted) {
 const m={...(node?.metrics||{})};
 // An assisted attempt has no process, so its executor time is not a phase of it.
 if(assisted)delete m.executor_time;
 return [['host_pickup','Ждал, пока агент возьмёт задание'],['host_work','Агент работал'],['decision_wait','Ждал ответа на вопрос'],['executor_time','Программа работала'],['result_to_acceptance','Приёмка результата']]
  .map(([key,title])=>({key,title,d:m[key]})).filter(p=>p.d&&p.d.quality!=='not_applicable');
}
function isRelatedView(view) { return view==='related' || view==='related-newest'; }
const boardColumns=[['work','Работа и готовые стадии',['working','ready']],['waiting','Ответ, хост или сигнал',['decision','waiting']],['resource','Ожидание ресурса',['resource']],['stopped','Остановка или неопределённость',['stopped']],['terminal','Завершённые Run',['terminal']],['unknown','Причина неизвестна',['unknown']]];
function boardAction(board) {
 const actions={terminal:'Просмотреть результат',uncertain:'Разобрать неопределённость',restricted:'Проверить ограничения',guarded:'Проверить условие допуска',resume_required:'Проверить возобновление',waiting_decision:'Открыть вопрос',active:'Открыть исполнение',check:'Открыть проверку',acceptance:'Открыть приёмку',publication_checks:'Открыть проверки публикации',cancel:'Проверить завершение отмены',session_resume:'Проверить повторный допуск',stage:'Открыть готовую стадию'};
 return actions[board?.next_action]||(board?.category==='working'?'Открыть исполнение':board?.category==='waiting'?'Открыть задание или ожидание':'Уточнить состояние по журналу');
}
const reasonPriority=['disconnected','unresolved','cancelling','stop','restricted','guarded','resume_required','branch_cancel','branch_resume','decision','resource_busy','admission','signal'];
function reasonRank(reason){
 const rank=reasonPriority.indexOf(reason.code);
 return rank<0?reasonPriority.length:rank;
}
function reasonText(reason){
 if(reason.code==='attempt_deadline_expired')return 'Истёк срок попытки';
 if(reason.code==='terminal')return 'Исполнение завершено';
 return reason.text || 'Причина не установлена по текущим данным';
}
function exactDecision(run,identity){
 const {decision,decision_attempt,decision_digest}=identity;
 if(!decision || !decision_attempt || !decision_digest || !run.attempts?.[decision_attempt])return null;
 const records=(run.decision_ledger || []).filter(record=>record.definition_id===decision &&
  record.attempt_id===decision_attempt && record.definition_digest===decision_digest);
 return records.length===1?records[0]:null;
}
function pendingDecisionTarget(run){
 const pending=run.pending_decision;
 if(!pending)return null;
 const identity={decision:pending.decision_id,decision_attempt:pending.attempt_id,decision_digest:pending.definition_digest};
 return exactDecision(run,identity)?.status==='pending'?identity:null;
}
function decisionRoute(params){
 const keys=['decision','decision_attempt','decision_digest'];
 return keys.some(key=>params.has(key))?Object.fromEntries(keys.map(key=>[key,params.get(key)])):null;
}
function decisionDetailsHTML(run,requested){
 const identity=requested || pendingDecisionTarget(run);
 if(!identity && !run.pending_decision)return '';
 const record=identity && exactDecision(run,identity);
 if(!record)return '<section class="decision-target" id="decision-target" tabindex="-1"><h4>Точная запись вопроса недоступна</h4><p>Доступные записи находятся в разделе «Решения».</p></section>';
 const definition=catalogDecision(run,record.definition_id);
 const choices=definition?.choices || [];
 const status=({pending:'Ожидается ответ',answered:'Ответ записан',closed:'Вопрос закрыт',excluded:'Не применяется'})[record.status] || 'Состояние вопроса не установлено';
 const answer=record.status==='answered'?(choiceTitle(definition,record.value) || JSON.stringify(record.value)):null;
 return `<section class="decision-target" id="decision-target" tabindex="-1"><h4>${esc(definition?.title || record.definition_id)}</h4><p><strong>${status}</strong>${answer?' · '+esc(answer):''}</p>${definition?.description?`<p>${esc(definition.description)}</p>`:''}${choices.length?`<p>Варианты ответа</p><ul>${choices.map(choice=>`<li>${esc(choice.title)}</li>`).join('')}</ul>`:''}</section>`;
}
function diagnosticBelongs(run,attempt,diagnostic){
 return attempt && diagnostic.attempt_id===attempt.id &&
  (!diagnostic.run_id || diagnostic.run_id===run.id) &&
  (!diagnostic.stage_activation_id || diagnostic.stage_activation_id===attempt.stage_activation_id);
}
function attemptDiagnosticsHTML(run,attempt,source){
 if(!attempt)return '';
 const errors=(run.diagnostics || []).filter(diagnostic=>diagnostic.severity==='error' && diagnosticBelongs(run,attempt,diagnostic));
 if(!errors.length && attempt.status!=='failed')return '';
 const body=errors.length?errors.map(diagnostic=>`<p>${esc(reasonText({code:diagnostic.code,text:diagnostic.message}))} <code>${esc(diagnostic.code)}</code></p>`).join(''):
  `<p>Причина ошибки не установлена по текущим данным.</p><a href="${esc(runHash(source,run.id,{tab:'facts',focus:'1'}))}">Открыть результаты и решения</a>`;
 return `<section class="attempt-diagnostic"><h4>Причина ошибки попытки</h4>${body}</section>`;
}
function reasonLink(source,id,reason){
 const tab=runTabs.includes(reason.action)?reason.action:'facts';
 const target={tab,focus:'1'};
 const node=reason.attempt_id || reason.stage_activation_id;
 if(tab==='workflow' && node){
  target.node=node;
  if(reason.workflow_invocation_id)target.invocation=reason.workflow_invocation_id;
 }
 if(tab==='facts' && reason.question)Object.assign(target,reason.question);
 if(tab==='journal' && Number.isSafeInteger(reason.event_seq) && reason.event_seq>0)target.event=reason.event_seq;
 return runHash(source,id,target);
}
function reasonsHTML(reasons,source,id,action){
 const ordered=reasons.filter(r=>r.code!=='terminal').sort((a,b)=>reasonRank(a)-reasonRank(b));
 const [primary,...rest]=ordered;
 const reason=primary || reasons.find(r=>r.code==='terminal') || {text:'Причина текущего состояния не записана',action:'facts'};
 const area={workflow:reason.attempt_id?'Открыть Attempt':reason.stage_activation_id?'Открыть стадию':'Открыть схему и шаги',facts:'Открыть результаты и решения',journal:'Открыть журнал'};
 const key=source+':'+id;
 const show=r=>esc(reasonText(r))+(r.code==='attempt_deadline_expired'?' <code>attempt_deadline_expired</code>':'')+(r.scope?` <small>${esc(r.scope)}</small>`:'');
 const primaryText=reasonText(reason);
 const characters=Array.from(primaryText);
 const main=characters.length>180
  ? `<details data-key="${esc(key+':long-reason')}"><summary>${esc(characters.slice(0,175).join(''))}…</summary><p>${show(reason)}</p></details>`
  : `<p>${show(reason)}</p>`;
 return `<div class="run-explanation">${main}<a data-focus="${esc(key+':reason')}" href="${esc(reasonLink(source,id,reason))}">${esc(reason.question?'Открыть вопрос':action || area[reason.action] || 'Открыть результаты и решения')}</a>${rest.length?`<details data-key="${esc(key+':reasons')}"><summary data-focus="${esc(key+':reasons')}">Другие причины · ${rest.length}</summary><ul class="board-reasons">${rest.map(r=>`<li><a href="${esc(reasonLink(source,id,r))}">${show(r)}</a></li>`).join('')}</ul></details>`:''}</div>`;
}
function listReasons(run){
 return (run.board?.reasons || []).map(reason=>{
  const stage=run.board.stages?.find(s=>s.stage_activation_id===reason.stage_activation_id && reason.stage_activation_id);
  return {...reason,scope:[...(stage?.branches || []),stage?.stage_id].filter(Boolean).join(' / ')};
 });
}
function detailReasons(run,capacity){
 const reasons=[];
 const add=(code,text,action='facts',scope='',target={})=>{
  const activation=run.activations?.[run.attempts?.[target.attempt_id]?.stage_activation_id || target.stage_activation_id];
  reasons.push({code,text,action,scope,...target,...(activation?.workflow_invocation_id?{workflow_invocation_id:activation.workflow_invocation_id}:{})});
 };
 if(run.status==='completed'){
  add('result',run.outcome==='partial'?'Записан частичный результат':'Run завершён');
  return reasons;
 }
 if(run.status==='cancelled'){
  add('result','Отмена завершена','journal');
  return reasons;
 }
 if(run.status==='failed'){
  const current=(run.diagnostics || []).filter(d=>{
   const attempt=run.attempts?.[d.attempt_id];
   const step=run.steps?.[attempt?.step_instance_id];
   return d.severity==='error' && attempt?.status==='failed' && step?.status==='failed' &&
    step?.attempt_ids?.at(-1)===attempt.id &&
    diagnosticBelongs(run,attempt,d);
  });
  if(current.length===1)add(current[0].code,current[0].message,'workflow','',{attempt_id:current[0].attempt_id});
  else add('unknown','Причина ошибки не установлена по текущим данным','journal');
  return reasons;
 }
 if(run.has_unresolved_effects || run.status==='uncertain')add('unresolved','Исход обязательств не установлен');
 if(run.cancel_requested || run.status==='stopping')add('cancelling','Запрошена отмена; завершение ещё не записано','journal');
 if(run.resume_required)add('resume_required','Требуется явное возобновление');
 for(const stop of (run.stops || []).filter(s=>s.status==='active'))add('stop',stop.reason,'facts',stop.scope_id);
 if(run.pending_decision){
  const pending=run.pending_decision, question=pendingDecisionTarget(run);
  const title=catalogDecision(run,pending.decision_id)?.title || pending.decision_id;
  const scope=run.activations?.[run.attempts?.[pending.attempt_id]?.stage_activation_id]?.stage_id || '';
  add('decision','Ожидается ответ: '+title,'facts',scope,question?{question}:{});
 }
 for(const attempt of values(run.attempts).filter(a=>!a.settled)){
  if(attempt.session?.host_state==='waiting_decision' && run.pending_decision?.attempt_id===attempt.id)continue;
  const role=executionRole(attempt);
  const code=({waiting_decision:'decision',waiting_admission:'admission'})[attempt.session?.host_state] || attempt.session?.host_state || 'attempt';
  add(code,role.state || 'Начало исполнения не подтверждено','workflow',run.activations?.[attempt.stage_activation_id]?.stage_id || attempt.id,{attempt_id:attempt.id});
 }
 for(const wait of values(run.wait_registrations).filter(w=>w.status==='active'))add('signal','Стадия ожидает сигнал','facts',wait.target_stage_id);
 if(Object.hasOwn(capacity?.waiting || {},run.id))add('admission','Run ожидает допуска');
 for(const activation of values(run.activations).filter(a=>a.status==='ready')){
  const names=run.stage_resources?.[activation.stage_id] || [];
  const blocked=names.filter(name=>run.resource_limits?.[name]>0 && (capacity?.resources?.[name]?.length || 0)>=run.resource_limits[name]);
  if(blocked.length)add('resource_busy','По данным о ресурсах занято: '+blocked.join(', '),'facts',activation.stage_id);
  else add('ready','Стадия готова к выдаче','workflow',activation.stage_id,{stage_activation_id:activation.id});
 }
 return reasons;
}
function boardCardHTML(run,sources=[]) {
 const source=sources.find(s=>s.id===run.source),board=run.board;
 const link=(text,tab='overview',id=run.run_id,extra={})=>`<a href="${esc(runHash(run.source,id,{tab,...extra}))}">${esc(text)}</a>`;
 const stages=(board?.stages||[]).map(s=>`<li>${link(s.stage_id||'Стадия не записана','workflow',run.run_id,{...(s.workflow_invocation_id?{invocation:s.workflow_invocation_id}:{}),...(s.stage_activation_id?{node:s.stage_activation_id}:{})})} <span>${esc(s.status)}</span>${s.branches?.length?`<div class="board-branches">Ветвь: ${esc(s.branches.join(' / '))}</div>`:''}<small>${esc(s.workflow_invocation_id)}</small>${(s.attempts||[]).map(a=>`<small>${esc(a.attempt_id)} · ${esc(a.status)}${a.host_state?' · '+esc(a.host_state):''}</small>`).join('')}</li>`).join('');

 const parent=run.fork_source_run_id?`<p>Источник: ${link(run.fork_source_run_id,'facts',run.fork_source_run_id)}<small>${esc(run.fork_reason||'Тип связи не записан')}${run.fork_source_available?'':' · источник вне прочитанного каталога'}</small></p>`:'<p class="muted">Связь с исходным Run не записана</p>';
 const children=(run.related_runs||[]).map(child=>`<li>${link(child.run_id,'facts',child.run_id)} · ${esc(child.status)}${child.outcome?' / '+esc(child.outcome):''}</li>`).join('');
 const error=run.error||source?.error;
 return `<article class="board-card" data-run-key="${esc(runKey(run))}" data-run-id="${esc(run.run_id)}"><h3>${link(run.subject||'Название задачи не записано')}</h3><p>${runBadge(run.status||'unknown')}${run.outcome?` <b>Outcome: ${esc(run.outcome)}</b>`:''}</p>${error?`<p class="error" role="status">Устаревшие данные: ${esc(error)}</p>`:''}${reasonsHTML(listReasons(run),run.source,run.run_id,boardAction(board))}<details data-key="board-meta:${esc(runKey(run))}"><summary>Сведения о Run</summary><small>${esc(run.run_id)} · ${esc(run.source)}</small><p>${esc(run.project_title||run.project_fallback||'Имя не записано')}${!run.project_title?'<small>Локальное имя · fallback</small>':''}<small>${esc(run.workflow_id)} · ${esc(run.project)}</small><small>${esc(run.root)}</small></p></details>${stages?`<h4>Активные стадии и ветви</h4><ul class="board-stages">${stages}</ul>`:''}${parent}${children?`<h4>Производные Run</h4><ul>${children}</ul>`:''}<nav aria-label="Подробности Run">${link('Схема','workflow')} · ${link('Результаты и решения','facts')} · ${link('Журнал','journal')}</nav><small>v${esc(run.run_version??'?')} · e${esc(run.event_sequence??'?')} · authority cut ${esc(board?.cut??'?')}</small></article>`;
}
function boardHTML(runs,sources=[]) {
 return '<p class="muted board-note">Текущая страница: одна карточка на Run. Продолжения имеют собственные исходы. Счётчики колонок относятся к этой странице.</p><div class="board-grid" data-scroll="board">'+boardColumns.map(([id,title,categories])=>{
  const cards=runs.filter(r=>categories.includes(boardColumns.some(c=>c[2].includes(r.board?.category))?r.board.category:'unknown'));
  return `<section class="board-column${cards.length?'':' is-empty'}" tabindex="0" data-scroll="board-column-${id}" data-category="${id}" aria-label="${esc(title)}"><h2>${esc(title)} <span>${cards.length}</span></h2>${cards.map(r=>boardCardHTML(r,sources)).join('')}${!cards.length?'<p class="muted">На этой странице нет Run</p>':''}</section>`;
 }).join('')+'</div>';
}
function positionNewGraph(box,key,active){
 if(!box.clientHeight || box.dataset.graphView===key)return;
 box.dataset.graphView=key;
 if(!active.length)return;
 const pad=parseFloat(getComputedStyle(box).paddingTop)||0;
 const ys=active.map(n=>n.y),xs=active.map(n=>n.x);
 box.scrollTop=pad+(Math.min(...ys)+Math.max(...ys)+78)/2-box.clientHeight/2;
 box.scrollLeft=pad+(Math.min(...xs)+Math.max(...xs)+230)/2-box.clientWidth/2;
}
if(typeof module !== 'undefined') module.exports = {exactDecision,pendingDecisionTarget,decisionDetailsHTML,attemptDiagnosticsHTML,reasonLink,positionNewGraph,runKey,catalogueAddress,currentRuns,matchingProgress,observedProgress,runHash,boardHTML,boardCardHTML,detailReasons,listReasons,reasonsHTML,isRelatedView,esc,label,duration,graphData,definitionInvocation,fileChanges,nodeCard,executionRole,activityRows,runTaskTitle,progressView,progressHTML,outputHTML,inertText,receivedAttempt,partialCause,nowHTML,executionCounts,relatedTitle,expandedRuns,runStatus,stepReceipt,launchInputs,timeBreakdown,attemptPhases,resourceRows};
if(typeof document !== 'undefined') {
const $ = id => document.getElementById(id);
let selected='',source='',generation=0,page=1,pages=1,state=null,nodeID='',invocation='',definition='',tab='overview',stamp='',filterTimer,revealed='';
let observing=false,readingList=false,detailFlight=null,detailAux=new Set(),focusRoute='',restoreTreeFocus='';
let selectedRead={at:null,stale:false},eventView='recent',eventCursor=0,eventMore=false,eventPending=true,eventKey='',recentKey='';
let listScroll=0,listPositions=new Map(),listFocus=null,maintenanceBusy=false,storageBusy=false;
let relatedInitialized=false,expandSearch=false;
const childPages=new Map(),childCache=new Map();
const artifactCache=new Map(),fileCache=new Map();
const fileKey=a=>source+selected+nodeID+JSON.stringify(a?.accepted?.outputs || {});
const form=$('filters');
const query = () => {
 const params=new URLSearchParams(new FormData(form));
 if(isRelatedView(params.get('view')))params.delete('sort');
 else if(form.elements.sort.value)params.set('sort',form.elements.sort.value);
 return params;
};
const route = () => new URLSearchParams(location.hash.slice(1));
const endpoint = path => new URL(path.slice(1),new URL('.',location.href));
async function get(path,params={},signal) {
 const url=endpoint(path);url.search=new URLSearchParams(params);
 const timeout=AbortSignal.timeout(15000);
 const response=await fetch(url,{signal:signal?AbortSignal.any([signal,timeout]):timeout});
 if(!response.ok) throw new Error((await response.text()).slice(0,1200));
 return response.json();
}
function syncCatalogueAddress() {
 const url=new URL(location.href),params=catalogueAddress(listParams());
 if(params.get('view')==='related-newest')params.delete('view');
 for(const name of [...catalogueFields,'page'])url.searchParams.delete(name);
 for(const [name,value] of params)url.searchParams.set(name,value);
 if(url.href!==location.href)history.replaceState(history.state,'',url);
}
function restoreCatalogueAddress() {
 const params=catalogueAddress(location.search);
 for(const name of catalogueFields){
  const field=form.elements[name],value=params.get(name)||(name==='view'?'related-newest':'');
  if(field.tagName==='SELECT' && value && ![...field.options].some(o=>o.value===value))field.add(new Option(name==='status'?label(value):value,value));
  field.value=value;
 }
 page=Number(params.get('page'))||1;
}
function setCatalogueView(kind) {
 const presets={current:['flat',$('current-state').value],recent:['flat',''],failed:['flat','failed'],history:['related-newest','']};
 const [view,status]=presets[kind];
 if(status && ![...form.elements.status.options].some(o=>o.value===status))form.elements.status.add(new Option(label(status),status));
 form.elements.view.value=view;
 form.elements.status.value=status;
 form.elements.sort.value='';
 changeCatalogue();
}
function renderCatalogueControls() {
 const {view,status,sort}=form.elements;
 const preset=sort.value?'':view.value==='flat'?(['running','stopping'].includes(status.value)?'current':status.value==='failed'?'failed':status.value?'':'recent'):isRelatedView(view.value)?'history':'';
 $('current-status').hidden=preset!=='current';
 if(preset==='current')$('current-state').value=status.value;
 for(const button of $('catalog-views').querySelectorAll('[data-view]'))button.setAttribute('aria-pressed',String(button.dataset.view===preset));
 for(const suffix of ['','-top']){
  $('page'+suffix).textContent=pages?`${page} / ${pages}`:'';
  $('previous'+suffix).disabled=page<=1;
  $('next'+suffix).disabled=page>=pages;
 }
}
function changeCatalogue(debounce=false){
 clearTimeout(filterTimer);
 generation++;page=1;expandSearch=true;
 syncCatalogueAddress();prepareListRead(listParams());renderCatalogueControls();
 if(debounce)filterTimer=setTimeout(schedule,200);
 else schedule();
}

const pinStorage='prifly-full-observation/1';
const pins=new Map();
const titles=new Map();
const sourceNames=new Map();
function restorePins() {
 try{
  const pairs=JSON.parse(sessionStorage.getItem(pinStorage));
  if(!Array.isArray(pairs))return;
  for(const pair of pairs){
   if(pins.size>=2)break;
   if(typeof pair?.source!=='string' || !pair.source || typeof pair.run_id!=='string' || !pair.run_id)continue;
   const key=runKey(pair);
   if(!pins.has(key))pins.set(key,{source:pair.source,run_id:pair.run_id});
  }
 }catch{}
}
function pinControl(source,id,title='') {
 const pair={source,run_id:id},key=runKey(pair);
 return `<button type="button" data-pin="${esc(key)}" data-source="${esc(source)}" data-run="${esc(id)}" data-title="${esc(title)}" data-focus="${esc('pin:'+key)}" aria-label="${esc((pins.has(key)?'Снять закрепление: ':'Закрепить: ')+(title||short(id)))}">${pins.has(key)?'Снять':'Закрепить'}</button>`;
}
function togglePin(source,id,title) {
 const pair={source,run_id:id},key=runKey(pair);
 if(pins.has(key)){cancelPinReads(pins.get(key));pins.delete(key);titles.delete(key);}
 else if(pins.size<2)pins.set(key,pair);
 else{
  const box=observing?'observation-message':selected?'pin-message':'catalogue-message';
  $(box).textContent='Снимите одно из двух закреплений, чтобы добавить этот Run.';
  return;
 }
 if(pins.has(key) && title && title!=='Название задачи не записано')titles.set(key,title);
 for(const id of ['observation-message','pin-message','catalogue-message'])$(id).textContent='';
 try{sessionStorage.setItem(pinStorage,JSON.stringify([...pins.values()].map(({source,run_id})=>({source,run_id}))));}catch{}
 updatePinControls();
 if(observing)refreshPins();
}
function updatePinControls() {
 $('pin-count').textContent=pins.size;
 $('observe-link').setAttribute('aria-current',observing?'page':'false');
 $('observation-count').textContent=`Закреплено ${pins.size} из 2`;
 $('no-pins').hidden=!!pins.size;
 const control=$('pin-run');
 control.hidden=!selected || !source;
 control.textContent=pins.has(runKey({source,run_id:selected}))?'Снять закрепление':'Закрепить';
 // Re-render only the catalogue preview that owns pin controls.
 if(!selected && !observing && currentRead)renderCurrent();
 if(observing)renderPins();
}
function rememberSource(run){
 const name=run.project_title || run.project_fallback;
 if(name)sourceNames.set(run.source,name);
}
function renderCurrent(){
 if(!currentRead)return;
 const {rows,counts,partial}=currentRead;
 $('current').hidden=!(rows.length || partial);
 $('current-counts').textContent=counts.join(' · ')+(partial?' · выборка неполная':'');
 preserve($('current-runs'),rows.map(run=>{
  const title=run.subject||'Название задачи не записано';
  if(run.subject && pins.has(runKey(run)))titles.set(runKey(run),run.subject);
  rememberSource(run);
  const stages=(run.board?.stages||[]).map(stage=>stage.stage_id).filter(Boolean);
  return `<article class="observation-card current-run" data-current-key="${esc(runKey(run))}"><div class="observation-heading"><h3><a href="${esc(runHash(run.source,run.run_id,{tab:'overview'}))}">${esc(title)}</a></h3>${runStatus(run)}</div><small>${esc([run.project_title||run.project_fallback,stages.slice(0,2).join(', ')].filter(Boolean).join(' · '))}</small>${run.error?'<p class="error">Данные источника устарели.</p>':''}${pinControl(run.source,run.run_id,run.subject||'')}</article>`;
 }).join('') || '<p class="muted">Текущие Run не прочитаны.</p>');
}
let currentRead=null;
async function refreshCurrent(params,g) {
 const results=await Promise.allSettled(['running','stopping'].map(status=>{
  const search=new URLSearchParams(params);
  search.set('view','flat');search.set('status',status);search.delete('page');search.delete('sort');
  return get('/api/runs',search);
 }));
 if(g!==generation)return;
 const rows=[],counts=[];
 let partial=false,truncated=false;
 for(const [i,result] of results.entries()){
  const name=i?'останавливается':'в работе или ожидании';
  if(result.status==='rejected'){partial=true;counts.push(`${name}: недоступно`);continue;}
  const data=result.value;
  truncated ||= data.filtered>data.runs.length;
  counts.push(`${data.filtered} ${name}`);
  rows.push(...data.runs);
  partial ||= data.discovery.scanning || data.sources.some(s=>s.error || !s.indexed);
 }
 const preview=currentRuns(rows);
 if(truncated || new Set(rows.map(runKey)).size>preview.length)counts.push(`показано ${preview.length}`);
 // Retain the compact preview only when it adds information or an unavailable source matters.
 currentRead={rows:preview,counts,partial};
 renderCurrent();
}

function snapshotReadHTML(at,stale) {
 return `<span>${stale?'Последнее успешное чтение':'Чтение Run'}: <time>${esc(date(at))}</time></span>`;
}
function renderPins() {
 for(const card of [...$('pinned-runs').children])if(!pins.has(card.dataset.pinKey))card.remove();
 for(const [key,pin] of pins){
  let card=[...$('pinned-runs').children].find(el=>el.dataset.pinKey===key);
  if(!card){card=document.createElement('article');card.className='observation-card pinned-run';card.dataset.pinKey=key;card.dataset.runId=pin.run_id;card.dataset.source=pin.source;$('pinned-runs').append(card);}
  const run=pin.view?.run,title=pin.title||titles.get(key)||'Название задачи не прочитано';
  const outcome=run?.outcome;
  const activity=run?activityRows(run,null,null).filter(row=>!row.run):[];
  const rows=activity.map(row=>{
   const attempt=run.attempts?.[row.node],stage=run.activations?.[attempt?.stage_activation_id||row.node]?.stage_id;
   const activity=attempt?.process?executionRole(attempt).state:row.detail;
   return `<li><b>${esc(stage||row.title)}</b> · ${esc(activity)}</li>`;
  });
  const origin=run?.project_title || sourceNames.get(pin.source) || 'Источник '+short(pin.source);
  const main=run?`<div>${runBadge(run.status)}${outcome?` · ${esc(label(outcome))}`:''}</div>${activity.length?`<ul class="observation-activity">${rows.join('')}</ul>`:terminalStatuses.includes(run.status)?'':'<p class="muted">Текущая стадия не записана.</p>'}`:'<p class="muted">Снимок Run ещё не прочитан.</p>';
  const reasons=run?detailReasons(run,null).filter(reason=>run.status==='failed' || reasonPriority.includes(reason.code) || reason.code==='awaiting_host'):[];
  const explanation=reasons.length?(pin.error?'<small>Последняя известная причина</small>':'')+reasonsHTML(reasons,pin.source,pin.run_id):'';
  const error=pin.error?`<p class="error">${esc(readStatus(!!run,'прогон'))}</p>`:'';
  const progress=run?observedProgress(pin.progress,programsIn(run),!!pin.error||!!pin.progressError):'';
  preserve(card,`<div class="observation-heading"><h2>${esc(title)}</h2>${pinControl(pin.source,pin.run_id,pin.title||titles.get(key)||'')}</div><small>${esc(origin)} · Run ${esc(short(pin.run_id))}</small>${error}${main}${explanation}${progress}${pin.progressError?'<p class="muted">Прогресс программы недоступен.</p>':''}${run?`<div class="read-facts muted">${snapshotReadHTML(pin.at,pin.error)}<span>Наблюдение Run: ${esc(date(run.last_observed?.utc))}</span></div>`:''}<a data-focus="${esc('open:'+key)}" href="${esc(runHash(pin.source,pin.run_id,{tab:'overview'}))}">Открыть Run →</a>`);
 }
}
function cancelPinReads(pin){
 pin.runFlight?.abort();
 pin.progressFlight?.abort();
 pin.runFlight=pin.progressFlight=null;
}
const pinIsCurrent=(key,pin,signal)=>observing && pins.get(key)===pin && !signal.aborted;
async function refreshPinProgress(key,pin,params){
 if(pin.progressFlight)return;
 const flight=new AbortController();pin.progressFlight=flight;
 try{
  const progress=await get('/api/progress',params,flight.signal);
  if(!pinIsCurrent(key,pin,flight.signal))return;
  pin.progress=matchingProgress(progress,programsIn(pin.view.run));
  pin.progressError=false;
 }catch{
  if(!pinIsCurrent(key,pin,flight.signal))return;
  pin.progressError=true;
 }finally{if(pin.progressFlight===flight)pin.progressFlight=null;}
 renderPins();
}
async function refreshPin(key,pin){
 if(pin.runFlight)return;
 const flight=new AbortController();pin.runFlight=flight;
 const params={source:pin.source,id:pin.run_id};
 try{
  const view=await get('/api/run',params,flight.signal);
  if(view.run?.id!==pin.run_id)throw new Error('Mismatched Run');
  if(!pinIsCurrent(key,pin,flight.signal))return;
  pin.view=view;pin.at=new Date().toISOString();pin.error=false;
  const title=runTaskTitle(view.input_values,null);
  if(title!=='Название задачи не записано')pin.title=title;
  const programs=programsIn(view.run);
  pin.progress=matchingProgress(pin.progress,programs);
  if(programs.length)refreshPinProgress(key,pin,params);
  else pin.progressError=false;
 }catch{
  if(!pinIsCurrent(key,pin,flight.signal))return;
  pin.error=true;
 }finally{if(pin.runFlight===flight)pin.runFlight=null;}
 renderPins();
}
function refreshPins(){
 for(const [key,pin] of pins)refreshPin(key,pin);
}

const lastMarkup=new WeakMap();
function preserve(box,html) {
 if(lastMarkup.get(box)===html)return;
 lastMarkup.set(box,html);
 const open=new Map([...box.querySelectorAll('details[data-key]')].map(d=>[d.dataset.key,d.open]));
 const scroll=new Map([...box.querySelectorAll('[data-scroll]')].map(e=>[e.dataset.scroll,[e.scrollLeft,e.scrollTop]]));
 const active=document.activeElement;
 const focus=box.contains(active)?active.dataset.focus:null;
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
function resourceHTML(run,capacity) {
 const rows=resourceRows(run,capacity);
 if(!rows.length)return '';
 const row=(title,body)=>`<div class="receipt-row"><div class="receipt-key">${title}</div><div>${body}</div></div>`;
 return '<details class="facts-section" data-key="resources" open><summary>Исключительные ресурсы</summary><div class="facts-body"><p class="muted">Стадии разных Run, держащие один ресурс, не идут одновременно сверх лимита; ждущая стадия допускается на следующем запуске драйвера, когда держатель завершится.</p>'+rows.map(r=>row(`<code>${esc(r.name)}</code><br><small class="muted">лимит ${esc(r.limit)}</small>`,`стадии: ${r.stages.map(s=>`<code>${esc(s)}</code>`).join(' ')}<br>${r.holders===null?'<small class="muted">держатели не прочитаны</small>':r.holders.length?`<small>держат сейчас: ${r.holders.map(h=>`${h.mine?'этот Run':esc(short(h.run))} (${esc(short(h.attempt))})`).join(', ')}</small>`:'<small class="muted">свободен</small>'}`)).join('')+'</div></details>';
}
function launchHTML(run,inputValues) {
 const l=launchInputs(run,inputValues);
 if(!l.inputs.length&&!l.decisions.length)return '';
 const row=(title,body)=>`<div class="receipt-row"><div class="receipt-key">${title}</div><div>${body}</div></div>`;
 let html='<details class="facts-section receipt" data-key="launch"><summary>Входы и решения запуска</summary><div class="facts-body">';
 if(l.profile)html+=row('Профиль пакета',`<b>${esc(l.profile)}</b> <small class="muted">${esc(answerSources[l.profileSource]||l.profileSource)}${l.policy?' · политика: '+esc(l.policy):''}</small>`);
 html+=l.inputs.map(i=>row(`Вход <code>${esc(i.port)}</code>`,`${launchValue(i.value,'launch:in:'+i.port)}<br><small class="muted">${esc(answerSources[i.source]||i.source)}</small>`)).join('');
 html+=l.decisions.map(d=>row(`${esc(d.title)}<br><small class="muted">${esc(d.id)}${d.phase==='runtime'?' · вопрос во время работы':''}</small>`,d.excluded?'<span class="muted">не применяется к этому профилю</span>':!d.answered?'<span class="muted">не отвечено до запуска</span>':`${d.choice?`<b>${esc(d.choice)}</b> `:''}${shownValue(d.value,'launch:d:'+d.id)}<br><small class="muted">${esc(answerSources[d.source]||d.source||'источник не записан')}</small>`)).join('');
 return html+'</div></details>';
}
function timeHTML(timing) {
 const t=timeBreakdown(timing);
 if(!t)return '';
 const total=amount(t.elapsed)||0,share=d=>total&&amount(d)!=null?Math.min(100,Math.round(amount(d)*100/total)):0;
 let html=`<details class="facts-section" data-key="time"><summary>Куда ушло время</summary><div class="facts-body"><p>Всего: <b>${esc(duration(t.elapsed))}</b></p><div class="time-bars">`;
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
 for(const element of box.querySelectorAll('[data-run-depth]')) {const depth=Number(element.dataset.runDepth);if(Number.isSafeInteger(depth)&&depth>=0)element.style.setProperty('--run-depth',String(depth));}
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
 form.elements.sort.disabled=isRelatedView(form.elements.view.value);
 form.elements.sort.title=form.elements.sort.disabled?'Порядок связанных Run выбран в поле «Вид»':'';
 $('totals').innerHTML=`<span><b>${data.total}</b> прогонов</span><span><b>${data.active}</b> с активными попытками</span>`;
 const d=data.discovery,errors=data.sources.filter(s=>s.error),pending=data.sources.filter(s=>!s.indexed && !s.error).length;
 $('coverage-title').textContent=`${data.sources.length} хранилищ · ${d.scanning?'поиск продолжается':'обход завершён'} · ${d.directories} каталогов${d.unreadable?' · недоступно: '+d.unreadable:''}${errors.length?' · ошибок чтения: '+errors.length:''}${pending?' · индексируется: '+pending:''}`;
 preserve($('sources'),`<p class="muted">Область поиска: ${esc(d.roots.join(', '))}. Исключены системные/сетевые каталоги: ${d.excluded}. Символические ссылки при обходе не раскрываются; физические каталоги читаются по исходному пути. Полнота относится к доступной проверенной области.</p>${d.completed?`<p>Последний обход: ${esc(date(d.completed))}</p>`:''}${data.sources.map(s=>`<div class="source"><b>${esc(s.project || 'Хранилище')}</b><small>${esc(s.root)}</small>${s.conflict?'<p class="error">Конфликт: одинаковая идентичность у нескольких хранилищ. Их Run показаны отдельно.</p>':''}<p class="${s.error?'error':'muted'}">${esc(s.error || (s.indexed?'Прочитано '+date(s.updated):'Индексируется…'))}</p></div>`).join('')}${d.errors.length?section('Недоступные пути (примеры)',d.errors,'scan-errors'):''}`);
 for(const run of data.runs){
  rememberSource(run);
  if(run.subject && pins.has(runKey(run)))titles.set(runKey(run),run.subject);
 }
 options('project',data.filters.projects,'Все проекты');options('status',data.filters.statuses,'Все статусы');options('executor',data.filters.executors,'Все исполнители');
 const selectedFilters=['project','status','executor','from','to','sort'].filter(name=>form.elements[name].value).length+Number(form.elements.view.value!=='related-newest');
 $('filter-summary').textContent='Фильтры и вид'+(selectedFilters?' · '+selectedFilters:'');
 const boardView=form.elements.view.value==='board';
 document.querySelector('.table-wrap').hidden=boardView;$('board').hidden=!boardView;
 const listBox=boardView?$('board'):$('runs');
 const anchor=[...listBox.querySelectorAll('[data-run-key]')].find(row=>row.getBoundingClientRect().bottom>0);
 const anchorTop=anchor?.getBoundingClientRect().top,anchorKey=anchor?.dataset.runKey;
 if(boardView)preserve($('board'),boardHTML(data.runs,data.sources));
 const listed=[];
 const append=(run,depth)=>{
  listed.push({run,depth});
  if(!isRelatedView(form.elements.view.value) || !expandedRuns.has(runKey(run)))return;
  const loaded=childCache.get(runKey(run));
  for(const child of loaded?.runs||[])append(child,depth+1);
  if(loaded && loaded.loadedPages<loaded.pages)listed.push({more:run,depth:depth+1});
 };
 for(const run of data.runs)append(run,0);
 if(!boardView)preserve($('runs'),listed.length?listed.map(({run,depth,more})=>{
  if(more)return `<tr><td colspan="5" class="run-more" data-run-depth="${depth}"><button type="button" data-more="${esc(runKey(more))}">Показать ещё дочерние Run →</button></td></tr>`;
  const hash=new URLSearchParams({source:run.source,run:run.run_id,tab:'overview'});
  const task=run.subject || 'Название задачи не записано',project=run.project_title || run.project_fallback || 'Локальное имя не найдено',fallback=!run.project_title;
  return `<tr role="row" data-run-key="${esc(runKey(run))}" class="${run.source===source && run.run_id===selected?'selected':''} ${run.context?'run-context-row':''}"><td role="cell" class="run-title-cell">${isRelatedView(form.elements.view.value)?relatedTitle(run,depth):`<a data-focus="${esc(runKey(run))}" href="#${esc(hash)}">${esc(task)}</a><small>${esc(run.run_id)}</small>${run.error?`<p class="error">${esc(run.error)}</p>`:''}`}</td><td role="cell" class="run-project-cell"><details data-key="project:${esc(runKey(run))}"><summary>${esc(project)}</summary>${fallback?'<small>Локальное имя</small>':''}<small>${esc(run.project)}</small><small>${esc(run.workflow_id)}</small><small>${esc(run.root)}</small></details></td><td role="cell" class="run-status-cell">${runStatus(run)}${reasonsHTML(listReasons(run),run.source,run.run_id,boardAction(run.board))}</td><td role="cell" class="run-execution-cell">${executionCell(run)}</td><td role="cell" class="run-created-cell">${esc(date(run.created))}<small>v${esc(run.run_version)} · e${esc(run.event_sequence)}</small></td></tr>`;
 }).join(''):'<tr><td colspan="5" class="empty">Прогоны не найдены в прочитанной области. Измените фильтры или проверьте состояние поиска выше.</td></tr>');
 if(!selected && anchorKey && anchorTop!=null){const current=[...listBox.querySelectorAll('[data-run-key]')].find(row=>row.dataset.runKey===anchorKey);if(current)window.scrollBy(0,current.getBoundingClientRect().top-anchorTop);}
 $('matches').textContent=`Найдено: ${data.filtered}`;renderCatalogueControls();syncCatalogueAddress();
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
function executionCell(run) {const c=executionCounts(run);return `Шагов создано: ${esc(known(c.steps))}<br>Попыток: ${esc(known(c.attempts))}<small>Учтено: ${esc(known(c.settled))} · не учтено: ${esc(known(c.open))}${c.hosts?` · выдано хосту: ${esc(c.hosts)}`:''}</small>`;}
function currentWorkflow(run) {
 if(definition) return definitions(run).find(d=>d.id===definition) || run.workflow;
 const ref=run.invocations?.[invocation]?.workflow_ref || run.workflow_ref;
 return (run.definitions || []).find(d=>d.ref?.digest===ref?.digest)?.bytes || run.workflow;
}
function renderGraph() {
 if(!state)return;const run=state.run,workflow=parseMaybe(currentWorkflow(run));
 const selectedActivation=run.attempts?.[nodeID]?.stage_activation_id || run.steps?.[nodeID]?.stage_activation_id || nodeID;
 const graphInvocation=definition?definitionInvocation(run,definition):invocation,data=graphData(workflow,run,graphInvocation,state.choices,state.input_values),byID=new Map(data.nodes.map(n=>[n.id,n]));
 const svg=`<svg width="${data.width}" height="${data.height}" viewBox="0 0 ${data.width} ${data.height}" role="group" aria-label="Связи закреплённых стадий workflow"><defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z"/></marker></defs>${data.edges.map((e,index)=>{const a=byID.get(e.from),b=byID.get(e.to);if(!a||!b)return '';const x=a.x+230,y=a.y+39+e.fromPort,tx=b.x,ty=b.y+39+e.toPort,channel=e.route==='return'?42+index%3*18:data.height-42-index%3*18,route=e.route==='forward'?`M ${x} ${y} H ${tx-22} V ${ty} H ${tx}`:`M ${x} ${y} H ${x+22} V ${channel} H ${tx-22} V ${ty} H ${tx}`,classes=[`route-${e.route}`,e.path?'active-path':'',e.muted?'muted-path':''].filter(Boolean).join(' ');return `<path class="${classes}" d="${route}" marker-end="url(#arrow)"/><text class="edge-label ${e.muted?'muted-path':''}" x="${(x+tx)/2}" y="${e.route==='forward'?(y+ty)/2-8:channel-6}" text-anchor="middle">${esc(e.label)}</text>`;}).join('')}${data.nodes.map(n=>`<g tabindex="0" role="button" aria-label="${esc(n.title+', '+label(n.state)+(n.reason?': '+n.reason:''))}" data-stage="${esc(n.id)}" data-focus="stage:${esc(n.id)}" class="node-${n.visual} ${(n.id===nodeID || n.actual?.some(a=>a.id===selectedActivation))?'selected':''}"><rect x="${n.x}" y="${n.y}" width="230" height="78" rx="9"/><text x="${n.x+12}" y="${n.y+22}">${esc(n.title.length>26?n.title.slice(0,24)+'…':n.title)}</text><text x="${n.x+12}" y="${n.y+43}">${esc(label(n.kind))}</text><text x="${n.x+12}" y="${n.y+64}">${esc(n.state==='reused'||n.state==='revalidated'?label(n.state):n.visual==='skipped'?label('skipped'):(n.visual==='reachable'?'Далее':label(n.state)))}</text>${n.reason?`<title>${esc(n.reason)}</title>`:''}</g>`).join('')}</svg>`;
 const left=$('graph').scrollLeft,top=$('graph').scrollTop;preserve($('graph'),svg);$('graph').scrollLeft=left;$('graph').scrollTop=top;
 // Centre the active stages on first opening a graph. Subsequent polling keeps its position.
 const focusKey=JSON.stringify([source,selected,invocation,definition]);
 positionNewGraph($('graph'),focusKey,data.nodes.filter(n=>n.visual==='current'));
 $('graph').onclick=event=>{const target=event.target.closest('[data-stage]');if(!target)return;const n=byID.get(target.dataset.stage);if(n.invocation){navigateNode('',n.invocation,'');return;}if(n.ref){navigateNode('','',n.ref.id);return;}navigateNode(n.actual?.at(-1)?.id || n.id,invocation);};
 $('graph').onkeydown=event=>{if(event.key==='Enter'||event.key===' '){const n=event.target.closest('[data-stage]');if(n){event.preventDefault();n.dispatchEvent(new MouseEvent('click',{bubbles:true}));}}};
}
function treeNode(n,depth=0) {
 return `<details class="node" data-key="tree:${esc(n.kind+':'+n.id)}" ${depth<2?'open':''}><summary><button data-node="${esc(n.id)}" data-focus="node:${esc(n.id)}" aria-pressed="${n.id===nodeID}">${esc(n.stage_id || label(n.kind))} · ${esc(short(n.id))}</button> ${badge(n.status)} <small>${esc(duration(n.metrics?.elapsed))}</small></summary>${(n.children||[]).map(c=>treeNode(c,depth+1)).join('')}</details>`;
}
function timingNode(node,id) {if(!node)return null;if(node.id===id)return node;for(const child of node.children||[]){const found=timingNode(child,id);if(found)return found;}return null;}
function renderNode() {
 const run=state.run,object=nodeObject(run,nodeID),timing=timingNode(state.timing?.root,nodeID);
 const activation=run.activations?.[object?.stage_activation_id||object?.activation_id||nodeID],stageID=activation?.stage_id||object?.stage_id||nodeID,graph=graphData(parseMaybe(currentWorkflow(run)),run,definition?definitionInvocation(run,definition):invocation,state.choices,state.input_values);
 let html=attemptDiagnosticsHTML(run,run.attempts?.[nodeID],source)+nodeCard(graph,stageID,object,run),role=executionRole(object,parseMaybe(currentWorkflow(run))?.definition?.stages?.[stageID]);
 const received=receivedAttempt(run,nodeID);
 if(received)html+=receiptHTML(run,received,state.input_values,attemptPhases(timingNode(state.timing?.root,received.id),!!received.session),received.id===nodeID?'':'Последнее исполнение этой стадии: '+short(received.id));
 if(!object) {
  const stage=parseMaybe(currentWorkflow(run))?.definition?.stages?.[nodeID];
  $('node-title').textContent=stage ? nodeID+' · '+label(stage.kind) : 'Выберите шаг или попытку';
  if(stage){html+='<p class="muted">Закреплённое определение стадии.</p>'+readable(stage,nodeID);const actual=values(run.activations).filter(a=>a.stage_id===nodeID && (a.workflow_invocation_id || '')===invocation);html+=actual.map(a=>`<p><button data-node="${esc(a.id)}">Открыть исполнение ${esc(short(a.id))}</button></p>`).join('');}
  preserve($('node'),html);return;
 }
 $('node-title').textContent=(timing?.stage_id || label(timing?.kind) || 'Исполнение')+' · '+short(nodeID);html+=`<h4>Роль исполнения</h4><p><b>${esc(role.kind)}</b><br><span class="muted">${esc(role.detail)}${role.state?' · '+esc(role.state):''}</span></p>`;
 html+=`<p>${badge(object.status)} ${esc(object.verdict || object.outcome || '')}</p>`;
 if(received&&!received.session)html+=progressHTML(state.progress?.attempts?.find(a=>a.attempt_id===received.id),state.progress?.as_of?.utc,selectedRead.stale||detailAux.has('progress'))+(state.output?.attempt_id===received.id?outputHTML(state.output):'');
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
 if(timing?.children?.length)html+='<h4>Вложенные исполнения</h4>'+timing.children.map(n=>`<p><button data-node="${esc(n.id)}" data-focus="node:${esc(n.id)}">${esc(label(n.kind))} · ${esc(n.stage_id || short(n.id))}</button> ${badge(n.status)}</p>`).join('');
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
function currentStages(run){
 const ids=new Set(values(run.attempts).filter(a=>!a.settled).map(a=>a.stage_activation_id));
 const names=values(run.activations).filter(a=>ids.has(a.id) || ['ready','running','waiting'].includes(a.status)).map(a=>a.stage_id).filter(Boolean);
 return [...new Set(names)];
}
function renderDetailRead(){
 if(!state)return;
 const run=state.run,stages=terminalStatuses.includes(state.run.status)?[]:currentStages(state.run);
 const names=stages.slice(0,2).map(esc).join(', ');
 const stageSummary=stages.length?`<p class="run-current-stages"><b>Стадии</b> ${names}${stages.length>2?` · ещё ${stages.length-2}`:''}</p>`:'';
 preserve($('overview'),`<div class="detail-primary"><div class="observation-heading"><p>${runBadge(run.status)}${run.outcome?' · '+esc(label(run.outcome)):''}</p><small>${selectedRead.stale?'При последнем чтении: ':''}${state.driver_live?'Драйвер активен':'Активность драйвера не подтверждена'}</small></div>${stageSummary}${reasonsHTML(detailReasons(run,state.capacity),source,run.id)}<div class="read-facts muted">${snapshotReadHTML(selectedRead.at,selectedRead.stale)}<span>Наблюдение Run: ${esc(date(run.last_observed?.utc))}</span></div></div>`);
}
function renderDetail() {
 const run=state.run;
 $('delete-run').disabled=maintenanceBusy || !run.settled || !terminalStatuses.includes(run.status);
 $('detail-title').textContent=state.taskTitle;
 $('detail-workflow').textContent='План: '+(parseMaybe(run.workflow)?.title || run.workflow_ref?.id || 'Название не записано');
 $('detail-id').textContent=run.project_id+' · '+run.id;
 $('permalink').href=location.hash;
 renderDetailRead();
 if(tab==='overview'){
  const activity=activityRows(run,state.capacity,null);
  preserve($('activity'),nowHTML({...run,source},activity,state.choices)+observedProgress(state.progress,programsIn(run),selectedRead.stale||detailAux.has('progress')));
  const attempts=values(run.attempts),active=attempts.filter(a=>!a.settled),waiting=active.filter(a=>a.session?.host_state==='awaiting_host' && !a.session.taken);
  preserve($('overview-metadata'),`<details class="facts-section" data-key="run-metadata"><summary data-focus="run-metadata">Попытки и версии</summary><p>Всего: ${attempts.length} · незавершённых: ${active.length} · принятие не записано: ${waiting.length}</p><p>v${esc(state.run_version)} · e${esc(state.event_sequence)}</p></details>${timeHTML(state.timing)}${resourceHTML(run,state.capacity)}${launchHTML(run,state.input_values)}${run.brief_ref?artifact(run.brief_ref,'Задача и критерии завершения'):''}${run.pending_decision?section('Ожидается решение',run.pending_decision,'pending',true):''}${run.stops?.length?section('Причины остановки',run.stops,'stops',true):''}${run.gaps?.length?section('Разрывы наблюдения',run.gaps,'gaps',true):''}`);
 }
 if(tab==='workflow'){
  const invs=values(run.invocations),plans=definitions(run);
  if(definition && !plans.some(plan=>plan.id===definition))definition='';
  if(!definition && !run.invocations?.[invocation])invocation=run.root_workflow_invocation_id || '';
  const options=invs.map(i=>`<option value="${esc(i.id)}">${esc(i.workflow_ref.id)} · ${esc(i.branch_id || '')}${i.iteration?' · итерация '+i.iteration:''} · ${esc(short(i.id))}</option>`).join('')+(!invs.length?'<option value="">Корневой workflow</option>':'')+plans.map(d=>`<option value="def:${esc(d.id)}">План: ${esc(d.title || d.id)}</option>`).join('');
  if($('invocation').innerHTML!==options)$('invocation').innerHTML=options;
  $('invocation').value=definition?'def:'+definition:invocation;
  renderGraph();
  preserve($('tree'),state.timing?.root?treeNode(state.timing.root):'<p>Дерево времени не записано.</p>');
  renderNode();
  const revealKey=JSON.stringify([source,selected,nodeID,invocation]);
  if(revealed!==revealKey){
   const target=[...$('tree').querySelectorAll('[data-node]')].find(n=>n.dataset.node===(nodeID || invocation));
   revealTreePath(target);
   revealed=revealKey;
  }
 }
 if(tab==='facts'){
  const costs=values(run.attempts).flatMap(a=>(a.reported_costs||[]).map(c=>({attempt_id:a.id,...c})));
  preserve($('facts'),decisionDetailsHTML(run,decisionRoute(route()))+section('Выходные артефакты',run.output_artifacts,'outputs',true)+section('Решения',run.decision_ledger,'decisions',true)+section('Проверки',run.check_executions,'checks')+section('Ожидающая приёмка',run.pending_acceptance,'acceptance')+section('Диагностика',run.diagnostics,'diagnostics',true)+section('Отклонения качества',run.waivers,'waivers')+section('Публикации артефактов',run.artifact_publications,'publications')+section('Заявленная стоимость по попыткам и источникам',costs,'costs')+section('Входные артефакты',run.input_artifacts,'inputs')+section('Закреплённые ресурсы контекста',run.context_resources,'resources')+section('Ожидания, сигналы и ограничения',{waits:run.wait_registrations,inbox:run.inbox,guards:run.guards},'control')+raw(run,'run'));
 }
 focusDetailRoute();
}
function navigateNode(id,inv=invocation,plan=definition) {
 const params={tab:'workflow'};
 if(id)params.node=id;
 if(inv)params.invocation=inv;
 if(plan)params.definition=plan;
 if(id)params.focus='1';
 goTo(runHash(source,selected,params));
}
function goTo(hash) {
 if(location.hash===hash){focusRoute=hash;focusDetailRoute();}
 else{
  const url=new URL(location.href);url.hash=hash;
  history.pushState(history.state,'',url);
  useRoute();
 }
}
function revealTreePath(target){
 for(let parent=target?.parentElement;parent && parent!==$('tree');parent=parent.parentElement)if(parent.tagName==='DETAILS')parent.open=true;
}
function focusDetailRoute(){
 if(restoreTreeFocus && state && tab==='workflow' && !nodeID){
  const returnedNode=restoreTreeFocus;
  restoreTreeFocus='';
  const target=[...$('tree').querySelectorAll('[data-node]')].find(el=>el.dataset.node===returnedNode);
  revealTreePath(target);
  (target || $('execution-tree-title')).focus();
 }
 if(!focusRoute || focusRoute!==location.hash || !state)return;
 const h=route();
 if(tab==='journal' && h.has('event') && eventPending)return;
 focusRoute='';
 const exact=tab==='journal' && h.has('event')?$('events').querySelector('[data-seq="'+Number(h.get('event'))+'"] summary'):null;
 const question=tab==='facts' && decisionRoute(h)?$('decision-target'):null;
 const target=exact || question || $(tab==='workflow' && nodeID?'node-title':tab==='overview'?'detail-title':tab+'-title');
 target?.focus();
}
function rememberListPosition(source,id){
 const list=$('run-list'), focus=document.activeElement;
 listScroll=window.scrollY;
 listPositions=new Map([...$('board').querySelectorAll('[data-scroll]')].map(el=>[el.dataset.scroll,[el.scrollLeft,el.scrollTop]]));
 listFocus={
  key:list.contains(focus)?focus.dataset.focus:null,
  href:list.contains(focus)?focus.getAttribute('href'):null,
  run:runKey({source,run_id:id})
 };
}
function restoreListPosition(){
 for(const el of $('board').querySelectorAll('[data-scroll]')){
  const saved=listPositions.get(el.dataset.scroll);
  if(saved){el.scrollLeft=saved[0];el.scrollTop=saved[1];}
 }
 if(!listFocus)return;
 const list=$('run-list');
 const same=[...list.querySelectorAll('[data-focus], a[href]')].find(el=>
  el.getClientRects().length && (listFocus.key?el.dataset.focus===listFocus.key:listFocus.href && el.getAttribute('href')===listFocus.href));
 const row=[...list.querySelectorAll('[data-run-key]')].find(el=>el.dataset.runKey===listFocus.run && el.getClientRects().length);
 (same || row?.querySelector('.run-title-cell a, h3 a') || form.elements.q).focus({preventScroll:true});
 listFocus=null;
}
function useRoute() {
 const h=route(),next=h.get('run')||'',nextSource=next?(h.get('source')||''):'',nextObserve=!next && h.get('observe')==='1';
 const changed=next!==selected || nextSource!==source,screenChanged=changed || nextObserve!==observing;
 if(!selected && !observing && (next || nextObserve))rememberListPosition(nextSource,next);
 if(observing && !nextObserve)for(const pin of pins.values())cancelPinReads(pin);
 observing=nextObserve;
 if(changed){
  selected=next;source=nextSource;state=null;stamp='';selectedRead={at:null,stale:false};detailAux.clear();recentKey='';
  restoreTreeFocus='';
  artifactCache.clear();fileCache.clear();resetEvents('recent');
  delete $('graph').dataset.graphView;
  for(const name of ['overview','activity','overview-metadata','recent-rows','tree','node','graph','facts'])preserve($(name),'');
  $('recent-events').hidden=true;
  $('overview').textContent='Загрузка прогона…';lastMarkup.delete($('overview'));
  $('detail-title').textContent='Загрузка прогона…';$('detail-id').textContent=selected;
  $('detail-error').hidden=true;$('retry-run').hidden=true;
 }
 const node=h.get('node')||'',inv=h.get('invocation')||'',plan=h.get('definition')||'';
 const requested=h.get('tab'),nextTab=runTabs.includes(requested)?requested:(node || inv || plan?'workflow':'overview');
 if(screenChanged || nextTab!==tab || node!==nodeID || inv!==invocation || plan!==definition || location.hash!==currentHash){
  generation++;
  detailFlight?.abort();detailFlight=null;
  for(const flight of detailRequests.values())flight.abort();
  detailRequests.clear();detailAux.clear();
 }
 currentHash=location.hash;
 nodeID=node;invocation=inv;definition=plan;tab=nextTab;
 if(tab==='journal' && (!eventKey || eventKey!==h.get('event'))){
  const seq=Number(h.get('event'));
  if(Number.isSafeInteger(seq) && seq>0)resetEvents('exact',seq);
  else if(eventView==='exact')resetEvents('recent');
 }
 if(tab!=='journal' && eventView==='exact')resetEvents('recent');
 const panes={overview:'overview-pane',facts:'facts-pane',workflow:'workflow',journal:'journal'};
 for(const button of document.querySelectorAll('[data-tab]')){
  const active=button.dataset.tab===tab;button.setAttribute('aria-pressed',String(active));$(panes[button.dataset.tab]).hidden=!active;
 }
 $('workflow').classList.toggle('inspecting',!!nodeID);
 $('run-list').hidden=!!selected || observing;$('detail').hidden=!selected;$('observations').hidden=!observing;
 updatePinControls();
 $('retry-list').hidden=!!selected || observing || !listFailed;
 $('connection').hidden=observing;
 if(selected)renderReadState();
 else $('connection').textContent=listFailed?readStatus(!!listSnapshot,'список'):listSnapshot?'Список прочитан':'Загрузка списка…';
 if(screenChanged){
  window.scrollTo(0,selected || observing?0:listScroll);
  if(observing)$('observations-title').focus({preventScroll:true});
  else if(selected)$('back-to-runs').focus({preventScroll:true});
  else restoreListPosition();
 }
 if(h.get('focus')==='1')focusRoute=location.hash;
 if(state)renderDetail();
 schedule();
}
let currentHash='';
const recentAfter=seq=>Math.max(0,seq-50);
function eventHTML(event,exact=false) {
 return `<details class="event" data-key="event:${esc(event.seq)}" data-seq="${esc(event.seq)}"${exact?' open':''}><summary data-focus="event:${esc(event.seq)}"><small>#${esc(event.seq)} · v${esc(event.run_version)} · ${esc(event.actor)}</small>${esc(event.type)}</summary>${readable(event.data,'event:'+event.seq)}</details>`;
}
function resetEvents(view,seq=0){
 eventView=view;eventKey=seq?String(seq):'';eventCursor=seq?seq-1:0;eventMore=false;eventPending=true;
 $('events').replaceChildren();$('event-status').textContent='';
 $('more-events').hidden=true;
 $('event-recent').setAttribute('aria-pressed',String(view==='recent'));
 $('event-history').setAttribute('aria-pressed',String(view==='history'));
}
async function refreshRecent(g,signal) {
 const seq=state.event_sequence;
 const key=JSON.stringify([source,selected,seq]);
 if(recentKey===key)return;
 $('recent-events').hidden=false;
 const data=await get('/api/events',{source,id:selected,after:recentAfter(seq)},signal);
 if(g!==generation)return;
 const events=data.events.slice(-3).reverse();
 const boundary=data.more || (data.events.at(-1)?.seq || 0)<seq;
 preserve($('recent-rows'),events.map(event=>`<p><a href="${esc(runHash(source,selected,{tab:'journal',event:event.seq,focus:'1'}))}">#${esc(event.seq)} · ${esc(event.type)}</a></p>`).join('')+(boundary?'<p class="muted">Недавние события прочитаны частично.</p>':events.length?'':'<p class="muted">События не записаны.</p>'));
 recentKey=key;
}
async function refreshEvents(g,signal) {
 if(!eventPending && (!$('event-follow').checked || eventMore || eventView==='exact'))return;
 const initial=eventPending && !$('events').childElementCount;
 const after=initial && eventView==='recent'?recentAfter(state.event_sequence):eventCursor;
 if(!eventPending && state.event_sequence<=after)return;
 const mode=eventView,exact=Number(eventKey);
 const data=await get('/api/events',{source,id:selected,after},signal);
 if(g!==generation || mode!==eventView || exact!==Number(eventKey))return;
 if(mode==='exact'){
  const match=data.events.find(e=>e.seq===exact);
  $('events').innerHTML=match?eventHTML(match,true):'';
  $('event-status').textContent=match?`Событие #${exact}`:`Событие #${exact} в этом Run не прочитано.`;
  eventMore=false;
 }else{
  const rows=data.events.filter(e=>e.seq>after);
  $('events').insertAdjacentHTML('beforeend',rows.map(e=>eventHTML(e)).join(''));
  eventCursor=rows.at(-1)?.seq || after;
  eventMore=data.more;
  $('event-status').textContent=eventMore?'Есть следующая страница.':eventCursor<state.event_sequence?'Прочитана часть журнала.':eventCursor?'Достигнут последний прочитанный номер.':'События не записаны.';
  if(rows.length && $('event-follow').checked && !initial)$('events').lastElementChild.scrollIntoView({block:'nearest'});
 }
 eventPending=false;$('more-events').hidden=!eventMore;focusDetailRoute();
}
let listSnapshot='', listRequest='',listFailed=false;
function readStatus(saved,object){
 return saved?`Не удалось обновить ${object}. Показаны устаревшие данные.`:`Не удалось прочитать ${object}.`;
}
function listParams(){
 const params=query();
 params.set('page',page);
 return params;
}
function prepareListRead(params){
 const key=params.toString();
 if(key===listRequest)return;
 listRequest=key;
 listSnapshot='';
 childCache.clear();currentRead=null;$('current').hidden=true;
 for(const id of ['runs','board','totals','matches','page','sources'])preserve($(id),'');
 $('connection').textContent='Загрузка списка…';
 $('retry-list').hidden=true;
 for(const suffix of ['','-top']){$('next'+suffix).disabled=true;$('previous'+suffix).disabled=page<=1;}
}
async function refreshList(g){
 const params=listParams();
 prepareListRead(params);
 const key=listRequest;
 try{
  const data=await get('/api/runs',params);
  if(g!==generation)return;
  let relatedError=false;
  if(isRelatedView(form.elements.view.value)){
   if(!relatedInitialized){for(const run of data.runs.filter(r=>r.children).slice(0,10))expandedRuns.add(runKey(run));relatedInitialized=true;}
   if(expandSearch)for(const run of data.runs)if(run.context && run.children)expandedRuns.add(runKey(run));
   try{await loadRelated(data.runs,params,g,expandSearch);}
   catch{relatedError=true;}
   if(g!==generation)return;
   expandSearch=false;
  }
  renderList(data);
  listSnapshot=key;
  listRequest=listParams().toString();
  if(listRequest!==key)listSnapshot=listRequest;
  const partial=data.sources.some(s=>s.error);
  $('connection').textContent=relatedError?'Список прочитан. Часть связанных Run недоступна.':partial?'Список прочитан. Есть недоступные источники.':'Список прочитан';
  listFailed=relatedError;$('retry-list').hidden=!relatedError;
 }catch{
  if(g!==generation)return;
  $('connection').textContent=readStatus(listSnapshot===key,'список');
  listFailed=true;$('retry-list').hidden=false;
 }
}
const detailRequests=new Map();
const unavailableNames={brief:'название задачи',capacity:'допуск и ресурсы',progress:'ход исполнения',output:'вывод программы',journal:'журнал'};
function renderReadState(){
 $('connection').textContent=selectedRead.stale?'Чтение Run прервано':selectedRead.at?'Run прочитан':'Чтение Run…';
 const error=selectedRead.stale?readStatus(!!state,'прогон'):detailAux.size?'Прогон прочитан. Недоступно: '+[...detailAux].map(name=>unavailableNames[name]).join(', ')+'.':'';
 $('detail-error').hidden=!error;$('detail-error').textContent=error;$('retry-run').hidden=!error;
}
function updateDetail() {
 if(!state)return;
 const version=JSON.stringify([state.taskTitle,state.run_version,state.event_sequence,state.driver_live,state.capacity,state.progress?.attempts,state.progress?.as_of?.utc,state.output?.attempt_id,state.output?.state,(state.output?.streams||[]).map(x=>x.end_offset),selectedRead.stale,detailAux.has('progress')]);
 if(stamp!==version){stamp=version;renderDetail();}
 else renderDetailRead();
 renderReadState();
}
function readDetailPart(name,g,read,apply=()=>{}){
 if(detailRequests.has(name))return;
 const flight=new AbortController();
 detailRequests.set(name,flight);
 const valid=()=>g===generation && !flight.signal.aborted;
 (async()=>{
  try{
   const result=await read(flight.signal);
   if(!valid())return;
   apply(result);detailAux.delete(name);
  }catch{
   if(!valid())return;
   detailAux.add(name);
   if(name==='capacity' || name==='output')delete state[name];
   if(name==='journal' && tab==='overview')preserve($('recent-rows'),'<p class="muted">Недавние события недоступны.</p>');
  }finally{
   if(detailRequests.get(name)===flight)detailRequests.delete(name);
   if(valid())updateDetail();
  }
 })();
}
async function refreshDetail(g){
 if(detailFlight)return;
 const flight=new AbortController();detailFlight=flight;
 const params={source,id:selected};
 let next;
 try{
  next=await get('/api/run',params,flight.signal);
  if(next.run?.id!==params.id)throw new Error('Mismatched Run');
 }catch{
  if(g===generation && !flight.signal.aborted){
   selectedRead.stale=true;
   if(!state){$('detail-title').textContent='Прогон';$('overview').textContent='';lastMarkup.delete($('overview'));}
   updateDetail();renderReadState();
  }
  return;
 }finally{if(detailFlight===flight)detailFlight=null;}
 if(g!==generation || flight.signal.aborted)return;
 const previous=state,run=next.run,programs=programsIn(run);
 const shown=tab==='workflow' && nodeID?receivedAttempt(run,nodeID):null;
 const brief=run.brief_ref,briefKey=brief && params.source+':'+brief.digest;
 const briefArtifact=briefKey && artifactCache.get(briefKey);
 next.taskTitle=runTaskTitle(next.input_values,parseMaybe(briefArtifact?.content));
 next.capacity=previous?.capacity;
 next.progress=matchingProgress(previous?.progress,programs);
 if(shown && previous?.output?.attempt_id===shown.id)next.output=previous.output;
 state=next;selectedRead={at:new Date().toISOString(),stale:false};
 updateDetail();
 if(brief && !briefArtifact)readDetailPart('brief',g,signal=>get('/api/artifact',{source:params.source,...brief},signal),value=>{
  artifactCache.set(briefKey,value);state.taskTitle=runTaskTitle(state.input_values,parseMaybe(value.content));
 });
 if((tab==='overview' || tab==='facts') && (run.stage_resources || !terminalStatuses.includes(run.status)))readDetailPart('capacity',g,signal=>get('/api/capacity',{source:params.source},signal),value=>{state.capacity=value;});
 if(programs.length && (tab==='overview' || (shown && !shown.session && shown.process)))readDetailPart('progress',g,signal=>get('/api/progress',params,signal),value=>{
  state.progress=matchingProgress(value,programsIn(state.run));
 });
 if(shown && !shown.session && shown.process)readDetailPart('output',g,signal=>get('/api/output',{...params,attempt:shown.id},signal),value=>{
  if(receivedAttempt(state.run,nodeID)?.id===shown.id && value.attempt_id===shown.id)state.output=value;
 });
 if(tab==='overview' && recentKey!==JSON.stringify([source,selected,state.event_sequence]))readDetailPart('journal',g,signal=>refreshRecent(g,signal));
 if(tab==='journal')readDetailPart('journal',g,signal=>refreshEvents(g,signal));
}
function schedule(){
 if(observing){refreshPins();return;}
 if(selected){refreshDetail(generation);return;}
 if(readingList)return;
 readingList=true;
 const g=generation,params=listParams();
 Promise.all([refreshList(g),refreshCurrent(params,g)]).finally(()=>{
  readingList=false;
  if(g!==generation && !selected && !observing)schedule();
 });
}
$('retry-list').onclick=schedule;$('retry-run').onclick=schedule;
form.onsubmit=e=>e.preventDefault();
form.oninput=()=>changeCatalogue(true);
$('catalog-views').addEventListener('click',event=>{
 const control=event.target.closest('[data-view]');
 if(control)setCatalogueView(control.dataset.view);
});
$('current-state').onchange=()=>setCatalogueView('current');
$('runs').addEventListener('click',event=>{
 const expand=event.target.closest('[data-expand]');
 if(expand){const key=expand.dataset.expand;if(expandedRuns.has(key))expandedRuns.delete(key);else expandedRuns.add(key);schedule();}
 const more=event.target.closest('[data-more]');
 if(more){childPages.set(more.dataset.more,(childPages.get(more.dataset.more)||1)+1);schedule();}
});
function turnPage(delta){
 page+=delta;generation++;syncCatalogueAddress();prepareListRead(listParams());schedule();
}
for(const suffix of ['','-top']){
 $('previous'+suffix).onclick=()=>turnPage(-1);
 $('next'+suffix).onclick=()=>turnPage(1);
}
$('invocation').onchange=e=>{
 const value=e.target.value;
 navigateNode('',value.startsWith('def:')?'':value,value.startsWith('def:')?value.slice(4):'');
};
$('pin-run').onclick=()=>togglePin(source,selected,state?.taskTitle||'');
$('back-to-tree').onclick=()=>{restoreTreeFocus=nodeID;navigateNode('');};
document.addEventListener('click',event=>{
 const pin=event.target.closest('[data-pin]');
 if(pin){togglePin(pin.dataset.source,pin.dataset.run,pin.dataset.title);return;}
 const link=event.target.closest('a[href^="#"]');
 if(!link || event.defaultPrevented || event.button!==0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)return;
 event.preventDefault();
 goTo(link.hash);
});
$('detail').addEventListener('click',e=>{
 const target=e.target.closest('[data-node]');
 if(target && state){
  const obj=nodeObject(state.run,target.dataset.node);
  navigateNode(target.dataset.node,(state.run.invocations?.[obj?.id]?obj.id:obj?.workflow_invocation_id||state.run.activations?.[obj?.stage_activation_id]?.workflow_invocation_id||invocation));
 }
 if(e.target.closest('#load-files'))loadFiles();
});
function openTab(name){
 const h=route();h.set('tab',name);h.delete('focus');h.delete('event');
 for(const key of ['decision','decision_attempt','decision_digest'])h.delete(key);
 goTo('#'+h);
}
for(const button of document.querySelectorAll('[data-tab]'))button.onclick=()=>openTab(button.dataset.tab);
$('open-journal').onclick=()=>openTab('journal');
for(const mode of ['recent','history'])$('event-'+mode).onclick=()=>{
 resetEvents(mode);
 const h=route();
 if(h.has('event')){h.delete('event');h.delete('focus');goTo('#'+h);}
 else schedule();
};
$('event-follow').onchange=schedule;
async function cleanStorage(targetSource,run='') {
 if(maintenanceBusy)return;maintenanceBusy=true;$('delete-run').disabled=true;
 const status=$('maintenance-status');status.textContent='Подготовка очистки: проверяем Run и ссылки на артефакты…';
 try {
  const {token}=await get('/api/maintenance-token');
  const request=async(action,digest='')=>{const response=await fetch(endpoint('/api/maintenance'),{method:'POST',headers:{'Content-Type':'application/json','X-PriFly-Maintenance':token},body:JSON.stringify({source:targetSource,run,action,digest}),signal:AbortSignal.timeout(120000)});const body=await response.json();if(!response.ok)throw new Error(({storage_busy:'Хранилище используется другим процессом. Дождитесь завершения операции и повторите.',run_not_deletable:'Run не завершён окончательно или защищён зависимостями.',cleanup_plan_changed:'Данные изменились после предпросмотра. Откройте очистку заново.'})[body.code]||body.message||body.detail||JSON.stringify(body));return body;};
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
$('more-events').onclick=()=>{eventPending=true;schedule();};
restorePins();
history.scrollRestoration='manual';
restoreCatalogueAddress();
renderCatalogueControls();
window.addEventListener('hashchange',useRoute);
window.addEventListener('popstate',()=>{restoreCatalogueAddress();prepareListRead(listParams());useRoute();});
useRoute();
setInterval(schedule,1500);
function readVisibleStorage(){
 if(!observing && !selected && $('storage').closest('.list-metadata').open)refreshStorage();
}
document.addEventListener('toggle',event=>{
 if(event.target===$('storage') || event.target.classList?.contains('list-metadata'))readVisibleStorage();
},true);
setInterval(readVisibleStorage,5000);
}
