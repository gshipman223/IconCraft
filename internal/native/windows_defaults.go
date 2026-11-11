//go:build windows

package native

import (
	"context"
	"os"

	"github.com/gshipman223/IconCraft/internal/i18n"
)

func windowsDefaultCategories() []mimeCategory {
	return []mimeCategory{
		{ID: "browser", Label: "Web browser", Detail: "Links you click in mail, messages and other apps"},
		{ID: "email", Label: "Email", Detail: "Email links that start a new message"},
		{ID: "pdf", Label: "PDF documents", Detail: "Opening a PDF"},
		{ID: "images", Label: "Images", Detail: "Opening a picture"},
		{ID: "video", Label: "Movies & video", Detail: "Opening a video"},
		{ID: "audio", Label: "Audio", Detail: "Opening a music or audio file"},
		{ID: "text", Label: "Text files", Detail: "Plain text, notes and read-me files"},
		{ID: "code", Label: "Source code", Detail: "Code and configuration files"},
		{ID: "html", Label: "HTML files", Detail: "Opening a saved web page"},
		{ID: "archives", Label: "ZIP archives", Detail: "Opening a .zip"},
		{ID: "calendar", Label: "Calendar files", Detail: "Calendar invites saved from mail and the web"},
	}
}

// mimeCategory is shared in shape with the Linux table. It is declared
// here as well because the Linux file is not compiled on Windows.
type mimeCategory struct {
	ID     string
	Label  string
	Detail string
	Mimes  []string
}

func (w *Windows) DefaultApps(ctx context.Context) ([]DefaultCategoryState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []DefaultCategoryState
	for _, cat := range windowsDefaultCategories() {
		out = append(out, DefaultCategoryState{
			ID:         cat.ID,
			Label:      i18n.T(cat.Label),
			Detail:     i18n.T(cat.Detail),
			Candidates: []DefaultCandidate{},
		})
	}
	return out, nil
}

func (w *Windows) SetDefaultApp(ctx context.Context, categoryID, _ string) (DefaultCategoryState, error) {
	if err := w.OpenDefaultSettings(ctx); err != nil {
		return DefaultCategoryState{}, err
	}
	for _, cat := range windowsDefaultCategories() {
		if cat.ID == categoryID {
			return DefaultCategoryState{ID: cat.ID, Label: i18n.T(cat.Label), Detail: i18n.T(cat.Detail), Candidates: []DefaultCandidate{}}, ErrSettingsOpened
		}
	}
	return DefaultCategoryState{}, os.ErrNotExist
}

func (w *Windows) OpenDefaultSettings(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.SkipRegistry {
		return nil
	}
	return startDetached("cmd", "/c", "start", "", "ms-settings:defaultapps")
}
