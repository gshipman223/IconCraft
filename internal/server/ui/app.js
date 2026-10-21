'use strict';
const $=id=>document.getElementById(id);
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
async function api(path,body,signal){const opts={signal};if(body!==undefined){opts.method='POST';opts.headers={'Content-Type':'application/json'};opts.body=JSON.stringify(body);}const res=await fetch('api/'+path,opts);let data;try{data=await res.json();}catch(_){throw Error('IconCraft could not read the response. Please try again.');}if(!res.ok)throw Error(data.error||'The operation failed.');return data;}
function tell(text,error=false){$('notice-text').textContent=text;$('notice').classList.toggle('error',error);$('notice').hidden=!text;const act=$('notice-action');if(act)act.hidden=!(error&&/App Management/.test(text||''));}
function rs(a){if(!rowState.has(a.id))rowState.set(a.id,{checking:false,error:'',note:'',working:false,revision:0});return rowState.get(a.id);}
function chosen(a){return model.choices[a.bundleId]||model.originals[a.craftId]?.assetId||'';}
function ready(a){return !a.protected&&!!model.assets[chosen(a)];}
function live(r){return ['applied','prepared','rollback-required'].includes(r.status);}
function active(a){return a.renamed||model.records.some(r=>r.appId===a.id&&live(r));}
/* Every installed app is listed with its real icon. Apps with a known
   original counterpart get background original-artwork checks; every app can
   still be given any installed app's icon, a custom image, or a new name. */
