//go:build linux

package native

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gshipman223/IconCraft/internal/i18n"
)

type mimeCategory struct {
	ID     string
	Label  string
	Detail string
	Mimes  []string
}

func linuxDefaultCategories() []mimeCategory {
	return []mimeCategory{
		{ID: "browser", Label: "Web browser", Detail: "Links you click in mail, messages and other apps", Mimes: []string{"x-scheme-handler/https", "x-scheme-handler/http"}},
		{ID: "email", Label: "Email", Detail: "Email links that start a new message", Mimes: []string{"x-scheme-handler/mailto"}},
		{ID: "pdf", Label: "PDF documents", Detail: "Opening a PDF", Mimes: []string{"application/pdf"}},
		{ID: "images", Label: "Images", Detail: "Opening a picture", Mimes: []string{"image/jpeg"}},
		{ID: "video", Label: "Movies & video", Detail: "Opening a video", Mimes: []string{"video/mp4"}},
		{ID: "audio", Label: "Audio", Detail: "Opening a music or audio file", Mimes: []string{"audio/mpeg"}},
		{ID: "text", Label: "Text files", Detail: "Plain text, notes and read-me files", Mimes: []string{"text/plain"}},
		{ID: "code", Label: "Source code", Detail: "Code and configuration files", Mimes: []string{"text/x-csrc"}},
		{ID: "html", Label: "HTML files", Detail: "Opening a saved web page", Mimes: []string{"text/html"}},
		{ID: "archives", Label: "ZIP archives", Detail: "Opening a .zip", Mimes: []string{"application/zip"}},
		{ID: "calendar", Label: "Calendar files", Detail: "Calendar invites saved from mail and the web", Mimes: []string{"text/calendar"}},
	}
}

func (l *Linux) DefaultApps(ctx context.Context) ([]DefaultCategoryState, error) {
	current := readMimeDefaults(l.MimeApps)
	cands := l.mimeCandidates(ctx)
	var out []DefaultCategoryState
	for _, cat := range linuxDefaultCategories() {
		state := DefaultCategoryState{ID: cat.ID, Label: i18n.T(cat.Label), Detail: i18n.T(cat.Detail)}
		seen := map[string]bool{}
		for _, mime := range cat.Mimes {
			if id := current[mime]; id != "" && !seen[id] {
				state.Current = id
			}
			for _, c := range cands[mime] {
				if seen[c.BundleID] {
					continue
				}
				seen[c.BundleID] = true
				state.Candidates = append(state.Candidates, c)
			}
		}
		if state.Candidates == nil {
			state.Candidates = []DefaultCandidate{}
		}
		sort.Slice(state.Candidates, func(i, j int) bool { return state.Candidates[i].Name < state.Candidates[j].Name })
		out = append(out, state)
	}
	return out, nil
}

func (l *Linux) SetDefaultApp(ctx context.Context, categoryID, bundleID string) (DefaultCategoryState, error) {
	if err := ctx.Err(); err != nil {
		return DefaultCategoryState{}, err
	}
	var cat mimeCategory
	found := false
	for _, c := range linuxDefaultCategories() {
		if c.ID == categoryID {
			cat = c
			found = true
			break
		}
	}
	if !found {
		return DefaultCategoryState{}, os.ErrNotExist
	}
	if err := writeMimeDefaults(l.MimeApps, cat.Mimes, bundleID); err != nil {
		return DefaultCategoryState{}, err
	}
	all, err := l.DefaultApps(ctx)
	if err != nil {
		return DefaultCategoryState{}, err
	}
	for _, s := range all {
		if s.ID == categoryID {
			return s, nil
		}
	}
	return DefaultCategoryState{}, os.ErrNotExist
}

func (l *Linux) mimeCandidates(ctx context.Context) map[string][]DefaultCandidate {
	out := map[string][]DefaultCandidate{}
	paths, _ := l.Discover(nil)
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return out
		}
		e, err := l.readEntry(p)
		if err != nil {
			continue
		}
		id, ok := l.launcherID(p)
		if !ok {
			continue
		}
		name := displayName(e)
		if name == "" {
			name = id
		}
		for _, mime := range strings.Split(e.Get("MimeType"), ";") {
			mime = strings.TrimSpace(mime)
			if mime == "" {
				continue
			}
			out[mime] = append(out[mime], DefaultCandidate{BundleID: id, Name: name, Path: p})
		}
	}
	return out
}

func readMimeDefaults(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			in = line == "[Default Applications]"
			continue
		}
		if !in {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		id := strings.TrimSpace(strings.Split(val, ";")[0])
		if id != "" {
			out[strings.TrimSpace(key)] = id
		}
	}
	return out
}

func writeMimeDefaults(path string, mimes []string, id string) error {
	if !strings.HasSuffix(id, ".desktop") || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return os.ErrInvalid
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(raw)
	if !strings.Contains(text, "[Default Applications]") {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "[Default Applications]\n"
	}
	lines := strings.Split(text, "\n")
	in := false
	wanted := map[string]bool{}
	for _, m := range mimes {
		wanted[m] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if in {
				for mime := range wanted {
					if !seen[mime] {
						out = append(out, mime+"="+id)
					}
				}
			}
			in = trim == "[Default Applications]"
			out = append(out, line)
			continue
		}
		if in {
			key, _, ok := strings.Cut(trim, "=")
			if ok && wanted[strings.TrimSpace(key)] {
				out = append(out, strings.TrimSpace(key)+"="+id)
				seen[strings.TrimSpace(key)] = true
				continue
			}
		}
		out = append(out, line)
	}
	if in {
		for mime := range wanted {
			if !seen[mime] {
				out = append(out, mime+"="+id)
			}
		}
	}
	body := strings.Join(out, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeNoSymlink(path, []byte(body), 0644)
}
