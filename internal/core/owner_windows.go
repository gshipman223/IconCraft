//go:build windows

package core

func bundleOwner(path string) (uint32, bool) {
	if bundleOwnerOverride != nil {
		return bundleOwnerOverride(path)
	}
	return 0, false
}
