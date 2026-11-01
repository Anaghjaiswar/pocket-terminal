package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anagh/pocket-terminal/internal/auth"
	"github.com/anagh/pocket-terminal/internal/pty"
	"github.com/anagh/pocket-terminal/internal/tunnel"
	"github.com/anagh/pocket-terminal/internal/ws"
	"github.com/anagh/pocket-terminal/web"
	qrcode "github.com/skip2/go-qrcode"
)

func main() {
	port := flag.Int("port", 8080, "HTTP/WebSocket server port")
	project := flag.String("project", ".", "Default project directory for terminal sessions")
	shell := flag.String("shell", pty.DefaultShell(), "Interactive shell binary to launch (e.g. powershell.exe, cmd.exe, /bin/bash)")
	enableTunnel := flag.Bool("tunnel", envBool("ENABLE_TUNNEL"), "Automatically launch a Cloudflare Quick Tunnel for public HTTPS mobile access")
	customToken := flag.String("token", strings.TrimSpace(os.Getenv("POCKET_TERMINAL_TOKEN")), "Optional custom session token (auto-generated if empty)")
	flag.Parse()

	token := *customToken
	if token == "" {
		var err error
		token, err = auth.GenerateToken()
		if err != nil {
			log.Fatalf("Failed to generate secure session token: %v", err)
		}
	}

	staticHandler, err := web.StaticHandler()
	if err != nil {
		log.Fatalf("Failed to initialize embedded web assets: %v", err)
	}

	wsHandler := ws.NewHandler(token, *shell)
	wsHandler.Dir = *project
	defer wsHandler.Close()

	mux := http.NewServeMux()
	mux.Handle("/ws", wsHandler)
	mux.HandleFunc("/api/sessions", wsHandler.Sessions)
	mux.HandleFunc("/api/directories", wsHandler.Directories)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", auth.Middleware(token, staticHandler))

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	localIP := detectLANIP()
	localURL := fmt.Sprintf("http://localhost:%d/?token=%s", *port, token)
	lanURL := fmt.Sprintf("http://%s:%d/?token=%s", localIP, *port, token)
	qrTargetURL := lanURL

	var cfTunnel *tunnel.Tunnel
	if *enableTunnel {
		fmt.Println("[PocketTerminal] Starting Cloudflare Quick Tunnel...")
		cfTunnel, err = tunnel.Start(ctx, fmt.Sprintf("http://localhost:%d", *port))
		if err != nil {
			log.Printf("[PocketTerminal] Warning: Cloudflare tunnel failed (%v). Falling back to LAN URL.", err)
		} else {
			defer cfTunnel.Close()
			qrTargetURL = fmt.Sprintf("%s/?token=%s", strings.TrimRight(cfTunnel.URL, "/"), token)
		}
	}

	printBanner(*shell, token, localURL, lanURL, cfTunnel, qrTargetURL)

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	fmt.Println("\n[PocketTerminal] Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func printBanner(shell, token, localURL, lanURL string, cfTunnel *tunnel.Tunnel, qrTargetURL string) {
	fmt.Println("======================================================================")
	fmt.Println("  ____            _        _   _____                   _             _ ")
	fmt.Println(" |  _ \\ ___   ___| | _____| |_|_   _|__ _ __ _ __ ___ (_)_ __   __ _| |")
	fmt.Println(" | |_) / _ \\ / __| |/ / _ \\ __| | |/ _ \\ '__| '_ ` _ \\| | '_ \\ / _` | |")
	fmt.Println(" |  __/ (_) | (__|   <  __/ |_  | |  __/ |  | | | | | | | | | | (_| | |")
	fmt.Println(" |_|   \\___/ \\___|_|\\_\\___|\\__| |_|\\___|_|  |_| |_| |_|_|_| |_|\\__,_|_|")
	fmt.Println("======================================================================")
	fmt.Printf("  Shell       : %s\n", shell)
	fmt.Printf("  Auth Token  : %s\n", token)
	fmt.Printf("  Local URL   : %s\n", localURL)
	fmt.Printf("  LAN URL     : %s\n", lanURL)
	if cfTunnel != nil && cfTunnel.URL != "" {
		fmt.Printf("  Public URL  : %s/?token=%s\n", strings.TrimRight(cfTunnel.URL, "/"), token)
	}
	fmt.Println("----------------------------------------------------------------------")
	fmt.Println("  Scan this QR code with your mobile camera to open PocketTerminal:")
	fmt.Println()

	if qr, err := qrcode.New(qrTargetURL, qrcode.Medium); err == nil {
		fmt.Println(qr.ToSmallString(false))
	} else {
		fmt.Printf("  [QR error: %v]\n  Open directly: %s\n", err, qrTargetURL)
	}
	fmt.Println("======================================================================")
}

func detectLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && !localAddr.IP.IsLoopback() {
			return localAddr.IP.String()
		}
	}

	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}
