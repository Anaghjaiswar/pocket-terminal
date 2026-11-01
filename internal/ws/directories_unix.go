//go:build !windows

package ws

func directoryRoots() ([]directoryEntry, error) {
	return []directoryEntry{{Name: "/", Path: "/"}}, nil
}
