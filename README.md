# cert-manager ACME DNS01 Webhook for Better WAPI

This is a cert-manager webhook solver for [Better WAPI](https://github.com/loupeznik/better-wapi), enabling ACME DNS-01 challenges for WEDOS DNS through cert-manager.

## Prerequisites

- Kubernetes cluster with cert-manager installed
- Better WAPI instance running and accessible from the cluster
- Better WAPI credentials (login and secret)

## Installation

### Install using Helm

```bash
helm install better-wapi-webhook ./deploy/better-wapi-webhook \
  --namespace cert-manager \
  --set image.repository=ghcr.io/loupeznik/cert-manager-webhook-better-wapi \
  --set image.tag=latest
```

### Create credentials secret

Create a Kubernetes secret with your Better WAPI credentials:

```bash
kubectl create secret generic better-wapi-credentials \
  --namespace=cert-manager \
  --from-literal=login=your-login \
  --from-literal=secret=your-secret
```

## Usage

### Create an Issuer

Create a cert-manager Issuer (or ClusterIssuer) configured to use the Better WAPI webhook:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-prod
  namespace: default
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - dns01:
        webhook:
          groupName: acme.loupeznik.dev
          solverName: better-wapi
          config:
            baseUrl: "https://your-better-wapi-instance.com"
            userLoginSecretRef:
              name: better-wapi-credentials
              key: login
            userSecretSecretRef:
              name: better-wapi-credentials
              key: secret
```

### Request a certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com
  namespace: default
spec:
  secretName: example-com-tls
  issuerRef:
    name: letsencrypt-prod
  dnsNames:
  - example.com
  - '*.example.com'
```

## Configuration Options

| Field | Description | Required |
|-------|-------------|----------|
| `baseUrl` | Base URL of your Better WAPI instance | Yes |
| `userLoginSecretRef` | Reference to secret containing Better WAPI login | Yes |
| `userSecretSecretRef` | Reference to secret containing Better WAPI secret | Yes |

## Development

### Building

```bash
docker build -t cert-manager-webhook-better-wapi:latest .
```

### Testing

```bash
TEST_ZONE_NAME=example.com. go test -v .
```

### Running locally

```bash
go run .
```

## How it works

When cert-manager requests a DNS-01 challenge:

1. The webhook receives a `Present` call with the challenge domain and validation key
2. It authenticates with Better WAPI using the provided credentials
3. Creates a TXT record at `_acme-challenge.<domain>` with the validation key
4. Let's Encrypt validates the DNS record
5. The webhook receives a `CleanUp` call to remove the TXT record

The webhook uses Better WAPI's v1 API with autocommit enabled for immediate DNS propagation.

## License

This project is licensed under the same license as Better WAPI - [GPL-3.0](LICENSE).

## Credits

Created by [Dominik Zarsky](https://github.com/Loupeznik).

Based on the [cert-manager webhook-example](https://github.com/cert-manager/webhook-example).
