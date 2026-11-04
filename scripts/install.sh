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
DB_PATH="/var/lib/goapache/goapache.db"

APACHE_CONF="/etc/apache2/sites-available/goapache.conf"
HELPER_SOCKET="/run/goapache/helper.sock"

PANEL_DOMAIN="${PANEL_DOMAIN:-}"
SKIP_ADMIN="${SKIP_ADMIN:-0}"

log() {
    echo
    echo "[$(date '+%H:%M:%S')] $*"
}

fail() {
    echo
    echo "ERROR: $*" >&2
    exit 1
}

if [[ "$EUID" -ne 0 ]]; then
    fail "Run installer as root: sudo bash scripts/install.sh"
fi

cd "$(dirname "$0")/.."

log "Checking operating system..."

if [[ ! -f /etc/os-release ]]; then
    fail "Cannot detect Linux distribution."
fi

source /etc/os-release

DISTRO_ID="${ID:-}"
DISTRO_LIKE="${ID_LIKE:-}"

case "$DISTRO_ID" in
    ubuntu|debian|linuxmint)
        APACHE_COMMAND="/usr/sbin/apache2ctl"
        ;;
    *)
        case " $DISTRO_LIKE " in
            *" ubuntu "*|*" debian "*)
                APACHE_COMMAND="/usr/sbin/apache2ctl"
                ;;
            *)
                fail "Only Debian, Ubuntu, Linux Mint and their derivatives are currently supported."
                ;;
        esac
        ;;
esac

if [[ ! -x "$APACHE_COMMAND" ]]; then
    fail "Apache is not installed. Install it first: apt install apache2"
fi

for command_name in systemctl curl runuser getent groupadd useradd usermod install; do
    command -v "$command_name" >/dev/null 2>&1 ||
        fail "Required command not found: $command_name"
done

log "Checking application files..."

[[ -f ./goapache ]] ||
    fail "Backend binary ./goapache not found."

[[ -f ./goapache-helper ]] ||
    fail "Helper binary ./goapache-helper not found."

[[ -f ./frontend/dist/index.html ]] ||
    fail "Frontend build missing. Run: cd frontend && npm ci && npm run build"

chmod 0755 ./goapache ./goapache-helper

log "Checking Apache..."

systemctl enable --now apache2

log "Creating system account..."

if ! getent group "$APP_GROUP" >/dev/null 2>&1; then
    groupadd --system "$APP_GROUP"
fi

if ! id "$APP_USER" >/dev/null 2>&1; then
    useradd \
        --system \
        --gid "$APP_GROUP" \
        --home-dir "$DATA_DIR" \
        --no-create-home \
        --shell /usr/sbin/nologin \
        "$APP_USER"
else
    usermod \
        --gid "$APP_GROUP" \
        --home "$DATA_DIR" \
        --shell /usr/sbin/nologin \
        "$APP_USER"
fi

log "Creating application directories..."

install -d -o root -g root -m 0755 "$INSTALL_DIR"
install -d -o root -g root -m 0755 "$CONFIG_DIR"

install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$DATA_DIR/backups"
install -d -o "$APP_USER" -g "$APP_GROUP" -m 0750 "$LOG_DIR"

install -d -o root -g root -m 0755 "$WEB_DIR"

log "Installing application binaries..."

install -o root -g root -m 0755 \
    ./goapache "$INSTALL_DIR/goapache"

install -o root -g root -m 0755 \
    ./goapache-helper "$INSTALL_DIR/goapache-helper"

log "Publishing frontend..."

# Copy new frontend assets without deleting unrelated existing files.
cp -a ./frontend/dist/. "$WEB_DIR/"

find "$WEB_DIR" -type d -exec chmod 0755 {} +
find "$WEB_DIR" -type f -exec chmod 0644 {} +
chown -R root:root "$WEB_DIR"

log "Installing systemd services..."

[[ -f packaging/systemd/goapache.service ]] ||
    fail "Missing packaging/systemd/goapache.service"

[[ -f packaging/systemd/goapache-helper.service ]] ||
    fail "Missing packaging/systemd/goapache-helper.service"

install -o root -g root -m 0644 \
    packaging/systemd/goapache-helper.service \
    /etc/systemd/system/goapache-helper.service

