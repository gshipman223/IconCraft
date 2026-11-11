package native

import "errors"

// PlatformInfo tells the page which operating system it is talking to.
// The macOS app does not set it, and the page keeps its current copy.
type PlatformInfo struct {
	OS                string `json:"os"`
	FullSkin          bool   `json:"fullSkin"`
	RenameSuffix      string `json:"renameSuffix"`
	DefaultsMode      string `json:"defaultsMode"`
	TaglineKey        string `json:"taglineKey"`
	NameHintKey       string `json:"nameHintKey"`
	LauncherNoteKey   string `json:"launcherNoteKey"`
	DefaultsIntroKey  string `json:"defaultsIntroKey"`
	DefaultsNoteKey   string `json:"defaultsNoteKey"`
	HideDockSetting   bool   `json:"hideDockSetting"`
	MenuBarLabelKey   string `json:"menuBarLabelKey"`
	MenuBarTextKey    string `json:"menuBarTextKey"`
	MenuStartLabelKey string `json:"menuStartLabelKey"`
	MenuStartTextKey  string `json:"menuStartTextKey"`
	NotificationsKey  string `json:"notificationsKey"`
	MaintenanceKey    string `json:"maintenanceKey"`
	FolderTitleKey    string `json:"folderTitleKey"`
	ThemeTitleKey     string `json:"themeTitleKey"`
	SearchShortcut    string `json:"searchShortcut"`
	SetupNoteKey      string `json:"setupNoteKey"`
	SetupSubtitleKey  string `json:"setupSubtitleKey"`
}

// ErrSettingsOpened means the platform opened its own Settings app
// instead of switching the default. Callers show that to the user.
var ErrSettingsOpened = errors.New("the system settings app was opened so the default can be chosen there")
