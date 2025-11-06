// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

func TestPipeline_Render(t *testing.T) {
	devEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: dev
      namespace: test-namespace
    spec:
      dataPlaneRef: dev-dataplane
      isProduction: false
      gateway:
        dnsPrefix: dev
        security:
          remoteJwks:
            uri: https://auth.example.com/.well-known/jwks.json`
	devDataplaneYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: DataPlane
    metadata:
      name: dev-dataplane
      namespace: test-namespace
    spec:
      kubernetesCluster:
        name: development-cluster
        credentials:
          apiServerURL: https://k8s-api.example.com:6443
          caCert: LS0tLS1CRUdJTi
          clientCert: LS0tLS1CRUdJTi
          clientKey: LS0tLS1CRUdJTi
      registry:
        prefix: docker.io/myorg
        secretRef: registry-credentials
      gateway:
        publicVirtualHost: api.example.com
        organizationVirtualHost: internal.example.com
      observer:
        url: https://observer.example.com
        authentication:
          basicAuth:
            username: admin
            password: secretpassword`
	prodEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: prod
      namespace: test-namespace
    spec:
      dataPlaneRef: prod-dataplane
      isProduction: true
      gateway:
        dnsPrefix: prod
        security:
          remoteJwks:
            uri: https://auth.example.com/.well-known/jwks.json
  `
	prodDataplaneYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: DataPlane
    metadata:
      name: production-dataplane
      namespace: test-namespace
    spec:
      kubernetesCluster:
        name: production-cluster
        credentials:
          apiServerURL: https://k8s-api.example.com:6443
          caCert: LS0tLS1CRUdJTi
          clientCert: LS0tLS1CRUdJTi
          clientKey: LS0tLS1CRUdJTi
      registry:
        prefix: docker.io/myorg
        secretRef: registry-credentials
      gateway:
        publicVirtualHost: api.example.com
        organizationVirtualHost: internal.example.com
      observer:
        url: https://observer.example.com
        authentication:
          basicAuth:
            username: admin
            password: secretpassword
      secretStoreRef:
        name: prod-vault-store
  `
	tests := []struct {
		name                 string
		snapshotYAML         string
		settingsYAML         string
		wantErr              bool
		wantResourceYAML     string
		environmentYAML      string
		dataplaneYAML        string
		secretReferencesYAML string
	}{
		{
			name: "simple component without traits",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    replicas: 2
`,
			wantErr: false,
		},
		{
			name: "component with environment overrides",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
  workload: {}
`,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  overrides:
    replicas: 5
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 5
`,
			wantErr: false,
		},
		{
			name: "component with includeWhen",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        expose: true
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
        - id: service
          includeWhen: ${parameters.expose}
          template:
            apiVersion: v1
            kind: Service
            metadata:
              name: ${component.name}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Service
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
			wantErr: false,
		},
		{
			name: "component with forEach",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        secrets:
          - secret1
          - secret2
  componentType:
    spec:
      resources:
        - id: secrets
          forEach: ${parameters.secrets}
          var: secret
          template:
            apiVersion: v1
            kind: Secret
            metadata:
              name: ${secret}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret1
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret2
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
			wantErr: false,
		},
		{
			name: "component with trait creates",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
      traits:
        - name: mysql
          instanceName: db-1
          parameters:
            database: mydb
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
  traits:
    - metadata:
        name: mysql
      spec:
        creates:
          - template:
              apiVersion: v1
              kind: Secret
              metadata:
                name: ${trait.instanceName}-secret
              data:
                database: ${parameters.database}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  data:
    database: mydb
`,
			wantErr: false,
		},
		{
			name: "component with trait patches",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
      traits:
        - name: monitoring
          instanceName: mon-1
          config: {}
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
            spec:
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
  traits:
    - metadata:
        name: monitoring
      spec:
        patches:
          - target:
              kind: Deployment
              group: apps
              version: v1
            operations:
              - op: add
                path: /metadata/labels
                value:
                  monitoring: enabled
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      monitoring: enabled
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
`,
			wantErr: false,
		},
		{
			name: "component with env configurations",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      envFrom: |
                        ${(has(configurations.configs.envs) && configurations.configs.envs.size() > 0 ?
                          [{
                            "configMapRef": {
                              "name": oc_generate_name(metadata.name, "env-configs")
                            }
                          }] : [])}
        - id: env-config
          includeWhen: ${has(configurations.configs.envs) && configurations.configs.envs.size() > 0}
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "env-configs")}
            data: |
              ${has(configurations.configs.envs) ? configurations.configs.envs.transformMapEntry(index, env, {env.name: env.value}) : oc_omit()}
  workload:
    spec:
      containers:
        main:
          image: myapp:latest
          env:
            - key: LOG_LEVEL
              value: info
            - key: DEBUG_MODE
              value: "true"
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-env-configs-3e553e36
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  data:
    LOG_LEVEL: info
    DEBUG_MODE: "true"
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            envFrom:
              - configMapRef:
                  name: test-component-dev-12345678-env-configs-3e553e36
