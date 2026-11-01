//go:build windows

package pty

import (
	"context"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestDecodeWSLOutput(t *testing.T) {
	message := "WSL could not start: Ubuntu – error"
	var encoded []byte
	for _, unit := range utf16.Encode([]rune("\ufeff" + message + "\r\n")) {
		encoded = append(encoded, byte(unit), byte(unit>>8))
	}
	for _, output := range [][]byte{encoded, []byte(message + "\r\n")} {
		if got := decodeWSLOutput(output); got != message {
			t.Fatalf("decoded %q, want %q", got, message)
		}
	}
}

func TestWSLStartupTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := checkWSL(ctx, Options{Shell: "wsl.exe", Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "WSL startup timed out") {
		t.Fatalf("expected actionable timeout, got %v", err)
	}
}
