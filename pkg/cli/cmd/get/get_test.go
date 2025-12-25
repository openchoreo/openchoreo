package get

import (
	"testing"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func TestGetCmd_FlagParsing_Limit_DefaultsToZero(t *testing.T) {
	var captured api.GetParams

	cmd := (&builder.CommandBuilder{
		Command: constants.ListOrganization,
		Flags:   []flags.Flag{flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			captured = api.GetParams{
				OutputFormat: fg.GetString(flags.Output),
				Name:         "",
				Limit:        limit,
			}
			return nil
		},
	}).Build()

	// Execute with no limit flag
	cmd.SetArgs([]string{constants.ListOrganization.Use})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}
	if captured.Limit != 0 {
		t.Fatalf("expected default Limit==0 when no --limit provided, got %d", captured.Limit)
	}

	// Execute with explicit --limit=5
	cmd.SetArgs([]string{constants.ListOrganization.Use, "--limit", "5"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}
	if captured.Limit != 5 {
		t.Fatalf("expected Limit==5 when --limit=5 provided, got %d", captured.Limit)
	}

	// Execute with --all which should set Limit == 0
	cmd.SetArgs([]string{constants.ListOrganization.Use, "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}
	if captured.Limit != 0 {
		t.Fatalf("expected Limit==0 when --all provided, got %d", captured.Limit)
	}
}
