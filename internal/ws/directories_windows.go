//go:build windows

package ws

import "golang.org/x/sys/windows"

func directoryRoots() ([]directoryEntry, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	roots := []directoryEntry{}
	for drive := 0; drive < 26; drive++ {
		if mask&(1<<drive) != 0 {
			path := string(rune('A'+drive)) + `:\`
			roots = append(roots, directoryEntry{Name: path, Path: path})
		}
	}
	return roots, nil
}