function compareApps(a,b){
 if(organize.sort==='name-desc')return b.name.localeCompare(a.name);
 if(organize.sort==='status'){const d=Number(active(b))-Number(active(a));if(d)return d;const r=Number(ready(b))-Number(ready(a));if(r)return r;}
 return a.name.localeCompare(b.name);
}
function relevant(){return [...model.apps].sort(compareApps);}
function groupOf(a){
 if(organize.group==='status')return active(a)?'Customised':ready(a)?'Original ready':'No original found yet';
 if(organize.group==='location')return a.path.replace(/\/[^/]*\.app$/i,'')||'Applications';
 return '';
}
function visible(){return relevant().filter(a=>(a.name+' '+a.equivalent+' '+a.craftId+' '+(model.originals[a.craftId]?.product||'')).toLowerCase().includes(query));}
function sourceStatus(a){const v=model.originals[a.craftId];return v?.source||'';}
function appIcon(a){return 'api/icon?app='+encodeURIComponent(a.id)+'&v='+encodeURIComponent((a.iconStamp||'1')+'-'+rs(a).revision);}
function assetIcon(id){return id?'api/icon?asset='+encodeURIComponent(id):'';}
function imageView(small=false){const wrap=node('span','icon-wrap'+(small?' small':''));const f=svg('grid');f.classList.add('fallback');const img=node('img','icon');img.alt='';/* NOT lazy: icons are display:none until load, and WebKit never fetches hidden lazy images — the load event that reveals them would never fire. Local thumbnails are cheap; load eagerly. */img.decoding='async';wrap.append(f,img);img.addEventListener('load',()=>wrap.classList.add('has-image'));img.addEventListener('error',()=>wrap.classList.remove('has-image'));return {wrap,img,src:''};}
function setImage(view,src,alt){view.img.alt=alt;if(view.src===src)return;view.src=src;view.wrap.classList.remove('has-image');if(src)view.img.src=src;else view.img.removeAttribute('src');}
function updateModel(data){if(data.organize){organize={sort:data.organize.sort||'name-asc',group:data.organize.group||'none'};const so=$('sort'),gr=$('group');if(so)so.value=organize.sort;if(gr)gr.value=organize.group;}model.apps=data.apps||model.apps;Object.assign(model.assets,data.assets||{});Object.assign(model.originals,data.originals||{});model.choices=data.choices||{};model.records=data.records||[];model.renames=data.renames||[];if(typeof data.keepAlive==='boolean')model.keepAlive=data.keepAlive;if(typeof data.setupRequired==='boolean')model.setupRequired=data.setupRequired;if(data.settings)model.settings=data.settings;if(typeof data.loginAgent==='boolean')model.loginAgent=data.loginAgent;model.rebrands=data.rebrands||{};model.rebrandRuntime=data.rebrandRuntime||{};model.vault=data.vault||{};$('version').textContent=data.version||'0.13.0';const v2=$('version-2');if(v2)v2.textContent=data.version||'0.13.0';selected.forEach(id=>{if(!model.apps.some(a=>a.id===id))selected.delete(id);});}
async function session(){updateModel(await api('session'));render();}
function makeRow(a){
 const currentApp=()=>model.apps.find(x=>x.id===a.id)||a;
 const root=node('div','app-row');root.dataset.app=a.id;
 const check=node('input');check.type='checkbox';check.addEventListener('change',()=>{selectFuture=false;if(check.checked)selected.add(a.id);else selected.delete(a.id);render();});
 const current=imageView(),target=imageView();
 const left=node('div','app-cell'),copy=node('div','app-copy'),name=node('div','app-name'),sub=node('div','subtitle');copy.append(name,sub);left.append(current.wrap,copy);
 const arr=svg('arrow');arr.classList.add('arrow');
 const right=node('div','app-cell'),targetButton=button('Choose icon','icon-button',()=>openPicker(currentApp()));targetButton.replaceChildren(target.wrap);
 const detail=node('div','app-copy'),title=node('div','replacement-name'),note=node('div','row-note');detail.append(title,note);right.append(targetButton,detail);
 const actions=node('div','row-actions');const custom=button('Choose custom icon','icon-action',()=>openPicker(currentApp()),'grid');const rename=button('Rename app','icon-action',()=>openRename(currentApp()),'edit');const restore=button('Restore this app','icon-action',()=>restoreOne(currentApp()),'restore');const set=button('Replace','set',()=>applyApps([currentApp()]));
 actions.append(custom,rename,restore,set);const error=node('div','row-error');error.hidden=true;root.append(check,left,arr,right,actions,error);
 return {root,check,current,target,name,sub,title,note,targetButton,custom,rename,restore,set,error};
}
function updateRow(a,v){
 const st=rs(a),id=chosen(a),custom=!!model.choices[a.bundleId],orig=model.originals[a.craftId];
 v.root.classList.toggle('selected',selected.has(a.id));v.check.checked=selected.has(a.id);v.check.disabled=busy||!ready(a);v.check.setAttribute('aria-label','Select '+a.name);
 v.name.textContent=a.name;v.name.title=a.path;v.sub.textContent=(active(a)?(a.renamed?'Renamed'+(model.records.some(r=>r.appId===a.id&&live(r))?' · Icon changed':''):'Icon changed'):(a.version?'v'+a.version:'Installed'))+(model.rebrands[a.bundleId]?' · Full skin'+(model.rebrandRuntime[a.bundleId]==='own'?' (app controls its running icon)':''):'')+(model.vault[a.bundleId]?' · Original saved':'');v.sub.classList.toggle('changed',active(a));
 setImage(v.current,appIcon(a),a.name+' current icon');
 /* With no mapped/custom target yet, preview the app's own icon dimmed so
    every row shows real artwork instead of an empty placeholder. */
 setImage(v.target,id?assetIcon(id):appIcon(a),(custom?'Custom':orig?.product||a.equivalent||'Original')+' icon');
 v.target.wrap.classList.toggle('dim',!id);
 v.title.textContent=custom?'Custom icon':orig?.product||a.equivalent?.replace(' (suggested)','')||(a.craftId?'Finding original…':'No original mapped');v.title.title=custom?model.assets[id]?.name||'Custom icon':sourceStatus(a);
 let note=custom?'Your choice':st.checking?(id?'Saved · checking sources…':'Finding original…'):st.note|| (id?'Original ready':a.craftId?'Choose a custom icon':'Pick any icon →');
 if(st.error&&!id&&!custom)note='Not found · choose icon';v.note.textContent=note;v.note.title=st.error||sourceStatus(a);v.note.className='row-note'+(st.checking&&!custom?' checking':'')+(st.error&&!id?' error':'');
 v.targetButton.disabled=busy;v.custom.disabled=busy;v.rename.disabled=busy||a.protected;v.restore.disabled=busy||!active(a);v.set.disabled=busy||!ready(a);v.set.textContent=st.working?'Working…':'Replace';
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
   const head=node('div','group-head',g+(groupCounts[g]?' · '+groupCounts[g]:''));
   groupHeadNodes.push(head);sequence.push(head);lastGroup=g;
  }
  sequence.push(v.root);
 }
 for(const n of sequence)$('apps').append(n);
 const eligible=filtered.filter(ready),count=eligible.filter(a=>selected.has(a.id)).length;
 $('select-all').disabled=busy||eligible.length===0;$('select-all').checked=eligible.length>0&&count===eligible.length;$('select-all').indeterminate=count>0&&count<eligible.length;
 $('replace').disabled=busy||count===0;$('selection-count').textContent=count+' selected';$('restore-all').disabled=busy||!model.records.some(live)&&!model.apps.some(a=>a.renamed);
 $('scan').disabled=scanning||busy;$('scan').classList.toggle('refreshing',scanning);$('folder').disabled=busy||scanning;
 $('keepalive').textContent='Keep skins applied: '+(model.keepAlive?'On':'Off');$('keepalive').classList.toggle('on',model.keepAlive);
 const checks=all.filter(a=>rs(a).checking).length;$('activity').classList.toggle('updating',checks>0||scanning);
 $('status').textContent=scanning?(all.length?all.length+' apps · scanning':'Scanning this Mac…'):checks?all.length+' apps · '+checks+' refreshing':all.length+' apps · '+all.filter(ready).length+' ready';
 $('loading').hidden=!firstLoad||all.length>0;$('empty').hidden=firstLoad||filtered.length>0;
}
function applyTheme(mode,persist=false){
 theme=['system','light','dark'].includes(mode)?mode:'system';document.documentElement.dataset.theme=theme==='system'?(media.matches?'dark':'light'):theme;
 document.querySelectorAll('[data-theme-choice]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.themeChoice===theme)));
 if(persist){themeTouched=true;api('theme',{theme}).catch(e=>tell('Could not save appearance: '+e.message,true));}
}
media.addEventListener('change',()=>{if(theme==='system')applyTheme('system');});document.querySelectorAll('[data-theme-choice]').forEach(b=>b.addEventListener('click',()=>applyTheme(b.dataset.themeChoice,true)));
function rasterize(dataURL){return new Promise((resolve,reject)=>{
 const im=new Image();const timer=setTimeout(()=>reject(Error('This source returned an unreadable icon.')),5000);
 im.onload=()=>{clearTimeout(timer);try{const w=im.naturalWidth,h=im.naturalHeight;if(!w||!h||w>4096||h>4096||w/h<.6||w/h>1.6)throw Error('This source did not return a square app icon.');const canvas=document.createElement('canvas');canvas.width=512;canvas.height=512;const ctx=canvas.getContext('2d');if(!ctx)throw Error('Image decoding is unavailable.');const scale=512/Math.max(w,h);ctx.drawImage(im,(512-w*scale)/2,(512-h*scale)/2,w*scale,h*scale);resolve(canvas.toDataURL('image/png'));}catch(e){reject(e);}};
 im.onerror=()=>{clearTimeout(timer);reject(Error('This source returned an unreadable icon.'));};im.src=dataURL;
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
 state.error='';state.note=result.status==='fresh'?'Original · source checked':result.status==='installed'?'From installed original':'Saved original · offline';
 if(selectFuture&&ready(a))selected.add(a.id);
 }catch(e){if(run===generation&&e.name!=='AbortError'){state.error=e.message;state.note=chosen(a)?'Saved original · offline':'Choose a custom icon';}}
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
 tell(done+' '+(done===1?'icon replaced.':'icons replaced.')+(errors.length?'\n'+errors.join('\n'):''),errors.length>0);render();
}
async function restoreOne(a){if(busy)return;busy=true;rs(a).actionError='';render();try{const r=await api('restore-app',{appId:a.id,confirmed:true});rs(a).revision++;await session();tell(r.steps&&r.steps.length?('Restored '+a.name+': '+r.steps.join('; ')+'.'):('Restored '+a.name+'.'));}catch(e){rs(a).actionError=e.message;tell(e.message,true);}finally{busy=false;render();}}
async function restoreAll(){if(busy)return;busy=true;render();try{const r=await api('restore-all',{confirmed:true});rowState.forEach(s=>{s.revision++;s.actionError='';});await session();tell(r.restored+' apps restored.'+(r.errors?.length?'\n'+r.errors.join('\n'):''),!!r.errors?.length);}catch(e){tell(e.message,true);}finally{busy=false;render();}}
function dialogError(id,text){$(id).textContent=text;$(id).hidden=!text;}
function setupMessage(text){const m=$('setup-message');m.textContent=text;m.hidden=!text;}
function setupCheck(id){return setupData?.checks?.find(c=>c.id===id);}
function setupActionButton(label,fn,primary=false){const b=node('button',primary?'primary':'secondary',label);b.disabled=setupChecking||busy;b.addEventListener('click',fn);return b;}
function drawSetup(){
 const checks=setupData?.checks||[],blocked=checks.filter(c=>c.state==='blocked').length,unchecked=checks.filter(c=>c.state==='unknown').length,ready=checks.filter(c=>c.state==='ready').length;
 $('setup-summary').textContent=blocked?blocked+(blocked===1?' item needs':' items need')+' attention. Open the highlighted macOS setting, allow IconCraft, then run the checks again. You can still finish setup and return later.':unchecked?'Some permissions have not been tested yet. Run the checks, then allow anything macOS is blocking.':ready+' essential items are ready. Optional items are labelled clearly below.';
 const list=$('setup-checks');list.replaceChildren();
 for(const check of checks){
  const row=node('section','setup-row'),copy=node('div'),side=node('div','setup-side'),pill=node('span','status-pill '+check.state,check.label),actions=node('div','setup-actions');
  copy.append(node('h3','',check.title),node('p','',check.detail));
  if(check.id==='appManagement'&&setupData?.appAccess?.checkedAt){const when=new Date(setupData.appAccess.checkedAt);if(!Number.isNaN(when.getTime()))copy.append(node('p','setup-meta','Last checked '+when.toLocaleString()+(setupData.appAccess.appName?' · Tested app: '+setupData.appAccess.appName:'')));}
  if(check.id==='appManagement'){
   actions.append(setupActionButton('Open App Management',openSetupAppManagement),setupActionButton(check.state==='unknown'?'Check now':'Check again',runSetupChecks,true));
  }else if(check.id==='background'){
   const on=!!setupData?.keeper?.desired;
   actions.append(setupActionButton(on?'Turn keeper off':'Turn keeper on',toggleSetupKeeper,on&&check.state!=='ready'));
  }
  side.append(pill,actions);row.append(copy,side);list.append(row);
 }
 $('setup-check').disabled=setupChecking||busy;$('setup-check').textContent=setupChecking?'Checking…':'Run checks';
 $('setup-close').disabled=setupChecking;
 $('setup-later').disabled=setupChecking;$('setup-later').textContent=setupData?.setup?.completed?'Close':'Review later';
 $('setup-finish').disabled=setupChecking;$('setup-finish').textContent=setupData?.setup?.completed?'Done':'Finish setup';
 $('setup-subtitle').textContent=setupData?.setup?.completed?'Your saved setup is complete. Recheck permissions after an IconCraft or macOS update.':'Give IconCraft the access it needs, and see exactly what it does not ask for.';
}
async function openSetup(){
 if(setupOpening||$('setup-dialog').open)return;setupOpening=true;dialogError('setup-error','');setupMessage('');
 try{setupData=await api('permissions');drawSetup();$('setup-dialog').showModal();}
 catch(e){tell('Could not load setup and permissions: '+e.message,true);}
 finally{setupOpening=false;}
}
async function runSetupChecks(){
 if(setupChecking)return;setupChecking=true;dialogError('setup-error','');setupMessage('');drawSetup();
 try{setupData=await api('permissions/check',{});const access=setupCheck('appManagement');setupMessage(access?.state==='ready'?'Checks finished. App Management passed the live write test.':access?.state==='blocked'?'Checks finished. App Management is blocking IconCraft — open the setting below, switch IconCraft on, then check again.':'Checks finished. Review any item still marked as needing attention.');}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();render();}
}
async function openSetupAppManagement(){
 dialogError('setup-error','');try{await api('open-settings',{target:'appManagement'});setupMessage('System Settings opened at Privacy & Security → App Management. Switch IconCraft on, return here, then choose Check again.');}
 catch(e){dialogError('setup-error',e.message);}
}
async function toggleSetupKeeper(){
 if(setupChecking)return;const on=!setupData?.keeper?.desired;setupChecking=true;drawSetup();dialogError('setup-error','');
 try{const r=await api('keepalive',{enabled:on});model.keepAlive=!!r.enabled;setupData=await api('permissions');setupMessage(on?'Keep skins applied is on. macOS has been asked to load the background helper; its live state is shown above.':'Keep skins applied is off. Existing icons and names remain in place; automatic repairs are paused.');}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();render();}
}
async function finishSetup(){
 if(setupChecking)return;
 if(setupData?.setup?.completed){$('setup-dialog').close();return;}
 setupChecking=true;drawSetup();dialogError('setup-error','');
 try{await api('setup/complete',{confirmed:true});model.setupRequired=false;setupWanted=false;if(setupData?.setup)setupData.setup.completed=true;$('setup-dialog').close();tell('Setup complete. You can revisit permissions any time from Setup & permissions.');}
 catch(e){dialogError('setup-error',e.message);}
 finally{setupChecking=false;drawSetup();}
}
function closeSetup(){if(!setupChecking)$('setup-dialog').close();}
function closePicker(){if(!busy){$('picker').close();pickerApp=null;}}
function openPicker(a){if(busy)return;pickerApp=a;$('picker-for').textContent='For '+a.name;$('picker-search').value='';dialogError('picker-error','');drawPicker();$('picker').showModal();$('picker-search').focus();}
function drawPicker(){
 if(!pickerApp)return;const a=pickerApp,original=model.originals[a.craftId];$('picker-original').replaceChildren();
 if(original?.assetId){const b=button('Use original icon','original-tile',async()=>{await pickChoice(()=>api('choice',{appId:a.id,assetId:''}),()=>{delete model.choices[a.bundleId];});});const v=imageView(true);setImage(v,assetIcon(original.assetId),original.product);const t=node('span');t.append(node('span','',original.product),node('small','','Use automatically discovered original'));b.replaceChildren(v.wrap,t);$('picker-original').append(b);}
 const q=$('picker-search').value.toLowerCase().trim(),frag=document.createDocumentFragment();
 for(const donor of model.apps.filter(d=>d.name.toLowerCase().includes(q)).sort((a,b)=>a.name.localeCompare(b.name)).slice(0,200)){
  const b=button('Use icon from '+donor.name,'icon-tile',()=>pickChoice(()=>api('donor-choice',{appId:a.id,donorId:donor.id}),r=>{model.assets[r.asset.id]=r.asset;model.choices[a.bundleId]=r.asset.id;}));
  const v=imageView(true);setImage(v,appIcon(donor),donor.name+' icon');b.replaceChildren(v.wrap,node('span','',donor.name));frag.append(b);
 }
 $('picker-grid').replaceChildren(frag);if(!frag.childNodes.length&&!$('picker-grid').children.length)$('picker-grid').append(node('p','row-note','No matching installed apps.'));
}
async function pickChoice(action,update){if(busy)return;busy=true;$('picker').querySelectorAll('button').forEach(b=>b.disabled=true);render();try{const result=await action();if(!result.cancelled){update(result);$('picker').close();pickerApp=null;tell('Icon selected. Click Replace to apply.');}}catch(e){dialogError('picker-error',e.message);}finally{busy=false;$('picker').querySelectorAll('button').forEach(b=>b.disabled=false);render();}}
$('browse').addEventListener('click',()=>{if(!pickerApp)return;const a=pickerApp;pickChoice(()=>api('custom',{appId:a.id}),r=>{model.assets[r.asset.id]=r.asset;model.choices[a.bundleId]=r.asset.id;});});
function openRename(a){if(busy)return;renameApp=a;$('rename-for').textContent=a.name;$('name-input').value=a.name;$('name-original').disabled=!(model.originals[a.craftId]?.product||a.equivalent);$('name-restore').hidden=!a.renamed;dialogError('rename-error','');
 const sv=model.vault[a.bundleId];$('vault-state').textContent=sv?('Original saved in the vault: “'+sv.appName+'” (folder “'+sv.folderName+'.app”'+(sv.internalName?', internal name “'+sv.internalName+'”':'')+(sv.iconAssetId?', original icon backed up':'')+'). Restore rebuilds from this — even if the app updates, moves, or is renamed again.'):'No saved original yet — IconCraft saves the original name and icon to its vault before it changes anything.';
 const skinned=!!model.rebrands[a.bundleId];$('name-unskin').hidden=!skinned;$('fullskin-state').textContent=skinned?(model.rebrandRuntime[a.bundleId]==='own'?'Full skin applied (“'+model.rebrands[a.bundleId]+'”), but this app sets its own Dock icon while running and no replaceable copy was found inside it — its running icon cannot be changed safely.':'Full skin applied (“'+model.rebrands[a.bundleId]+'”), including the icon it shows while running.'):(chosen(a)?'Uses the icon currently chosen for this app.':'Choose an icon for this app first.');
 const ib=$('name-internal');ib.dataset.armed='';ib.textContent='Apply full skin…';ib.disabled=!chosen(a);
 $('rename-dialog').showModal();$('name-input').focus();$('name-input').select();}
