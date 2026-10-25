// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint

// The occ linter command tree, built with cobra.
//
// The tree mirrors the dispatch the CLI had before cobra took over, including
// the exit-code convention (0 success, 1 runtime/validation failure, 2 usage
// error) and the wording of every diagnostic. cobra returns errors rather than
// exit codes, so commands signal the process status with an *exitError and run
// translates it back.
//
// Two commands (occ and linter) disable flag parsing on purpose. They are the
// compatibility surfaces for a CLI that is being adopted into another binary:
// they must keep reporting "unknown subcommand" for arguments that pflag would
// otherwise reject with a different message. The commands people actually use
// (vali, schema, version) are ordinary cobra commands with real flag sets.

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lint/templateconfig"
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// exitErrorf returns the exit error for a deliberate non-zero status, or nil
// for zero. It is the one place a status is chosen, so ErrFindings and ErrUsage
// are not spread across the call sites.
func exitErrorf(code int) error {
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code}
}

// legacyError prints a diagnostic followed by the usage block, and returns the
// usage-error exit code. This is the reporting shape every error path in the
// pre-cobra CLI used.
func legacyError(msg string) error {
	fmt.Fprint(os.Stderr, msg+"\n\n")
	fmt.Print(usageText)
	return exitErrorf(2)
}

// NewLintCmd builds the `occ lint` subtree (alias `occ linter`).
//
// The toolkit built the whole standalone `occ …` root from NewRootCmd. A cobra
// root cannot be attached as a child of another root, so what moves here is
// only the linter subtree, with its own dispatch, help and exit-code behavior
// intact. occ keeps its own root, its own `config`/`version` commands and its
// own help; nothing in this subtree shadows them.
//
// The command is annotated so that occ's per-command context bootstrap is
// skipped for it and all of its children. Validation is offline: it needs no
// login, no control plane and no ~/.openchoreo/config, and writing a default
// context on a clean machine just to read local files would be a bug.
func NewLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "lint",
		Aliases: []string{"linter"},
		Short:   "Validate OpenChoreo resources",
		// 'occ lint -b dir' has always been an error rather than a run, so
		// flag parsing is left to the child commands.
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		SilenceErrors:      true,
		Annotations:        map[string]string{config.SkipContextBootstrapAnnotation: ""},
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return legacyError("Error: missing linter subcommand")
			}
			// 'help' and 'version' are registered subcommands, so only their
			// flag-style spellings reach this point.
			switch args[0] {
			case "help", "-h", "--help":
				fmt.Print(usageText)
				return nil
			case "version", "--version", "-version":
				printLinterVersion()
				return nil
			}
			return legacyError(fmt.Sprintf("Error: unknown linter subcommand %q (run 'occ linter help' for usage)", args[0]))
		},
	}
	// The linter's usage block replaces cobra's, because occ's root help would
	// otherwise be printed for every command in this subtree.
	cmd.SetHelpFunc(func(*cobra.Command, []string) { fmt.Print(usageText) })
	// occ's root does this too; a subtree cannot disable a completion command
	// the root owns, so this only documents the intent for the standalone case.

	cmd.AddCommand(
		newValidateCmd(),
		newSchemaCmd(),
		newLinterVersionCmd(),
		newHelpCmd(),
		renamedCmd("templates", "'occ linter templates' was renamed to 'occ linter schema' (short alias: 'occ lint schema')"),
		renamedCmd("config", "'occ linter config' is not a command"),
	)

	// 'occ linter --version' and '-version' are accepted, and extra arguments
	// after 'version' are ignored, as they always were.
	cmd.SetHelpCommand(newHelpCmd())
	return cmd
}

// renamedCmd builds a hidden command that only reports a rename.
func renamedCmd(name, message string) *cobra.Command {
	return &cobra.Command{
		Use:    name,
		Hidden: true,
		RunE: func(*cobra.Command, []string) error {
			return legacyError("Error: " + message)
		},
	}
}

func newHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help",
		Short: "Show help for the linter",
		RunE: func(*cobra.Command, []string) error {
			fmt.Print(usageText)
			return nil
		},
	}
}

// newLinterVersionCmd builds `occ linter version`.
func newLinterVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the linter version and the selected template version",
		// Trailing arguments have always been ignored here.
		Args: cobra.ArbitraryArgs,
		RunE: func(*cobra.Command, []string) error {
			printLinterVersion()
			return nil
		},
	}
}

// printLinterVersion reports the linter version and the template version
// currently selected for validation.
//
// The linter ships inside occ, so it reports occ's own version rather than a
// separate linter version: the binary contract is one binary and one version.
func printLinterVersion() {
	selected := template.LatestVersion
	if persisted, err := templateconfig.Load(); err == nil {
		if ver := normalizeTemplateVersion(persisted); ver != "" {
			selected = ver
		}
	}
	fmt.Printf("occ linter version %s\n", linterVersion())
	fmt.Printf("selected openchoreo version: %s\n", selected)
}

