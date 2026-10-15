'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process');
const [base,target,binary,modulePath,chromium]=process.argv.slice(2);
const {chromium:engine}=require(modulePath);
const cli=JSON.parse(fs.readFileSync(path.join(target,'authorities/cli-fixtures.json')));
const runtime=JSON.parse(fs.readFileSync(path.join(target,'authorities/runtime-fixtures.json')));
const live=JSON.parse(fs.readFileSync(path.join(target,'authorities/live-fixture.json')));
const ids={parallel:runtime.parallel.run_id,resource:runtime.resource.run_id,holder:runtime.holder.run_id};
async function until(check,message){const end=Date.now()+20000;while(Date.now()<end){if(await check())return;await new Promise(r=>setTimeout(r,100));}throw new Error(message);}
(async()=>{
 const browser=await engine.launch({executablePath:chromium,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 const context=await browser.newContext({viewport:{width:1440,height:1000}});await context.tracing.start({screenshots:true,snapshots:true,sources:true});
 const page=await context.newPage(),errors=[],consoleErrors=[],requests=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')consoleErrors.push({text:m.text(),location:m.location()});});page.on('request',r=>requests.push(new URL(r.url()).pathname));
 const listing=async(query='')=>(await context.request.get(base+'/api/runs?view=board&'+query)).json();
 const search=async(id)=>{await page.locator('[name=q]').fill(id);await until(async()=>{const cards=page.locator('.board-card');return await cards.count()===1 && await cards.first().getAttribute('data-run-id')===id;},'search did not settle on '+id);};
 const command=(authority,...args)=>JSON.parse(execFileSync(binary,['--json','--project',path.join(target,'authorities',authority),...args],{encoding:'utf8'}));
 await page.clock.install();
 const checks=[];
 try {
  await page.goto(base);await page.locator('[name=view]').selectOption('board');await until(async()=>await page.locator('.board-column').count()===6,'six board columns missing');
  await search(ids.parallel);
  const parallel=page.locator('.board-card');assert.match(await parallel.innerText(),/reader-left/);assert.match(await parallel.innerText(),/reader-right/);assert.match(await parallel.innerText(),/Ожидается ответ/);assert.equal(await parallel.locator('xpath=..').getAttribute('data-category'),'waiting');
  await page.screenshot({path:path.join(target,'parallel.png'),fullPage:true});
  assert.match(await parallel.innerText(),/Название задачи не записано/);checks.push('A1/A2/A4: pending decision, both saved branches, generic title fallback');
  await search(ids.resource);assert.equal(await page.locator('.board-card').locator('xpath=..').getAttribute('data-category'),'resource');assert.match(await page.locator('.board-card').innerText(),/heavy.*1\/1/);
  await search(live.run_id);const before=(await listing('q='+encodeURIComponent(live.run_id))).runs[0];
  assert.equal(before.board.category,'ready');command('live','run','drive',live.run_id);
  await until(async()=>await page.locator('.board-card').locator('xpath=..').getAttribute('data-category')==='terminal','live state change did not refresh board');
  const after=(await listing('q='+encodeURIComponent(live.run_id))).runs[0];assert.ok(after.run_version>before.run_version);assert.ok(after.event_sequence>before.event_sequence);assert.equal(after.board.category,'terminal');assert.equal(after.status,'completed');assert.equal(after.board.stages.length,0);
  checks.push('A3/A8: resource blocker displayed; live CLI Run replaces its complete card on terminal transition');
  for(const [id,category,status] of [[cli.partial,'terminal','partial'],[cli.cancelled,'terminal','cancelled'],[runtime.failed.run_id,'terminal','failed'],[runtime.uncertain.run_id,'stopped','uncertain']]) {
   await search(id);assert.equal(await page.locator('.board-card').locator('xpath=..').getAttribute('data-category'),category);
   const row=(await listing('q='+encodeURIComponent(id))).runs[0];assert.ok(row.status===status || row.outcome===status);
  }
  await search(cli.partial);let row=(await listing('q='+encodeURIComponent(cli.partial))).runs[0];assert.deepEqual(new Set(row.related_runs.map(r=>r.run_id)),new Set([cli.cancelled,cli.repeated]));assert.equal(row.outcome,'partial');
  await search(cli.next);row=(await listing('q='+encodeURIComponent(cli.next))).runs[0];assert.equal(row.fork_source_run_id,cli.cancelled);assert.notEqual(row.status,'cancelled');
  await search(cli.repeated);row=(await listing('q='+encodeURIComponent(cli.repeated))).runs[0];assert.equal(row.fork_source_run_id,cli.partial);
  await search(cli.legacy);assert.match(await page.locator('.board-card').innerText(),/Связь с исходным Run не записана/);assert.equal(await page.locator('.board-card img').count(),0);
  checks.push('A3/A5: partial, cancelled, failed, uncertain, continuation chain, repeated continue, legacy missing provenance');
  // Delay an obsolete list response, then select a different filter while it is in flight.
  let release,intercepted=false,obsoleteFinished=false;
  const obsoleteDone=request=>{if(new URL(request.url()).searchParams.get('q')==='obsolete-filter')obsoleteFinished=true;};
  page.on('requestfinished',obsoleteDone);
  await page.route('**/api/runs?*',async route=>{
   const url=new URL(route.request().url());
   if(url.searchParams.get('q')==='obsolete-filter' && !intercepted){
    const response=await route.fetch();assert.equal((await response.json()).runs.length,0);
    const held=new Promise(resolve=>{release=resolve;});intercepted=true;await held;await route.fulfill({response});
   }else await route.continue();
  });
  try {
   await page.locator('[name=q]').fill('obsolete-filter');await until(()=>intercepted,'delayed response was not intercepted');
   await page.clock.pauseAt(await page.evaluate(()=>Date.now()+100));
   await page.evaluate(desired=>{
    window.obsoleteBoardInstalls=[];
    window.boardFilterObserver=new MutationObserver(records=>{
     if(document.querySelector('[name=q]').value!==desired)return;
     for(const record of records)for(const added of record.addedNodes){
      if(added.nodeType===1 && added.matches('.board-grid'))window.obsoleteBoardInstalls.push(added.querySelectorAll('.board-card').length);
     }
    });
    window.boardFilterObserver.observe(document.querySelector('#board'),{childList:true});
   },ids.parallel);
   await page.locator('[name=q]').fill(ids.parallel);release();
   await until(()=>obsoleteFinished,'obsolete HTTP response did not finish');
   // Network/DOM work settles while the 200ms debounce is still frozen.
   await page.evaluate(()=>new Promise(resolve=>queueMicrotask(resolve)));
   await page.clock.runFor(200);
   await until(async()=>await page.locator('.board-card').count()===1 && await page.locator('.board-card').getAttribute('data-run-id')===ids.parallel,'obsolete response overwrote new filter');
   const filterObservations=await page.evaluate(()=>window.obsoleteBoardInstalls);
   assert.ok(!filterObservations.includes(0),'obsolete empty response was installed before debounce completed');
   if(process.env.LOG_LEVEL==='debug')console.log('[FIX:monitor-filter]',JSON.stringify({filter:ids.parallel,card_counts:filterObservations,obsolete_installed:false}));
  }finally{
   if(release)release();await page.clock.resume();
   await page.evaluate(()=>window.boardFilterObserver?.disconnect());
   await page.unroute('**/api/runs?*');page.off('requestfinished',obsoleteDone);
  }
  checks.push('A8: obsolete real HTTP response discarded immediately on input with debounce clock frozen');
  const config=path.join(target,'authorities/generic/prifly.json'),saved=fs.readFileSync(config);
  try {fs.writeFileSync(config,'{}');await search(cli.legacy);await until(async()=>/Устаревшие данные/.test(await page.locator('.board-card').innerText()),'source error did not mark retained card stale');await search(ids.parallel);assert.doesNotMatch(await page.locator('.board-card').innerText(),/Устаревшие данные/);}finally{fs.writeFileSync(config,saved);}
  checks.push('A6: failed source keeps explicitly stale cards; healthy authority remains readable');
  await page.locator('[name=q]').fill('');await page.locator('[name=status]').selectOption('completed');await until(async()=>await page.locator('.board-card').count()===50,'full page of equal-title Runs missing');
  const keys=await page.locator('.board-card').evaluateAll(cards=>cards.map(c=>c.dataset.runKey));assert.equal(new Set(keys).size,50);
  for(let n=0;n<4;n++){await page.locator('#next').click();await until(async()=>new RegExp(`^${n+2} /`).test(await page.locator('#page').innerText()),'page did not advance');}
  assert.match(await page.locator('#page').innerText(),/^5 \/ /);assert.equal(await page.locator('.board-card').count(),50);
  assert.ok(!requests.includes('/api/run') && !requests.some(p=>p.includes('artifact')),'board eagerly fetched Run details or artifacts');
  checks.push('A7/A9: complete history >200, global filter, page 5, equal-title independent cards, compact metadata only');
  const grid=page.locator('.board-grid');await grid.evaluate(e=>{e.scrollLeft=e.scrollWidth;});
  await page.evaluate(()=>scrollTo(0,0));await page.screenshot({path:path.join(target,'board.png'),fullPage:true});
  for(const tab of ['workflow','facts','journal']) {
   const card=page.locator('.board-card').first();const link=card.locator(`nav a[href*="tab=${tab}"]`);await link.scrollIntoViewIfNeeded();
   const position=await page.evaluate(()=>({y:scrollY,x:document.querySelector('.board-grid').scrollLeft}));const run=await card.getAttribute('data-run-id');
   await link.click();await until(async()=>!await page.locator('#detail').isHidden() && await page.locator(`[data-tab=${tab}]`).getAttribute('aria-pressed')==='true','detail tab was not selected');await until(async()=>!(await page.locator('#detail-title').innerText()).includes('Загрузка'),'detail never loaded');
   assert.equal(new URLSearchParams(new URL(page.url()).hash.slice(1)).get('run'),run);
   await page.locator('#back-to-runs').click();await until(async()=>!await page.locator('#run-list').isHidden(),'back did not restore list');assert.equal(await page.locator('[name=view]').inputValue(),'board');assert.equal(await page.locator('[name=status]').inputValue(),'completed');assert.match(await page.locator('#page').innerText(),/^5 \/ /);
   const returned=await page.evaluate(()=>({y:scrollY,x:document.querySelector('.board-grid').scrollLeft}));assert.ok(Math.abs(returned.y-position.y)<8,`vertical scroll lost: ${JSON.stringify({position,returned})}`);assert.ok(Math.abs(returned.x-position.x)<8,'board horizontal scroll lost');
  }
  checks.push('A8: graph/facts/journal links and back preserve view, filter, page and both scroll positions');
  // A real new Run changes the hidden list while a different Run's details
  // are open. Column scroll must survive the resulting DOM replacement.
  await search(cli.partial);await page.locator('[name=q]').fill('');await until(async()=>await page.locator('.board-card').count()===50 && /^1 \/ /.test(await page.locator('#page').innerText()),'first page did not return');
  const column=page.locator('.board-column[data-category=terminal]');await column.evaluate(e=>e.scrollTop=600);
  const detailLink=column.locator('.board-card').nth(1).locator('nav a[href*="tab=facts"]');await detailLink.scrollIntoViewIfNeeded();const columnPosition=await column.evaluate(e=>e.scrollTop);assert.ok(columnPosition>0);
  await detailLink.click();await until(async()=>!await page.locator('#detail').isHidden() && !(await page.locator('#detail-title').innerText()).includes('Загрузка'),'details did not load');
  const total=(await listing()).total;
  command('live','run','start','--workflow','workflows/shell.json','--brief','brief.json','--command-id','command:board-detail-update','--drive');
  await until(async()=>(await listing()).total===total+1,'new live Run did not enter catalog');await page.waitForTimeout(3000);
  await page.locator('#back-to-runs').click();await until(async()=>!await page.locator('#run-list').isHidden(),'updated board did not return');assert.ok(Math.abs(await column.evaluate(e=>e.scrollTop)-columnPosition)<8,'hidden polling lost column scroll');
  checks.push('A8: real Run arrival during details preserves column scroll after hidden-list DOM replacement');
  await page.locator('[name=status]').selectOption('');await search(cli.next);
  for(const view of ['flat','related','related-newest']) {await page.locator('[name=view]').selectOption(view);await until(async()=>!await page.locator('.table-wrap').isHidden(),'table view did not return');assert.ok(await page.locator('#runs tr').count()>0);}
  await until(async()=>await page.locator('[data-run-depth="1"]').count()>0,'related indentation fixture missing');
  assert.equal(await page.locator('[data-run-depth="1"]').first().evaluate(e=>getComputedStyle(e).getPropertyValue('--run-depth')),'1');
  assert.deepEqual(errors,[]);assert.deepEqual(consoleErrors,[]);await page.screenshot({path:path.join(target,'table.png')});
  fs.writeFileSync(path.join(target,'browser.json'),JSON.stringify({outcome:'passed',checks,console_page_errors:errors,console_errors:consoleErrors,card_population:(await listing()).total,artifacts:['board.png','parallel.png','table.png','trace.zip']},null,2));
 } finally {await context.tracing.stop({path:path.join(target,'trace.zip')});await browser.close();}
})().catch(err=>{console.error(err);process.exitCode=1;});
