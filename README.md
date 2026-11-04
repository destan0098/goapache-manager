# GoApache Manager

**Manage Apache VirtualHosts from a secure web dashboard — without manually editing configuration files or running Apache commands.**

GoApache Manager provides a web interface for managing Apache VirtualHosts on Linux servers. It is designed to simplify common Apache administration tasks while keeping privileged operations isolated in a dedicated root helper process that talks to the web app over a locked-down Unix socket.

The panel itself is served through your existing Apache installation (static files + reverse proxy), so no new ports need to be exposed to the internet. The app listens on `127.0.0.1:9090` and Apache proxies `/api` to it.

## Screenshot

![GoApache Manager](screenshot.png "GoApache Manager")

## Features

- **VirtualHost Management**
    - List all Apache VirtualHosts (managed and external).
    - Create managed VirtualHosts (config, webroot, enable + reload in one step).
    - Edit DocumentRoot (validated to live under `/var/www`) and ServerAliases.
    - Enable and disable VirtualHosts.
    - Delete managed VirtualHosts (DocumentRoot is preserved).
    - Backups are created automatically before edits and restores.

- **Apache Service Management**
    - Check Apache service status.
    - Run `apache2ctl -t` (configtest) from the UI.
    - Reload Apache.
    - Restart Apache.

- **Backup Management**
    - Automatic backups before every edit/restore/delete.
    - List, restore and delete backups per domain.

- **Security**
    - Username/password login with bcrypt password hashing.
    - Session cookies: `HttpOnly` + `Secure` + `SameSite=Strict` (1 hour lifetime).
    - CSRF protection on all state-changing requests.
    - Privileged operations are performed by a separate root helper, not the web app.
    - Helper accepts connections **only** from the `goapache` system UID (verified via `SO_PEERCRED`) over a Unix socket — it is never exposed to the network.
    - Managed configs carry an ownership marker; external configs are read-only.
    - DocumentRoot validation with symlink rejection.
    - `apache2ctl -t` is run before applying changes; automatic rollback on failure.

## How It Works

```text
                 ┌──────────────────────────────────┐
 Browser ──────► │ Apache (static + reverse proxy)  │
  HTTPS :443     │   /            → /var/www/goapache (React build)
                 │   /api/*       → 127.0.0.1:9090
                 └───────────────┬──────────────────┘
                                 │  (loopback only)
                 ┌───────────────▼──────────────────┐
                 │ goapache (web backend, :9090)    │
                 │ - auth, sessions, CSRF           │
                 │ - serves JSON API only           │
                 └───────────────┬──────────────────┘
                                 │  Unix socket: /run/goapache/helper.sock
                                 │  (peer credentials checked: UID of goapache)
                 ┌───────────────▼──────────────────┐
                 │ goapache-helper (root)           │
                 │ - writes /etc/apache2 configs    │
                 │ - runs apache2ctl / systemctl    │
                 │ - creates backups + rollbacks    │
                 └──────────────────────────────────┘
```

Three components are installed on the server:

| Component | Binary | Runs as | Purpose |
|---|---|---|---|
| Web backend | `/opt/goapache/goapache` | `goapache` (system user) | JSON API, auth, sessions |
| Privileged helper | `/opt/goapache/goapache-helper` | `root` | Apache config changes, service control |
| Panel frontend | `/var/www/goapache` (static files) | served by Apache | React single-page app |

## Requirements

- **Debian or Ubuntu** (the Apache layout under `/etc/apache2` is required; other distributions are not supported yet)
- **Apache HTTP Server 2.4** (`sudo apt install apache2`)
- **Go 1.26** or later — <https://go.dev/dl/>
- **Node.js 22** or later and npm — <https://nodejs.org/>
- **Git**, and a C compiler (needed to build the SQLite driver via cgo): `sudo apt install -y git build-essential`
- Root / sudo access on the server
- A domain name with a DNS A/AAAA record pointing to the server, **recommended** (required for the default HTTPS setup — see the note below)

