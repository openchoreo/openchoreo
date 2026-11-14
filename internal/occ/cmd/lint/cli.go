// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package lint implements the occ lint command line. It owns the command
// tree, argument dispatch, validation, and report rendering; the rules
// themselves live in the ruleengine and fixer packages.
package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/version"
	clivalidator "github.com/openchoreo/openchoreo/tools/lint/cli-validator"
	"github.com/openchoreo/openchoreo/tools/lint/fixer"
	"github.com/openchoreo/openchoreo/tools/lint/parallel"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	postrendervalidator "github.com/openchoreo/openchoreo/tools/lint/post-render-validator"
	schemavalidator "github.com/openchoreo/openchoreo/tools/lint/pre-render-validator"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lint/suppress"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
	"github.com/openchoreo/openchoreo/tools/lint/templateconfig"
)

// linterVersion is the version the linter reports, in the -obj report's
// linter_version field and in `occ lint version`.
//
// The linter ships inside occ, so it reports occ's own version rather than a
// separate linter version: the binary contract is one binary and one version.
// occ's version is set at build time with
// -ldflags "-X github.com/openchoreo/openchoreo/internal/version.version=<ver>",
// and reads "not-set" in an unlinked build.
func linterVersion() string { return version.Get().Version }

// colorOutput controls whether glyph icons (✗/⚠/✓) are used in text output.
// It is disabled by --no-color or the NO_COLOR environment variable.
var colorOutput = true

// reportWriter is where a validation report is written. It is stdout in normal
// use; a caller that only needs the validation verdict can swap in a buffer and
// decide whether to surface the report.
var reportWriter io.Writer = os.Stdout

// currentSchemaVersion is the template version the current validation run
// resolved to; the -obj report shows it as schema_version.
var currentSchemaVersion string

func init() {
	if os.Getenv("NO_COLOR") != "" {
		colorOutput = false
	}
}

const usageText = `occ - OpenChoreo linter

Usage:
  occ linter vali <file.yaml>          validate a configuration YAML file
  occ linter vali -b <folder>          validate YAML files directly in the folder only
  occ linter vali -detect <folder>     validate YAML files in the folder and all subfolders
  occ linter vali -obj <target>        print the result as a JSON report
  occ linter schema list                  show the available template versions
  occ linter schema select <ver>          select the template version to validate against
  occ linter version                   print the linter version and the selected template version
  occ linter help                      show this help (same as -h/--help)

Note: 'occ lint' is a short alias for 'occ linter'. Every command starts with
'occ linter' (or 'occ lint').

Only YAML files that declare an OpenChoreo apiVersion
(apiVersion: openchoreo.dev/v1alpha1 or apiVersion: openchoreo.dev/...) are
validated. Every other YAML file is ignored.

Template versions:
  Validation runs against a template version, which defines the JSON schema
  used for a given OpenChoreo version. The version is resolved with this
  precedence:
    1. --template-version on the validation command (also saved as current)
    2. the version selected with 'occ linter schema select'
    3. the latest template version

Commands:
  linter vali <file>                   validate the given YAML file and report issues
  linter vali -b <folder>              validate all .yaml/.yml files in the given folder
                                       (subfolders are NOT scanned)
  linter vali -detect <folder>         scan every subfolder recursively using path/filepath
                                       and validate all .yaml/.yml files found
  linter vali -obj <file|folder>       print the result as a JSON report:
                                       linter_version, schema_version, runs[0].
                                       tool.driver.name "occ linter", results[]
                                       with ruleId, level, message.text, locations
                                       and, with -fix, fixes[] for the repairs made.
                                       Combine with -b or -detect.
  linter vali -fix <file|folder>       repair what can be repaired safely:
                                         - a misspelled field, kind, or enum value
                                         - trailing whitespace, repeated blank
                                           lines, CRLF line endings, the final
                                           newline, comments and indentation
                                       Renames only apply when a single schema
                                       field is within 3 edits, the field is
                                       missing, and no sibling already uses the
                                       name. Ambiguous candidates are reported as
                                       skipped and left alone. Only OpenChoreo
                                       YAML is touched, and '# occ:ignore'
                                       suppressions are honored. The file is
                                       written atomically, then validated again
                                       so the report shows what is left to do.
                                       Add -dry-run to preview a diff instead.
  linter vali --template-version <v>   validate using template version <v> and save it as
                                       the current template version (e.g. v1.2.0, v1.3.0,
                                       or latest)
  schema list                           print every available template version with the
                                       current one marked
  schema select <v>                     save <v> as the current template version (e.g.
                                       v1.2.0, v1.3.0, or latest)
  version / --version / -version       print the linter version and the selected
                                       template version, then exit
  help / -h / --help                   show this help

Resource location validation:
  All commands also report a "location-not-found" WARNING when a resource name
  referenced in the file (e.g. spec.componentType.name, spec.workflow.name,
  spec.allowedWorkflows[].name, spec.allowedTraits[].name,
  spec.deploymentPipelineRef.name) does not resolve to a resource defined in
  the workspace. The workspace scope used to resolve references depends on the
  mode:
    <file> / -obj file      the whole repo containing the file (git root,
                            or filesystem root when not in a git repo)
    -b <folder>             only YAML files directly inside the folder
    -detect <folder>        the folder and all of its subfolders

Options:
  -b        batch mode: validate YAML files directly inside the given folder only
  -detect   scan the folder and all subfolders recursively for YAML files
  -obj      output the result as a JSON report instead of text
  -fix      repair misspelled field, kind and enum values and normalize YAML
            formatting in place, then report the issues that remain
  -dry-run  with -fix, print the proposed changes as a diff and write nothing
  -p N      validate up to N files in parallel (default: number of CPUs)
  -q        suppress progress output, print only results
  -v        print verbose/debug detail to stderr
  -no-color use plain text markers (ERROR/WARN/OK) instead of ✗/⚠/✓

Other commands:
  Every command starts with 'occ linter' (or the short alias 'occ lint'),
  for example 'occ linter version' and 'occ linter help'.

Suppression:
  Diagnostics can be suppressed with comments in the YAML file:
    # occ:ignore <code>[, <code2> ...]  suppress the listed codes on this line
    # occ:ignore                         suppress every code on this line
    # occ:ignore-file <code>[...]       suppress the listed codes for this whole file
    # occ:ignore-file                    suppress every code for this whole file
  Use a code list of "all" to mean every code. The not-openchoreo structural
  error cannot be suppressed. See docs/cli-usage.md for the full list of
  diagnostic codes.
`

