//go:build linux

package native

func Open(dir string) (Host, error) { return newLinux(dir) }