install -o root -g root -m 0644 \
    packaging/systemd/goapache.service \
    /etc/systemd/system/goapache.service

systemd-analyze verify \
    /etc/systemd/system/goapache-helper.service \
    /etc/systemd/system/goapache.service

systemctl daemon-reload

systemctl enable goapache-helper.service goapache.service

log "Starting privileged Helper..."

systemctl restart goapache-helper.service

HELPER_READY=0

for attempt in $(seq 1 30); do
    if [[ -S "$HELPER_SOCKET" ]] &&
       runuser -u "$APP_USER" -- test -r "$HELPER_SOCKET" &&
       runuser -u "$APP_USER" -- test -w "$HELPER_SOCKET"; then
        HELPER_READY=1
        break
    fi

    sleep 1
done

if [[ "$HELPER_READY" -ne 1 ]]; then
    systemctl status goapache-helper.service --no-pager || true
    journalctl -u goapache-helper.service -n 40 --no-pager || true
    fail "Helper did not create an accessible Unix socket."
fi

echo "Helper socket: ready"

log "Starting Backend..."

systemctl restart goapache.service

BACKEND_READY=0

for attempt in $(seq 1 30); do
    if curl -fsS --max-time 3 \
        http://127.0.0.1:9090/api/v1/health >/dev/null 2>&1; then
        BACKEND_READY=1
        break
    fi

    sleep 1
done

if [[ "$BACKEND_READY" -ne 1 ]]; then
    systemctl status goapache.service --no-pager || true
    journalctl -u goapache.service -n 40 --no-pager || true
    fail "Backend health check failed."
fi

echo "Backend API: ready"

log "Configuring Apache modules..."

a2enmod proxy proxy_http >/dev/null

is_valid_domain() {
    [[ "$1" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$ ]]
}

is_local_domain() {
    local domain="${1,,}"

    case "$domain" in
        *.localhost|*.test|*.invalid|*.example|*.local)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

configure_cookie_mode() {
    local mode="$1"
    local dropin_dir="/etc/systemd/system/goapache.service.d"
    local dropin="$dropin_dir/90-goapache-cookie-mode.conf"

    install -d -o root -g root -m 0755 "$dropin_dir"

    if [[ "$mode" == "local" ]]; then
        cat > "$dropin" <<'EOF'
[Service]
Environment="GOAPACHE_INSECURE_COOKIES=1"
EOF
    else
        # Explicitly override older/manual insecure-cookie settings.
        cat > "$dropin" <<'EOF'
[Service]
Environment="GOAPACHE_INSECURE_COOKIES="
EOF
    fi

    chmod 0644 "$dropin"
    chown root:root "$dropin"

    systemctl daemon-reload
    systemctl restart goapache

    for attempt in $(seq 1 15); do
        if curl -fsS --max-time 2 \
            http://127.0.0.1:9090/api/v1/health >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done

    journalctl -u goapache -n 30 --no-pager || true
    fail "Backend did not restart after configuring cookie security."
}