`,
			wantErr: false,
		},
		{
			name: "component with env configurations override",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      envFrom: |
                        ${(has(configurations.configs.envs) && configurations.configs.envs.size() > 0 ?
                          [{
                            "configMapRef": {
                              "name": oc_generate_name(metadata.name, "env-configs")
                            }
                          }] : [])}
        - id: env-config
          includeWhen: ${has(configurations.configs.envs) && configurations.configs.envs.size() > 0}
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "env-configs")}
            data: |
              ${has(configurations.configs.envs) ? configurations.configs.envs.transformMapEntry(index, env, {env.name: env.value}) : oc_omit()}
  workload:
    spec:
      containers:
        main:
          image: myapp:latest
          env:
            - key: LOG_LEVEL
              value: info
            - key: DEBUG_MODE
              value: "true"
`,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  configurationOverrides:
    env:
      - key: LOG_LEVEL
        value: error
      - key: NEW_KEY
        value: newValue
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-env-configs-3e553e36
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  data:
    LOG_LEVEL: error
    DEBUG_MODE: "true"
    NEW_KEY: newValue
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            envFrom:
              - configMapRef:
                  name: test-component-dev-12345678-env-configs-3e553e36
`,
			wantErr: false,
		},
		{
			name: "component with file configurations override",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      volumeMounts: |
                        ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                          configurations.configs.files.map(f, {
                            "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                            "mountPath": f.mountPath+"/"+f.name,
                            "subPath": f.name
                          }) : oc_omit()}
                  volumes: |
                    ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                      configurations.configs.files.map(f, {
                        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                        "configMap": {
                          "name": oc_generate_name(metadata.name, "config", f.name).replace(".", "-")
                        }
                      }) : oc_omit()}
        - id: file-config
          includeWhen: ${has(configurations.configs.files) && configurations.configs.files.size() > 0}
          forEach: ${configurations.configs.files}
          var: config
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "config", config.name).replace(".", "-")}
              namespace: ${metadata.namespace}
            data:
              ${config.name}: |
                ${config.value}
  workload:
    spec:
      containers:
        main:
          image: myapp:latest
          files:
            - key: config.json
              value: |
                {
                  "database": {
                    "host": "localhost",
                    "port": 5432
                  }
                }
              mountPath: /etc/config
            - key: app.properties
              value: |
                app.name=myapp
                app.version=1.0.0
                log.level=INFO
              mountPath: /etc/config
`,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  configurationOverrides:
    files:
      - key: config.json
        value: |
          {
            "database": {
              "host": "prod.db.example.com",
              "port": 5432
            }
          }
        mountPath: /etc/config
      - key: new-config.yaml
        value: |
          apiVersion: v1
          kind: Config
          setting: production
        mountPath: /etc/config
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-config-app-properties-7a40d758
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  data:
    app.properties: |
      app.name=myapp
      app.version=1.0.0
      log.level=INFO
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-config-config-json-4334abe4
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  data:
    config.json: |
      {
        "database": {
          "host": "prod.db.example.com",
          "port": 5432
        }
      }
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-config-new-config-yaml-0fbbcd4a
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  data:
    new-config.yaml: |
      apiVersion: v1
      kind: Config
      setting: production
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            volumeMounts:
              - name: file-mount-d08babc2
                mountPath: /etc/config/app.properties
                subPath: app.properties
              - name: file-mount-6c698306
                mountPath: /etc/config/config.json
                subPath: config.json
              - name: file-mount-bc372c14
                mountPath: /etc/config/new-config.yaml
                subPath: new-config.yaml
        volumes:
          - name: file-mount-bc372c14
            configMap:
              name: test-component-dev-12345678-config-new-config-yaml-0fbbcd4a
          - name: file-mount-6c698306
            configMap:
              name: test-component-dev-12345678-config-config-json-4334abe4
          - name: file-mount-d08babc2
            configMap:
              name: test-component-dev-12345678-config-app-properties-7a40d758
