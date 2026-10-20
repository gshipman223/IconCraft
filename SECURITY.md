# Security

## Supported versions

Only the latest release (currently **0.5.1**) receives security fixes.

## Reporting a vulnerability

Please **do not** open a public issue for security problems. Use GitHub's
private vulnerability reporting (**Security → Report a vulnerability**) on
this repository, or contact the maintainer privately (@gshipman223).

Do not attach imported proprietary artwork, full recovery journals, signing
keys, session tokens, or personal app paths to any report.

## Security model (summary)

- The backend binds only to `127.0.0.1` with a fresh 256-bit per-launch
  session token; Host and Origin are validated; cross-origin requests are
  refused; responses carry a restrictive CSP; request sizes are bounded.
- There is no generic command-execution or arbitrary-file API. UI-supplied
  IDs resolve only against the backend's own app/asset inventory. External
  links the UI can open are limited to an allowlist (the project's PayPal
  contribution page and this repository), enforced in both Go and the
  native bridge.
- Icon changes are journalled *before* mutation, bundle identity and code
  signatures are verified, and restores check bundle contents and current
  icon state. Symlinked bundles/executables/icon files, protected system
  paths, invalid or active-SVG artwork, oversized responses and redirects
  outside approved publisher origins are rejected.
- Known limits: races with external app updaters and same-user local
  attackers are reduced, not eliminated. The build is ad-hoc signed, not
  Developer-ID signed or notarised.
