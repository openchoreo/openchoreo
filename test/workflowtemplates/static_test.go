// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"regexp"
	"strings"
	"testing"
)

type templateContract struct {
	metadataName string
	templateName string
}

// allTemplates maps every shipped template file to its expected metadata.name
// and the Argo template that owns the embedded shell contract.
var allTemplates = map[string]templateContract{
	"checkout-source.yaml":           {"checkout-source", "checkout"},
	"containerfile-build.yaml":       {"containerfile-build", "build-image"},
	"ballerina-buildpack-build.yaml": {"ballerina-buildpack-build", "build-image"},
	"gcp-buildpacks-build.yaml":      {"gcp-buildpacks-build", "build-image"},
	"paketo-buildpacks-build.yaml":   {"paketo-buildpacks-build", "build-image"},
	"publish-image.yaml":             {"publish-image", "publish-image"},
	"publish-image-k3d.yaml":         {"publish-image", "publish-image"},
	"generate-workload.yaml":         {"generate-workload", "generate-workload-cr"},
	"generate-workload-k3d.yaml":     {"generate-workload", "generate-workload-cr"},
}

// buildTemplates are the four image-build templates that share a common
// contract (path validation, build-env plumbing, app-image.tar output).
var buildTemplates = []string{
	"containerfile-build.yaml",
	"ballerina-buildpack-build.yaml",
	"gcp-buildpacks-build.yaml",
	"paketo-buildpacks-build.yaml",
}

var buildpackTemplates = []string{
	"ballerina-buildpack-build.yaml",
	"gcp-buildpacks-build.yaml",
	"paketo-buildpacks-build.yaml",
}

// requireContains asserts every needle is present in the script (scenario 21+:
// feature-deletion guard).
func requireContains(t *testing.T, script string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(script, n) {
			continue
		}
		t.Fatalf(`
contract:
  workflow-template script must contain required construct

missing:
  %q`, n)
	}
}

func requireRegexp(t *testing.T, pattern *regexp.Regexp, script string, contract string) {
	t.Helper()
	if pattern.MatchString(script) {
		return
	}
	t.Fatalf(`
contract:
  %s

missing pattern:
  %q`, contract, pattern.String())
}

func requireEqualContract[T comparable](t *testing.T, got, want T, contract string) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  %v

actual:
  %v`, contract, want, got)
}

func requireTrueContract(t *testing.T, got bool, contract string) {
	t.Helper()
	if got {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  true

actual:
  false`, contract)
}

func requireNotContains(t *testing.T, script string, needle string, contract string) {
	t.Helper()
	if !strings.Contains(script, needle) {
		return
	}
	t.Fatalf(`
contract:
  %s

unexpected:
  %q`, contract, needle)
}

// --- Cross-cutting invariants (scenario 21, 22) ---

func TestAllTemplates_ParseAndShape(t *testing.T) {
	for file, contract := range allTemplates {
		t.Run(file, func(t *testing.T) {
			wt := loadTemplate(t, file)
			requireEqualContract(t, wt.Kind, "ClusterWorkflowTemplate",
				"workflow-template YAML must be an Argo ClusterWorkflowTemplate")
			requireEqualContract(t, wt.Metadata.Name, contract.metadataName,
				"workflow-template metadata.name must match the shipped contract")
			requireTrueContract(t, len(wt.Spec.Templates) > 0,
				"workflow-template must define at least one Argo template")

			script := scriptForTemplate(t, file, contract.templateName)
			requireTrueContract(t, strings.HasPrefix(strings.TrimSpace(script), "set -e"),
				"workflow-template script must start with set -e")
		})
	}
}

// --- checkout-source: provider + auth contract (scenarios 1-6) ---

