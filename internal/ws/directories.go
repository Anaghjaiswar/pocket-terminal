package ws

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anagh/pocket-terminal/internal/auth"
)

type directoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type directoryListing struct {
	Path        string           `json:"path"`
	Parent      string           `json:"parent"`
	Roots       []directoryEntry `json:"roots"`
	Directories []directoryEntry `json:"directories"`
}

// Directories lists laptop folders for the authenticated project picker.
func (h *Handler) Directories(w http.ResponseWriter, r *http.Request) {
	if !auth.ValidateRequest(r, h.Token) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir, err := h.projectDir(r.URL.Query().Get("path"))
	if err != nil {
		directoryError(w, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		directoryError(w, err)
		return
	}
	roots, err := directoryRoots()
	if err != nil {
		http.Error(w, "Cannot list drives: "+err.Error(), 500)
		return
	}
	listing := directoryListing{Path: dir, Parent: filepath.Dir(dir), Roots: roots, Directories: []directoryEntry{}}
	if listing.Parent == dir {
		listing.Parent = ""
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		isDir := entry.IsDir()
		if !isDir && entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(path)
			isDir = err == nil && info.IsDir()
		}
		if isDir {
			listing.Directories = append(listing.Directories, directoryEntry{Name: entry.Name(), Path: path})
		}
	}
	sort.SliceStable(listing.Directories, func(i, j int) bool {
		return strings.ToLower(listing.Directories[i].Name) < strings.ToLower(listing.Directories[j].Name)
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(listing)
}

func directoryError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if os.IsPermission(err) {
		status = http.StatusForbidden
	} else if os.IsNotExist(err) {
		status = http.StatusNotFound
	}
	http.Error(w, "Cannot open folder: "+err.Error(), status)
}
