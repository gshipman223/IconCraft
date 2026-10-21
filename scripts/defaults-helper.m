// iconcraft-defaults — bundled helper for changing macOS default apps.
//
// Why this exists: LaunchServices refuses default-handler writes from
// unbundled processes (the JXA bridge runs as /usr/bin/osascript and gets
// -54 / silent no-ops, both verified on macOS 26), and the supported
// NSWorkspace setters only work from a process inside an app bundle —
// they may also ask the user to confirm. This helper lives in
// IconCraft.app/Contents/MacOS so calls carry the app identity.
//
// Usage: iconcraft-defaults <uti|scheme> <key> <bundle-id>
// Prints one JSON line: {"current":"<bundle-id>"} after re-reading the
// live default, or {"error":"...","cancelled":true} and exits 1.
#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#import <CoreServices/CoreServices.h>

static BOOL finished;
static NSError *failure;

static void pump(void) {
    NSDate *limit = [NSDate dateWithTimeIntervalSinceNow:300];
    while (!finished && [[NSDate date] compare:limit] == NSOrderedAscending)
        [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                 beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
    if (!finished) {
        failure = [NSError errorWithDomain:@"IconCraft" code:1
                     userInfo:@{NSLocalizedDescriptionKey: @"Timed out waiting for macOS to answer."}];
    }
}

static NSString *currentScheme(NSString *scheme) {
    CFStringRef v = LSCopyDefaultHandlerForURLScheme((__bridge CFStringRef)scheme);
    return CFBridgingRelease(v) ?: @"";
}
static NSString *currentUTI(NSString *uti) {
    CFStringRef v = LSCopyDefaultRoleHandlerForContentType((__bridge CFStringRef)uti, kLSRolesAll);
    return CFBridgingRelease(v) ?: @"";
}

static void emit(NSString *json) { printf("%s\n", json.UTF8String); fflush(stdout); }
static NSString *jsonEscape(NSString *s) {
    s = [s stringByReplacingOccurrencesOfString:@"\\" withString:@"\\\\"];
    s = [s stringByReplacingOccurrencesOfString:@"\"" withString:@"\\\""];
    return [s stringByReplacingOccurrencesOfString:@"\n" withString:@" "];
}

int main(int argc, char **argv) { @autoreleasepool {
    if (argc != 4) { emit(@"{\"error\":\"usage: iconcraft-defaults <uti|scheme> <key> <bundle-id>\"}"); return 2; }
    NSString *kind = @(argv[1]), *key = @(argv[2]), *bundleID = @(argv[3]);
    NSWorkspace *ws = [NSWorkspace sharedWorkspace];
    NSURL *app = [ws URLForApplicationWithBundleIdentifier:bundleID];
    if (!app) {
        emit([NSString stringWithFormat:@"{\"error\":\"%@\"}", jsonEscape([NSString stringWithFormat:@"macOS cannot find the app %@.", bundleID])]);
        return 1;
    }
    finished = NO; failure = nil;
    void (^done)(NSError *) = ^(NSError *err) { failure = err; finished = YES; };
    if ([kind isEqualToString:@"scheme"]) {
        [ws setDefaultApplicationAtURL:app toOpenURLsWithScheme:key completionHandler:done];
    } else if ([kind isEqualToString:@"uti"]) {
        UTType *type = [UTType typeWithIdentifier:key];
        if (!type) { emit(@"{\"error\":\"Unknown file type.\"}"); return 1; }
        [ws setDefaultApplicationAtURL:app toOpenContentType:type completionHandler:done];
    } else { emit(@"{\"error\":\"Unknown kind.\"}"); return 2; }
    pump();
    if (failure) {
        BOOL cancelled = failure.code == -128 ||
            [failure.localizedDescription localizedCaseInsensitiveContainsString:@"cancel"];
        emit([NSString stringWithFormat:@"{\"error\":\"%@\",\"cancelled\":%@}",
              jsonEscape(failure.localizedDescription ?: @"macOS refused the change."),
              cancelled ? @"true" : @"false"]);
        return 1;
    }
    NSString *now = [kind isEqualToString:@"scheme"] ? currentScheme(key) : currentUTI(key);
    emit([NSString stringWithFormat:@"{\"current\":\"%@\"}", jsonEscape(now)]);
    return [now isEqualToString:bundleID] ? 0 : 1;
} }
