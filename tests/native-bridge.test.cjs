'use strict';
// Contract/regression tests using simulated Objective-C values. These do NOT
// claim to execute macOS frameworks. The same bridge is exercised on macOS by
// the startup diagnostics and metadata_darwin_test.go.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../internal/native/bridge.js'), 'utf8');
const metadata = { CFBundleIdentifier: 'ai.storyteller.photocraft', CFBundleName: 'PhotoCraft', CFBundleExecutable: 'photocraft', CFBundleShortVersionString: '1.2' };

function harness(options = {}) {
    const nil = { isNil: () => true }; // Truthy: the precise edge case missed before.
    const wrapper = value => ({ __native: value, isNil: () => false });
    const unwrap = value => value === nil ? undefined : value && Object.hasOwn(value, '__native') ? value.__native : value;
    const calls = [];
    const image = { isNil: () => false, isValid: true, TIFFRepresentation: wrapper('tiff') };
    const $ = value => value === undefined ? nil : wrapper(value);
    $.NSDictionary = { dictionaryWithContentsOfFile: name => {
        const p = unwrap(name);
        calls.push(['dictionary', p]);
        if (options.throwDictionary) throw Error('Simulated metadata read failure');
        const value = (options.plists || {})[p];
        if (value === undefined) return nil;
        return options.plainDictionary ? value : wrapper(value);
    }};
    $.NSString = { stringWithContentsOfFileEncodingError: () => options.missingRequest ? nil : wrapper(JSON.stringify(options.request || {action: 'inspect', paths: []})) };
    $.NSWorkspace = { sharedWorkspace: {
        iconForFile: p => { calls.push(['workspaceIcon', unwrap(p)]); return options.noWorkspaceIcon ? nil : image; },
        setIconForFileOptions: (im, p) => { calls.push(['set', unwrap(p)]); return !options.refuseSet; },
        noteFileSystemChanged: p => calls.push(['notify', unwrap(p)])
    }};
    $.NSImage = { alloc: { initWithContentsOfFile: p => { calls.push(['loadImage', unwrap(p)]); return options.fileImage ? image : nil; } } };
    $.NSBitmapImageRep = { imageRepWithData: () => options.noRepresentation ? nil : {
        representationUsingTypeProperties: () => options.noPNG ? nil : { base64EncodedStringWithOptions: () => wrapper('aWNvbg==') }
    }};
    $.NSUTF8StringEncoding = 4;
    $.NSBitmapImageFileTypePNG = 4;
    if (options.ls) {
        // Simulated LaunchServices: CF-string returns ride in wrappers,
        // CF arrays expose count/objectAtIndex like a bridged NSArray.
        const ls = options.ls;
        const refArr = a => ({ isNil: () => false, count: a.length, objectAtIndex: i => wrapper(a[i]) });
        const key = (kind, id) => kind + ':' + id;
        $.kLSRolesAll = -1;
        $.LSCopyDefaultHandlerForURLScheme = s => ls.current[key('scheme', unwrap(s))] ? wrapper(ls.current[key('scheme', unwrap(s))]) : nil;
        $.LSCopyDefaultRoleHandlerForContentType = u => ls.current[key('uti', unwrap(u))] ? wrapper(ls.current[key('uti', unwrap(u))]) : nil;
        $.LSCopyAllHandlersForURLScheme = s => refArr(ls.candidates[key('scheme', unwrap(s))] || []);
        $.LSCopyAllRoleHandlersForContentType = u => refArr(ls.candidates[key('uti', unwrap(u))] || []);
        $.LSSetDefaultHandlerForURLScheme = (s, b) => { calls.push(['setScheme', unwrap(s), unwrap(b)]); if (ls.setStatus) return ls.setStatus; ls.current[key('scheme', unwrap(s))] = unwrap(b); return 0; };
        $.LSSetDefaultRoleHandlerForContentType = (u, r, b) => { calls.push(['setUTI', unwrap(u), unwrap(b)]); if (ls.setStatus) return ls.setStatus; ls.current[key('uti', unwrap(u))] = unwrap(b); return 0; };
        $.NSWorkspace.sharedWorkspace.URLForApplicationWithBundleIdentifier = b => { const p = (ls.paths || {})[unwrap(b)]; return p ? { isNil: () => false, path: wrapper(p) } : nil; };
    }
    const ctx = vm.createContext({ $, ObjC: { import: () => {}, unwrap, deepUnwrap: unwrap, castRefToObject: v => v } });
    vm.runInContext(source, ctx, {filename: 'embedded-native-bridge.js'});
    return { run: () => JSON.parse(ctx.run(['/private/request.json'])), calls, nil, ctx };
}

