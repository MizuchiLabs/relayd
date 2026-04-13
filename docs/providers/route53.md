# AWS Route53

```env
RELAYD_PROVIDER_R53_TYPE=route53
RELAYD_PROVIDER_R53_ZONE_ID=your-zone-id
RELAYD_PROVIDER_R53_ZONES=example.com
```

- **Scope**: `public`
- **Requires**: AWS credentials and `route53:ChangeResourceRecordSets` permission.

### Authentication

The Route53 provider automatically loads AWS credentials from standard environment variables, the default AWS configuration files (`~/.aws/credentials`), or the IAM role if running on AWS EC2/ECS/EKS.

**Using Environment Variables:**

```env
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
AWS_REGION=us-east-1
# AWS_PROFILE=my-profile (Optional, if using shared credentials file)
```

**Required IAM Policy:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["route53:ChangeResourceRecordSets", "route53:ListResourceRecordSets"],
      "Resource": "arn:aws:route53:::hostedzone/your-zone-id"
    },
    {
      "Effect": "Allow",
      "Action": ["route53:ListHostedZones", "route53:ListHostedZonesByName"],
      "Resource": "*"
    }
  ]
}
```
