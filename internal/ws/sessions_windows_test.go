//go:build windows

package ws

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWindowsShellStartsInProject(t *testing.T) {
	for _, shell := range []string{"powershell", "gitbash"} {
		t.Run(shell, func(t *testing.T) {
			if shell == "powershell" {
				// Use installed Windows modules rather than unsigned modules
				// inherited from a host application's bundled runtime.
				t.Setenv("PSModulePath", filepath.Join(os.Getenv("SystemRoot"), `System32\WindowsPowerShell\v1.0\Modules`))
			}
			if shell == "gitbash" {
				if _, err := os.Stat(`C:\Program Files\Git\bin\bash.exe`); err != nil {
					t.Skip("Git Bash not installed")
				}
			}
			h := NewHandler("test", "powershell.exe")
			h.Dir = t.TempDir()
			defer h.Close()
			w := sessionRequest(t, h, "POST", "", `{"shell":"`+shell+`"}`, 201)
			var info sessionInfo
			if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(h)
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?token=test&session="+info.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
			_ = conn.WriteJSON(controlMessage{Type: "resize", Cols: 240, Rows: 24})
			command := "Write-Output ('PROJECT_CHECK:' + (Get-Location).Path)\r"
			if shell == "gitbash" {
				command = "printf 'PROJECT_CHECK:%s\\n' \"$(pwd -W)\"\r"
			}
			_ = conn.WriteJSON(controlMessage{Type: "input", Data: command})
			var output strings.Builder
			want := "PROJECT_CHECK:" + strings.ReplaceAll(h.Dir, `\`, "/")
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					t.Fatalf("project directory not received: %v; output: %q", err, output.String())
				}
				output.Write(data)
				if strings.Contains(strings.ReplaceAll(output.String(), `\`, "/"), want) {
					break
				}
			}
		})
	}
}
