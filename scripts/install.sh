#!/bin/bash
set -euo pipefail

# ATC (Air Traffic Control) installer
# Usage: curl -fsSL https://raw.githubusercontent.com/kevinzwang/air-traffic-control/main/scripts/install.sh | bash

REPO="kevinzwang/air-traffic-control"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --version)
            VERSION="$2"
            shift 2
            ;;
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: install.sh [--version v1.0.0] [--install-dir /path]"
            exit 1
            ;;
    esac
done

# Detect OS
detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Darwin)
            echo "darwin"
            ;;
        Linux)
            echo "linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "windows"
            ;;
        *)
            echo "Unsupported OS: $os" >&2
            exit 1
            ;;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            echo "Unsupported architecture: $arch" >&2
            exit 1
            ;;
    esac
}

# Get latest release version from GitHub API
get_latest_version() {
    local latest
    latest=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ -z "$latest" ]]; then
        echo "Failed to fetch latest version" >&2
        exit 1
    fi
    echo "$latest"
}

main() {
    local os arch binary_name download_url

    os=$(detect_os)
    arch=$(detect_arch)

    # Get version
    if [[ -z "$VERSION" ]]; then
        echo "Fetching latest version..."
        VERSION=$(get_latest_version)
    fi
    echo "Installing ATC ${VERSION}..."

    # Construct binary name
    binary_name="atc-${os}-${arch}"
    if [[ "$os" == "windows" ]]; then
        binary_name="${binary_name}.exe"
    fi

    # Download URL
    download_url="https://github.com/${REPO}/releases/download/${VERSION}/${binary_name}"

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Download binary
    echo "Downloading ${binary_name}..."
    local target="${INSTALL_DIR}/atc"
    if [[ "$os" == "windows" ]]; then
        target="${INSTALL_DIR}/atc.exe"
    fi
    curl -fsSL "$download_url" -o "$target"

    # Set executable permissions (not needed on Windows)
    if [[ "$os" != "windows" ]]; then
        chmod +x "$target"
    fi

    echo ""
    echo "ATC ${VERSION} installed successfully to ${target}"
    echo ""

    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        echo "Add the following to your shell profile to use 'atc':"
        echo ""
        echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
        echo ""
    else
        echo "Run 'atc' to get started!"
    fi
}

main
