package ws

import (
	"path/filepath"
	"testing"
)

// TempDir may use a symlinked location, such as /var -> /private/var on
// macOS. Production resolves project symlinks, so fixtures must match.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
