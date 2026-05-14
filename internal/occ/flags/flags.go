// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package flags provides helper functions for registering and reading commonly
// used CLI flags across occ subcommands.
package flags

import "github.com/spf13/cobra"

// Mode constants for the --mode flag.
const (
	ModeAPIServer  = "api-server"
	ModeFileSystem = "file-system"
)

// --- Namespace ---

func AddNamespace(cmd *cobra.Command) {
	cmd.Flags().StringP("namespace", "n", "", "Name of the namespace (e.g., acme-corp)")
}

func GetNamespace(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("namespace")
	return val
}

// --- Project ---

func AddProject(cmd *cobra.Command) {
	cmd.Flags().StringP("project", "p", "", "Name of the project (e.g., online-store)")
}

func GetProject(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("project")
	return val
}

// --- Component ---

func AddComponent(cmd *cobra.Command) {
	cmd.Flags().StringP("component", "c", "", "Name of the component (e.g., product-catalog)")
}

func GetComponent(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("component")
	return val
}

// --- Resource ---

func AddResource(cmd *cobra.Command) {
	cmd.Flags().String("resource", "", "Name of the resource (e.g., analytics-db)")
}

func GetResource(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("resource")
	return val
}

// --- Environment ---

func AddEnvironment(cmd *cobra.Command) {
	cmd.Flags().String("env", "", "Environment where the component will be deployed (e.g., dev, staging, production)")
}

func GetEnvironment(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("env")
	return val
}

// --- Follow ---

func AddFollow(cmd *cobra.Command) {
	cmd.Flags().BoolP("follow", "f", false, "Follow the logs of the specified resource")
}

func GetFollow(cmd *cobra.Command) bool {
	val, _ := cmd.Flags().GetBool("follow")
	return val
}

// --- Since ---

func AddSince(cmd *cobra.Command) {
	cmd.Flags().String("since", "", "Only return logs newer than a relative duration like 5m, 1h, or 24h")
}

func GetSince(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("since")
	return val
}

// --- Tail ---

func AddTail(cmd *cobra.Command) {
	cmd.Flags().Int("tail", 0, "Number of lines to show from the end of logs")
}

func GetTail(cmd *cobra.Command) int {
	val, _ := cmd.Flags().GetInt("tail")
	return val
}

// --- Mode ---

func AddMode(cmd *cobra.Command) {
	cmd.Flags().String("mode", "", "Operational mode: 'api-server' (default) or 'file-system'")
}

func GetMode(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("mode")
	return val
}

// --- RootDir ---

func AddRootDir(cmd *cobra.Command) {
	cmd.Flags().String("root-dir", "", "Root directory path for file-system mode (defaults to current directory)")
}

func GetRootDir(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("root-dir")
	return val
}

// --- OutputPath ---

func AddOutputPath(cmd *cobra.Command) {
	cmd.Flags().StringP("output-path", "o", "", "Custom output directory path")
}

func GetOutputPath(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("output-path")
	return val
}

// --- DryRun ---

func AddDryRun(cmd *cobra.Command) {
	cmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
}

func GetDryRun(cmd *cobra.Command) bool {
	val, _ := cmd.Flags().GetBool("dry-run")
	return val
}

// --- Set (key=value overrides) ---

func AddSet(cmd *cobra.Command) {
	cmd.Flags().StringArray("set", nil, "Set override values (format: type.path=value)")
}

func GetSet(cmd *cobra.Command) []string {
	val, _ := cmd.Flags().GetStringArray("set")
	return val
}

// --- TargetEnv ---

func AddTargetEnv(cmd *cobra.Command) {
	cmd.Flags().StringP("target-env", "e", "", "Target environment for the release binding")
}

func GetTargetEnv(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("target-env")
	return val
}

// --- Release ---

func AddRelease(cmd *cobra.Command) {
	cmd.Flags().String("release", "", "Component release name to deploy to lowest environment")
}

func GetRelease(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("release")
	return val
}

// --- To ---

func AddTo(cmd *cobra.Command) {
	cmd.Flags().String("to", "", "Target environment to promote to")
}

func GetTo(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("to")
	return val
}

// --- WorkflowRun ---

func AddWorkflowRun(cmd *cobra.Command) {
	cmd.Flags().String("workflowrun", "", "Workflow run name (defaults to latest run)")
}

func GetWorkflowRun(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("workflowrun")
	return val
}

// --- OutputFile (single-file output, -o shorthand) ---

func AddOutputFile(cmd *cobra.Command) {
	cmd.Flags().StringP("output-file", "o", "", "Write output to specified file instead of stdout")
}

func GetOutputFile(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("output-file")
	return val
}

// --- UsePipeline ---

func AddUsePipeline(cmd *cobra.Command) {
	cmd.Flags().String("use-pipeline", "", "Deployment pipeline name for environment validation")
}

func GetUsePipeline(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("use-pipeline")
	return val
}

// --- ComponentRelease ---

func AddComponentRelease(cmd *cobra.Command) {
	cmd.Flags().String("component-release", "", "Explicit component release name (only valid with --project and --component)")
}

func GetComponentRelease(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("component-release")
	return val
}

// --- All ---

func AddAll(cmd *cobra.Command) {
	cmd.Flags().Bool("all", false, "Process all resources")
}

func GetAll(cmd *cobra.Command) bool {
	val, _ := cmd.Flags().GetBool("all")
	return val
}

// --- ControlPlane (config context flag) ---

func AddControlPlane(cmd *cobra.Command) {
	cmd.Flags().String("controlplane", "", "Control plane name for this context")
}

func GetControlPlane(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("controlplane")
	return val
}

// --- Credentials (config context flag) ---

func AddCredentials(cmd *cobra.Command) {
	cmd.Flags().String("credentials", "", "Credentials name for this context")
}

func GetCredentials(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("credentials")
	return val
}

// --- URL (config controlplane flag) ---

func AddURL(cmd *cobra.Command) {
	cmd.Flags().String("url", "", "Control plane URL")
}

func GetURL(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("url")
	return val
}
