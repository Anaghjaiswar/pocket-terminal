package ws

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anagh/pocket-terminal/internal/pty"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// controlMessage represents a JSON control payload sent from the browser client
type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	Data string `json:"data,omitempty"`
}

// Handler manages WebSocket upgrades and bridges bidirectional I/O with a spawned PTY session.
type Handler struct {
	Token    string
	Shell    string
	Dir      string
	mu       sync.Mutex
	sessions map[string]*session
	start    func(pty.Options) (pty.Terminal, error)
	closed   bool
	starting int
	upgrader websocket.Upgrader
}

// NewHandler creates a new WebSocket PTY handler.
func NewHandler(token, shell string) *Handler {
	return &Handler{
		Token:    token,
		Shell:    shell,
		sessions: make(map[string]*session),
		start:    pty.Start,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// resolveShell picks the requested shell or falls back to server default.
func (h *Handler) resolveShell(requested string) string {
	req := strings.ToLower(strings.TrimSpace(requested))
	if req == "" {
		return h.Shell
	}

	if runtime.GOOS == "windows" {
		switch req {
		case "gitbash", "git-bash", "bash":
			gitBashPaths := []string{
				`C:\Program Files\Git\bin\bash.exe`,
				`C:\Program Files\Git\usr\bin\bash.exe`,
				`C:\Users\Anagh\AppData\Local\Programs\Git\bin\bash.exe`,
			}
			for _, p := range gitBashPaths {
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
			return "bash.exe"
		case "powershell", "pwsh":
			return "powershell.exe"
		case "cmd":
			return "cmd.exe"
		case "wsl", "ubuntu":
			return "wsl.exe"
		default:
			return req
		}
	}

	// On Linux / macOS / Docker
	switch req {
	case "bash":
		return "/bin/bash"
	case "sh":
		return "/bin/sh"
	case "zsh":
		return "/bin/zsh"
	default:
		return req
	}
}