function inspect(plists, extra = {}) {
    const paths = Object.keys(plists).map(p => p.replace('/Contents/Info.plist', ''));
    return harness({ plists, request: {action: 'inspect', paths}, ...extra }).run();
}

test('missing dictionary reproduces the old objectForKey crash and is now isolated', () => {
    const h = harness({request: {action: 'inspect', paths: ['/Missing.app']}});
    const d = h.nil;
    assert.ok(d, 'JXA nil wrapper is truthy');
    assert.throws(() => d.objectForKey('CFBundleName'), /objectForKey is not a function/);
    const result = h.run();
    assert.equal(result.ok, true);
    assert.equal(result.result.apps.length, 0);
    assert.equal(result.result.warnings.length, 1);
    assert.match(result.result.warnings[0], /Missing.app.*Info.plist/);
});

test('wrapped valid metadata is unwrapped before field access', () => {
    const result = inspect({'/PhotoCraft.app/Contents/Info.plist': metadata});
    assert.equal(result.ok, true);
    assert.deepEqual(result.result.apps[0], {path: '/PhotoCraft.app', name: 'PhotoCraft', bundleId: metadata.CFBundleIdentifier, executable: 'photocraft', version: '1.2'});
    assert.deepEqual(result.result.warnings, []);
});

test('plain dictionary values do not require an Objective-C objectForKey method', () => {
    const result = inspect({'/PhotoCraft.app/Contents/Info.plist': metadata}, {plainDictionary: true});
    assert.equal(result.result.apps[0].name, 'PhotoCraft');
});

test('a broken bundle does not discard valid applications before or after it', () => {
    const h = harness({plists: {'/Before.app/Contents/Info.plist': metadata, '/After.app/Contents/Info.plist': metadata}, request: {action: 'inspect', paths: ['/Before.app', '/Broken.app', '/After.app']}});
    const result = h.run();
    assert.equal(result.ok, true);
    assert.deepEqual(result.result.apps.map(a=>a.path), ['/Before.app', '/After.app']);
    assert.equal(result.result.warnings.length, 1);
});

test('display name takes precedence and numeric version is retained', () => {
    const m = {...metadata, CFBundleDisplayName: 'Photo Craft – 日本語', CFBundleShortVersionString: '', CFBundleVersion: 42};
    const r = inspect({'/PhotoCraft.app/Contents/Info.plist': m});
    assert.equal(r.result.apps[0].name, m.CFBundleDisplayName);
    assert.equal(r.result.apps[0].version, '42');
});

test('missing names fall back to the app filename', () => {
    const m = {...metadata, CFBundleName: '', CFBundleDisplayName: []};
    const r = inspect({'/Fallback.APP/Contents/Info.plist': m});
    assert.equal(r.result.apps[0].name, 'Fallback');
});

test('nested metadata is not coerced into text', () => {
    const m = {...metadata, CFBundleName: {unsafe: 'not a name'}, CFBundleVersion: ['not a version'], CFBundleShortVersionString: false};
    const r = inspect({'/Fallback.app/Contents/Info.plist': m});
    assert.equal(r.result.apps[0].name, 'Fallback');
    assert.equal(r.result.apps[0].version, '');
});

