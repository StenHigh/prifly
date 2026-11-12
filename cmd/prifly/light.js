'use strict';
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const blockerPriority=['disconnected','unresolved','cancelling','stop','restricted','guarded','resume_required','branch_cancel','branch_resume','decision','resource_busy','admission','signal','attempt_deadline_expired','review_gate_blocked'];

function lightReasonRun(card,reason){
 if(!reason.attempt_id && !reason.stage_activation_id)return null;
 const candidates=(card.current_runs || []).filter(run=>{
  const matches=other=>
   other.code===reason.code && other.attempt_id===reason.attempt_id &&
   other.stage_activation_id===reason.stage_activation_id;
  return run.board?.reasons?.some(matches) || run.board?.stages?.some(stage=>
   (!reason.stage_activation_id || stage.stage_activation_id===reason.stage_activation_id) &&
   (!reason.attempt_id || stage.attempts?.some(attempt=>attempt.attempt_id===reason.attempt_id)));
 });
 if(candidates.length===1)return candidates[0].run_id;
 const [only]=card.current_runs || [];
 const knownStage=card.active_branches?.some(stage=>
  (!reason.stage_activation_id || stage.stage_activation_id===reason.stage_activation_id) &&
  (!reason.attempt_id || stage.attempts?.some(attempt=>attempt.attempt_id===reason.attempt_id)));
 return candidates.length===0 && card.current_count===1 && knownStage?only?.run_id || null:null;
}
function lightExplanation(card,detailRoute,fullLink,inDetails=false){
 const rank=reason=>{
  const index=blockerPriority.indexOf(reason.code);
  return index<0?blockerPriority.length:index;
 };
 const ordered=(card.reasons || []).filter(reason=>reason.code!=='terminal').sort((a,b)=>rank(a)-rank(b));
 const [primary,...other]=ordered;
 const fallback=card.status==='completed'
  ? card.outcome==='partial'?'Записан частичный результат':'Исполнение завершено'
  : card.status==='cancelled'?'Отмена завершена':'Причина текущего состояния не записана';
 const key=card.source+':'+card.key;
 const show=(reason,index=0)=>{
  const run=lightReasonRun(card,reason);
  const tab=['workflow','facts','journal'].includes(reason.action)?reason.action:'facts';
  const text=reason.code==='attempt_deadline_expired'?'Истёк срок попытки':reason.text || fallback;
  const code=reason.code==='attempt_deadline_expired'?' <code>attempt_deadline_expired</code>':'';
  const stage=card.active_branches?.find(stage=>reason.stage_activation_id && stage.stage_activation_id===reason.stage_activation_id);
  const scope=stage?[...(stage.branches || []),stage.stage_id].filter(Boolean).join(' / '):reason.stage_activation_id || reason.attempt_id;
  const focus=`data-focus="${esc(key+':reason:'+index)}"`;
  const next=run
   ? `<a ${focus} href="${esc(fullLink(card.source,run,tab))}" target="_blank" rel="noopener">Открыть Run ↗</a>`
   : inDetails?`<a ${focus} href="#light-run-history" data-task-history>Открыть историю</a>`
    : `<a ${focus} href="${esc(detailRoute(card.source,card.key))}">Открыть задачу</a>`;
  const reasonText=`${esc(text)}${code}${scope?` <small>${esc(scope)}</small>`:''}`;
  const characters=Array.from(text);
  const body=characters.length>180
   ? `<details data-preserve="${esc(key+':text:'+index)}"><summary>${esc(characters.slice(0,175).join(''))}…</summary><p class="reason">${reasonText}</p></details>`
   : `<p class="reason">${reasonText}</p>`;
  return body+next;
 };
 return `<div class="run-explanation">${show(primary || {})}${other.length?`<details data-preserve="${esc(key+':reasons')}"><summary data-focus="${esc(key+':reasons')}">Другие причины · ${other.length}</summary>${other.map((reason,i)=>show(reason,i+1)).join('')}</details>`:''}</div>`;
}
function readFailure(saved){
 return saved?'Обновление недоступно. Показаны устаревшие данные.':'Не удалось прочитать данные.';
}
function observedLightRun(data,pin){
 if(data.source?.source!==pin.source || data.card?.key!==pin.key)return null;
 return (data.runs || []).find(run=>run.run_id===pin.run) ||
  (data.card.current_runs || []).find(run=>run.run_id===pin.run) || null;
}
function lightRunReason(run){
 const rank=reason=>{
  const index=blockerPriority.indexOf(reason.code);
  return index<0?blockerPriority.length:index;
 };
 return (run.board?.reasons || []).filter(reason=>reason.code!=='terminal').sort((a,b)=>rank(a)-rank(b))[0];
}
function lightRunTarget(run,reason){
 const tab=['workflow','facts','journal'].includes(reason?.action)?reason.action:'overview';
 const node=reason?.attempt_id || reason?.stage_activation_id;
 if(node && (tab==='workflow' || reason.code==='attempt_deadline_expired')){
  const stage=run.board?.stages?.find(stage=>(reason.stage_activation_id && reason.stage_activation_id===stage.stage_activation_id) ||
   (reason.attempt_id && stage.attempts?.some(attempt=>attempt.attempt_id===reason.attempt_id)));
  return {tab:'workflow',node,invocation:stage?.workflow_invocation_id,label:reason.attempt_id?'Открыть попытку в Full':'Открыть стадию в Full'};
 }
 const labels={workflow:'Схема и шаги в Full',facts:'Результаты и решения в Full',journal:'Журнал в Full',overview:'Обзор Run в Full'};
 return {tab,label:labels[tab]};
}
if(typeof module!=='undefined')module.exports={lightExplanation,lightReasonRun,readFailure,observedLightRun,lightRunReason,lightRunTarget};
if(typeof document!=='undefined')(() => {
 const $=id=>document.getElementById(id), form=$('filters');
 const lanes=[['attention','Нужно участие'],['queue','Очередь'],['plan','План'],['implementation','Реализация'],['review','Ревью'],['work','В работе'],['result','Результат получен'],['done','Готово']];
 const states={passed:'Пройдено',defect:'Есть дефект',unavailable:'Приёмка заблокирована',waiting:'Ожидает ревью',disabled:'Выключено',not_applicable:'Не применяется',unknown:'Нет данных',warnings:'Есть замечания'};
 const statuses={ready:'Подготовлено к выдаче',pending:'Ожидает запуска',running:'Выполняется',completed:'Завершён',failed:'Ошибка',cancelled:'Отменён',uncertain:'Неопределённый исход',mixed:'Несколько текущих Runs',claimed:'Задача захвачена',done:'Завершено локально',partial:'Частичный результат',succeeded:'Успешно'};
 let fullURL='http://127.0.0.1:7777/', cached=null, boardRead=null, taskRead=null, queryEpoch=0, controller=null, timer=null, busy=false, failures=0, cursorStack=[''], cursorIndex=0, currentDetail=null, detailRunsCursor='', selectedFocus='';
 const pins=new Map(), pinReads=new Map(), pinFlights=new Map();
 const pinStorage='prifly.light.observation/1';
 let observing=false,observeReturn=null,restorePosition=null;
 const pinKey=(source,run)=>JSON.stringify([source,run]);
 const route=()=>new URLSearchParams(location.hash.slice(1));
 try{
  const saved=JSON.parse(sessionStorage.getItem(pinStorage) || '{}');
  for(const pin of (Array.isArray(saved.pins)?saved.pins:[]).slice(0,2)){
   if(pin && ['source','key','run'].every(name=>typeof pin[name]==='string' && pin[name]))pins.set(pinKey(pin.source,pin.run),pin);
  }
  if(typeof saved.origin?.hash==='string' && !new URLSearchParams(saved.origin.hash.slice(1)).has('observe'))observeReturn=saved.origin;
 }catch{}
 const query=()=>{const q=new URLSearchParams(new FormData(form));for(const [k,v]of [...q])if(!v)q.delete(k);const c=cursorStack[cursorIndex];if(c)q.set('cursor',c);return q;};
 const detailRoute=(source,key)=>'#'+new URLSearchParams({source,key});
 function fullLink(source,run,tab='facts',extra={}){
  const url=new URL(fullURL);url.hash=new URLSearchParams({source,run,tab,...extra}).toString();return url.href;
 }
 function pinButton(source,key,run,showID=false){
  const id=pinKey(source,run),pinned=pins.has(id);
  return `<button type="button" data-light-pin data-source="${esc(source)}" data-task-key="${esc(key)}" data-run="${esc(run)}" data-focus="${esc('pin:'+id)}" data-show-id="${showID?'yes':'no'}" aria-pressed="${pinned}" aria-label="${pinned?'Снять':'Закрепить'} Run ${esc(run)}">${pinned?'Снять':'Закрепить'}${showID?' '+esc(run.slice(4,12)):''}</button>`;
 }
 function cardPins(card){
  const runs=(card.current_runs || []).filter(run=>run.run_id);
  return runs.length?`<div class="light-pin-actions">${runs.map(run=>pinButton(card.source,card.key,run.run_id,runs.length>1)).join('')}</div>`:'';
 }
 function saveObservation(){
  try{sessionStorage.setItem(pinStorage,JSON.stringify({pins:[...pins.values()],origin:observeReturn}));}catch{}
 }
 function updatePinControls(){
  $('light-pin-count').textContent=String(pins.size);
  for(const button of document.querySelectorAll('[data-light-pin]')){
   const pinned=pins.has(pinKey(button.dataset.source,button.dataset.run)),label=pinned?'Снять':'Закрепить';
   button.setAttribute('aria-pressed',String(pinned));
   button.setAttribute('aria-label',label+' Run '+button.dataset.run);
   button.textContent=label+(button.dataset.showId==='yes'?' '+button.dataset.run.slice(4,12):'');
  }
 }
 function togglePin(button){
  const {source,taskKey:key,run}=button.dataset,id=pinKey(source,run);
  if(pins.has(id)){
   pins.delete(id);pinReads.delete(id);
  }else if(pins.size>=2){
   $('light-pin-message').textContent='Закреплено два Run. Снимите один, чтобы выбрать другой.';
   return;
  }else pins.set(id,{source,key,run});
  $('light-pin-message').textContent='';
  saveObservation();
  if(observing){
   renderObservation();
   $('observe-return').focus({preventScroll:true});
   restart();
  }else updatePinControls();
 }
 function observationRow(id,pin){
  const read=pinReads.get(id),run=read?.run;
  const reason=run && lightRunReason(run),stage=run?.board?.stages?.find(stage=>['running','waiting','ready'].includes(stage.status));
  const title=read?.title || 'Выбранный Run';
  const sameTitle=run && [...pinReads].some(([other,value])=>other!==id && value.run && value.title===title);
  const identity=sameTitle?`<small title="${esc(pin.run+' · '+pin.source)}">${esc(pin.run.slice(4,12))} · ${esc(pin.source.slice(0,8))}</small>`:'';
  const status=run?(statuses[run.status] || run.status)+(run.outcome?' · '+(statuses[run.outcome] || run.outcome):''):'';
  const text=reason?.code==='attempt_deadline_expired'?'Истёк срок попытки':reason?.text || stage?.stage_id ||
   run?.phases?.[0]?.label || (run?'Причина или стадия не записана':'');
  const messages={stale:'Последние данные устарели',missing:'Run отсутствует в прочитанной странице',denied:'Доступ к источнику запрещён',unavailable:'Чтение недоступно'};
  const message=messages[read?.state] || (read?'':'Читаем Run…');
  const target=run && lightRunTarget(run,reason);
  const link=target?fullLink(pin.source,pin.run,target.tab,target.node?{node:target.node,focus:'1',...(target.invocation?{invocation:target.invocation}:{})}:{}):'';
  const at=read?.at?new Date(read.at).toLocaleTimeString('ru-RU'):'';
  const observed=run?.last_observed?new Date(run.last_observed).toLocaleString('ru-RU'):'';
  return `<article class="observed-run" data-observed-run="${esc(pin.run)}" data-observed-source="${esc(pin.source)}">
   <div><h3>${esc(title)}</h3>${identity}${status?`<p class="run-status">${esc(status)}</p>`:''}</div>
   <div>${text?`<p>${esc(text)}</p>`:''}${message?`<p class="observation-read">${esc(message)}</p>`:''}
   ${read?.compact?'<small>Краткая запись. Подробности в Full.</small>':''}
   ${at?`<small>Чтение источника · ${esc(at)}</small>`:''}${observed?`<small>Наблюдение Run · ${esc(observed)}</small>`:''}</div>
   <div class="light-pin-actions">${link?`<a data-focus="${esc('open:'+id)}" href="${esc(link)}" target="_blank" rel="noopener">${esc(target.label)} ↗</a>`:''}
   ${read?.state==='missing'?`<a data-focus="${esc('task:'+id)}" href="${esc(detailRoute(pin.source,pin.key))}">Открыть историю задачи</a>`:''}
   ${pinButton(pin.source,pin.key,pin.run)}</div>
  </article>`;
 }
 function renderObservation(){
  preserveRender($('observed-runs'),pins.size?[...pins].map(([id,pin])=>observationRow(id,pin)).join(''):
   '<p class="empty">Закрепите Run на доске или в истории задачи.</p>');
  updatePinControls();
 }
 async function readObservationGroup(group,epoch,signal){
  const {source,key}=group[0][1];
  const current=()=>observing && epoch===queryEpoch;
  try{
   const {body}=await request('api/light/task',{source,key},signal);
   if(!current() || signal.aborted)return;
   if(body.source?.source!==source || body.card?.key!==key)throw new Error('Ответ для другой задачи');
   fullURL=body.full_monitor_url;
   for(const [id,pin] of group){
    const run=observedLightRun(body,pin),previous=pinReads.get(id);
    pinReads.set(id,run?{run,title:body.card.title,at:Date.now(),compact:!body.runs?.includes(run),state:body.source.freshness==='fresh'?'fresh':'stale'}:
     {...previous,state:'missing'});
   }
  }catch(error){
   if(!current())return;
   for(const [id] of group){
    const previous=pinReads.get(id);
    pinReads.set(id,error.status===403?{state:'denied'}:{...previous,state:previous?.run?'stale':'unavailable'});
   }
  }
  if(current())renderObservation();
 }
 function refreshObservation(){
  const groups=new Map(),epoch=queryEpoch;
  for(const [id,pin] of pins){
   const key=JSON.stringify([pin.source,pin.key]);
   if(!groups.has(key))groups.set(key,[]);
   groups.get(key).push([id,pin]);
  }
  for(const [key,group] of groups){
   if(pinFlights.has(key))continue;
   const flight=new AbortController();
   pinFlights.set(key,flight);
   const timeout=setTimeout(()=>flight.abort(),15000);
   void readObservationGroup(group,epoch,flight.signal).finally(()=>{
    clearTimeout(timeout);
    if(pinFlights.get(key)===flight)pinFlights.delete(key);
   });
  }
 }
 function chip(r){const freshness=r.freshness==='stale'||r.freshness==='invalid'?' · нужна новая проверка':'';return `<span class="chip ${esc(r.state)}" title="${esc(r.reason)}">${esc(r.lens)}: ${esc(states[r.state]||r.state)}${freshness}</span>`;}
 function cardHTML(c,sources){const source=sources.find(x=>x.source===c.source),stale=source&&source.freshness!=='fresh';return `<article class="light-card${stale?' stale':''}" data-key="${esc(c.key)}"><p class="project">${esc(c.project_title||c.project_id)}</p><h4><a class="card-title" data-card="${esc(c.key)}" data-focus="${esc(c.source+':'+c.key)}" href="${esc(detailRoute(c.source,c.key))}">${esc(c.title)}</a></h4><p class="run-status">${esc(statuses[c.status]||c.status)}${c.outcome?` · ${esc(statuses[c.outcome]||c.outcome)}`:''}</p>${lightExplanation(c,detailRoute,fullLink)}${cardPins(c)}<p class="phase">${esc(c.phase?.label||'В работе / этап не сопоставлен')}${c.round!==null?` · round ${esc(c.round)}`:''}</p>${c.branch_count?`<p class="reason">Активных стадий / ветвей: ${c.branch_count}</p>`:''}<div class="chips">${(c.reviews||[]).map(chip).join('')}${!c.review_count&&c.history_count?'<span class="chip unknown">Ревью: нет данных</span>':''}</div><p class="scope">${esc(c.completion_scope)}</p>${stale?'<p class="reason">Источник недоступен · данные устарели</p>':''}<div class="card-bottom"><a data-focus="${esc('history:'+c.source+':'+c.key)}" href="${esc(detailRoute(c.source,c.key))}">История: ${c.history_count} Runs${c.truncated?' · подробнее':''}</a>${c.current_runs?.map(r=>`<a data-focus="${esc('full:'+c.source+':'+r.run_id)}" href="${esc(fullLink(c.source,r.run_id))}" target="_blank" rel="noopener">Run ↗</a>`).join('')||''}</div></article>`;}
 function updateOptions(name,values){const select=form.elements.namedItem(name),value=select.value;const first=select.options[0].outerHTML;const unique=new Map(values);if(value&&!unique.has(value))unique.set(value,value);const html=first+[...unique].map(([v,label])=>`<option value="${esc(v)}">${esc(label)}</option>`).join('');if(select.innerHTML!==html){select.innerHTML=html;select.value=value;}}
 function preserveRender(node,html){
  if(node.innerHTML===html)return;
  const focus=document.activeElement?.dataset.focus;
  const open=new Set([...node.querySelectorAll('details[data-preserve][open]')].map(detail=>detail.dataset.preserve));
  node.innerHTML=html;
  for(const detail of node.querySelectorAll('details[data-preserve]'))detail.open=open.has(detail.dataset.preserve);
  if(focus)[...node.querySelectorAll('[data-focus]')].find(target=>target.dataset.focus===focus)?.focus({preventScroll:true});
 }
 function renderBoard(data){cached=data;fullURL=data.full_monitor_url;const link=$('full-monitor');link.href=fullURL;
  updateOptions('project',data.sources.filter(x=>x.project_id).map(x=>[x.project_id,x.project_id]));updateOptions('source',data.sources.map(x=>[x.source,(x.project_id||'Источник')+' · '+x.source.slice(0,7)]));
  $('source-state').innerHTML=data.sources.filter(x=>x.error||x.coverage!=='complete').map(x=>`<div class="source-error"><p>${esc(x.project_id||x.source.slice(0,10))}: ${esc(x.error||'Индекс строится')}</p><small>Последнее успешное чтение: ${esc(x.last_success_at||'пока отсутствует')}</small></div>`).join('');
  $('source-state').innerHTML+=`<details class="source-freshness"><summary>Источники: ${data.sources.length} · свежесть по проектам</summary>${data.sources.map(x=>`<p><strong>${esc(data.cards.find(c=>c.source===x.source)?.project_title||x.project_id||'Источник')}</strong> · ${esc(({fresh:'доступен',updating:'обновляется',stale:'устаревшие данные',unavailable:'недоступен'})[x.freshness]||x.freshness)} · данные на ${esc(x.last_success_at?new Date(x.last_success_at).toLocaleString('ru-RU'):'ещё не прочитаны')}<small> · cut ${x.cut}</small></p>`).join('')}</details>`;
  $('counters').innerHTML=lanes.filter(([key])=>data.counters[key]).map(([key,label])=>`<span class="counter ${key}">${label}<strong>${data.counters[key]}</strong></span>`).join('');
  const total=data.page.total,from=cursorIndex*data.page.limit+1,to=from+data.cards.length-1;
  $('page-status').textContent=`${total===null?'Итог пока неизвестен':`Найдено ${total}`} · ${data.cards.length?`${from}–${to}`:'0 на странице'}${data.page.complete?' · полный индекс':' · неполные источники'}`;
  $('previous').disabled=cursorIndex===0;$('next').disabled=!data.page.next_cursor;
  let html='';const attention=data.cards.filter(c=>c.lane==='attention');if(attention.length)html=`<section class="attention-group"><h3>Нужно участие · ${attention.length} на странице / ${data.counters.attention||0} всего</h3><div class="cards">${attention.map(c=>cardHTML(c,data.sources)).join('')}</div></section>`;
  const active=lanes.filter(([key])=>key!=='attention'&&data.cards.some(card=>card.lane===key));
  html+=`<div class="lanes">${active.map(([key,label])=>{const cards=data.cards.filter(c=>c.lane===key);return `<section class="lane" aria-label="${label}"><h3>${label}<span>${cards.length} / ${data.counters[key]||0}</span></h3><div class="cards">${cards.map(c=>cardHTML(c,data.sources)).join('')||'<p class="empty-lane">На этой странице нет задач</p>'}</div></section>`;}).join('')}</div>`;
  if(!data.cards.length)html=`<div class="empty">${data.sources.some(x=>x.coverage==='indexing')?'Собираем индекс…':query().get('q')||query().get('project')||query().get('lane')?'По этим условиям ничего не найдено. Сбросьте фильтры.':data.sources.some(x=>x.error)?'Источники недоступны. Последние данные не считаются пустым успешным результатом.':'В этих проектах ещё нет задач и Runs.'}</div>`;
  $('board').setAttribute('aria-busy','false');preserveRender($('board'),html);
 }
 async function request(endpoint,params={},signal,tag=''){
  const response=await fetch(endpoint+'?'+new URLSearchParams(params),{signal,headers:tag?{'If-None-Match':tag}:{}});
  if(response.status===304)return {unchanged:true};
  if(!response.ok){
   const body=await response.json().catch(()=>({}));
   const error=new Error(body?.message||body?.detail||'Ошибка HTTP '+response.status);
   error.status=response.status;error.expired=response.status===409;throw error;
  }
  return {body:await response.json(),tag:response.headers.get('ETag')||''};
 }
 function schedule(){clearTimeout(timer);timer=setTimeout(poll,failures?Math.min(30000,5000*failures):document.hidden?15000:2000);}
 function activeRead(){
  if(!currentDetail)return {kind:'board',endpoint:'api/light/board',params:query()};
  const params=new URLSearchParams(currentDetail);
  if(detailRunsCursor)params.set('cursor',detailRunsCursor);
  return {kind:'task',endpoint:'api/light/task',params};
 }
 const requestKey=({endpoint,params})=>endpoint+'?'+params;
 function readControls(request,failed=false,expired=false){
  const task=request.kind==='task', current=task?taskRead:boardRead;
  const same=current?.key===requestKey(request);
  const status=$(task?'detail-state':'freshness'), retry=$(task?'retry-detail':'retry-board');
  if(!same){
   if(task){
    taskRead=null;
    $('detail').textContent='';
    $('detail').dataset.signature='';
   }else{
    boardRead=null;
    cached=null;
    preserveRender($('board'),failed?'':'<p class="skeleton">Загружаем задачи…</p>');
    $('source-state').textContent='';
    $('counters').textContent='';
    $('page-status').textContent='';
    $('next').disabled=true;
    $('previous').disabled=cursorIndex===0;
   }
  }
  if(!task)$('board').setAttribute('aria-busy',String(!same&&!failed));
  if(!same || failed)status.textContent=failed?readFailure(same):'Читаем данные…';
  retry.hidden=!failed;
  retry.textContent=expired?'Обновить страницу данных':'Повторить чтение';
  retry.onclick=()=>{
   if(expired){
    if(task)detailRunsCursor='';
    else {cursorStack=[''];cursorIndex=0;}
   }
   restart();
  };
 }
 async function poll(){
  if(busy)return;
  if(observing){
   refreshObservation();
   schedule();
   return;
  }
  busy=true;
  const epoch=queryEpoch;
  controller=new AbortController();
  const signal=controller.signal;
  const timeout=setTimeout(()=>controller?.abort(),15000);
  const active=activeRead(),key=requestKey(active),task=active.kind==='task';
  const snapshot=task?taskRead:boardRead,same=snapshot?.key===key;
  const isCurrent=()=>epoch===queryEpoch && key===requestKey(activeRead());
  try{
   let result=await request(active.endpoint,active.params,signal,same?snapshot.tag:'');
   if(!isCurrent())return;
   if(result.unchanged && !same)result=await request(active.endpoint,active.params,signal);
   if(!isCurrent())return;
   if(result.unchanged && !same)throw new Error('Ответ без соответствующего снимка');
   const read=result.unchanged?{...snapshot,at:Date.now()}:{key,tag:result.tag,at:Date.now()};
   if(task){
    if(!result.unchanged)renderTask(result.body);
    taskRead=read;
    $('detail-state').textContent='Задача прочитана';
   }else{
    if(!result.unchanged)renderBoard(result.body);
    boardRead=read;
    $('freshness').textContent='Список прочитан';
   }
   failures=0;
   readControls(active);
   restoreObservationPosition();
  }catch(error){
   if(isCurrent()){
    failures++;
    readControls(active,true,error.expired);
   }
  }finally{
   clearTimeout(timeout);
   busy=false;
   if(epoch!==queryEpoch)poll();
   else schedule();
  }
 }
 function restart(){
  queryEpoch++;
  failures=0;
  controller?.abort();
  for(const flight of pinFlights.values())flight.abort();
  pinFlights.clear();
  clearTimeout(timer);
  if(!observing)readControls(activeRead());
  if(!busy)poll();
 }
 function updateFilterSummary(){
  const count=['project','source','lane'].filter(name=>form.elements.namedItem(name).value).length;
  $('filter-summary').textContent='Фильтры'+(count?' · '+count:'');
 }
 function syncURL(){const params=query();params.delete('cursor');history.replaceState(null,'','?'+params+location.hash);updateFilterSummary();}
 function renderTask(data){fullURL=data.full_monitor_url;$('full-monitor').href=fullURL;const history=new Map(data.runs.map(run=>[run.run_id,run]));const c={...data.card,source:data.source.source,current_runs:(data.card.current_runs || []).map(run=>history.get(run.run_id) || run)},html=`<p class="eyebrow">${esc(c.project_title||c.project_id)}</p><h2>${esc(c.title)}</h2><p class="run-status">${esc(statuses[c.status]||c.status)}${c.outcome?` · ${esc(statuses[c.outcome]||c.outcome)}`: ''}</p>${lightExplanation(c,detailRoute,fullLink,true)}${data.source.freshness==='fresh'?'':'<p class="source-error">Источник не подтвердил свежесть данных</p>'}<details><summary>Фаза и сведения о чтении</summary><p>${esc(c.completion_scope)}</p><p class="muted">${esc(c.phase?.label)} · source cut ${esc(data.source.cut)}</p></details><article id="light-run-history" tabindex="-1"><h3>Записанные Runs · ${data.page.total??'?'}</h3>${data.runs.length?data.runs.map(run=>`<div class="run-row"><p><strong>${esc(statuses[run.status]||run.status)}${run.outcome?' · '+esc(statuses[run.outcome]||run.outcome):''}</strong> <a target="_blank" rel="noopener" href="${esc(fullLink(data.source.source,run.run_id))}">Полный monitor ↗</a></p><code>${esc(run.run_id)}</code><p class="muted">v${run.run_version} · e${run.event_sequence}${run.task_revision!==null?' · task revision '+run.task_revision:''}${run.parent_run_id?' · источник '+esc(run.parent_run_id):''}</p>${run.board?.stages?.map(stage=>`<p>${esc(stage.stage_id)} · ${esc(stage.branches?.join(' / '))} · ${esc(statuses[stage.status]||stage.status)}</p>`).join('')||''}${run.truncated?`<p class="muted">Показана часть стадий (${run.branch_count} всего). Все ветви — в полном monitor.</p>`:''}<div class="light-pin-actions">${pinButton(data.source.source,c.key,run.run_id)}<button type="button" data-reviews="${esc(run.run_id)}">Ревью и источники</button></div><div data-review-result="${esc(run.run_id)}"></div></div>`).join(''):'<p>Задача ещё не запускалась.</p>'}${data.page.next_cursor?`<button type="button" data-history-cursor="${esc(data.page.next_cursor)}">Следующая страница истории</button>`:''}${detailRunsCursor?'<button type="button" data-history-first>В начало истории</button>':''}</article>`;
  // Keep expanded provenance and reading position while the source cut is unchanged.
  const signature=JSON.stringify([data.source.cut,data.source.freshness,detailRunsCursor,c.key]);if($('detail').dataset.signature!==signature){$('detail').dataset.signature=signature;preserveRender($('detail'),html);}
 }
 async function showReviews(button,cursor=''){const run=button.dataset.reviews,box=[...$('detail').querySelectorAll('[data-review-result]')].find(x=>x.dataset.reviewResult===run);if(!box)return;button.disabled=true;box.textContent='Читаем результаты…';const epoch=queryEpoch;try{const result=await request('api/light/reviews',{source:currentDetail.source,run,...(cursor?{cursor}:{})});if(epoch!==queryEpoch||!box.isConnected)return;const data=result.body;box.innerHTML=data.reviews.length?data.reviews.map(row=>`<article>${chip(row)}<p>Проверка: <strong>${esc(states[row.check_status]||row.check_status)}</strong> · Приёмка gate: <strong>${esc(({passed:'Пройдено',blocked:'Заблокировано',failed:'Отклонено',warnings:'Замечания'})[row.gate_status]||row.gate_status)}</strong></p><p>${esc(row.reason)}</p><p class="muted">Round ${row.round??'не установлен'} · ${esc(row.candidate_digest||'candidate не установлен')} · gate: ${esc(row.gate_status)} (${esc(row.gate_scope)})</p>${row.findings.map(f=>`<p>${esc(f)}</p>`).join('')}${row.truncated?`<small>Показана часть findings (${row.finding_count} всего)</small>`:''}<details><summary>Происхождение и raw данные</summary><dl>${Object.entries(row).filter(([key])=>!['findings'].includes(key)).map(([key,value])=>`<dt>${esc(key)}</dt><dd>${esc(typeof value==='object'?JSON.stringify(value):value)}</dd>`).join('')}</dl>${['report','gate','candidate','request'].filter(kind=>row[kind+'_ref']).map(kind=>`<button type="button" data-raw="${kind}" data-review="${esc(row.id)}" data-run="${esc(run)}">${kind}</button>`).join(' ')}<div class="raw-result"></div></details></article>`).join(''):'<p class="muted">Нет данных: workflow не публикует подходящие typed reports или mapping отсутствует.</p>';if(data.page.next_cursor){const next=document.createElement('button');next.textContent='Следующая страница ревью';next.onclick=()=>showReviews(button,data.page.next_cursor);box.append(next);}}catch(e){if(epoch===queryEpoch)box.textContent='Не удалось прочитать ревью: '+e.message;}finally{button.disabled=false;}}
 async function showRaw(button){const box=button.closest('details').querySelector('.raw-result'),epoch=queryEpoch;button.disabled=true;try{const result=await request('api/light/report',{source:currentDetail.source,run:button.dataset.run,review:button.dataset.review,kind:button.dataset.raw});if(epoch!==queryEpoch||!box.isConnected)return;box.innerHTML=`<p class="muted">${result.body.total_bytes} байт${result.body.truncated?' · предпросмотр обрезан; полный raw доступен в monitor':''}</p><pre>${esc(result.body.text)}</pre>`;}catch(e){if(epoch===queryEpoch)box.textContent=e.message;}finally{button.disabled=false;}}
 function restoreObservationPosition(){
  if(!restorePosition)return;
  const {scroll,focus}=restorePosition;
  restorePosition=null;
  window.scrollTo(0,Number(scroll)||0);
  const node=[...document.querySelectorAll('[data-focus]')].find(el=>el.dataset.focus===focus && el.getClientRects().length);
  (node || $('observe-light')).focus({preventScroll:true});
 }
 function routeChanged(){
  const params=route(), wasObserving=observing;
  observing=params.get('observe')==='1' && !params.get('key');
  $('observe-screen').hidden=!observing;
  $('observe-light').setAttribute('aria-current',observing?'page':'false');
  $('light-pin-message').textContent='';
  if(observing){
   $('board-screen').hidden=true;$('detail-screen').hidden=true;
   renderObservation();
   if(!wasObserving){window.scrollTo(0,0);$('observe-title').focus({preventScroll:true});}
   restart();
   return;
  }
  const returning=wasObserving && observeReturn?.hash===location.hash;
  if(returning){
   restorePosition=observeReturn;
   if(observeReturn.search===location.search && Array.isArray(observeReturn.cursors) &&
      observeReturn.cursors.length && observeReturn.cursors.every(cursor=>typeof cursor==='string')){
    cursorStack=observeReturn.cursors.slice();
    cursorIndex=cursorStack.length-1;
   }
  }
  if(params.get('key')&&params.get('source')){
   if(!currentDetail && !returning){
    selectedFocus=document.activeElement?.closest('[data-key]')?.dataset.key || document.activeElement?.dataset.card || '';
    try{sessionStorage.setItem('prifly.light.scroll',String(scrollY));}catch{}
   }
   const selection={source:params.get('source'),key:params.get('key')};
   if(!returning || currentDetail?.source!==selection.source || currentDetail?.key!==selection.key){
    taskRead=null;
    detailRunsCursor=returning && typeof observeReturn.detailCursor==='string'?observeReturn.detailCursor:'';
    $('detail').dataset.signature='';$('detail').textContent='';
    $('detail-state').textContent='Загружаем задачу…';
   }
   currentDetail=selection;
   $('board-screen').hidden=true;$('detail-screen').hidden=false;
   if(!returning){window.scrollTo(0,0);$('back').focus({preventScroll:true});}
  }else{
   const wasDetail=!!currentDetail;currentDetail=null;
   $('board-screen').hidden=false;$('detail-screen').hidden=true;
   if(wasDetail && !returning){
    try{window.scrollTo(0,Number(sessionStorage.getItem('prifly.light.scroll')||0));}catch{}
    [...document.querySelectorAll('[data-card]')].find(x=>x.dataset.card===selectedFocus)?.focus({preventScroll:true});
   }
  }
  if(returning && (currentDetail?taskRead:boardRead))restoreObservationPosition();
  restart();
 }
 $('observe-light').onclick=()=>{
  if(observing)return;
  observeReturn={hash:location.hash,search:location.search,scroll:scrollY,focus:document.activeElement?.dataset.focus || '',
   cursors:cursorStack.slice(0,cursorIndex+1),detailCursor:detailRunsCursor};
  saveObservation();
  location.hash='observe=1';
 };
 $('observe-return').onclick=()=>{location.hash=observeReturn?.hash || '';};
 form.addEventListener('submit',e=>e.preventDefault());let filterTimer;form.addEventListener('input',()=>{clearTimeout(filterTimer);filterTimer=setTimeout(()=>{cursorStack=[''];cursorIndex=0;syncURL();restart();},250);});
 $('reset').onclick=()=>{HTMLFormElement.prototype.reset.call(form);cursorStack=[''];cursorIndex=0;syncURL();restart();};$('next').onclick=()=>{if(cached?.page.next_cursor){cursorStack[++cursorIndex]=cached.page.next_cursor;restart();}};$('previous').onclick=()=>{if(cursorIndex){cursorIndex--;restart();}};$('back').onclick=()=>{location.hash='';};
 document.addEventListener('click',e=>{if(e.target.closest('[data-task-history]')){e.preventDefault();$('light-run-history')?.focus();return;}const button=e.target.closest('button');if(!button)return;if(button.hasAttribute('data-light-pin')){togglePin(button);return;}if(button.dataset.reviews)showReviews(button);if(button.dataset.raw)showRaw(button);if(button.dataset.historyCursor){detailRunsCursor=button.dataset.historyCursor;restart();}if(button.hasAttribute('data-history-first')){detailRunsCursor='';restart();}});
 window.addEventListener('hashchange',routeChanged);document.addEventListener('visibilitychange',()=>{if(!document.hidden)restart();else schedule();});
 const initial=new URLSearchParams(location.search);for(const name of ['q','project','source','lane']){const field=form.elements.namedItem(name);if(initial.has(name)){if(field.tagName==='SELECT'&&![...field.options].some(x=>x.value===initial.get(name))){const option=new Option(initial.get(name),initial.get(name));field.add(option);}field.value=initial.get(name);}}
 setInterval(()=>{if(observing)return;const read=currentDetail?taskRead:boardRead;if(read?.key===requestKey(activeRead())&&!failures){const age=Math.floor((Date.now()-read.at)/1000);if(age>3)$(currentDetail?'detail-state':'freshness').textContent=`Последнее чтение ${age} с назад`;}},1000);
 updateFilterSummary();
 updatePinControls();
 routeChanged();
})();
