# Pi-hole

```env
RELAYD_PROVIDER_PIHOLE_TYPE=pihole
RELAYD_PROVIDER_PIHOLE_SCOPE=local
RELAYD_PROVIDER_PIHOLE_URL=http://10.0.0.5:8080
RELAYD_PROVIDER_PIHOLE_TOKEN=your-password
RELAYD_PROVIDER_PIHOLE_ZONES=home.lan
RELAYD_PROVIDER_PIHOLE_INSECURE=false # Optional, set true to accept a self-signed certificate
```

- **Scope**: Recommended `local`
- **Requires**: Pi-hole v6 or newer. `TOKEN` is the web interface password or an app password (Settings > Web interface / API > Configure app password). Leave it empty if your Pi-hole has no password.
- **URL**: The base address of the web interface, without `/admin` or `/api`.

> **Note:** Force mode is always enabled for Pi-hole. The Pi-hole API does not support TXT records, so relayd cannot use its standard TXT-based ownership tracking. The `FORCE` environment variable is ignored for this provider.
>
> **This means relayd deletes every local DNS entry inside the configured zones that no container asks for.** Use a zone for relayd only (e.g. `docker.home.lan`) if you also keep hand-made entries in Pi-hole.
