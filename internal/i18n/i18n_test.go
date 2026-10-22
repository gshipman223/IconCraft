package i18n

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestMatchPrefersFirstSupportedTag(t *testing.T) {
	cases := []struct {
		tags []string
		want Locale
	}{
		{nil, English},
		{[]string{"en-US"}, English},
		{[]string{"ja-JP", "en-US"}, Japanese},
		{[]string{"ko-KR"}, Korean},
		{[]string{"zh-Hans-CN", "en"}, ChineseSimplified},
		{[]string{"zh-CN"}, ChineseSimplified},
		{[]string{"zh"}, ChineseSimplified},
		{[]string{"zh-Hant-TW", "ja"}, Japanese}, // Traditional is not shipped: next preference wins
		{[]string{"zh-HK"}, English},
		{[]string{"fr-FR", "ko-KR", "en-US"}, Korean}, // unsupported first choice falls through
		{[]string{"ja_JP.UTF-8"}, Japanese},           // POSIX spelling
		{[]string{"C"}, English},
		{[]string{"de-DE"}, English},
	}
	for _, c := range cases {
		if got := Match(c.tags); got != c.want {
			t.Errorf("Match(%v) = %s, want %s", c.tags, got, c.want)
		}
	}
}

func TestParseAppleLanguages(t *testing.T) {
	out := "(\n    \"ja-JP\",\n    \"en-US\",\n    zh-Hans\n)\n"
	got := ParseAppleLanguages(out)
	want := []string{"ja-JP", "en-US", "zh-Hans"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
	if len(ParseAppleLanguages("(\n)")) != 0 {
		t.Fatal("empty array should yield no tags")
	}
}

func TestPosixTagsOrder(t *testing.T) {
	env := map[string]string{"LANGUAGE": "ko:ja", "LC_ALL": "", "LC_MESSAGES": "zh_CN.UTF-8", "LANG": "en_US.UTF-8"}
	got := PosixTags(func(k string) string { return env[k] })
	if strings.Join(got, ",") != "ko,ja,zh_CN.UTF-8,en_US.UTF-8" {
		t.Fatalf("got %v", got)
	}
}

func TestDetectHonoursOverride(t *testing.T) {
	t.Setenv(EnvOverride, "ko")
	if got := Detect(); got != Korean {
		t.Fatalf("override ignored: %s", got)
	}
	t.Setenv(EnvOverride, "xx")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "ja_JP.UTF-8")
	if got := Detect(); got != Japanese && got != English {
		// On macOS hosts the AppleLanguages list wins over LC_ALL; on
		// others the POSIX variables decide. Either way an unknown
		// override must never crash or select a nonexistent locale.
		t.Fatalf("unexpected locale %s", got)
	}
}

func TestSetRejectsUnknownLocale(t *testing.T) {
	Set(Locale("tlh"))
	defer Set(English)
	if Current() != English {
		t.Fatal("unknown locale should fall back to English")
	}
}

func TestTTranslatesAndFormats(t *testing.T) {
	defer Set(English)
	Set(Japanese)
	if got := T("Web browser"); got != "Web ブラウザ" {
		t.Fatalf("got %q", got)
	}
	if got := T("IconCraft %s is available", "1.2"); !strings.Contains(got, "1.2") || strings.Contains(got, "%") {
		t.Fatalf("got %q", got)
	}
	Set(English)
	if got := T("IconCraft %s is available", "1.2"); got != "IconCraft 1.2 is available" {
		t.Fatalf("english passthrough: %q", got)
	}
	if got := T("not in catalogue"); got != "not in catalogue" {
		t.Fatalf("unknown msgid must pass through: %q", got)
	}
}

