<p align="center">
<img src="./.github/logo.svg" width="80">
<br><br>
<img alt="GitHub Tag" src="https://img.shields.io/github/v/tag/MizuchiLabs/relayd?label=Version">
<img alt="GitHub License" src="https://img.shields.io/github/license/MizuchiLabs/relayd">
<img alt="GitHub Issues or Pull Requests" src="https://img.shields.io/github/issues/MizuchiLabs/relayd">
</p>

# Relayd

`relayd` is a lightweight, "set and forget" external DNS synchronization agent for Docker. It seamlessly updates DNS records (A, AAAA, and TXT ownership records) across various providers based on your Docker container labels.

## Features

- **Docker Native**: Automatically extracts hostnames from `relayd.hosts` and Traefik `.rule` labels.
- **Dual-Stack Support**: Synchronizes both `A` (IPv4) and `AAAA` (IPv6) records simultaneously.
- **Safe Ownership**: Uses specific `TXT` records to track ownership, guaranteeing it will never overwrite or delete domains it doesn't own.
- **Multi-Provider**: Sync your public domains to Cloudflare, while simultaneously syncing your internal/local domains to Pi-hole, UniFi, or PowerDNS.

## Usage

Simply run the container and mount the docker socket:

### Docker Compose

In standard Docker, `relayd` automatically discovers the primary local IP using the OS routing table.
Use `network_mode: host` to give relayd direct access to the host's network interfaces so it can discover the host's actual LAN IP:

```yaml
services:
  relayd:
    image: ghcr.io/mizuchilabs/relayd:latest
    network_mode: host
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
      - RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-api-token
      - RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
```

### Docker Swarm

`relayd` has **native Docker Swarm support**. Because Swarm's ingress routing mesh automatically routes traffic received on any node to the correct container, you only need to publish **one** IP address for your services.

You should run exactly **one instance** of `relayd` on a manager node. It will automatically detect all swarm services with the `relayd.enable=true` label across the entire cluster and publish its own node's IP.

```yaml
services:
  relayd:
    image: ghcr.io/mizuchilabs/relayd:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role == manager
    environment:
      - RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
      - RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-api-token
      - RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
```

> **Note:** Without host networking or manual overrides, relayd will discover its internal container IP. If you are using local-scoped providers and need the actual Host IP, you can configure the `RELAYD_LOCAL_OVERRIDE_IPV4` and `RELAYD_LOCAL_OVERRIDE_IPV6` environment variables manually. For public-scoped providers (the default), `relayd` uses external services to resolve the public IP automatically, so network mode does not matter.

### Adding domains to your containers

You can configure DNS targets by adding the `relayd.enable` and `relayd.hosts` label to any container:

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      - relayd.enable=true
      - relayd.hosts=whoami.example.com,test.example.com
      # If you are using Traefik, you can also add the following and remove the `relayd.hosts` label:
      - traefik.enable=true
      - traefik.http.routers.whoami.rule=Host(`whoami.example.com`)
      # To restrict which DNS providers or scopes a container uses use the `relayd.providers` label.
      # Values are provider names (the <NAME> in RELAYD_PROVIDER_<NAME>_TYPE, any case) or scopes:
      - relayd.providers=local # or public, cloudflare, pihole, unifi...
      # Optional TTL for this container's records, in seconds or as a duration (60s to 24h):
      - relayd.ttl=5m
