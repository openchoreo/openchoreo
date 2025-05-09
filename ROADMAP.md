# Roadmap

This roadmap outlines high-level features and improvements for the OpenChoreo project.  
It does **not** represent all the ongoing work by contributors and is subject to change based on community feedback and priorities.

## 2025 Q2 (April – June)

### Introduce Developer Abstraction with `openchoreo.yaml`

Define a developer-friendly configuration file (`openchoreo.yaml`) to describe application endpoints, dependencies, and deployment configurations.  
This file will act as the central contract between developers and the platform, enabling automation through GitOps workflows.

### Plugin for Backstage

Develop a custom Backstage plugin to integrate OpenChoreo into internal developer portals.  

### New UI for OpenChoreo

Introduce a dedicated web interface for managing OpenChoreo resources.

### Runtime Observability

Enable observability features for deployed components, helping users view logs and metrics.

### API Management Support

Add minimal support for exposing and securing APIs through OpenChoreo-managed gateways.
Initial features will focus on simple authentication models such as API Keys and OAuth2.0 token validation. This includes the ability to define security policies in `openchoreo.yaml`, generate necessary CRDs, and configure API gateways in the data plane.

### Control Plane and Data Plane Separation

Split the current single cluster architecture into separate control plane (CP) and data plane (DP) clusters.