// newSchemaCmd builds `occ linter schema`.
func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Show or select the template version used for validation",
		// 'occ linter schema' with no subcommand is a usage error, not help.
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return legacyError("Error: missing schema subcommand")
			}
			return legacyError(fmt.Sprintf("Error: unknown schema subcommand %q", args[0]))
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List available template versions",
			RunE: func(*cobra.Command, []string) error {
				return runSchemaList()
			},
		},
		&cobra.Command{
			Use:   "select <version>",
			Short: "Select the template version to validate against",
			Args:  cobra.ArbitraryArgs,
			RunE: func(_ *cobra.Command, args []string) error {
				return runSchemaSelect(args)
			},
		},
	)
	return cmd
}

// newValidateCmd builds `occ linter vali` (alias `validate`).
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "vali [file|folder]",
		Aliases: []string{"validate"},
		Short:   "Validate OpenChoreo resource files",
		// Only the first positional argument is used; the rest have always
		// been ignored rather than rejected.
		Args: cobra.ArbitraryArgs,
		RunE: recovered("occ linter vali", func(cmd *cobra.Command, args []string) error {
			// Test-only recovery hook: when OCC_TEST_PANIC is set the command
			// panics so the black-box recovery test can exercise the recovered
			// decorator end-to-end (exit code 1 + internal-error report on
			// stderr). It is never set in normal use.
			if env := os.Getenv("OCC_TEST_PANIC"); env != "" {
				panic(env)
			}

			f := validateFlags{
				batch:           mustBool(cmd, "batch"),
				detect:          mustBool(cmd, "detect"),
				obj:             mustBool(cmd, "obj"),
				parallel:        mustInt(cmd, "parallel"),
				quiet:           mustBool(cmd, "quiet"),
				verbose:         mustBool(cmd, "verbose"),
				noColor:         mustBool(cmd, "no-color"),
				templateVersion: mustString(cmd, "template-version"),
				fix:             mustBool(cmd, "fix"),
				dryRun:          mustBool(cmd, "dry-run"),
				render:          mustBool(cmd, "render"),
			}
			// The dependency check runs before the arity check, so that
			// 'vali -dry-run' reports the dependency rather than the path.
			if f.dryRun && !f.fix {
				return legacyError("Error: -dry-run only applies together with -fix")
			}
			if f.noColor {
				colorOutput = false
			}
			if len(args) == 0 {
				return legacyError("Error: missing file or folder path")
			}
			return runValidate(f, args[0])
		}),
	}

	fs := cmd.Flags()
	fs.BoolP("batch", "b", false, "batch mode: validate all YAML files under the given folder")
	fs.Bool("detect", false, "scan every subfolder for YAML files and validate them")
	fs.Bool("obj", false, "print the result as a JSON report instead of text")
	fs.IntP("parallel", "p", runtime.GOMAXPROCS(0), "number of parallel workers for folder validation (default: number of CPUs)")
	fs.BoolP("quiet", "q", false, "suppress progress output, print only results")
	fs.BoolP("verbose", "v", false, "print verbose/debug detail to stderr")
	fs.Bool("no-color", false, "use plain text markers instead of glyph icons")
	fs.String("template-version", "", "template version to validate against (e.g. v1.2.0, v1.3.0, latest); also saved as the current version")
	fs.Bool("fix", false, "repair misspelt field, kind and enum values and normalize YAML formatting")
	fs.Bool("dry-run", false, "with -fix, print the proposed changes as a diff instead of writing them")
	fs.Bool("render", false, "enable post-render validation")

	// Keep pflag's diagnostics in the shape the previous flag-based CLI used.
	// The message and the usage block are printed here, in that order, because
	// the standalone binary used to print them from its top-level dispatcher and
	// as a subtree there is no such dispatcher left to intercept the error.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		msg := stdlibFlagMessage(err)
		fmt.Fprintln(os.Stderr, msg)
		fmt.Fprint(os.Stderr, c.UsageString())
		return ErrUsage
	})

	// `vali` is the one command with flags worth documenting. It sets its own
	// help function because cobra would otherwise inherit the linter's, which
	// prints the top-level usage for every command in the subtree.
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		fmt.Fprint(c.OutOrStdout(), valiHelpText)
		fmt.Fprintln(c.OutOrStdout())
		fmt.Fprint(c.OutOrStdout(), c.UsageString())
	})
	// The usage line is hand-written to match the CLI's "occ linter vali"
	// phrasing rather than cobra's parent-path form.
	cmd.SetUsageTemplate(valiUsageTemplate)

	return cmd
}

// valiUsageTemplate is cobra's default usage template with the command path
// replaced, so `occ linter vali --help` names the command the user invoked.
const valiUsageTemplate = `Usage:{{if .Runnable}}
  occ linter vali [file|folder] [flags]{{end}}{{if .HasAvailableSubCommands}}
  occ linter vali [command]{{end}}

{{if gt (len .Aliases) 0}}
Aliases:
  {{.NameAndAliases}}

{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`

