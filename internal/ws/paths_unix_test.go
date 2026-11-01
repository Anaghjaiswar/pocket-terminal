//go:build !windows

package ws

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/anagh/pocket-terminal/internal/pty"
)

func TestSymlinkedProjectUsesCanonicalDirectory(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "project")
	alias := filepath.Join(root, "alias")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	h := NewHandler("test", "fake")
	h.Dir = alias
	defer h.Close()
	h.start = func(opts pty.Options) (pty.Terminal, error) {
		if opts.Dir != target {
			t.Fatalf("shell dir %q, want canonical path %q", opts.Dir, target)
		}
		return newFakeTerminal(), nil
	}
	w := httptest.NewRecorder()
	h.Directories(w, httptest.NewRequest("GET", "/api/directories?token=test", nil))
	if w.Code != 200 {
		t.Fatalf("directory listing: %s", w.Body.String())
	}
	var folders directoryListing
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatal(err)
	}
	if folders.Path != target || folders.Parent != root {
		t.Fatalf("incorrect resolved folder: %+v", folders)
	}
	created := sessionRequest(t, h, "POST", "", `{}`, 201)
	var session sessionInfo
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Project != target {
		t.Fatalf("session project %q, want %q", session.Project, target)
	}
	// Both spellings must retrieve the same session rather than separate projects.
	for _, path := range []string{alias, target} {
		w := sessionRequest(t, h, "GET", "project="+url.QueryEscape(path), "", 200)
		var listing struct {
			Sessions []sessionInfo `json:"sessions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.Sessions) != 1 || listing.Sessions[0].ID != session.ID {
			t.Fatalf("symlink and target returned different projects: %s", w.Body.String())
		}
	}
}
