'use strict';
const $=id=>document.getElementById(id);
/* ---- localisation ---- strings.js (served per system language by the
   Go side) defines window.ICONCRAFT_I18N; English is always complete. */
const I18N=window.ICONCRAFT_I18N||{locale:'en',strings:{}};
const LOCALE=I18N.locale||'en';
/* Dates follow the macOS Region setting (e.g. en-AU → 22/10/2026), which Go
   sends separately from the UI language; a tag Intl rejects falls back to
   the UI locale, then to the browser default. */
const FORMAT_LOCALE=(()=>{for(const tag of [I18N.format,LOCALE]){if(!tag)continue;try{new Intl.DateTimeFormat(tag);return tag;}catch(_){}}return undefined;})();
function t(key,vars){let s=I18N.strings[key];if(s===undefined)s=key;if(vars)for(const k in vars)s=s.split('{'+k+'}').join(String(vars[k]));return s;}
function tn(n,key,vars){return t(key+(n===1?'.one':'.other'),Object.assign({n},vars||{}));}
function fmtDate(v){const d=new Date(v);if(Number.isNaN(d.getTime()))return String(v);try{return d.toLocaleString(FORMAT_LOCALE);}catch(_){return d.toLocaleString();}}
function rich(el,s){el.replaceChildren();s.split('**').forEach((part,i)=>{if(!part)return;if(i%2)el.append(node('strong','',part));else el.append(document.createTextNode(part));});}
function localizeDocument(){
 if(!window.ICONCRAFT_I18N)return; /* no catalogue (dev harness): keep the authored English */
 document.documentElement.lang=LOCALE;
 const attrs={'data-i18n-title':'title','data-i18n-aria':'aria-label','data-i18n-placeholder':'placeholder','data-i18n-alt':'alt'};
 document.querySelectorAll('[data-i18n]').forEach(el=>{el.textContent=t(el.dataset.i18n);});
 document.querySelectorAll('[data-i18n-rich]').forEach(el=>rich(el,t(el.dataset.i18nRich)));
 for(const a in attrs)document.querySelectorAll('['+a+']').forEach(el=>el.setAttribute(attrs[a],t(el.getAttribute(a))));
}
const model={apps:[],assets:{},originals:{},choices:{},records:[],renames:[],rebrands:{},rebrandRuntime:{},vault:{},keepAlive:false,setupRequired:false,settings:null,loginAgent:false};
const rows=new Map(), rowState=new Map(), selected=new Set();
let scanning=false, busy=false, firstLoad=true, selectFuture=false, query='',theme='system',themeTouched=false;
let generation=0,workerCount=0,queue=[],seen=new Set(),controllers=new Set(),pickerApp=null,renameApp=null;
let setupData=null,setupChecking=false,setupOpening=false,setupWanted=false;
let tab='apps',defaultsData=null,defaultsBusy=false;
/* Sort/group persist server-side (the journal): this page is served from
   a fresh loopback port every launch, so browser storage never survives. */
let organize={sort:'name-asc',group:'none'};
let groupHeadNodes=[];
const media=window.matchMedia('(prefers-color-scheme: dark)');
const paths={grid:'M3 3h5v5H3zM12 3h5v5h-5zM3 12h5v5H3zM12 12h5v5h-5z',restore:'M4 7a6 6 0 111 8M1.5 3v5h5',edit:'m4 13-1 4 4-1L17 6l-3-3zM12 5l3 3',arrow:'M4 10h12m-4-4 4 4-4 4',refresh:'M16 7A6 6 0 004 5L2 7m0-4v4h4M4 13a6 6 0 0012 2l2-2m0 4v-4h-4'};
function svg(key){const s=document.createElementNS('http://www.w3.org/2000/svg','svg');s.setAttribute('viewBox','0 0 20 20');s.setAttribute('aria-hidden','true');const p=document.createElementNS(s.namespaceURI,'path');p.setAttribute('d',paths[key]||paths.grid);s.append(p);return s;}
function node(tag,cls,text){const e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e;}
function button(label,cls,fn,icon){const b=node('button',cls);b.title=label;b.setAttribute('aria-label',label);if(icon)b.append(svg(icon));else b.textContent=label;b.addEventListener('click',fn);return b;}
function compareNames(a,b){try{return a.localeCompare(b,FORMAT_LOCALE||LOCALE);}catch(_){return a.localeCompare(b);}}
const SEP=' · ';
async function api(path,body,signal){const opts={signal};if(body!==undefined){opts.method='POST';opts.headers={'Content-Type':'application/json'};opts.body=JSON.stringify(body);}const res=await fetch('api/'+path,opts);let data;try{data=await res.json();}catch(_){throw Error(t('api.unreadable'));}if(!res.ok)throw Error(data.error||t('api.failed'));return data;}
function mentionsAppManagement(text){return /App Management/.test(text||'')||(text||'').includes(t('term.appManagement'));}
function tell(text,error=false){$('notice-text').textContent=text;$('notice').classList.toggle('error',error);$('notice').hidden=!text;const act=$('notice-action');if(act)act.hidden=!(error&&mentionsAppManagement(text));}
function rs(a){if(!rowState.has(a.id))rowState.set(a.id,{checking:false,error:'',note:'',working:false,revision:0});return rowState.get(a.id);}
function chosen(a){return model.choices[a.bundleId]||model.originals[a.craftId]?.assetId||'';}
function ready(a){return !a.protected&&!!model.assets[chosen(a)];}
function live(r){return ['applied','prepared','rollback-required'].includes(r.status);}
function active(a){return a.renamed||model.records.some(r=>r.appId===a.id&&live(r));}
/* Every installed app is listed with its real icon. Apps with a known
   original counterpart get background original-artwork checks; every app can
   still be given any installed app's icon, a custom image, or a new name. */
