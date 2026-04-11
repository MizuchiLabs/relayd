# AWS Route53

```env
RELAYD_PROVIDER_R53_TYPE=route53
RELAYD_PROVIDER_R53_ZONE_ID=your-zone-id
RELAYD_PROVIDER_R53_ZONES=example.com
```

- **Scope**: `public`
- **Requires**: Standard AWS credentials (via environment variables, IAM role, or `~/.aws/credentials`) with `route53:ChangeResourceRecordSets` permission.
