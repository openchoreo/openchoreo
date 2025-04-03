## Contributor Guide

This section provides a comprehensive guide for contributors to set up their development environment, build and use the binaries, and deploy Choreo on a Kubernetes cluster for testing and development purposes.

### Prerequisites for Contributors
- Go version v1.23.0+
- Docker version 17.03+
- Kubernetes cluster with version v1.30.0+

### Build and Use Binaries

1. Clone the repository:
   ```sh
   git clone https://github.com/<org>/openchoreo.git
   cd openchoreo
   ```

2. Build the binaries:
   ```sh
   make build
   ```

3. Run the binaries:
   ```sh
   ./bin/manager
   ```

4. Follow the deployment steps mentioned below under "To Deploy on the cluster" section.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/choreo:tag
```

> [!Note] 
> This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/choreo:tag
```

> [!Note] 
> If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Code Generation and Linting

After updating the Custom Resource Definitions (CRDs) or the controller code, run the following commands to generate necessary code and lint the codebase before committing the changes.

1. Run the linter:
    ```sh
    make lint
    ```
2. Run the code generator:
    ```sh
    make code.gen
    ```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/choreo:tag
```

> [!Note] 
> The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/choreo/<tag or branch>/dist/install.yaml
```
### Implement Custom Resources
> [!Note] 
>  Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
