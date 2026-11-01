//go:build !windows

package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	creackpty "github.com/creack/pty"
)

// unixTerminal implements the Terminal interface on Linux, macOS, and Docker using github.com/creack/pty.
type unixTerminal struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	mu     sync.Mutex
	closed bool
}

func startPlatformPTY(opts Options) (Terminal, error) {
	cmd := exec.Command(opts.Shell)
	cmd.Dir = opts.Dir
	cmd.Env = append(opts.Env,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	winSize := &creackpty.Winsize{
		Cols: opts.Cols,
		Rows: opts.Rows,
	}

	ptmx, err := creackpty.StartWithSize(cmd, winSize)
	if err != nil {
		return nil, fmt.Errorf("failed to start pty with shell %q: %w", opts.Shell, err)
	}

	return &unixTerminal{
		cmd:  cmd,
		ptmx: ptmx,
	}, nil
}

func (t *unixTerminal) Read(p []byte) (int, error) {
	return t.ptmx.Read(p)
}

func (t *unixTerminal) Write(p []byte) (int, error) {
	return t.ptmx.Write(p)
}

func (t *unixTerminal) Resize(cols, rows uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return os.ErrClosed
	}
	return creackpty.Setsize(t.ptmx, &creackpty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

func (t *unixTerminal) Wait() error {
	return t.cmd.Wait()
}

func (t *unixTerminal) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	_ = t.ptmx.Close()
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return nil
}
