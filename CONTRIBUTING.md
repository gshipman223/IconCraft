# Contributing to IconCraft

Thanks for helping! IconCraft is a small, safety-obsessed codebase: it
modifies users' installed applications, so correctness and reversibility
matter more than speed of development.

## Ground rules

1. **Never commit third-party artwork.** Icons in tests/fixtures must be
   neutral, self-generated images. Vendor artwork stays with its owners.
2. **Mappings must be honest.** Product-equivalence data must distinguish
   *verified* equivalents (evidence: bundle ID, publisher listing) from
   *suggestions*. A guessed icon is worse than "Not found".
3. **Don't claim Mac-native validation you didn't run.** Cross-compilation
   and Linux/CI tests are not native execution. State `NOT RUN` plainly;
   use the `--self-test` fixture suite on a real Mac for native claims.
4. **Backup/restore and trust-boundary changes need regression tests.**

## Dev setup

- macOS 12.3+ (for native work), a current Go toolchain, Node (for bridge
  contract tests), Python 3 (build/packaging scripts).
- No third-party Go dependencies — standard library only. Keep it that way
  unless there's a compelling, discussed reason.

## Workflow

```sh
bash build-current.sh                 # fmt · JS syntax · go test · race · vet · universal signed build
./build/IconCraft.app/Contents/MacOS/IconCraft --self-test   # native fixture checks (Mac only)
node --test tests/*.test.cjs          # bridge + UI contract tests
```

- Branch from `main`, keep PRs focused, fill in the PR template.
- Run `gofmt` (the build does), `go vet ./...`, and the full test suite
  before requesting review.
- For icon-resolver changes, add tests proving both acceptance of the genuine
  source *and* rejection of knockoffs/lookalikes (seller/name/bundle checks).

## UI text and translations

IconCraft ships in English (default), Simplified Chinese, Japanese and
Korean; the language follows the macOS preferred-language list. All
user-facing text lives in `internal/i18n/locales/<locale>.json`:

- `ui` — page, dialog and menu-bar strings, keyed by short IDs. The HTML
  keeps the English text as its authored default and tags each element
  with `data-i18n="key"` (or `data-i18n-title`/`-aria`/`-placeholder`);
  `app.js` uses `t('key', {vars})`. Add the key to **every** locale file.
- `messages` — Go-side strings (permission rows, default-app categories,
  notifications, dialogs) and engine error messages, keyed by their
  English text. Core packages stay English-only; translation happens at
  the presentation boundary (`i18n.T`, `i18n.Text`, the HTTP `fail`).

`go test ./internal/i18n` and `node --test tests/i18n.test.cjs` fail on a
missing key, a mismatched placeholder, or HTML text that drifts from
`en.json`. To try another language on one Mac without changing System
Settings: `defaults write io.iconcraft.mac AppleLanguages '(ja)'`
(remove with `defaults delete io.iconcraft.mac AppleLanguages`), or set
`ICONCRAFT_LANG=ko` when launching from a terminal.

## Adding a product mapping

Open a PR touching `internal/originals/originals.go` (+ catalogue in
`internal/core/catalog.go`) with: product/craft IDs, publisher name, official
product page, App Store track ID/bundle IDs where applicable, and test data
showing a knockoff is rejected. When in doubt, open an issue first.

## Reporting bugs / requesting features

Use the issue templates. Include IconCraft version, macOS version, chip,
and exact steps. **Never** attach commercial artwork, recovery journals,
signing keys, session tokens, or private app paths.
