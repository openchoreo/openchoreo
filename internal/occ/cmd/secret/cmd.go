// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// NewSecretCmd returns the root `occ secret` command.
func NewSecretCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "secret",
		Aliases: []string{"secrets"},
		Short:   "Manage secrets",
		Long:    "Manage secrets that are pushed to a target plane's external secret store.",
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newCreateCmd(f),
		newUpdateCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets",
		Long:  "List secrets in a namespace.",
		Example: `  # List all secrets in a namespace
  occ secret list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [SECRET_NAME]",
		Short: "Get a secret",
		Long:  "Get a secret and display its details in YAML format.",
		Example: `  # Get a secret
  occ secret get my-secret --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:  flags.GetNamespace(cmd),
				SecretName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [SECRET_NAME]",
		Short: "Delete a secret",
		Long:  "Delete a secret from the external secret store.",
		Example: `  # Delete a secret
  occ secret delete my-secret --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:  flags.GetNamespace(cmd),
				SecretName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func readUpdateInput(cmd *cobra.Command, args []string) UpdateInput {
	literals, _ := cmd.Flags().GetStringArray("from-literal")
	files, _ := cmd.Flags().GetStringArray("from-file")
	envFiles, _ := cmd.Flags().GetStringArray("from-env-file")
	replace, _ := cmd.Flags().GetBool("replace")

	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	return UpdateInput{
		Namespace:   flags.GetNamespace(cmd),
		SecretName:  name,
		FromLiteral: literals,
		FromFile:    files,
		FromEnvFile: envFiles,
		Replace:     replace,
	}
}

func newUpdateCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [SECRET_NAME]",
		Short: "Update a secret's data",
		Long: `Update the data of an existing secret.

By default the update merges: keys passed via --from-literal, --from-file, or
--from-env-file are set or added, and every other existing key is left
unchanged.

Use --replace to set the data to exactly what the --from-* flags specify,
pruning any keys not listed.

The secret type, target plane, and category cannot be changed; use
'occ secret delete' and 'occ secret create' to change them.`,
		Example: `  # Rotate one key, keep the rest
  occ secret update db-creds --namespace acme-corp \
    --from-literal=password=n3ws3cret

  # Add a key from a file
  occ secret update db-creds --namespace acme-corp \
    --from-file=ca.crt=./ca.crt

  # Replace the data with exactly these keys
  occ secret update db-creds --namespace acme-corp --replace \
    --from-literal=username=admin --from-literal=password=n3ws3cret`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Update(readUpdateInput(cmd, args))
		},
	}
	flags.AddNamespace(cmd)
	cmd.Flags().StringArray("from-literal", nil, "Key=value literal to set (repeatable)")
	cmd.Flags().StringArray("from-file", nil, "Key=path or path to set (repeatable). Key defaults to the filename.")
	cmd.Flags().StringArray("from-env-file", nil, "Path to a KEY=VALUE env file (repeatable)")
	cmd.Flags().Bool("replace", false, "Replace the secret data with exactly the --from-* keys, pruning all others")
	return cmd
}

func newCreateCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a secret",
		Long:  "Create a secret of the specified type on a target plane.",
	}
	cmd.AddCommand(
		newCreateGenericCmd(f),
		newCreateDockerRegistryCmd(f),
		newCreateTLSCmd(f),
	)
	return cmd
}

// addCommonCreateFlags wires the flags shared by every `create` subcommand.
func addCommonCreateFlags(cmd *cobra.Command) {
	flags.AddNamespace(cmd)
	cmd.Flags().String("target-plane", "", "Target plane in Kind/Name form (e.g. DataPlane/dp-prod)")
	cmd.Flags().String("category", "", "Secret category: 'generic' (default) or 'git-credentials'")
}

func readCreateInput(cmd *cobra.Command, args []string) CreateInput {
	literals, _ := cmd.Flags().GetStringArray("from-literal")
	files, _ := cmd.Flags().GetStringArray("from-file")
	envFiles, _ := cmd.Flags().GetStringArray("from-env-file")
	targetPlane, _ := cmd.Flags().GetString("target-plane")
	category, _ := cmd.Flags().GetString("category")

	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	return CreateInput{
		Namespace:   flags.GetNamespace(cmd),
		SecretName:  name,
		TargetPlane: targetPlane,
		Category:    category,
		FromLiteral: literals,
		FromFile:    files,
		FromEnvFile: envFiles,
	}
}

func newCreateGenericCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generic [SECRET_NAME]",
		Short: "Create a generic (Opaque) secret",
		Long: `Create an Opaque secret from literals, files, or env files.