function compareApps(a,b){
 if(organize.sort==='name-desc')return compareNames(b.name,a.name);
 if(organize.sort==='status'){const d=Number(active(b))-Number(active(a));if(d)return d;const r=Number(ready(b))-Number(ready(a));if(r)return r;}
 return compareNames(a.name,b.name);
}
function relevant(){return [...model.apps].sort(compareApps);}
function groupOf(a){
 if(organize.group==='status')return active(a)?t('group.customised'):ready(a)?t('group.originalReady'):t('group.noOriginal');
 if(organize.group==='location')return a.path.replace(/\/[^/]*\.app$/i,'')||t('group.applications');
 return '';
}
function visible(){return relevant().filter(a=>(a.name+' '+a.equivalent+' '+a.craftId+' '+(model.originals[a.craftId]?.product||'')).toLowerCase().includes(query));}
function sourceStatus(a){const v=model.originals[a.craftId];return v?.source||'';}
function appIcon(a){return 'api/icon?app='+encodeURIComponent(a.id)+'&v='+encodeURIComponent((a.iconStamp||'1')+'-'+rs(a).revision);}
function assetIcon(id){return id?'api/icon?asset='+encodeURIComponent(id):'';}
function imageView(small=false){const wrap=node('span','icon-wrap'+(small?' small':''));const f=svg('grid');f.classList.add('fallback');const img=node('img','icon');img.alt='';/* NOT lazy: icons are display:none until load, and WebKit never fetches hidden lazy images — the load event that reveals them would never fire. Local thumbnails are cheap; load eagerly. */img.decoding='async';wrap.append(f,img);img.addEventListener('load',()=>wrap.classList.add('has-image'));img.addEventListener('error',()=>wrap.classList.remove('has-image'));return {wrap,img,src:''};}
function setImage(view,src,alt){view.img.alt=alt;if(view.src===src)return;view.src=src;view.wrap.classList.remove('has-image');if(src)view.img.src=src;else view.img.removeAttribute('src');}
function updateModel(data){if(data.organize){organize={sort:data.organize.sort||'name-asc',group:data.organize.group||'none'};const so=$('sort'),gr=$('group');if(so)so.value=organize.sort;if(gr)gr.value=organize.group;}model.apps=data.apps||model.apps;Object.assign(model.assets,data.assets||{});Object.assign(model.originals,data.originals||{});model.choices=data.choices||{};model.records=data.records||[];model.renames=data.renames||[];if(typeof data.keepAlive==='boolean')model.keepAlive=data.keepAlive;if(typeof data.setupRequired==='boolean')model.setupRequired=data.setupRequired;if(data.settings)model.settings=data.settings;if(typeof data.loginAgent==='boolean')model.loginAgent=data.loginAgent;model.rebrands=data.rebrands||{};model.rebrandRuntime=data.rebrandRuntime||{};model.vault=data.vault||{};$('version').textContent=data.version||'0.13.1';const v2=$('version-2');if(v2)v2.textContent=data.version||'0.13.1';selected.forEach(id=>{if(!model.apps.some(a=>a.id===id))selected.delete(id);});}
async function session(){updateModel(await api('session'));render();}
/* Restores and full skins change names written inside app bundles; the
   inventory only re-reads those on a scan, so follow such actions with
   one instead of trusting the cached session. */
