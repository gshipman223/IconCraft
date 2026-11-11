//go:build windows

package native

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

type storeApp struct {
	Name  string `json:"Name"`
	AppID string `json:"AppID"`
}

// listStoreApps is replaceable. Tests leave SkipStore set so this is
// never called and PowerShell is never started.
var listStoreApps = func(ctx context.Context) ([]storeApp, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Get-StartApps | Select-Object Name,AppID | ConvertTo-Json -Compress`)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" || text == "null" {
		return nil, nil
	}
	if strings.HasPrefix(text, "[") {
		var apps []storeApp
		if err := json.Unmarshal([]byte(text), &apps); err != nil {
			return nil, err
		}
		return apps, nil
	}
	var one storeApp
	if err := json.Unmarshal([]byte(text), &one); err != nil {
		return nil, err
	}
	if one.AppID == "" {
		return nil, nil
	}
	return []storeApp{one}, nil
}