// recovered decorates a command's RunE so that a panic becomes a clean
// internal-error report on stderr and a runtime failure, instead of the raw Go
// crash trace a panic would otherwise print. It mirrors the LSP's
// handler-recovery wrappers, and replaces the process-scope recovery the
// toolkit wrapped around its standalone main: as a subtree there is no single
// entry point left to wrap, so each leaf recovers for itself.
func recovered(scope string, fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) (err error) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "%s\n%s", panicMessage(scope, r), debug.Stack())
				err = &ExitError{Code: 1}
			}
		}()
		return fn(c, args)
	}
}

// panicMessage formats the headline for a recovered panic, mirroring the
// language server's reportPanic.
func panicMessage(scope string, recovered any) string {
	return fmt.Sprintf("internal error in %s: %v", scope, recovered)
}

// reportPanicLog logs a recovered panic with its stack trace to stderr and
// returns an error carrying the same message so the caller can surface it.
func reportPanicLog(scope string, recovered any) error {
	msg := panicMessage(scope, recovered)
	fmt.Fprintf(os.Stderr, "%s\n%s", msg, debug.Stack())
	return fmt.Errorf("%s", msg)
}

// safeAnalyze wraps a per-file analyzer so a panic becomes a per-file error
// instead of aborting the run, mirroring the LSP's handler-recovery wrappers.
func safeAnalyze(path string, fn func() fileOutcome) (out fileOutcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = fileOutcome{path: path, errs: 1, err: reportPanicLog("analyzing "+path, recovered)}
		}
	}()
	return fn()
}

// runSchemaList prints every available template version and marks the one
// that validation currently uses.
func runSchemaList() error {
	current, err := templateconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read template config: %v\n", err)
	}
	current = normalizeTemplateVersion(current)
	if current == "" {
		current = template.LatestVersion
	}

	versions := template.AvailableVersions()
	fmt.Println("Available openchoreo versions:")
	for _, v := range versions {
		marker := "  "
		if v == current {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, v)
	}
	fmt.Printf("\nCurrent template version: %s\n", current)
	fmt.Println("Use 'occ linter schema select <version>' to change it.")
	return nil
}

// runSchemaSelect persists a template version as the current selection.
func runSchemaSelect(args []string) error {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, "Error: 'schema select' requires exactly one version argument\n\n")
		fmt.Print(usageText)
		return exitErrorf(2)
	}
	ver := normalizeTemplateVersion(args[0])
	if ver == "" {
		fmt.Fprintf(os.Stderr, "Error: unknown template version %q (available: %s)\n\n",
			args[0], strings.Join(template.AvailableVersions(), ", "))
		fmt.Print(usageText)
		return exitErrorf(2)
	}
	if err := templateconfig.Save(ver); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not save template version: %v\n", err)
		return exitErrorf(1)
	}
	fmt.Printf("Template version set to %s\n", ver)
	return nil
}

// normalizeTemplateVersion maps a user-supplied version string to a known
// template version, treating "" and "latest" as the latest template version.
// It returns "" when the version is unknown.
func normalizeTemplateVersion(v string) string {
	if v == "" || v == "latest" {
		return template.LatestVersion
	}
	for _, av := range template.AvailableVersions() {
		if v == av {
			return av
		}
	}
	return ""
}

// resolveTemplateVersion computes the template version to use for a
// validation run. Precedence: an explicit version from the command line,
// then the persisted selection, then the latest template version. When an
// explicit version is given it is also persisted as the current selection.
// It returns the resolved version and a short source description.
func resolveTemplateVersion(explicit string) (version, source string) {
	if explicit != "" {
		ver := normalizeTemplateVersion(explicit)
		if ver == "" {
			fmt.Fprintf(os.Stderr, "Error: unknown template version %q (available: %s)\n",
				explicit, strings.Join(template.AvailableVersions(), ", "))
			return "", ""
		}
		if err := templateconfig.Save(ver); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save template version: %v\n", err)
		}
		return ver, "command line"
	}
	persisted, err := templateconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read template config: %v\n", err)
	}
	if persisted != "" {
		if ver := normalizeTemplateVersion(persisted); ver != "" {
			return ver, "saved selection"
		}
	}
	return template.LatestVersion, "default (latest)"
}

// validateFlags is the raw set of flags accepted by `occ linter vali`.
type validateFlags struct {
	batch           bool
	detect          bool
	obj             bool
	parallel        int
	quiet           bool
	verbose         bool
	noColor         bool
	templateVersion string
	fix             bool
	dryRun          bool
	render          bool
}