async function rescan(){try{await api('scan',{});}catch(_){}await session();}
function makeRow(a){
 const currentApp=()=>model.apps.find(x=>x.id===a.id)||a;
 const root=node('div','app-row');root.dataset.app=a.id;
 const check=node('input');check.type='checkbox';check.addEventListener('change',()=>{selectFuture=false;if(check.checked)selected.add(a.id);else selected.delete(a.id);render();});
 const current=imageView(),target=imageView();
 const left=node('div','app-cell'),copy=node('div','app-copy'),name=node('div','app-name'),sub=node('div','subtitle');const orig=imageView(true),origLine=node('div','orig-line'),origText=node('span','orig-text');origLine.append(orig.wrap,origText);origLine.hidden=true;copy.append(name,sub,origLine);left.append(current.wrap,copy);
 const arr=svg('arrow');arr.classList.add('arrow');
 const right=node('div','app-cell'),targetButton=button(t('row.chooseIcon'),'icon-button',()=>openPicker(currentApp()));targetButton.replaceChildren(target.wrap);
 const detail=node('div','app-copy'),title=node('div','replacement-name'),note=node('div','row-note');detail.append(title,note);right.append(targetButton,detail);
 const actions=node('div','row-actions');const custom=button(t('row.chooseCustomIcon'),'icon-action',()=>openPicker(currentApp()),'grid');const rename=button(t('row.rename'),'icon-action',()=>openRename(currentApp()),'edit');const restore=button(t('row.restore'),'icon-action',()=>restoreOne(currentApp()),'restore');const set=button(t('row.replace'),'set',()=>applyApps([currentApp()]));
 actions.append(custom,rename,restore,set);const error=node('div','row-error');error.hidden=true;root.append(check,left,arr,right,actions,error);
 return {root,check,current,target,name,sub,orig,origLine,origText,title,note,targetButton,custom,rename,restore,set,error};
}
function subtitleFor(a){
 const iconChanged=model.records.some(r=>r.appId===a.id&&live(r));
 const parts=[];
 if(active(a)){if(a.renamed){parts.push(t('row.renamed'));if(iconChanged)parts.push(t('row.iconChanged'));}else parts.push(t('row.iconChanged'));}
 else parts.push(a.version?t('row.version',{version:a.version}):t('row.installed'));
 if(model.rebrands[a.bundleId])parts.push(t('row.fullSkin')+(model.rebrandRuntime[a.bundleId]==='own'?' '+t('row.ownIcon'):''));
 if(model.vault[a.bundleId])parts.push(t('row.originalSaved'));
 return parts.join(SEP);
}
function updateRow(a,v){
 const st=rs(a),id=chosen(a),custom=!!model.choices[a.bundleId],orig=model.originals[a.craftId];
 v.root.classList.toggle('selected',selected.has(a.id));v.check.checked=selected.has(a.id);v.check.disabled=busy||!ready(a);v.check.setAttribute('aria-label',t('row.selectAria',{name:a.name}));
 v.name.textContent=a.name;v.name.title=a.path;v.sub.textContent=subtitleFor(a);v.sub.classList.toggle('changed',active(a));
 /* Once an app wears a new identity, the installed column alone no longer
    says what the app really is — surface the saved underlying original. */
 const saved=model.vault[a.bundleId],origName=saved?.appName||model.renames.find(r=>r.bundleId===a.bundleId)?.oldName||'';
 const showOrig=!!(origName&&origName!==a.name&&(a.renamed||model.rebrands[a.bundleId]));
 v.origLine.hidden=!showOrig;
 if(showOrig){v.origText.textContent=t('row.originally',{name:origName});v.origText.title=t('row.originallyTitle',{name:origName});setImage(v.orig,saved?.iconAssetId?assetIcon(saved.iconAssetId):'',t('row.originalIconAlt',{name:origName}));}
 setImage(v.current,appIcon(a),t('row.currentIconAlt',{name:a.name}));
 /* With no mapped/custom target yet, preview the app's own icon dimmed so
    every row shows real artwork instead of an empty placeholder. */
 setImage(v.target,id?assetIcon(id):appIcon(a),t('row.iconAlt',{name:custom?t('row.custom'):orig?.product||a.equivalent||t('row.original')}));
 v.target.wrap.classList.toggle('dim',!id);
 v.title.textContent=custom?t('row.customIcon'):orig?.product||a.equivalent?.replace(' (suggested)','')||(a.craftId?t('row.finding'):t('row.noMapped'));v.title.title=custom?model.assets[id]?.name||t('row.customIcon'):sourceStatus(a);
 let note=custom?t('row.yourChoice'):st.checking?(id?t('row.savedChecking'):t('row.finding')):st.note||(id?t('row.originalReady'):a.craftId?t('row.chooseCustom'):t('row.pickAny'));
 if(st.error&&!id&&!custom)note=t('row.notFound');v.note.textContent=note;v.note.title=st.error||sourceStatus(a);v.note.className='row-note'+(st.checking&&!custom?' checking':'')+(st.error&&!id?' error':'');
 v.targetButton.disabled=busy;v.custom.disabled=busy;v.rename.disabled=busy||a.protected;v.restore.disabled=busy||!active(a);v.set.disabled=busy||!ready(a);v.set.textContent=st.working?t('row.working'):t('row.replace');
 v.error.textContent=st.actionError||'';v.error.hidden=!st.actionError;
}
function render(){
 const all=relevant(),filtered=visible(),visibleIDs=new Set(filtered.map(a=>a.id)),allIDs=new Set(all.map(a=>a.id));
 for(const [id,v]of rows){if(!allIDs.has(id)){v.root.remove();rows.delete(id);}}
 for(const h of groupHeadNodes)h.remove();groupHeadNodes=[];
 const sequence=[],groupCounts={};let lastGroup=null;
 let ordered=all;
 if(organize.group!=='none'){
  for(const a of filtered){const g=groupOf(a);if(g)groupCounts[g]=(groupCounts[g]||0)+1;}
  /* Cluster by group (first-appearance order under the chosen sort), keeping
     the sort order inside each group: Array.sort is stable. A single reused
     header node per group would be moved by append() whenever the groups
     interleaved, stranding the first run of each group without a header. */
  const rank=new Map();
  for(const a of all){const g=groupOf(a);if(g&&!rank.has(g))rank.set(g,rank.size);}
  ordered=[...all].sort((x,y)=>(rank.get(groupOf(x))??0)-(rank.get(groupOf(y))??0));
 }
 for(const a of ordered){
  let v=rows.get(a.id);if(!v){v=makeRow(a);rows.set(a.id,v);}
  v.root.hidden=!visibleIDs.has(a.id);updateRow(a,v);
  if(v.root.hidden){sequence.push(v.root);continue;}
  const g=groupOf(a);
  if(g&&g!==lastGroup){
   const head=node('div','group-head',g+(groupCounts[g]?SEP+groupCounts[g]:''));
   groupHeadNodes.push(head);sequence.push(head);lastGroup=g;
  }
  sequence.push(v.root);
 }
 for(const n of sequence)$('apps').append(n);
 const eligible=filtered.filter(ready),count=eligible.filter(a=>selected.has(a.id)).length;
 $('select-all').disabled=busy||eligible.length===0;$('select-all').checked=eligible.length>0&&count===eligible.length;$('select-all').indeterminate=count>0&&count<eligible.length;
 $('replace').disabled=busy||count===0;$('selection-count').textContent=t('footer.selected',{n:count});$('restore-all').disabled=busy||!model.records.some(live)&&!model.apps.some(a=>a.renamed);
 $('scan').disabled=scanning||busy;$('scan').classList.toggle('refreshing',scanning);$('folder').disabled=busy||scanning;
 $('keepalive').textContent=t('footer.keepalive',{state:model.keepAlive?t('common.on'):t('common.off')});$('keepalive').classList.toggle('on',model.keepAlive);
 const checks=all.filter(a=>rs(a).checking).length;$('activity').classList.toggle('updating',checks>0||scanning);
 $('status').textContent=scanning?(all.length?tn(all.length,'status.scanningCount'):t('status.scanning')):checks?tn(all.length,'status.refreshing',{checks}):tn(all.length,'status.ready',{ready:all.filter(ready).length});
 $('loading').hidden=!firstLoad||all.length>0;$('empty').hidden=firstLoad||filtered.length>0;
}
function applyTheme(mode,persist=false){
 theme=['system','light','dark'].includes(mode)?mode:'system';document.documentElement.dataset.theme=theme==='system'?(media.matches?'dark':'light'):theme;
 document.querySelectorAll('[data-theme-choice]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.themeChoice===theme)));
 if(persist){themeTouched=true;api('theme',{theme}).catch(e=>tell(t('theme.saveFailed',{error:e.message}),true));}
}
media.addEventListener('change',()=>{if(theme==='system')applyTheme('system');});document.querySelectorAll('[data-theme-choice]').forEach(b=>b.addEventListener('click',()=>applyTheme(b.dataset.themeChoice,true)));
function rasterize(dataURL){return new Promise((resolve,reject)=>{
 const im=new Image();const timer=setTimeout(()=>reject(Error(t('icon.unreadable'))),5000);
 im.onload=()=>{clearTimeout(timer);try{const w=im.naturalWidth,h=im.naturalHeight;if(!w||!h||w>4096||h>4096||w/h<.6||w/h>1.6)throw Error(t('icon.notSquare'));const canvas=document.createElement('canvas');canvas.width=512;canvas.height=512;const ctx=canvas.getContext('2d');if(!ctx)throw Error(t('icon.decodeUnavailable'));const scale=512/Math.max(w,h);ctx.drawImage(im,(512-w*scale)/2,(512-h*scale)/2,w*scale,h*scale);resolve(canvas.toDataURL('image/png'));}catch(e){reject(e);}};
 im.onerror=()=>{clearTimeout(timer);reject(Error(t('icon.unreadable')));};im.src=dataURL;
 });}
async function fetchOriginal(a,run){
 const state=rs(a),controller=new AbortController();controllers.add(controller);
 try{let result;const exclude=[];for(let i=0;i<3;i++){
  result=await api('original',{appId:a.id,exclude},controller.signal);if(!result.ticket)break;
  try{const png=await rasterize(result.dataURL);result=await api('original/commit',{ticket:result.ticket,png},controller.signal);break;}
  catch(e){if(controller.signal.aborted||i===2)throw e;exclude.push(result.source);}
 }
 if(run!==generation||!result.asset)return;
 model.assets[result.asset.id]=result.asset;model.originals[a.craftId]={assetId:result.asset.id,product:result.product,source:result.source};
 state.error='';state.note=result.status==='fresh'?t('row.sourceChecked'):result.status==='installed'?t('row.fromInstalled'):t('row.savedOffline');
 if(selectFuture&&ready(a))selected.add(a.id);
 }catch(e){if(run===generation&&e.name!=='AbortError'){state.error=e.message;state.note=chosen(a)?t('row.savedOffline'):t('row.chooseCustom');}}
 finally{controllers.delete(controller);if(run===generation){state.checking=false;render();}}
}
function pump(){while(workerCount<3&&queue.length){const {a,run}=queue.shift();workerCount++;fetchOriginal(a,run).finally(()=>{workerCount--;pump();});}}
function enqueue(apps){for(const a of apps){if(!a.craftId||seen.has(a.id))continue;seen.add(a.id);rs(a).checking=true;queue.push({a,run:generation});}render();pump();}
async function refresh(){
 if(busy||scanning)return;generation++;controllers.forEach(c=>c.abort());queue=[];seen=new Set();rowState.forEach(s=>{s.checking=false;s.actionError='';});scanning=true;tell('');enqueue(relevant());render();
 api('refresh',{}).catch(()=>{});
 try{await api('scan',{});await session();firstLoad=false;enqueue(relevant());}
 catch(e){firstLoad=false;tell(e.message,true);}
 finally{scanning=false;render();}
}
async function applyApps(list){
 if(busy)return;const targets=list.filter(ready).map(a=>({a,asset:chosen(a)}));if(!targets.length)return;
 busy=true;render();let done=0;const errors=[];
 for(const {a,asset}of targets){const st=rs(a);st.working=true;st.actionError='';render();try{await api('apply',{appId:a.id,assetId:asset,confirmed:true});st.revision++;selected.delete(a.id);done++;}catch(e){st.actionError=e.message;errors.push(a.name+': '+e.message);}finally{st.working=false;}}
 try{await session();}catch(e){errors.push(e.message);}busy=false;selectFuture=false;
 tell(tn(done,'apply.done')+(errors.length?'\n'+errors.join('\n'):''),errors.length>0);render();
}
async function restoreOne(a){if(busy)return;busy=true;rs(a).actionError='';render();try{const r=await api('restore-app',{appId:a.id,confirmed:true});rs(a).revision++;await rescan();tell(r.steps&&r.steps.length?t('restore.doneSteps',{name:a.name,steps:r.steps.join('; ')}):t('restore.done',{name:a.name}));}catch(e){rs(a).actionError=e.message;tell(e.message,true);}finally{busy=false;render();}}
async function restoreAll(){if(busy)return;busy=true;render();try{const r=await api('restore-all',{confirmed:true});rowState.forEach(s=>{s.revision++;s.actionError='';});await rescan();tell(tn(r.restored,'restore.all')+(r.errors?.length?'\n'+r.errors.join('\n'):''),!!r.errors?.length);}catch(e){tell(e.message,true);}finally{busy=false;render();}}
function dialogError(id,text){$(id).textContent=text;$(id).hidden=!text;}
function setupMessage(text){const m=$('setup-message');m.textContent=text;m.hidden=!text;}
function setupCheck(id){return setupData?.checks?.find(c=>c.id===id);}
function setupActionButton(label,fn,primary=false){const b=node('button',primary?'primary':'secondary',label);b.disabled=setupChecking||busy;b.addEventListener('click',fn);return b;}
function drawSetup(){
 const checks=setupData?.checks||[],blocked=checks.filter(c=>c.state==='blocked').length,unchecked=checks.filter(c=>c.state==='unknown').length,ready=checks.filter(c=>c.state==='ready').length;
 $('setup-summary').textContent=blocked?tn(blocked,'setup.summaryBlocked'):unchecked?t('setup.summaryUnchecked'):tn(ready,'setup.summaryReady');
 const list=$('setup-checks');list.replaceChildren();
 for(const check of checks){
  const row=node('section','setup-row'),copy=node('div'),side=node('div','setup-side'),pill=node('span','status-pill '+check.state,check.label),actions=node('div','setup-actions');
  copy.append(node('h3','',check.title),node('p','',check.detail));
  if(check.id==='appManagement'&&setupData?.appAccess?.checkedAt){const when=new Date(setupData.appAccess.checkedAt);if(!Number.isNaN(when.getTime()))copy.append(node('p','setup-meta',t('setup.lastChecked',{when:fmtDate(when)})+(setupData.appAccess.appName?SEP+t('setup.testedApp',{name:setupData.appAccess.appName}):'')));}
  if(check.id==='appManagement'){
   actions.append(setupActionButton(t('setup.openAppManagement'),openSetupAppManagement),setupActionButton(check.state==='unknown'?t('setup.checkNow'):t('setup.checkAgain'),runSetupChecks,true));
  }else if(check.id==='background'){
   const on=!!setupData?.keeper?.desired;
   actions.append(setupActionButton(on?t('setup.keeperOff'):t('setup.keeperOn'),toggleSetupKeeper,on&&check.state!=='ready'));
  }
  side.append(pill,actions);row.append(copy,side);list.append(row);
 }
 $('setup-check').disabled=setupChecking||busy;$('setup-check').textContent=setupChecking?t('setup.checking'):t('setup.runChecks');
 $('setup-close').disabled=setupChecking;
 $('setup-later').disabled=setupChecking;$('setup-later').textContent=setupData?.setup?.completed?t('common.close'):t('setup.reviewLater');
 $('setup-finish').disabled=setupChecking;$('setup-finish').textContent=setupData?.setup?.completed?t('common.done'):t('setup.finish');
 $('setup-subtitle').textContent=setupData?.setup?.completed?t('setup.subtitleDone'):t('setup.subtitle');
}
async function openSetup(){
 if(setupOpening||$('setup-dialog').open)return;setupOpening=true;dialogError('setup-error','');setupMessage('');
 try{setupData=await api('permissions');drawSetup();$('setup-dialog').showModal();}
 catch(e){tell(t('setup.loadFailed',{error:e.message}),true);}
 finally{setupOpening=false;}
}
async function runSetupChecks(){
 if(setupChecking)return;setupChecking=true;dialogError('setup-error','');setupMessage('');drawSetup();
 try{setupData=await api('permissions/check',{});const access=setupCheck('appManagement');setupMessage(access?.state==='ready'?t('setup.checksPassed'):access?.state==='blocked'?t('setup.checksBlocked'):t('setup.checksReview'));}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();render();}
}
async function openSetupAppManagement(){
 dialogError('setup-error','');try{await api('open-settings',{target:'appManagement'});setupMessage(t('setup.settingsOpened'));}
 catch(e){dialogError('setup-error',e.message);}
}
async function toggleSetupKeeper(){
 if(setupChecking)return;const on=!setupData?.keeper?.desired;setupChecking=true;drawSetup();dialogError('setup-error','');
 try{const r=await api('keepalive',{enabled:on});model.keepAlive=!!r.enabled;setupData=await api('permissions');setupMessage(on?t('setup.keeperNowOn'):t('setup.keeperNowOff'));}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();render();}
}
async function finishSetup(){
 if(setupChecking)return;
 if(setupData?.setup?.completed){$('setup-dialog').close();return;}
 setupChecking=true;drawSetup();dialogError('setup-error','');
 try{await api('setup/complete',{confirmed:true});model.setupRequired=false;setupWanted=false;if(setupData?.setup)setupData.setup.completed=true;$('setup-dialog').close();tell(t('setup.complete'));}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();}
}
function closeSetup(){if(!setupChecking)$('setup-dialog').close();}
function closePicker(){if(!busy){$('picker').close();pickerApp=null;}}
function openPicker(a){if(busy)return;pickerApp=a;$('picker-for').textContent=t('picker.for',{name:a.name});$('picker-search').value='';dialogError('picker-error','');drawPicker();$('picker').showModal();$('picker-search').focus();}
function drawPicker(){
 if(!pickerApp)return;const a=pickerApp,original=model.originals[a.craftId];$('picker-original').replaceChildren();
 if(original?.assetId){const b=button(t('picker.useOriginal'),'original-tile',async()=>{await pickChoice(()=>api('choice',{appId:a.id,assetId:''}),()=>{delete model.choices[a.bundleId];});});const v=imageView(true);setImage(v,assetIcon(original.assetId),original.product);const tile=node('span');tile.append(node('span','',original.product),node('small','',t('picker.useDiscovered')));b.replaceChildren(v.wrap,tile);$('picker-original').append(b);}
 const q=$('picker-search').value.toLowerCase().trim(),frag=document.createDocumentFragment();
 for(const donor of model.apps.filter(d=>d.name.toLowerCase().includes(q)).sort((x,y)=>compareNames(x.name,y.name)).slice(0,200)){
  const b=button(t('picker.useFrom',{name:donor.name}),'icon-tile',()=>pickChoice(()=>api('donor-choice',{appId:a.id,donorId:donor.id}),r=>{model.assets[r.asset.id]=r.asset;model.choices[a.bundleId]=r.asset.id;}));
  const v=imageView(true);setImage(v,appIcon(donor),t('row.iconAlt',{name:donor.name}));b.replaceChildren(v.wrap,node('span','',donor.name));frag.append(b);
 }
 $('picker-grid').replaceChildren(frag);if(!frag.childNodes.length&&!$('picker-grid').children.length)$('picker-grid').append(node('p','row-note',t('picker.noMatches')));
}
async function pickChoice(action,update){if(busy)return;busy=true;$('picker').querySelectorAll('button').forEach(b=>b.disabled=true);render();try{const result=await action();if(!result.cancelled){update(result);$('picker').close();pickerApp=null;tell(t('picker.selected'));}}catch(e){dialogError('picker-error',e.message);}finally{busy=false;$('picker').querySelectorAll('button').forEach(b=>b.disabled=false);render();}}
$('browse').addEventListener('click',()=>{if(!pickerApp)return;const a=pickerApp;pickChoice(()=>api('custom',{appId:a.id}),r=>{model.assets[r.asset.id]=r.asset;model.choices[a.bundleId]=r.asset.id;});});
function vaultText(sv){
 if(!sv)return t('rename.vaultNone');
 return t('rename.vaultSaved',{name:sv.appName,folder:sv.folderName,internal:sv.internalName?t('rename.vaultInternal',{name:sv.internalName}):'',icon:sv.iconAssetId?t('rename.vaultIcon'):''});
}
function openRename(a){if(busy)return;renameApp=a;$('rename-for').textContent=a.name;$('name-input').value=a.name;$('name-original').disabled=!(model.originals[a.craftId]?.product||a.equivalent);$('name-restore').hidden=!a.renamed;dialogError('rename-error','');
 $('vault-state').textContent=vaultText(model.vault[a.bundleId]);
 const skinned=!!model.rebrands[a.bundleId];$('name-unskin').hidden=!skinned;$('fullskin-state').textContent=skinned?(model.rebrandRuntime[a.bundleId]==='own'?t('rename.skinOwn',{name:model.rebrands[a.bundleId]}):t('rename.skinApplied',{name:model.rebrands[a.bundleId]})):(chosen(a)?t('rename.usesChosen'):t('rename.chooseFirst'));
 const ib=$('name-internal');ib.dataset.armed='';ib.textContent=t('rename.applyFullSkin');ib.disabled=!chosen(a);
 $('rename-dialog').showModal();$('name-input').focus();$('name-input').select();}
