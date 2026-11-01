package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anagh/pocket-terminal/internal/auth"
	"github.com/anagh/pocket-terminal/internal/pty"
	"github.com/gorilla/websocket"
)

const historyLimit = 1024 * 1024

type sessionInfo struct {
	ID      string    `json:"id"`
	Shell   string    `json:"shell"`
	Project string    `json:"project"`
	Created time.Time `json:"created"`
	Exited  bool      `json:"exited"`
}

// A session owns its shell independently of browser connections. Slow readers
// are detached and can reconnect using the bounded output history.
type session struct {
	mu         sync.Mutex
	info       sessionInfo
	term       pty.Terminal
	history    []byte
	clients    map[chan []byte]struct{}
	done       chan struct{}
	finishOnce sync.Once
	stopOnce   sync.Once
}

func (s *session) publish(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chunk := append([]byte(nil), data...)
	s.history = append(s.history, chunk...)
	if len(s.history) > historyLimit {
		s.history = append([]byte(nil), s.history[len(s.history)-historyLimit:]...)
	}
	for ch := range s.clients {
		select {
		case ch <- chunk:
		default:
			delete(s.clients, ch)
			close(ch)
		}
	}
}

func (s *session) finish() {
	s.finishOnce.Do(func() {
		s.stop()
		s.mu.Lock()
		s.info.Exited = true
		close(s.done)
		s.mu.Unlock()
	})
}

func (s *session) stop() {
	s.stopOnce.Do(func() { _ = s.term.Close() })
}

func (s *session) pump() {
	readDone := make(chan struct{})
	go func() {
		_ = s.term.Wait()
		// ConPTY needs closing after process exit to unblock its output pipe.
		// Keep draining that pipe before telling attached browsers to close.
		s.stop()
		<-readDone
		s.publish([]byte("\r\n\x1b[33m[Process exited]\x1b[0m\r\n"))
		s.finish()
	}()
	buf := make([]byte, 4096)
	for {
		n, err := s.term.Read(buf)
		if n > 0 {
			s.publish(buf[:n])
		}
		if err != nil {
			break
		}
	}
	close(readDone)
	s.stop()
}

func (h *Handler) projectDir(dir string) (string, error) {
	base := h.Dir
	if base == "" {
		base = "."
	}
	if strings.TrimSpace(dir) == "" {
		dir = base
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project must be a directory")
	}
	return dir, nil
}

// Sessions serves authenticated listing, creation and explicit termination.
func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	if !auth.ValidateRequest(r, h.Token) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		dir, err := h.projectDir(r.URL.Query().Get("project"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		items := []sessionInfo{}
		h.mu.Lock()
		for _, s := range h.sessions {
			s.mu.Lock()
			if s.info.Project == dir {
				items = append(items, s.info)
			}
			s.mu.Unlock()
		}
		h.mu.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i].Created.Before(items[j].Created) })
		_ = json.NewEncoder(w).Encode(struct {
			Project  string        `json:"project"`
			Sessions []sessionInfo `json:"sessions"`
		}{dir, items})
	case http.MethodPost:
		var req struct {
			Shell   string `json:"shell"`
			Project string `json:"project"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
			http.Error(w, "Invalid session request", 400)
			return
		}
		dir, err := h.projectDir(req.Project)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			http.Error(w, "Cannot create session ID", 500)
			return
		}
		h.mu.Lock()
		if h.closed || len(h.sessions)+h.starting >= 32 {
			h.mu.Unlock()
			http.Error(w, "Session limit reached or server shutting down", 503)
			return
		}
		h.starting++
		h.mu.Unlock()
		term, err := h.start(pty.Options{Shell: h.resolveShell(req.Shell), Dir: dir, Cols: 80, Rows: 24})
		h.mu.Lock()
		h.starting--
		if err != nil {
			h.mu.Unlock()
			http.Error(w, "Failed to launch shell: "+err.Error(), 500)
			return
		}
		if h.closed {
			h.mu.Unlock()
			go func() { _ = term.Wait() }()
			_ = term.Close()
			http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
			return
		}
		s := &session{info: sessionInfo{ID: hex.EncodeToString(id[:]), Shell: req.Shell, Project: dir, Created: time.Now()}, term: term, clients: make(map[chan []byte]struct{}), done: make(chan struct{})}
		if s.info.Shell == "" {
			s.info.Shell = h.Shell
		}
		h.sessions[s.info.ID] = s
		created := s.info
		h.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
		go s.pump()
		log.Printf("[session] created %s in %s (%s)", s.info.ID, dir, s.info.Shell)
	case http.MethodDelete:
		h.mu.Lock()
		s := h.sessions[r.URL.Query().Get("id")]
		if s != nil {
			delete(h.sessions, s.info.ID)
		}
		h.mu.Unlock()
		if s == nil {
			http.Error(w, "Session not found", 404)
			return
		}
		s.finish()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "Method not allowed", 405)
	}
}

func (h *Handler) Close() {
	h.mu.Lock()
	h.closed = true
	items := h.sessions
	h.sessions = make(map[string]*session)
	h.mu.Unlock()
	for _, s := range items {
		s.finish()
	}
}

// ServeHTTP attaches a browser to an existing shell; socket closure only detaches.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !auth.ValidateRequest(r, h.Token) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	h.mu.Lock()
	s := h.sessions[r.URL.Query().Get("session")]
	h.mu.Unlock()
	if s == nil {
		http.Error(w, "Session not found", 404)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch := make(chan []byte, 256)
	s.mu.Lock()
	history := append([]byte(nil), s.history...)
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
		log.Printf("[ws] detached %s from session %s (shell retained)", r.RemoteAddr, s.info.ID)
	}()
	write := func(kind int, data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
		return conn.WriteMessage(kind, data)
	}
	if err := write(websocket.BinaryMessage, history); err != nil {
		return
	}
	detached := make(chan struct{})
	defer close(detached)
	go func() {
		defer conn.Close()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case data, ok := <-ch:
				if !ok || write(websocket.BinaryMessage, data) != nil {
					return
				}
			case <-ticker.C:
				if write(websocket.PingMessage, nil) != nil {
					return
				}
			case <-s.done:
				// Flush queued output before reporting exit.
				for {
					select {
					case data, ok := <-ch:
						if !ok || write(websocket.BinaryMessage, data) != nil {
							return
						}
					default:
						_ = write(websocket.TextMessage, []byte(`{"type":"exit"}`))
						_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Process exited"), time.Now().Add(writeWait))
						return
					}
				}
			case <-detached:
				return
			}
		}
	}()
	conn.SetReadLimit(1024 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage && len(payload) > 0 && payload[0] == '{' {
			var ctrl controlMessage
			if json.Unmarshal(payload, &ctrl) == nil && ctrl.Type != "" {
				switch ctrl.Type {
				case "resize":
					if ctrl.Cols > 0 && ctrl.Rows > 0 {
						_ = s.term.Resize(ctrl.Cols, ctrl.Rows)
					}
					continue
				case "input":
					payload = []byte(ctrl.Data)
				case "ping":
					continue
				}
			}
		}
		if len(payload) > 0 {
			if _, err := s.term.Write(payload); err != nil && err != io.EOF {
				return
			}
		}
	}
}
