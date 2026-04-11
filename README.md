<p align="center">
<img src="./.github/logo.svg" width="80">
<br><br>
<img alt="GitHub Tag" src="https://img.shields.io/github/v/tag/MizuchiLabs/relayd?label=Version">
<img alt="GitHub License" src="https://img.shields.io/github/license/MizuchiLabs/relayd">
<img alt="GitHub Issues or Pull Requests" src="https://img.shields.io/github/issues/MizuchiLabs/relayd">
</p>

# Relayd

`relayd` is a lightweight "set and forget" external DNS synchronization agent for Docker. It seamlessly updates DNS records (A, AAAA, and TXT ownership records) across various providers based on Docker container labels.

It supports dual-stack IPv4/IPv6 out of the box, handles seamless resolution of local (LAN) and public (WAN) interface IPs, and supports multiple DNS providers like Cloudflare, DigitalOcean, Route53, PowerDNS, Pi-hole, UniFi, and standard RFC2136.

## Features

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
      - RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
      - RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-api-token
      - RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
```

### Adding domains to your containers

You can configure DNS targets by adding the `relayd.enable` and `relayd.hosts` label to any container:

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      - relayd.enable=true
      - relayd.hosts=whoami.example.com,test.example.com
```

If you use **Traefik**, `relayd` automatically parses `Host()` rules, so you don't even need to add the `relayd.hosts` label!

You can also restrict which DNS providers or scopes a container uses by setting the `relayd.providers` label to a comma-separated list of provider names or scopes:

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      - relayd.enable=true
      - relayd.hosts=local.example.com
      - relayd.providers=local,cloudflare
```

## Configuration

Relayd can be configured entirely via environment variables.

### Global Options

| Variable                      | Default | Description                                                       |
| :---------------------------- | :------ | :---------------------------------------------------------------- |
| `RELAYD_INTERVAL`             | `5m`    | Background sync interval (e.g. `5m`, `1h`).                       |
| `RELAYD_FORCE`                | `false` | Forcefully overwrite existing DNS records ignoring TXT ownership. |
| `RELAYD_LOCAL_OVERRIDE_IPV4`  | _auto_  | Hardcode the local IPv4 address instead of auto-discovering.      |
| `RELAYD_LOCAL_OVERRIDE_IPV6`  | _auto_  | Hardcode the local IPv6 address instead of auto-discovering.      |
| `RELAYD_PUBLIC_OVERRIDE_IPV4` | _auto_  | Hardcode the public IPv4 address instead of auto-discovering.     |
| `RELAYD_PUBLIC_OVERRIDE_IPV6` | _auto_  | Hardcode the public IPv6 address instead of auto-discovering.     |

### Configuring Providers

Providers are automatically discovered by scanning your environment variables for any variable ending in `_TYPE` with the `RELAYD_PROVIDER_` prefix. You can name your providers anything you like (e.g., `CF`, `LOCAL`, `MYDNS`).

For detailed configuration examples per provider, see the [docs/providers](/docs/providers) directory:

- [Cloudflare](/docs/providers/cloudflare.md)
- [DigitalOcean](/docs/providers/digitalocean.md)
- [Pi-hole](/docs/providers/pihole.md)
- [PowerDNS](/docs/providers/powerdns.md)
- [AWS Route53](/docs/providers/route53.md)
- [RFC2136](/docs/providers/rfc2136.md)
- [UniFi](/docs/providers/unifi.md)

## License

Apache 2.0 License - see [LICENSE](LICENSE) for details

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
