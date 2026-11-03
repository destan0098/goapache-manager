#!/usr/bin/env bash

set -Eeuo pipefail

APP="goapache"
APP_USER="goapache"
APP_GROUP="goapache"

INSTALL_DIR="/opt/goapache"
DATA_DIR="/var/lib/goapache"
CONFIG_DIR="/etc/goapache"
LOG_DIR="/var/log/goapache"
WEB_DIR="/var/www/goapache"
APACHE_CONF="/etc/apache2/sites-available/goapache.conf"

# Optional non-interactive configuration:
#   PANEL_DOMAIN=panel.example.com  — create/refresh the panel vhost
#   PANEL_DOMAIN=skip               — do not touch Apache vhosts
#   SKIP_ADMIN=1                    — do not offer to create the admin user
PANEL_DOMAIN="${PANEL_DOMAIN:-}"
SKIP_ADMIN="${SKIP_ADMIN:-0}"

if [[ $EUID -ne 0 ]]; then
    echo "Please run as root: sudo bash scripts/install.sh"
    exit 1
fi

cd "$(dirname "$0")/.."

if [[ ! -f "./goapache" ]]; then
    echo "Binary ./goapache not found."
    echo "Build it first:  go build -o goapache ./cmd/server"
    exit 1
fi

if [[ ! -f "./goapache-helper" ]]; then
    echo "Binary ./goapache-helper not found."
    echo "Build it first:  go build -o goapache-helper ./cmd/helper"
    exit 1
fi

if [[ ! -f "./frontend/dist/index.html" ]]; then
    echo "Frontend build not found at ./frontend/dist."
    echo "Build it first:  cd frontend && npm ci && npm run build"
    exit 1
fi

echo "[1/9] Detecting operating system..."

if [[ ! -f /etc/os-release ]]; then
    echo "Unsupported Linux distribution: /etc/os-release not found."
    exit 1
fi

source /etc/os-release

case "${ID:-}" in
    ubuntu|debian)
        DISTRO="debian"
        APACHE_CONFIG="/etc/apache2"
        APACHE_COMMAND="/usr/sbin/apache2ctl"
        ;;
    *)
        echo "Unsupported distribution: ${ID:-unknown}"
        echo "GoApache Manager currently supports Debian/Ubuntu with the /etc/apache2 layout."
        exit 1
        ;;
esac

if ! command -v "$APACHE_COMMAND" >/dev/null 2>&1; then
    echo "Apache is not installed."
    echo "Install it first:  sudo apt install -y apache2"
    exit 1
fi

echo "[2/9] Creating service account..."

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

echo "[3/9] Creating directories..."

install -d -o root -g root -m 0755 "$INSTALL_DIR"
install -d -o root -g root -m 0755 "$CONFIG_DIR"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR/backups"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$LOG_DIR"
install -d -o root -g root -m 0755 "$WEB_DIR"

echo "[4/9] Installing binaries..."

install -o root -g root -m 0755 \
    ./goapache "$INSTALL_DIR/goapache"

install -o root -g root -m 0755 \
    ./goapache-helper "$INSTALL_DIR/goapache-helper"

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

echo "[5/9] Installing systemd services..."

install -o root -g root -m 0644 \
    ./packaging/systemd/goapache.service \
    /etc/systemd/system/goapache.service

install -o root -g root -m 0644 \
    ./packaging/systemd/goapache-helper.service \
    /etc/systemd/system/goapache-helper.service

systemctl daemon-reload
systemctl enable goapache.service goapache-helper.service

echo "[6/9] Publishing frontend..."

rm -rf "${WEB_DIR:?}/"*
cp -a ./frontend/dist/. "$WEB_DIR/"
find "$WEB_DIR" -type d -exec chmod 0755 {} +
find "$WEB_DIR" -type f -exec chmod 0644 {} +
chown -R root:root "$WEB_DIR"

echo "[7/9] Configuring Apache for the panel..."

a2enmod proxy proxy_http >/dev/null

is_valid_domain() {
    [[ "$1" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$ ]]
}

if [[ -z "$PANEL_DOMAIN" && -t 0 ]]; then
    echo
    echo "Enter the domain or server name for the panel, e.g. panel.example.com"
    echo "(leave empty to skip; you can configure the vhost manually later)."
    read -r -p "Panel domain: " PANEL_DOMAIN
fi

case "${PANEL_DOMAIN,,}" in
    ""|skip|no|none)
        echo "Skipping panel vhost creation."
        PANEL_CONFIGURED=0
        ;;
    *)
        if ! is_valid_domain "$PANEL_DOMAIN"; then
            echo "Invalid panel domain: $PANEL_DOMAIN"
            exit 1
        fi

        if [[ -f "$APACHE_CONF" ]]; then
            cp -a "$APACHE_CONF" "$APACHE_CONF.bak-$(date -u +%Y%m%dT%H%M%S)"
        fi

        cat > "$APACHE_CONF" <<EOF
