/* Isolated native AppKit/WebKit window for the authenticated local UI. */
function run(argv) {
    var stage = 'loading macOS frameworks';
    try {
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
    appMenu.addItemWithTitleActionKeyEquivalent($('Quit IconCraft'),'terminate:',$('q'));
    top.setSubmenu(appMenu);menu.addItem(top);
    const editTop=$.NSMenuItem.alloc.init;
    const edit=$.NSMenu.alloc.initWithTitle($('Edit'));
    [['Undo','undo:','z'],['Cut','cut:','x'],['Copy','copy:','c'],['Paste','paste:','v'],['Select All','selectAll:','a']].forEach(function(v){edit.addItemWithTitleActionKeyEquivalent($(v[0]),v[1],$(v[2]));});
    editTop.setSubmenu(edit);menu.addItem(editTop);app.setMainMenu(menu);
    stage = 'creating the window';
    const bounds=$.NSMakeRect(0,0,940,640);
    const mask=$.NSWindowStyleMaskTitled|$.NSWindowStyleMaskClosable|$.NSWindowStyleMaskMiniaturizable|$.NSWindowStyleMaskResizable;
    const win=$.NSWindow.alloc.initWithContentRectStyleMaskBackingDefer(bounds,mask,$.NSBackingStoreBuffered,false);
    win.setTitle($('IconCraft'));win.setMinSize($.NSMakeSize(720,450));win.setReleasedWhenClosed(false);
    const delegate=$.IconCraftWindowDelegate.alloc.init;win.setDelegate(delegate);
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
    win.center;win.makeKeyAndOrderFront(null);app.activateIgnoringOtherApps(true);
    console.log('IconCraft: native window created');
    stage = 'running the window';
    app.run;
    } catch (error) {
        throw new Error('IconCraft could not open (' + stage + '): ' + String(error));
    }
}