function closeRename(){if(!busy){$('rename-dialog').close();renameApp=null;}}
async function saveName(restore=false){if(busy||!renameApp)return;const a=renameApp;busy=true;$('name-save').disabled=true;$('name-restore').disabled=true;render();try{const result=await api(restore?'restore-name':'rename',{appId:a.id,...(restore?{}:{name:$('name-input').value}),confirmed:true});await session();$('rename-dialog').close();renameApp=null;tell(t('rename.renamed',{name:result.app.name}));}catch(e){dialogError('rename-error',e.message);}finally{busy=false;$('name-save').disabled=false;$('name-restore').disabled=false;render();}}
$('name-internal').addEventListener('click',async()=>{if(busy||!renameApp)return;const a=renameApp,ib=$('name-internal');
 if(!ib.dataset.armed){ib.dataset.armed='1';ib.textContent=t('rename.confirmSkin',{name:a.name});
  /* The confirm label is taller (it wraps in CJK) and the sticky action
     footer overlays whatever sits under it mid-scroll, so bring the armed
     button fully above the footer (scroll-margin-bottom leaves room). */
  if(ib.scrollIntoView)ib.scrollIntoView({block:'end'});
  setTimeout(()=>{ib.dataset.armed='';ib.textContent=t('rename.applyFullSkin');},5000);return;}
 busy=true;ib.disabled=true;render();try{await api('rebrand',{appId:a.id,assetId:chosen(a),name:$('name-input').value,confirmed:true});await rescan();$('rename-dialog').close();renameApp=null;tell(t('rename.skinDone',{name:a.name}));}catch(e){dialogError('rename-error',e.message);}finally{busy=false;ib.disabled=false;ib.dataset.armed='';ib.textContent=t('rename.applyFullSkin');render();}});
