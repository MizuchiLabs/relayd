# Pi-hole

```env
RELAYD_PROVIDER_PIHOLE_TYPE=pihole
RELAYD_PROVIDER_PIHOLE_SCOPE=local
RELAYD_PROVIDER_PIHOLE_URL=http://10.0.0.5:8080
RELAYD_PROVIDER_PIHOLE_TOKEN=your-api-key
RELAYD_PROVIDER_PIHOLE_ZONES=home.local
```

- **Scope**: Recommended `local`
- **Requires**: API key from Pi-hole web interface settings.

> **Note:** Force mode is always enabled for Pi-hole. The Pi-hole API does not support TXT records, so relayd cannot use its standard TXT-based ownership tracking. Force mode ensures stale DNS records are always cleaned up when containers are removed. The `FORCE` environment variable is ignored for this provider.
