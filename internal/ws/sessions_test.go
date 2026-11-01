package ws

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anagh/pocket-terminal/internal/pty"
	"github.com/gorilla/websocket"
)

type fakeTerminal struct {
	reader *io.PipeReader
	output *io.PipeWriter
	done   chan struct{}
	once   sync.Once
}

func newFakeTerminal() *fakeTerminal {
	r, w := io.Pipe()
	return &fakeTerminal{reader: r, output: w, done: make(chan struct{})}
}
func (f *fakeTerminal) Read(p []byte) (int, error)  { return f.reader.Read(p) }
func (f *fakeTerminal) Write(p []byte) (int, error) { return len(p), nil }
func (f *fakeTerminal) Resize(uint16, uint16) error { return nil }
func (f *fakeTerminal) Wait() error                 { <-f.done; return nil }
func (f *fakeTerminal) Close() error {
	f.once.Do(func() { close(f.done); _ = f.reader.Close(); _ = f.output.Close() })
	return nil
}

func sessionRequest(t *testing.T, h *Handler, method, query, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/sessions?token=test&"+query, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Sessions(w, r)
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	return w
}

func TestSessionSurvivesDisconnectAndReplaysOutput(t *testing.T) {
	h := NewHandler("test", "fake")
	h.Dir = t.TempDir()
	defer h.Close()
	fake := newFakeTerminal()
	starts := 0
	h.start = func(opts pty.Options) (pty.Terminal, error) {
		starts++
		if opts.Dir != h.Dir {
			t.Fatalf("shell dir %q, want %q", opts.Dir, h.Dir)
		}
		return fake, nil
	}
	w := sessionRequest(t, h, "POST", "", `{}`, 201)
	var info sessionInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	attach := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?token=test&session="+info.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		return conn
	}
	first := attach()
	_, _, err := first.ReadMessage() // initial replay, possibly empty
	if err != nil {
		t.Fatal(err)
	}
	_ = first.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1000, "switch"), time.Now().Add(time.Second))
	_ = first.Close()
	if _, err := fake.output.Write([]byte("still running\r\n")); err != nil {
		t.Fatal(err)
	}
	second := attach()
	defer second.Close()
	var output []byte
	for !bytes.Contains(output, []byte("still running")) {
		_, data, err := second.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, data...)
	}
	if starts != 1 {
		t.Fatalf("spawned %d shells, want 1", starts)
	}
	select {
	case <-fake.done:
		t.Fatal("disconnect terminated shell")
	default:
	}
	sessionRequest(t, h, "DELETE", "id="+info.ID, "", 204)
	select {
	case <-fake.done:
	case <-time.After(time.Second):
		t.Fatal("explicit close did not terminate shell")
	}
	sessionRequest(t, h, "GET", "", "", 200)
}

func TestProjectsAndAuthentication(t *testing.T) {
	h := NewHandler("test", "fake")
	h.Dir = t.TempDir()
	defer h.Close()
	h.start = func(pty.Options) (pty.Terminal, error) { return newFakeTerminal(), nil }
	unauthorized := httptest.NewRecorder()
	h.Sessions(unauthorized, httptest.NewRequest("POST", "/api/sessions", strings.NewReader(`{}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatal("missing authentication accepted")
	}
	sessionRequest(t, h, "POST", "", `{}`, 201)
	other := t.TempDir()
	body, _ := json.Marshal(map[string]string{"project": other, "shell": "fake"})
	sessionRequest(t, h, "POST", "", string(body), 201)
	for _, dir := range []string{h.Dir, other} {
		w := sessionRequest(t, h, "GET", "project="+url.QueryEscape(dir), "", 200)
		var listing struct {
			Sessions []sessionInfo `json:"sessions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.Sessions) != 1 || listing.Sessions[0].Project != dir {
			t.Fatalf("incorrect project listing: %s", w.Body.String())
		}
	}
	sessionRequest(t, h, "POST", "", `{"project":"does-not-exist"}`, 400)
	sessionRequest(t, h, "DELETE", "id=missing", "", 404)
}

func TestHistoryIsBoundedAndSlowClientDetaches(t *testing.T) {
	ch := make(chan []byte, 1)
	s := &session{clients: map[chan []byte]struct{}{ch: {}}}
	s.publish(bytes.Repeat([]byte("x"), historyLimit+100))
	s.publish([]byte("tail"))
	if len(s.history) != historyLimit || !bytes.HasSuffix(s.history, []byte("tail")) {
		t.Fatal("history not bounded correctly")
	}
	if len(s.clients) != 0 {
		t.Fatal("slow client still subscribed")
	}
}

func TestSlowShellStartupDoesNotBlockSessionListing(t *testing.T) {
	h := NewHandler("test", "fake")
	h.Dir = t.TempDir()
	defer h.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	h.start = func(pty.Options) (pty.Terminal, error) {
		close(started)
		<-release
		return newFakeTerminal(), nil
	}
	created := make(chan struct{})
	go func() {
		defer close(created)
		h.Sessions(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/sessions?token=test", strings.NewReader(`{}`)))
	}()
	<-started
	listed := make(chan struct{})
	go func() {
		h.Sessions(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/sessions?token=test", nil))
		close(listed)
	}()
	select {
	case <-listed:
	case <-time.After(time.Second):
		close(release)
		<-created
		t.Fatal("slow shell startup blocked other sessions")
	}
	close(release)
	<-created
}
