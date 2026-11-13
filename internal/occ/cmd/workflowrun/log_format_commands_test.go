// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/auditlogs"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/component"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Exercise flag registration and forwarding through each command to the shared log reader.
func TestWorkflowLogCommands_Formats(t *testing.T) {
	for _, format := range []string{"json", "custom-columns=timestamp,log"} {
		t.Run(format, func(t *testing.T) {
			for _, flag := range []string{"--output", "-o"} {
				t.Run(flag, func(t *testing.T) {
					for _, tc := range []struct {
						name   string
						newCmd func(client.NewClientFunc) *cobra.Command
						path   []string
					}{
						{"workflow", workflow.NewWorkflowCmd, []string{"logs"}},
						{"workflowrun", workflowrun.NewWorkflowRunCmd, []string{"logs"}},
						{"clusterworkflow", clusterworkflow.NewClusterWorkflowCmd, []string{"logs"}},
						{"component workflow", component.NewComponentCmd, []string{"workflow", "logs"}},
						{"component workflowrun", component.NewComponentCmd, []string{"workflowrun", "logs"}},
					} {
						t.Run(tc.name, func(t *testing.T) {
							timestamp := time.Date(2026, 11, 10, 8, 21, 4, 512000000, time.UTC)
							mc := mocks.NewMockInterface(t)
							mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
								&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
							mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
								[]gen.WorkflowRunLogEntry{
									{Timestamp: &timestamp, Log: "first\nsecond\t\"quoted\" <tag>"},
									{Log: "no timestamp"},
								}, nil)
							root := tc.newCmd(func() (client.Interface, error) { return mc, nil })
							cmd, _, err := root.Find(tc.path)
							require.NoError(t, err)
							require.NotNil(t, cmd.Flags().Lookup("output"))
							assert.Equal(t, "text", cmd.Flags().Lookup("output").DefValue)
							args := []string{"--namespace", "ns", flag, format}
							if tc.name != "workflowrun" {
								args = append(args, "--workflowrun", "run-1")
							}
							require.NoError(t, cmd.ParseFlags(args))
							out := testutil.CaptureStdout(t, func() {
								require.NoError(t, cmd.RunE(cmd, []string{"run-1"}))
							})
							if format != "json" {
								assert.Equal(t, "2026-11-10T08:21:04.512Z\tfirst\nsecond\t\"quoted\" <tag>\n-\tno timestamp\n", out)
								return
							}
							lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
							require.Len(t, lines, 2)
							assert.JSONEq(t, `{"timestamp":"2026-11-10T08:21:04.512Z","log":"first\nsecond\t\"quoted\" <tag>"}`, lines[0])
							assert.JSONEq(t, `{"log":"no timestamp"}`, lines[1])
						})
					}
				})
			}
		})
	}
}

func TestLogCommands_InvalidOutputBeforeClientCreation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		newCmd func(client.NewClientFunc) *cobra.Command
		path   []string
	}{
		{"component", component.NewComponentCmd, []string{"logs"}},
		{"workflow", workflow.NewWorkflowCmd, []string{"logs"}},
		{"workflowrun", workflowrun.NewWorkflowRunCmd, []string{"logs"}},
		{"clusterworkflow", clusterworkflow.NewClusterWorkflowCmd, []string{"logs"}},
		{"component workflow", component.NewComponentCmd, []string{"workflow", "logs"}},
		{"component workflowrun", component.NewComponentCmd, []string{"workflowrun", "logs"}},
		{"observabilityplane", observabilityplane.NewObservabilityPlaneCmd, []string{"logs"}},
		{"clusterobservabilityplane", clusterobservabilityplane.NewClusterObservabilityPlaneCmd, []string{"logs"}},
		{"auditlogs", auditlogs.NewAuditLogsCmd, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, flag := range []string{"-o", "--output"} {
				for _, format := range []struct{ value, wantErr string }{
					{"yaml", `invalid log format "yaml": must be text, json, or 'custom-columns=FIELD,...'`},
					{"custom-columns=timestamp,,log", `invalid custom column "": use non-empty dot-separated attribute names`},
					{"custom-columns=TIME:.timestamp,log", `invalid custom column "TIME:.timestamp": named columns (NAME:path) are not supported; use 'custom-columns=FIELD,...'`},
				} {
					t.Run(flag+"/"+format.value, func(t *testing.T) {
						root := tc.newCmd(func() (client.Interface, error) {
							t.Fatal("invalid output must be rejected before creating an API client")
							return nil, nil
						})
						cmd, _, err := root.Find(tc.path)
						require.NoError(t, err)
						require.NoError(t, cmd.ParseFlags([]string{flag, format.value}))
						args := []string{"nope"}
						if tc.path == nil {
							args = nil
						}
						assert.EqualError(t, cmd.RunE(cmd, args), format.wantErr)
					})
				}
			}
		})
	}
}

func TestLogCommands_OutputHelp(t *testing.T) {
	const usagePrefix = "Output format: text, json (one object per line), or custom-columns=FIELD,...; e.g. custom-columns="
	const logExampleColumns = "timestamp,log"
	for _, tc := range []struct {
		name           string
		newCmd         func(client.NewClientFunc) *cobra.Command
		path           []string
		exampleColumns string
	}{
		{"component", component.NewComponentCmd, []string{"logs"}, logExampleColumns},
		{"workflow", workflow.NewWorkflowCmd, []string{"logs"}, logExampleColumns},
		{"workflowrun", workflowrun.NewWorkflowRunCmd, []string{"logs"}, logExampleColumns},
		{"clusterworkflow", clusterworkflow.NewClusterWorkflowCmd, []string{"logs"}, logExampleColumns},
		{"component workflow", component.NewComponentCmd, []string{"workflow", "logs"}, logExampleColumns},
		{"component workflowrun", component.NewComponentCmd, []string{"workflowrun", "logs"}, logExampleColumns},
		{"observabilityplane", observabilityplane.NewObservabilityPlaneCmd, []string{"logs"}, logExampleColumns},
		{"clusterobservabilityplane", clusterobservabilityplane.NewClusterObservabilityPlaneCmd, []string{"logs"}, logExampleColumns},
		{"auditlogs", auditlogs.NewAuditLogsCmd, nil, "event_time,action,result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := usagePrefix + tc.exampleColumns
			root := tc.newCmd(func() (client.Interface, error) {
				t.Fatal("help must not create an API client")
				return nil, nil
			})
			cmd, _, err := root.Find(tc.path)
			require.NoError(t, err)
			output := cmd.Flags().Lookup("output")
			require.NotNil(t, output)
			assert.Equal(t, "o", output.Shorthand)
			assert.Equal(t, "text", output.DefValue)
			assert.Equal(t, usage, output.Usage)
			var help bytes.Buffer
			cmd.SetOut(&help)
			require.NoError(t, cmd.Help())
			assert.Contains(t, help.String(), "-o, --output string")
			assert.Contains(t, help.String(), usage+` (default "text")`)
		})
	}
}