`,
			wantErr: false,
		},
		{
			name: "component with file configurations",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      volumeMounts: |
                        ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                          configurations.configs.files.map(f, {
                            "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                            "mountPath": f.mountPath+"/"+f.name,
                            "subPath": f.name
                          }) : oc_omit()}
                  volumes: |
                    ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                      configurations.configs.files.map(f, {
                        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                        "configMap": {
                          "name": oc_generate_name(metadata.name, "config", f.name).replace(".", "-")
                        }
                      }) : oc_omit()}
        - id: file-config
          includeWhen: ${has(configurations.configs.files) && configurations.configs.files.size() > 0}
          forEach: ${configurations.configs.files}
          var: config
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "config", config.name).replace(".", "-")}
              namespace: ${metadata.namespace}
            data:
              ${config.name}: |
                ${config.value}
  workload:
    spec:
      containers:
        main:
          image: myapp:latest
          files:
            - key: config.json
              value: |
                {
                  "database": {
                    "host": "localhost",
                    "port": 5432
                  }
                }
              mountPath: /etc/config
            - key: app.properties
              value: |
                app.name=myapp
                app.version=1.0.0
                log.level=INFO
              mountPath: /etc/config
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-config-app-properties-7a40d758
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  data:
    app.properties: |
      app.name=myapp
      app.version=1.0.0
      log.level=INFO
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test-component-dev-12345678-config-config-json-4334abe4
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  data:
    config.json: |
      {
        "database": {
          "host": "localhost",
          "port": 5432
        }
      }
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            volumeMounts:
              - name: file-mount-d08babc2
                mountPath: /etc/config/app.properties
                subPath: app.properties
              - name: file-mount-6c698306
                mountPath: /etc/config/config.json
                subPath: config.json
        volumes:
          - name: file-mount-6c698306
            configMap:
              name: test-component-dev-12345678-config-config-json-4334abe4
          - name: file-mount-d08babc2
            configMap:
              name: test-component-dev-12345678-config-app-properties-7a40d758
`,
			wantErr: false,
		},
		{
			name: "component with env and file secrets",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      envFrom: |
                        ${has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0 ?
                          [{
                            "secretRef": {
                              "name": oc_generate_name(metadata.name, "env-secrets")
                            }
                          }] : []}
                      volumeMounts: |
                        ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
                          configurations.secrets.files.map(f, {
                            "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                            "mountPath": f.mountPath+"/"+f.name,
                            "subPath": f.name
                          }) : oc_omit()}
                  volumes: |
                    ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
                      configurations.secrets.files.map(f, {
                        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                        "secret": {
                          "secretName": oc_generate_name(metadata.name, "secret", f.name).replace(".", "-")
                        }
                      }) : oc_omit()}
        - id: secret-env-external
          includeWhen: ${has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0}
          template:
            apiVersion: external-secrets.io/v1
            kind: ExternalSecret
            metadata:
              name: ${oc_generate_name(metadata.name, "env-secrets")}
              namespace: ${metadata.namespace}
            spec:
              refreshInterval: 15s
              secretStoreRef:
                name: ${dataplane.secretStore}
                kind: ClusterSecretStore
              target:
                name: ${oc_generate_name(metadata.name, "env-secrets")}
                creationPolicy: Owner
              data: |
                ${has(configurations.secrets.envs) ? configurations.secrets.envs.map(secret, {
                  "secretKey": secret.name,
                  "remoteRef": {
                    "key": secret.remoteRef.key,
                    "property": secret.remoteRef.property
                  }
                }) : oc_omit()}
        - id: secret-file-external
          includeWhen: ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0}
          forEach: ${configurations.secrets.files}
          var: file
          template:
            apiVersion: external-secrets.io/v1
            kind: ExternalSecret
            metadata:
              name: ${oc_generate_name(metadata.name, "secret", file.name).replace(".", "-")}
              namespace: ${metadata.namespace}
            spec:
              refreshInterval: 15s
              secretStoreRef:
                name: ${dataplane.secretStore}
                kind: ClusterSecretStore
              target:
                name: ${oc_generate_name(metadata.name, "secret", file.name).replace(".", "-")}
                creationPolicy: Owner
              data:
                - secretKey: ${file.name}
                  remoteRef:
                    key: ${file.remoteRef.key}
                    property: ${file.remoteRef.property}
  workload:
    spec:
      containers:
        app:
          image: myapp:latest
          env:
            - key: DB_PASSWORD
              valueFrom:
                secretRef:
                  name: db-secret
                  key: password
            - key: API_KEY
              valueFrom:
                secretRef:
                  name: api-secret
                  key: api_key
          files:
            - key: tls.crt
              mountPath: /etc/tls
              valueFrom:
                secretRef:
                  name: tls-secret
                  key: tls.crt
            - key: application.yaml
              mountPath: /etc/config
              valueFrom:
                secretRef:
                  name: app-config-secret
                  key: application.yaml
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			secretReferencesYAML: `
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: db-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: password
        remoteRef:
          key: prod/db
          property: password
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: api-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: api_key
        remoteRef:
          key: prod/api
          property: api_key
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: tls-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: tls.crt
        remoteRef:
          key: prod/certificates
          property: tls.crt
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: app-config-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: application.yaml
        remoteRef:
          key: prod/config
          property: application.yaml
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 2
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            envFrom:
              - secretRef:
                  name: test-component-dev-12345678-env-secrets-7d163eae
            volumeMounts:
              - name: file-mount-5953ef7b
                mountPath: /etc/config/application.yaml
                subPath: application.yaml
              - name: file-mount-9b2ef275
                mountPath: /etc/tls/tls.crt
                subPath: tls.crt
        volumes:
          - name: file-mount-5953ef7b
            secret:
              secretName: test-component-dev-12345678-secret-application-yaml-f2042975
          - name: file-mount-9b2ef275
            secret:
              secretName: test-component-dev-12345678-secret-tls-crt-baf3eb48
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-env-secrets-7d163eae
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-env-secrets-7d163eae
      creationPolicy: Owner
    data:
      - secretKey: DB_PASSWORD
        remoteRef:
          key: prod/db
          property: password
      - secretKey: API_KEY
        remoteRef:
          key: prod/api
          property: api_key
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-secret-application-yaml-f2042975
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-secret-application-yaml-f2042975
      creationPolicy: Owner
    data:
      - secretKey: application.yaml
        remoteRef:
          key: prod/config
          property: application.yaml
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-secret-tls-crt-baf3eb48
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-secret-tls-crt-baf3eb48
      creationPolicy: Owner
    data:
      - secretKey: tls.crt
        remoteRef:
          key: prod/certificates
          property: tls.crt
`,
			wantErr: false,
		},
		{
			name: "component with secret overrides",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      envFrom: |
                        ${has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0 ?
                          [{
                            "secretRef": {
                              "name": oc_generate_name(metadata.name, "env-secrets")
                            }
                          }] : []}
                      volumeMounts: |
                        ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
                          configurations.secrets.files.map(f, {
                            "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                            "mountPath": f.mountPath+"/"+f.name,
                            "subPath": f.name
                          }) : oc_omit()}
                  volumes: |
                    ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
                      configurations.secrets.files.map(f, {
                        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                        "secret": {
                          "secretName": oc_generate_name(metadata.name, "secret", f.name).replace(".", "-")
                        }
                      }) : oc_omit()}
        - id: secret-env-external
          includeWhen: ${has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0}
          template:
            apiVersion: external-secrets.io/v1
            kind: ExternalSecret
            metadata:
              name: ${oc_generate_name(metadata.name, "env-secrets")}
              namespace: ${metadata.namespace}
            spec:
              refreshInterval: 15s
              secretStoreRef:
                name: ${dataplane.secretStore}
                kind: ClusterSecretStore
              target:
                name: ${oc_generate_name(metadata.name, "env-secrets")}
                creationPolicy: Owner
              data: |
                ${has(configurations.secrets.envs) ? configurations.secrets.envs.map(secret, {
                  "secretKey": secret.name,
                  "remoteRef": {
                    "key": secret.remoteRef.key,
                    "property": secret.remoteRef.property
                  }
                }) : oc_omit()}
        - id: secret-file-external
          includeWhen: ${has(configurations.secrets.files) && configurations.secrets.files.size() > 0}
          forEach: ${configurations.secrets.files}
          var: file
          template:
            apiVersion: external-secrets.io/v1
            kind: ExternalSecret
            metadata:
              name: ${oc_generate_name(metadata.name, "secret", file.name).replace(".", "-")}
              namespace: ${metadata.namespace}
            spec:
              refreshInterval: 15s
              secretStoreRef:
                name: ${dataplane.secretStore}
                kind: ClusterSecretStore
              target:
                name: ${oc_generate_name(metadata.name, "secret", file.name).replace(".", "-")}
                creationPolicy: Owner
              data:
                - secretKey: ${file.name}
                  remoteRef:
                    key: ${file.remoteRef.key}
                    property: ${file.remoteRef.property}
  workload:
    spec:
      containers:
        app:
          image: myapp:latest
          env:
            - key: DATABASE_PASSWORD
              valueFrom:
                secretRef:
                  name: database-dev-secret
                  key: password
          files:
            - key: config.yaml
              mountPath: /etc/config
              valueFrom:
                secretRef:
                  name: config-dev-secret
                  key: config.yaml
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			secretReferencesYAML: `
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: database-dev-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: password
        remoteRef:
          key: database/dev
          property: password
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: database-prod-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: password
        remoteRef:
          key: database/prod
          property: password
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: api-prod-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: token
        remoteRef:
          key: api/prod
          property: token
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: config-dev-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: config.yaml
        remoteRef:
          key: config/dev
          property: config.yaml
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: config-prod-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: config.yaml
        remoteRef:
          key: config/prod
          property: config.yaml
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: redis-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: url
        remoteRef:
          key: redis/prod
          property: url
- apiVersion: openchoreo.dev/v1alpha1
  kind: SecretReference
  metadata:
    name: monitoring-secret
  spec:
    template:
      type: Opaque
    data:
      - secretKey: monitoring.yaml
        remoteRef:
          key: monitoring/prod
          property: monitoring.yaml
`,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  configurationOverrides:
    env:
      - key: DATABASE_PASSWORD
        valueFrom:
          secretRef:
            name: database-prod-secret
            key: password
      - key: API_TOKEN
        valueFrom:
          secretRef:
            name: api-prod-secret
            key: token
      - key: REDIS_URL
        valueFrom:
          secretRef:
            name: redis-secret
            key: url
    files:
      - key: config.yaml
        mountPath: /etc/config
        valueFrom:
          secretRef:
            name: config-prod-secret
            key: config.yaml
      - key: monitoring.yaml
        mountPath: /etc/monitoring
        valueFrom:
          secretRef:
            name: monitoring-secret
            key: monitoring.yaml
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            envFrom:
              - secretRef:
                  name: test-component-dev-12345678-env-secrets-7d163eae
            volumeMounts:
              - name: file-mount-8bf9a3a3
                mountPath: /etc/config/config.yaml
                subPath: config.yaml
              - name: file-mount-8dcd70ef
                mountPath: /etc/monitoring/monitoring.yaml
                subPath: monitoring.yaml
        volumes:
          - name: file-mount-8bf9a3a3
            secret:
              secretName: test-component-dev-12345678-secret-config-yaml-90b39a2c
          - name: file-mount-8dcd70ef
            secret:
              secretName: test-component-dev-12345678-secret-monitoring-yaml-ecfe7563
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-env-secrets-7d163eae
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-env-secrets-7d163eae
      creationPolicy: Owner
    data:
      - secretKey: API_TOKEN
        remoteRef:
          key: api/prod
          property: token
      - secretKey: REDIS_URL
        remoteRef:
          key: redis/prod
          property: url
      - secretKey: DATABASE_PASSWORD
        remoteRef:
          key: database/prod
          property: password
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-secret-config-yaml-90b39a2c
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-secret-config-yaml-90b39a2c
      creationPolicy: Owner
    data:
      - secretKey: config.yaml
        remoteRef:
          key: config/prod
          property: config.yaml
