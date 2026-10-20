#!/usr/bin/env python3
"""UI integration tests; all app inventory, native operations and icons are simulated.
No test images are vendor lookalikes, or shipped as application replacements.
"""
import json,shutil
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];UI=ROOT/'internal/server/ui';checks=[]
def passed(s):
 checks.append({'name':s,'passed':True});print('PASS',s,flush=True)
with sync_playwright() as pw:
 browser=pw.chromium.launch(executable_path=shutil.which('chromium'),headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':940,'height':640},color_scheme='dark')
 page.set_default_timeout(30000)
 errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 html=(UI/'index.html').read_text().replace('<link rel="stylesheet" href="style.css">','').replace('<script src="app.js"></script>','')
 page.set_content(html);page.add_style_tag(content=(UI/'style.css').read_text())
 page.evaluate(r'''() => {
  const list=[['p','PhotoCraft','photocraft','Adobe Photoshop'],['v','VectorCraft','vectorcraft','Adobe Illustrator'],['f','FigCraft','figcraft','Figma'],['l','LightCraft','lightcraft','Adobe Lightroom'],['w','WordCraft','wordcraft','Microsoft Word'],['s','SoundCraft','soundcraft','Pro Tools'],['n','FutureCraft','futurecraft','']];
  window.fixture={apps:list.map(([id,name,craftId,equivalent])=>({id,name,craftId,equivalent,bundleId:'ai.storyteller.'+craftId,path:'/Applications/'+name+'.app',version:'1.2',iconStamp:'v1'})),assets:{},originals:{},choices:{},records:[],renames:[],calls:[],theme:'system',scanDelay:600,originalDelay:1500,failOriginal:false,failApply:'',cancelCustom:false};
  fixture.apps.push({id:'donor',name:'Original Installed Editor',path:'/Applications/Original Installed Editor.app',bundleId:'com.vendor.original',iconStamp:'v1'});
  const svg='data:image/svg+xml;base64,'+btoa('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect x="3" y="3" width="58" height="58" rx="14" fill="#4477dd"/><path d="M20 20h24v24H20z" fill="none" stroke="#cee6ff" stroke-width="3"/></svg>');
  const src=Object.getOwnPropertyDescriptor(HTMLImageElement.prototype,'src');
  Object.defineProperty(HTMLImageElement.prototype,'src',{get:src.get,set:function(v){src.set.call(this,v.startsWith('api/icon')?svg:v);}});
  function original(a){const asset={id:'original-'+a.id,name:a.equivalent,source:'OFFLINE TEST FIXTURE'};fixture.assets[asset.id]=asset;fixture.originals[a.craftId]={assetId:asset.id,product:a.equivalent,source:'https://www.adobe.com/test-original.svg'};return {asset,product:a.equivalent,source:'https://www.adobe.com/test-original.svg',status:'fresh'};}
  for(const a of fixture.apps){if(a.equivalent)original(a);}
  const sleep=(ms,signal)=>new Promise((resolve,reject)=>{const timer=setTimeout(resolve,ms);signal?.addEventListener('abort',()=>{clearTimeout(timer);reject(new DOMException('Aborted','AbortError'));},{once:true});});
  fixture.firstRowAt=0;new MutationObserver(()=>{if(!fixture.firstRowAt&&document.querySelector('[data-app=p]')){fixture.firstRowAt=performance.now();window.__firstImage=document.querySelector('[data-app=p] img');}}).observe(document.body,{childList:true,subtree:true});
  window.fetch=async(url,opts={})=>{
   const name=url.replace(/^api\//,'').split('?')[0],body=JSON.parse(opts.body||'{}');fixture.calls.push({name,body,time:performance.now()});let result={},ok=true;
   if(name==='session')result={version:'0.4.1 · UI TEST DATA',...fixture};
   else if(name==='scan'){await sleep(fixture.scanDelay,opts.signal);result={apps:fixture.apps,warnings:[]};}
   else if(name==='original'){
    await sleep(fixture.originalDelay,opts.signal);const a=fixture.apps.find(a=>a.id===body.appId);
    if(!a?.equivalent){ok=false;result={error:'Unknown counterpart; custom icon remains available.'};}
    else if(fixture.failOriginal){result={...original(a),status:'cached'};}
    else if(a.id==='p')result={ticket:'p-ticket',product:a.equivalent,source:'https://www.adobe.com/test-original.svg',dataURL:svg};
    else result=original(a);
   }else if(name==='original/commit'){
    if(!body.png.startsWith('data:image/png;base64,'))throw Error('SVG not rasterized');result=original(fixture.apps.find(a=>a.id==='p'));
   }else if(name==='theme'){fixture.theme=body.theme;result={ok:true};}
   else if(name==='apply'){
    if(!body.confirmed)throw Error('not explicit');
    if(body.appId===fixture.failApply){ok=false;result={error:'Simulated native permission error: operation not permitted'};}
    else{fixture.records.push({id:'record-'+fixture.records.length,appId:body.appId,status:'applied'});result={status:'applied'};}
   }else if(name==='restore-app'){
    fixture.records=fixture.records.filter(r=>r.appId!==body.appId);const a=fixture.apps.find(a=>a.id===body.appId);if(a.renamed){a.name=a.beforeName;a.renamed=false;}result={restored:1};
   }else if(name==='restore-all'){
    result={restored:new Set(fixture.records.map(r=>r.appId)).size,errors:[]};fixture.records=[];fixture.apps.forEach(a=>{if(a.renamed){a.name=a.beforeName;a.renamed=false;}});
   }else if(name==='custom'||name==='donor-choice'){
    if(name==='custom'&&fixture.cancelCustom)result={cancelled:true};else{const a=fixture.apps.find(a=>a.id===body.appId);const asset={id:'custom-'+a.id,name:'Selected custom artwork',source:'SIMULATED LOCAL ICON'};fixture.assets[asset.id]=asset;fixture.choices[a.bundleId]=asset.id;result={asset};}
   }else if(name==='choice'){
    const a=fixture.apps.find(a=>a.id===body.appId);if(body.assetId)fixture.choices[a.bundleId]=body.assetId;else delete fixture.choices[a.bundleId];result={ok:true};
   }else if(name==='rename'){
    const a=fixture.apps.find(a=>a.id===body.appId);if(body.name.includes('/')){ok=false;result={error:'Name cannot contain slashes'};}else{a.beforeName=a.beforeName||a.name;a.name=body.name;a.renamed=true;result={app:a};}
   }else if(name==='restore-name'){
    const a=fixture.apps.find(a=>a.id===body.appId);a.name=a.beforeName;a.renamed=false;result={app:a};
   }else if(name==='root')result={cancelled:true};
   return {ok,json:async()=>JSON.parse(JSON.stringify(result))};
  };
 }''')
 page.add_script_tag(content=(UI/'app.js').read_text())
 page.wait_for_selector('[data-app=p]')
 assert page.locator('[data-app=p] .replacement-name').inner_text()=='Adobe Photoshop'
 # The delayed source hasn't resolved, but cached rows have already rendered.
 assert page.evaluate("fixture.firstRowAt>0 && (!fixture.calls.find(c=>c.name==='original/commit') || fixture.firstRowAt<fixture.calls.find(c=>c.name==='original/commit').time)")
 passed('Cached apps and originals render before delayed native scan or network work finishes')
 assert page.locator('[data-app=p] .set').is_enabled()
 passed('Icon actions remain enabled while originals refresh in the background')
 assert page.locator('aside,nav,[role=tab]').count()==0
 passed('Compact single page with no management tabs')
 assert page.evaluate("document.documentElement.dataset.theme")=='dark'
 passed('Default appearance follows dark macOS preference')
 page.emulate_media(color_scheme='light')
 page.wait_for_function("document.documentElement.dataset.theme==='light'")
 passed('System mode tracks a live OS appearance change')
 page.get_by_role('button',name='Dark appearance',exact=True).click()
 page.emulate_media(color_scheme='light')
 assert page.evaluate("document.documentElement.dataset.theme")=='dark'
 assert page.evaluate('fixture.theme')=='dark'
 passed('Manual dark override is saved and does not follow the OS until System is restored')
 page.get_by_role('button',name='Light appearance',exact=True).click()
 assert page.evaluate("document.documentElement.dataset.theme")=='light'
 page.get_by_role('button',name='System appearance',exact=True).click()
 passed('Light, Dark and System controls work without a settings page')
 page.wait_for_function("!document.querySelector('#scan').disabled")
 assert page.locator('[data-app=p] .icon-button').inner_text()==''
 page.evaluate("window.__keptImage=document.querySelector('[data-app=p] img')")
 page.wait_for_timeout(4500)
 assert page.evaluate("window.__firstImage===document.querySelector('[data-app=p] img') && window.__keptImage===window.__firstImage")
 passed('Async source updates preserve existing row and image DOM nodes')
 assert page.evaluate("fixture.calls.some(c=>c.name==='original/commit')")
 passed('Original SVG is converted to PNG rather than redrawn')
 assert page.locator('[data-app=n] .set').is_disabled()
 passed('Unknown products never receive an invented vendor icon')
 assert not page.evaluate("fixture.calls.some(c=>c.name==='apply')")
 passed('Scanning, source checks and previews never change installed apps')
 page.locator('[data-app=p] .icon-action[aria-label="Choose custom icon"]').click()
 assert page.locator('#picker').is_visible()
 assert page.locator('#picker-grid .icon-tile').count()==8
 assert page.locator('#picker').bounding_box()['width']<=540
 assert len(browser.contexts[0].pages)==1
 passed('App selection is a compact in-window icon grid, not a new native app window')
 page.locator('#picker-search').fill('Original Installed')
 assert page.locator('#picker-grid .icon-tile').count()==1
 page.locator('#picker-grid .icon-tile').click()
 page.wait_for_function("!document.querySelector('#picker').open")
 assert page.locator('[data-app=p] .replacement-name').inner_text()=='Custom icon'
 assert page.evaluate("fixture.calls.filter(c=>c.name==='donor-choice').at(-1).body.donorId")=='donor'
 passed('Donor selection copies the selected preview without opening the donor app')
 page.locator('[data-app=p] .set').click();page.wait_for_function("document.querySelector('#notice').textContent.includes('1 icon replaced')")
 assert page.evaluate("fixture.calls.filter(c=>c.name==='apply').at(-1).body.assetId")=='custom-p'
 passed('Individual Replace applies exactly the selected custom asset')
 page.locator('[data-app=p] .icon-action[aria-label="Restore this app"]').click()
 page.wait_for_function("document.querySelector('#notice').textContent.includes('Restored PhotoCraft')")
 passed('Individual Restore works from the same row')
 page.locator('[data-app=p] .icon-action[aria-label="Choose custom icon"]').click()
 page.locator('#picker-original .original-tile').click()
 page.wait_for_function("!document.querySelector('#picker').open")
 assert page.locator('[data-app=p] .replacement-name').inner_text()=='Adobe Photoshop'
 passed('Original artwork can be selected again after using a custom icon')
 page.evaluate('fixture.cancelCustom=true')
 page.locator('[data-app=v] .icon-action[aria-label="Choose custom icon"]').click();page.locator('#browse').click()
 page.wait_for_function("!document.querySelector('#browse').disabled")
 assert page.locator('[data-app=v] .replacement-name').inner_text()=='Adobe Illustrator'
 page.locator('#picker-close').click()
 passed('Cancelling a file choice preserves the previous original icon')
 page.locator('[data-app=p] .icon-action[aria-label="Rename app"]').click()
 page.locator('#name-original').click()
 assert page.locator('#name-input').input_value()=='Adobe Photoshop'
 page.locator('#name-save').click();page.wait_for_function("!document.querySelector('#rename-dialog').open")
 assert page.locator('[data-app=p] .app-name').inner_text()=='Adobe Photoshop'
 passed('Rename presets use the original application name')
 page.locator('[data-app=p] .icon-action[aria-label="Rename app"]').click();page.locator('#name-input').fill('Gav’s Photo Editor');page.locator('#name-save').click();page.wait_for_function("!document.querySelector('#rename-dialog').open")
 assert page.locator('[data-app=p] .app-name').inner_text()=='Gav’s Photo Editor'
 passed('Custom Unicode display names are accepted and shown in the row')
 page.locator('[data-app=p] .icon-action[aria-label="Rename app"]').click();page.locator('#name-input').fill('../Bad Name');page.locator('#name-save').click()
 page.wait_for_selector('#rename-error:visible')
 assert 'slash' in page.locator('#rename-error').inner_text()
 page.locator('#name-restore').click();page.wait_for_function("!document.querySelector('#rename-dialog').open")
 assert page.locator('[data-app=p] .app-name').inner_text()=='PhotoCraft'
 passed('Rename errors stay in the small dialog and Restore name returns the original')
 page.locator('#search').fill('Photo')
 assert page.locator('.app-row:visible').count()==1
 page.locator('#select-all').check()
 assert page.locator('.app-row input:checked').count()==1
 page.locator('#replace').click();page.wait_for_function("document.querySelector('#notice').textContent.includes('1 icon replaced')")
 passed('Filtered Select all and Replace affect only visible eligible rows')
 page.locator('#search').fill('');page.locator('#select-all').check()
 assert page.locator('.app-row input:checked').count()==6
 page.locator('#replace').click();page.wait_for_function("document.querySelector('#notice').textContent.includes('6 icons replaced')")
 passed('Bulk replacement applies all eligible app selections')
 page.locator('#restore-all').click();page.wait_for_function("document.querySelector('#notice').textContent.includes('6 apps restored')")
 assert page.locator('#restore-all').is_disabled()
 passed('Restore all remains a single main-screen action')
 page.evaluate("fixture.failApply='v'")
 page.locator('[data-app=v] .set').click();page.wait_for_selector('[data-app=v] .row-error:visible')
 assert 'operation not permitted' in page.locator('[data-app=v] .row-error').inner_text()
 assert 'operation not permitted' in page.locator('#notice').inner_text()
 passed('Exact replacement failure is visible in the affected row and message banner')
 page.evaluate("fixture.failApply='';fixture.failOriginal=true;fixture.scanDelay=0;fixture.originalDelay=10")
 page.locator('#scan').click();page.wait_for_timeout(600)
 assert 'offline' in page.locator('[data-app=p] .row-note').inner_text()
 assert page.locator('[data-app=p] .set').is_enabled()
 passed('Network failure preserves usable saved originals without blocking controls')
 page.evaluate("fixture.apps[0].name='<img src=x onerror=window.bad=true>'")
 page.locator('#scan').click();page.wait_for_timeout(400)
 assert page.evaluate('window.bad===undefined')
 assert '<img' in page.locator('[data-app=p] .app-name').inner_text()
 passed('App names are rendered as text, not executable markup')
 page.evaluate("fixture.apps[0].name='PhotoCraft';fixture.failOriginal=false")
 page.locator('#scan').click();page.wait_for_timeout(400)
 # screenshots are test fixtures; labels do not claim these are real vendor logos.
 page.get_by_role('button',name='Dark appearance',exact=True).click()
 for width in [720,800,940,1200]:
  page.set_viewport_size({'width':width,'height':640});page.wait_for_timeout(30)
  assert page.evaluate('document.documentElement.scrollWidth<=innerWidth'),width
  assert page.locator('[data-app=p]').bounding_box()['height']<=64
 passed('Compact rows fit 720, 800, 940 and 1200px windows without horizontal scrolling')
 page.set_viewport_size({'width':940,'height':740});page.locator('#dismiss').click() if page.locator('#dismiss').is_visible() else None
 page.wait_for_timeout(200);page.screenshot(path=str(ROOT/'docs/v04-ui-dark-fixtures.png'),full_page=True)
 page.get_by_role('button',name='Light appearance',exact=True).click();page.wait_for_timeout(200);page.screenshot(path=str(ROOT/'docs/v04-ui-light-fixtures.png'),full_page=True)
 page.locator('[data-app=p] .icon-action[aria-label="Choose custom icon"]').click();page.screenshot(path=str(ROOT/'docs/v04-ui-picker-fixtures.png'),full_page=True)
 assert not errors,errors
 passed('No JavaScript runtime errors in all exercised workflows')
 browser.close()
report={'environment':'Linux Chromium; native operations, installed applications and artwork simulated. Not macOS acceptance or publisher availability testing.','passed':len(checks),'checks':checks}
(ROOT/'docs/v04-ui-results.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
