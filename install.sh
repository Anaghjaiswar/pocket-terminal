#!/usr/bin/env sh
# PocketTerminal Unix Installer (Linux & macOS)
# Usage: curl -fsSL https://raw.githubusercontent.com/Anaghjaiswar/pocket-terminal/main/install.sh | sh

set -e

REPO="Anaghjaiswar/pocket-terminal"
INSTALL_DIR="$HOME/.local/bin"

echo "========================================="
echo "   Installing PocketTerminal for Unix    "
echo "========================================="

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux*)  TARGET_OS="linux" ;;
  darwin*) TARGET_OS="darwin" ;;
  *)
    echo "Error: Unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) TARGET_ARCH="amd64" ;;
  arm64|aarch64) TARGET_ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Fallback for linux-arm64 if needed, otherwise amd64
if [ "$TARGET_OS" = "linux" ] && [ "$TARGET_ARCH" != "amd64" ]; then
  echo "Notice: Falling back to amd64 for Linux."
  TARGET_ARCH="amd64"
fi

ARTIFACT="pocket-terminal-${TARGET_OS}-${TARGET_ARCH}.tar.gz"

echo "Fetching latest release tag..."
TAG=$(curl -sSL -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "v1.0.0")

if [ -z "$TAG" ]; then
  TAG="v1.0.0"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARTIFACT}"

echo "Downloading ${DOWNLOAD_URL}..."
TMP_DIR="$(mktemp -d)"
curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARTIFACT}"

mkdir -p "$INSTALL_DIR"
tar -xzf "${TMP_DIR}/${ARTIFACT}" -C "$TMP_DIR"

mv "${TMP_DIR}/pocket-terminal" "${INSTALL_DIR}/pocket-terminal"
chmod +x "${INSTALL_DIR}/pocket-terminal"
rm -rf "$TMP_DIR"

echo ""
echo "PocketTerminal installed successfully to ${INSTALL_DIR}/pocket-terminal!"
echo ""

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Note: Make sure ${INSTALL_DIR} is in your PATH. Add this to your ~/.bashrc or ~/.zshrc:"
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    ;;
esac

echo "Run it anytime by typing:"
echo "    pocket-terminal"
echo ""