// runValidate performs a validation run. Flag parsing, arity checking and the
// -dry-run/-fix dependency check are the command's responsibility; everything
// here is the actual work.
func runValidate(f validateFlags, path string) error {
	cfg := fixConfig{enabled: f.fix, dryRun: f.dryRun, maxDistance: 3, render: f.render}
	if f.noColor {
		colorOutput = false
	}

	resolvedVersion, versionSource := resolveTemplateVersion(f.templateVersion)
	if resolvedVersion == "" {
		return exitErrorf(2)
	}
	if err := template.SetVersion(resolvedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "Error: applying template version %s: %v\n", resolvedVersion, err)
		return exitErrorf(1)
	}
	currentSchemaVersion = resolvedVersion

	if f.verbose {
		fmt.Fprintf(os.Stderr, "DEBUG parallel: %d\n", f.parallel)
		fmt.Fprintf(os.Stderr, "DEBUG path: %s\n", path)
		fmt.Fprintf(os.Stderr, "DEBUG template-version: %s (%s)\n", resolvedVersion, versionSource)
	}

	info, err := os.Stat(path)
	if err != nil {
		if f.obj {
			printJSON([]diagnosticJSON{{File: path, Severity: valueError, Message: err.Error()}}, nil)
			return exitErrorf(1)
		}
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
		return exitErrorf(1)
	}
	scopeRoot := path
	if !info.IsDir() {
		scopeRoot = filepath.Dir(path)
	}
	if sources, e := targetschema.ReadSources(scopeRoot, info.IsDir() && f.detect); e == nil {
		cfg.schemaScope = targetschema.NewScope(scopeRoot, sources)
	}
	if info.IsDir() {
		if !f.batch && !f.detect {
			fmt.Fprintf(os.Stderr, "Error: %s is a folder; use -b to validate all YAML files or -detect to scan subfolders\n\n", path)
			fmt.Print(usageText)
			return exitErrorf(2)
		}
		if f.obj {
			return exitErrorf(validateFolderObj(path, f.detect, f.parallel, cfg))
		}
		return exitErrorf(validateFolder(path, f.detect, f.parallel, f.quiet, f.verbose, cfg))
	}

	data, rerr := os.ReadFile(path)
	if rerr != nil {
		if f.obj {
			printJSON([]diagnosticJSON{{File: path, Severity: valueError, Message: rerr.Error()}}, nil)
			return exitErrorf(1)
		}
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, rerr)
		return exitErrorf(1)
	}
	if !parser.HasOpenChoreoAPIVersion(data) {
		if f.obj {
			return exitErrorf(validateFileObj(path, nil, cfg))
		}
		errs, warns, wrote := validateFile(path, nil, cfg)
		if errs > 0 {
			fmt.Printf("Validation failed: %d error(s), %d warning(s)%s\n", errs, warns, summaryNote(cfg, countRewritten(wrote)))
			fmt.Printf("Hint: try 'occ lint vali -fix <folder path>'\n")
			return exitErrorf(1)
		}
		fmt.Printf("Validation passed: %d warning(s)%s\n", warns, summaryNote(cfg, countRewritten(wrote)))
		return nil
	}

	if f.obj {
		return exitErrorf(validateFileObj(path, buildNamesForFile(path), cfg))
	}

	errs, warns, wrote := validateFile(path, buildNamesForFile(path), cfg)
	var rendered []diagnosticJSON
	if f.render {
		// A single file cannot be rendered on its own: the bundle needs the
		// ComponentType, Component and Workload, so the whole folder that
		// holds the file is rendered instead.
		siblings, serr := parser.CollectYAMLFiles(filepath.Dir(path), false)
		if serr == nil {
			rendered = runPostRenderValidation(siblings)
			for _, d := range rendered {
				printDiagnostic(d)
				switch d.Severity {
				case valueError:
					errs++
				case valueWarning:
					warns++
				}
			}
		}
	}
	siblings2, serr2 := parser.CollectYAMLFiles(filepath.Dir(path), false)
	if serr2 == nil {
		for _, d := range distinctRuntimeFindings(runPreRenderValidation(siblings2), rendered) {
			printDiagnostic(d)
			switch d.Severity {
			case valueError:
				errs++
			case valueWarning:
				warns++
			}
		}
	}
	if errs > 0 {
		fmt.Printf("Validation failed: %d error(s), %d warning(s)%s\n", errs, warns, summaryNote(cfg, countRewritten(wrote)))
		fmt.Printf("Hint: try 'occ lint vali -fix <folder path>'\n")
		return exitErrorf(1)
	}
	fmt.Printf("Validation passed: %d warning(s)%s\n", warns, summaryNote(cfg, countRewritten(wrote)))
	return nil
}

// RunValidate validates a single path and prints a human-readable report
// to stdout (like `occ linter vali`), returning nil if clean and an error if
// validation finds issues or fails.
func RunValidate(path string) error {
	err, warns, wrote := validateFile(path, buildNamesForFile(path), fixConfig{})
	if err > 0 {
		fmt.Printf("Validation failed: %d error(s), %d warning(s)%s\n", err, warns, summaryNote(fixConfig{}, countRewritten(wrote)))
		fmt.Printf("Hint: try 'occ lint vali -fix %s'\n", path)
		return ErrFindings
	}
	fmt.Printf("Validation passed: %d warning(s)%s\n", warns, summaryNote(fixConfig{}, countRewritten(wrote)))
	return nil
}

