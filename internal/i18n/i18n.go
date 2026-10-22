// Package i18n localises IconCraft's user-facing text.
//
// Design: the engine, native layer and logs stay English-only — the
// English wording is the message identifier, exactly as in gettext — and
// translation happens once at the presentation boundary: the web UI
// receives its catalogue as strings.js, HTTP error payloads pass through
// Text, and notifications/dialogs call T directly. Core packages and
// their safety tests are therefore untouched by the choice of language.
//
// The language is chosen once per process from the macOS preferred
// languages (per-app AppleLanguages override first, then the global
// list), falling back to POSIX locale variables and finally English.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed locales/*.json
var files embed.FS

// Locale is a BCP-47 tag for one of the shipped UI languages.
type Locale string

const (
	English           Locale = "en"
	ChineseSimplified Locale = "zh-Hans"
	Japanese          Locale = "ja"
	Korean            Locale = "ko"
)

// Supported lists every shipped locale; English is the default.
var Supported = []Locale{English, ChineseSimplified, Japanese, Korean}

// BundleID is the defaults domain checked for a per-app language override
// (`defaults write io.iconcraft.mac AppleLanguages '(ja)'`), the standard
// macOS way to run a single app in another language for testing.
const BundleID = "io.iconcraft.mac"

// EnvOverride forces a locale regardless of system settings (QA aid).
const EnvOverride = "ICONCRAFT_LANG"

type catalog struct {
	UI       map[string]string `json:"ui"`
	Messages map[string]string `json:"messages"`
	patterns []pattern
}

// pattern is a templated message ("Test passed on %s: …") compiled to a
// regular expression so an already-formatted English message can be
// recognised and re-rendered in the target language.
type pattern struct {
	msgid string
	re    *regexp.Regexp
	verbs []byte
}

var catalogs = map[Locale]*catalog{}

var current atomic.Value

// format holds the BCP-47 tag used for dates and numbers (macOS "Region"),
// which may differ from the UI language: an en-AU user reads English but
// expects 22/10/2026, not 10/22/2026.
var format atomic.Value

func init() {
	for _, l := range Supported {
		b, err := files.ReadFile("locales/" + string(l) + ".json")
		if err != nil {
			panic("i18n: missing catalogue for " + string(l) + ": " + err.Error())
		}
		c := &catalog{}
		if err := json.Unmarshal(b, c); err != nil {
			panic("i18n: invalid catalogue for " + string(l) + ": " + err.Error())
		}
		if c.UI == nil {
			c.UI = map[string]string{}
		}
		if c.Messages == nil {
			c.Messages = map[string]string{}
		}
		for id, v := range c.Messages {
			c.Messages[id] = normaliseVerbs(v)
			if v == "" {
				continue
			}
			if p, ok := compilePattern(id); ok {
				c.patterns = append(c.patterns, p)
			}
		}
		// Longer (more specific) templates first so "x: y" shapes are not
		// swallowed by a shorter template with a trailing capture.
		sort.SliceStable(c.patterns, func(i, j int) bool { return len(c.patterns[i].msgid) > len(c.patterns[j].msgid) })
		catalogs[l] = c
	}
	current.Store(English)
	format.Store("")
}

// Set fixes the process-wide UI locale; unknown locales fall back to English.
func Set(l Locale) {
	if _, ok := catalogs[l]; !ok {
		l = English
	}
	current.Store(l)
}

// Current returns the active UI locale.
func Current() Locale { return current.Load().(Locale) }

// SetFormatLocale fixes the tag used for date and number formatting.
// Empty means "same as the UI locale".
func SetFormatLocale(tag string) { format.Store(cleanTag(tag)) }

// FormatLocale returns the formatting tag, defaulting to the UI locale.
func FormatLocale() string {
	if f, _ := format.Load().(string); f != "" {
		return f
	}
	return string(Current())
}

// Detect chooses the UI locale from the environment: ICONCRAFT_LANG,
// then the macOS preferred-language lists, then POSIX locale variables.
func Detect() Locale {
	if v := os.Getenv(EnvOverride); v != "" {
		if l, ok := matchTag(v); ok {
			return l
		}
	}
	if runtime.GOOS == "darwin" {
		if tags := appleLanguages(); len(tags) > 0 {
			return Match(tags)
		}
	}
	return Match(PosixTags(os.Getenv))
}