function closeRename(){if(!busy){$('rename-dialog').close();renameApp=null;}}
async function saveName(restore=false){if(busy||!renameApp)return;const a=renameApp;busy=true;$('name-save').disabled=true;$('name-restore').disabled=true;render();try{const result=await api(restore?'restore-name':'rename',{appId:a.id,...(restore?{}:{name:$('name-input').value}),confirmed:true});await session();$('rename-dialog').close();renameApp=null;tell('App name changed to '+result.app.name+'.');}catch(e){dialogError('rename-error',e.message);}finally{busy=false;$('name-save').disabled=false;$('name-restore').disabled=false;render();}}
$('name-internal').addEventListener('click',async()=>{if(busy||!renameApp)return;const a=renameApp,ib=$('name-internal');
 if(!ib.dataset.armed){ib.dataset.armed='1';ib.textContent='Confirm: write inside “'+a.name+'” & re-sign';setTimeout(()=>{ib.dataset.armed='';ib.textContent='Apply full skin…';},5000);return;}
 busy=true;ib.disabled=true;render();try{await api('rebrand',{appId:a.id,assetId:chosen(a),name:$('name-input').value,confirmed:true});await session();$('rename-dialog').close();renameApp=null;tell('Full skin applied to '+a.name+'. Its Dock, menu-bar and Launchpad identity now match — quit and relaunch it to see it.');}catch(e){dialogError('rename-error',e.message);}finally{busy=false;ib.disabled=false;ib.dataset.armed='';ib.textContent='Apply full skin…';render();}});
