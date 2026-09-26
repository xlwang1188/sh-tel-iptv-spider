#!/bin/sh
set -e

# One-Click Installer for rtp2httpd on Linux (Alpine / Debian / Ubuntu / CentOS)
# rtp2httpd 一键安装与服务部署脚本

VERSION=${1:-"3.11.0"}
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64)
        BIN_ARCH="x86_64"
        ;;
    aarch64|arm64)
        BIN_ARCH="aarch64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "=========================================================="
echo " Installing rtp2httpd v${VERSION} for ${BIN_ARCH} ..."
echo "=========================================================="

# 1. Download binary from official GitHub release
DOWNLOAD_URL="https://github.com/stackia/rtp2httpd/releases/download/v${VERSION}/rtp2httpd-${VERSION}-${BIN_ARCH}"
TMP_BIN="/tmp/rtp2httpd"

echo "Downloading from: $DOWNLOAD_URL ..."
if command -v curl >/dev/null 2>&1; then
    curl -L "$DOWNLOAD_URL" -o "$TMP_BIN"
elif command -v wget >/dev/null 2>&1; then
    wget -O "$TMP_BIN" "$DOWNLOAD_URL"
else
    echo "Error: Neither curl nor wget is installed."
    exit 1
fi

chmod +x "$TMP_BIN"
sudo mv "$TMP_BIN" /usr/local/bin/rtp2httpd
echo "Binary installed to /usr/local/bin/rtp2httpd"

# 2. Setup configuration directory
sudo mkdir -p /etc/rtp2httpd
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
if [ -f "$SCRIPT_DIR/rtp2httpd.conf" ]; then
    sudo cp "$SCRIPT_DIR/rtp2httpd.conf" /etc/rtp2httpd/rtp2httpd.conf
else
    echo "Warning: rtp2httpd.conf not found in script directory. Please configure /etc/rtp2httpd/rtp2httpd.conf manually."
fi

# 3. Kernel tuning for high-bitrate 4K IPTV streams
echo "Tuning kernel socket buffers (net.core.rmem_max) ..."
sudo sysctl -w net.core.rmem_max=4194304 2>/dev/null || true
sudo sysctl -w net.core.wmem_max=4194304 2>/dev/null || true

# 4. Service installation (OpenRC or systemd)
if [ -d "/etc/runlevels" ]; then
    # Alpine Linux OpenRC
    echo "Configuring OpenRC service on Alpine Linux ..."
    if [ -f "$SCRIPT_DIR/rtp2httpd.initd" ]; then
        sudo cp "$SCRIPT_DIR/rtp2httpd.initd" /etc/init.d/rtp2httpd
        sudo chmod +x /etc/init.d/rtp2httpd
        sudo rc-update add rtp2httpd default
        sudo rc-service rtp2httpd restart || true
    fi
elif command -v systemctl >/dev/null 2>&1; then
    # systemd (Ubuntu, Debian, CentOS)
    echo "Configuring systemd service ..."
    if [ -f "$SCRIPT_DIR/rtp2httpd.service" ]; then
        sudo cp "$SCRIPT_DIR/rtp2httpd.service" /etc/systemd/system/rtp2httpd.service
        sudo systemctl daemon-reload
        sudo systemctl enable rtp2httpd
        sudo systemctl restart rtp2httpd || true
    fi
fi

echo "=========================================================="
echo " rtp2httpd installation completed successfully!"
echo " Listening port: 5140"
echo " Web status UI : http://<YOUR_IP>:5140/status"
echo " Web player UI : http://<YOUR_IP>:5140/player"
echo "=========================================================="
