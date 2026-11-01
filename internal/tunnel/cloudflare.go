package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

var tryCloudflareRegex = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// Tunnel represents a running Cloudflare Quick Tunnel process.
type Tunnel struct {
	URL string
	cmd *exec.Cmd
}

// Start launches `cloudflared tunnel --url <localURL>`, automatically downloading a local
// cloudflared binary if one is not already installed in PATH, and waits until the public
// `https://*.trycloudflare.com` URL is emitted.
func Start(ctx context.Context, localURL string) (*Tunnel, error) {
	binPath, err := ensureCloudflared(ctx)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binPath, "tunnel", "--no-autoupdate", "--url", localURL)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to attach cloudflared stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to attach cloudflared stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start cloudflared: %w", err)
	}

	urlCh := make(chan string, 1)
	errCh := make(chan error, 2)
	scanStream := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if match := tryCloudflareRegex.FindString(line); match != "" {
				select {
				case urlCh <- match:
				default:
				}
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case errCh <- fmt.Errorf("failed to read cloudflared output: %w", err):
			default:
			}
		}
	}

	go scanStream(stdout)
	go scanStream(stderr)

	select {
	case publicURL := <-urlCh:
		return &Tunnel{
			URL: publicURL,
			cmd: cmd,
		}, nil
	case err := <-errCh:
		_ = cmd.Process.Kill()
		return nil, err
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("timed out waiting for Cloudflare Quick Tunnel URL")
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	}
}

// Close gracefully terminates the background cloudflared process.
func (t *Tunnel) Close() error {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	return t.cmd.Process.Kill()
}

func ensureCloudflared(ctx context.Context) (string, error) {
	if path, err := exec.LookPath("cloudflared"); err == nil {
		return path, nil
	}

	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	targetDir := filepath.Join(cacheDir, "pocket-terminal", "bin")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create tunnel cache dir: %w", err)
	}

	targetPath := filepath.Join(targetDir, binName)
	if info, err := os.Stat(targetPath); err == nil && info.Size() > 0 {
		return targetPath, nil
	}

	downloadURL, err := cloudflaredDownloadURL()
	if err != nil {
		return "", err
	}

	fmt.Printf("[Tunnel] cloudflared not found in PATH; downloading minimal binary from %s ...\n", downloadURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download cloudflared: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download cloudflared: HTTP %d", resp.StatusCode)
	}

	tmpPath := targetPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return "", err
	}
	_ = f.Close()

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func cloudflaredDownloadURL() (string, error) {
	const base = "https://github.com/cloudflare/cloudflared/releases/latest/download/"
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "386" {
			return base + "cloudflared-windows-386.exe", nil
		}
		return base + "cloudflared-windows-amd64.exe", nil
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return base + "cloudflared-linux-amd64", nil
		case "arm64":
			return base + "cloudflared-linux-arm64", nil
		case "arm":
			return base + "cloudflared-linux-arm", nil
		}
	}
	return "", fmt.Errorf("automatic cloudflared download is not supported for %s/%s; please install cloudflared in PATH", runtime.GOOS, runtime.GOARCH)
}
