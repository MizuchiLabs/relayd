# relayd

`relayd` is a lightweight, zero-configuration "set and forget" external DNS synchronization agent for Docker. It seamlessly updates DNS records (A, AAAA, and TXT ownership records) across various providers based on Docker container labels.

It supports dual-stack IPv4/IPv6 out of the box, handles seamless resolution of local (LAN) and public (WAN) interface IPs, and supports multiple DNS providers like Cloudflare, DigitalOcean, Route53, PowerDNS, Pi-hole, UniFi, and standard RFC2136.

## Features

- **Zero-config Defaults**: Drop it into your `docker-compose.yml` and it just works.
- **Docker Label Discovery**: Automatically extracts hostnames from `relayd.hosts` and Traefik `.rule` labels.
- **Dual-Stack Support**: Synchronizes both `A` (IPv4) and `AAAA` (IPv6) records simultaneously.
- **Safe Ownership**: Uses `TXT` records to track ownership, preventing it from overwriting domains it doesn't own (unless `--force` is used).
- **Multi-Provider**: Sync to Cloudflare for public domains while simultaneously syncing to PowerDNS for local domains.

## Usage

Simply run the container and mount the docker socket:

```yaml
services:
  relayd:
    image: ghcr.io/mizuchilabs/relayd:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - RELAYD_PROVIDERS=cloudflare
      - RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-api-token
      - RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
```

### Adding domains to your containers

You can configure DNS targets by adding the `relayd.hosts` label to any container:

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      - "relayd.hosts=whoami.example.com,test.example.com"
```

If you use **Traefik**, `relayd` automatically parses `Host()` rules, so you don't even need to add the `relayd.hosts` label!

## Configuration

Relayd can be configured entirely via environment variables.

### Global Options

| Variable                             | Default | Description                                                       |
| :----------------------------------- | :------ | :---------------------------------------------------------------- |
| `RELAYD_INTERVAL`                    | `5m`    | Background sync interval (e.g. `5m`, `1h`).                       |
| `RELAYD_FORCE`                       | `false` | Forcefully overwrite existing DNS records ignoring TXT ownership. |
| `RELAYD_TARGET_LOCAL_OVERRIDE_IPV4`  | _auto_  | Hardcode the local IPv4 address instead of auto-discovering.      |
| `RELAYD_TARGET_PUBLIC_OVERRIDE_IPV4` | _auto_  | Hardcode the public IPv4 address instead of auto-discovering.     |

### Configuring Providers

Providers are automatically discovered by scanning your environment variables for any variable ending in `_TYPE` with the `RELAYD_PROVIDER_` prefix. You can name your providers anything you like (e.g., `CF`, `LOCAL`, `MYDNS`).

**Common Variables for all providers:**

- `..._TYPE`: The type of provider (`cloudflare`, `digitalocean`, `route53`, `powerdns`, `unifi`, `rfc2136`).
- `..._SCOPE`: `public` (uses external WAN IP) or `local` (uses local LAN interface IP). Defaults to `public`.
- `..._ZONES`: Comma-separated list of root zones to manage (e.g. `example.com,test.com`).
- `..._TOKEN`: The API token (for Cloudflare, DigitalOcean, PowerDNS, etc).

**Provider-Specific Examples:**

#### Cloudflare

```env
RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-token
RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
```

#### Pi-hole

```env
RELAYD_PROVIDER_PIHOLE_TYPE=pihole
RELAYD_PROVIDER_PIHOLE_SCOPE=local
RELAYD_PROVIDER_PIHOLE_URL=http://10.0.0.5:8080
RELAYD_PROVIDER_PIHOLE_TOKEN=your-api-key
RELAYD_PROVIDER_PIHOLE_ZONES=home.local
```

#### RFC2136 (Technitium, BIND)

```env
RELAYD_PROVIDER_TECH_TYPE=rfc2136
RELAYD_PROVIDER_TECH_SCOPE=local
RELAYD_PROVIDER_TECH_SERVER=192.168.1.5:53
RELAYD_PROVIDER_TECH_KEY_NAME=tsig-key
RELAYD_PROVIDER_TECH_KEY=base64-encoded-key
RELAYD_PROVIDER_TECH_ZONES=home.local
```