// runPostRenderValidation renders the OpenChoreo documents found in the given
// files as one bundle and reports the post-render diagnostics. The bundle needs
// the ComponentType, the Component and the Workload together, so a single-file
// run has to be given its siblings rather than just the file it was asked
// about. It returns the diagnostics with the file they belong to; a diagnostic
// that cannot be attributed to one document is attached to the first file.
func runPostRenderValidation(files []string) []diagnosticJSON {
	findings := postrendervalidator.ValidateFiles(context.Background(), files)
	if len(findings) == 0 {
		return nil
	}
	out := make([]diagnosticJSON, 0, len(findings))
	for _, f := range findings {
		out = append(out, diagnosticJSON{
			File:     f.Path,
			Severity: f.Severity,
			Code:     f.Code,
			Line:     f.Line,
			Column:   f.Column,
			Message:  f.Message,
		})
	}
	return out
}

func runPreRenderValidation(files []string) []diagnosticJSON {
	findings := schemavalidator.ValidateFiles(context.Background(), files)
	if len(findings) == 0 {
		return nil
	}
	out := make([]diagnosticJSON, 0, len(findings))
	for _, f := range findings {
		out = append(out, diagnosticJSON{
			File:     f.Path,
			Severity: string(f.Severity),
			Code:     f.Code,
			Line:     f.Line,
			Column:   f.Column,
			Message:  f.Message,
		})
	}
	return out
}

// countRewritten turns a single file's rewritten flag into the count the
// summary reports, so one file reads the same way as a whole folder.
func countRewritten(wrote bool) int {
	if wrote {
		return 1
	}
	return 0
}

