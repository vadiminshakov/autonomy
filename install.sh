#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

BINARY_NAME="autonomy"
INSTALL_DIR="/usr/local/bin"
REPO_URL="https://github.com/vadiminshakov/autonomy"
GITHUB_API_URL="https://api.github.com/repos/vadiminshakov/autonomy"

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

detect_os_arch() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $OS in
        linux*)
            GOOS="linux"
            ;;
        darwin*)
            GOOS="darwin"
            ;;
        cygwin*|mingw*|msys*)
            GOOS="windows"
            ;;
        *)
            print_error "unsupported operating system: $OS"
            exit 1
            ;;
    esac
    
    case $ARCH in
        x86_64|amd64)
            GOARCH="amd64"
            ;;
        aarch64|arm64)
            GOARCH="arm64"
            ;;
        *)
            print_error "unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    print_status "detected system: $GOOS-$GOARCH"
}

get_latest_release() {
    print_status "getting latest release information..."
    
    if command -v curl &> /dev/null; then
        LATEST_RELEASE=$(curl -s "$GITHUB_API_URL/releases/latest" | grep -o '"tag_name": "[^"]*' | grep -o '[^"]*$' 2>/dev/null)
    elif command -v wget &> /dev/null; then
        LATEST_RELEASE=$(wget -qO- "$GITHUB_API_URL/releases/latest" | grep -o '"tag_name": "[^"]*' | grep -o '[^"]*$' 2>/dev/null)
    else
        print_error "curl or wget is required to download binary"
        exit 1
    fi
    
    if [[ -z "$LATEST_RELEASE" ]]; then
        print_error "failed to get latest release information"
        exit 1
    fi
    
    print_status "latest release: $LATEST_RELEASE"
    return 0
}

download_binary() {
    if [[ "$GOOS" == "windows" ]]; then
        ARCHIVE_NAME="autonomy-${GOOS}-${GOARCH}.zip"
        BINARY_FILE="autonomy.exe"
    else
        ARCHIVE_NAME="autonomy-${GOOS}-${GOARCH}.tar.gz"
        BINARY_FILE="autonomy"
    fi
    
    DOWNLOAD_URL="$REPO_URL/releases/download/$LATEST_RELEASE/$ARCHIVE_NAME"
    
    print_status "downloading binary: $DOWNLOAD_URL"
    
    if command -v curl &> /dev/null; then
        if ! curl -sL "$DOWNLOAD_URL" -o "$ARCHIVE_NAME"; then
            print_error "failed to download binary"
            exit 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -q "$DOWNLOAD_URL" -O "$ARCHIVE_NAME"; then
            print_error "failed to download binary"
            exit 1
        fi
    else
        print_error "curl or wget is required to download binary"
        exit 1
    fi
    
    print_status "extracting archive..."
    
    if [[ "$GOOS" == "windows" ]]; then
        if command -v unzip &> /dev/null; then
            unzip -q "$ARCHIVE_NAME"
        else
            print_error "unzip is required to extract archive"
            exit 1
        fi
    else
        tar -xzf "$ARCHIVE_NAME"
    fi
    
    if [[ ! -f "$BINARY_FILE" ]]; then
        print_error "binary not found after extraction"
        exit 1
    fi
    
    # rename for consistency
    if [[ "$BINARY_FILE" != "$BINARY_NAME" ]]; then
        mv "$BINARY_FILE" "$BINARY_NAME"
    fi
    
    # remove archive
    rm -f "$ARCHIVE_NAME"
    
    print_status "binary downloaded successfully"
    return 0
}

check_permissions() {
    if [[ "$EUID" -ne 0 ]]; then
        print_warning "script not running as root. Trying to install with sudo..."
        if ! sudo -n true 2>/dev/null; then
            print_error "sudo privileges required for installation to $INSTALL_DIR"
            echo "run script with sudo or ensure you have write permissions to $INSTALL_DIR"
            exit 1
        fi
        USE_SUDO=true
    else
        USE_SUDO=false
    fi
}

install_binary() {
    print_status "installing binary to $INSTALL_DIR..."
    
    if [[ "$USE_SUDO" == true ]]; then
        sudo mv "$BINARY_NAME" "$INSTALL_DIR/"
        sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"
    else
        mv "$BINARY_NAME" "$INSTALL_DIR/"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"
    fi
    
    if [[ $? -eq 0 ]]; then
        print_status "binary installed successfully to $INSTALL_DIR/$BINARY_NAME"
    else
        print_error "failed to install binary"
        exit 1
    fi
}

verify_installation() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        print_status "installation completed successfully!"
        print_status "you can now run '$BINARY_NAME' from any directory"
    else
        print_warning "binary installed but not found in PATH"
        print_warning "ensure $INSTALL_DIR is in your PATH"
    fi
}

cleanup() {
    rm -f "$BINARY_NAME" autonomy-*.tar.gz autonomy-*.zip autonomy.exe
}

show_usage() {
    echo "Installation script for $BINARY_NAME"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -d, --dir DIR        install to specified directory (default: $INSTALL_DIR)"
    echo "  -v, --version VER    install specific version (e.g., v1.0.0)"
    echo "  -h, --help           show this help"
    echo ""
    echo "Examples:"
    echo "  $0                        # download and install latest version"
    echo "  $0 -d ~/bin               # install to ~/bin"
    echo "  $0 -v v1.0.0              # install version v1.0.0"
    echo "  sudo $0                   # install with root privileges"
}

main() {
    SPECIFIC_VERSION=""
    
    # parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -d|--dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -v|--version)
                SPECIFIC_VERSION="$2"
                shift 2
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                print_error "unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    print_status "starting installation of $BINARY_NAME..."
    
    # create install directory if it doesn't exist
    if [[ ! -d "$INSTALL_DIR" ]]; then
        print_status "creating directory $INSTALL_DIR..."
        if [[ "$INSTALL_DIR" == "/usr/local/bin" ]] || [[ "$INSTALL_DIR" == "/usr/bin" ]]; then
            sudo mkdir -p "$INSTALL_DIR"
        else
            mkdir -p "$INSTALL_DIR"
        fi
    fi
    
    # check permissions only for system directories
    if [[ "$INSTALL_DIR" == "/usr/local/bin" ]] || [[ "$INSTALL_DIR" == "/usr/bin" ]]; then
        check_permissions
    fi
    
    # detect system and download binary
    detect_os_arch
    
    if [[ -n "$SPECIFIC_VERSION" ]]; then
        LATEST_RELEASE="$SPECIFIC_VERSION"
        print_status "using specified version: $LATEST_RELEASE"
    else
        get_latest_release
    fi
    
    download_binary
    install_binary
    verify_installation
    cleanup
    
    print_status "done! Don't forget to configure environment variables:"
    print_status "  - OPENAI_API_KEY or ANTHROPIC_API_KEY"
}

trap cleanup EXIT

main "$@" 