add_local_hostname() {
    local domain="$1"
    local hosts_file="/etc/hosts"

    # Check whether the hostname already exists in /etc/hosts.
    if awk -v domain="$domain" '
        {
            for (i = 2; i <= NF; i++) {
                if ($i ~ /^#/) break
                if ($i == domain) {
                    found = 1
                    if ($1 == "127.0.0.1" || $1 == "::1") {
                        valid = 1
                    }
                }
            }
        }
        END {
            if (found && valid) exit 0
            if (found && !valid) exit 2
            exit 1
        }
    ' "$hosts_file"; then
        return 0
    else
        status=$?
        if [[ "$status" -eq 2 ]]; then
            fail "Domain $domain already exists in /etc/hosts with a non-loopback IP. Please fix it manually."
        fi
    fi

    printf '\n127.0.0.1 %s # GoApache Manager local hostname: %s\n' \
        "$domain" "$domain" >> "$hosts_file"
}

if [[ -z "$PANEL_DOMAIN" && -t 0 ]]; then
    echo
    echo "Enter the domain for the GoApache Manager panel."
    echo "Default: goapache.test (local installation)"
    echo "For a public domain, enter e.g. panel.example.com"
    echo "Enter 'skip' to skip Apache virtual host configuration."
    read -r -p "Panel domain [goapache.test]: " PANEL_DOMAIN
fi

if [[ -z "$PANEL_DOMAIN" ]]; then
    PANEL_DOMAIN="goapache.test"
fi

PANEL_CONFIGURED=0
LOCAL_PANEL=0

case "${PANEL_DOMAIN,,}" in
    skip|no|none)
        log "Skipping panel virtual host configuration."
        configure_cookie_mode "public"
        ;;

    *)
        PANEL_DOMAIN="${PANEL_DOMAIN,,}"

        is_valid_domain "$PANEL_DOMAIN" ||
            fail "Invalid panel domain: $PANEL_DOMAIN"

        [[ "$PANEL_DOMAIN" != "localhost" ]] ||
            fail "Do not use localhost as the panel domain. Use goapache.test instead."

        if is_local_domain "$PANEL_DOMAIN"; then
            LOCAL_PANEL=1
            log "Local domain detected: $PANEL_DOMAIN"
            add_local_hostname "$PANEL_DOMAIN"
            configure_cookie_mode "local"
            echo "Local hostname: configured"
            echo "HTTP cookies: configured for local access"
        else
            LOCAL_PANEL=0
            log "Public domain detected: $PANEL_DOMAIN"
            configure_cookie_mode "public"
            echo "Secure session cookies: enabled"
        fi

        log "Configuring panel virtual host: $PANEL_DOMAIN"

        SITE_WAS_ENABLED=0

        if [[ -L /etc/apache2/sites-enabled/goapache.conf ]]; then
            SITE_WAS_ENABLED=1
        fi

        OLD_CONFIG=""
        if [[ -f "$APACHE_CONF" ]]; then
            OLD_CONFIG="$APACHE_CONF.backup-$(date -u +%Y%m%dT%H%M%S)"
            cp -a "$APACHE_CONF" "$OLD_CONFIG"
            echo "Previous Apache configuration backed up: $OLD_CONFIG"
        fi

        TMP_CONF="$(mktemp /etc/apache2/sites-available/.goapache.XXXXXX)"

        cat > "$TMP_CONF" <<EOF
# GoApache Manager panel
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

    ErrorLog \${APACHE_LOG_DIR}/goapache-error.log
    CustomLog \${APACHE_LOG_DIR}/goapache-access.log combined
</VirtualHost>
EOF

        chmod 0644 "$TMP_CONF"
        chown root:root "$TMP_CONF"
        mv -f "$TMP_CONF" "$APACHE_CONF"

        a2ensite goapache >/dev/null

        if ! "$APACHE_COMMAND" configtest; then
            echo "Apache configuration failed. Restoring previous configuration."

            if [[ -n "$OLD_CONFIG" ]]; then
                cp -a "$OLD_CONFIG" "$APACHE_CONF"
            else
                rm -f "$APACHE_CONF"
            fi

            if [[ "$SITE_WAS_ENABLED" -eq 0 ]]; then
                a2dissite goapache >/dev/null 2>&1 || true
            fi

            "$APACHE_COMMAND" configtest || true
            fail "Apache configuration test failed. Previous configuration was restored."
        fi

        systemctl reload apache2

        PANEL_CONFIGURED=1
        ;;
esac

log "Verifying Apache and frontend..."

"$APACHE_COMMAND" configtest ||
    fail "Apache configuration test failed."

systemctl is-active --quiet apache2 ||
    fail "Apache is not running."

if [[ "$PANEL_CONFIGURED" -eq 1 ]]; then
    FRONTEND_READY=0

    for attempt in $(seq 1 15); do
        if curl -fsS --max-time 3 \
            -H "Host: $PANEL_DOMAIN" \
            http://127.0.0.1/ | grep -qi '<html'; then
            FRONTEND_READY=1
            break
        fi

        sleep 1
    done

    if [[ "$FRONTEND_READY" -ne 1 ]]; then
        journalctl -u apache2 -n 40 --no-pager || true
        fail "Frontend HTTP check failed."
    fi

    echo "Frontend: ready"

    if ! curl -fsS --max-time 5 \
        -H "Host: $PANEL_DOMAIN" \
        http://127.0.0.1/api/v1/health | grep -q '"status":"ok"'; then
        fail "Backend through Apache reverse proxy is not responding correctly."
    fi

    echo "Apache API proxy: ready"
