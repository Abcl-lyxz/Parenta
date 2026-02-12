# Parenta

A plug-and-play parental control system for OpenWrt routers using OpenNDS captive portal.

Designed for **Xiaomi AX3000T** (256MB RAM) and similar resource-constrained routers.

## Features

- **Captive Portal Authentication** - Children must login to access internet
- **Time Quotas** - Set daily screen time limits per child
- **Schedules** - Define when internet access is allowed
- **Domain Filtering** - Whitelist/blacklist domains
  - Study Mode: Only whitelisted domains allowed
  - Normal Mode: Blacklisted domains blocked
- **Session Management** - View active sessions, kick devices
- **Auto Device Discovery** - Devices registered automatically on login
- **Anti-Circumvention** - DNS hijacking, DoT/DoH blocking

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Child Device  │────▶│    OpenNDS      │────▶│  Parenta-Core   │
│   (WiFi Client) │     │ (Captive Portal)│     │  (Go Backend)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                                                ┌─────────────────┐
                                                │    Storage      │
                                                │  (JSON Files)   │
                                                └─────────────────┘
```

## Requirements

- OpenWrt 22.03+ with fw4 (nftables)
- OpenNDS package installed
- ~20MB free storage
- 256MB+ RAM

## Quick Start

### 1. Build

```bash
# On development machine (requires Go 1.21+)
./scripts/build.sh
```

### 2. Deploy

```bash
# Copy to router
scp build/parenta-1.0.0-openwrt-arm64.tar.gz root@192.168.1.1:/tmp/

# On router
cd /tmp
tar -xzf parenta-1.0.0-openwrt-arm64.tar.gz
chmod +x scripts/setup.sh
setsid ./scripts/setup.sh > setup.log 2>&1 &
```

### 3. Access

- Admin Dashboard: `http://192.168.1.1:8080`
- Default login: `admin` / `parenta123`

**Change the password on first login!**

## Configuration

Configuration file: `/etc/parenta/parenta.json`

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080
  },
  "storage": {
    "data_dir": "/opt/parenta/data"
  },
  "opennds": {
    "ndsctl_path": "/usr/bin/ndsctl",
    "fas_key": "your-secret-key",
    "gateway_ip": "192.168.1.1"
  },
  "defaults": {
    "daily_quota_minutes": 120,
    "admin_username": "admin",
    "admin_password": "parenta123"
  }
}
```

## Directory Structure

```
/opt/parenta/
├── parenta           # Binary
├── web/              # Frontend files
└── data/             # JSON storage
    ├── admin.json
    ├── children.json
    ├── sessions.json
    ├── schedules.json
    └── filters.json

/etc/parenta/
└── parenta.json      # Configuration

/etc/dnsmasq.d/
├── parenta-blocklist.conf
├── parenta-whitelist.conf
└── parenta-antidoh.conf
```

## Security Limitations

This system provides reasonable parental controls but has known limitations:

1. **DoH over CDN** - DNS-over-HTTPS servers on shared CDN IPs cannot be blocked
2. **VPN** - VPN connections bypass all filtering
3. **Mobile Data** - Cellular connections bypass the router
4. **MAC Spoofing** - Tech-savvy users can spoof MAC addresses

**Parental controls are a tool to help manage screen time, not a complete solution. Open communication with children is essential.**

## API Reference

### Authentication
- `POST /api/auth/login` - Parent login
- `POST /api/auth/logout` - Logout
- `GET /api/auth/me` - Current user info
- `POST /api/auth/password` - Change password

### Children
- `GET /api/children` - List all children
- `POST /api/children` - Create child
- `GET /api/children/:id` - Get child details
- `PUT /api/children/:id` - Update child
- `DELETE /api/children/:id` - Delete child
- `POST /api/children/:id/reset-quota` - Reset daily quota

### Sessions
- `GET /api/sessions` - List active sessions
- `POST /api/sessions/:id/kick` - Disconnect session
- `POST /api/sessions/:id/extend` - Add time

### Schedules
- `GET /api/schedules` - List schedules
- `POST /api/schedules` - Create schedule
- `PUT /api/schedules/:id` - Update schedule
- `DELETE /api/schedules/:id` - Delete schedule

### Filters
- `GET /api/filters` - List filter rules
- `POST /api/filters` - Create filter rule
- `DELETE /api/filters/:id` - Delete filter rule
- `POST /api/filters/reload` - Apply filter changes

### System
- `GET /api/system/status` - System status
- `POST /api/system/restart` - Restart service

## Troubleshooting

### Check service status
```bash
/etc/init.d/parenta status
/etc/init.d/opennds status
```

### View logs
```bash
logread | grep parenta
logread | grep opennds
```

### Test captive portal
```bash
ndsctl status
ndsctl json
```

### Manual deauth
```bash
ndsctl deauth AA:BB:CC:DD:EE:FF
```

## Development

### Build for development
```bash
go run ./cmd/parenta -config configs/parenta.json -web web
```

### Project structure
```
├── cmd/parenta/         # Entry point
├── internal/
│   ├── api/             # HTTP handlers
│   ├── config/          # Configuration
│   ├── models/          # Data models
│   ├── services/        # Business logic
│   └── storage/         # JSON storage
├── web/                 # Frontend SPA
├── scripts/             # Build/deploy scripts
├── deploy/openwrt/      # OpenWrt configs
└── configs/             # Config templates
```

## License

MIT License

## Credits

- [OpenNDS](https://github.com/openNDS/openNDS) - Captive portal
- [OpenWrt](https://openwrt.org/) - Router firmware