```

## Architecture & Edge Cases

To prevent disaster, `relayd` uses a **Safe Ownership** model. Whenever it creates an `A` or `AAAA` record, it creates a companion `TXT` record (e.g., `relayd.yoursubdomain="managed-by=relayd-yourhostname"`). Outside of force mode, `relayd` will **never** delete or modify a DNS record unless it sees its exact matching TXT record.

### Force Mode

Set `RELAYD_PROVIDER_<NAME>_FORCE=true` to skip ownership tracking. relayd then treats the whole zone as its own: no TXT records are created, and **every `A` and `AAAA` record in the zone that no container asks for is deleted**. Only use it on zones dedicated to relayd. Pi-hole always runs in force mode because it has no TXT records.

### Multiple Instances

Each instance tags its TXT records with its instance name (`RELAYD_INSTANCE`, defaults to the hostname). Instances with different names ignore each other's hosts, so several Docker hosts can share one zone safely.
Do not point two instances at the same zone in force mode. Each one deletes the other's records.

### TTL

Without a TTL, relayd leaves it to the provider: Cloudflare and UniFi use Auto, Hetzner, DigitalOcean and Linode use the zone default. Route53, PowerDNS and RFC2136 have no auto, so relayd uses `5m` there.

Set a default per provider with `RELAYD_PROVIDER_<NAME>_TTL`, or per container with the `relayd.ttl` label (the label wins). If several containers publish the same host with different TTLs, the lowest one is used. When a TTL changes, relayd updates the existing records in place. Pi-hole has no TTL, so both settings are ignored there.

### Dry Run

Set `RELAYD_DRY_RUN=true` to log every record relayd would create or delete without touching your DNS. Handy before pointing it at a real zone.

### Using with tether & tetherd (Centralized Traefik)

If you are using `relayd` in combination with `tether` and `tetherd` to proxy Traefik configurations to a central hub, you may encounter a DNS routing issue.

By default, an agent running `relayd` will publish its _own_ local IP for its containers. However, since all traffic must flow through your central Traefik instance, DNS queries will point to the agent instead of the central router, causing resolution to fail.

To fix this, you need to explicitly tell `relayd` on the agent machines to broadcast the central Traefik server's IP instead of their own. You can do this using the IP override environment variables:

```yaml
services:
  relayd:
    image: ghcr.io/mizuchilabs/relayd:latest
    environment:
      # Override the detected IP with the IP of your central Traefik instance
      - RELAYD_LOCAL_OVERRIDE_IPV4=192.168.1.10
```

## Configuration

Relayd can be configured entirely via environment variables.

### Global Options

| Variable                      | Default    | Description                                                   |
| :---------------------------- | :--------- | :------------------------------------------------------------ |
| `RELAYD_INTERVAL`             | `5m`       | Background sync interval (e.g. `5m`, `1h`).                   |
| `RELAYD_DRY_RUN`              | `false`    | Log planned changes without applying them.                    |
| `RELAYD_DEBUG`                | `false`    | Enable debug logging.                                         |
| `RELAYD_INSTANCE`             | _hostname_ | Instance name (e.g. `my-nas`, `test-server`).                 |
| `RELAYD_IP_FAMILY`            | `ipv4`     | IP family to synchronize (`ipv4`, `ipv6`, `dual`).            |
| `RELAYD_LOCAL_OVERRIDE_IPV4`  | _auto_     | Hardcode the local IPv4 address instead of auto-discovering.  |
| `RELAYD_LOCAL_OVERRIDE_IPV6`  | _auto_     | Hardcode the local IPv6 address instead of auto-discovering.  |
| `RELAYD_PUBLIC_OVERRIDE_IPV4` | _auto_     | Hardcode the public IPv4 address instead of auto-discovering. |
| `RELAYD_PUBLIC_OVERRIDE_IPV6` | _auto_     | Hardcode the public IPv6 address instead of auto-discovering. |

### Configuring Providers

Providers are automatically discovered by scanning your environment variables for any variable ending in `_TYPE` with the `RELAYD_PROVIDER_` prefix. You can name your providers anything you like (e.g., `CF`, `LOCAL`, `MYDNS`).

Every provider takes these variables. relayd refuses to start if one is invalid.

| Variable                       | Default  | Description                                                  |
| :----------------------------- | :------- | :----------------------------------------------------------- |
| `RELAYD_PROVIDER_<NAME>_TYPE`  |          | Provider type, see the list below.                           |
| `RELAYD_PROVIDER_<NAME>_ZONES` |          | Comma-separated zones to manage. Required.                   |
| `RELAYD_PROVIDER_<NAME>_SCOPE` | `public` | `public` publishes the public IP, `local` the host's LAN IP. |
| `RELAYD_PROVIDER_<NAME>_FORCE` | `false`  | Skip ownership tracking, see [Force Mode](#force-mode).      |
| `RELAYD_PROVIDER_<NAME>_TTL`   | _auto_   | Default TTL (`300` or `5m`), see [TTL](#ttl).                |

For detailed configuration examples per provider, see the [docs/providers](/docs/providers) directory:

- [AWS Route53](/docs/providers/route53.md)
- [Cloudflare](/docs/providers/cloudflare.md)
- [DigitalOcean](/docs/providers/digitalocean.md)
- [Hetzner](/docs/providers/hetzner.md)
- [Linode](/docs/providers/linode.md)
- [Namecheap](/docs/providers/namecheap.md)
- [Pi-hole](/docs/providers/pihole.md)
- [PowerDNS](/docs/providers/powerdns.md)
- [RFC2136](/docs/providers/rfc2136.md)
- [Scaleway](/docs/providers/scaleway.md)
- [UniFi](/docs/providers/unifi.md)

## License

Apache 2.0 License - see [LICENSE](LICENSE) for details
