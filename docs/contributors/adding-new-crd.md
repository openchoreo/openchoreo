
# Adding New CRD Resources

This guide explains how to add new Custom Resource Definitions (CRDs) to the project using **Kubebuilder** and how to refactor the generated controller into a dedicated package for better project organization.

## Steps to Add a New CRD Resource

### 1. **Generate the CRD with Kubebuilder**

Run the following command to scaffold a new API group, version, and kind:
```bash
kubebuilder create api --group <group_name> --version <version> --kind <kind_name>
```

Replace `<group_name>`, `<version>`, and `<kind_name>` with the appropriate values for your resource. For example:
```bash
kubebuilder create api --group core --version v1 --kind Component
```

This generates:
- **API types** under `api/<version>/`
- **Controller files** under `controller/`:
    - `<kind_name>_controller.go`
    - `<kind_name>_controller_test.go`
    - `suite_test.go`

### 2. **Edit the API Types**

Modify the API types in `api/<version>/<kind_name>_types.go` as needed. Define your resource's fields and annotations.

### 3. **Run Code Generators**

Run the following command to generate code for deep copies, CRD manifests, and other boilerplate:

```bash
make generate
make manifests
```

### 4. **Register the New Controller**

By default, the generated controller is already registered in `main.go`. You’ll refactor this registration in the next steps.

---

## Steps to Refactor the Controller

### 1. **Move the Controller Files**

Manually move the generated controller and test files to a dedicated package under `controller/<kind_name>/`. 
Change the name of the controller file from `<kind_name>_controller.go` to `controller.go`.

For example:
```plaintext
controller/
├── component/
│   ├── controller.go
│   ├── controller_test.go
│   └── suite_test.go
```

### 2. **Update Package Declarations**

Update the package declaration at the top of the moved files. Change it to match the new location. For example:
```go
package component
```

### 3. **Update Struct Name**

Update the struct name from `<Kind>Reconciler` to `Reconciler` in controller file. For example:

```go
type Reconciler struct {
    // Add fields here
}
```

### 4. **Update `suite_test.go`**

The `suite_test.go` file contains the setup of test environments which requires CRDs and Kubernetes binaries path.
The relative paths of this file need to be updated to reflect the new location of the controller.

Add extra `".."` to the file path.

```go
By("bootstrapping test environment")
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
    ErrorIfCRDPathMissing: true,

    // The BinaryAssetsDirectory is only required if you want to run the tests directly
    // without call the makefile target test. If not informed it will look for the
    // default path defined in controller-runtime which is /usr/local/kubebuilder/.
    // Note that you must have the required binaries setup under the bin directory to perform
    // the tests directly. When we run make test it will be setup and used automatically.
    BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
        fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
}
```

### 5. **Update Imports in `main.go`**

In `main.go`, update the import path for the moved controller and ensure it’s registered with the manager. For example:
```go
import (
	component "github.com/openchoreo/openchoreo/internal/controller/component"
)

// Register the controller
if err := (&component.Reconciler{
	Client: mgr.GetClient(),
    Scheme: mgr.GetScheme(),
}).SetupWithManager(mgr); err != nil {
	setupLog.Error(err, "unable to create controller", "controller", "Component")
	os.Exit(1)
}
```

### 4. **Test the Refactored Controller**

Run the following to verify that your changes are working as expected:

```bash
make test
```

## Update the helm chart 

Run the following to update the helm chart for your generated CRD.

```bash
make helm-generate
```
