// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func newRolloutCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Run an endpoint's progressive delivery",
		Long: `Run the rollout plan in an endpoint's spec.rollout. Each step writes the ` +
			`backendOverrides of one environment's EndpointReleaseBinding through ` +
			`openchoreo-api, the same way the portal does.`,
	}
	cmd.AddCommand(
		newRolloutGetCmd(f),
		newRolloutListCmd(f),
		newRolloutStartCmd(f),
		newRolloutStepCmd(f),
		newRolloutAbortCmd(f),
		newRolloutStatusCmd(f),
		newRolloutSetWeightsCmd(f),
		newRolloutFinishCmd(f),
	)
	return cmd
}

func rolloutParams(cmd *cobra.Command, args []string) RolloutParams {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	output, _ := cmd.Flags().GetString("output")
	return RolloutParams{
		Namespace:    flags.GetNamespace(cmd),
		EndpointName: args[0],
		DryRun:       dryRun,
		Output:       output,
	}
}

func addWriteFlags(cmd *cobra.Command) {
	flags.AddNamespace(cmd)
	cmd.Flags().Bool("dry-run", false, "Show the change without writing it")
	cmd.Flags().StringP("output", "o", "", "Output format: yaml prints the resource the call writes")
}

func optionalInt(cmd *cobra.Command, name string) *int {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	v, _ := cmd.Flags().GetInt(name)
	return &v
}

func stepParams(cmd *cobra.Command, args []string) RolloutStepParams {
	back, _ := cmd.Flags().GetBool("back")
	last, _ := cmd.Flags().GetBool("last")
	return RolloutStepParams{
		RolloutParams: rolloutParams(cmd, args),
		Environment:   flags.GetEnvironment(cmd),
		To:            optionalInt(cmd, "to"),
		ExpectedStep:  optionalInt(cmd, "expect-step"),
		Back:          back,
		Last:          last,
	}
}

// envCmd is a rollout command that acts on one environment.
func envCmd(
	f client.NewClientFunc, use, short, example string,
	run func(*Endpoint, *cobra.Command, []string) error,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use + " [ENDPOINT_NAME]",
		Short:   short,
		Example: example,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return run(New(cl), cmd, args)
		},
	}
	addWriteFlags(cmd)
	flags.AddEnvironment(cmd)
	return cmd
}

func newRolloutGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [ENDPOINT_NAME]",
		Short: "Show an endpoint's rollout as a tree",
		Long: `Show the plan, each environment's step with the weights it sets and serves, ` +
			`and its progress through the steps. --watch redraws on every change.`,
		Example: `  # Watch a canary move through its steps
  occ endpoint rollout get trips-api --watch

  # Only one environment
  occ endpoint rollout get trips-api --env production`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			watch, _ := cmd.Flags().GetBool("watch")
			return New(cl).RolloutGet(RolloutGetParams{
				RolloutParams: RolloutParams{Namespace: flags.GetNamespace(cmd), EndpointName: args[0]},
				Environment:   flags.GetEnvironment(cmd),
				Watch:         watch,
				Interval:      2 * time.Second,
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	cmd.Flags().BoolP("watch", "w", false, "Redraw on every change until interrupted")
	return cmd
}

func newRolloutListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List endpoints with a rollout plan",
		Example: `  occ endpoint rollout list --namespace default`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).RolloutList(ListParams{Namespace: flags.GetNamespace(cmd), Project: flags.GetProject(cmd)})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	return cmd
}

func newRolloutStartCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start [ENDPOINT_NAME]",
		Short: "Write an endpoint's rollout plan",
		Long: `Write spec.rollout from flags, or from a file holding the rollout and the ` +
			`backends to add or replace with it.`,
		Example: `  # Add test's canary release line as a canary, in steps; testers reach it first
  occ endpoint rollout start trips-api --strategy canary --stable test \
    --candidate canary=test@canary --steps 10,20,50,100 --header x-canary=true

  # Shadow a new component with copies of the requests
  occ endpoint rollout start trips-api --strategy mirror --stable test \
    --candidate shadow=trips-go/http --steps 10,50,100

  # A/B test: the exp=b group gets test's canary release line
  occ endpoint rollout start trips-api --strategy abTest --stable test \
    --candidate b=test@canary --cookie exp=b

  # From a file
  occ endpoint rollout start trips-api -f canary.yaml`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			file, _ := cmd.Flags().GetString("file")
			strategy, _ := cmd.Flags().GetString("strategy")
			stable, _ := cmd.Flags().GetString("stable")
			candidate, _ := cmd.Flags().GetString("candidate")
			var candidates []string
			if candidate != "" {
				candidates = []string{candidate}
			}
			header, _ := cmd.Flags().GetString("header")
			cookie, _ := cmd.Flags().GetString("cookie")
			steps, err := parsePercents(cmd)
			if err != nil {
				return err
			}
			return New(cl).RolloutStart(RolloutStartParams{
				RolloutParams: rolloutParams(cmd, args),
				File:          file,
				Plan: RolloutPlanFlags{
					Strategy: strategy, Stable: stable, Candidates: candidates, Steps: steps, Header: header, Cookie: cookie,
				},
			})
		},
	}
	addWriteFlags(cmd)
	cmd.Flags().StringP("file", "f", "", "File with the rollout and the backends to add")
	cmd.Flags().String("strategy", "", "canary, blueGreen, abTest or mirror")
	cmd.Flags().String("stable", "", "The backend serving now")
	cmd.Flags().String("candidate", "",
		"The backend being rolled out: NAME of one the endpoint has, or NAME=COMPONENT[@RELEASELINE][/ENDPOINT] to add it")
	cmd.Flags().String("steps", "", "Each step's percent for canary and mirror, e.g. 5,25,100")
	cmd.Flags().String("header", "", "A header that reaches the candidates before any step, name=value (optional)")
	cmd.Flags().String("cookie", "", "abTest: the cookie of the group that gets the candidate, name=value")
	return cmd
}

