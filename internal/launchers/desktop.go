// Package launchers reads and writes the files that give an app its name and
// icon on Windows and Linux: Freedesktop desktop entries and Shell Link
// shortcuts. The programs those launchers point at are never modified.
package launchers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Entry is one .desktop file. Groups and keys keep their original order so a
// rewritten launcher stays a faithful copy of the one it was based on.
type Entry struct {
	Groups []Group
}

// Group is a bracketed section such as "Desktop Entry" or an action.
type Group struct {
	Name string
	Keys []Field
}

// Field is one key. Locale is the bracketed language, if any ("Name[fr]").
type Field struct {
	Key    string
	Locale string
	Value  string
}

// ParseDesktop parses a desktop entry. Lines it does not understand are kept
// as keys with an empty key name and the raw line as the value, so a rewrite
// does not drop comments.
func ParseDesktop(b []byte) (Entry, error) {
	if !utf8.Valid(b) {
		return Entry{}, errors.New("desktop entry is not UTF-8")
	}
	var e Entry
	var g *Group
	sc := bufio.NewScanner(bytes.NewReader(b))
	// Desktop files are small; a launcher that claims to be one but is huge
	// is rejected by the caller. Scanner's default token is enough here.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			if g == nil {
				e.Groups = append(e.Groups, Group{})
				g = &e.Groups[len(e.Groups)-1]
			}
			g.Keys = append(g.Keys, Field{Value: line})
			continue
		}
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			e.Groups = append(e.Groups, Group{Name: trim[1 : len(trim)-1]})
			g = &e.Groups[len(e.Groups)-1]
			continue
		}
		if g == nil {
			return Entry{}, errors.New("desktop entry has a key before its first group")
		}
		key, value, ok := strings.Cut(trim, "=")
		if !ok {
			g.Keys = append(g.Keys, Field{Value: line})
			continue
		}
		name, locale := splitLocale(strings.TrimSpace(key))
		g.Keys = append(g.Keys, Field{Key: name, Locale: locale, Value: unescape(value)})
	}
	if err := sc.Err(); err != nil {
		return Entry{}, err
	}
	if e.Desktop() == nil {
		return Entry{}, errors.New("desktop entry has no Desktop Entry group")
	}
	return e, nil
}

func splitLocale(key string) (string, string) {
	i := strings.IndexByte(key, '[')
	if i < 0 || !strings.HasSuffix(key, "]") {
		return key, ""
	}
	return key[:i], key[i+1 : len(key)-1]
}

// Desktop returns the main group, or nil when the file has none.
func (e *Entry) Desktop() *Group {
	for i := range e.Groups {
		if e.Groups[i].Name == "Desktop Entry" {
			return &e.Groups[i]
		}
	}
	return nil
}

// Get returns the unlocalised value of a key in the Desktop Entry group.
func (e *Entry) Get(key string) string {
	g := e.Desktop()
	if g == nil {
		return ""
	}
	return g.Get(key)
}

// Get returns the unlocalised value of a key.
func (g *Group) Get(key string) string {
	for _, f := range g.Keys {
		if f.Key == key && f.Locale == "" {
			return f.Value
		}
	}
	return ""
}

// Bool reports a desktop boolean (true/false, case-insensitive).
func (e *Entry) Bool(key string) bool {
	return strings.EqualFold(e.Get(key), "true")
}

// Set replaces or appends an unlocalised key in the Desktop Entry group.
func (e *Entry) Set(key, value string) {
	g := e.Desktop()
	if g == nil {
		e.Groups = append([]Group{{Name: "Desktop Entry"}}, e.Groups...)
		g = &e.Groups[0]
	}
	for i := range g.Keys {
		if g.Keys[i].Key == key && g.Keys[i].Locale == "" {
			g.Keys[i].Value = value
			return
		}
	}
	g.Keys = append(g.Keys, Field{Key: key, Value: value})
}

// Del removes every locale of a key from the Desktop Entry group.
func (e *Entry) Del(key string) {
	g := e.Desktop()
	if g == nil {
		return
	}
	out := g.Keys[:0]
	for _, f := range g.Keys {
		if f.Key == key {
			continue
		}
		out = append(out, f)
	}
	g.Keys = out
}

// TypeOK reports a launcher IconCraft is willing to list.
func (e *Entry) TypeOK() bool {
	g := e.Desktop()
	if g == nil {
		return false
	}
	if !strings.EqualFold(g.Get("Type"), "Application") {
		return false
	}
	if e.Bool("Hidden") || e.Bool("NoDisplay") {
		return false
	}
	return strings.TrimSpace(g.Get("Exec")) != "" || e.Bool("DBusActivatable")
}

// Bytes encodes the entry. A trailing newline is always present.
func (e Entry) Bytes() []byte {
	var b strings.Builder
	for _, g := range e.Groups {
		if g.Name != "" {
			fmt.Fprintf(&b, "[%s]\n", g.Name)
		}
		for _, f := range g.Keys {
			if f.Key == "" {
				b.WriteString(f.Value)
				if !strings.HasSuffix(f.Value, "\n") {
					b.WriteByte('\n')
				}
				continue
			}
			name := f.Key
			if f.Locale != "" {
				name += "[" + f.Locale + "]"
			}
			fmt.Fprintf(&b, "%s=%s\n", name, escape(f.Value))
		}
	}
	return []byte(b.String())
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte(s[i+1])
		}
		i++
	}
	return b.String()
}

func escape(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// DesktopID is the desktop-file id of a file inside an applications
// directory: subdirectories become dashes, so kde/foo.desktop is
// kde-foo.desktop. The id is what the session uses to match a running app.
func DesktopID(applicationsDir, path string) (string, bool) {
	rel := path
	if applicationsDir != "" {
		prefix := strings.TrimRight(applicationsDir, `/\`) + "/"
		if !strings.HasPrefix(path, prefix) {
			return "", false
		}
		rel = path[len(prefix):]
	}
	rel = strings.ReplaceAll(rel, `\`, "/")
	if rel == "" || strings.Contains(rel, "..") || !strings.HasSuffix(rel, ".desktop") {
		return "", false
	}
	return strings.ReplaceAll(rel, "/", "-"), true
}

// ExecBinary returns the program Exec would start, with field codes removed.
// Quotes around an argument are honoured so paths with spaces survive.
func ExecBinary(exec string) string {
	args := splitExec(exec)
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "%") {
			continue
		}
		return a
	}
	return ""
}

func splitExec(s string) []string {
	var out []string
	var cur strings.Builder
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			if c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
				continue
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// utf16LE is used by the shortcut reader in the same package. Desktop files
// themselves are UTF-8; this helper stays here so both formats share one
// checked decoder.
func decodeUTF16(b []byte) string {
	if len(b)%2 != 0 {
		return ""
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}

func encodeUTF16(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[i*2] = byte(c)
		b[i*2+1] = byte(c >> 8)
	}
	return b
}