func TestCheckoutSource_AuthAndProviderContract(t *testing.T) {
	s := scriptForTemplate(t, "checkout-source.yaml", "checkout")

	t.Run("ssh config covers all four known providers", func(t *testing.T) {
		requireContains(t, s,
			"Host github.com",
			"Host gitlab.com",
			"Host bitbucket.org",
			"Host git-codecommit.*.amazonaws.com",
			"StrictHostKeyChecking no",
		)
	})

	t.Run("auth-type detection by secret file", func(t *testing.T) {
		requireContains(t, s,
			"ssh-privatekey", // SSH auth path
			"password",       // basic-auth path
			"ssh-key-id",     // AWS CodeCommit SSH key id
		)
	})

	t.Run("ssh private key validation", func(t *testing.T) {
		requireContains(t, s, "BEGIN.*PRIVATE KEY", "ssh-keygen")
	})

	t.Run("https to ssh rewrite for non-codecommit providers", func(t *testing.T) {
		// The generic sed rewrite is what makes a *new* provider work over
		// https-rewritten-to-ssh. Keep this assertion host-agnostic.
		requireRegexp(t, regexp.MustCompile(`git@\\1:\\2\.git`), s, "https repo URL is rewritten to git@host:path.git")
	})

	t.Run("codecommit ssh key-id injection", func(t *testing.T) {
		requireContains(t, s, "git-codecommit", "${SSH_KEY_ID}@")
	})

	t.Run("basic-auth percent-encodes credentials", func(t *testing.T) {
		// %40 == '@', %3A == ':' — sentinel of the URL-encoding sed.
		requireContains(t, s, "USERNAME_ENCODED", "PASSWORD_ENCODED", "%40", "%3A")
		requireContains(t, s, "credential.helper store")
	})

	t.Run("checkout by branch and by commit", func(t *testing.T) {
		requireContains(t, s,
			"git clone --no-checkout --depth 1",  // commit path
			"git clone --single-branch --branch", // branch path
			"git-revision.txt",
		)
	})

	t.Run("validation guards", func(t *testing.T) {
		requireContains(t, s,
			"Git repository URL is required",
			"Either a branch or commit must be specified",
		)
	})
}

func TestCheckoutSource_SecretWiring(t *testing.T) {
	requireEqualContract(t, mountPath(t, "checkout-source.yaml", "git-secret"), "/etc/secrets/git-secret",
		"script reads $GIT_SECRET_PATH=/etc/secrets/git-secret")
	requireEqualContract(t, mountPath(t, "checkout-source.yaml", "workspace"), "/mnt/vol",
		"checkout-source workspace volume must be mounted at /mnt/vol")

	found, optional := secretVolumeOptional(t, "checkout-source.yaml", "git-secret")
	requireTrueContract(t, found, "checkout-source must define a git-secret volume")
	requireTrueContract(t, optional, "git-secret must be optional so public repos work without it")
}

// --- build templates: shared contract (scenarios 7-11) ---

func TestBuildTemplates_SharedContract(t *testing.T) {
	for _, file := range buildTemplates {
		t.Run(file, func(t *testing.T) {
			s := scriptForTemplate(t, file, "build-image")
			// Output handoff to publish-image.
			requireContains(t, s, "/mnt/vol/app-image.tar")
			// Path validation guard.
			requireContains(t, s, "exit 1")
			// build-env JSON -> --env flags, with empty/[] skipped.
			requireContains(t, s, "build-env", `!= "[]"`, "--env")
		})
	}
}

func TestContainerfileBuild_Specifics(t *testing.T) {
	s := scriptForTemplate(t, "containerfile-build.yaml", "build-image")
	requireContains(t, s,
		"podman build",
		"dockerfile-path",
		"docker-context",
		"--build-arg", // containerfile additionally handles build-args
		"podman save -o /mnt/vol/app-image.tar",
	)
}

func TestBuildpackTemplates_Specifics(t *testing.T) {
	for _, file := range buildpackTemplates {
		t.Run(file, func(t *testing.T) {
			s := scriptForTemplate(t, file, "build-image")
			requireContains(t, s,
				"pack build",
				"--builder",
				"--run-image",
				"--pull-policy always",
				"--docker-host inherit",
				"app-path",
			)
			// Supply-chain: builder/run images pinned by digest, not a tag.
			requireContains(t, s, "@sha256:")
			// Rootless podman service is started and waited on.
			requireContains(t, s,
				"podman system service",
				"RemoteSocket.Exists",
				"podman image exists",
			)
		})
	}
}

