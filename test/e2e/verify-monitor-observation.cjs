'use strict';

const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {execFileSync}=require('node:child_process');
const [base,target,binary,modulePath,chromium,lightBase]=process.argv.slice(2);
const {chromium:engine}=require(modulePath);
const read=name=>JSON.parse(fs.readFileSync(path.join(target,name)));
const fixture=read('observation-fixtures.json');
const saved=read('authorities/runtime-fixtures.json');
const title=variant=>`Наблюдение · Run ${variant.toUpperCase()}`;

async function until(check,message,timeout=20000){
 const end=Date.now()+timeout;
 while(Date.now()<end){
  const result=await check();
  if(result)return result;
  await new Promise(resolve=>setTimeout(resolve,100));
 }
 throw new Error(message);
}
function signal(run,name){
 const work=path.join(run.authority,'.prifly','work');
 const found=fs.readdirSync(work,{withFileTypes:true}).find(entry=>entry.isDirectory() && fs.existsSync(path.join(work,entry.name,'observe-step.sh')));
 assert.ok(found,'program scratch must exist before signalling');
 fs.writeFileSync(path.join(work,found.name,name),'');
}
const query=(run,extra={})=>new URLSearchParams({source:run.source,id:run.run_id,...extra});
const hash=(run,extra={})=>new URLSearchParams({source:run.source,run:run.run_id,...extra});
const go=(page,run,extra={})=>page.goto(`${base}/#${hash(run,extra)}`);
const seqs=page=>page.locator('#events [data-seq]').evaluateAll(rows=>rows.map(row=>Number(row.dataset.seq)));
const runRoute=url=>url.pathname==='/api/run';
const progressRoute=url=>url.pathname==='/api/progress';

