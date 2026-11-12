'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),zlib=require('node:zlib');
const [base,full,target,modulePath,chromium,scenario]=process.argv.slice(2),{chromium:engine}=require(modulePath);
async function until(fn,message){const end=Date.now()+25000;while(Date.now()<end){if(await fn())return;await new Promise(r=>setTimeout(r,100));}throw new Error(message);}
async function stopCases(browser){
 const fixtures=path.join(target,'authorities');
 const saved=name=>JSON.parse(fs.readFileSync(path.join(fixtures,name)));
 const runtime=saved('runtime-fixtures.json'), cli=saved('cli-fixtures.json');
 const ids={
  decision:runtime.parallel.run_id,resource:runtime.resource.run_id,
  failed:runtime.failed.run_id,uncertain:runtime.uncertain.run_id,partial:cli.partial
 };
 const context=await browser.newContext({viewport:{width:390,height:844}});
 const page=await context.newPage(), reads=[];
 page.on('request',request=>reads.push({url:request.url(),method:request.method()}));
 const boardRoute='**/api/light/board?*', taskRoute='**/api/light/task?*';
 const offline=route=>route.fulfill({status:503,json:{message:'offline'}});
 const isInside=async locator=>{
  await locator.waitFor();
  const box=await locator.boundingBox();
  assert.ok(box.x>=0 && box.x+box.width<=391 && box.y>=0 && box.y+box.height<=844,JSON.stringify(box));
 };
 try{
  await page.route(boardRoute,offline);
  await page.goto(base);
  await until(async()=>/Не удалось прочитать/.test(await page.locator('#freshness').innerText()),'first GET error was not shown');
  assert.doesNotMatch(await page.locator('#freshness').innerText(),/устаревшие|сохранены/);
  await page.unroute(boardRoute,offline);
  await page.locator('#retry-board').click();
  await isInside(page.locator('.card-title').first());
  await isInside(page.locator('.run-explanation a').first());
  const find=async id=>{
   await page.locator('[name=q]').fill(id);
   const response=await context.request.get(base+'api/light/board?'+new URLSearchParams({q:id}));
   const board=await response.json();
   assert.equal(board.cards.length,1,id);
   const card=page.locator('.light-card').filter({has:page.locator(`.card-title[href="${'#'+new URLSearchParams({source:board.cards[0].source,key:board.cards[0].key})}"]`)});
   await until(async()=>await page.locator('.light-card').count()===1 && await card.count()===1,'query did not render '+id);
   return card;
  };
  for(const [type,pattern] of Object.entries({
   decision:/Ожидается ответ/,resource:/Ресурс .* занят/,
   failed:/Истёк срок попытки/,uncertain:/Связь с хостом потеряна/
  })){
   const card=await find(ids[type]);
   const explanation=card.locator('.run-explanation');
   assert.match(await explanation.innerText(),pattern);
   await isInside(explanation.locator('a').first());
   assert.equal(await page.locator('.empty-lane:visible').count(),0);
   if(type==='uncertain')await page.screenshot({path:path.join(target,'light-stop-mobile.png')});
  }
  const partial=await find(ids.partial);
  await partial.locator('.card-title').click();
  await until(async()=>await page.locator('.run-row').filter({hasText:ids.partial}).count()>0,'partial Run missing from history');
  assert.match(await page.locator('.run-row').filter({hasText:ids.partial}).innerText(),/Частичный|Частично/);
  await page.locator('#back').click();
  const uncertain=await find(ids.uncertain);
  await page.route(taskRoute,offline);
  await uncertain.locator('.card-title').focus();
  await page.keyboard.press('Enter');
  await until(async()=>/Не удалось прочитать/.test(await page.locator('#detail-state').innerText()),'first detail failure not shown');
  assert.doesNotMatch(await page.locator('#detail-state').innerText(),/устаревшие|сохранены/);
  await page.unroute(taskRoute,offline);
  await page.locator('#retry-detail').click();
  await isInside(page.locator('#detail .run-explanation a').first());
  assert.match(await page.locator('#detail .run-explanation').innerText(),/Связь с хостом потеряна/);
  await page.route(taskRoute,offline);
  await until(async()=>/устаревшие/.test(await page.locator('#detail-state').innerText()),'same-object refresh did not mark snapshot stale');
  assert.match(await page.locator('#detail .run-explanation').innerText(),/Связь с хостом потеряна/);
  await page.unroute(taskRoute,offline);
  await page.locator('#retry-detail').click();
  await until(async()=>!/устаревшие/.test(await page.locator('#detail-state').innerText()),'recovered status retained a stale warning');
  const href=new URL(await page.locator('#detail .run-explanation a[target=_blank]').first().getAttribute('href'));
  assert.equal(href.origin,new URL(full).origin);
  assert.equal(new URLSearchParams(href.hash.slice(1)).get('run'),ids.uncertain);
  assert.equal(new URLSearchParams(href.hash.slice(1)).get('source'),new URLSearchParams(new URL(page.url()).hash.slice(1)).get('source'));
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await page.screenshot({path:path.join(target,'light-stop-detail.png')});
  await page.locator('#back').click();
  for(const type of ['failed','decision']){
   const card=await find(ids[type]);
   await card.locator(`[data-light-pin][data-run="${ids[type]}"]`).click();
  }
  await page.locator('#observe-light').click();
  const rows=page.locator('.observed-run');
  const expired=rows.nth(0),waiting=rows.nth(1);
  await expired.getByRole('link',{name:/Открыть попытку в Full/}).waitFor();
  await waiting.getByRole('link',{name:/Результаты и решения в Full/}).waitFor();
  const exact=new URL(await expired.locator('a[target=_blank]').getAttribute('href'));
  const area=new URL(await waiting.locator('a[target=_blank]').getAttribute('href'));
  const exactRoute=new URLSearchParams(exact.hash.slice(1)),areaRoute=new URLSearchParams(area.hash.slice(1));
  assert.equal(exactRoute.get('tab'),'workflow');
  assert.ok(exactRoute.get('node')?.startsWith('attempt:'));
  assert.equal(exactRoute.get('run'),ids.failed);
  assert.equal(areaRoute.get('tab'),'facts');
  assert.equal(areaRoute.get('run'),ids.decision);
  assert.equal(areaRoute.has('node'),false);
  const inspect=await context.newPage();
  try{
   await inspect.goto(exact.href);
   await inspect.locator('#node .attempt-diagnostic').getByText('attempt_deadline_expired',{exact:true}).waitFor();
  }finally{await inspect.close();}
  await page.locator('#observe-return').click();
  await page.locator('#board-screen').waitFor({state:'visible'});
  assert.equal(await page.locator('[name=q]').inputValue(),ids.decision);
  assert.ok(reads.every(r=>r.method==='GET'));
  fs.writeFileSync(path.join(target,'stop-cases.json'),JSON.stringify({outcome:'passed',cases:Object.keys(ids),reads},null,2)+'\n');
 }finally{await context.close();}
}
(async()=>{
 const browser=await engine.launch({executablePath:chromium,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 if(scenario==='stops'){try{await stopCases(browser);}finally{await browser.close();}return;}
 const context=await browser.newContext({viewport:{width:1440,height:900}}),page=await context.newPage(),errors=[],requests=[],responses=[];
 page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>requests.push(r.url()));page.on('response',r=>responses.push({url:r.url(),status:r.status()}));
 const api=async(q='')=>{const r=await context.request.get(base+'api/light/board'+(q?'?'+q:''));assert.equal(r.status(),200);return r.json();};
 const metrics={};
 try {
  await page.goto(base);await until(async()=>await page.locator('.light-card').count()>0,'Light board did not render');
  const first=await api();assert.ok(first.page.total>=2003);assert.ok(first.cards.length<=25);assert.ok(first.cards.some(c=>c.kind==='queue_task'));assert.ok(first.cards.some(c=>c.reviews.some(r=>r.state==='unavailable')),'typed blocked gate missing from board');
  metrics.initial_requests=requests.length;assert.ok(metrics.initial_requests<=6,'first render request budget');
  metrics.page_bytes=Buffer.byteLength(JSON.stringify(first));assert.ok(metrics.page_bytes<=128*1024);for(const card of first.cards)assert.ok(Buffer.byteLength(JSON.stringify(card))<=4096+100);
  metrics.asset_gzip_bytes=0;for(const name of ['','light.css','light.js','monitor-theme.js']){const response=await context.request.get(base+name);metrics.asset_gzip_bytes+=zlib.gzipSync(await response.body()).length;}assert.ok(metrics.asset_gzip_bytes<=250*1024);
  await page.screenshot({path:path.join(target,'light-desktop.png'),fullPage:true});await page.screenshot({path:path.join(target,'light-first-screen.png')});
  const fullPage=await context.newPage();await fullPage.goto(full);await fullPage.locator('#runs-title').waitFor();assert.notEqual(await fullPage.title(),await page.title());assert.equal(await fullPage.locator('#board-screen').count(),0);await fullPage.screenshot({path:path.join(target,'full-desktop.png'),fullPage:true});
  const latencies=[];for(let i=0;i<100;i++){const t=performance.now();await api();latencies.push(performance.now()-t);}latencies.sort((a,b)=>a-b);metrics.warm_p95_ms=latencies[94];assert.ok(metrics.warm_p95_ms<=250,'warm p95 budget');
  if(fs.existsSync(path.join(target,'authorities/feature-mapping.json'))){const feature=await api('q='+encodeURIComponent('Выпущенный aif-feature'));assert.equal(feature.cards.length,1);assert.equal(feature.cards[0].lane,'plan');assert.equal(feature.cards[0].phase.mapped,true);metrics.shipped_feature_mapped=true;}
  const board=await context.request.get(base+'api/light/board'),etag=board.headers().etag;const unchanged=await context.request.get(base+'api/light/board',{headers:{'If-None-Match':etag}});assert.equal(unchanged.status(),304);assert.equal((await unchanged.body()).length,0);
  await page.locator('[name=q]').fill('Задача 0999');await until(async()=>await page.locator('.light-card').count()===1,'search outside first page');assert.match(await page.locator('.light-card').innerText(),/Задача 0999/);
  await page.locator('.card-title').focus();await page.keyboard.press('Enter');await page.locator('#detail-screen').waitFor({state:'visible'});await until(async()=>/ещё не запускалась/.test(await page.locator('#detail').innerText()),'queued task details did not load');await page.locator('#back').click();await page.locator('#board-screen').waitFor({state:'visible'});assert.equal(await page.locator('[name=q]').inputValue(),'Задача 0999');
  await page.locator('#reset').click();await until(async()=>await page.locator('.light-card').count()>1,'reset did not restore data');
  const continuation=(await api(new URLSearchParams({q:'current request for the next Run'}).toString())).cards.find(card=>card.history_count===3);
  assert.ok(continuation,'native continuation task not found');
  const taskHash='#'+new URLSearchParams({source:continuation.source,key:continuation.key});
  const taskRoute='**/api/light/task?*';
  const longStage='stage_'+ 'x'.repeat(190), longBranch='branch_'+ 'y'.repeat(190);
  let responseMode='long';
  const changeTask=async route=>{
   if(responseMode==='offline')return route.fulfill({status:503,json:{message:'offline'}});
   if(responseMode==='denied')return route.fulfill({status:403,json:null});
   if(responseMode==='delay')await new Promise(resolve=>setTimeout(resolve,1500));
   const response=await route.fetch();
   const data=await response.json();
   if(responseMode==='long'){
    data.runs[0].board??={};
    data.runs[0].board.stages=[{stage_id:longStage,branches:[longBranch],status:'running'}];
   }
   if(responseMode==='missing'){
    data.runs=data.runs.slice(2);
    data.card.current_runs=[];
   }
   await route.fulfill({response,json:data});
  };
  await page.route(taskRoute,changeTask);
  await page.goto(base+taskHash);
  const history=page.locator('#light-run-history'), historyRows=history.locator('.run-row');
  await historyRows.nth(2).waitFor();
  await page.getByText(longStage+' · '+longBranch+' · Выполняется',{exact:true}).waitFor();
  for(const width of [390,375,320]){
   await page.setViewportSize({width,height:812});
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'long task history overflows at '+width);
   assert.ok((await history.innerText()).includes(longStage+' · '+longBranch));
  }
  await page.setViewportSize({width:1440,height:900});
  const historyPins=history.locator('[data-light-pin]');
  const selected=await historyPins.evaluateAll(buttons=>buttons.slice(0,2).map(button=>button.dataset.run));
  await historyPins.nth(0).click();
  await historyPins.nth(1).click();
  await historyPins.nth(2).click();
  assert.match(await page.locator('#light-pin-message').innerText(),/Закреплено два Run/);
  await page.evaluate(()=>{
   window.observationReads=[];
   const request=window.fetch;
   window.fetch=(...args)=>{
    if(!document.getElementById('observe-screen').hidden)window.observationReads.push(new URL(args[0],location.href).pathname);
    return request(...args);
   };
  });
  await page.clock.install();
  await page.clock.pauseAt(await page.evaluate(()=>Date.now()+1000));
  await page.locator('#observe-light').click();
  const rows=page.locator('.observed-run');
  await until(async()=>await rows.locator('.run-status').count()===2,'same-task pair did not load');
  assert.deepEqual(await rows.evaluateAll(items=>items.map(item=>item.dataset.observedRun)),selected);
  assert.notEqual(await rows.nth(0).locator('.run-status').innerText(),await rows.nth(1).locator('.run-status').innerText());
  assert.deepEqual(await page.evaluate(()=>window.observationReads),[new URL('api/light/task',base).pathname],'same task fetched twice');
  await page.clock.resume();
  responseMode='offline';
  await rows.nth(0).getByText(/Последние данные устарели/).waitFor();
  assert.equal(await rows.locator('.run-status').count(),2);
  responseMode='missing';
  await rows.nth(0).getByText(/Run отсутствует/).waitFor();
  assert.deepEqual(await rows.evaluateAll(items=>items.map(item=>item.dataset.observedRun)),selected);
  responseMode='denied';
  await rows.nth(0).getByText(/Доступ к источнику запрещён/).waitFor();
  assert.equal(await rows.locator('.run-status,a[target=_blank]').count(),0);
  responseMode='normal';
  await until(async()=>await rows.locator('.run-status').count()===2,'observation did not recover after denied read');
  responseMode='delay';
  await page.waitForRequest(request=>new URL(request.url()).pathname.endsWith('/api/light/task'),{timeout:12000});
  await page.locator('#observe-return').click();
  await page.locator('#detail-screen').waitFor({state:'visible'});
  assert.equal(new URL(page.url()).hash,taskHash);
  await page.waitForTimeout(1700);
  assert.equal(await page.locator('#observe-screen').isVisible(),false,'obsolete observation response changed the view');
  await page.unroute(taskRoute,changeTask);
  await page.locator('#back').click();
  metrics.refined_observation=['same task pinned twice with one bounded request','third pin refused','503, missing and 403, recovery and late response','full unbroken stage and branch at 390, 375 and 320 px'];
  const native=first.cards.find(c=>c.reviews.some(r=>r.state==='unavailable'));await page.goto(base+'#'+new URLSearchParams({source:native.source,key:native.key}));await page.locator('[data-reviews]').first().click();await page.locator('[data-review-result] article details > summary').first().waitFor();assert.match(await page.locator('#detail').innerText(),/Приёмка заблокирована|blocked/);assert.match(await page.locator('#detail').innerText(),/Пройдено|passed/);await page.locator('[data-review-result] article details > summary').first().click();await page.locator('[data-raw=gate]').first().click();await page.locator('.raw-result pre').waitFor();assert.match(await page.locator('.raw-result pre').first().innerText(),/blocked/);
  const link=new URL(await page.locator('#detail a[target=_blank]').first().getAttribute('href'));assert.equal(link.origin,new URL(full).origin);const hash=new URLSearchParams(link.hash.slice(1));assert.equal(hash.get('source'),native.source);assert.ok(hash.get('run'));await page.screenshot({path:path.join(target,'typed-review-provenance.png'),fullPage:true});
  await page.locator('#back').click();await page.locator('#theme').selectOption('dark');assert.equal(await page.locator('html').getAttribute('data-theme'),'dark');await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(target,'light-mobile-dark.png'),fullPage:true});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mobile horizontal overflow');
  await page.locator('[name=q]').focus();await page.keyboard.press('Tab');assert.notEqual(await page.evaluate(()=>document.activeElement.tagName),'BODY');
  await page.locator('[name=q]').fill('nothing-matches-this');await until(async()=>await page.locator('.empty').count()===1,'empty filter state');assert.match(await page.locator('.empty').innerText(),/ничего не найдено/);
  // An obsolete response must not restore the previous filter.
  await page.route('**/api/light/board?*',async route=>{const url=new URL(route.request().url());if(url.searchParams.get('q')==='obsolete'){await new Promise(r=>setTimeout(r,500));}await route.continue().catch(()=>{});});
  await page.locator('[name=q]').fill('obsolete');await page.waitForTimeout(300);await page.locator('[name=q]').fill('Задача 0001');await until(async()=>await page.locator('.light-card').count()===1,'new query did not settle');await page.waitForTimeout(700);assert.match(await page.locator('.light-card').innerText(),/Задача 0001/);await page.unroute('**/api/light/board?*');
  await page.locator('#reset').click();await until(async()=>await page.locator('.light-card').count()>1,'reset for source failure');
  const config=path.join(target,'authorities/performance/prifly.json'),away=config+'.unavailable';fs.renameSync(config,away);try{await until(async()=>await page.locator('.source-error').count()>0,'inaccessible source hidden');assert.ok(await page.locator('.light-card').count()>0,'healthy sources disappeared');}finally{fs.renameSync(away,config);}
  assert.equal((await context.request.post(base+'api/light/board')).status(),405);assert.equal((await context.request.get(base+'api/maintenance/delete')).status(),404);assert.equal((await context.request.get(base+'api/runs')).status(),404);
  const pollStart=responses.length;await page.waitForTimeout(4500);metrics.poll_responses=responses.slice(pollStart).filter(r=>r.url.includes('api/light/board')).length;assert.ok(metrics.poll_responses<=3,'unbounded poll requests');
  assert.ok(!requests.some(r=>/\/api\/(events|run\?|timing|storage|artifact)/.test(r)&&r.startsWith(base)),'heavy endpoint loaded');assert.deepEqual(errors,[]);
  const hiddenStart=responses.length;await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,get:()=>true});document.dispatchEvent(new Event('visibilitychange'));});await page.waitForTimeout(16000);metrics.hidden_poll_responses=responses.slice(hiddenStart).filter(r=>r.url.includes('api/light/board')).length;assert.ok(metrics.hidden_poll_responses<=1,'hidden poll budget');await page.evaluate(()=>{delete document.hidden;document.dispatchEvent(new Event('visibilitychange'));});
  metrics.browser='Chromium '+browser.version();metrics.observations=['separate Light/full DOM and first screen','native queue without Run','typed usable report + blocked gate, raw provenance','search beyond 200, pagination totals','direct task deep link and exact full source/Run link','mobile dark theme and keyboard','empty/error/source-loss and stale query handling','conditional bounded polling, no heavy Light APIs'];
  fs.writeFileSync(path.join(target,'browser-metrics.json'),JSON.stringify(metrics,null,2)+'\n');fs.writeFileSync(path.join(target,'browser-requests.json'),JSON.stringify(responses,null,2)+'\n');console.log(JSON.stringify(metrics,null,2));
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
