/* Contract tests only: this is a simulated bridge, NOT AppKit/macOS execution. */
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '../internal/native/window.js'), 'utf8');
function simulate(options = {}) {
    const calls=[], messages=[];
    const noop = (name) => (...args) => { calls.push(name); return undefined; };
    const app = { setActivationPolicy:noop('activation-policy'),setMainMenu:noop('menu'),setApplicationIconImage:noop('icon'),activateIgnoringOtherApps:noop('activate'),terminate:noop('terminate') };
    Object.defineProperty(app, 'run', {get(){calls.push('event-loop');}});
    const menu = {addItemWithTitleActionKeyEquivalent:noop('menu-item'),addItem:noop('menu-add')};
    const win = {setTitle:noop('title'),setMinSize:noop('size'),setReleasedWhenClosed:noop('release'),setDelegate:noop('delegate'),setContentView:noop('content'),makeKeyAndOrderFront:noop('show')};
    Object.defineProperty(win, 'center', {get(){calls.push('center');}});
    const config = {setWebsiteDataStore:noop('data-store')};
    // No WKScriptMessageHandler or native appearance APIs exist in this mock.
    const web = {setAutoresizingMask:noop('resize'),loadRequest:noop('load')};
    const $ = (x) => x;
    Object.assign($, {
        NSApplication:{sharedApplication:app}, NSProcessInfo:{processInfo:{setProcessName:noop('name')}},
        NSApplicationActivationPolicyRegular:0, NSMenu:{alloc:{init:menu,initWithTitle:()=>menu}},
        NSMenuItem:{alloc:{init:{setSubmenu:noop('submenu')}}},
        NSMakeRect:(...args)=>args, NSMakeSize:(...args)=>args,
        NSWindowStyleMaskTitled:1,NSWindowStyleMaskClosable:2,NSWindowStyleMaskMiniaturizable:4,NSWindowStyleMaskResizable:8,NSBackingStoreBuffered:2,
        NSWindow:{alloc:{initWithContentRectStyleMaskBackingDefer(){if(options.windowFailure)throw Error('fixture window failure');return win;}}},
        WKWebViewConfiguration:{alloc:{init:config}}, WKWebsiteDataStore:{nonPersistentDataStore:{}},
        WKWebView:{alloc:{initWithFrameConfiguration(){if(options.webFailure)throw Error('fixture WebKit failure');return web;}}},
        NSViewWidthSizable:2,NSViewHeightSizable:16,
        NSURLRequest:{requestWithURL:(x)=>x}, NSURL:{URLWithString:(x)=>x},
        NSImage:{alloc:{initWithContentsOfFile(){if(options.iconFailure)throw Error('fixture icon failure');return {isNil:()=>false};}}}
    });
    const ObjC = {import(name){if(options.importFailure)throw Error('fixture framework failure');calls.push('import:'+name);},registerSubclass(def){calls.push('register:'+def.name);if(def.protocols.some(x=>x==='WKScriptMessageHandler'))throw Error('unsupported optional WebKit protocol');$[def.name]={alloc:{init:{}}};}};
    const context=vm.createContext({$,ObjC,console:{log:(x)=>messages.push(x)}});
    vm.runInContext(source,context,{filename:'window.js'});
    return {calls,messages,run:()=>context.run(['http://127.0.0.1:12345/private/','/IconCraft.app/Contents/Resources/icon.icns'])};
}

test('Window opens without native appearance or script-message protocol support',()=>{
    const h=simulate();h.run();assert.ok(h.calls.includes('show'));assert.ok(h.calls.includes('event-loop'));
    assert.deepEqual(h.calls.filter(x=>x.startsWith('register:')),['register:IconCraftWindowDelegate']);
});
test('Cosmetic Dock icon error cannot prevent main window from opening',()=>{
    const h=simulate({iconFailure:true});h.run();assert.ok(h.calls.includes('show'));assert.ok(h.messages.some(s=>s.includes('optional Dock artwork')));
});
test('Native window creation failure identifies its startup stage',()=>{
    const h=simulate({windowFailure:true});assert.throws(()=>h.run(),/creating the window.*fixture window failure/);
});
test('WebKit creation failure identifies its startup stage',()=>{
    const h=simulate({webFailure:true});assert.throws(()=>h.run(),/creating the WebKit view.*fixture WebKit failure/);
});
test('Framework import failure is caught inside the launcher',()=>{
    const h=simulate({importFailure:true});assert.throws(()=>h.run(),/loading macOS frameworks.*fixture framework failure/);
});
test('Ready marker is emitted only after showing and activating window',()=>{
    const h=simulate();h.run();assert.ok(h.messages.includes('IconCraft: native window created'));
    assert.ok(h.calls.indexOf('show')<h.calls.indexOf('event-loop'));
    assert.ok(h.calls.indexOf('activate')<h.calls.indexOf('event-loop'));
});
test('UI appearance no longer depends on a native script-message bridge',()=>{
    const ui=fs.readFileSync(path.join(__dirname,'../internal/server/ui/app.js'),'utf8');
    assert.doesNotMatch(ui,/messageHandlers.*appearance/);
    assert.match(ui,/document\.documentElement\.dataset\.theme/);
});
