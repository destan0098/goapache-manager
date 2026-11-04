## Quick Start

**Get GoApache Manager running in minutes — no Go, Node.js, or manual build required!**

### Download the latest release

Download the prebuilt Linux AMD64 package:

[**Download GoApache Manager v1.1.0**](https://github.com/destan0098/goapache-manager/releases/download/v1.1.0/goapache-manager-linux-amd64.tar.gz)

### Verify and extract

```bash
wget https://github.com/destan0098/goapache-manager/releases/download/v1.1.0/goapache-manager-linux-amd64.tar.gz

wget https://github.com/destan0098/goapache-manager/releases/download/v1.1.0/goapache-manager-linux-amd64.tar.gz.sha256

sha256sum -c goapache-manager-linux-amd64.tar.gz.sha256

tar -xzf goapache-manager-linux-amd64.tar.gz

cd goapache-manager-linux-amd64
```

### Install

```bash
sudo bash scripts/install.sh
```

The interactive installer will guide you through the panel domain configuration and administrator account setup.

### Requirements

- Linux AMD64 (x86_64)
- Debian, Ubuntu, or Linux Mint
- Apache HTTP Server 2.4
- Root / sudo access

**No Go, Node.js, npm, or compiler is required when using the prebuilt release.**

### Documentation

- [Installation Guide](#installation)
- [Security Considerations](#security-considerations)
- [Troubleshooting](#troubleshooting)
- [All Releases](https://github.com/destan0098/goapache-manager/releases)