func TestTextTranslatesFormattedErrors(t *testing.T) {
	defer Set(English)
	Set(ChineseSimplified)
	// Exact message.
	if got := Text("application not found; rescan"); got != "未找到该应用程序；请重新扫描" {
		t.Fatalf("exact: %q", got)
	}
	// Templated message with the app name captured and re-inserted.
	got := Text("Test passed on Safari.app: macOS allowed IconCraft to create and remove a temporary file inside that app — the exact write access needed before changing its icon or name.")
	if !strings.Contains(got, "Safari.app") || strings.Contains(got, "Test passed") {
		t.Fatalf("template: %q", got)
	}
	// Wrapped error: translated context, untranslatable cause kept verbatim.
	got = Text("could not rename app: rename /a /b: permission denied")
	if !strings.HasPrefix(got, "无法重命名 App: ") || !strings.HasSuffix(got, "rename /a /b: permission denied") {
		t.Fatalf("wrapped: %q", got)
	}
	// Wrapped "%s: %w" shape: the name stays, the known cause is translated.
	got = Text("could not set the icon for Foo.app: macOS refused the change. Check write access and macOS Privacy & Security → App Management for IconCraft. No application contents were replaced.")
	if !strings.Contains(got, "Foo.app") || strings.Contains(got, "macOS refused the change") {
		t.Fatalf("wrapped cause: %q", got)
	}
	// Captured values that are themselves catalogue entries are translated too.
	got = Text("macOS kept com.apple.Safari as the default for Web browser — if it showed a confirmation, accept it there and try again")
	if !strings.Contains(got, "网页浏览器") || !strings.Contains(got, "com.apple.Safari") {
		t.Fatalf("nested label: %q", got)
	}
	// Numbers survive %d templates.
	got = Text("Adobe Photoshop: no usable icon after 6 checks (publisher site blocked)")
	if !strings.Contains(got, "6") || !strings.Contains(got, "Adobe Photoshop") || strings.Contains(got, "no usable icon") {
		t.Fatalf("numeric template: %q", got)
	}
	// Unknown text is returned unchanged, never hidden.
	if got := Text("totally novel diagnostic"); got != "totally novel diagnostic" {
		t.Fatalf("unknown: %q", got)
	}
	if Error(nil) != "" {
		t.Fatal("nil error must map to empty string")
	}
	if got := Error(errors.New("backup not found")); got != "未找到备份" {
		t.Fatalf("Error: %q", got)
	}
}

func TestEnglishIsIdentityAndUntouched(t *testing.T) {
	Set(English)
	if got := Text("application not found; rescan"); got != "application not found; rescan" {
		t.Fatal(got)
	}
	_, msgs := Catalogue(English)
	for k, v := range msgs {
		if normaliseVerbs(k) != v {
			t.Errorf("English message %q must be its own identifier, got %q", k, v)
		}
	}
}

func TestCataloguesAreComplete(t *testing.T) {
	enUI, enMsgs := Catalogue(English)
	if len(enUI) == 0 || len(enMsgs) == 0 {
		t.Fatal("English catalogue is empty")
	}
	for _, l := range Supported {
		if l == English {
			continue
		}
		ui, msgs := Catalogue(l)
		for k, en := range enUI {
			v, ok := ui[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: ui key %q missing", l, k)
				continue
			}
			if !samePlaceholders(en, v) {
				t.Errorf("%s: ui key %q placeholders differ: %q vs %q", l, k, en, v)
			}
			if strings.Count(en, "**")%2 == 0 && strings.Count(v, "**")%2 != 0 {
				t.Errorf("%s: ui key %q has unbalanced ** emphasis", l, k)
			}
		}
		for k := range ui {
			if _, ok := enUI[k]; !ok {
				t.Errorf("%s: ui key %q has no English source", l, k)
			}
		}
		for id, en := range enMsgs {
			v, ok := msgs[id]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: message %q missing", l, id)
				continue
			}
			if !sameVerbs(en, v) {
				t.Errorf("%s: message %q verbs differ: %q", l, id, v)
			}
		}
		for id := range msgs {
			if _, ok := enMsgs[id]; !ok {
				t.Errorf("%s: message %q has no English source", l, id)
			}
		}
	}
}

var placeholderRE = regexp.MustCompile(`\{[a-zA-Z]+\}`)

func samePlaceholders(a, b string) bool {
	pa, pb := placeholderRE.FindAllString(a, -1), placeholderRE.FindAllString(b, -1)
	sort.Strings(pa)
	sort.Strings(pb)
	return strings.Join(pa, ",") == strings.Join(pb, ",")
}

