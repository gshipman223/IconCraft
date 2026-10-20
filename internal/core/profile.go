package core

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ProfileAsset struct {
	Asset Asset  `json:"asset"`
	PNG   []byte `json:"png"`
}
type Profile struct {
	Schema      int                   `json:"schema"`
	Application string                `json:"application"`
	Assets      []ProfileAsset        `json:"assets"`
	Preferences map[string]Preference `json:"preferences"`
}

func (e *Engine) ExportProfile() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := Profile{Schema: 1, Application: "IconCraft", Assets: []ProfileAsset{}, Preferences: e.State.Preferences}
	size := 0
	for id, a := range e.State.Assets {
		path, err := e.assetPath(id)
		if err != nil {
			return nil, err
		}
		b, err := ReadLimited(path, MaxIconBytes)
		if err != nil {
			return nil, err
		}
		size += len(b)
		if size > 40<<20 {
			return nil, errors.New("profile exceeds 40 MiB of icon data")
		}
		p.Assets = append(p.Assets, ProfileAsset{Asset: a, PNG: b})
	}
	return json.MarshalIndent(p, "", "  ")
}
func (e *Engine) ImportProfile(b []byte) error {
	if len(b) > 64<<20 {
		return errors.New("profile too large")
	}
	var p Profile
	if err := DecodeStrict(b, &p); err != nil {
		return err
	}
	if p.Schema != 1 || p.Application != "IconCraft" || len(p.Assets) > 500 || len(p.Preferences) > 500 {
		return errors.New("invalid profile schema or entry count")
	}
	available := map[string]bool{}
	for _, a := range p.Assets {
		if ValidatePNG(a.PNG) != nil || hash(a.PNG) != a.Asset.ID {
			return errors.New("profile icon failed integrity validation")
		}
		available[a.Asset.ID] = true
	}
	for key, pref := range p.Preferences {
		if key == "" || key != pref.BundleID || !available[pref.AssetID] {
			return fmt.Errorf("invalid profile preference %q", key)
		}
	}
	// Imported data never contains filesystem paths, shell commands, or automatic apply instructions.
	for _, a := range p.Assets {
		if _, err := e.ImportPNG(a.Asset.Name, "Imported portable profile: "+a.Asset.Source, a.PNG); err != nil {
			return err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range p.Preferences {
		e.State.Preferences[k] = v
	}
	return e.save()
}