# GoApache Manager panel (serves the UI and reverse-proxies /api).
<VirtualHost *:80>
    ServerName $PANEL_DOMAIN

    DocumentRoot $WEB_DIR

    <Directory $WEB_DIR>
        Options FollowSymLinks
        AllowOverride None
        Require all granted
        FallbackResource /index.html
    </Directory>

    ProxyPass        /api http://127.0.0.1:9090/api
    ProxyPassReverse /api http://127.0.0.1:9090/api

    ErrorLog  \${APACHE_LOG_DIR}/goapache-error.log
    CustomLog \${APACHE_LOG_DIR}/goapache-access.log combined
</VirtualHost>
EOF
        chmod 0644 "$APACHE_CONF"
        chown root:root "$APACHE_CONF"

        a2ensite goapache >/dev/null

        if ! "$APACHE_COMMAND" configtest >/dev/null 2>&1; then
            echo "Apache configtest failed; disabling the panel vhost."
            a2dissite goapache >/dev/null || true
            rm -f /etc/apache2/sites-enabled/goapache.conf
            exit 1
        fi

        systemctl reload apache2
        PANEL_CONFIGURED=1
        ;;
esac

echo "[8/9] Starting services..."

systemctl restart goapache-helper.service
systemctl restart goapache.service

echo "[9/9] Verifying installation..."

if command -v curl >/dev/null 2>&1; then
    if curl -fsS --max-time 5 http://127.0.0.1:9090/api/v1/health >/dev/null 2>&1; then
        echo "Backend health check: ok"
    else
        echo "WARNING: backend is not responding on 127.0.0.1:9090."
        echo "Check:  systemctl status goapache && journalctl -u goapache -n 50"
    fi
else
    echo "curl not installed; skipping health check."
fi

if [[ -S /run/goapache/helper.sock ]]; then
    echo "Helper socket: /run/goapache/helper.sock"
else
    echo "WARNING: helper socket not found; check: systemctl status goapache-helper"
fi

CREATE_ADMIN=0
if [[ "$SKIP_ADMIN" != "1" && -t 0 ]] && command -v runuser >/dev/null 2>&1; then
    echo
    read -r -p "Create the administrator account now? [Y/n] " answer
    case "${answer,,}" in
        ""|y|yes) CREATE_ADMIN=1 ;;
        *) echo "Skipped. Create it later with:" ;;
    esac
fi

if [[ "$CREATE_ADMIN" == "1" ]]; then
    runuser -u "$APP_USER" -- "$INSTALL_DIR/goapache" --create-admin || {
        echo "Admin creation failed or an administrator already exists."
        echo "You can retry later with:"
        echo "  sudo -u $APP_USER $INSTALL_DIR/goapache --create-admin"
    }
fi

echo
echo "======================================"
echo " GoApache Manager installed"
echo "======================================"
echo "Distribution:   ${PRETTY_NAME:-$DISTRO}"
echo "Apache config:  $APACHE_CONFIG"
echo "Backend:        127.0.0.1:9090 (loopback only)"
echo "Panel files:    $WEB_DIR"
echo
echo "Create the administrator account (first install only):"
echo "  sudo -u $APP_USER $INSTALL_DIR/goapache --create-admin"
echo

if [[ "${PANEL_CONFIGURED:-0}" == "1" ]]; then
    echo "Panel vhost:    $APACHE_CONF (ServerName $PANEL_DOMAIN)"
    echo
    echo "HTTPS is REQUIRED for login (session cookies are Secure)."
    echo "Point DNS A/AAAA for $PANEL_DOMAIN at this server, then run:"
    echo "  sudo apt install -y certbot python3-certbot-apache"
    echo "  sudo certbot --apache -d $PANEL_DOMAIN"
    echo
    echo "Or, without a domain, use an SSH tunnel from your workstation:"
    echo "  ssh -L 8080:127.0.0.1:80 user@<server>   # open http://localhost:8080"
else
    echo "Panel vhost:    NOT configured (skipped)."
    echo "To serve the panel, create /etc/apache2/sites-available/goapache.conf"
    echo "with DocumentRoot $WEB_DIR and ProxyPass /api http://127.0.0.1:9090/api,"
    echo "or re-run:  sudo PANEL_DOMAIN=panel.example.com bash scripts/install.sh"
fi
echo
echo "Services:  systemctl status goapache goapache-helper"
