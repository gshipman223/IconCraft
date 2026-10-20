# 0.4.0 — compact / fast / personal

Implemented: cached first paint, lightweight state, incremental metadata scans,
keyed rows, disk thumbnails, batched native thumbnail export, request coalescing,
HTTP ETags, parallel official-source lookup with bounded deadlines and cancellation,
System/Light/Dark appearance, compact in-window donor picker, per-app rename and
restore-name, automatic name recovery, and better inline action errors.

## Native contracts

- The outer bundle filename is renamed, not CFBundleDisplayName or signed files.
- Darwin uses exclusive `renameatx_np` (XNU syscall 488), `AT_FDCWD=-2`,
  `RENAME_EXCL=4` to prevent overwriting a destination that appears mid-operation.
- The appearance bridge accepts exactly system/light/dark. It does not evaluate
  script received through messages.
- Native image export uses public NSWorkspace/NSImage APIs. Preview failures are
  isolated to one app; genuine replacement images are not downscaled to the
  96px thumbnails used for display.

Reference documentation used during implementation (not a claim of testing on Mac):

- Apple NSWorkspace: https://developer.apple.com/documentation/appkit/nsworkspace
- Apple bundle keys: https://developer.apple.com/library/archive/documentation/General/Reference/InfoPlistKeyReference/Articles/CoreFoundationKeys.html
- Apple XNU syscall contract: https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/syscalls.master
- Official source examples: https://www.adobe.com/products/photoshop.html ;
  https://www.figma.com/downloads/ ; https://www.microsoft.com/en-us/microsoft-365/word

## Test boundaries

Automated core tests manipulate isolated fixture bundles on Linux. The native
bridge suite simulates Objective-C wrapper contracts; UI tests run in Chromium
with all native operations and downloads simulated. Both actual Darwin executable
architectures and native test executables are cross-compiled, not run here.
Publisher retrieval from the user's Mac and the user's unspecified replacement
failure have not been reproduced. This release exposes the exact error inline.

UI screenshots in this source tree deliberately use the same neutral test image
rather than imitations of commercial logos. They illustrate layout only.
