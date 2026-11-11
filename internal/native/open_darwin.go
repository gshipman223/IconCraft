//go:build darwin

package native

func Open(dir string) (Host, error) { return New(dir) }