// DetectFormatLocale chooses the formatting tag the way macOS does: the
// Region setting (AppleLocale, e.g. en_AU) first, then the first preferred
// language when it carries a region (en-AU), then POSIX LC_TIME/LANG, and
// finally the UI locale itself. An ICONCRAFT_LANG override with a region
// (en-AU) applies to formatting too.
func DetectFormatLocale(ui Locale) string {
	var apple string
	var tags []string
	if runtime.GOOS == "darwin" {
		apple = appleLocale()
		tags = appleLanguages()
	}
	return FormatTag(os.Getenv(EnvOverride), apple, tags, PosixTags(func(k string) string {
		if k == "LC_MESSAGES" {
			k = "LC_TIME"
		}
		return os.Getenv(k)
	}), ui)
}

// FormatTag is the pure selection behind DetectFormatLocale.
func FormatTag(override, appleLocale string, languages, posix []string, ui Locale) string {
	if t := cleanTag(override); hasRegion(t) {
		return t
	}
	if t := cleanTag(appleLocale); t != "" {
		return t
	}
	for _, list := range [][]string{languages, posix} {
		for _, raw := range list {
			if t := cleanTag(raw); hasRegion(t) {
				return t
			}
		}
	}
	return string(ui)
}

// cleanTag turns en_AU.UTF-8, en_AU@calendar=gregorian or "en-AU" into a
// BCP-47 tag Intl accepts; junk becomes "".
func cleanTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	if tag == "" || strings.EqualFold(tag, "C") || strings.EqualFold(tag, "POSIX") {
		return ""
	}
	for _, r := range tag {
		if !(r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return ""
		}
	}
	return tag
}

func hasRegion(tag string) bool {
	parts := strings.Split(tag, "-")
	for _, p := range parts[1:] {
		if len(p) == 2 || (len(p) == 3 && p[0] >= '0' && p[0] <= '9') {
			return true
		}
	}
	return false
}

func appleLocale() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Match returns the first supported locale in a preference-ordered list
// of language tags, mirroring how macOS picks an app's language. Tags no
// shipped locale serves (e.g. Traditional Chinese) are skipped so the
// user's next preference wins; English is the final fallback.
func Match(tags []string) Locale {
	for _, tag := range tags {
		if l, ok := matchTag(tag); ok {
			return l
		}
	}
	return English
}

func matchTag(tag string) (Locale, bool) {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i] // ja_JP.UTF-8 / en_GB@euro
	}
	parts := strings.Split(strings.ToLower(tag), "-")
	switch parts[0] {
	case "en", "c", "posix":
		return English, true
	case "ja":
		return Japanese, true
	case "ko":
		return Korean, true
	case "zh":
		for _, p := range parts[1:] {
			switch p {
			case "hans", "cn", "sg", "my":
				return ChineseSimplified, true
			case "hant", "tw", "hk", "mo":
				return "", false
			}
		}
		return ChineseSimplified, true
	}
	return "", false
}

// PosixTags collects language tags from the usual locale variables, most
// specific first (GNU LANGUAGE list, then LC_ALL, LC_MESSAGES, LANG).
func PosixTags(getenv func(string) string) []string {
	var tags []string
	for _, p := range strings.Split(getenv("LANGUAGE"), ":") {
		if p = strings.TrimSpace(p); p != "" {
			tags = append(tags, p)
		}
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			tags = append(tags, v)
		}
	}
	return tags
}

func appleLanguages() []string {
	for _, domain := range []string{BundleID, "-g"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", domain, "AppleLanguages").Output()
		cancel()
		if err != nil {
			continue
		}
		if tags := ParseAppleLanguages(string(out)); len(tags) > 0 {
			return tags
		}
	}
	return nil
}

