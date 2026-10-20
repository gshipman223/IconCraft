# 0.4.1 — launch repair

IconCraft 0.4.1 — Mac launch repair build
Apple Silicon + Intel, macOS 12.3 or later

INSTALL
Extract the ZIP and run Install IconCraft.command beside IconCraft.app.
Quit any running IconCraft first. The installer preserves the old application,
your settings, imported icons, original-icon recovery data and rename history.
It validates checksums, applies a local ad-hoc signature and checks the new
executable's version on your Mac before replacing the previous application.
No Apple developer account, Xcode or Homebrew is required.

WHAT CHANGED
The native appearance callback added in 0.4.0 is removed from window startup.
The main page still has System, Light and Dark controls and the same compact
layout, original-icon discovery, custom picker, bulk actions and renaming.
The native titlebar follows the Mac's system appearance independently of the
page's manual override. A Dock-artwork failure no longer prevents opening.

Startup failures now retain the native error and offer a small Show error
button. This is an error message, not a new diagnostics page. The report is
saved to ~/Library/Application Support/IconCraft/startup-error.txt. Nothing
resets the existing settings or recovery files.

If the app cannot open, click Show error and send the text it opens.
If no dialog appears, the existing startup log can be read in Terminal with:
tail -n 60 "$HOME/Library/Application Support/IconCraft/iconcraft.log"

The exact cause of the reported 0.4.0 failure is not confirmed: no target-Mac
launch log was available. This repair removes a startup dependency introduced
in that release and improves failure visibility. It is not a claim of native
macOS acceptance testing. See BUILD-VALIDATION.json for checks actually run.

Developer ID signing and notarization have not been performed. The installer
signs locally and does not disable Gatekeeper or remove quarantine attributes.
If macOS blocks the download, use Privacy & Security > Open Anyway to approve
this item. Do not turn off macOS security globally.