// diagnosticJSON is the internal machine-readable form of a single validation
// issue, converted into the report structure before printing.
type diagnosticJSON struct {
	File     string `json:"file"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
}

// sarifLog is the top-level -obj report object.
type sarifLog struct {
	LinterVersion string     `json:"linter_version"`
	SchemaVersion string     `json:"schema_version"`
	Runs          []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
	// Fixes lists the repairs -fix applied, omitted when none were made.
	Fixes []fixReport `json:"fixes,omitempty"`
}

// fixReport is one repair applied by -fix, as printed in the JSON report.
type fixReport struct {
	URI     string `json:"uri"`
	Code    string `json:"code"`
	Message string `json:"message"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// fixReports converts the repairs of one file into report entries.
func fixReports(path string, applied []fixer.Applied, dryRun bool) []fixReport {
	if len(applied) == 0 {
		return nil
	}
	out := make([]fixReport, 0, len(applied))
	for _, a := range applied {
		message := a.Message
		if dryRun {
			message = futureTense(message)
		}
		out = append(out, fixReport{
			URI:     path,
			Code:    a.Code,
			Message: message,
			From:    a.From,
			To:      a.To,
			Line:    a.Line,
			Column:  a.Column,
		})
	}
	return out
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name string `json:"name"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// fixConfig holds the -fix and -dry-run options of a validation run.
type fixConfig struct {
	schemaScope *targetschema.Scope
	// enabled turns on the repair pass.
	enabled bool
	// dryRun computes the repairs and reports them without writing the file.
	dryRun bool
	// maxDistance is the largest edit distance accepted for a replacement.
	maxDistance int
	// render enables post-render validation.
	render bool
}

// summaryNote qualifies a run's totals. A dry run's totals are the issues that
// survive the proposed fixes, not the ones in the per-file report. A -fix run
// that actually wrote says so, because the number of files it replaced on disk
// is the one fact the per-file report alone does not convey.
func summaryNote(cfg fixConfig, rewritten int) string {
	switch {
	case cfg.enabled && cfg.dryRun:
		return " (after the proposed fixes; no file was written)"
	case cfg.enabled && rewritten > 0:
		return fmt.Sprintf(" (%d file(s) rewritten)", rewritten)
	}
	return ""
}

// futureTense rewrites a fix message for a dry run, where nothing was written.
func futureTense(msg string) string {
	if rest, ok := strings.CutPrefix(msg, "fixed "); ok {
		return "would fix " + rest
	}
	if rest, ok := strings.CutPrefix(msg, "normalized "); ok {
		return "would normalize " + rest
	}
	return msg
}

// loadResults validates YAML content and returns its per-document results. When
// names is non-nil, name references that do not resolve to a resource in the
// workspace are reported as warnings. Diagnostics suppressed by an
// # occ:ignore comment are dropped.
func loadResults(path string, data []byte, names map[string]bool, resolvers ...targetschema.Resolver) ([]clivalidator.DocumentResult, error) {
	var results []clivalidator.DocumentResult
	var err error
	if names != nil {
		results, err = clivalidator.ValidateYAMLWithNames(data, names, resolvers...)
	} else {
		results, err = clivalidator.ValidateYAML(data, resolvers...)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	supp := suppress.New(data)
	for i := range results {
		kept := results[i].Diagnostics[:0]
		for _, d := range results[i].Diagnostics {
			if supp.Suppressed(d.Range.Start.Line, d.Code) {
				continue
			}
			kept = append(kept, d)
		}
		results[i].Diagnostics = kept
	}
	return results, nil
}

// diagnosticsJSON converts per-document results into JSON entries for a file.
func diagnosticsJSON(path string, results []clivalidator.DocumentResult) []diagnosticJSON {
	out := make([]diagnosticJSON, 0, len(results))
	for _, res := range results {
		for _, d := range res.Diagnostics {
			out = append(out, diagnosticJSON{
				File:     path,
				Severity: severityName(d.Severity),
				Code:     d.Code,
				Line:     d.Range.Start.Line,
				Column:   d.Range.Start.Column,
				Message:  d.Message,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fixFile repairs a file's typos and formatting. It returns the fixed content
// and the repairs made. When the run is a dry run the file is left untouched,
// and original holds the content the diff is rendered against. wrote reports
// whether the file was actually replaced on disk, which is the one fact a
// -fix run must not leave ambiguous.
func fixFile(path string, original []byte, cfg fixConfig) (fixed []byte, applied []fixer.Applied, skipped []string, wrote bool, err error) {
	supp := suppress.New(original)
	res, err := fixer.Fix(original, fixer.Options{
		MaxDistance: cfg.maxDistance,
		Format:      true,
		Skip:        supp.Suppressed,
	})
	if err != nil {
		// A file that does not parse has no positions to repair; the parse
		// error is reported by the validation pass that follows.
		return original, nil, nil, false, nil
	}
	if cfg.dryRun {
		return res.Data, res.Applied, res.Skipped, false, nil
	}
	wrote = !bytes.Equal(res.Data, original)
	if wrote {
		if err := writeFileAtomic(path, res.Data); err != nil {
			return original, nil, nil, false, err
		}
	}
	return res.Data, res.Applied, res.Skipped, wrote, nil
}

// writeFileAtomic writes content to path through a temporary file in the same
// directory, so an interrupted run cannot leave a half-written YAML file, and
// keeps the original file mode.
func writeFileAtomic(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".occ-fix-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// validateFileObj validates a single file and prints the JSON result.
func validateFileObj(path string, names map[string]bool, cfg fixConfig) int {
	out := safeAnalyze(path, func() fileOutcome { return analyzeFile(path, names, cfg) })
	if out.err != nil {
		printJSON([]diagnosticJSON{{File: path, Severity: valueError, Message: out.err.Error()}}, nil)
		return 1
	}
	printJSON(out.diags, fixReports(out.path, out.fixes, out.dryRun))
	if out.errs > 0 {
		return 1
	}
	return 0
}

// validateFolderObj validates the YAML files under a folder and prints the
// aggregated JSON result.
func validateFolderObj(folder string, recursive bool, workers int, cfg fixConfig) int {
	if cfg.schemaScope == nil {
		if sources, e := targetschema.ReadSources(folder, recursive); e == nil {
			cfg.schemaScope = targetschema.NewScope(folder, sources)
		}
	}
	info, err := os.Stat(folder)
	if err != nil {
		printJSON([]diagnosticJSON{{File: folder, Severity: valueError, Message: err.Error()}}, nil)
		return 1
	}
	if !info.IsDir() {
		return validateFileObj(folder, buildNamesForFile(folder), cfg)
	}

	names, err := clivalidator.ResourceNames(folder, recursive)
	if err != nil {
		printJSON([]diagnosticJSON{{File: folder, Severity: valueError, Message: err.Error()}}, nil)
		return 1
	}

	files, err := parser.CollectYAMLFiles(folder, recursive)
	if err != nil {
		printJSON([]diagnosticJSON{{File: folder, Severity: valueError, Message: err.Error()}}, nil)
		return 1
	}

	analyze := func(path string) fileOutcome {
		return safeAnalyze(path, func() fileOutcome { return analyzeFile(path, names, cfg) })
	}
	outcomes := parallel.Run(files, workers, analyze)

	var all []diagnosticJSON
	var allFixes []fixReport
	failed := false
	for _, out := range outcomes {
		if out.err != nil {
			failed = true
			all = append(all, diagnosticJSON{File: out.path, Severity: valueError, Message: out.err.Error()})
			continue
		}
		if out.errs > 0 {
			failed = true
		}
		all = append(all, out.diags...)
		allFixes = append(allFixes, fixReports(out.path, out.fixes, out.dryRun)...)
	}

	var renderDiags []diagnosticJSON
	if cfg.render {
		renderDiags = runPostRenderValidation(files)
		all = append(all, renderDiags...)
		for _, d := range renderDiags {
			if d.Severity == valueError {
				failed = true
			}
		}
	}
	preRenderDiags := distinctRuntimeFindings(runPreRenderValidation(files), renderDiags)
	all = append(all, preRenderDiags...)
	for _, d := range preRenderDiags {
		if d.Severity == valueError {
			failed = true
		}
	}

	printJSON(all, allFixes)
	if failed {
		return 1
	}
	return 0
}

// buildNamesForFile builds a workspace resource-name index for the whole repo
// that contains the given file, so single-file validation can still flag
// references to resources defined in sibling folders. It walks up from the
// file to the git root (or the filesystem root when not in a git repo).
func buildNamesForFile(file string) map[string]bool {
	dir := filepath.Dir(file)
	root := findRepoRoot(dir)
	names, _ := clivalidator.ResourceNames(root, true)
	return names
}

// findRepoRoot returns the git working-tree root for dir when available;
// otherwise it walks up to the filesystem root.
func findRepoRoot(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		if root := strings.TrimSpace(string(out)); root != "" {
			return root
		}
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for {
		parent := filepath.Dir(abs)
		if parent == abs {
			return abs
		}
		abs = parent
	}
}

// printJSON prints a validation report to the active report writer.
func printJSON(data []diagnosticJSON, fixes []fixReport) {
	if data == nil {
		data = []diagnosticJSON{}
	}
	out, err := json.MarshalIndent(buildSARIF(data, fixes), "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		return
	}
	fmt.Fprintln(reportWriter, string(out))
}

func buildSARIF(data []diagnosticJSON, fixes []fixReport) sarifLog {
	results := make([]sarifResult, 0, len(data))
	for _, d := range data {
		loc := sarifLocation{PhysicalLocation: sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: d.File}}}
		if d.Line > 0 && d.Column > 0 {
			loc.PhysicalLocation.Region = &sarifRegion{StartLine: d.Line, StartColumn: d.Column}
		}
		results = append(results, sarifResult{
			RuleID:    d.Code,
			Level:     sarifLevel(d.Severity),
			Message:   sarifMessage{Text: d.Message},
			Locations: []sarifLocation{loc},
		})
	}
	return sarifLog{
		LinterVersion: linterVersion(),
		SchemaVersion: "openchoreo " + currentSchemaVersion,
		Runs: []sarifRun{{
			Tool:    sarifTool{Driver: sarifDriver{Name: "occ linter"}},
			Results: results,
			Fixes:   fixes,
		}},
	}
}

// sarifLevel maps a diagnostic severity to a SARIF result level.
func sarifLevel(sev string) string {
	switch sev {
	case valueError, valueWarning:
		return sev
	case "info", "hint":
		return "note"
	default:
		return "note"
	}
}

// severityName maps a ruleengine severity to its string form.
func severityName(s ruleengine.Severity) string {
	switch s {
	case ruleengine.SeverityError:
		return valueError
	case ruleengine.SeverityWarning:
		return valueWarning
	case ruleengine.SeverityInfo:
		return "info"
	case ruleengine.SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// fileOutcome is the per-file result of analysis, produced without any printing
// so that folder validation can run files in parallel and print them in order.
type fileOutcome struct {
	path         string
	results      []clivalidator.DocumentResult
	diags        []diagnosticJSON
	fixes        []fixer.Applied
	skipped      []string
	errs         int
	warns        int
	err          error
	isOpenChoreo bool
	// original and fixed hold the file content before and after a -fix run.
	// They are only set when fixing ran, and drive the -dry-run diff.
	original []byte
	fixed    []byte
	dryRun   bool
	// wrote records that this file's content was replaced on disk, so the
	// summary can count the files a -fix run actually rewrote.
	wrote bool
}

// analyzeFile reads, optionally repairs, and validates a single YAML file. It
// performs no output; errors are reported through the outcome. When -fix ran,
// the diagnostics describe the repaired content, so the report shows what is
// left to do, while a dry run reports the file as it is on disk.
func analyzeFile(path string, names map[string]bool, cfg fixConfig) fileOutcome {
	out := fileOutcome{path: path}

	var data []byte
	var err error
	if cfg.schemaScope != nil {
		data, _ = cfg.schemaScope.Content(path)
	}
	if data == nil {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		out.errs = 1
		out.err = err
		return out
	}
	if !parser.HasOpenChoreoAPIVersion(data) {
		out.errs = 1
		return out
	}
	out.isOpenChoreo = true

	content := data
	if cfg.enabled {
		fixed, applied, skipped, wrote, ferr := fixFile(path, data, cfg)
		if ferr != nil {
			out.errs = 1
			out.err = fmt.Errorf("writing %s: %w", path, ferr)
			return out
		}
		out.fixes = applied
		out.skipped = skipped
		out.original = data
		out.fixed = fixed
		out.dryRun = cfg.dryRun
		out.wrote = wrote
		if !cfg.dryRun {
			content = fixed
		}
	}

	results, err := loadResults(path, content, names, cfg.schemaScope)
	if err != nil {
		out.errs = 1
		out.err = err
		return out
	}
	out.results = results
	out.diags = diagnosticsJSON(path, results)

	// A dry run leaves the file alone, so its report describes the file as it
	// is on disk; the exit code still reflects the state the proposed fixes
	// would leave behind, so CI fails only on issues that survive the fix.
	counted := results
	if cfg.enabled && cfg.dryRun {
		if post, perr := loadResults(path, out.fixed, names, cfg.schemaScope); perr == nil {
			counted = post
		}
	}
	for _, res := range counted {
		for _, d := range res.Diagnostics {
			switch d.Severity {
			case ruleengine.SeverityError:
				out.errs++
			case ruleengine.SeverityWarning:
				out.warns++
			}
		}
	}
	return out
}

// printFixReport prints what -fix did to a file: the list of repairs it made,
// or the diff it proposes when the run is a dry run. It prints nothing when the
// run had nothing to repair.
func printFixReport(out fileOutcome, includePath bool) {
	if len(out.fixes) == 0 && len(out.skipped) == 0 {
		return
	}

	if out.dryRun {
		if diff := fixer.Diff(out.original, out.fixed, out.path); diff != "" {
			fmt.Print(diff)
		}
	}

	if len(out.fixes) > 0 {
		for _, f := range out.fixes {
			msg := f.Message
			if out.dryRun {
				msg = futureTense(msg)
			}
			var line string
			switch {
			case includePath:
				line = fmt.Sprintf("  %s %s: %s", okMarker(), out.path, msg)
			case f.Line > 0:
				line = fmt.Sprintf("  %s %s (%d:%d)", okMarker(), msg, f.Line, f.Column)
			default:
				line = fmt.Sprintf("  %s %s", okMarker(), msg)
			}
			fmt.Println(line)
		}
		past := "fixed"
		if out.dryRun {
			past = "would be fixed"
		}
		fmt.Printf("  %s %d issue(s) %s in %s\n", okMarker(), len(out.fixes), past, out.path)
	}

	// A candidate that was left alone is reported too, so a typo that is still
	// in the file is never mistaken for a missed repair.
	for _, reason := range out.skipped {
		fmt.Printf("  %s skipped %s\n", iconFor(ruleengine.SeverityWarning), reason)
	}
	if out.dryRun {
		fmt.Println("  (dry run: no file was written)")
	}
}

// printFileOutcome prints one analyzed file's report. When includePath is true
// (batch mode), the report is only printed if the file has issues and every
// diagnostic line is prefixed with the file path. It returns the outcome's
// error and warning counts, and whether the file was rewritten on disk.
// printDiagnostic prints a single diagnostic in the same shape as the
// per-file report, so a post-render finding reads like any other finding.
func printDiagnostic(d diagnosticJSON) {
	sev := ruleengine.SeverityError
	if d.Severity == valueWarning {
		sev = ruleengine.SeverityWarning
	}
	loc := d.File
	if d.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Column)
	}
	fmt.Printf("  %s %s [%s] %s\n", iconFor(sev), loc, d.Code, d.Message)
}

// hasErrorDiag reports whether any of the given diagnostics is an error.
func hasErrorDiag(diags []diagnosticJSON) bool {
	for _, d := range diags {
		if d.Severity == valueError {
			return true
		}
	}
	return false
}

func printFileOutcome(out fileOutcome, includePath bool) (errors, warnings int, wrote bool) {
	if out.err != nil {
		fmt.Fprintf(os.Stderr, "Error %v\n", out.err)
		return out.errs, out.warns, out.wrote
	}
	if !out.isOpenChoreo {
		if !includePath {
			fmt.Printf("File: %s\n  %s [not-openchoreo] not an OpenChoreo CRD file: expected apiVersion: openchoreo.dev/v1alpha1 (or apiVersion: openchoreo.dev/...)\n\n", out.path, iconFor(ruleengine.SeverityError))
		}
		return out.errs, out.warns, out.wrote
	}
	if includePath && out.errs == 0 && out.warns == 0 && len(out.fixes) == 0 && len(out.results) > 0 {
		return 0, 0, out.wrote
	}

	fmt.Printf("File: %s\n\n", out.path)

	printFixReport(out, includePath)

	if len(out.results) == 0 {
		fmt.Printf("  %s No documents found in file\n", iconFor(ruleengine.SeverityWarning))
		fmt.Println()
	}

	for i, res := range out.results {
		kind := res.Kind
		if kind == "" {
			kind = "unknown"
		}
		fmt.Printf("Document %d: %s\n", i, kind)

		if len(res.Diagnostics) == 0 {
			fmt.Printf("  %s No issues found\n", okMarker())
		} else {
			for _, d := range res.Diagnostics {
				pos := fmt.Sprintf("%d:%d", d.Range.Start.Line, d.Range.Start.Column)
				if includePath {
					fmt.Printf("  %s %s:%s [%s] %s\n", iconFor(d.Severity), out.path, pos, d.Code, d.Message)
				} else {
					fmt.Printf("  %s %s [%s] %s\n", iconFor(d.Severity), pos, d.Code, d.Message)
				}
			}
		}
		fmt.Println()
	}
	return out.errs, out.warns, out.wrote
}

// validateFile validates a single YAML file and returns the number of errors
// and warnings found, plus whether the file was rewritten on disk. When
// includePath is true (batch mode), the file report is only printed if the
// file has issues, and each diagnostic line is prefixed with the file path so
// errors are self-contained.
func validateFile(path string, names map[string]bool, cfg fixConfig) (errors, warnings int, wrote bool) {
	includePath := false

	return printFileOutcome(safeAnalyze(path, func() fileOutcome { return analyzeFile(path, names, cfg) }), includePath)
}

// phaseTiming holds the wall-clock time spent in each phase of a folder run.
// The phases add up to roughly the total run time and let the parallel
// validation slice be compared directly across worker counts.
type phaseTiming struct {
	scan     time.Duration // folder walk + apiVersion gate (reads every file)
	index    time.Duration // cross-file reference map (reads every file again)
	validate time.Duration // parallel.Run analysis pool
	print    time.Duration // ordered result printing
}

// fmtMs renders a duration for the phase breakdown: whole milliseconds for
// phases of 1ms or more (e.g. "80ms", "1.452s") and a µs/duration string for
// anything under a millisecond so fast phases stay visible.
func fmtMs(d time.Duration) string {
	if d < time.Millisecond {
		return d.String()
	}
	return d.Round(time.Millisecond).String()
}

// breakdown formats a machine-friendly one-line summary of the phase timing.
func (p phaseTiming) breakdown() string {
	return fmt.Sprintf("[scan %s | index %s | validate %s | print %s]",
		fmtMs(p.scan), fmtMs(p.index), fmtMs(p.validate), fmtMs(p.print))
}

// validateFolder validates the .yaml/.yml files under the given folder. When
// recursive is true it scans every subfolder using path/filepath; otherwise it
// only looks at the files directly inside the folder. Files are analyzed in
// parallel with up to workers goroutines, but reports are printed in file order.
// It returns the process exit code.
func validateFolder(folder string, recursive bool, workers int, quiet, verbose bool, cfg fixConfig) int {
	if cfg.schemaScope == nil {
		if sources, e := targetschema.ReadSources(folder, recursive); e == nil {
			cfg.schemaScope = targetschema.NewScope(folder, sources)
		}
	}
	start := time.Now()
	info, err := os.Stat(folder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", folder, err)
		return 1
	}
	if !info.IsDir() {
		errs, warns, wrote := validateFile(folder, buildNamesForFile(folder), cfg)
		if errs > 0 {
			fmt.Printf("Validation failed: %d error(s), %d warning(s)%s\n", errs, warns, summaryNote(cfg, countRewritten(wrote)))
			return 1
		}
		fmt.Printf("Validation passed: %d warning(s)%s\n", warns, summaryNote(cfg, countRewritten(wrote)))
		return 0
	}

	files, err := parser.CollectYAMLFiles(folder, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking %s: %v\n", folder, err)
		return 1
	}

	if len(files) == 0 {
		fmt.Printf("No OpenChoreo YAML files found in %s\n", folder)
		return 0
	}
	scan := time.Since(start)

	t := time.Now()
	names, err := clivalidator.ResourceNames(folder, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing resources in %s: %v\n", folder, err)
		return 1
	}
	index := time.Since(t)
	if verbose {
		fmt.Fprintf(os.Stderr, "indexed %d resource name(s) in %s\n", len(names), folder)
	}

	if !quiet {
		fmt.Printf("Validating %d YAML file(s) in %s\n\n", len(files), folder)
	}

	t = time.Now()
	analyze := func(path string) fileOutcome {
		return safeAnalyze(path, func() fileOutcome { return analyzeFile(path, names, cfg) })
	}
	outcomes := parallel.Run(files, workers, analyze)
	preRenderDiags := runPreRenderValidation(files)
	validate := time.Since(t)

	var renderDiags []diagnosticJSON
	if cfg.render {
		// The bundle needs the ComponentType, Component and Workload together,
		// so the whole scanned set is rendered as one bundle and the findings
		// are reported after the per-file report.
		renderDiags = runPostRenderValidation(files)
	}

	preRenderDiags = distinctRuntimeFindings(preRenderDiags, renderDiags)
	t = time.Now()
	var totalErrors, totalWarnings, failed, rewritten int
	failedFiles := make(map[string]bool)
	for _, out := range outcomes {
		errs, warns, wrote := printFileOutcome(out, true)
		totalErrors += errs
		totalWarnings += warns
		rewritten += countRewritten(wrote)
		if errs > 0 {
			failedFiles[out.path] = true
			failed++
		}
	}
	if len(preRenderDiags) > 0 {
		fmt.Printf("Pre-render validation:\n\n")
		for _, d := range preRenderDiags {
			printDiagnostic(d)
			switch d.Severity {
			case valueError:
				totalErrors++
				if !failedFiles[d.File] {
					failedFiles[d.File] = true
					failed++
				}
			case valueWarning:
				totalWarnings++
			}
		}
		fmt.Println()
	}
	if len(renderDiags) > 0 {
		fmt.Printf("Post-render validation:\n\n")
		for _, d := range renderDiags {
			printDiagnostic(d)
			switch d.Severity {
			case valueError:
				totalErrors++
			case valueWarning:
				totalWarnings++
			}
		}
		fmt.Println()
		if hasErrorDiag(renderDiags) {
			failed++
		}
	}
	printTime := time.Since(t)

	phases := phaseTiming{scan: scan, index: index, validate: validate, print: printTime}
	if totalErrors > 0 {
		fmt.Printf("Validation failed: %d file(s) with issues, %d error(s), %d warning(s) across %d file(s)%s\n",
			failed, totalErrors, totalWarnings, len(files), summaryNote(cfg, rewritten))
		fmt.Fprintf(os.Stderr, "Total validation time: %s (%d workers)\n", time.Since(start).Round(time.Millisecond), workers)
		fmt.Fprintf(os.Stderr, "%s\n", phases.breakdown())
		return 1
	}
	fmt.Printf("Validation passed: %d file(s), %d warning(s)%s\n", len(files), totalWarnings, summaryNote(cfg, rewritten))
	fmt.Fprintf(os.Stderr, "Total validation time: %s (%d workers)\n", time.Since(start).Round(time.Millisecond), workers)
	fmt.Fprintf(os.Stderr, "%s\n", phases.breakdown())
	return 0
}

func iconFor(s ruleengine.Severity) string {
	if !colorOutput {
		switch s {
		case ruleengine.SeverityError:
			return "ERROR"
		case ruleengine.SeverityWarning:
			return "WARN"
		case ruleengine.SeverityInfo:
			return "INFO"
		case ruleengine.SeverityHint:
			return "HINT"
		default:
			return "OK"
		}
	}
	switch s {
	case ruleengine.SeverityError:
		return "✗"
	case ruleengine.SeverityWarning:
		return "⚠"
	case ruleengine.SeverityInfo:
		return "ℹ"
	case ruleengine.SeverityHint:
		return "➜"
	default:
		return "?"
	}
}

// okMarker returns the glyph (or plain text) used to mark a clean result.
func okMarker() string {
	if !colorOutput {
		return "OK"
	}
	return "✓"
}

// Preserve pipeline errors and omit only new runtime findings already represented
// by that same resource's rendering failure.
func distinctRuntimeFindings(findings, rendered []diagnosticJSON) []diagnosticJSON {
	out := make([]diagnosticJSON, 0, len(findings))
	for _, f := range findings {
		duplicate := false
		if f.Code == "pre-render-validation-failed" {
			for _, prior := range rendered {
				if prior.File == f.File && prior.Code == "post-render-error" && strings.Contains(prior.Message, f.Message) {
					duplicate = true
					break
				}
			}
		}
		if !duplicate {
			out = append(out, f)
		}
	}
	return out
}