func parsePercents(cmd *cobra.Command) ([]int32, error) {
	raw, _ := cmd.Flags().GetString("steps")
	if raw == "" {
		return nil, nil
	}
	var out []int32
	for _, part := range strings.Split(raw, ",") {
		p, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(part, "%")), 10, 32)
		if err != nil || p < 0 || p > 100 {
			return nil, fmt.Errorf("--steps takes percents from 0 to 100, got %q", part)
		}
		out = append(out, int32(p))
	}
	return out, nil
}

func newRolloutStepCmd(f client.NewClientFunc) *cobra.Command {
	cmd := envCmd(f, "step", "Move an environment to its next step", `  # Next step in development
  occ endpoint rollout step trips-api --env development

  # Next step only if staging is still at step 2, safe to retry from a pipeline
  occ endpoint rollout step trips-api --env staging --expect-step 2

  # One step back, or straight to the last step
  occ endpoint rollout step trips-api --env staging --back
  occ endpoint rollout step trips-api --env staging --last

  # Print the binding the step would write, for a GitOps commit
  occ endpoint rollout step trips-api --env production --dry-run -o yaml`,
		func(e *Endpoint, cmd *cobra.Command, args []string) error {
			return e.RolloutStep(stepParams(cmd, args))
		})
	cmd.Flags().Int("to", 0, "Move to this step instead of the next one")
	cmd.Flags().Bool("back", false, "Move one step back")
	cmd.Flags().Bool("last", false, "Move to the last step")
	cmd.Flags().Int("expect-step", 0, "Refuse unless the environment is at this step")
	return cmd
}

func newRolloutAbortCmd(f client.NewClientFunc) *cobra.Command {
	return envCmd(f, "abort", "Move an environment back to step 0; match rules still route their requests", `  # Move production back to step 0: weighted traffic goes to stable
  occ endpoint rollout abort trips-api --env production`,
		func(e *Endpoint, cmd *cobra.Command, args []string) error {
			return e.RolloutAbort(stepParams(cmd, args))
		})
}

func newRolloutStatusCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [ENDPOINT_NAME]",
		Short: "Wait until an environment serves its latest change",
		Long: `Wait until the environment serves the weights its binding sets. Exits non-zero ` +
			`when it doesn't within --timeout, so a pipeline can gate the next step on it.`,
		Example: `  occ endpoint rollout step trips-api --env staging && \
    occ endpoint rollout status trips-api --env staging --timeout 2m`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			return New(cl).RolloutStatus(RolloutWaitParams{
				RolloutParams: RolloutParams{Namespace: flags.GetNamespace(cmd), EndpointName: args[0]},
				Environment:   flags.GetEnvironment(cmd),
				Timeout:       timeout,
				Interval:      2 * time.Second,
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	cmd.Flags().Duration("timeout", 2*time.Minute, "How long to wait")
	return cmd
}

func newRolloutSetWeightsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-weights [ENDPOINT_NAME] BACKEND=WEIGHT...",
		Short: "Set hand-picked weights in an environment",
		Long: `Set backend weights on the environment's binding directly. The rollout shows ` +
			`the environment as set by hand until a step or abort moves it back on plan.`,
		Example: `  occ endpoint rollout set-weights trips-api --env development stable=70 canary=30`,
		Args:    cobra.MinimumNArgs(2),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			weights := map[string]int32{}
			for _, pair := range args[1:] {
				name, raw, ok := strings.Cut(pair, "=")
				w, err := strconv.ParseInt(raw, 10, 32)
				if !ok || name == "" || err != nil || w < 0 || w > 1_000_000 {
					return fmt.Errorf("expected BACKEND=WEIGHT with a weight from 0 to 1000000, got %q", pair)
				}
				weights[name] = int32(w)
			}
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).RolloutSet(RolloutSetParams{
				RolloutParams: rolloutParams(cmd, args),
				Environment:   flags.GetEnvironment(cmd),
				Weights:       weights,
			})
		},
	}
	addWriteFlags(cmd)
	flags.AddEnvironment(cmd)
	return cmd
}

func newRolloutFinishCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finish [ENDPOINT_NAME]",
		Short: "End the rollout keeping one backend",
		Long: `End the rollout: keep stable to drop the candidates, or keep a candidate ` +
			`to replace stable with it. Refused while a backend that leaves still gets ` +
			`traffic in an environment.`,
		Example: `  # Ship the new version
  occ endpoint rollout finish trips-api --keep trips-go`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			keep, _ := cmd.Flags().GetString("keep")
			return New(cl).RolloutFinish(RolloutFinishParams{RolloutParams: rolloutParams(cmd, args), Keep: keep})
		},
	}
	addWriteFlags(cmd)
	cmd.Flags().String("keep", "", "The backend to keep: stable or a candidate")
	return cmd
}
