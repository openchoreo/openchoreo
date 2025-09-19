# Using an External Identity Provider to Secure APIs in OpenChoreo

**Authors**:  
_@vajiraprabuddhaka_

**Reviewers**:  
_@Mirage20 @sameerajayasoma_

**Created Date**:  
_2025-04-16_

**Status**:  
_Submitted_

**Related Issues/PRs**:  
_https://github.com/openchoreo/openchoreo/issues/186_

---

## Summary

This proposal introduces the capability for OpenChoreo to integrate with external Identity Providers (IdPs) such as Asgardeo, Auth0, Okta, Azure AD, or AWS Cognito to secure APIs exposed through the platform. Currently, OpenChoreo's API security model relies on built-in authentication mechanisms, which limits enterprise adoption where organizations have standardized on external identity management systems.

This enhancement enables platform engineers to define external identity providers in OpenChoreo and allows application developers to utilize those providers for securing their APIs. The integration will work seamlessly with OpenChoreo's multi-plane architecture, where identity provider configurations are managed in the control plane and enforced in data plane clusters through the API gateway infrastructure.

The proposal enables organizations to leverage their existing identity infrastructure investments while maintaining OpenChoreo's declarative, GitOps-compatible approach to API management.

---

## Motivation

Enterprise organizations have standardized on external identity providers (Asgardeo, Auth0, Okta, Azure AD, AWS Cognito) for user authentication and authorization across their applications. To enable OpenChoreo adoption in enterprise environments, APIs exposed through the platform must integrate seamlessly with these existing identity management systems.

This integration is essential for organizations to maintain centralized user management, enforce consistent security policies, ensure compliance requirements, and leverage their existing identity infrastructure investments. Without external IdP support, organizations cannot fully adopt OpenChoreo as their internal developer platform.

---

## Goals

### Security requirements

- **Environment level isolation**.
  - The primary way we should support environment-level isolation is by having two different identity tenants (or maybe two different IDPs)
  - Shouldn’t be very opinionated; users should have the capability to do whatever they want by dealing with claim validations.
    - Can use single IDP across multiple environments by bringing up client details (client_id) into the OpenChoreo Endpoint level.
- **Across component isolation**.
  - If the same Identity Provider is shared across multiple components, the same token should not be used to access multiple components.
- **Operation-level access control**.
  - If a component has multiple operations, it should be possible to provide access control using scopes.

---

## Non-Goals

- Securing the OpenChoreo system APIs using an external Identity provider.

---

## Impact

- **APIClass CRD**: Place where Platform engineers can configure external identity providers.
- **ServiceBinding Controller**: Generate the necessary gateway artifacts to propagate the external identity provider configuration to the API gateway.

---

## Design

### Overview

External identity provider integration leverages OpenChoreo's existing APIClass CRD. Platform engineers define external IdP configurations in APIClass resources, while application developers reference the APIClass when they are creating their services.

### APIClass Configuration

The existing `AuthenticationPolicy` structure in APIClass will be extended to support external identity providers:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: APIClass
metadata:
  name: enterprise-api-class
  namespace: acme
spec:
  restPolicy:
    defaults:
      authentication:
        type: jwt
        jwt:
          issuer: "https://auth.company.com"
          jwks: "https://auth.company.com/.well-known/jwks.json"
          audience: ["api://choreo-apis"]
```

### Service Configuration

Application developers reference the APIClass when exposing their APIs:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Service
metadata:
  name: user-service
  namespace: acme
spec:
  className: enterprise-api-class
  workloadName: user-service-workload
  apis:
    user-api:
      className: enterprise-api-class
      rest:
        backend:
          basePath: /api/v1/users
          port: 8080
        exposeLevels:
        - Organization
```

### Control Flow

1. **Platform Engineer**: Creates APIClass with external IdP configuration
2. **Application Developer**: References APIClass in Service resource
3. **ServiceBinding Controller**: Generates gateway configurations with IdP settings
4. **Data Plane Gateway**: Enforces authentication using external IdP for API requests

---
