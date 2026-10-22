/* Native macOS operations. Requests are data from a private JSON file, never code.
 * JXA wraps Objective-C nil as a truthy object. Test isNil(), not only !value.
 * Convert plist dictionaries to plain JavaScript once, before reading fields.
 */
ObjC.import('Foundation');
ObjC.import('AppKit');
ObjC.import('CoreServices');

function isNil(value) {
    return value === null || value === undefined ||
        (typeof value.isNil === 'function' && value.isNil());
}

function readDictionary(path) {
    const nativeValue = $.NSDictionary.dictionaryWithContentsOfFile($(path));
    if (isNil(nativeValue)) throw Error('Cannot read Info.plist (missing, unreadable or invalid)');
    const value = ObjC.deepUnwrap(nativeValue);
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
        throw Error('Info.plist must contain a dictionary');
    }
    return value;
}

function textValue(dictionary, key) {
    if (!Object.prototype.hasOwnProperty.call(dictionary, key)) return '';
    const value = dictionary[key];
    if (typeof value === 'string') return value;
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
    return '';
}

function validImage(image) { return !isNil(image) && Boolean(image.isValid); }

// LaunchServices C functions return CF refs, which JXA surfaces as opaque
// Ref objects; castRefToObject bridges them before unwrapping. Proven on
// macOS 26: the modern NSWorkspace default-app methods are invisible to
// JXA (frozen BridgeSupport), so the classic LS C API is the read route.
// Writes are refused from this unbundled process (-54 / silent no-ops)
// and live in the bundled iconcraft-defaults helper instead.
function cfString(ref) {
    if (isNil(ref)) return '';
    try {
        const value = ObjC.unwrap(ObjC.castRefToObject(ref));
        return typeof value === 'string' ? value : '';
    } catch (_) { return ''; }
}
function cfStringList(ref) {
    if (isNil(ref)) return [];
    const out = [];
    try {
        const list = ObjC.castRefToObject(ref);
        const count = Number(list.count);
        for (let i = 0; i < count; i++) {
            const item = ObjC.unwrap(list.objectAtIndex(i));
            if (typeof item === 'string' && item && out.indexOf(item) < 0) out.push(item);
        }
    } catch (_) { /* a partial list beats failing the whole request */ }
    return out;
}
function appPathForBundleId(w, bundleId) {
    try {
        const url = w.URLForApplicationWithBundleIdentifier($(bundleId));
        return isNil(url) ? '' : (ObjC.unwrap(url.path) || '');
    } catch (_) { return ''; }
}
function defaultHandler(c) {
    return c.kind === 'scheme'
        ? cfString($.LSCopyDefaultHandlerForURLScheme($(c.scheme)))
        : cfString($.LSCopyDefaultRoleHandlerForContentType($(c.uti), $.kLSRolesAll));
}
function handlerCandidates(c) {
    return c.kind === 'scheme'
        ? cfStringList($.LSCopyAllHandlersForURLScheme($(c.scheme)))
        : cfStringList($.LSCopyAllRoleHandlersForContentType($(c.uti), $.kLSRolesAll));
}

function imagePNG(image) {
    if (!validImage(image)) throw Error('macOS could not decode the icon');
    const tiff = image.TIFFRepresentation;
    if (isNil(tiff)) throw Error('No raster data is available for this icon');
    const rep = $.NSBitmapImageRep.imageRepWithData(tiff);
    if (isNil(rep)) throw Error('No raster representation is available');
    const data = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $({}));
    if (isNil(data)) throw Error('PNG conversion failed');
    return ObjC.unwrap(data.base64EncodedStringWithOptions(0));
}

// Batch previews avoid starting an osascript process for each visible row.
function thumbnailPNG(image) {
    if (!validImage(image)) throw Error('No app icon available');
    const small = $.NSImage.alloc.initWithSize($.NSMakeSize(96, 96));
    small.lockFocus;
    try { image.drawInRectFromRectOperationFraction($.NSMakeRect(0,0,96,96), $.NSZeroRect, $.NSCompositingOperationCopy, 1.0); }
    finally { small.unlockFocus; }
    return imagePNG(small);
}

