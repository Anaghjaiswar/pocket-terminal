package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryPickerListsFoldersAndNavigates(t *testing.T) {
	h := NewHandler("test", "fake")
	h.Dir = t.TempDir()
	for _, name := range []string{"z-last", "Alpha", "folder with spaces"} {
		if err := os.Mkdir(filepath.Join(h.Dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(h.Dir, "file.txt"), []byte("not a folder"), 0600); err != nil {
		t.Fatal(err)
	}
	get := func(path string) directoryListing {
		t.Helper()
		w := httptest.NewRecorder()
		h.Directories(w, httptest.NewRequest("GET", "/api/directories?token=test&path="+url.QueryEscape(path), nil))
		if w.Code != http.StatusOK {
			t.Fatalf("listing returned %d: %s", w.Code, w.Body.String())
		}
		var listing directoryListing
		if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		return listing
	}
	listing := get("")
	if listing.Path != h.Dir || len(listing.Roots) == 0 {
		t.Fatalf("wrong path or missing drives: %+v", listing)
	}
	if len(listing.Directories) != 3 || listing.Directories[0].Name != "Alpha" || listing.Directories[2].Name != "z-last" {
		t.Fatalf("expected sorted folders without files: %+v", listing.Directories)
	}
	child := get(listing.Directories[1].Path)
	if child.Parent != h.Dir || len(child.Directories) != 0 {
		t.Fatalf("wrong child listing: %+v", child)
	}
	// Another drive/folder need not be below the initial project directory.
	other := t.TempDir()
	if get(other).Path != other {
		t.Fatal("cannot browse outside initial project")
	}
}

func TestDirectoryPickerRejectsInvalidRequests(t *testing.T) {
	h := NewHandler("test", "fake")
	h.Dir = t.TempDir()
	for _, tc := range []struct {
		method, query string
		status        int
	}{
		{"GET", "", 401},
		{"POST", "?token=test", 405},
		{"GET", "?token=test&path=" + url.QueryEscape(filepath.Join(h.Dir, "missing")), 404},
	} {
		w := httptest.NewRecorder()
		h.Directories(w, httptest.NewRequest(tc.method, "/api/directories"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("got %d, want %d", w.Code, tc.status)
		}
	}
}