$('name-unskin').addEventListener('click',async()=>{if(busy||!renameApp)return;const a=renameApp;busy=true;render();try{await api('restore-rebrand',{appId:a.id,confirmed:true});await session();$('rename-dialog').close();renameApp=null;tell('Internal original restored for '+a.name+'.');}catch(e){dialogError('rename-error',e.message);}finally{busy=false;render();}});
$('name-original').addEventListener('click',()=>{if(renameApp)$('name-input').value=model.originals[renameApp.craftId]?.product||renameApp.equivalent.replace(' (suggested)','');});
$('name-save').addEventListener('click',()=>saveName());$('name-restore').addEventListener('click',()=>saveName(true));$('name-input').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();saveName();}});
$('rename-close').addEventListener('click',closeRename);$('rename-cancel').addEventListener('click',closeRename);$('picker-close').addEventListener('click',closePicker);$('picker-search').addEventListener('input',drawPicker);
for(const id of ['picker','rename-dialog']){$(id).addEventListener('cancel',e=>{if(busy)e.preventDefault();});$(id).addEventListener('click',e=>{if(e.target===$(id)&&!busy){const r=$(id).getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)$(id).close();}});}
async function openSupport(target){try{const r=await api('support',{target});if(!r.opened&&r.url)window.open(r.url,'_blank');else if(r.opened)tell(target==='paypal'?'Opening PayPal in your browser — thank you for supporting IconCraft!':'Opening GitHub in your browser…');}catch(e){tell((target==='paypal'?'Support via PayPal: https://www.paypal.com/ncp/payment/E8YDD8BNAKVJA':'GitHub: https://github.com/gshipman223/IconCraft'),true);}}
$('keepalive').addEventListener('click',async()=>{if(busy)return;busy=true;render();try{const r=await api('keepalive',{enabled:!model.keepAlive});model.keepAlive=!!r.enabled;tell(model.keepAlive?'Keep-applied is on. If an app or updater reverts your icon or name, IconCraft will quietly re-apply it (checks at login, every few minutes, and when apps change). While an app is running, macOS shows its own Dock/menu identity — IconCraft never edits inside apps, so that part is the app’s, by design.':'Keep-applied is off. Icons and names stay as-is; nothing will re-apply them if an app reverts them.');}catch(e){tell(e.message,true);}finally{busy=false;render();}});
function settingsMessage(text,error=false){const m=$('settings-message'),e=$('settings-error');if(error){e.textContent=text;e.hidden=false;m.hidden=true;}else{m.textContent=text;m.hidden=!text;e.hidden=true;}}
function currentSettings(){return model.settings||{launchAtLogin:false,startInMenuBar:false,showDockIcon:true,showMenuBarIcon:true,notifyRepaired:false,notifyAttention:true,autoCheckUpdates:true};}
function drawUpdateStatus() {const s=currentSettings(),el=$('update-status');if(s.latestVersion){el.textContent=(s.latestVersion===($('version').textContent||'')?'You are on the latest version (v'+s.latestVersion+')':'v'+s.latestVersion+' is available — Check now, then download it from GitHub.')+(s.lastUpdateCheck?' Last checked '+new Date(s.lastUpdateCheck).toLocaleString()+'.':'');}else el.textContent='IconCraft never updates itself — checking just compares your version with the latest GitHub release.';}
function openSettings() {const s=currentSettings();$('set-login').checked=!!s.launchAtLogin;$('set-menustart').checked=!!s.startInMenuBar;$('set-dock').checked=!!s.showDockIcon;$('set-menubar').checked=!!s.showMenuBarIcon;$('set-notify-repaired').checked=!!s.notifyRepaired;$('set-notify-attention').checked=!!s.notifyAttention;$('set-autoupdate').checked=!!s.autoCheckUpdates;settingsMessage('');drawUpdateStatus();$('settings-dialog').showModal();}
function closeSettings(){if($('settings-dialog').open)$('settings-dialog').close();}
async function saveSettings(){const s={launchAtLogin:$('set-login').checked,startInMenuBar:$('set-menustart').checked,showDockIcon:$('set-dock').checked,showMenuBarIcon:$('set-menubar').checked,notifyRepaired:$('set-notify-repaired').checked,notifyAttention:$('set-notify-attention').checked,autoCheckUpdates:$('set-autoupdate').checked};try{const r=await api('settings',{settings:s});model.settings=r.settings;model.loginAgent=!!r.loginAgent;closeSettings();tell(r.warning||'Settings saved.'+(s.showDockIcon?'':' IconCraft will leave the Dock within a few seconds — it stays in the menu bar.'),!!r.warning);}catch(e){settingsMessage(e.message,true);}}
$('settings-open').addEventListener('click',openSettings);$('settings-close').addEventListener('click',closeSettings);$('settings-cancel').addEventListener('click',closeSettings);$('settings-save').addEventListener('click',saveSettings);
$('set-notify-test').addEventListener('click',async()=>{try{await api('test-notification',{});settingsMessage('Test notification sent. If nothing appeared, allow IconCraft under System Settings → Notifications.');}catch(e){settingsMessage(e.message,true);}});
$('set-update-check').addEventListener('click',async()=>{settingsMessage('Checking GitHub for the latest release…');try{const r=await api('update-check',{});if(model.settings){model.settings.latestVersion=r.latest;model.settings.lastUpdateCheck=r.checkedAt;}drawUpdateStatus();settingsMessage(r.updateAvailable?'IconCraft v'+r.latest+' is available. Download it from the GitHub releases page — your skins and settings carry over.':'You are on the latest version (v'+r.current+').');if(r.updateAvailable&&r.releaseUrl)window.open(r.releaseUrl,'_blank');}catch(e){settingsMessage(e.message,true);}});
$('set-refresh-cache').addEventListener('click',async()=>{try{const r=await api('refresh-icon-cache',{});settingsMessage('Asked macOS to re-read the icons of '+r.refreshed+' skinned app(s). Give Finder and the Dock a few seconds.');}catch(e){settingsMessage(e.message,true);}});
$('set-open-data').addEventListener('click',async()=>{try{await api('open-data-folder',{});settingsMessage('Opened the IconCraft data folder in Finder — backups, history and logs live there.');}catch(e){settingsMessage(e.message,true);}});
$('set-open-setup').addEventListener('click',()=>{closeSettings();openSetup();});
$('settings-dialog').addEventListener('click',e=>{if(e.target===$('settings-dialog'))closeSettings();});
$('setup-open').addEventListener('click',openSetup);$('setup-close').addEventListener('click',closeSetup);$('setup-later').addEventListener('click',closeSetup);$('setup-check').addEventListener('click',runSetupChecks);$('setup-finish').addEventListener('click',finishSetup);
$('setup-dialog').addEventListener('cancel',e=>{if(setupChecking)e.preventDefault();});$('setup-dialog').addEventListener('click',e=>{if(e.target===$('setup-dialog')&&!setupChecking){const r=$('setup-dialog').getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)closeSetup();}});
window.addEventListener('hashchange',()=>{if(location.hash==='#setup')openSetup();});
$('support-paypal').addEventListener('click',()=>openSupport('paypal'));
$('support-github').addEventListener('click',()=>openSupport('github'));
$('dismiss').addEventListener('click',()=>tell(''));$('notice-action').addEventListener('click',async()=>{try{await api('open-settings',{target:'appManagement'});tell('Opening System Settings → Privacy & Security → App Management. Switch IconCraft on, then try again.');}catch(e){tell(e.message,true);}});$('scan').addEventListener('click',refresh);$('search').addEventListener('input',()=>{query=$('search').value.trim().toLowerCase();selectFuture=false;render();});
$('select-all').addEventListener('change',()=>{selectFuture=$('select-all').checked;for(const a of visible()){if(selectFuture&&ready(a))selected.add(a.id);else selected.delete(a.id);}render();});
$('replace').addEventListener('click',()=>applyApps(visible().filter(a=>selected.has(a.id))));$('restore-all').addEventListener('click',restoreAll);
$('folder').addEventListener('click',async()=>{if(busy)return;busy=true;render();let added=false;try{const r=await api('root',{});added=!r.cancelled;}catch(e){tell(e.message,true);}finally{busy=false;render();}if(added)refresh();});
/* ---- organise: sort + group, remembered between launches ---- */
function saveOrganize(){api('organize',organize).catch(e=>tell('Could not save list order: '+e.message,true));}
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
 if(!cats.length){list.append(node('p','row-note','macOS reported no choices for any category.'));return;}
 for(const cat of cats){
  const row=node('section','default-row'),left=node('div','app-cell');
  const view=imageView(true);setImage(view,cat.current.appId?'api/icon?app='+encodeURIComponent(cat.current.appId)+'&v=1':'',cat.current.name+' icon');
  const copy=node('div','app-copy');copy.append(node('div','app-name',cat.label),node('div','subtitle',cat.detail),node('div','row-note default-current','Currently: '+cat.current.name));
  left.append(view.wrap,copy);
  const sel=node('select','default-select');sel.setAttribute('aria-label','Default app for '+cat.label);
  for(const c of [...cat.candidates].sort((x,y)=>x.name.localeCompare(y.name))){const o=node('option','',c.name);o.value=c.bundleId;sel.append(o);}
  sel.value=cat.current.bundleId||'';
  sel.addEventListener('change',()=>changeDefault(cat,sel));
  const side=node('div','default-side');side.append(sel);row.append(left,side);list.append(row);
 }
}
async function loadDefaults(force=false){
 if(defaultsBusy||(defaultsData&&!force)){if(defaultsData)drawDefaults();return;}
 defaultsBusy=true;$('defaults-list').replaceChildren(node('div','empty','Reading your current defaults…'));
 try{const r=await api('defaults');defaultsData=r.categories;drawDefaults();}
 catch(e){const d=node('div','empty');d.append(node('h2','','Could not read your defaults'),node('p','',e.message));$('defaults-list').replaceChildren(d);}
 finally{defaultsBusy=false;}
}
async function changeDefault(cat,sel){
 const bid=sel.value,dest=(cat.candidates||[]).find(c=>c.bundleId===bid);sel.disabled=true;
 try{const r=await api('defaults/set',{category:cat.id,bundleId:bid});const i=(defaultsData||[]).findIndex(c=>c.id===cat.id);if(i>=0)defaultsData[i]=r.category;tell(cat.label+' now opens with '+(dest?dest.name:'the selected app')+'.');}
 catch(e){tell('Could not change '+cat.label+': '+e.message,true);}
 finally{drawDefaults();}
}
document.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();$('search').focus();}});
(async()=>{applyTheme('system');try{const s=await api('session');updateModel(s);setupWanted=!!s.setupRequired;if(!themeTouched)applyTheme(s.theme||'system');render();}catch(e){tell(e.message,true);}await refresh();if(setupWanted||location.hash==='#setup')await openSetup();})();
