// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun_test

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/component"
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
					require.NotNil(t, cmd.Flags().Lookup("log-format"))
					assert.Equal(t, "text", cmd.Flags().Lookup("log-format").DefValue)
					args := []string{"--namespace", "ns", "--log-format", format}
					if tc.name != "workflowrun" {
						args = append(args, "--workflowrun", "run-1")
					}
					require.NoError(t, cmd.ParseFlags(args))
					out := testutil.CaptureStdout(t, func() {
						require.NoError(t, cmd.RunE(cmd, []string{"run-1"}))
					})
					if format != "json" {
						assert.Equal(t, "2026-11-10T08:21:04.512Z first\nsecond\t\"quoted\" <tag>\n- no timestamp\n", out)
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
}
