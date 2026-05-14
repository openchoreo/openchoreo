// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Secret implements user-workload secret operations. Read paths (list, get)
// use the SecretReference API and surface only references that target a plane.
// Write paths (create, delete) call the secret API which provisions or removes
// the secret on the target plane's external secret store.
type Secret struct {
	client client.Interface
}

// New creates a new Secret implementation.
func New(c client.Interface) *Secret {
	return &Secret{client: c}
}

// List lists secrets in a namespace via the Secret API. It also fetches
// SecretReferences in the same namespace to enrich each row with the secret's
// target plane (which is not exposed on the Secret API response shape).
func (s *Secret) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "secret", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Secret, string, error) {
		p := &gen.ListSecretsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := s.client.ListSecrets(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}

	targets, err := s.listTargetPlanes(ctx, params.Namespace)
	if err != nil {
		return err
	}
	return printList(items, targets)
}

// Get retrieves a single secret via the Secret API and outputs it as YAML.
// Values in `.data` are base64-encoded, matching kubectl's default shape.
func (s *Secret) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "secret", map[string]string{
		"namespace": params.Namespace,
		"name":      params.SecretName,
	}); err != nil {
		return err
	}

	ctx := context.Background()
	result, err := s.client.GetSecret(ctx, params.Namespace, params.SecretName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal secret to YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

// listTargetPlanes returns a map from secret name to "Kind/Name" target plane,
// by enumerating SecretReferences in the namespace.
func (s *Secret) listTargetPlanes(ctx context.Context, namespace string) (map[string]string, error) {
	refs, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.SecretReference, string, error) {
		p := &gen.ListSecretReferencesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := s.client.ListSecretReferences(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(refs))
	for _, r := range refs {
		if r.Spec != nil && r.Spec.TargetPlane != nil {
			out[r.Metadata.Name] = fmt.Sprintf("%s/%s", r.Spec.TargetPlane.Kind, r.Spec.TargetPlane.Name)
		}
	}
	return out, nil
}

// Delete removes a secret from the control plane and the target plane.
func (s *Secret) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "secret", map[string]string{
		"namespace": params.Namespace,
		"name":      params.SecretName,
	}); err != nil {
		return err
	}

	ctx := context.Background()
	if err := s.client.DeleteSecret(ctx, params.Namespace, params.SecretName); err != nil {
		return err
	}
	fmt.Printf("Secret '%s' deleted\n", params.SecretName)
	return nil
}

// CreateGeneric creates an Opaque secret (or basic-auth / ssh-auth via the
// optional secretType override).
func (s *Secret) CreateGeneric(in CreateInput, secretType string) error {
	if err := requireCreateFields(in); err != nil {
		return err
	}

	tp, err := parseTargetPlane(in.TargetPlane)
	if err != nil {
		return err
	}

	data, err := collectData(in.FromLiteral, in.FromFile, in.FromEnvFile)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("at least one of --from-literal, --from-file, or --from-env-file is required")
	}

	st := gen.SecretTypeOpaque
	if secretType != "" {
		st = gen.SecretType(secretType)
	}
	return s.create(in.Namespace, in.SecretName, st, *tp, data)
}

// CreateDockerRegistry creates a kubernetes.io/dockerconfigjson secret.
func (s *Secret) CreateDockerRegistry(in CreateInput, server, username, password, email string) error {
	if err := requireCreateFields(in); err != nil {
		return err
	}
	missing := map[string]string{
		"docker-server":   server,
		"docker-username": username,
		"docker-password": password,
	}
	if err := cmdutil.RequireFields("create docker-registry", "secret", missing); err != nil {
		return err
	}

	tp, err := parseTargetPlane(in.TargetPlane)
	if err != nil {
		return err
	}

	cfg, err := buildDockerConfigJSON(server, username, password, email)
	if err != nil {
		return err
	}
	data := map[string]string{".dockerconfigjson": cfg}

	return s.create(in.Namespace, in.SecretName, gen.SecretTypeKubernetesIodockerconfigjson, *tp, data)
}

// CreateTLS creates a kubernetes.io/tls secret from a cert/key pair.
func (s *Secret) CreateTLS(in CreateInput, certPath, keyPath string) error {
	if err := requireCreateFields(in); err != nil {
		return err
	}
	if err := cmdutil.RequireFields("create tls", "secret", map[string]string{
		"cert": certPath,
		"key":  keyPath,
	}); err != nil {
		return err
	}

	tp, err := parseTargetPlane(in.TargetPlane)
	if err != nil {
		return err
	}

	cert, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read --cert %s: %w", certPath, err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read --key %s: %w", keyPath, err)
	}
	data := map[string]string{
		"tls.crt": string(cert),
		"tls.key": string(key),
	}

	return s.create(in.Namespace, in.SecretName, gen.SecretTypeKubernetesIotls, *tp, data)
}

func (s *Secret) create(namespace, name string, st gen.SecretType, tp gen.TargetPlaneRef, data map[string]string) error {
	ctx := context.Background()
	req := gen.CreateSecretRequest{
		SecretName:  name,
		SecretType:  st,
		TargetPlane: tp,
		Data:        data,
	}
	resp, err := s.client.CreateSecret(ctx, namespace, req)
	if err != nil {
		return err
	}
	respName := name
	if resp != nil && resp.Metadata.Name != "" {
		respName = resp.Metadata.Name
	}
	fmt.Printf("Secret '%s' created\n", respName)
	return nil
}

func requireCreateFields(in CreateInput) error {
	return cmdutil.RequireFields("create", "secret", map[string]string{
		"namespace":    in.Namespace,
		"name":         in.SecretName,
		"target-plane": in.TargetPlane,
	})
}

// buildDockerConfigJSON returns the JSON payload stored under ".dockerconfigjson"
// for a kubernetes.io/dockerconfigjson secret.
func buildDockerConfigJSON(server, username, password, email string) (string, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	entry := map[string]string{
		"username": username,
		"password": password,
		"auth":     auth,
	}
	if email != "" {
		entry["email"] = email
	}
	cfg := map[string]map[string]map[string]string{
		"auths": {server: entry},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encode docker config: %w", err)
	}
	return string(b), nil
}

func printList(items []gen.Secret, targets map[string]string) error {
	if len(items) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tTARGET PLANE\tAGE")

	for _, sec := range items {
		age := "<unknown>"
		if sec.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*sec.Metadata.CreationTimestamp)
		}
		target := targets[sec.Metadata.Name]
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sec.Metadata.Name, sec.Type, target, age)
	}

	return w.Flush()
}