func sameVerbs(a, b string) bool {
	va, vb := Verbs(a), Verbs(b)
	sort.Slice(va, func(i, j int) bool { return va[i] < va[j] })
	sort.Slice(vb, func(i, j int) bool { return vb[i] < vb[j] })
	return string(va) == string(vb)
}

// Every msgid passed to i18n.T in Go source must exist in the English
// catalogue, and every English UI key must be referenced by the page, the
// window host or the Go side — so stale and missing keys both fail here.
func TestSourceReferencesMatchCatalogue(t *testing.T) {
	root := filepath.Join("..", "..")
	goSrc := readAll(t, root, []string{"cmd", "internal"}, ".go", "_test.go")
	tCall := regexp.MustCompile(`i18n\.T\("((?:[^"\\]|\\.)*)"`)
	_, enMsgs := Catalogue(English)
	for _, m := range tCall.FindAllSubmatch(goSrc, -1) {
		id := unquote(string(m[1]))
		if _, ok := enMsgs[id]; !ok {
			t.Errorf("i18n.T msgid not in en.json: %q", id)
		}
	}
	// Default-app category table and vault/restore steps are translated
	// via T/Text with their English text as the key.
	defaults, _ := os.ReadFile(filepath.Join(root, "internal/native/defaults.go"))
	for _, m := range regexp.MustCompile(`(?:Label|Detail): "([^"]+)"`).FindAllSubmatch(defaults, -1) {
		if _, ok := enMsgs[string(m[1])]; !ok {
			t.Errorf("default-app text not in en.json: %q", m[1])
		}
	}

	page := readAll(t, root, []string{"internal/server/ui"}, ".html", "")
	page = append(page, readAll(t, root, []string{"internal/server/ui"}, ".js", "")...)
	window, _ := os.ReadFile(filepath.Join(root, "internal/native/window.js"))
	refs := map[string]bool{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`\bt\('([a-zA-Z.]+)'`),
		regexp.MustCompile(`\btn\([^,]+,'([a-zA-Z.]+)'`),
		regexp.MustCompile(`data-i18n(?:-[a-z]+)?="([a-zA-Z.]+)"`),
	} {
		for _, m := range re.FindAllSubmatch(page, -1) {
			refs[string(m[1])] = true
		}
	}
	for _, m := range regexp.MustCompile(`\bL\('([a-zA-Z.]+)'`).FindAllSubmatch(window, -1) {
		refs[string(m[1])] = true
	}
	enUI, _ := Catalogue(English)
	for k := range enUI {
		base := strings.TrimSuffix(strings.TrimSuffix(k, ".one"), ".other")
		if !refs[k] && !refs[base] {
			t.Errorf("ui key %q is not referenced by index.html, app.js or window.js", k)
		}
	}
	for k := range refs {
		if _, ok := enUI[k]; ok {
			continue
		}
		if _, ok := enUI[k+".one"]; ok {
			continue
		}
		t.Errorf("ui key %q referenced but missing from en.json", k)
	}
}

func readAll(t *testing.T, root string, dirs []string, ext, skipSuffix string) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, d := range dirs {
		_ = filepath.WalkDir(filepath.Join(root, d), func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ext) || (skipSuffix != "" && strings.HasSuffix(p, skipSuffix)) {
				return nil
			}
			b, rerr := os.ReadFile(p)
			if rerr == nil {
				out.Write(b)
				out.WriteByte('\n')
			}
			return nil
		})
	}
	return out.Bytes()
}

func unquote(s string) string {
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s)
}

func TestScriptIsSameOriginJavaScript(t *testing.T) {
	b := Script(Japanese)
	s := string(b)
	if !strings.HasPrefix(s, "window.ICONCRAFT_I18N=") || !strings.HasSuffix(s, ";\n") {
		t.Fatal(s[:40])
	}
	if !strings.Contains(s, `"locale":"ja"`) || !strings.Contains(s, "アプリ") {
		t.Fatal("locale or strings missing")
	}
	if strings.Contains(s, "</script") || strings.Contains(s, "<") {
		t.Fatal("markup must be escaped inside the script payload")
	}
	// English fills any gap so the page never shows a bare key.
	ui := UIStrings(Korean)
	for k, v := range ui {
		if v == "" {
			t.Errorf("empty ui string %q", k)
		}
	}
}