else
    [[ -f "$WEB_DIR/index.html" ]] ||
        fail "Frontend index.html is missing."

    echo "Frontend files: installed"
    echo "Apache: running"
    echo "Panel HTTP test: skipped because no domain was configured."
fi

log "Administrator setup..."

CREATE_ADMIN=0

if [[ "$SKIP_ADMIN" != "1" && -t 0 ]]; then
    echo
    read -r -p "Create the initial administrator now? [Y/n] " answer

    case "${answer,,}" in
        ""|y|yes)
            CREATE_ADMIN=1
            ;;
        *)
            echo "Administrator creation skipped."
            ;;
    esac
fi

ADMIN_CREATED=0

if [[ "$CREATE_ADMIN" -eq 1 ]]; then
    echo
    echo "Starting administrator creation as system user: $APP_USER"
    echo "Use the prompts to enter the administrator credentials."
    echo

    if runuser -u "$APP_USER" -- \
        env GOAPACHE_DB="$DB_PATH" \
        "$INSTALL_DIR/goapache" --create-admin; then
        ADMIN_CREATED=1
        echo
        echo "Administrator creation command completed."
    else
        echo
        echo "WARNING: Administrator creation failed."
        echo "An administrator may already exist, or the operation returned an error."
        echo "You can retry manually using:"
        echo "sudo -u goapache env GOAPACHE_DB=$DB_PATH $INSTALL_DIR/goapache --create-admin"
    fi
fi

log "Final service status..."

systemctl is-active --quiet goapache-helper ||
    fail "Helper service is not active."

systemctl is-active --quiet goapache ||
    fail "Backend service is not active."

systemctl is-active --quiet apache2 ||
    fail "Apache service is not active."

echo
echo "=============================================="
echo "       GoApache Manager installation"
echo "=============================================="
echo
echo "Distribution: ${PRETTY_NAME:-$ID}"
echo "Install dir:  $INSTALL_DIR"
echo "Database:     $DB_PATH"
echo "Frontend:     $WEB_DIR"
echo "Backend:      http://127.0.0.1:9090"
echo "Helper:       $HELPER_SOCKET"
echo
echo "Services:"
echo "  goapache-helper: $(systemctl is-active goapache-helper)"
echo "  goapache:        $(systemctl is-active goapache)"
echo "  apache2:         $(systemctl is-active apache2)"
echo

if [[ "$ADMIN_CREATED" -eq 1 ]]; then
    echo "Administrator: created"
else
    echo "Administrator: not created by this installation."
    echo "Create it with:"
    echo "sudo -u goapache env GOAPACHE_DB=$DB_PATH $INSTALL_DIR/goapache --create-admin"
fi

echo

if [[ "$PANEL_CONFIGURED" -eq 1 ]]; then
    if [[ "$LOCAL_PANEL" -eq 1 ]]; then
        echo "Panel URL: http://$PANEL_DOMAIN"
        echo
        echo "Local hostname configured in /etc/hosts."
        echo "Insecure cookies enabled for local HTTP access."
        echo "Open this URL in your browser on this machine."
    else
        echo "Panel URL: http://$PANEL_DOMAIN"
        echo
        echo "IMPORTANT: Configure HTTPS before logging in."
        echo "Secure session cookies are enabled."
        echo
        echo "After pointing DNS to this server, install TLS with:"
        echo "sudo apt install -y certbot python3-certbot-apache"
        echo "sudo certbot --apache -d $PANEL_DOMAIN"
        echo
        echo "After TLS setup, open: https://$PANEL_DOMAIN"
    fi
else
    echo "Panel virtual host: not configured."
    echo "Re-run with:"
    echo "sudo PANEL_DOMAIN=panel.example.com bash scripts/install.sh"
fi

echo
echo "Service logs:"
echo "journalctl -u goapache -f"
echo "journalctl -u goapache-helper -f"
echo
echo "Installation completed."
