# GoApache Manager

**A lightweight web-based Apache HTTP Server VirtualHost management panel built with Go and React.**

GoApache Manager provides a web interface for managing Apache VirtualHosts on Linux systems. It is designed to simplify common Apache administration tasks while keeping privileged operations isolated in a dedicated helper process.

## ScreenShot

![GoApache Manager](/screenshot.png?raw=true "GoApache Manager")


## Features

- **VirtualHost Management**
    - List Apache VirtualHosts.
    - Create managed VirtualHosts.
    - Edit DocumentRoot and ServerAliases.
    - Enable and disable VirtualHosts.
    - Delete managed VirtualHosts.
    - Distinguish managed configurations from external configurations.

- **Apache Service Management**
    - Check Apache service status.
    - Run Apache configuration tests.
    - Reload Apache.
    - Restart Apache.

- **Backup Management**
    - Automatically create backups before editing configurations.
    - List available backups.
    - Restore previous configurations.
    - Delete backups.

- **Security**
    - Authentication and role-based access control.
    - CSRF protection.
    - Privileged operations handled by a separate helper.
    - Unix socket communication between the application and helper.
    - Ownership markers for managed VirtualHost configurations.
    - DocumentRoot validation and symlink checks.
    - Configuration testing before applying changes.
    - Rollback when configuration changes fail.

## Technology Stack

| Component | Technology |
|---|---|
| Backend | Go |
| Frontend | React |
| Language | TypeScript |
| Build Tool | Vite |
| UI Icons | Lucide |
| Database | SQLite |
| Web Server | Apache HTTP Server |
| Privileged Operations | Go Helper + Unix Socket |

## Requirements

- Linux operating system.
- Apache HTTP Server 2.4 or later.
- Go 1.26 or compatible version.
- Node.js 22 or later.
- npm.
- Git.
- Root or sudo access for Apache administration.

The current implementation targets Debian/Ubuntu-based systems using the standard Apache configuration layout:

- `/etc/apache2/sites-available`
- `/etc/apache2/sites-enabled`
- `/var/www`

Other Linux distributions may require configuration changes.

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/destan0098/goapache-manager.git
cd goapache-manager
```

### 2. Build the backend

```bash
go build ./...
```

### 3. Build the frontend

```bash
cd frontend
npm install
npm run build
```

### 4. Configure Apache

Make sure Apache is installed and working:

```bash
sudo apache2ctl configtest
sudo systemctl status apache2
```

### 5. Configure the Helper

GoApache Manager uses a separate privileged helper for Apache operations.

The helper must be built and installed according to the deployment configuration of the project. It requires appropriate root privileges and communicates with the backend through a Unix socket.

**Important:** Never expose the privileged helper directly to the public network. Keep its communication restricted to the intended local Unix socket and authenticated application.

### 6. Start the application

Configure the backend, database, helper socket and frontend serving according to the project's deployment configuration.

For development, the frontend can be started using:

```bash
cd frontend
npm run dev
```

The development frontend uses the Vite development server.

## Security Considerations

GoApache Manager performs privileged system operations. Before deploying it:

- Use HTTPS in production.
- Change all default credentials.
- Do not expose the helper socket or helper HTTP interface to untrusted networks.
- Restrict access to the application.
- Protect configuration files and database files.
- Review firewall rules.
- Keep Go, Node.js and Apache updated.
- Use the least privileges necessary.
- Test configuration changes in a safe environment before production deployment.

This software is provided as-is. Users are responsible for securing their deployments and reviewing the code before granting administrative access.

## Project Structure

```text
goapache-manager/
├── cmd/
│   └── helper/
├── internal/
│   ├── api/
│   ├── apache/
│   ├── auth/
│   └── helper/
└── frontend/
    └── src/
```

## Roadmap

- [ ] Complete installation and deployment automation.
- [ ] Systemd service installation.
- [ ] Improved error reporting and logging.
- [ ] More detailed Apache configuration inspection.
- [ ] Additional security hardening.
- [ ] Automated tests and CI/CD.

## Contributing

Contributions, bug reports and feature requests are welcome.

1. Fork the repository.
2. Create a feature branch.
3. Make your changes.
4. Test your changes.
5. Submit a Pull Request.

Please include clear descriptions and reproduction steps when reporting bugs.

## License

This project is licensed under the MIT License.

Copyright (c) 2026 Omid Zarifi.

See the [LICENSE](LICENSE) file for details.

## Disclaimer

GoApache Manager is an independent open-source project and is not affiliated with or endorsed by the Apache Software Foundation.