test('non-dictionary plist returns a per-app warning', () => {
    for (const value of [[], 'not a dictionary', null, 5]) {
        const r = inspect({'/Bad.app/Contents/Info.plist': value});
        assert.equal(r.ok, true);
        assert.equal(r.result.apps.length, 0);
        assert.equal(r.result.warnings.length, 1);
    }
});

test('incomplete app metadata is skipped visibly', () => {
    for (const key of ['CFBundleIdentifier', 'CFBundleExecutable']) {
        const m = {...metadata}; delete m[key];
        const r = inspect({'/Incomplete.app/Contents/Info.plist': m});
        assert.equal(r.result.apps.length, 0);
        assert.match(r.result.warnings[0], /Incomplete bundle/);
    }
});

test('native exceptions are isolated to the affected bundle', () => {
    const r = inspect({'/Bad.app/Contents/Info.plist': metadata}, {throwDictionary: true});
    assert.equal(r.ok, true);
    assert.match(r.result.warnings[0], /Simulated metadata read failure/);
});

test('empty scan is successful and explicit', () => {
    const r = harness().run();
    assert.deepEqual(r, {ok: true, result: {apps: [], warnings: []}});
});

test('icon export falls back when the plist is unavailable', () => {
    const h = harness({request: {action: 'export', path: '/Donor.app', bundled: true}});
    assert.deepEqual(h.run(), {ok: true, result: 'aWNvbg=='});
    assert.ok(h.calls.some(c=>c[0]==='workspaceIcon'));
});

test('missing bundled icon uses NSWorkspace instead of messaging a nil object', () => {
    const h = harness({plists: {'/Donor.app/Contents/Info.plist': {...metadata, CFBundleIconFile: 'AppIcon'}}, request: {action: 'export', path: '/Donor.app', bundled: true}});
    assert.equal(h.run().ok, true);
    assert.ok(h.calls.some(c=>c[0]==='loadImage' && c[1].endsWith('/AppIcon.icns')));
    assert.ok(h.calls.some(c=>c[0]==='workspaceIcon'));
});

test('real bundled artwork is preferred over the displayed icon', () => {
    const h = harness({fileImage: true, plists: {'/Donor.app/Contents/Info.plist': {...metadata, CFBundleIconFile: 'AppIcon.icns'}}, request: {action: 'export', path: '/Donor.app', bundled: true}});
    assert.equal(h.run().ok, true);
    assert.ok(!h.calls.some(c=>c[0]==='workspaceIcon'));
});

test('unsafe resource paths are not read', () => {
    for (const filename of ['../../other.icns', '/etc/secret', '..']) {
        const h = harness({plists: {'/Donor.app/Contents/Info.plist': {...metadata, CFBundleIconFile: filename}}, request: {action: 'export', path: '/Donor.app', bundled: true}});
        assert.equal(h.run().ok, true);
        assert.ok(!h.calls.some(c=>c[0]==='loadImage'));
    }
});

test('invalid icon fails before NSWorkspace can change an application', () => {
    const h = harness({request: {action: 'set', path: '/Target.app', icon: '/bad.icns'}});
    const r = h.run();
    assert.equal(r.ok, false);
    assert.equal(r.action, 'set');
    assert.match(r.error, /Cannot decode/);
    assert.ok(!h.calls.some(c=>c[0]==='set'));
});

test('set reports an explicit permission refusal', () => {
    const h = harness({fileImage: true, refuseSet: true, request: {action: 'set', path: '/Target.app', icon: '/good.icns'}});
    assert.match(h.run().error, /macOS refused/);
    assert.ok(!h.calls.some(c=>c[0]==='notify'));
});

test('successful set notifies only its target', () => {
    const h = harness({fileImage: true, request: {action: 'set', path: '/Target.app', icon: '/good.icns'}});
    assert.deepEqual(h.run(), {ok: true, result: true});
    assert.deepEqual(h.calls.filter(c=>c[0]==='notify'), [['notify','/Target.app']]);
});

test('missing request file is detected before reading properties of nil', () => {
    const h = harness({missingRequest: true});
    assert.deepEqual(h.run(), {ok: false, error: 'Could not read the private request', action: 'request'});
});

