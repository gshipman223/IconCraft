//go:build !windows

package core

func windowsInvalidName(error) bool { return false }
