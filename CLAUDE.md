# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a cert-manager webhook for Better WAPI, implementing ACME DNS-01 challenge solving for WEDOS DNS via the Better WAPI service. It runs as a Kubernetes service and integrates with cert-manager to automate TLS certificate issuance.

## Development Commands

### Build the webhook
```bash
go build -o webhook .
```

Or build Docker image:
```bash
docker build -t cert-manager-webhook-better-wapi:latest .
```

### Run conformance tests
```bash
TEST_ZONE_NAME=example.com. go test -v .
```

**IMPORTANT**: All cert-manager DNS01 webhooks must pass the conformance test suite.

### Linting and formatting
```bash
go vet ./...
go fmt ./...
```

### Download dependencies
```bash
go mod tidy
```

### CI/CD

The GitHub Actions workflow:
- **On push to branches or PRs**: Runs linting and builds
- **On push to tags (v*.*.*)**: Creates GitHub release and publishes multi-arch Docker images to GHCR

## Architecture

### Webhook Implementation

The webhook implements the cert-manager `Solver` interface with four key methods:

1. **Name()** - Returns "better-wapi" as the solver identifier
2. **Present()** - Creates TXT record for ACME challenge
3. **CleanUp()** - Removes TXT record after validation
4. **Initialize()** - Sets up Kubernetes client for secret access

### Better WAPI Integration

The webhook makes HTTP calls to a Better WAPI instance:

1. **Authentication** (`/api/auth/token`)
   - POST with login/secret from Kubernetes secrets
   - Returns JWT token for subsequent requests

2. **Create Record** (`/api/v1/domain/{domain}/record`)
   - POST with TXT record data
   - Subdomain format: `_acme-challenge.<subdomain>`
   - Uses autocommit=true for immediate DNS propagation
   - TTL set to 300 seconds

3. **Delete Record** (`/api/v1/domain/{domain}/record`)
   - DELETE with matching TXT record data
   - Same payload structure as create

### Domain Parsing

- `extractDomain()` extracts the TLD from FQDN (e.g., "example.com" from "*.example.com.")
- `extractSubdomain()` extracts subdomain prefix before the TLD
- ACME challenge always uses `_acme-challenge.` prefix

### Kubernetes Integration

**RBAC**: Webhook needs permission to read secrets in the namespace where Issuer/Certificate resources exist

**APIService**: Registered as `v1alpha1.acme.loupeznik.dev` Kubernetes API extension

**PKI**: Uses cert-manager to generate self-signed certificates for webhook TLS

## Configuration

The webhook is configured via Issuer/ClusterIssuer resources:

```yaml
solvers:
- dns01:
    webhook:
      groupName: acme.loupeznik.dev
      solverName: better-wapi
      config:
        baseUrl: "https://your-better-wapi.com"
        userLoginSecretRef:
          name: better-wapi-credentials
          key: login
        userSecretSecretRef:
          name: better-wapi-credentials
          key: secret
```

## Deployment

The Helm chart in `deploy/better-wapi-webhook/` includes:
- Deployment with webhook container
- Service (ClusterIP on port 443)
- RBAC (ServiceAccount, Role, ClusterRole)
- APIService registration
- Self-signed certificate via cert-manager

## Testing

Conformance tests in `main_test.go` validate the webhook against cert-manager's test suite. Test configuration is in `testdata/better-wapi/`.