function run(argv) {
    let action = 'request';
    try {
        const readError = $();
        const data = $.NSString.stringWithContentsOfFileEncodingError(argv[0], $.NSUTF8StringEncoding, readError);
        if (isNil(data)) throw Error('Could not read the private request');
        const r = JSON.parse(ObjC.unwrap(data));
        action = r.action;
        const w = $.NSWorkspace.sharedWorkspace;
        let result;
        switch (r.action) {
        case 'inspect':
            // A broken/uninstalled bundle must never discard other applications.
            result = { apps: [], warnings: [] };
            r.paths.forEach(function (p) {
                try {
                    const d = readDictionary(p + '/Contents/Info.plist');
                    const bundleId = textValue(d, 'CFBundleIdentifier');
                    const executable = textValue(d, 'CFBundleExecutable');
                    if (!bundleId || !executable) throw Error('Incomplete bundle metadata: identifier or executable is missing');
                    result.apps.push({
                        path: p,
                        name: textValue(d, 'CFBundleDisplayName') || textValue(d, 'CFBundleName') || p.split('/').pop().replace(/\.app$/i, ''),
                        bundleId: bundleId,
                        version: textValue(d, 'CFBundleShortVersionString') || textValue(d, 'CFBundleVersion'),
                        executable: executable
                    });
                } catch (e) {
                    result.warnings.push(p + ': ' + String(e.message || e));
                }
            });
            break;
        case 'thumbnails':
            result = {};
            r.paths.forEach(function(p) {
                try { result[p] = thumbnailPNG(w.iconForFile($(p))); }
                catch (_) { try { result[p] = imagePNG(w.iconForFile($(p))); } catch (_) { /* isolate broken bundle */ } }
            });
            break;
        case 'running':
            result = ObjC.unwrap(w.runningApplications).some(function(a) {
                try { return !isNil(a.bundleURL) && ObjC.unwrap(a.bundleURL.path) === r.path; }
                catch (_) { return false; }
            });
            break;
        case 'set':
            const im = $.NSImage.alloc.initWithContentsOfFile($(r.icon));
            if (!validImage(im)) throw Error('Cannot decode selected icon');
            if (!w.setIconForFileOptions(im, $(r.path), 0)) {
                throw Error('macOS refused the change. Check write access and macOS Privacy & Security → App Management for IconCraft. No application contents were replaced.');
            }
            w.noteFileSystemChanged($(r.path));
            result = true;
            break;
        case 'export':
            let image = null;
            if (r.bundled) {
                // Some apps use asset catalogues rather than CFBundleIconFile.
                // A missing plist or icon file falls back to the macOS icon.
                try {
                    const d = readDictionary(r.path + '/Contents/Info.plist');
                    let file = textValue(d, 'CFBundleIconFile');
                    if (file && file.indexOf('/') < 0 && file.indexOf('..') < 0) {
                        if (!/\.icns$/i.test(file)) file += '.icns';
                        image = $.NSImage.alloc.initWithContentsOfFile($(r.path + '/Contents/Resources/' + file));
                    }
                } catch (_) { /* The public NSWorkspace icon is the fallback. */ }
            }
            if (!validImage(image)) image = w.iconForFile($(r.path));
            result = imagePNG(image);
            break;
        case 'normalize':
            result = imagePNG($.NSImage.alloc.initWithContentsOfFile($(r.path)));
            break;
        case 'notify':
            w.noteFileSystemChanged($(r.path));
            result = true;
            break;
        case 'defaults':
            result = (r.categories || []).map(function (c) {
                const current = defaultHandler(c);
                const candidates = handlerCandidates(c);
                if (current && candidates.indexOf(current) < 0) candidates.push(current);
                const paths = {};
                candidates.forEach(function (id) {
                    const p = appPathForBundleId(w, id);
                    if (p) paths[id] = p;
                });
                return { id: c.id, current: current, candidates: candidates, paths: paths };
            });
            break;
        case 'openurl':
            // Defence in depth: Go also allowlists these two exact URLs.
            if (r.url !== 'https://www.paypal.com/ncp/payment/E8YDD8BNAKVJA' &&
                r.url !== 'https://github.com/gshipman223/IconCraft' &&
                r.url !== 'x-apple.systempreferences:com.apple.preference.security?Privacy_AppBundles') {
                throw Error('External link is not on IconCraft\'s allowlist');
            }
            if (!w.openURL($.NSURL.URLWithString($(r.url)))) throw Error('macOS could not open the link in your browser');
            result = true;
            break;
        case 'choose':
            const app = $.NSApplication.sharedApplication;
            app.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
            app.activateIgnoringOtherApps(true);
            const panel = $.NSOpenPanel.openPanel;
            panel.setAllowsMultipleSelection(false);
            panel.setCanChooseDirectories(r.kind === 'folder');
            panel.setCanChooseFiles(r.kind !== 'folder');
            panel.setResolvesAliases(true);
            // Go supplies the title in the UI language; English is the fallback.
            panel.setTitle(typeof r.title === 'string' && r.title ? r.title : (r.kind === 'folder' ? 'Choose a folder' : r.kind === 'profile' ? 'Choose an IconCraft profile' : 'Choose an original icon'));
            if (r.kind === 'profile') panel.setAllowedFileTypes($(['json']));
            else if (r.kind !== 'folder') panel.setAllowedFileTypes($(['png', 'icns']));
            result = Number(panel.runModal) === Number($.NSModalResponseOK) ? ObjC.unwrap(panel.URL.path) : '';
            break;
        default:
            throw Error('Unsupported native operation');
        }
        return JSON.stringify({ ok: true, result: result });
    } catch (e) {
        return JSON.stringify({ ok: false, error: String(e.message || e), action: action });
    }
}
