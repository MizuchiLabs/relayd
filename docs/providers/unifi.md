# UniFi

```env
RELAYD_PROVIDER_UNIFI_TYPE=unifi
RELAYD_PROVIDER_UNIFI_SCOPE=local
RELAYD_PROVIDER_UNIFI_URL=https://192.168.1.1
RELAYD_PROVIDER_UNIFI_TOKEN=api-token
RELAYD_PROVIDER_UNIFI_ZONES=example.com
RELAYD_PROVIDER_UNIFI_SITE=default # Optional, site name or ID
RELAYD_PROVIDER_UNIFI_INSECURE=true # Needed for the default self-signed certificate
```

- **Scope**: `local`
- **Requires**: UniFi controller credentials.
- **TLS**: UniFi gateways ship with a self-signed certificate. Set `INSECURE=true` to accept it, or put a trusted certificate on the gateway.

### Adopting Existing Domains

If you manually created a DNS record in your provider's Web UI (e.g., UniFi) and then tell `relayd` to manage that same domain, `relayd` will throw errors since UniFi handles ownership of the record.

- **The Fix:** To transfer ownership to `relayd`, simply delete the manually created record from your DNS provider's UI. Within seconds, `relayd` will see the gap, recreate the record via the API, and officially take ownership of it.