Use --type to create a typed Kubernetes secret (e.g. kubernetes.io/basic-auth,
kubernetes.io/ssh-auth) while still sourcing data the same way.`,
		Example: `  # Create an Opaque secret from literal values
  occ secret create generic db-creds \
    --namespace acme-corp \
    --target-plane DataPlane/dp-prod \
    --from-literal=username=admin \
    --from-literal=password=s3cret

  # Create a basic-auth secret
  occ secret create generic basic --type=kubernetes.io/basic-auth \
    --namespace acme-corp --target-plane DataPlane/dp-prod \
    --from-literal=username=admin --from-literal=password=s3cret`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			st, _ := cmd.Flags().GetString("type")
			return New(cl).CreateGeneric(readCreateInput(cmd, args), st)
		},
	}
	addCommonCreateFlags(cmd)
	cmd.Flags().StringArray("from-literal", nil, "Key=value literal (repeatable)")
	cmd.Flags().StringArray("from-file", nil, "Key=path or path (repeatable). Key defaults to the filename.")
	cmd.Flags().StringArray("from-env-file", nil, "Path to a KEY=VALUE env file (repeatable)")
	cmd.Flags().String("type", "", "Kubernetes secret type override (e.g. kubernetes.io/basic-auth)")
	return cmd
}

func newCreateDockerRegistryCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker-registry [SECRET_NAME]",
		Short: "Create a Docker registry credentials secret",
		Long:  "Create a kubernetes.io/dockerconfigjson secret from registry credentials.",
		Example: `  # Create a Docker registry secret
  occ secret create docker-registry regcred \
    --namespace acme-corp --target-plane DataPlane/dp-prod \
    --docker-server=https://index.docker.io/v1/ \
    --docker-username=jdoe --docker-password=hunter2 \
    --docker-email=jdoe@example.com`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			server, _ := cmd.Flags().GetString("docker-server")
			user, _ := cmd.Flags().GetString("docker-username")
			pass, _ := cmd.Flags().GetString("docker-password")
			email, _ := cmd.Flags().GetString("docker-email")
			return New(cl).CreateDockerRegistry(readCreateInput(cmd, args), server, user, pass, email)
		},
	}
	addCommonCreateFlags(cmd)
	cmd.Flags().String("docker-server", "", "Docker registry server URL")
	cmd.Flags().String("docker-username", "", "Docker registry username")
	cmd.Flags().String("docker-password", "", "Docker registry password")
	cmd.Flags().String("docker-email", "", "Docker registry email (optional)")
	return cmd
}

func newCreateTLSCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tls [SECRET_NAME]",
		Short: "Create a TLS secret",
		Long:  "Create a kubernetes.io/tls secret from a certificate and key pair.",
		Example: `  # Create a TLS secret
  occ secret create tls my-tls \
    --namespace acme-corp --target-plane DataPlane/dp-prod \
    --cert=./tls.crt --key=./tls.key`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			cert, _ := cmd.Flags().GetString("cert")
			key, _ := cmd.Flags().GetString("key")
			return New(cl).CreateTLS(readCreateInput(cmd, args), cert, key)
		},
	}
	addCommonCreateFlags(cmd)
	cmd.Flags().String("cert", "", "Path to PEM-encoded certificate file")
	cmd.Flags().String("key", "", "Path to PEM-encoded private key file")
	return cmd
}
