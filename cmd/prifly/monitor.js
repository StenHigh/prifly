'use strict';
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const labels = {running:'В работе',completed:'Завершён',failed:'Ошибка',cancelled:'Отменён',uncertain:'Неопределённость',waiting:'Ожидание',pending:'Ожидает',ready:'Готов',succeeded:'Выполнен',rejected:'Отклонён',stopping:'Останавливается',awaiting_host:'Ожидает агента',reported:'Результат получен',disconnected:'Связь потеряна',settled:'Завершено',future:'Ещё не выполнен',unchosen:'Ветвь не выбрана',not_taken:'Не выполнен в этом исполнении',workflow_invocation:'WorkflowInvocation',stage_activation:'StageActivation',step_instance:'StepInstance',attempt:'Attempt',run:'Run',check_execution:'Проверка',step:'Шаг',choice:'Развилка',parallel:'Параллельные ветви',call:'Вложенный workflow',repeat:'Повтор',map:'Обработка элементов',wait:'Ожидание события',finish:'Завершение',elapsed:'Общее время',executor:'Работа исполнителя',queue:'Ожидание допуска',active:'Активное время',quality:'Качество измерения',measured:'Измерено',estimated:'Оценка',partial:'Частично',unknown:'Неизвестно',not_applicable:'Не применимо',unavailable:'Недоступно',incomparable_clock_domains:'Разные сессии часов',calendar_suspend_coverage_unqualified:'Сон машины не учтён достоверно',authority_wall_estimate:'Оценка по календарным часам authority',executor_time:'Работа исполнителя',executor_sum:'Сумма времени исполнителей',executor_active_union:'Интервалы активности исполнителей',check_executor_sum:'Сумма времени проверок',check_executor_active_union:'Интервалы активности проверок',ready_queue:'Ожидание в очереди',restricted_time:'Время ограничений',dispatch_latency:'Задержка передачи',result_to_acceptance:'От результата до приёмки',post_execution_settlement:'Завершение после исполнения',output_contracts:'Требования к результату',subject:'Задача',desired_outcome:'Ожидаемый результат',in_scope:'Входит в задачу',out_of_scope:'За пределами задачи',completion_criteria:'Критерии завершения',source_refs:'Источники',assumptions:'Предположения',confirmation:'Подтверждение',summary:'Описание результата',verdict:'Вердикт',outputs:'Выходы',inputs:'Входы',instructions:'Инструкции',context:'Контекст',description:'Описание',status:'Состояние',reason:'Причина',message:'Сообщение',code:'Код',severity:'Важность',phase:'Фаза',observed:'Зафиксировано',actor:'Автор',actor_id:'Автор',amount:'Сумма',currency:'Валюта',source:'Источник',reported_costs:'Заявленная стоимость',permitted_effects:'Разрешённые действия',workspace:'Рабочая папка',skill_refs:'Навыки',host_state:'Состояние агента',deadline:'Срок',process_outcome:'Итог процесса',error:'Ошибка',evidence_refs:'Свидетельства',effect_receipt_refs:'Подтверждения эффектов'};
const label = s => labels[s] || s;
const badge = s => `<span class="badge ${/^[a-z_]+$/.test(s || '') ? s : ''}">${esc(label(s) || 'Не записано')}</span>`;
const parseMaybe = value => { if(typeof value !== 'string') return value; try{return JSON.parse(value);}catch{return value;} };
const values = o => Object.values(o || {}).filter(Boolean);
const short = s => String(s || '').replace(/^[a-z_]+:/,'').slice(0,12);
const ms = n => n >= 60000 ? `${(n/60000).toFixed(1)} мин` : n >= 1000 ? `${(n/1000).toFixed(1)} с` : `${n} мс`;
function duration(d,withReasons=false) {
 if(!d) return 'Не записано';
 const text = d.value_ms != null ? ms(d.value_ms) : d.estimate_ms != null ? `около ${ms(d.estimate_ms)}` : d.known_ms != null ? `не менее ${ms(d.known_ms)}` : label(d.quality || 'unknown');
 return `${text}${d.is_open ? ' · интервал открыт' : ''}${d.quality && d.quality !== 'measured' && (d.value_ms!=null || d.known_ms!=null || d.estimate_ms!=null) ? ' · '+label(d.quality) : ''}${withReasons && d.reasons?.length ? ' · '+d.reasons.map(label).join(', ') : ''}`;
}
const date = s => s ? new Date(s).toLocaleString('ru-RU') : 'Не записано';
function graphData(workflow, run, invocation, choices=[]) {
 const stages = workflow?.definition?.stages || {}, nodes = [], edges = [];
 const inv = run.invocations?.[invocation];
 const activations = values(run.activations).filter(a => (a.workflow_invocation_id || '') === (invocation || ''));
 for(const [id,stage] of Object.entries(stages)) {
  const actual = activations.filter(a=>a.stage_id===id);
  const state = actual.at(-1)?.status || (invocation==='__definition__' ? 'future' : ((inv?.ready_stages || run.ready_stages || []).includes(id) ? 'ready' : (inv?.settled || (!inv && run.settled)) ? 'not_taken' : 'future'));
  nodes.push({id,title:id,kind:stage.kind,state,actual,stage});
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
 for(const edge of edges) {const decision=decisions.get(edge.from);if(decision)edge.unchosen=edge.to!==decision.next_stage_id || edge.label!==(decision.route==='branch'?decision.branch_id:decision.route);}
 const reachable=filtered=>{const found=new Set([workflow?.definition?.entry]),pending=[workflow?.definition?.entry];for(let i=0;i<pending.length;i++)for(const e of edges)if(e.from===pending[i] && (!filtered || !e.unchosen) && !found.has(e.to)){found.add(e.to);pending.push(e.to);}return found;};
 const all=reachable(false),chosen=reachable(true);
 for(const n of nodes)if(['future','not_taken'].includes(n.state) && all.has(n.id) && !chosen.has(n.id))n.state='unchosen';
 // ponytail: a breadth-first layout handles cycles without a graph dependency; dense graphs remain scrollable.
 const rank = new Map(), queue = [];
 if(stages[workflow?.definition?.entry]) {rank.set(workflow.definition.entry,0);queue.push(workflow.definition.entry);}
 for(let i=0;i<queue.length;i++) for(const edge of edges.filter(e=>e.from===queue[i])) if(!rank.has(edge.to)){rank.set(edge.to,rank.get(edge.from)+1);queue.push(edge.to);}
 const counts = new Map();let maxRank=0,maxRow=0;
 for(const node of nodes) {const column=rank.get(node.id) ?? 0,row=counts.get(column)||0;counts.set(column,row+1);node.x=30+column*360;node.y=35+row*150;maxRank=Math.max(maxRank,column);maxRow=Math.max(maxRow,row);}
 return {nodes,edges,width:Math.max(600,(maxRank+1)*360+50),height:Math.max(180,(maxRow+1)*150+40)};
}
function fileChanges(before,after) {
 if(!before || !after) return null;
 const left = new Map(before.files.map(f=>[f.path,f.ref?.digest])), right = new Map(after.files.map(f=>[f.path,f.ref?.digest]));
 return [...new Set([...left.keys(),...right.keys()])].sort().flatMap(path => !left.has(path) ? [{path,change:'Добавлен'}] : !right.has(path) ? [{path,change:'Удалён'}] : left.get(path)!==right.get(path) ? [{path,change:'Изменён'}] : []);
}
if(typeof module !== 'undefined') module.exports = {esc,duration,graphData,fileChanges};
if(typeof document !== 'undefined') {
const $ = id => document.getElementById(id);
let selected='',source='',generation=0,page=1,pages=1,state=null,nodeID='',invocation='',definition='',tab='workflow',stamp='',eventsCursor=0,eventsMore=false,busy=false,queued=false,filterTimer,revealed='';
let listScroll=0,maintenanceBusy=false,storageBusy=false;
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
 preserve($('storage-details'),`<p class="muted">Выделенное место по данным файловой системы. Общие inode учтены один раз; APFS clones и snapshots могут разделять блоки. Внешние рабочие репозитории не включены. ${data.scanning?'Измерение продолжается.':''} ${data.measured?'Измерено '+esc(date(data.measured)):''}</p>${(data.sources||[]).map(s=>`<div class="storage-source"><b>${esc(s.root)}</b><p>${s.errors.length?'Учтено (неполно)':'Занято'}: ${sizeText(s.measured?s.bytes:null)} · артефакты: ${sizeText(s.measured?s.artifacts:null)} · доступно на диске: ${sizeText(s.available)}</p>${s.errors.length?'':`<button type="button" data-clean-source="${esc(s.id)}">Очистить завершённые Run…</button>`}${s.errors.map(e=>`<p class="error">${esc(e)}</p>`).join('')}</div>`).join('')}`);
 }catch(err){$('storage-title').textContent='Размер хранилищ недоступен: '+err.message;}finally{storageBusy=false;}
}
function renderList(data) {
 pages=data.pages;page=data.page;
 $('totals').innerHTML=`<b>${data.total}</b> прогонов <b>${data.active}</b> с активными попытками`;
 const d=data.discovery,errors=data.sources.filter(s=>s.error),pending=data.sources.filter(s=>!s.indexed && !s.error).length;
 $('coverage-title').textContent=`${data.sources.length} хранилищ · ${d.scanning?'поиск продолжается':'обход завершён'} · ${d.directories} каталогов${d.unreadable?' · недоступно: '+d.unreadable:''}${errors.length?' · ошибок чтения: '+errors.length:''}${pending?' · индексируется: '+pending:''}`;
 preserve($('sources'),`<p class="muted">Область поиска: ${esc(d.roots.join(', '))}. Исключены системные/сетевые каталоги: ${d.excluded}. Символические ссылки при обходе не раскрываются; физические каталоги читаются по исходному пути. Полнота относится к доступной проверенной области.</p>${d.completed?`<p>Последний обход: ${esc(date(d.completed))}</p>`:''}${data.sources.map(s=>`<div class="source"><b>${esc(s.project || 'Хранилище')}</b><small>${esc(s.root)}</small>${s.conflict?'<p class="error">Конфликт: одинаковая идентичность у нескольких хранилищ. Их Run показаны отдельно.</p>':''}<p class="${s.error?'error':'muted'}">${esc(s.error || (s.indexed?'Прочитано '+date(s.updated):'Индексируется…'))}</p></div>`).join('')}${d.errors.length?section('Недоступные пути (примеры)',d.errors,'scan-errors'):''}`);
 options('project',data.filters.projects,'Все проекты');options('status',data.filters.statuses,'Все статусы');options('executor',data.filters.executors,'Все исполнители');
 preserve($('runs'),data.runs.length?data.runs.map(run=>{
  const hash=new URLSearchParams({source:run.source,run:run.run_id});
  return `<tr class="${run.source===source && run.run_id===selected?'selected':''}"><td><a data-focus="${esc(run.source+run.run_id)}" href="#${esc(hash)}">${esc(run.subject || run.run_id)}</a><small>${esc(run.run_id)}</small>${run.error?`<p class="error">${esc(run.error)}</p>`:''}</td><td>${esc(run.project)}<small>${esc(run.workflow_id)}</small><small>${esc(run.root)}</small></td><td>${badge(run.status==='running' && run.active_attempts>0 && run.active_attempts===run.awaiting_hosts?'awaiting_host':run.status)}${run.outcome?`<small>Outcome: ${esc(run.outcome)}</small>`:''}</td><td>${esc(run.step_instances ?? '—')} / ${esc(run.attempts ?? '—')}<small>${run.settled_attempts ?? '—'} попыток завершено</small></td><td>${esc(date(run.created))}<small>v${esc(run.run_version)} · e${esc(run.event_sequence)}</small></td></tr>`;
 }).join(''):'<tr><td colspan="5" class="empty">Прогоны не найдены в прочитанной области. Измените фильтры или проверьте состояние поиска выше.</td></tr>');
 $('matches').textContent=`Найдено: ${data.filtered}`;$('page').textContent=`${page} / ${pages}`;$('previous').disabled=page<=1;$('next').disabled=page>=pages;
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
 const data=graphData(workflow,run,definition?'__definition__':invocation,state.choices),byID=new Map(data.nodes.map(n=>[n.id,n]));
 const svg=`<svg width="${data.width}" height="${data.height}" viewBox="0 0 ${data.width} ${data.height}" role="group" aria-label="Связи закреплённых стадий workflow"><defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z"/></marker></defs>${data.edges.map(e=>{const a=byID.get(e.from),b=byID.get(e.to);if(!a||!b)return '';const x=a.x+210,y=a.y+35,tx=b.x,ty=b.y+35;return `<path class="${e.unchosen?'unchosen':''}" d="M ${x} ${y} C ${x+35} ${y+20}, ${tx-35} ${ty+20}, ${tx} ${ty}" marker-end="url(#arrow)"/><text class="edge-label" x="${(x+tx)/2}" y="${(y+ty)/2+(ty>y?14:-10)}" text-anchor="middle">${esc(e.label)}</text>`;}).join('')}${data.nodes.map(n=>`<g tabindex="0" role="button" aria-label="${esc(n.title+', '+label(n.state))}" data-stage="${esc(n.id)}" class="${['future','unchosen'].includes(n.state)?'future':''} ${(n.id===nodeID || n.actual?.some(a=>a.id===selectedActivation))?'current':''}"><rect x="${n.x}" y="${n.y}" width="210" height="78" rx="9"/><text x="${n.x+12}" y="${n.y+22}">${esc(n.title.length>26?n.title.slice(0,24)+'…':n.title)}</text><text x="${n.x+12}" y="${n.y+43}">${esc(label(n.kind))}</text><text x="${n.x+12}" y="${n.y+64}">${esc(label(n.state))}</text></g>`).join('')}</svg>`;
 const left=$('graph').scrollLeft,top=$('graph').scrollTop;$('graph').innerHTML=svg;$('graph').scrollLeft=left;$('graph').scrollTop=top;
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
 let html='';
 if(!object) {
  const stage=parseMaybe(currentWorkflow(run))?.definition?.stages?.[nodeID];
  $('node-title').textContent=stage ? nodeID+' · '+label(stage.kind) : 'Выберите шаг или попытку';
  if(stage){html='<p class="muted">Закреплённое определение стадии.</p>'+readable(stage,nodeID);const actual=values(run.activations).filter(a=>a.stage_id===nodeID && (a.workflow_invocation_id || '')===invocation);html+=actual.map(a=>`<p><button data-node="${esc(a.id)}">Открыть исполнение ${esc(short(a.id))}</button></p>`).join('');}
  preserve($('node'),html);return;
 }
 $('node-title').textContent=(timing?.stage_id || label(timing?.kind) || 'Исполнение')+' · '+short(nodeID);
 html=`<p>${badge(object.status)} ${esc(object.verdict || object.outcome || '')}</p>`;
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
 const active=values(run.attempts).filter(a=>!a.settled),waiting=active.filter(a=>a.session?.host_state==='awaiting_host');
 preserve($('overview'),`<div class="cards"><div class="card"><small>Состояние исполнения</small><strong>${badge(run.status)}</strong>${run.outcome?`<p>Outcome: ${esc(run.outcome)}</p>`:''}</div><div class="card"><small>Общее время · по данным Pri-Fly</small><strong>${esc(duration(state.timing?.root?.metrics?.elapsed))}</strong></div><div class="card"><small>Попытки / ожидают агента</small><strong>${values(run.attempts).length} / ${waiting.length}</strong><small>${active.length} незавершённых</small></div><div class="card"><small>Чтение</small><strong>v${state.run_version} · e${state.event_sequence}</strong><small>${state.driver_live?'Драйвер активен':'Активность драйвера не подтверждена'} · ${esc(date(run.last_observed?.utc))}</small></div></div>${run.brief_ref?artifact(run.brief_ref,'Задача и критерии завершения'):''}${run.pending_decision?section('Ожидается решение',run.pending_decision,'pending',true):''}${run.stops?.length?section('Причины остановки',run.stops,'stops',true):''}${run.gaps?.length?section('Разрывы наблюдения',run.gaps,'gaps',true):''}`);
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
form.onsubmit=e=>e.preventDefault();form.oninput=()=>{clearTimeout(filterTimer);filterTimer=setTimeout(()=>{generation++;page=1;schedule();},200);};
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