// --- publish-image: cloud vs k3d (scenarios 12-14) ---

func TestPublishImage_Cloud(t *testing.T) {
	s := scriptForTemplate(t, "publish-image.yaml", "publish-image")
	requireContains(t, s,
		"ttl.sh/openchoreo-builds",
		"--tls-verify=true",
		"podman load -i /mnt/vol/app-image.tar",
		"podman tag",
		"/tmp/image.txt",
		"Built image tar not found", // missing-tar guard
		"--authfile",                // auth branch
	)
}

func TestPublishImage_K3d(t *testing.T) {
	s := scriptForTemplate(t, "publish-image-k3d.yaml", "publish-image")
	requireContains(t, s,
		"host.k3d.internal:10082",
		"--tls-verify=false",
		"--authfile",
		"podman load -i /mnt/vol/app-image.tar",
		"/tmp/image.txt",
	)
	requireNotContains(t, s, "--tls-verify=true",
		"k3d publish-image variant must not use TLS verification for the local registry")
}

func TestPublishImage_SecretWiring(t *testing.T) {
	for _, file := range []string{"publish-image.yaml", "publish-image-k3d.yaml"} {
		t.Run(file, func(t *testing.T) {
			requireEqualContract(t, mountPath(t, file, "registry-push-secret"), "/etc/secrets/registry-push-secret",
				"publish-image registry-push-secret volume must mount at /etc/secrets/registry-push-secret")
			found, optional := secretVolumeOptional(t, file, "registry-push-secret")
			requireTrueContract(t, found, "publish-image must define a registry-push-secret volume")
			requireTrueContract(t, optional, "registry-push-secret must be optional for anonymous push")
		})
	}
}

// --- generate-workload: shared + variant contract (scenarios 15-20) ---

func TestGenerateWorkload_SharedContract(t *testing.T) {
	for _, file := range []string{"generate-workload.yaml", "generate-workload-k3d.yaml"} {
		t.Run(file, func(t *testing.T) {
			s := scriptForTemplate(t, file, "generate-workload-cr")
			requireContains(t, s,
				"occ workload create",
				"WORKLOAD_FROM_SOURCE",          // source vs auto-generated branch
				"del(.apiVersion)",              // YAML->JSON strip
				"del(.kind)",                    //
				"grant_type=client_credentials", // OAuth
				"access_token",
				`RESPONSE=$(curl`,
				`-X POST "${API_URL}/api/v1/namespaces/${NAMESPACE_NAME}/workloads"`,
				`-d @"${WORKLOAD_JSON}"`,
				"-eq 409",                 // conflict handling
				".spec.container.image",   // image-only merge on auto-generated
				"openchoreo.dev/workload", // WorkflowRun annotation
				"workload-from-source",
			)
		})
	}
}

func TestGenerateWorkload_Cloud(t *testing.T) {
	// The oauth URL lives in the parameter defaults, not the script body.
	requireEqualContract(t, inputParamDefault(t, "generate-workload.yaml", "oauth-token-url"),
		"https://host.k3d.internal:8080/oauth2/token",
		"cloud variant talks to oauth over https")

	s := scriptForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	requireContains(t, s, "curl -sk")
	requireNotContains(t, s, `-H "Host: ${OAUTH_HOST}"`,
		"cloud variant does not use host-header routing")
}

func TestGenerateWorkload_K3d(t *testing.T) {
	requireEqualContract(t, inputParamDefault(t, "generate-workload-k3d.yaml", "oauth-token-url"),
		"http://host.k3d.internal:8080/oauth2/token",
		"k3d variant talks to oauth over http (TLS terminates at ingress)")

	s := scriptForTemplate(t, "generate-workload-k3d.yaml", "generate-workload-cr")
	requireContains(t, s,
		`-H "Host: ${OAUTH_HOST}"`,
		`-H "Host: ${API_HOST}"`,
		"curl -s ", // TLS-valid ingress, no -k
	)
	requireNotContains(t, s, "curl -sk", "k3d variant must not use -k")
}
