# Cloudflare

```env
RELAYD_PROVIDER_CLOUDFLARE_TYPE=cloudflare
RELAYD_PROVIDER_CLOUDFLARE_TOKEN=your-token
RELAYD_PROVIDER_CLOUDFLARE_ZONES=example.com
RELAYD_PROVIDER_CLOUDFLARE_PROXIED=true # Optional, see below
```

- **Scope**: `public`
- **Requires**: API Token with `Zone.DNS` permissions.

## Features

- **Proxied**: With the default `public` scope, `relayd` enables the Cloudflare Proxy (orange cloud) for the `A`, `AAAA`, and `CNAME` records it manages. The proxy hides your origin IP from DNS lookups. With `local` scope proxying is off by default, since Cloudflare can't proxy private IPs. Set `RELAYD_PROVIDER_<NAME>_PROXIED=true` or `false` to override either default.
