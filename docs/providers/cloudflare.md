# Cloudflare

```env
RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-token
RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
RELAYD_PROVIDER_CLOUDFLARE_PROXIED=true # Optional, defaults to true
```

- **Scope**: `public`
- **Requires**: API Token with `Zone.DNS` permissions.

## Features

- **Proxied (default: enabled)**: By default, `relayd` enables the Cloudflare Proxy (orange cloud) for all `A`, `AAAA`, and `CNAME` records it manages. This is a **security default** — Cloudflare's proxy hides your origin server's IP address from DNS lookups, protecting it from direct attacks. To disable proxying and expose your origin IP directly in DNS records, explicitly set `RELAYD_PROVIDER_<NAME>_PROXIED=false`.
