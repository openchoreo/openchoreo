# Security Policy
OpenChoreo takes the security of our users and the ecosystem seriously. We follow industry-standard practices for responsible disclosure and encourage the community to report vulnerabilities safely and privately. 

If you discover a security issue, please do not open a public GitHub issue. Instead, follow the process below.

# Reporting a Vulnerability
To report a security vulnerability, please email: [security@openchoreo.io](mailto:security@openchoreo.io)

Your report should include:
- A description of the issue
- Steps to reproduce (if possible)
- Potential impact
- Any suggested remediation
- Your contact information for follow-up questions


You will receive an acknowledgment within 3 business days.

# Security Response Process
Once a report is received:
1. The security team reviews the issue and determines the severity.
2. A fix is developed privately in a restricted branch or fork.
3. Once validated, the patch is merged and released.
4. Security advisories (GHSA) are published as needed.
5. The reporter is credited unless they request anonymity.

We strive to resolve critical issues as quickly as possible and follow coordinated disclosure best practices.

# Supported Versions
Security fixes will be backported to:
- The latest stable release
- The previous stable minor release (if feasible)


Pre-1.0 releases (0.x) may receive fixes at the discretion of the maintainers.

# Public Communication
Security announcements will be published through:
- GitHub Security Advisories
- Release notes
- The OpenChoreo community channels

# Operational Security Best Practices

The following recommendations help platform engineers secure their OpenChoreo installation.

## Cluster Isolation

- **Separate control and data planes**: Run the control plane and data plane(s) in separate Kubernetes clusters for production deployments. The single-cluster mode is convenient for development but does not provide workload isolation.
- **Use dedicated namespaces**: Do not deploy user workloads into OpenChoreo system namespaces (`openchoreo-control-plane`, `openchoreo-data-plane`, etc.).
- **Enable network policies**: Restrict traffic between planes to only the required ports and protocols. The cluster agent uses WebSocket connections that should be TLS-encrypted.

## Secret Management

- **Use External Secrets Operator (ESO)**: OpenChoreo integrates with ESO via `SecretReference` resources. Use a production-grade secret backend (AWS Secrets Manager, Azure Key Vault, HashiCorp Vault) rather than the default OpenBao dev instance.
- **Rotate secrets regularly**: Set appropriate `refreshInterval` values on SecretReference resources and rotate backend credentials on a schedule.
- **Never store secrets in YAML manifests**: Use SecretReference or Kubernetes Secrets with ESO — never inline sensitive values in Component or Workload specs.

## Cluster Agent Security

- **TLS for agent connections**: The DataPlane and WorkflowPlane cluster agents connect to the control plane via WebSocket. Always configure TLS certificates for the `clusterAgent` configuration. The Helm charts generate self-signed certificates by default; replace these with certificates from your organization's CA for production.
- **Restrict agent permissions**: The cluster agent service account should have only the minimum permissions required to manage resources in its plane.

## RBAC Configuration

- **Configure AuthzRoleBindings**: Do not run OpenChoreo without authorization in production. Define roles for developers, platform engineers, and CI/CD systems.
- **Use deny bindings for critical actions**: Explicitly deny destructive actions (delete, promote to production) for broad groups, then allow them for specific elevated roles.
- **Integrate with your identity provider**: Configure the IdP integration to map JWT claims (groups, roles) to OpenChoreo AuthzRoleBindings.

## Container Image Security

- **Use private registries**: Configure the Workflow Plane to push built images to a private container registry with vulnerability scanning enabled.
- **Pin image tags**: In production Workloads, use immutable image tags (SHA digests) rather than mutable tags like `latest`.
- **Scan base images**: When using Buildpacks or Dockerfiles, ensure base images are regularly updated and scanned for vulnerabilities.

## Audit and Monitoring

- **Enable Kubernetes audit logging**: Monitor API server audit logs for unauthorized access attempts to OpenChoreo CRDs.
- **Use the Observability Plane**: Deploy the Observability Plane with OpenSearch to get centralized logging and alerting for all components.
- **Monitor controller logs**: The control-plane controller manager logs all reconciliation events. Monitor these for unexpected errors or permission denied messages.
