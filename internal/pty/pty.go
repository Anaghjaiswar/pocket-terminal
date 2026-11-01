package pty

import (
	"io"
	"os"
	"runtime"
)

// Terminal defines a cross-platform interface for interacting with a pseudo-terminal or piped shell process.
type Terminal interface {
	io.ReadWriteCloser
	// Resize updates the terminal window dimensions (columns and rows).
	Resize(cols, rows uint16) error
	// Wait waits for the underlying shell process to exit.
	Wait() error
}

// Options configures how the shell process is spawned.
type Options struct {
	Shell string
	Dir   string
	Env   []string
	Cols  uint16
	Rows  uint16
}

// DefaultShell returns the platform-appropriate default interactive shell.
// On Windows it defaults to powershell.exe (falling back to cmd.exe);
// on Linux/macOS/Docker it checks $SHELL, then /bin/bash, then /bin/sh.
func DefaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// Start spawns a new interactive shell session using the platform-specific implementation.
func Start(opts Options) (Terminal, error) {
	if opts.Shell == "" {
		opts.Shell = DefaultShell()
	}
	if opts.Cols == 0 {
		opts.Cols = 80
	}
	if opts.Rows == 0 {
		opts.Rows = 24
	}
	if opts.Dir == "" {
		if wd, err := os.Getwd(); err == nil {
			opts.Dir = wd
		}
	}
	if len(opts.Env) == 0 {
		opts.Env = os.Environ()
	}
	return startPlatformPTY(opts)
}
