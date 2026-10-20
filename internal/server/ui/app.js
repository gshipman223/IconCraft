'use strict';
const $=id=>document.getElementById(id);
const model={apps:[],assets:{},originals:{},choices:{},records:[],renames:[],rebrands:{},vault:{},keepAlive:false};
const rows=new Map(), rowState=new Map(), selected=new Set();
let scanning=false, busy=false, firstLoad=true, selectFuture=false, query='',theme='system',themeTouched=false;
let generation=0,workerCount=0,queue=[],seen=new Set(),controllers=new Set(),pickerApp=null,renameApp=null;
const media=window.matchMedia('(prefers-color-scheme: dark)');
const paths={grid:'M3 3h5v5H3zM12 3h5v5h-5zM3 12h5v5H3zM12 12h5v5h-5z',restore:'M4 7a6 6 0 111 8M1.5 3v5h5',edit:'m4 13-1 4 4-1L17 6l-3-3zM12 5l3 3',arrow:'M4 10h12m-4-4 4 4-4 4',refresh:'M16 7A6 6 0 004 5L2 7m0-4v4h4M4 13a6 6 0 0012 2l2-2m0 4v-4h-4'};
function svg(key){const s=document.createElementNS('http://www.w3.org/2000/svg','svg');s.setAttribute('viewBox','0 0 20 20');s.setAttribute('aria-hidden','true');const p=document.createElementNS(s.namespaceURI,'path');p.setAttribute('d',paths[key]||paths.grid);s.append(p);return s;}
function node(tag,cls,text){const e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e;}
function button(label,cls,fn,icon){const b=node('button',cls);b.title=label;b.setAttribute('aria-label',label);if(icon)b.append(svg(icon));else b.textContent=label;b.addEventListener('click',fn);return b;}
async function api(path,body,signal){const opts={signal};if(body!==undefined){opts.method='POST';opts.headers={'Content-Type':'application/json'};opts.body=JSON.stringify(body);}const res=await fetch('api/'+path,opts);let data;try{data=await res.json();}catch(_){throw Error('IconCraft could not read the response. Please try again.');}if(!res.ok)throw Error(data.error||'The operation failed.');return data;}
function tell(text,error=false){$('notice-text').textContent=text;$('notice').classList.toggle('error',error);$('notice').hidden=!text;}
function rs(a){if(!rowState.has(a.id))rowState.set(a.id,{checking:false,error:'',note:'',working:false,revision:0});return rowState.get(a.id);}
function chosen(a){return model.choices[a.bundleId]||model.originals[a.craftId]?.assetId||'';}
function ready(a){return !a.protected&&!!model.assets[chosen(a)];}
function live(r){return ['applied','prepared','rollback-required'].includes(r.status);}
function active(a){return a.renamed||model.records.some(r=>r.appId===a.id&&live(r));}
/* Every installed app is listed with its real icon. Apps with a known
   original counterpart get background original-artwork checks; every app can
   still be given any installed app's icon, a custom image, or a new name. */
function relevant(){return [...model.apps].sort((a,b)=>a.name.localeCompare(b.name));}
function visible(){return relevant().filter(a=>(a.name+' '+a.equivalent+' '+a.craftId+' '+(model.originals[a.craftId]?.product||'')).toLowerCase().includes(query));}
function sourceStatus(a){const v=model.originals[a.craftId];return v?.source||'';}
function appIcon(a){return 'api/icon?app='+encodeURIComponent(a.id)+'&v='+encodeURIComponent((a.iconStamp||'1')+'-'+rs(a).revision);}
function assetIcon(id){return id?'api/icon?asset='+encodeURIComponent(id):'';}
function imageView(small=false){const wrap=node('span','icon-wrap'+(small?' small':''));const f=svg('grid');f.classList.add('fallback');const img=node('img','icon');img.alt='';/* NOT lazy: icons are display:none until load, and WebKit never fetches hidden lazy images — the load event that reveals them would never fire. Local thumbnails are cheap; load eagerly. */img.decoding='async';wrap.append(f,img);img.addEventListener('load',()=>wrap.classList.add('has-image'));img.addEventListener('error',()=>wrap.classList.remove('has-image'));return {wrap,img,src:''};}
function setImage(view,src,alt){view.img.alt=alt;if(view.src===src)return;view.src=src;view.wrap.classList.remove('has-image');if(src)view.img.src=src;else view.img.removeAttribute('src');}
function updateModel(data){model.apps=data.apps||model.apps;Object.assign(model.assets,data.assets||{});Object.assign(model.originals,data.originals||{});model.choices=data.choices||{};model.records=data.records||[];model.renames=data.renames||[];if(typeof data.keepAlive==='boolean')model.keepAlive=data.keepAlive;model.rebrands=data.rebrands||{};model.vault=data.vault||{};$('version').textContent=data.version||'0.8.0';const v2=$('version-2');if(v2)v2.textContent=data.version||'0.8.0';selected.forEach(id=>{if(!model.apps.some(a=>a.id===id))selected.delete(id);});}
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
 v.name.textContent=a.name;v.name.title=a.path;v.sub.textContent=(active(a)?(a.renamed?'Renamed'+(model.records.some(r=>r.appId===a.id&&live(r))?' · Icon changed':''):'Icon changed'):(a.version?'v'+a.version:'Installed'))+(model.rebrands[a.bundleId]?' · Full skin':'')+(model.vault[a.bundleId]?' · Original saved':'');v.sub.classList.toggle('changed',active(a));
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
 let cursor=$('apps').firstElementChild;
 for(const a of all){let v=rows.get(a.id);if(!v){v=makeRow(a);rows.set(a.id,v);}v.root.hidden=!visibleIDs.has(a.id);updateRow(a,v);if(v.root!==cursor)$('apps').insertBefore(v.root,cursor);cursor=v.root.nextElementSibling;}
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
 const skinned=!!model.rebrands[a.bundleId];$('name-unskin').hidden=!skinned;$('fullskin-state').textContent=skinned?'Full skin applied (“'+model.rebrands[a.bundleId]+'”).':(chosen(a)?'Uses the icon currently chosen for this app.':'Choose an icon for this app first.');
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
$('support-paypal').addEventListener('click',()=>openSupport('paypal'));
$('support-github').addEventListener('click',()=>openSupport('github'));
$('dismiss').addEventListener('click',()=>tell(''));$('scan').addEventListener('click',refresh);$('search').addEventListener('input',()=>{query=$('search').value.trim().toLowerCase();selectFuture=false;render();});
$('select-all').addEventListener('change',()=>{selectFuture=$('select-all').checked;for(const a of visible()){if(selectFuture&&ready(a))selected.add(a.id);else selected.delete(a.id);}render();});
$('replace').addEventListener('click',()=>applyApps(visible().filter(a=>selected.has(a.id))));$('restore-all').addEventListener('click',restoreAll);
$('folder').addEventListener('click',async()=>{if(busy)return;busy=true;render();let added=false;try{const r=await api('root',{});added=!r.cancelled;}catch(e){tell(e.message,true);}finally{busy=false;render();}if(added)refresh();});
document.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();$('search').focus();}});
(async()=>{applyTheme('system');try{const s=await api('session');updateModel(s);if(!themeTouched)applyTheme(s.theme||'system');render();}catch(e){tell(e.message,true);}await refresh();})();