$('name-unskin').addEventListener('click',async()=>{if(busy||!renameApp)return;const a=renameApp;busy=true;render();try{await api('restore-rebrand',{appId:a.id,confirmed:true});await rescan();$('rename-dialog').close();renameApp=null;tell(t('rename.unskinDone',{name:a.name}));}catch(e){dialogError('rename-error',e.message);}finally{busy=false;render();}});
$('name-original').addEventListener('click',()=>{if(renameApp)$('name-input').value=model.originals[renameApp.craftId]?.product||renameApp.equivalent.replace(' (suggested)','');});
$('name-save').addEventListener('click',()=>saveName());$('name-restore').addEventListener('click',()=>saveName(true));$('name-input').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();saveName();}});
$('rename-close').addEventListener('click',closeRename);$('rename-cancel').addEventListener('click',closeRename);$('picker-close').addEventListener('click',closePicker);$('picker-search').addEventListener('input',drawPicker);
for(const id of ['picker','rename-dialog']){$(id).addEventListener('cancel',e=>{if(busy)e.preventDefault();});$(id).addEventListener('click',e=>{if(e.target===$(id)&&!busy){const r=$(id).getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)$(id).close();}});}
async function openSupport(target){try{const r=await api('support',{target});if(!r.opened&&r.url)window.open(r.url,'_blank');else if(r.opened)tell(target==='paypal'?t('support.openingPaypal'):t('support.openingGithub'));}catch(e){tell((target==='paypal'?t('support.paypalFallback'):t('support.githubFallback')),true);}}
$('keepalive').addEventListener('click',async()=>{if(busy)return;busy=true;render();try{const r=await api('keepalive',{enabled:!model.keepAlive});model.keepAlive=!!r.enabled;tell(model.keepAlive?t('keepalive.on'):t('keepalive.off'));}catch(e){tell(e.message,true);}finally{busy=false;render();}});
function settingsMessage(text,error=false){const m=$('settings-message'),e=$('settings-error');if(error){e.textContent=text;e.hidden=false;m.hidden=true;}else{m.textContent=text;m.hidden=!text;e.hidden=true;}}
function currentSettings(){return model.settings||{launchAtLogin:false,startInMenuBar:false,showDockIcon:true,showMenuBarIcon:true,notifyRepaired:false,notifyAttention:true,autoCheckUpdates:true};}
function drawUpdateStatus() {const s=currentSettings(),el=$('update-status');if(s.latestVersion){el.textContent=(s.latestVersion===($('version').textContent||'')?t('settings.latest',{version:s.latestVersion}):t('settings.available',{version:s.latestVersion}))+(s.lastUpdateCheck?' '+t('settings.lastChecked',{when:fmtDate(s.lastUpdateCheck)}):'');}else el.textContent=t('settings.updatesText');}
function openSettings() {const s=currentSettings();$('set-login').checked=!!s.launchAtLogin;$('set-menustart').checked=!!s.startInMenuBar;$('set-dock').checked=!!s.showDockIcon;$('set-menubar').checked=!!s.showMenuBarIcon;$('set-notify-repaired').checked=!!s.notifyRepaired;$('set-notify-attention').checked=!!s.notifyAttention;$('set-autoupdate').checked=!!s.autoCheckUpdates;settingsMessage('');drawUpdateStatus();$('settings-dialog').showModal();}
function closeSettings(){if($('settings-dialog').open)$('settings-dialog').close();}
async function saveSettings(){const s={launchAtLogin:$('set-login').checked,startInMenuBar:$('set-menustart').checked,showDockIcon:$('set-dock').checked,showMenuBarIcon:$('set-menubar').checked,notifyRepaired:$('set-notify-repaired').checked,notifyAttention:$('set-notify-attention').checked,autoCheckUpdates:$('set-autoupdate').checked};try{const r=await api('settings',{settings:s});model.settings=r.settings;model.loginAgent=!!r.loginAgent;closeSettings();tell(r.warning||t('settings.saved')+(s.showDockIcon?'':' '+t('settings.leavingDock')),!!r.warning);}catch(e){settingsMessage(e.message,true);}}
$('settings-open').addEventListener('click',openSettings);$('settings-close').addEventListener('click',closeSettings);$('settings-cancel').addEventListener('click',closeSettings);$('settings-save').addEventListener('click',saveSettings);
$('set-notify-test').addEventListener('click',async()=>{try{await api('test-notification',{});settingsMessage(t('settings.testSent'));}catch(e){settingsMessage(e.message,true);}});
$('set-update-check').addEventListener('click',async()=>{settingsMessage(t('settings.checkingGithub'));try{const r=await api('update-check',{});if(model.settings){model.settings.latestVersion=r.latest;model.settings.lastUpdateCheck=r.checkedAt;}drawUpdateStatus();settingsMessage(r.updateAvailable?t('settings.updateAvailable',{version:r.latest}):t('settings.latestDone',{version:r.current}));if(r.updateAvailable&&r.releaseUrl)window.open(r.releaseUrl,'_blank');}catch(e){settingsMessage(e.message,true);}});
$('set-refresh-cache').addEventListener('click',async()=>{try{const r=await api('refresh-icon-cache',{});settingsMessage(t('settings.cacheRefreshed',{n:r.refreshed}));}catch(e){settingsMessage(e.message,true);}});
$('set-open-data').addEventListener('click',async()=>{try{await api('open-data-folder',{});settingsMessage(t('settings.dataOpened'));}catch(e){settingsMessage(e.message,true);}});
$('set-open-setup').addEventListener('click',()=>{closeSettings();openSetup();});
$('settings-dialog').addEventListener('click',e=>{if(e.target===$('settings-dialog'))closeSettings();});
$('setup-open').addEventListener('click',openSetup);$('setup-close').addEventListener('click',closeSetup);$('setup-later').addEventListener('click',closeSetup);$('setup-check').addEventListener('click',runSetupChecks);$('setup-finish').addEventListener('click',finishSetup);
$('setup-dialog').addEventListener('cancel',e=>{if(setupChecking)e.preventDefault();});$('setup-dialog').addEventListener('click',e=>{if(e.target===$('setup-dialog')&&!setupChecking){const r=$('setup-dialog').getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)closeSetup();}});
window.addEventListener('hashchange',()=>{if(location.hash==='#setup')openSetup();});
$('support-paypal').addEventListener('click',()=>openSupport('paypal'));
$('support-github').addEventListener('click',()=>openSupport('github'));
$('dismiss').addEventListener('click',()=>tell(''));$('notice-action').addEventListener('click',async()=>{try{await api('open-settings',{target:'appManagement'});tell(t('notice.openingSettings'));}catch(e){tell(e.message,true);}});$('scan').addEventListener('click',refresh);$('search').addEventListener('input',()=>{query=$('search').value.trim().toLowerCase();selectFuture=false;render();});
$('select-all').addEventListener('change',()=>{selectFuture=$('select-all').checked;for(const a of visible()){if(selectFuture&&ready(a))selected.add(a.id);else selected.delete(a.id);}render();});
$('replace').addEventListener('click',()=>applyApps(visible().filter(a=>selected.has(a.id))));$('restore-all').addEventListener('click',restoreAll);
$('folder').addEventListener('click',async()=>{if(busy)return;busy=true;render();let added=false;try{const r=await api('root',{});added=!r.cancelled;}catch(e){tell(e.message,true);}finally{busy=false;render();}if(added)refresh();});
/* ---- organise: sort + group, remembered between launches ---- */
function saveOrganize(){api('organize',organize).catch(e=>tell(t('organize.saveFailed',{error:e.message}),true));}
$('sort').value=organize.sort;$('group').value=organize.group;
$('sort').addEventListener('change',()=>{organize.sort=$('sort').value;saveOrganize();render();});
$('group').addEventListener('change',()=>{organize.group=$('group').value;saveOrganize();render();});
/* ---- tabs ---- */
function setTab(next){
 tab=next==='defaults'?'defaults':'apps';const appsOn=tab==='apps';
 $('apps-view').hidden=!appsOn;$('defaults-view').hidden=appsOn;
 $('tab-apps').setAttribute('aria-selected',String(appsOn));$('tab-defaults').setAttribute('aria-selected',String(!appsOn));
 if(appsOn)render();else loadDefaults();
}
$('tab-apps').addEventListener('click',()=>setTab('apps'));$('tab-defaults').addEventListener('click',()=>setTab('defaults'));
$('defaults-refresh').addEventListener('click',()=>loadDefaults(true));
/* ---- default apps ---- */
function drawDefaults(){
 const list=$('defaults-list');list.replaceChildren();
 const cats=(defaultsData||[]).filter(c=>c.candidates.length);
 if(!cats.length){list.append(node('p','row-note',t('defaults.none')));return;}
 for(const cat of cats){
  const row=node('section','default-row'),left=node('div','app-cell');
  const view=imageView(true);setImage(view,cat.current.appId?'api/icon?app='+encodeURIComponent(cat.current.appId)+'&v=1':'',t('row.iconAlt',{name:cat.current.name}));
  const copy=node('div','app-copy');copy.append(node('div','app-name',cat.label),node('div','subtitle',cat.detail),node('div','row-note default-current',t('defaults.currently',{name:cat.current.name})));
  left.append(view.wrap,copy);
  const sel=node('select','default-select');sel.setAttribute('aria-label',t('defaults.selectAria',{label:cat.label}));
  for(const c of [...cat.candidates].sort((x,y)=>compareNames(x.name,y.name))){const o=node('option','',c.name);o.value=c.bundleId;sel.append(o);}
  sel.value=cat.current.bundleId||'';
  sel.addEventListener('change',()=>changeDefault(cat,sel));
  const side=node('div','default-side');side.append(sel);row.append(left,side);list.append(row);
 }
}
async function loadDefaults(force=false){
 if(defaultsBusy||(defaultsData&&!force)){if(defaultsData)drawDefaults();return;}
 defaultsBusy=true;$('defaults-list').replaceChildren(node('div','empty',t('defaults.reading')));
 try{const r=await api('defaults');defaultsData=r.categories;drawDefaults();}
 catch(e){const d=node('div','empty');d.append(node('h2','',t('defaults.readFailed')),node('p','',e.message));$('defaults-list').replaceChildren(d);}
 finally{defaultsBusy=false;}
}
async function changeDefault(cat,sel){
 const bid=sel.value,dest=(cat.candidates||[]).find(c=>c.bundleId===bid);sel.disabled=true;
 try{const r=await api('defaults/set',{category:cat.id,bundleId:bid});const i=(defaultsData||[]).findIndex(c=>c.id===cat.id);if(i>=0)defaultsData[i]=r.category;tell(t('defaults.changed',{label:cat.label,name:dest?dest.name:t('defaults.selectedApp')}));}
 catch(e){tell(t('defaults.changeFailed',{label:cat.label,error:e.message}),true);}
 finally{drawDefaults();}
}
document.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();$('search').focus();}});
(async()=>{localizeDocument();$('selection-count').textContent=t('footer.selected',{n:0});$('keepalive').textContent=t('footer.keepalive',{state:t('common.off')});applyTheme('system');try{const s=await api('session');updateModel(s);setupWanted=!!s.setupRequired;if(!themeTouched)applyTheme(s.theme||'system');render();}catch(e){tell(e.message,true);}await refresh();if(setupWanted||location.hash==='#setup')await openSetup();})();
