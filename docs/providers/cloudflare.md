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

- **Proxied**: By default, `relayd` enables the Cloudflare Proxy (orange cloud) for all `A`, `AAAA`, and `CNAME` records it manages. This hides your origin IP address. To disable this and create unproxied DNS records, set the environment variable `RELAYD_PROVIDER_<NAME>_PROXIED=false`.
