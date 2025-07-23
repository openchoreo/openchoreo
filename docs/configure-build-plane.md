# Configuring the Build Plane in OpenChoreo

This guide walks you through setting up a **Build Plane** in OpenChoreo.

## Overview

An organization can have only **one Build Plane**, which is responsible for executing build workloads. These workloads run in a dedicated namespace named: `openchoreo-ci-<your-org>`

> [!NOTE]
> `BuildPlane` CRD support is on the roadmap and will be available in a future release. Currently, the Build Plane runs within a **Data Plane** cluster.

## Using a DataPlane as the Build Plane

Until native support for the `BuildPlane` custom resource is available, you can designate a `DataPlane` to act as the Build Plane.

### Steps

1. Ensure Argo Workflows is installed in the target DataPlane cluster.
2. Add the following label to the `DataPlane` resource:

   ```yaml
   openchoreo.dev/build-plane: "true"
   ```

> [!IMPORTANT]
> You must configure only one DataPlane as the Build Plane.
