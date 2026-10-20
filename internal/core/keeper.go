package core

// Desired-state for the keep-applied LaunchAgent. The agent's plist can
// be removed externally (cleaner tools managing launch agents, manual
// deletion, launchd housekeeping); the user's *intent* is what matters,
// so it is journalled here and the main process quietly restores the
// agent whenever it finds intent=on but the agent missing.

// KeeperPreference returns the journalled intent (desired, wasEverSet).
func (e *Engine) KeeperPreference() (bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.State.KeeperDesired, e.State.KeeperSet
}

// SetKeeperPreference journals the user's keeper intent.
func (e *Engine) SetKeeperPreference(on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.State.KeeperDesired = on
	e.State.KeeperSet = true
	return e.save()
}

// InitKeeperPreference seeds intent from the agent's current reality the
// first time it runs (upgrades from before intent was journalled).
func (e *Engine) InitKeeperPreference(currentlyEnabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.State.KeeperSet {
		return
	}
	e.State.KeeperDesired = currentlyEnabled
	e.State.KeeperSet = true
	_ = e.save()
}
