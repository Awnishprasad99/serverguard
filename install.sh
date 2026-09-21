
#!/usr/bin/env bash

set -euo pipefail

REPO="Awnishprasad99/serverguard"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="serverguard"

echo "==> Installing ServerGuard..."

if [[ "$(id -u)" -ne 0 ]]; then
    echo "Error: Please run this script with sudo or as root."
    exit 1
fi

ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    *)
        echo "Error: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

if ! command -v curl >/dev/null 2>&1; then
    echo "Error: curl is required."
    exit 1
fi

if ! command -v tar >/dev/null 2>&1; then
    echo "Error: tar is required."
    exit 1
fi

API_URL="https://api.github.com/repos/${REPO}/releases/latest"

echo "==> Detecting latest release..."

ASSET_URL="$(
    curl -fsSL \
        -H "Accept: application/vnd.github+json" \
        "$API_URL" |
    grep '"browser_download_url":' |
    grep "serverguard-linux-${GOARCH}\.tar\.gz" |
    head -n 1 |
    cut -d '"' -f 4
)"

if [[ -z "$ASSET_URL" ]]; then
    echo "Error: No compatible release found for ${GOARCH}."
    exit 1
fi

TMP_DIR="$(mktemp -d)"

cleanup() {
    rm -rf "$TMP_DIR"
}

trap cleanup EXIT

ARCHIVE="${TMP_DIR}/serverguard.tar.gz"

echo "==> Downloading ServerGuard..."
curl -fL "$ASSET_URL" -o "$ARCHIVE"

echo "==> Extracting files..."
tar -xzf "$ARCHIVE" -C "$TMP_DIR"

BINARY_PATH="${TMP_DIR}/serverguard-linux-${GOARCH}"

if [[ ! -f "$BINARY_PATH" ]]; then
    echo "Error: Binary not found after extraction."
    exit 1
fi

chmod 0755 "$BINARY_PATH"

echo "==> Installing to ${INSTALL_DIR}..."
install -m 0755 "$BINARY_PATH" "${INSTALL_DIR}/${BINARY_NAME}"

echo
echo "ServerGuard installed successfully!"
echo "Run it with: serverguard"