> **HTTPS is required.** Login cookies are `Secure`, so browsers refuse to store them over plain HTTP (except on `localhost`). If your panel is reachable over `http://` from anywhere but localhost, you will not be able to log in. You can either [enable HTTPS with certbot](#step-8--enable-https-required) or [reach the panel through an SSH tunnel](#access-without-a-domain-ssh-tunnel).

## Installation

Tested on Ubuntu 24.04 / Debian 12 with Apache 2.4. All commands are run on the target server unless noted.

### Step 0 — Install prerequisites

```bash
sudo apt update
sudo apt install -y apache2 git build-essential curl ca-certificates
# Install Go >= 1.26 from https://go.dev/dl/ and Node.js >= 22 from https://nodejs.org/
go version      # must report go1.26 or higher
node --version  # must report v22 or higher
apache2ctl -v
```

Make sure Apache itself is working:

```bash
sudo apache2ctl configtest
sudo systemctl status apache2
```

### Step 1 — Clone the repository

```bash
git clone https://github.com/destan0098/goapache-manager.git
cd goapache-manager
```

### Step 2 — Build the backend and the helper

The installer expects both binaries in the repository root, so build them explicitly (a plain `go build ./...` does not emit them):

```bash
go build -o goapache ./cmd/server
go build -o goapache-helper ./cmd/helper
```

### Step 3 — Build the frontend

```bash
cd frontend
npm ci
npm run build
cd ..
```

This produces `frontend/dist/` — a static site (React SPA + assets).

### Step 4 — Run the installer (everything in one go)

From anywhere (the script locates the repository root itself):

```bash
sudo bash scripts/install.sh
```

It is safe to re-run — it reinstalls the binaries, republishes the frontend and restarts the services.

What it does:

1. Verifies the OS is Debian/Ubuntu and Apache is installed (the helper requires the `/etc/apache2` layout; other distributions are rejected with a clear error).
2. Creates the `goapache` system group/user (nologin shell).
3. Creates `/opt/goapache`, `/etc/goapache`, `/var/lib/goapache` (0750, owned by `goapache`), `/var/lib/goapache/backups`, `/var/log/goapache`, `/var/www/goapache`.
4. Installs `goapache` and `goapache-helper` to `/opt/goapache/`.
5. Writes `/etc/goapache/config.yaml` if missing (reference/future use — see [Configuration](#configuration)).
6. Installs and enables both systemd units (`goapache.service`, `goapache-helper.service`) and (re)starts them.
7. Publishes the frontend build to `/var/www/goapache` with safe permissions and enables Apache `proxy`/`proxy_http`.
8. Creates the panel VirtualHost on port 80 — interactively, or from the `PANEL_DOMAIN` variable; a pre-existing `goapache.conf` is backed up first, and Apache is reloaded only after a successful `apache2ctl configtest` (on failure the site is removed again).
9. Verifies the installation (API health check, helper socket) and offers to create the administrator account.

Non-interactive install (automation):

```bash
sudo PANEL_DOMAIN=panel.example.com bash scripts/install.sh   # create the panel vhost too
sudo PANEL_DOMAIN=skip SKIP_ADMIN=1 bash scripts/install.sh    # binaries + services only
```

During an interactive run you are asked for:

- the **panel domain** (empty = skip; Apache vhost not touched),
- whether to **create the administrator account** now.

Verify the installation:

```bash
systemctl status goapache goapache-helper
curl -s http://127.0.0.1:9090/api/v1/health   # {"status":"ok"}
ls -l /run/goapache/helper.sock               # srw-rw---- root goapache
```

Notes:

- The panel VirtualHost is **not** managed by GoApache Manager (no ownership marker) and is never listed as editable/deletable in the UI — that is intentional.
- To configure the panel later, re-run `sudo PANEL_DOMAIN=panel.example.com bash scripts/install.sh`, or create the vhost manually as described below.
- There are no default credentials. If you skipped creating the admin during install, run:
  ```bash
  sudo -u goapache /opt/goapache/goapache --create-admin
  ```
  (as the `goapache` user, so the database keeps the right ownership; you will be prompted for a 3–64 character username and a strong password — bcrypt input limit is 72 bytes. A root-created database would not be writable by the service.)

Manually, the steps above are equivalent to installing `packaging/systemd/goapache.service` and `goapache-helper.service`, copying `frontend/dist/` to `/var/www/goapache`, enabling `proxy`/`proxy_http`, and writing:

```apache
<VirtualHost *:80>
    ServerName panel.example.com

    DocumentRoot /var/www/goapache

    <Directory /var/www/goapache>
        Options FollowSymLinks
        AllowOverride None
        Require all granted
        FallbackResource /index.html
    </Directory>

    ProxyPass        /api http://127.0.0.1:9090/api
    ProxyPassReverse /api http://127.0.0.1:9090/api

    ErrorLog  ${APACHE_LOG_DIR}/goapache-error.log
    CustomLog ${APACHE_LOG_DIR}/goapache-access.log combined
</VirtualHost>
```

`FallbackResource` makes client-side routes work (e.g. refreshing on a sub-page). If Apache's default site answers instead, give the panel vhost a unique `ServerName`, or disable the default: `sudo a2dissite 000-default && sudo systemctl reload apache2`.

### Step 5 — Enable HTTPS (required)

Browsers reject the panel's `Secure` session cookie over plain HTTP, so a certificate is effectively mandatory for any non-localhost access. With a domain and DNS in place:

```bash
sudo apt install -y certbot python3-certbot-apache
sudo certbot --apache -d panel.example.com
```

Certbot installs the certificate, rewrites the VirtualHost to listen on 443, and sets up auto-renewal. Then open:

```text
https://panel.example.com
```

and log in with the administrator account created in Step 4.

#### Access without a domain (SSH tunnel)

If you have no domain, keep the port-80 panel VirtualHost and tunnel it. Browsers treat `http://localhost` as a secure context, so the `Secure` cookies work:

```bash
ssh -L 8080:127.0.0.1:80 user@your-server
# then open http://localhost:8080 in your local browser
```

### Step 6 — First login

1. Open `https://panel.example.com` (or `http://localhost:8080` via tunnel).
2. Log in with the administrator account created during Step 4.
3. Sessions last 1 hour; you will be asked to log in again afterwards.

## Using the Panel

The UI has three pages (left sidebar):

### Dashboard
- Live Apache status (`systemctl is-active apache2`).
- Apache controls: run **configtest**, **reload**, **restart** — useful after manual changes.
- "System status" reflects the last result; actions report Apache's own output.

### Virtual Hosts
- **Create**: enter a domain (e.g. `shop.example.com`). The panel will:
    1. create `/var/www/<domain>`,
    2. write `/etc/apache2/sites-available/<domain>.conf` (port `*:80`),
    3. enable it via symlink in `sites-enabled/`,
    4. run `apache2ctl -t` and reload Apache (automatic rollback on failure).
- **Edit**: change DocumentRoot (must be an existing, real directory inside `/var/www`, no symlinks) and ServerAliases. A backup is taken automatically before the change.
- **Enable / Disable**: toggles the `sites-enabled` symlink (managed configs only).
- **Delete**: removes the symlink and moves the config to `<domain>.conf.deleted-<timestamp>`. The DocumentRoot is **not** deleted.
- **External vs managed**: vhosts created by hand only appear in the list; they are marked **External** in the UI and cannot be edited from it. GoApache only modifies configs that carry the first-line marker `# Managed by GoApache Manager: <domain>`.

### Backups
- Per domain, lists backups taken before edits/restores/deletes (`.bak-*`, `.restore-*`, `.deleted-*`).
- **Restore** replaces the current config (a safety backup of the current file is written first), preserves the enabled/disabled state, and re-validates + reloads Apache.
- **Delete** removes a backup file permanently.

## Configuration

| Setting | Where | Notes |
|---|---|---|
| Backend listen address | hardcoded `127.0.0.1:9090` | Not configurable in the current release; keep it loopback-only and proxy via Apache |
| Database path | env `GOAPACHE_DB` | Defaults to `/var/lib/goapache/goapache.db` |
| Database pragmas | built-in | WAL mode, foreign keys on, 5 s busy timeout |
| Session lifetime | built-in | 1 hour; cookies `HttpOnly`/`Secure`/`SameSite=Strict` |
| Helper socket | hardcoded `/run/goapache/helper.sock` | Mode `0660`, owner `root:goapache` |
| `/etc/goapache/config.yaml` | written by installer | **Not read at runtime** by the current release; kept for future versions |
| Panel DocumentRoot | you choose (`/var/www/goapache` shown) | Redeploy by re-copying `frontend/dist/` |

The systemd units are hardened: the backend runs as `goapache` with `ProtectSystem=strict` (writable only `/var/lib/goapache` and `/var/log/goapache`) and `NoNewPrivileges`; the helper runs as root with a private `RuntimeDirectory=goapache`.

## Security Considerations

Before deploying:

- **Always use HTTPS.** Session cookies are `Secure`; plain HTTP will silently break login.
- **Never expose port 9090.** The backend binds to loopback by design — do not add firewall rules or reverse proxies that publish it, and do not change the bind address.
- **The helper is root.** It only accepts connections from the `goapache` UID on a `0660 root:goapache` socket and executes a fixed allow-list of operations. Still, run it only on trusted servers and keep the host patched.
- Restrict access to the panel (firewall, VPN, or an allow-list via Apache `Require ip`) if it manages production sites.
- Backups of Apache configs are stored next to the live configs in `/etc/apache2/sites-available/` (`.bak-*` written `0600`) — protect that directory.
- Rotate the admin password periodically; there is currently no user-management UI, so create admins with `--create-admin` only when needed.
- Keep Go, Node.js and Apache updated; review firewall rules; apply the principle of least privilege everywhere.
- Test configuration changes in a safe environment before production deployment.

This software is provided as-is. You are responsible for securing your deployment and reviewing the code before granting administrative access.

## Upgrading

```bash
cd goapache-manager
git pull

# Rebuild backend + helper
go build -o goapache ./cmd/server
go build -o goapache-helper ./cmd/helper

# Rebuild frontend
cd frontend && npm ci && npm run build && cd ..

# Reinstall binaries + frontend, restart services (config & vhost preserved)
sudo PANEL_DOMAIN=skip SKIP_ADMIN=1 bash scripts/install.sh
```

Existing configuration, database and backups in `/var/lib/goapache` are preserved. The installer does not overwrite an existing `/etc/goapache/config.yaml`, and `PANEL_DOMAIN=skip` leaves the panel vhost untouched.

## Uninstalling

```bash
sudo systemctl disable --now goapache goapache-helper
sudo rm -f /etc/systemd/system/goapache.service /etc/systemd/system/goapache-helper.service
sudo systemctl daemon-reload

sudo rm -rf /opt/goapache /var/www/goapache /etc/goapache
sudo rm -f /etc/apache2/sites-available/goapache.conf /etc/apache2/sites-enabled/goapache.conf
sudo systemctl reload apache2

# Optional — this deletes the database and backups:
sudo rm -rf /var/lib/goapache /var/log/goapache
sudo userdel goapache && sudo groupdel goapache
```

VirtualHosts previously created by GoApache Manager are left in place; remove them manually with `sudo a2dissite <domain> && sudo rm /etc/apache2/sites-available/<domain>.conf` if desired.

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `502 Bad Gateway` on `/api/*` | Backend down — `systemctl status goapache`, `journalctl -u goapache -n 50` |
| Health check OK but login fails with 401 | Admin was never created (Step 4), or wrong credentials |
| Login "succeeds" but you are bounced back / every API call returns 401 | You are on plain HTTP — cookies are `Secure`. Use HTTPS or the SSH tunnel |
| 403 "CSRF validation failed" | Stale/missing CSRF cookie — hard-refresh and log in again |
| Apache actions fail with helper connection refused / socket missing | Helper not running — `systemctl status goapache-helper`, `journalctl -u goapache-helper` (first start creates `/run/goapache/helper.sock`) |
| Helper exits with "goapache system user not found" | Installer was not run or user was deleted — re-run Step 4 / recreate the user |
| "apache configtest failed" in UI | Inspect: `sudo apache2ctl -t`; fix the reported file |
| DocumentRoot rejected ("must be inside /var/www" / "unsafe directory") | The edit path must exist, be a real directory (no symlinks) under `/var/www` |
| Panel shows vhosts but they can't be edited | They are external (no GoApache marker) — by design |
| `database is locked` in journal | Another process holds the SQLite DB — stop duplicate backend instances |

## FAQ

**Does it work on CentOS/RHEL/AlmaLinux/Rocky?**
Not yet. Vhost operations require the Debian/Ubuntu Apache layout (`/etc/apache2` with `sites-available`/`sites-enabled`). The helper checks the layout at runtime and refuses otherwise.

**Can I change the panel port from 9090?**
Not in the current release. It binds to loopback; reach it through your Apache VirtualHost (Step 4) and don't publish 9090.

**Can I manage existing, hand-written vhosts?**
No. GoApache only modifies files carrying its ownership marker. Existing sites are listed for visibility but marked external and are never changed.

**Where are my backups?**
Next to the live config: `/etc/apache2/sites-available/<domain>.conf.bak-<timestamp>` (edits), `.restore-<timestamp>` (pre-restore safety copies), `.deleted-<timestamp>` (from deletions). The empty `/var/lib/goapache/backups` directory created by the installer is reserved for future use.

**Who can log in / are there roles?**
Only the admin created in Step 4. The database schema has `admin`/`operator`/`viewer` roles, but the current API does not enforce per-role permissions yet — treat every authenticated user as an admin.

**Does it create SSL vhosts (Let's Encrypt) for managed sites?**
Not yet; managed vhosts are port 80 only. Use certbot on the created site afterwards, or extend the config manually.

## Technology Stack

| Component | Technology |
|---|---|
| Backend | Go (net/http + chi) |
| Frontend | React, TypeScript, Vite |
| UI Icons | Lucide |
| Database | SQLite (WAL mode) |
| Auth | bcrypt + server-side sessions |
| Web Server | Apache HTTP Server |
| Privileged Operations | Go helper + Unix socket (`SO_PEERCRED`) |

## Project Structure

```text
goapache-manager/
├── cmd/
│   ├── server/          # web backend (API, auth) — binary: goapache
│   └── helper/          # privileged root helper — binary: goapache-helper
├── internal/
│   ├── api/             # chi router, handlers, CSRF middleware
│   ├── apache/          # vhost parser (apache2ctl -S output)
│   ├── auth/            # bcrypt, sessions, middleware
│   ├── database/        # SQLite open + migrations
│   └── helper/          # socket client + protocol types
├── frontend/            # React + TypeScript + Vite SPA
├── packaging/systemd/    # goapache.service, goapache-helper.service
├── scripts/install.sh   # full installer (user, dirs, binaries, systemd, frontend, panel vhost, admin)
├── configs/             # example config (not read at runtime yet)
├── screenshot.png       # UI screenshot used in this README
└── README.md
```

## Roadmap

- [x] Installer covers helper, admin creation and frontend publishing end-to-end
- [ ] Configurable listen address/port and config file loading
- [ ] HTTPS (443) managed vhosts via certbot integration
- [ ] Role-based permissions enforced in the API + user management UI
- [ ] Improved error reporting and logging
- [ ] Automated tests and CI/CD

## Contributing

Contributions, bug reports and feature requests are welcome.

1. Fork the repository.
2. Create a feature branch.
3. Make your changes.
4. Test your changes (against a throwaway Apache, never your production one).
5. Submit a Pull Request.

Please include clear descriptions and reproduction steps when reporting bugs.

## License

This project is licensed under the MIT License.

Copyright (c) 2026 Omid Zarifi.

See the [LICENSE](LICENSE) file for details.

## Disclaimer

GoApache Manager is an independent open-source project and is not affiliated with or endorsed by the Apache Software Foundation. Apache HTTP Server is a trademark of The Apache Software Foundation.
