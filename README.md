# PocketTerminal

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)](https://github.com/Anaghjaiswar/pocket-terminal)

**PocketTerminal** is a lightweight, zero-dependency remote terminal engineered in Go for prompting AI coding CLIs (`agy`, `aider`, `codex`, `claude`, `gh`, etc.) running on your main laptop directly from a mobile phone browser—with **zero configuration**.

Run one command, scan the terminal ASCII QR code on your phone, and immediately interact with your desktop shell using a mobile-optimized touch keypad and built-in **Web Speech API voice dictation**.

---

## Key Features

- **📱 Mobile-First Developer UI**:
  - Powered by **xterm.js** and **xterm-addon-fit** in a Catppuccin Mocha dark theme.
  - **Persistent Touch Keypad**: One-tap `[Esc]`, `[Tab]`, `[Ctrl]`, `[Alt]`, `[Ctrl+C]`, `[Ctrl+D]`, `[📋 Paste]`, and cursor arrows (`←`, `→`, `↑`, `↓`).
  - **Voice-to-Text Dictation**: Floating microphone powered by the browser Web Speech API plus a multi-line Prompt Composer drawer to dictate complex instructions hands-free directly into AI coding CLIs.
- **🔄 Instant Shell Switching**:
  - Switch on the fly between **Git Bash**, **PowerShell**, **Command Prompt (CMD)**, and **WSL** (Windows Subsystem for Linux) without restarting the server.
- **📑 Multi-Terminal Sessions & Reconnection**:
  - Open multiple tabs in any project folder.
  - Refreshing your phone's browser or losing Wi-Fi signal retains the shells and running commands intact.
  - Reconnecting automatically replays up to 1 MiB of recent scrollback buffer per session.
- **📂 Remote Project Browser**:
  - Select **Browse ▾** to explore laptop drives and folders visually from your phone.
- **🔒 Cryptographic Security**:
  - Generates a cryptographically random 16-character hex token on every startup, validated via constant-time token comparison.
  - Renders a ready-to-scan ASCII QR code directly in your laptop terminal.
- **🌐 Zero-Config Public HTTPS Tunneling (`--tunnel`)**:
  - Automatically spawns Cloudflare Quick Tunnels (`*.trycloudflare.com`) on demand so you can prompt your laptop while away on 4G/5G mobile data without port forwarding.
- **📦 Single Self-Contained Binary (`//go:embed`)**:
  - Compiles the entire frontend (`HTML`, `CSS`, `JS`) directly inside the binary. No Node.js runtime required on the host.

---

## Quickstart

### Prerequisites
- [Go 1.22+](https://go.dev/dl/) installed.

### 1. Clone the repository
```bash
git clone https://github.com/Anaghjaiswar/pocket-terminal.git
cd pocket-terminal
```

### 2. Run directly
```bash
go run .
```
*(Or compile into a single binary: `go build -o pocket-terminal.exe .`)*

### 3. Scan & Connect
Your laptop will print an ASCII QR code with your LAN IP (e.g. `http://192.168.1.5:8080/?token=...`). Point your phone camera at the screen to open PocketTerminal immediately!

### 4. Remote Access over 4G/5G (Optional)
To prompt your laptop when you are away from home Wi-Fi:
```bash
.\pocket-terminal.exe --tunnel
```
PocketTerminal will establish a secure Cloudflare tunnel and generate a public HTTPS URL QR code.

---

## Project Structure

| Path | Responsibility |
| :--- | :--- |
| `main.go` | CLI flags (`-port`, `-shell`, `-tunnel`, `-token`), ASCII banner, QR code rendering, HTTP server |
| `internal/auth/token.go` | Cryptographic 16-char hex token generation and constant-time validation middleware |
| `internal/pty/pty.go` | Cross-platform `Terminal` interface (`Read`, `Write`, `Resize`, `Close`, `Wait`) |
| `internal/pty/pty_windows.go` | Windows native ConPTY execution (`bash.exe`, `powershell.exe`, `cmd.exe`, `wsl.exe`) |
| `internal/pty/pty_linux.go` | Linux/Unix PTY implementation powered by `github.com/creack/pty` |
| `internal/ws/handler.go` | WebSocket connection upgrader, bidirectional PTY stream bridge, and JSON resize protocol |
| `internal/ws/sessions.go` | Decoupled session manager, scrollback history replay buffer, and multi-tab lifecycle |
| `internal/ws/directories.go` | Filesystem navigation API for the remote project browser |
| `internal/tunnel/cloudflare.go` | Automatic Cloudflare Quick Tunnel launcher and public URL extractor |
| `web/embed.go` | `//go:embed static/*` static asset server |
| `web/static/` | Mobile-first terminal UI (`index.html`, `style.css`, `app.js`) |

---

## CLI Flags & Environment Variables

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-port` | — | `8080` | Local TCP port for HTTP & WebSocket server |
| `-project` | — | Current folder | Default working directory for new terminal sessions |
| `-shell` | `SHELL` | `powershell.exe` (Win) / `/bin/bash` (Linux) | Initial interactive shell binary |
| `-tunnel` | `ENABLE_TUNNEL=true` | `false` | Spawn a zero-config Cloudflare Quick Tunnel (`*.trycloudflare.com`) |
| `-token` | `POCKET_TERMINAL_TOKEN` | *(auto-generated)* | Custom session token (defaults to 16-char random hex) |

---

## Docker Usage

```bash
docker build -t pocket-terminal .
docker run --rm -it -p 8080:8080 pocket-terminal
```

---

## License

This project is licensed under the [MIT License](LICENSE).
