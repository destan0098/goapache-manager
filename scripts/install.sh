#!/usr/bin/env bash

set -Eeuo pipefail

APP="goapache"
APP_USER="goapache"
APP_GROUP="goapache"

INSTALL_DIR="/opt/goapache"
DATA_DIR="/var/lib/goapache"
CONFIG_DIR="/etc/goapache"
LOG_DIR="/var/log/goapache"

if [[ $EUID -ne 0 ]]; then
    echo "Please run as root: sudo bash install.sh"
    exit 1
fi

if [[ ! -f "./goapache" ]]; then
    echo "Binary ./goapache not found."
    echo "Build the project first."
    exit 1
fi

echo "[1/7] Detecting operating system..."

if [[ ! -f /etc/os-release ]]; then
    echo "Unsupported Linux distribution."
    exit 1
fi

source /etc/os-release

case "${ID:-}" in
    ubuntu|debian)
        DISTRO="debian"
        APACHE_CONFIG="/etc/apache2"
        APACHE_COMMAND="/usr/sbin/apache2ctl"
        ;;
    rocky|almalinux|centos|rhel|fedora)
        DISTRO="rhel"
        APACHE_CONFIG="/etc/httpd"
        APACHE_COMMAND="/usr/sbin/apachectl"
        ;;
    *)
        echo "Unsupported distribution: ${ID:-unknown}"
        exit 1
        ;;
esac

if ! command -v "$APACHE_COMMAND" >/dev/null 2>&1; then
    echo "Apache is not installed."
    echo "Please install Apache before GoApache Manager."
    exit 1
fi

echo "[2/7] Creating service account..."

if ! getent group "$APP_GROUP" >/dev/null; then
    groupadd --system "$APP_GROUP"
fi

if ! id "$APP_USER" >/dev/null 2>&1; then
    useradd \
        --system \
        --gid "$APP_GROUP" \
        --home-dir "$DATA_DIR" \
        --shell /usr/sbin/nologin \
        "$APP_USER"
fi

echo "[3/7] Creating directories..."

install -d -o root -g root -m 0755 "$INSTALL_DIR"
install -d -o root -g root -m 0755 "$CONFIG_DIR"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR/backups"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$LOG_DIR"

echo "[4/7] Installing binary..."

install -o root -g root -m 0755 \
    ./goapache "$INSTALL_DIR/goapache"

if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
    cat > "$CONFIG_DIR/config.yaml" <<EOF
server:
  host: "127.0.0.1"
  port: 9090

database:
  path: "$DATA_DIR/goapache.db"

apache:
  config_path: "$APACHE_CONFIG"
  command: "$APACHE_COMMAND"

security:
  secure_cookie: true
EOF
    chmod 0640 "$CONFIG_DIR/config.yaml"
    chown root:"$APP_GROUP" "$CONFIG_DIR/config.yaml"
fi

echo "[5/7] Installing systemd service..."

install -o root -g root -m 0644 \
    ./packaging/systemd/goapache.service \
    /etc/systemd/system/goapache.service

echo "[6/7] Reloading systemd..."

systemctl daemon-reload
systemctl enable goapache.service

echo "[7/7] Starting service..."

systemctl restart goapache.service

echo
echo "======================================"
echo " GoApache Manager installed"
echo "======================================"
echo "Distribution: $DISTRO"
echo "Apache config: $APACHE_CONFIG"
echo "Port: 9090"
echo "Service: goapache"
echo
echo "Check status:"
echo "systemctl status goapache"
echo
echo "Application:"
echo "http://127.0.0.1:9090"