/* Isolated native AppKit/WebKit window for the authenticated local UI. */
function run(argv) {
    var stage = 'loading macOS frameworks';
    try {
    // Menu titles arrive from Go as an `i18n=<json>` argument, already in
    // the UI language; the English literals below are the fallback so the
    // window still opens if that argument is missing or malformed.
    var L = (function () {
        var table = {};
        try {
            for (var i = 0; i < argv.length; i++) {
                if (typeof argv[i] === 'string' && argv[i].indexOf('i18n=') === 0) table = JSON.parse(argv[i].slice(5)) || {};
            }
        } catch (e) { table = {}; }
        return function (key, fallback) { return typeof table[key] === 'string' && table[key] ? table[key] : fallback; };
    })();
    ObjC.import('Foundation');ObjC.import('AppKit');ObjC.import('WebKit');
    stage = 'creating the native application';
    const app=$.NSApplication.sharedApplication;
    $.NSProcessInfo.processInfo.setProcessName($('IconCraft'));
    app.setActivationPolicy($.NSApplicationActivationPolicyRegular);
    stage = 'registering the close-window handler';
    ObjC.registerSubclass({name:'IconCraftWindowDelegate',superclass:'NSObject',protocols:['NSWindowDelegate'],methods:{
        'windowWillClose:':{types:['void',['id']],implementation:function(){app.terminate(null);}}
    }});
    const menu=$.NSMenu.alloc.init;
    const top=$.NSMenuItem.alloc.init;
    const appMenu=$.NSMenu.alloc.initWithTitle($('IconCraft'));
    appMenu.addItemWithTitleActionKeyEquivalent($(L('menu.quit', 'Quit IconCraft')),'terminate:',$('q'));
    top.setSubmenu(appMenu);menu.addItem(top);
    const editTop=$.NSMenuItem.alloc.init;
    const edit=$.NSMenu.alloc.initWithTitle($(L('menu.edit', 'Edit')));
    [[L('menu.undo', 'Undo'),'undo:','z'],[L('menu.cut', 'Cut'),'cut:','x'],[L('menu.copy', 'Copy'),'copy:','c'],[L('menu.paste', 'Paste'),'paste:','v'],[L('menu.selectAll', 'Select All'),'selectAll:','a']].forEach(function(v){edit.addItemWithTitleActionKeyEquivalent($(v[0]),v[1],$(v[2]));});
    editTop.setSubmenu(edit);menu.addItem(editTop);app.setMainMenu(menu);
    // Menu bar (status) item with quick controls. The persistence keeper is
    // a LaunchAgent and keeps repairing skins even after IconCraft quits;
    // this item is a remote control for it while the app is open, never a
    // requirement. Any failure here must not stop the window from opening.
    try {
        const base = argv[0].endsWith('/') ? argv[0] : argv[0] + '/';
        let keeperOn = false;
        const httpJSON = function(method, path2, body) {
            const req = $.NSMutableURLRequest.requestWithURL($.NSURL.URLWithString($(base + path2)));
            req.setHTTPMethod($(method)); req.setTimeoutInterval(5);
            if (body) { req.setValueForHTTPHeaderField($('application/json'), $('Content-Type')); req.setHTTPBody($(JSON.stringify(body)).dataUsingEncoding($.NSUTF8StringEncoding)); }
            const data = $.NSURLConnection.sendSynchronousRequestReturningResponseError(req, null, null);
            if (!data || (typeof data.isNil === 'function' && data.isNil())) return null;
            return JSON.parse(ObjC.unwrap($.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding)));
        };
        ObjC.registerSubclass({name:'IconCraftStatusDelegate',superclass:'NSObject',protocols:['NSMenuDelegate'],methods:{
            'openApp:':{types:['void',['id']],implementation:function(){ win.makeKeyAndOrderFront(null); app.activateIgnoringOtherApps(true); }},
            'openSetup:':{types:['void',['id']],implementation:function(){ win.makeKeyAndOrderFront(null); app.activateIgnoringOtherApps(true); try { web.loadRequest($.NSURLRequest.requestWithURL($.NSURL.URLWithString($(base + '#setup')))); } catch (e) {} }},
            'toggleKeeper:':{types:['void',['id']],implementation:function(){ try { const r = httpJSON('POST', 'api/keepalive', {enabled: !keeperOn}); if (r && typeof r.enabled === 'boolean') keeperOn = r.enabled; } catch (e) {} refreshKeeper(); }},
            'openSettings:':{types:['void',['id']],implementation:function(){ $.NSWorkspace.sharedWorkspace.openURL($.NSURL.URLWithString($('x-apple.systempreferences:com.apple.preference.security?Privacy_AppBundles'))); }},
            'openSupport:':{types:['void',['id']],implementation:function(){ $.NSWorkspace.sharedWorkspace.openURL($.NSURL.URLWithString($('https://www.paypal.com/ncp/payment/E8YDD8BNAKVJA'))); }},
            'menuWillOpen:':{types:['void',['id']],implementation:function(){ refreshKeeper(); }},
            'pollSettings:':{types:['void',['id']],implementation:function(){ refreshKeeper(); }}
        }});
        // Function-scoped (var, not block-scoped const): JXA may collect
        // block-scoped ObjC wrappers once the block exits, which would make
        // the status item vanish and its delegate stop responding.
        var statusDelegate = $.IconCraftStatusDelegate.alloc.init;
        var statusMenu = $.NSMenu.alloc.init;
        const openItem = statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.open', 'Open IconCraft')), 'openApp:', $('')); openItem.setTarget(statusDelegate);
        const setupItem = statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.setup', 'Setup & Permissions…')), 'openSetup:', $('')); setupItem.setTarget(statusDelegate);
        var keeperItem = statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.keep', 'Keep Skins Applied')), 'toggleKeeper:', $('')); keeperItem.setTarget(statusDelegate);
        var keeperState = $.NSMenuItem.alloc.initWithTitleActionKeyEquivalent($(L('menu.keeperStatus', 'Background keeper status')), null, $('')); keeperState.setEnabled(false); statusMenu.addItem(keeperState);
        statusMenu.addItem($.NSMenuItem.separatorItem);
        const settingsItem = statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.appManagement', 'App Management Settings…')), 'openSettings:', $('')); settingsItem.setTarget(statusDelegate);
        const supportItem = statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.support', 'Support IconCraft ♥')), 'openSupport:', $('')); supportItem.setTarget(statusDelegate);
        statusMenu.addItem($.NSMenuItem.separatorItem);
        statusMenu.addItemWithTitleActionKeyEquivalent($(L('menu.quit', 'Quit IconCraft')), 'terminate:', $('q'));
        statusMenu.setDelegate(statusDelegate);
        var refreshKeeper = function() {
            try {
                const s = httpJSON('GET', 'api/session', null);
                if (s && typeof s.keepAlive === 'boolean') keeperOn = s.keepAlive;
                // Visibility preferences apply live: Dock presence is the
                // app activation policy, menu bar presence is the status
                // item's visibility. The server guarantees at least one
                // of the two is always on.
                if (s && s.settings) {
                    try { app.setActivationPolicy(s.settings.showDockIcon ? $.NSApplicationActivationPolicyRegular : $.NSApplicationActivationPolicyAccessory); } catch (e) {}
                    try { statusItem.setVisible(!!s.settings.showMenuBarIcon); } catch (e) {}
                }
            } catch (e) {}
            keeperItem.setTitle($(keeperOn ? L('menu.keepOn', 'Keep Skins Applied: On') : L('menu.keepOff', 'Keep Skins Applied: Off')));
            keeperItem.setState(keeperOn ? 1 : 0);
            keeperState.setTitle($(keeperOn ? L('menu.keeperOn', 'Background keeper: on — works with IconCraft closed') : L('menu.keeperOff', 'Background keeper: off')));
        };
        // Icon first: if the button can't be dressed, bail before installing.
        var statusItem = $.NSStatusBar.systemStatusBar.statusItemWithLength(-1);
        try {
            const img = $.NSImage.alloc.initWithContentsOfFile($(argv[1]));
            if (img && !img.isNil()) { img.setSize($.NSMakeSize(18, 18)); statusItem.button.setImage(img); }
            else statusItem.button.setTitle($('◆'));
        } catch (e) { statusItem.button.setTitle($('◆')); }
        statusItem.setHighlightMode(true);
        statusItem.setMenu(statusMenu);
        refreshKeeper();
        // Poll so Settings changes (Dock icon, menu bar icon) apply
        // without restarting. var-retained like the other ObjC wrappers.
        // The selector must be a plain JS string: JXA converts it to a SEL,
        // while a bridged NSString ($('pollSettings:')) is not converted
        // and the timer never fires.
        var settingsTimer = $.NSTimer.scheduledTimerWithTimeIntervalTargetSelectorUserInfoRepeats(10, statusDelegate, 'pollSettings:', null, true);
    } catch (statusError) { console.log('IconCraft: menu bar item unavailable'); }
    stage = 'creating the window';
    const bounds=$.NSMakeRect(0,0,940,640);
    const mask=$.NSWindowStyleMaskTitled|$.NSWindowStyleMaskClosable|$.NSWindowStyleMaskMiniaturizable|$.NSWindowStyleMaskResizable;
    // var, not const: the window delegate is held weakly by AppKit, and a
    // collected JXA wrapper would silently stop windowWillClose (the app
    // would not quit when its window closes).
    var win=$.NSWindow.alloc.initWithContentRectStyleMaskBackingDefer(bounds,mask,$.NSBackingStoreBuffered,false);
    win.setTitle($('IconCraft'));win.setMinSize($.NSMakeSize(720,450));win.setReleasedWhenClosed(false);
    var delegate=$.IconCraftWindowDelegate.alloc.init;win.setDelegate(delegate);
    // Appearance is applied inside the page using CSS. Do not make starting
    // the window depend on a WKScriptMessageHandler/Objective-C callback.
    stage = 'creating the WebKit view';
    const config=$.WKWebViewConfiguration.alloc.init;
    config.setWebsiteDataStore($.WKWebsiteDataStore.nonPersistentDataStore);
    const web=$.WKWebView.alloc.initWithFrameConfiguration(bounds,config);
    web.setAutoresizingMask($.NSViewWidthSizable|$.NSViewHeightSizable);
    win.setContentView(web);
    stage = 'loading the local interface';
    const req=$.NSURLRequest.requestWithURL($.NSURL.URLWithString($(argv[0])));
    web.loadRequest(req);
    // A cosmetic icon failure must not prevent the window from opening.
    try {
        if(argv[1]){const icon=$.NSImage.alloc.initWithContentsOfFile($(argv[1]));if(icon && !icon.isNil()) app.setApplicationIconImage(icon);}
    } catch (iconError) { console.log('IconCraft: optional Dock artwork unavailable'); }
    stage = 'showing the window';
    if (argv[2] === 'menu-only') {
        // Launch-at-login in menu bar mode: the window exists but stays
        // hidden until the user opens it from the menu bar item.
        win.center;
        console.log('IconCraft: started in menu bar');
    } else {
        win.center;win.makeKeyAndOrderFront(null);app.activateIgnoringOtherApps(true);
        console.log('IconCraft: native window created');
    }
    stage = 'running the window';
    app.run;
    } catch (error) {
        throw new Error('IconCraft could not open (' + stage + '): ' + String(error));
    }
}

