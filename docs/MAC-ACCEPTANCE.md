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
