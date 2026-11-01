//go:build windows

package pty

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/UserExistsError/conpty"
)

// conptyTerminal wraps the true Windows PseudoConsole (ConPTY)
type conptyTerminal struct {
	cpty   *conpty.ConPty
	closed bool
	mu     sync.Mutex
}

func (t *conptyTerminal) Read(p []byte) (int, error) {
	return t.cpty.Read(p)
}

func (t *conptyTerminal) Write(p []byte) (int, error) {
	return t.cpty.Write(p)
}

func (t *conptyTerminal) Resize(cols, rows uint16) error {
	return t.cpty.Resize(int(cols), int(rows))
}

func (t *conptyTerminal) Wait() error {
	_, err := t.cpty.Wait(context.Background())
	return err
}

func (t *conptyTerminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.cpty.Close()
}

func startPlatformPTY(opts Options) (Terminal, error) {
	if strings.EqualFold(filepath.Base(opts.Shell), "wsl.exe") {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := checkWSL(ctx, opts); err != nil {
			return nil, err
		}
	}
	if conpty.IsConPtyAvailable() {
		shellCmd := opts.Shell
		base := strings.ToLower(filepath.Base(opts.Shell))
		if strings.Contains(base, "powershell") || strings.Contains(base, "pwsh") {
			shellCmd = "powershell.exe -NoLogo"
		} else if strings.Contains(base, "bash") {
			shellCmd = fmt.Sprintf(`"%s" --login -i`, opts.Shell)
		} else if strings.Contains(base, "cmd") {
			shellCmd = "cmd.exe /k"
		} else if base == "wsl.exe" {
			shellCmd = syscall.EscapeArg(opts.Shell) + " --cd " + syscall.EscapeArg(opts.Dir)
		} else if strings.Contains(opts.Shell, " ") && !strings.HasPrefix(opts.Shell, `"`) {
			shellCmd = fmt.Sprintf(`"%s"`, opts.Shell)
		}

		cpty, err := conpty.Start(
			shellCmd,
			conpty.ConPtyDimensions(int(opts.Cols), int(opts.Rows)),
			conpty.ConPtyWorkDir(opts.Dir),
			conpty.ConPtyEnv(opts.Env),
		)
		if err == nil {
			return &conptyTerminal{cpty: cpty}, nil
		}
	}

	// Fallback to piped terminal if ConPTY is not supported
	return startPipedTerminal(opts)
}

// Probe WSL before creating a console: wsl.exe can exist and spawn successfully
// while its service is stuck, leaving a blank console indefinitely.
func checkWSL(ctx context.Context, opts Options) error {
	cmd := exec.CommandContext(ctx, opts.Shell, "--cd", opts.Dir, "--exec", "/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = opts.Env
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("WSL startup timed out: Ubuntu/WSL is not responding on the laptop. Save Linux and Docker work, then restart WSL and try again")
	}
	if err != nil {
		return fmt.Errorf("WSL could not start: %s (%w)", decodeWSLOutput(output), err)
	}
	return nil
}

func decodeWSLOutput(output []byte) string {
	// Windows-side WSL errors use UTF-16LE when redirected; Linux output uses UTF-8.
	if len(output) >= 2 && (output[1] == 0 || (output[0] == 0xff && output[1] == 0xfe)) {
		units := make([]uint16, 0, len(output)/2)
		for i := 0; i+1 < len(output); i += 2 {
			units = append(units, uint16(output[i])|uint16(output[i+1])<<8)
		}
		return strings.TrimSpace(strings.TrimPrefix(string(utf16.Decode(units)), "\ufeff"))
	}
	return strings.TrimSpace(string(output))
}

type windowsTerminal struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	outReader *io.PipeReader
	outWriter *io.PipeWriter
	mu        sync.Mutex
	cols      uint16
	rows      uint16
	closed    bool
}

func startPipedTerminal(opts Options) (Terminal, error) {
	shellExe := opts.Shell
	args := []string{}

	base := strings.ToLower(filepath.Base(shellExe))
	if strings.Contains(base, "powershell") || strings.Contains(base, "pwsh") {
		args = []string{"-NoLogo", "-NoExit"}
	} else if strings.Contains(base, "bash") {
		args = []string{"--login", "-i"}
	} else if strings.Contains(base, "cmd") {
		args = []string{"/k"}
	} else if base == "wsl.exe" {
		args = []string{"--cd", opts.Dir}
	}

	cmd := exec.Command(shellExe, args...)
	cmd.Dir = opts.Dir
	cmd.Env = append(opts.Env,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"FORCE_COLOR=1",
		fmt.Sprintf("COLUMNS=%d", opts.Cols),
		fmt.Sprintf("LINES=%d", opts.Rows),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start shell %q: %w", shellExe, err)
	}

	pr, pw := io.Pipe()
	t := &windowsTerminal{
		cmd:       cmd,
		stdin:     stdin,
		outReader: pr,
		outWriter: pw,
		cols:      opts.Cols,
		rows:      opts.Rows,
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stderr)
	}()
	go func() {
		wg.Wait()
		_ = pw.Close()
	}()

	return t, nil
}

func (t *windowsTerminal) Read(p []byte) (int, error) {
	return t.outReader.Read(p)
}

func (t *windowsTerminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return 0, os.ErrClosed
	}

	if bytes.Contains(p, []byte{0x03}) {
		t.sendCtrlBreak()
		_, _ = t.stdin.Write([]byte("\r\n"))
		return len(p), nil
	}

	var normalized bytes.Buffer
	for i := 0; i < len(p); i++ {
		if p[i] == '\r' {
			normalized.WriteByte('\r')
			if i+1 >= len(p) || p[i+1] != '\n' {
				normalized.WriteByte('\n')
			}
		} else {
			normalized.WriteByte(p[i])
		}
	}

	_, err := t.stdin.Write(normalized.Bytes())
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *windowsTerminal) Resize(cols, rows uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cols = cols
	t.rows = rows
	return nil
}

func (t *windowsTerminal) Wait() error {
	return t.cmd.Wait()
}

func (t *windowsTerminal) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	_ = t.stdin.Close()
	_ = t.outWriter.Close()
	_ = t.outReader.Close()

	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return nil
}

func (t *windowsTerminal) sendCtrlBreak() {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GenerateConsoleCtrlEvent")
	_, _, _ = proc.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(t.cmd.Process.Pid))
}
