# Native Mac acceptance checklist (not executed on Linux)

1. Install 0.4.0 beside the prior application using the supplied reversible
   installer. Existing icons, original assets and restore records must remain.
2. Relaunch with saved inventory: rows appear before online refresh finishes.
   Refresh again after installing/removing an app and verify the inventory.
3. Test a first-launch original download with working internet. Disconnect and
   relaunch: saved original icons remain usable and are labelled as saved.
4. In System mode change macOS appearance; then select Light and Dark overrides.
   Relaunch to confirm the selection is remembered.
5. Click the grid control: it opens only an in-window preview panel. Select a
   donor icon; the donor must not launch. File selection cancellation changes
   nothing. Close the picker with Escape or its close button.
6. Replace a disposable/test app's icon. Verify Finder and then the Dock after
   reopening it. Restore returns its exact previous custom/default icon. Test
   individual and selected/bulk operations, retaining exact failure messages.
7. Quit a test app, rename it to a custom or original-counterpart name. Verify
   the .app folder moved but signed contents stayed unchanged. Restore its name.
   Test a duplicate target filename (must be refused) and a running target
   (must be refused). Verify icon restoration still works after a rename.
8. Restore all reverses both icon and name records. No other app is modified.

Interactive Finder/Dock behavior, local permissions, Gatekeeper prompts,
accessibility and live publisher availability cannot be proven by Linux mock
checks or a cross-compiled Mach-O bundle. Run these on the target Mac.

## App Management error split — NOT RUN

Linux tests classify injected errno values. They do not show what macOS 27
returns. On a Mac (the maintainer’s machine is macOS 27.0.1, Apple Silicon;
issue #10 was macOS 27.2 beta RC):

1. App Management off, user-owned app: the message must say “operation not
   permitted”, report that result as App Management, and offer Open App
   Management settings.
2. App Management on for this exact IconCraft binary, same app, after a
   full quit: record whether the probe then succeeds. If it still says
   operation not permitted, the message must say the switch is not the
   remaining cause, and this change does not unblock that Mac. Compare
   `CreateTemp` inside the bundle with `NSWorkspace.setIcon` on a
   disposable app — the probe is a temp file, the real change is setIcon,
   and those can disagree. setIcon returning false must not say App
   Management.
3. Root-owned app (`ls -ld` uid 0), App Management off and on: record
   EACCES versus EPERM. Do not assume root ownership is EACCES until this
   Mac says so. An EACCES result must say permission denied or owned by
   root, title the Setup row “Write access — changing other apps”, and
   must not offer the Open App Management button.
4. Setup’s automatic check skips a root-owned app that sorts first and
   can pass on a later user-owned app that is not running. The row names
   the app that was tested.
5. An app on a read-only disk image says it is on a read-only disk, is
   “Could not test”, and does not offer the App Management button.