// ParseAppleLanguages reads the `defaults read … AppleLanguages` rendering
// of a string array — ( "ja-JP", "en-US" ) — tolerating unquoted entries.
func ParseAppleLanguages(out string) []string {
	out = strings.TrimSpace(out)
	out = strings.TrimSuffix(strings.TrimPrefix(out, "("), ")")
	var tags []string
	for _, part := range strings.Split(out, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"`)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

// T translates a message identifier (its English text) into the current
// locale. Arguments are formatted with fmt verbs kept by the translation;
// translations may reorder them with explicit indexes (%[2]s).
func T(msgid string, args ...any) string {
	s := msgid
	if c := catalogs[Current()]; c != nil {
		if v, ok := c.Messages[msgid]; ok && v != "" {
			s = v
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(normaliseVerbs(s), args...)
}

// TN picks the singular or plural English message identifier for n and
// formats it like T. Both forms are catalogue entries, so languages that
// do not inflect for number simply translate them identically.
func TN(n int, one, other string, args ...any) string {
	if n == 1 {
		return T(one, args...)
	}
	return T(other, args...)
}

// Text translates an already-formatted English message, such as an
// error produced deep in the engine. Exact messages, templated messages
// (with their captured values translated in turn) and "context: cause"
// chains are handled; anything unknown is returned unchanged, so a
// technical detail is never hidden — it merely stays in English.
func Text(s string) string {
	l := Current()
	if l == English || s == "" {
		return s
	}
	if t, ok := translate(catalogs[l], s, 0); ok {
		return t
	}
	return s
}

// Error returns err's message translated with Text ("" for nil).
func Error(err error) string {
	if err == nil {
		return ""
	}
	return Text(err.Error())
}

func translate(c *catalog, s string, depth int) (string, bool) {
	if v, ok := c.Messages[s]; ok && v != "" {
		return v, true
	}
	if depth > 4 {
		return "", false
	}
	for _, p := range c.patterns {
		m := p.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		args := make([]any, len(m)-1)
		for i, g := range m[1:] {
			if t, ok := translate(c, g, depth+1); ok {
				g = t
			}
			if p.verbs[i] == 'd' {
				n, _ := strconv.Atoi(g)
				args[i] = n
			} else {
				args[i] = g
			}
		}
		return fmt.Sprintf(c.Messages[p.msgid], args...), true
	}
	// Wrapped errors read "context: cause"; translate each half it can.
	if i := strings.Index(s, ": "); i > 0 {
		head, tail := s[:i], s[i+2:]
		th, okHead := translate(c, head, depth+1)
		tt, okTail := translate(c, tail, depth+1)
		if okHead || okTail {
			if !okHead {
				th = head
			}
			if !okTail {
				tt = tail
			}
			return th + ": " + tt, true
		}
	}
	return "", false
}

var verbRE = regexp.MustCompile(`%(\[\d+\])?[sdvwq]`)

// normaliseVerbs makes a template safe for fmt.Sprintf with string or
// int arguments: %w only exists for Errorf, so it becomes %v.
func normaliseVerbs(s string) string { return strings.ReplaceAll(s, "%w", "%v") }

// Verbs lists the fmt verbs of a template in order, with v/w/q folded to
// s, so catalogue tests can confirm translations keep every placeholder.
func Verbs(s string) []byte {
	var out []byte
	for _, m := range verbRE.FindAllString(s, -1) {
		v := m[len(m)-1]
		if v != 'd' {
			v = 's'
		}
		out = append(out, v)
	}
	return out
}

func compilePattern(msgid string) (pattern, bool) {
	verbs := Verbs(msgid)
	if len(verbs) == 0 {
		return pattern{}, false
	}
	var b strings.Builder
	b.WriteString(`(?s)^`)
	last := 0
	for _, loc := range verbRE.FindAllStringIndex(msgid, -1) {
		b.WriteString(regexp.QuoteMeta(msgid[last:loc[0]]))
		switch msgid[loc[1]-1] {
		case 'd':
			b.WriteString(`(\d+)`)
		case 'q':
			b.WriteString(`"(.*?)"`)
		default:
			b.WriteString(`(.+?)`)
		}
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(msgid[last:]))
	b.WriteString(`$`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return pattern{}, false
	}
	return pattern{msgid: msgid, re: re, verbs: verbs}, true
}

// UIStrings returns the web UI catalogue for l: English, overlaid with
// every translated key, so the page never shows a bare key.
func UIStrings(l Locale) map[string]string {
	out := map[string]string{}
	for k, v := range catalogs[English].UI {
		out[k] = v
	}
	if c := catalogs[l]; c != nil && l != English {
		for k, v := range c.UI {
			if v != "" {
				out[k] = v
			}
		}
	}
	return out
}

// Script renders the UI catalogue as a same-origin JavaScript file. The
// page's CSP forbids inline scripts, so the dictionary travels this way;
// encoding/json escapes <, > and & which keeps it inert as JS source.
func Script(l Locale) []byte {
	payload := struct {
		Locale  Locale            `json:"locale"`
		Format  string            `json:"format"`
		Strings map[string]string `json:"strings"`
	}{l, FormatLocale(), UIStrings(l)}
	b, _ := json.Marshal(payload)
	return []byte("window.ICONCRAFT_I18N=" + string(b) + ";\n")
}

// MessageIDs lists the Go-side message identifiers in the English catalogue.
func MessageIDs() []string {
	out := make([]string, 0, len(catalogs[English].Messages))
	for k := range catalogs[English].Messages {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// UIKeys lists the web UI keys in the English catalogue.
func UIKeys() []string {
	out := make([]string, 0, len(catalogs[English].UI))
	for k := range catalogs[English].UI {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Catalogue exposes one locale's raw tables for tests.
func Catalogue(l Locale) (ui, messages map[string]string) {
	c := catalogs[l]
	return c.UI, c.Messages
}