test('missing bitmap representation produces a controlled error', () => {
    const h = harness({noRepresentation: true, request: {action: 'export', path: '/Donor.app'}});
    assert.match(h.run().error, /No raster representation/);
});

test('failed PNG conversion produces a controlled error', () => {
    const h = harness({noPNG: true, request: {action: 'export', path: '/Donor.app'}});
    assert.match(h.run().error, /PNG conversion failed/);
});

test('missing workspace icon produces a controlled error', () => {
    const h = harness({noWorkspaceIcon: true, request: {action: 'export', path: '/Donor.app'}});
    assert.match(h.run().error, /macOS could not decode/);
});

test('unsupported native actions cannot execute arbitrary commands', () => {
    const h = harness({request: {action: 'exec', command: 'whoami'}});
    assert.match(h.run().error, /Unsupported native operation/);
    assert.equal(h.calls.length, 0);
});

test('embedded macOS window script parses before shipping', () => {
    const windowSource = fs.readFileSync(path.join(__dirname, '../internal/native/window.js'), 'utf8');
    assert.doesNotThrow(() => new vm.Script(windowSource, {filename:'window.js'}));
    assert.doesNotMatch(windowSource, /protocols:\s*\[.WKScriptMessageHandler/);
    assert.match(windowSource, /IconCraft could not open/);
});

test('thumbnail batch isolates unreadable files and uses export fallback', () => {
    const h = harness({request:{action:'thumbnails',paths:['/A.app','/B.app']}});
    const result=h.run();assert.equal(result.ok,true);
    assert.deepEqual(Object.keys(result.result),['/A.app','/B.app']);
    assert.equal(result.result['/A.app'],'aWNvbg==');
});

test('missing images produce an empty preview batch, not a global error', () => {
    const h=harness({noWorkspaceIcon:true,request:{action:'thumbnails',paths:['/Missing.app']}});
    assert.deepEqual(h.run(),{ok:true,result:{}});
});

test('defaults reads current handlers, candidates and app paths', () => {
    const ls = {
        current: {'scheme:https': 'com.example.alpha', 'uti:com.adobe.pdf': 'com.example.reader'},
        candidates: {'scheme:https': ['com.example.alpha', 'com.example.beta'], 'uti:com.adobe.pdf': ['com.example.reader']},
        paths: {'com.example.alpha': '/Applications/Alpha.app', 'com.example.beta': '/Applications/Beta.app', 'com.example.reader': '/Applications/Reader.app'}
    };
    const h = harness({ls, request: {action: 'defaults', categories: [
        {id: 'browser', kind: 'scheme', scheme: 'https', schemes: ['https', 'http']},
        {id: 'pdf', kind: 'uti', uti: 'com.adobe.pdf'}
    ]}});
    const r = h.run();
    assert.equal(r.ok, true);
    assert.deepEqual(r.result[0], {id: 'browser', current: 'com.example.alpha', candidates: ['com.example.alpha', 'com.example.beta'], paths: {'com.example.alpha': '/Applications/Alpha.app', 'com.example.beta': '/Applications/Beta.app'}});
    assert.equal(r.result[1].current, 'com.example.reader');
});

test('LaunchServices writes do not live in the bridge (macOS refuses them from unbundled processes)', () => {
    // Writes moved to the bundled iconcraft-defaults helper; the bridge
    // must stay read-only for defaults so nothing silently no-ops.
    const ls = { current: {'uti:com.adobe.pdf': 'com.example.reader'}, candidates: {}, paths: {}, setStatus: 0 };
    const h = harness({ls, request: {action: 'setdefault', bundleId: 'com.example.other', category: {id: 'pdf', kind: 'uti', uti: 'com.adobe.pdf'}}});
    const r = h.run();
    assert.equal(r.ok, false);
    assert.match(r.error, /Unsupported native operation/);
    assert.ok(!h.calls.some(c => c[0] === 'setUTI' || c[0] === 'setScheme'));
});
