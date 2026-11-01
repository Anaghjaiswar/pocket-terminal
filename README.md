# PocketTerminal

**PocketTerminal** is a lightweight, single-binary remote terminal engineered in Go for prompting AI coding CLIs (`aider`, `codex`, `claude`, `git`, etc.) running on your main laptop directly from a mobile phone browser—with **zero configuration**.

Run one command, scan the terminal ASCII QR code on your phone, and immediately interact with your desktop shell using a mobile-optimized touch keypad and built-in **Web Speech API voice dictation**.

## Key Features

- **Project terminal sessions**: Open multiple terminal tabs in a project folder. Switching tabs, refreshing the browser, or losing the connection preserves the shells and running commands. Reconnecting replays up to 1 MiB of recent output per session. Use **+ Terminal** to create a shell and **×** to terminate it. Up to 32 sessions are retained until explicitly closed or the server stops; sessions do not survive a server restart.
- **Browse project folders**: Select **Browse ▾** beside Project, choose a laptop drive from the dropdown, navigate folders, and select **Open This Project**. You can also enter a folder path directly. Folder access uses the server's existing authentication and filesystem permissions.

- **Single Self-Contained Binary (`//go:embed`)**: Bundles the mobile web UI (`index.html`, `style.css`, `app.js`) directly inside the compiled Go executable.
- **Cross-Platform Shell Support**:
  - **Windows**: Spawns `powershell.exe` (with automatic fallback to `cmd.exe`), merging `stdout`/`stderr` pipes with CRLF normalization and `Ctrl+C` break signaling.
  - **Linux / macOS / Docker**: Spawns a full pseudo-terminal (`pty`) via `github.com/creack/pty` with live window resize (`TIOCSWINSZ`) support.
- **Mobile-First Developer UI**:
  - Powered by **xterm.js** and **xterm-addon-fit** in a Catppuccin Mocha dark theme.
  - **Persistent Touch Keypad Bar**: One-tap `[Esc]`, `[Tab]`, `[Ctrl+C]`, `[↑ Up]`, `[↓ Down]`, `[Enter]`, and `[Clear]` buttons.
  - **Voice-to-Text Dictation**: Floating microphone button powered by `webkitSpeechRecognition` (`SpeechRecognition`) plus a multi-line Prompt Composer drawer so you can speak natural-language prompts directly into AI coding CLIs.
- **Cryptographic Token Authentication**: Generates a random 16-character hex token on every startup, validates `?token=` via constant-time comparison, and prints a ready-to-scan ASCII QR code in the terminal.
- **Zero-Config Public HTTPS Tunneling (`--tunnel`)**: Automatically spawns `cloudflared tunnel --url http://localhost:8080` (downloading the minimal `cloudflared` binary on demand if missing) and encodes the public `https://*.trycloudflare.com/?token=...` link into the startup QR code.

## Project Structure

| Path | Responsibility |
| :--- | :--- |
| `go.mod` | Go 1.22 module definition (`github.com/anagh/pocket-terminal`) and core dependencies |
| `main.go` | CLI flags (`-port`, `-shell`, `-tunnel`, `-token`), ASCII banner, QR code rendering, HTTP server |
| `internal/auth/token.go` | Cryptographic 16-char hex token generation and constant-time validation middleware |
| `internal/pty/pty.go` | Cross-platform `Terminal` interface (`Read`, `Write`, `Resize`, `Close`, `Wait`) |
| `internal/pty/pty_windows.go` | Windows interactive shell execution (`powershell.exe` / `cmd.exe`) with raw stdio piping |
| `internal/pty/pty_linux.go` | Linux/Unix PTY implementation powered by `github.com/creack/pty` |
| `internal/ws/handler.go` | Gorilla WebSocket upgrader, bidirectional PTY stream bridge, and JSON `resize` handler |
| `internal/tunnel/cloudflare.go` | Automatic Cloudflare Quick Tunnel launcher and public URL extractor |
| `web/embed.go` | `//go:embed static/*` static asset server |
| `web/static/index.html` | Mobile-first terminal layout, voice dictation button, prompt drawer, and touch keypad |
| `web/static/app.js` | WebSocket client, auto-reconnect, `FitAddon` resize sync, and `webkitSpeechRecognition` |
| `web/static/style.css` | Catppuccin Mocha dark theme and mobile viewport (`100dvh`) styling |
| `Dockerfile` | Multi-stage Alpine container build for Linux/Docker deployments |

## Quickstart (Windows `D:\phone-laptop-config`)

1. Open PowerShell in `D:\phone-laptop-config` and fetch dependencies:

```powershell
go mod tidy
```

2. Build the single Windows executable:

```powershell
go build -o pocket-terminal.exe .
```

3. Run PocketTerminal on your laptop:

```powershell
.\pocket-terminal.exe
```

4. For instant access outside your home Wi-Fi (over 4G/5G), enable the Cloudflare Quick Tunnel:

```powershell
.\pocket-terminal.exe --tunnel
```

## CLI Flags & Environment Variables

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-port` | — | `8080` | Local TCP port for the HTTP & WebSocket server |
| `-project` | — | Current folder | Default working directory for new terminal sessions; the Project field can open other folders on the laptop |
| `-shell` | `SHELL` | `powershell.exe` (Win) / `/bin/bash` (Linux) | Interactive shell binary to launch |
| `-tunnel` | `ENABLE_TUNNEL=true` | `false` | Spawn a zero-config Cloudflare Quick Tunnel (`*.trycloudflare.com`) |
| `-token` | `POCKET_TERMINAL_TOKEN` | *(auto-generated)* | Custom session token (defaults to 16-char random hex) |

## Docker Usage

Build and run PocketTerminal inside a container:

```bash
docker build -t pocket-terminal .
docker run --rm -it -p 8080:8080 -e ENABLE_TUNNEL=true pocket-terminal
```