- apiVersion: external-secrets.io/v1
  kind: ExternalSecret
  metadata:
    name: test-component-dev-12345678-secret-monitoring-yaml-ecfe7563
    namespace: test-namespace
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    refreshInterval: 15s
    secretStoreRef:
      name: prod-vault-store
      kind: ClusterSecretStore
    target:
      name: test-component-dev-12345678-secret-monitoring-yaml-ecfe7563
      creationPolicy: Owner
    data:
      - secretKey: monitoring.yaml
        remoteRef:
          key: monitoring/prod
          property: monitoring.yaml
`,
			wantErr: false,
		},
		{
			name: "component with no workload configurations and no config overrides",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 1
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
                      envFrom: |
                        ${(has(configurations.configs.envs) && configurations.configs.envs.size() > 0 ?
                          [{
                            "configMapRef": {
                              "name": oc_generate_name(metadata.name, "env-configs")
                            }
                          }] : [])}
                      volumeMounts: |
                        ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                          configurations.configs.files.map(f, {
                            "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                            "mountPath": f.mountPath+"/"+f.name,
                            "subPath": f.name
                          }) : oc_omit()}
                  volumes: |
                    ${has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
                      configurations.configs.files.map(f, {
                        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
                        "configMap": {
                          "name": oc_generate_name(metadata.name, "config", f.name).replace(".", "-")
                        }
                      }) : oc_omit()}
        - id: env-config
          includeWhen: ${has(configurations.configs.envs) && configurations.configs.envs.size() > 0}
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "env-configs")}
            data: |
              ${has(configurations.configs.envs) ? configurations.configs.envs.transformMapEntry(index, env, {env.name: env.value}) : oc_omit()}
        - id: file-config
          includeWhen: ${has(configurations.configs.files) && configurations.configs.files.size() > 0}
          forEach: ${configurations.configs.files}
          var: config
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: ${oc_generate_name(metadata.name, "config", config.name).replace(".", "-")}
              namespace: ${metadata.namespace}
            data:
              ${config.name}: |
                ${config.value}
  workload:
    spec:
      containers:
        app:
          image: myapp:latest
`,
			environmentYAML: prodEnvironmentYAML,
			dataplaneYAML:   prodDataplaneYAML,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  configurationOverrides: {}
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 1
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            envFrom: []
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot
			snapshot := &v1alpha1.ComponentEnvSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Parse settings if provided
			var settings *v1alpha1.ComponentDeployment
			if tt.settingsYAML != "" {
				settings = &v1alpha1.ComponentDeployment{}
				if err := yaml.Unmarshal([]byte(tt.settingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse settings YAML: %v", err)
				}
			}

			// Parse environment
			var environment *v1alpha1.Environment
			if tt.environmentYAML != "" {
				environment = &v1alpha1.Environment{}
				if err := yaml.Unmarshal([]byte(tt.environmentYAML), environment); err != nil {
					t.Fatalf("Failed to parse environment YAML: %v", err)
				}
			}

			// Parse dataplane
			var dataplane *v1alpha1.DataPlane
			if tt.dataplaneYAML != "" {
				dataplane = &v1alpha1.DataPlane{}
				if err := yaml.Unmarshal([]byte(tt.dataplaneYAML), dataplane); err != nil {
					t.Fatalf("Failed to parse dataplane YAML: %v", err)
				}
			}

			// Parse secret references if provided
			var secretReferences map[string]*v1alpha1.SecretReference
			if tt.secretReferencesYAML != "" {
				var refs []v1alpha1.SecretReference
				if err := yaml.Unmarshal([]byte(tt.secretReferencesYAML), &refs); err != nil {
					t.Fatalf("Failed to parse secretReferences YAML: %v", err)
				}
				secretReferences = make(map[string]*v1alpha1.SecretReference)
				for i := range refs {
					secretReferences[refs[i].Name] = &refs[i]
				}
			}

			// Create input
			input := &RenderInput{
				ComponentType:       &snapshot.Spec.ComponentType,
				Component:           &snapshot.Spec.Component,
				Traits:              snapshot.Spec.Traits,
				Workload:            &snapshot.Spec.Workload,
				Environment:         environment,
				DataPlane:           dataplane,
				ComponentDeployment: settings,
				SecretReferences:    secretReferences,
				Metadata: context.MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"openchoreo.org/component":   "test-component",
						"openchoreo.org/environment": "dev",
					},
				},
			}

			// Create pipeline and render
			pipeline := NewPipeline()
			output, err := pipeline.Render(input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.wantResourceYAML != "" {
				// Parse expected resources
				var wantResources []map[string]any
				if err := yaml.Unmarshal([]byte(tt.wantResourceYAML), &wantResources); err != nil {
					t.Fatalf("Failed to parse wantResourceYAML: %v", err)
				}

				if diff := cmp.Diff(wantResources, output.Resources, sortSlicesByName(), sortAnySlicesByName()); diff != "" {
					t.Errorf("Resources mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestPipeline_Options(t *testing.T) {
	devEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: dev
      namespace: test-namespace
    spec:
      dataPlaneRef: dev-dataplane
      isProduction: false
      gateway:
        dnsPrefix: dev
        security:
          remoteJwks:
            uri: https://auth.example.com/.well-known/jwks.json`
	devDataplaneYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: DataPlane
    metadata:
      name: dev-dataplane
      namespace: test-namespace
    spec:
      kubernetesCluster:
        name: development-cluster
        credentials:
          apiServerURL: https://k8s-api.example.com:6443
          caCert: LS0tLS1CRUdJTi
          clientCert: LS0tLS1CRUdJTi
          clientKey: LS0tLS1CRUdJTi
      registry:
        prefix: docker.io/myorg
        secretRef: registry-credentials
      gateway:
        publicVirtualHost: api.example.com
        organizationVirtualHost: internal.example.com
      observer:
        url: https://observer.example.com
        authentication:
          basicAuth:
            username: admin
            password: secretpassword`
	tests := []struct {
		name             string
		snapshotYAML     string
		options          []Option
		wantResourceYAML string
		environmentYAML  string
		dataplaneYAML    string
	}{
		{
			name: "with custom labels",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			options: []Option{
				WithResourceLabels(map[string]string{
					"custom": "label",
				}),
			},
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      custom: label
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
		},
		{
			name: "with custom annotations",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			options: []Option{
				WithResourceAnnotations(map[string]string{
					"custom": "annotation",
				}),
			},
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
    annotations:
      custom: annotation
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot
			snapshot := &v1alpha1.ComponentEnvSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Parse environment
			var environment *v1alpha1.Environment
			if tt.environmentYAML != "" {
				environment = &v1alpha1.Environment{}
				if err := yaml.Unmarshal([]byte(tt.environmentYAML), environment); err != nil {
					t.Fatalf("Failed to parse environment YAML: %v", err)
				}
			}

			// Parse dataplane
			var dataplane *v1alpha1.DataPlane
			if tt.dataplaneYAML != "" {
				dataplane = &v1alpha1.DataPlane{}
				if err := yaml.Unmarshal([]byte(tt.dataplaneYAML), dataplane); err != nil {
					t.Fatalf("Failed to parse dataplane YAML: %v", err)
				}
			}

			// Create input
			input := &RenderInput{
				ComponentType: &snapshot.Spec.ComponentType,
				Component:     &snapshot.Spec.Component,
				Traits:        snapshot.Spec.Traits,
				Workload:      &snapshot.Spec.Workload,
				Environment:   environment,
				DataPlane:     dataplane,
				Metadata: context.MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"openchoreo.org/component":   "test-component",
						"openchoreo.org/environment": "dev",
					},
				},
			}

			// Create pipeline with options
			pipeline := NewPipeline(tt.options...)
			output, err := pipeline.Render(input)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			// Parse expected resources
			var wantResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.wantResourceYAML), &wantResources); err != nil {
				t.Fatalf("Failed to parse wantResourceYAML: %v", err)
			}

			// Compare actual vs expected
			if diff := cmp.Diff(wantResources, output.Resources); diff != "" {
				t.Errorf("Resources mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	tests := []struct {
		name      string
		resources []map[string]any
		wantErr   bool
	}{
		{
			name: "valid resources",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			resources: []map[string]any{
				{
					"kind": "Pod",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing kind",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing metadata.name",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata":   map[string]any{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			err := p.validateResources(tt.resources)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSortResources(t *testing.T) {
	resources := []map[string]any{
		{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "svc-b",
			},
		},
		{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"metadata": map[string]any{
				"name": "deploy-a",
			},
		},
		{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "svc-a",
			},
		},
	}

	sortResources(resources)

	// Check sorted order: Deployment first, then Services sorted by name
	if resources[0]["kind"] != "Deployment" {
		t.Errorf("Expected Deployment first, got %v", resources[0]["kind"])
	}
	if resources[1]["kind"] != "Service" {
		t.Errorf("Expected Service second, got %v", resources[1]["kind"])
	}

	metadata := resources[1]["metadata"].(map[string]any)
	if metadata["name"] != "svc-a" {
		t.Errorf("Expected svc-a second, got %v", metadata["name"])
	}
}

// sortSlicesByName returns a cmp.Transformer to sort slices of maps with "name" or "secretKey" field during comparison.
// Configuration override merging uses maps which have non-deterministic iteration order.
func sortSlicesByName() cmp.Option {
	return cmp.Transformer("SortSlicesByName", func(in []map[string]any) []map[string]any {
		// Check if any map has a "name" or "secretKey" field
		hasKey := false
		for _, m := range in {
			if _, exists := m["name"]; exists {
				hasKey = true
				break
			}
			if _, exists := m["secretKey"]; exists {
				hasKey = true
				break
			}
		}

		// If no relevant field, return as-is
		if !hasKey {
			return in
		}

		// Helper to extract sort key (prefer "name" over "secretKey" if both exist)
		getKey := func(m map[string]any) (string, bool) {
			if v, ok := m["name"].(string); ok && v != "" {
				return v, true
			}
			if v, ok := m["secretKey"].(string); ok && v != "" {
				return v, true
			}
			return "", false
		}

		// Create a copy and sort by key
		out := make([]map[string]any, len(in))
		copy(out, in)
		sort.SliceStable(out, func(i, j int) bool {
			ki, iok := getKey(out[i])
			kj, jok := getKey(out[j])

			// Both missing keys -> preserve original order
			if !iok && !jok {
				return false
			}
			// i missing, j has -> j should come before i => return false
			if !iok && jok {
				return false
			}
			// i has, j missing -> i should come before j
			if iok && !jok {
				return true
			}
			// Both have keys -> compare lexicographically
			return ki < kj
		})
		return out
	})
}

// sortAnySlicesByName returns a cmp.Transformer to handle []any slices that contain maps with "name" or "secretKey" field.
func sortAnySlicesByName() cmp.Option {
	return cmp.Transformer("SortAnySlicesByName", func(in []any) []any {
		// Check if this is a slice of maps with "name" or "secretKey" field
		if len(in) == 0 {
			return in
		}

		firstMap, ok := in[0].(map[string]any)
		if !ok {
			return in
		}

		if _, hasName := firstMap["name"]; !hasName {
			if _, hasSecretKey := firstMap["secretKey"]; !hasSecretKey {
				return in
			}
		}

		// Helper to extract key from an any element (map[string]any)
		getKeyAny := func(x any) (string, bool) {
			m, ok := x.(map[string]any)
			if !ok {
				return "", false
			}
			if v, ok := m["name"].(string); ok && v != "" {
				return v, true
			}
			if v, ok := m["secretKey"].(string); ok && v != "" {
				return v, true
			}
			return "", false
		}

		// Create a copy and sort by key
		out := make([]any, len(in))
		copy(out, in)
		sort.SliceStable(out, func(i, j int) bool {
			ki, iok := getKeyAny(out[i])
			kj, jok := getKeyAny(out[j])

			// Both missing keys -> preserve original order
			if !iok && !jok {
				return false
			}
			// i missing, j has -> j should come before i => return false
			if !iok && jok {
				return false
			}
			// i has, j missing -> i should come before j
			if iok && !jok {
				return true
			}
			// Both have keys -> compare lexicographically
			return ki < kj
		})
		return out
	})
}