// valiHelpText is the long description printed above the usage block. The
// per-flag detail is left to the usage block so the two cannot drift apart.
const valiHelpText = `Validate OpenChoreo resource files.

A path selects what is validated: a single YAML file, or a folder, which needs
-batch or -detect. The report lists every issue per document. The exit code is
1 when any error was found, and 2 for a usage mistake.

Examples:
  occ linter vali ./component.yaml
  occ linter vali -b -p 8 ./manifests
  occ linter vali -detect -q ./sample-gitops
  occ linter vali -fix -dry-run ./manifests
  occ linter vali -obj ./component.yaml
  occ linter vali --template-version v1.2.0 ./manifests

With -fix, the repaired files are written atomically and then validated again,
so the report shows what is left to do. The summary says how many files were
rewritten. Add -dry-run to preview a diff instead of writing anything.

See 'occ linter help' for the full command list and docs/cli-usage.md for the
diagnostic codes.`

var (
	pflagUnknownShorthand = regexp.MustCompile(`^unknown shorthand flag: '.' in (-.+)$`)
	pflagUnknownFlag      = regexp.MustCompile(`^unknown flag: (--.+)$`)
	pflagNeedsArgShort    = regexp.MustCompile(`^flag needs an argument: '.' in (-.+)$`)
	pflagNeedsArgLong     = regexp.MustCompile(`^flag needs an argument: (--.+)$`)
	pflagInvalidValue     = regexp.MustCompile(`^invalid argument "([^"]*)" for "-([a-zA-Z0-9-]+)(?:,[^"]*)?" flag:`)
)

// stdlibFlagMessage rewrites pflag's wording to the wording the flag package
// produced, because that text is part of the CLI's observable surface. Note
// that the flag package spells every flag with a single leading dash, even when
// the user typed two.
func stdlibFlagMessage(err error) string {
	msg := err.Error()
	switch {
	case pflagUnknownShorthand.MatchString(msg):
		return "flag provided but not defined: " + pflagUnknownShorthand.FindStringSubmatch(msg)[1]
	case pflagUnknownFlag.MatchString(msg):
		return "flag provided but not defined: -" + strings.TrimPrefix(pflagUnknownFlag.FindStringSubmatch(msg)[1], "--")
	case pflagNeedsArgShort.MatchString(msg):
		return "flag needs an argument: " + pflagNeedsArgShort.FindStringSubmatch(msg)[1]
	case pflagNeedsArgLong.MatchString(msg):
		return "flag needs an argument: -" + strings.TrimPrefix(pflagNeedsArgLong.FindStringSubmatch(msg)[1], "--")
	case pflagInvalidValue.MatchString(msg):
		m := pflagInvalidValue.FindStringSubmatch(msg)
		return fmt.Sprintf("invalid value %q for flag -%s: parse error", m[1], m[2])
	}
	return msg
}

func mustBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func mustInt(cmd *cobra.Command, name string) int {
	v, _ := cmd.Flags().GetInt(name)
	return v
}

func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// longOnlyFlags are the vali flags that have no single-character shorthand.
// pflag reads a single-dash multi-character argument as a cluster of
// shorthands, so the historical spellings are rewritten to their double-dash
// form before parsing.
var longOnlyFlags = map[string]bool{
	"detect":           true,
	"obj":              true,
	"fix":              true,
	"dry-run":          true,
	"no-color":         true,
	"template-version": true,
}

// NormalizeArgs rewrites single-dash spellings of long-only vali flags, so that
// 'occ lint vali -dry-run' keeps working alongside '--dry-run'. Only arguments
// belonging to a validation command are touched, which keeps the error messages
// of the surrounding commands byte-for-byte unchanged.
//
// This has to run before cobra parses anything, because pflag reads '-obj' as a
// shorthand cluster and rejects it. The standalone binary did it in its own
// main(); as a subtree the linter has no main, so occ calls it while building
// the argument list.
func NormalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	normalizing := false
	terminated := false
	for _, a := range args {
		switch {
		case a == "--":
			normalizing, terminated = false, true
		case !terminated && !normalizing:
			// Start normalizing at the validation subcommand, so that
			// 'occ linter -dry-run' is still reported as an unknown
			// subcommand spelled the way the user typed it.
			normalizing = a == "vali" || a == "validate"
		case !terminated && longOnlySpelling(a) != "":
			out = append(out, "--"+longOnlySpelling(a))
			continue
		}
		out = append(out, a)
	}
	return out
}

// longOnlySpelling returns the long-flag spelling of a single-dash argument, or
// "" when the argument is not a long-only flag.
func longOnlySpelling(arg string) string {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return ""
	}
	body := arg[1:]
	if name, _, found := strings.Cut(body, "="); found {
		if longOnlyFlags[name] {
			return body
		}
		return ""
	}
	if longOnlyFlags[body] {
		return body
	}
	return ""
}
