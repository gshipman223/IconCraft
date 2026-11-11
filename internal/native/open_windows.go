//go:build windows

package native

func Open(dir string) (Host, error) { return newWindows(dir) }
