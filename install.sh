#!/bin/sh
set -e

# fcc one-line installer
# Usage: curl -fsSL https://github.com/brucehuu/fcc/releases/latest/download/install.sh | sh

OS=$(uname -s)
if [ "$OS" != "Darwin" ]; then
    echo "Error: fcc only supports macOS."
    exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
    arm64)
        ASSET="fcc-darwin-arm64"
        ;;
    x86_64)
        ASSET="fcc-darwin-amd64"
        ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

DOWNLOAD_URL="https://github.com/brucehuu/fcc/releases/latest/download/${ASSET}"
SHA_URL="${DOWNLOAD_URL}.sha256"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading fcc (${ARCH})..."
curl -fsSL "$DOWNLOAD_URL" -o "$TMPDIR/$ASSET"
curl -fsSL "$SHA_URL" -o "$TMPDIR/$ASSET.sha256"

cd "$TMPDIR"
shasum -a 256 -c "$ASSET.sha256"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

chmod +x "$TMPDIR/$ASSET"
mv "$TMPDIR/$ASSET" "$INSTALL_DIR/fcc"

echo ""
echo "fcc installed to $INSTALL_DIR/fcc"
echo ""
echo "Run 'fcc' to start. First run will open a config window."