async function verify(){
 const browser=await engine.launch({executablePath:chromium,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 const context=await browser.newContext({viewport:{width:1440,height:900}});
 await context.tracing.start({screenshots:true,snapshots:true,sources:true});
 const page=await context.newPage(),requests=[],errors=[],checks=[],inFlight=new Set();
 await page.clock.install();
 page.on('pageerror',error=>errors.push(error.message));
 page.on('request',request=>{
  const url=new URL(request.url());
  if(url.origin===base){
   requests.push({method:request.method(),path:url.pathname,query:Object.fromEntries(url.searchParams)});
   if(url.pathname.startsWith('/api/'))inFlight.add(request);
  }
 });
 for(const event of ['requestfinished','requestfailed'])page.on(event,request=>inFlight.delete(request));
 const api=async(endpoint,params='')=>{
  const response=await context.request.get(`${base}/api/${endpoint}?${params}`);
  assert.ok(response.ok(),`${endpoint}: ${response.status()}`);
  return response.json();
 };
 const listing=async value=>(await api('runs',new URLSearchParams({view:'flat',q:value}))).runs;
 const readRun=run=>api('run',query(run));
 const readProgress=async run=>(await api('progress',query(run))).attempts;
 const pin=variant=>page.locator(`.pinned-run[data-run-id="${fixture[variant].run_id}"]`);
 const phase=(variant,value)=>pin(variant).getByText(new RegExp(value));
 const screenshots=[];
 const shot=async name=>{await page.screenshot({path:path.join(target,name)});screenshots.push(name);};
 try {
  const running=await until(async()=>{
   const candidates=await listing('Наблюдение · Run');
   const a=candidates.find(run=>run.subject===title('a')),b=candidates.find(run=>run.subject===title('b'));
   if(!a || !b || a.status!=='running' || b.status!=='running')return null;
   const runs=await Promise.all([readRun(a),readRun(b)]);
   const progress=await Promise.all([readProgress(a),readProgress(b)]);
   if(runs.some(run=>run.run.status!=='running' || !run.driver_live) ||
      progress.some((rows,i)=>!rows.some(row=>row.phase===fixture[['a','b'][i]].phases[0] && !row.settled)))return null;
   return {a,b,runs,progress};
  },'two drivers and their distinct initial reports did not overlap');
  assert.notEqual(running.a.source,running.b.source);
  for(const variant of ['a','b']){
   fixture[variant].run_id=running[variant].run_id;
   assert.equal((await api('capacity',new URLSearchParams({source:running[variant].source}))).capacity,1);
  }
  const overlap=running.runs.map((view,i)=>({
   source:[running.a,running.b][i].source,run_id:view.run.id,status:view.run.status,driver_live:view.driver_live,
   run_version:view.run_version,event_sequence:view.event_sequence,progress:running.progress[i].find(row=>!row.settled)
  }));
  assert.notEqual(overlap[0].progress.attempt_id,overlap[1].progress.attempt_id);

  await page.goto(base);
  assert.equal(await page.locator('[name=view]').inputValue(),'related-newest');
  await page.locator('[name=q]').fill('Наблюдение · Run');
  await until(async()=>await page.locator('.current-run').count()===2,'the two active Runs are not above history');
  for(const variant of ['a','b'])await page.locator('.current-run').filter({hasText:title(variant)}).getByRole('button',{name:`Закрепить: ${title(variant)}`}).click();
  assert.match(await page.locator('#current-counts').innerText(),/2 в работе или ожидании.*0 останавливается/);
  await page.locator('[data-view=current]').click();
  assert.equal(await page.locator('#current-state').inputValue(),'running');
  await page.locator('#current-state').selectOption('stopping');
  assert.equal(new URL(page.url()).searchParams.get('status'),'stopping');
  await page.locator('[data-view=recent]').click();
  assert.equal(new URL(page.url()).searchParams.get('view'),'flat');
  assert.equal(new URL(page.url()).searchParams.has('status'),false);
  await page.locator('[data-view=failed]').click();
  assert.equal(new URL(page.url()).searchParams.get('status'),'failed');
  await page.locator('[data-view=history]').click();
  assert.equal(await page.locator('[name=view]').inputValue(),'related-newest');
  checks.push('O01/O03: current preview and separate status queries; all catalogue presets; related default');

  await page.locator('#observe-link').click();
  await phase('a','build-a').waitFor();await phase('b','compare-b').waitFor();
  const wide=await Promise.all([pin('a').boundingBox(),pin('b').boundingBox()]);
  assert.equal(wide[0].y,wide[1].y);
  await shot('two-runs-desktop.png');
  await page.setViewportSize({width:390,height:844});
  const narrow=await Promise.all([pin('a').boundingBox(),pin('b').boundingBox()]);
  assert.ok(narrow[0].y+narrow[0].height<=narrow[1].y);
  assert.ok(narrow.every(card=>card.x>=0 && card.x+card.width<=390 && card.y+card.height<=844));
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await shot('two-runs-mobile.png');
  for(const variant of ['a','b']){
   await pin(variant).getByRole('link',{name:/Открыть Run/}).click();
   await page.getByRole('heading',{name:title(variant)}).waitFor();
   assert.equal(new URLSearchParams(new URL(page.url()).hash.slice(1)).get('source'),running[variant].source);
   await page.locator('#observe-link').click();
   await pin(variant).waitFor();
  }
  const light=await context.newPage();
  try{
   const lightReads=[];
   light.on('pageerror',error=>errors.push(error.message));
   light.on('request',request=>{
    const url=new URL(request.url());
    if(url.pathname.startsWith('/light/api/'))lightReads.push({method:request.method(),path:url.pathname,query:Object.fromEntries(url.searchParams)});
   });
   await light.goto(lightBase+'?'+new URLSearchParams({q:'Наблюдение'}));
   for(const variant of ['a','b']){
    const id=fixture[variant].run_id;
    await light.locator(`[data-light-pin][data-run="${id}"]`).click();
   }
   await light.locator('#observe-light').click();
   const rows=light.locator('.observed-run');
   await until(async()=>await rows.count()===2 && await rows.locator('.run-status').count()===2,'Light did not read both active Runs');
   for(const [index,variant] of ['a','b'].entries()){
    const row=rows.nth(index);
    assert.equal(await row.getAttribute('data-observed-run'),fixture[variant].run_id);
    assert.equal(await row.getAttribute('data-observed-source'),running[variant].source);
    assert.match(await row.locator('.run-status').innerText(),/В работе|Исполняется|Выполняется/);
    const link=new URL(await row.locator('a[target=_blank]').getAttribute('href'));
    assert.equal(link.origin,base);
    assert.equal(new URLSearchParams(link.hash.slice(1)).get('run'),fixture[variant].run_id);
    assert.equal(new URLSearchParams(link.hash.slice(1)).get('source'),running[variant].source);
   }
   await light.screenshot({path:path.join(target,'light-two-runs-desktop.png')});
   screenshots.push('light-two-runs-desktop.png');
   await light.setViewportSize({width:390,height:844});
   for(const row of await rows.all()){
    const box=await row.boundingBox();
    assert.ok(box.x>=0 && box.x+box.width<=390 && box.y+box.height<=844,JSON.stringify(box));
   }
   assert.ok(await light.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   await light.screenshot({path:path.join(target,'light-two-runs-mobile.png')});
   screenshots.push('light-two-runs-mobile.png');
   const afterEntry=lightReads.length;
   await until(()=>lightReads.slice(afterEntry).filter(read=>read.path.endsWith('/task')).length>=2,'Light observation did not refresh');
   const fresh=lightReads.slice(afterEntry);
   assert.equal(fresh.length,2);
   assert.ok(fresh.every(read=>read.method==='GET' && read.path==='/light/api/light/task'));
   assert.equal(new Set(fresh.map(read=>read.query.source)).size,2);
   const taskRoute='**/api/light/task?*';
   let releaseRead;
   const heldRead=new Promise(resolve=>{releaseRead=resolve;});
   const holdA=async route=>{
    if(new URL(route.request().url()).searchParams.get('source')===running.a.source)await heldRead;
    await route.continue().catch(error=>{
     if(!error.message.includes('Route is already handled'))throw error;
    });
   };
   await light.route(taskRoute,holdA);
   const heldStart=lightReads.length;
   try{
    await until(()=>lightReads.slice(heldStart).filter(read=>read.query.source===running.b.source).length>=2,'a pending Light read delayed the other Run');
    assert.equal(lightReads.slice(heldStart).filter(read=>read.query.source===running.a.source).length,1,'overlapping reads for the same Light task');
   }finally{
    releaseRead();
    await light.unroute(taskRoute,holdA);
   }
   checks.push('L02/L04/L05: native active Runs in Light, exact Full links under a prefix, mobile first screen, bounded independent task reads');
   await light.reload();
   await until(async()=>await rows.locator('.run-status').count()===2,'Light reload lost the pinned pair');
   assert.deepEqual(await rows.evaluateAll(items=>items.map(row=>row.dataset.observedRun)),[fixture.a.run_id,fixture.b.run_id]);
   await light.locator('#observe-return').click();
   await light.locator('#board-screen').waitFor({state:'visible'});
   assert.equal(await light.locator('[name=q]').inputValue(),'Наблюдение');
  }finally{await light.close();}
  await go(page,running.a,{tab:'workflow',node:overlap[0].progress.attempt_id});
  await page.locator('#node').getByText(/Сейчас: build-a/).waitFor();
  const failInspector=route=>route.fulfill({status:503,body:'simulated inspector read failure'});
  await page.route(runRoute,failInspector);
  try{
   await page.locator('#node').getByText(/Последний отчёт — чтение прервано: build-a/).waitFor({timeout:6500});
  }finally{
   await page.unroute(runRoute,failInspector);
  }
  await page.locator('#observe-link').click();
  await phase('a','build-a').waitFor();
  signal(fixture.a,'advance');
  await phase('a','verify-a').waitFor();
  assert.match(await pin('b').innerText(),/compare-b/);
  const advanced=await readRun(running.a);
  assert.equal(advanced.run_version,overlap[0].run_version);
  assert.equal(advanced.event_sequence,overlap[0].event_sequence);
  assert.equal((await readProgress(running.a)).find(row=>row.attempt_id===overlap[0].progress.attempt_id).current,4);
  checks.push('O10: two native drivers, distinct progress, mobile and desktop, exact links, A changes without a Run revision');

  await page.clock.pauseAt(await page.evaluate(()=>Date.now()+1000));
  await until(()=>inFlight.size===0,'reads before the budget check did not finish');
  const budget=[];
  for(let cycle=0;cycle<2;cycle++){
   const from=requests.length;
   await page.clock.runFor(1500);
   await until(()=>inFlight.size===0 && requests.slice(from).filter(request=>request.path==='/api/progress').length>=2,
    'observation did not finish both progress reads in one cycle');
   const reads=requests.slice(from).filter(request=>request.path.startsWith('/api/'));
   assert.equal(reads.length,4);
   for(const run of [running.a,running.b]){
    assert.deepEqual(reads.filter(request=>request.query.source===run.source && request.query.id===run.run_id).map(request=>request.path).sort(),['/api/progress','/api/run']);
   }
   budget.push(reads);
  }
  await page.clock.resume();

  let slowProgress=false,releaseProgress,finishProgress;
  const heldProgress=new Promise(resolve=>{releaseProgress=resolve;});
  const progressContinued=new Promise(resolve=>{finishProgress=resolve;});
  const holdAProgress=async route=>{
   if(new URL(route.request().url()).searchParams.get('id')===fixture.a.run_id && !slowProgress){
    slowProgress=true;
    await heldProgress;
    try{await route.continue();}
    finally{finishProgress();}
    return;
   }
   await route.continue();
  };
  await page.route(progressRoute,holdAProgress);
  try{
   await until(()=>slowProgress,'progress request for A did not enter its barrier');
   const from=requests.length;
   await until(()=>{
    const cycle=requests.slice(from);
    return cycle.some(r=>r.path==='/api/run' && r.query.id===fixture.a.run_id) &&
           cycle.some(r=>r.path==='/api/progress' && r.query.id===fixture.b.run_id);
   },'held progress for A delayed a new Run read',6000);
   assert.match(await pin('a').innerText(),/verify-a/);
  }finally{
   releaseProgress();
   if(slowProgress)await progressContinued;
   await page.unroute(progressRoute,holdAProgress);
  }
  checks.push('A held progress response does not delay A state reads or B progress; inspector labels stale progress');

  const failA=route=>new URL(route.request().url()).searchParams.get('id')===fixture.a.run_id
   ?route.fulfill({status:503,body:'simulated read failure'}):route.continue();
  await page.route(runRoute,failA);
  await pin('a').getByText(/Показаны устаревшие данные/).waitFor();
  signal(fixture.b,'finish');
  await pin('b').locator('.badge.completed').waitFor();
  await phase('b','publish-b').waitFor();
  assert.match(await pin('b').innerText(),/Последняя фаза до завершения/);
  assert.doesNotMatch(await pin('b').innerText(),/устаревшие данные/);
  assert.match(await pin('a').innerText(),/verify-a/);
  assert.equal((await readRun(running.a)).run.status,'running');
  await shot('two-runs-one-read-failed.png');
  await page.unroute(runRoute,failA);
  await pin('a').getByText(/Показаны устаревшие данные/).waitFor({state:'hidden'});
  signal(fixture.a,'finish');
  await pin('a').locator('.badge.failed').waitFor();
  await pin('a').getByText(/Последняя фаза до завершения/).waitFor();
  const final=await Promise.all([readRun(running.a),readRun(running.b)]);
  assert.deepEqual(final.map(v=>[v.run.status,v.run.outcome??null]),[['failed',null],['completed','succeeded']]);
  checks.push('O11: 503 only for A while B finishes, A recovers and fails separately, both retain historical progress; 4 reads per cycle');

  await page.reload();
  await pin('a').locator('.badge.failed').waitFor();
  await pin('b').locator('.badge.completed').waitFor();
  assert.deepEqual(await page.locator('.pinned-run').evaluateAll(cards=>cards.map(card=>card.dataset.runId)),[fixture.a.run_id,fixture.b.run_id]);
  assert.equal(await page.getByRole('heading',{name:'Название задачи не прочитано'}).count(),2);
  const [history]=await listing(fixture.history);
  assert.ok(history);
  await go(page,history);
  await page.locator('#pin-run').click();
  await page.getByText(/Снимите одно из двух закреплений/).waitFor();
  await page.locator('#observe-link').click();
  await pin('a').getByRole('button',{name:/Снять закрепление/}).click();
  await pin('b').getByRole('button',{name:/Снять закрепление/}).click();
  const clones=await listing(saved.uncertain.run_id);
  assert.equal(clones.length,2);
  for(const clone of clones){
   await go(page,clone);
   await page.locator('#pin-run').click();
  }
  await page.locator('#observe-link').click();
  assert.equal(await page.locator('.pinned-run').count(),2);
  assert.equal((await page.locator('.pinned-run').evaluateAll(cards=>new Set(cards.map(card=>card.dataset.source)).size)),2);
  checks.push('O12: reload, maximum two, unpin, same Run ID from two independent sources');

  // The legacy fragment is exercised without a tab and includes an exact Attempt.
  const attempt=overlap[0].progress.attempt_id;
  await go(page,running.a,{node:attempt});
  await page.locator('#node-title').waitFor();
  await page.getByText('Output from Run A',{exact:true}).waitFor();
  assert.equal(await page.locator('[data-tab=workflow]').getAttribute('aria-pressed'),'true');
  await page.locator('#back-to-tree').click();
  const entry=page.locator(`#tree [data-node="${attempt}"]`).first();
  await entry.evaluate(el=>{for(let d=el.parentElement;d;d=d.parentElement)if(d.tagName==='DETAILS')d.open=true;});
  await entry.focus();
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(()=>document.activeElement.id),'node-title');
  await page.locator('#back-to-tree').click();
  await until(async()=>await page.evaluate(()=>document.activeElement.dataset.node)===attempt,'tree selection did not regain focus');
  const plan=await page.locator('#invocation option[value^="def:"]').first().getAttribute('value');
  await page.locator('#invocation').selectOption(plan);
  await page.reload();
  await until(async()=>await page.locator('#invocation').inputValue()===plan,'the definition was not restored');
  checks.push('O06/O09: legacy Attempt link and program output, keyboard inspection, return focus, plan link persists');

  await go(page,running.a,{tab:'overview'});
  const runFailure=route=>route.fulfill({status:503,body:'simulated detail read failure'});
  await page.locator('#recent-rows a').first().waitFor();
  await page.route(runRoute,runFailure);
  await page.locator('#detail-error').filter({hasText:/устаревшие данные/}).waitFor();
  assert.match(await page.locator('#connection').innerText(),/Чтение Run прервано/);
  await page.unroute(runRoute,runFailure);
  await page.locator('#retry-run').click();
  await page.locator('#detail-error').waitFor({state:'hidden'});
  const auxFailure=route=>route.fulfill({status:503,body:'simulated progress read failure'});
  await page.route(progressRoute,auxFailure);
  await page.locator('#detail-error').filter({hasText:/Недоступно: ход исполнения/}).waitFor();
  await page.locator('.detail-primary .badge.failed').waitFor();
  await page.unroute(progressRoute,auxFailure);
  await page.locator('#detail-error').waitFor({state:'hidden'});
  assert.match(await page.locator('.detail-primary').innerText(),/Чтение Run.*Наблюдение Run/s);
  const [holder]=await listing(saved.holder.run_id);
  await go(page,holder);
  await page.getByText(/Принятие хостом не записано/).first().waitFor();
  checks.push('O04/O05: selected Run becomes stale independently, optional failure preserves snapshot, host take is distinguished');

  await go(page,history);
  await page.locator('#recent-rows a').first().waitFor();
  const exactLink=page.locator('#recent-rows a').first();
  const exact=Number(new URLSearchParams((await exactLink.getAttribute('href')).slice(1)).get('event'));
  await exactLink.click();
  await page.locator(`#events [data-seq="${exact}"][open]`).waitFor();
  assert.equal(await page.evaluate(()=>document.activeElement.closest('[data-seq]')?.dataset.seq),String(exact));
  await page.locator('#event-history').click();
  await until(async()=>(await seqs(page)).length===200,'first full journal page is not 200 records');
  await page.locator('#more-events').click();
  const historyRows=await until(async()=>{const rows=await seqs(page);return rows.length>200 && rows;},'full journal did not continue from its cursor');
  assert.deepEqual(historyRows,[...new Set(historyRows)].sort((a,b)=>a-b));
  execFileSync(binary,['--json','--project',path.join(target,'authorities/live'),'run','pause',fixture.history,'--reason','Verify browser follow','--command-id','command:observe-history-new'],{encoding:'utf8'});
  await until(async()=>(await readRun(history)).event_sequence>historyRows.at(-1),'fixture did not add a new event');
  await page.waitForTimeout(1600);
  assert.deepEqual(await seqs(page),historyRows);
  await page.locator('#event-follow').check();
  await until(async()=>(await seqs(page)).length>historyRows.length,'following did not load the new event');
  checks.push('O07/O08: actual 200-event page, exact event and focus, independent manual cursor and follow');

  await page.goto(`${base}/?view=board&status=completed&page=4`);
  await until(async()=>/^4 \//.test(await page.locator('#page-top').innerText()),'catalogue did not restore its URL');
  await page.locator('#previous-top').click();
  await until(async()=>/^3 \//.test(await page.locator('#page').innerText()),'top pagination did not change the page');
  assert.equal(new URL(page.url()).searchParams.get('page'),'3');
  await page.reload();
  await until(async()=>/^3 \//.test(await page.locator('#page-top').innerText()),'reloaded page did not keep its address');
  checks.push('O02: URL restores selection, top and bottom pagination share the clamped server page');
  assert.deepEqual(errors,[]);
  assert.ok(requests.every(request=>['GET','HEAD'].includes(request.method)),'monitor issued a non-read request');
  fs.writeFileSync(path.join(target,'browser.json'),JSON.stringify({outcome:'passed',checks,overlap,advanced:{run_version:advanced.run_version,event_sequence:advanced.event_sequence},
   outcomes:final.map(v=>({run_id:v.run.id,status:v.run.status,outcome:v.run.outcome??null})),screenshots,budget,requests,console_page_errors:errors,artifacts:[...screenshots,'trace.zip']},null,2));
 } finally {
  await context.tracing.stop({path:path.join(target,'trace.zip')});
  await browser.close();
 }
}
verify().catch(error=>{console.error(error);process.exitCode=1;});
