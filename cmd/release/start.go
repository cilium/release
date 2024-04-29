// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/io"
	"github.com/cilium/release/pkg/types"

	gh "github.com/google/go-github/v62/github"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var cfg ReleaseConfig

type ReleaseConfig struct {
	types.CommonConfig

	QuayOrg  string
	QuayRepo string

	TargetVer     string
	DryRun        bool
	Force         bool
	RepoDirectory string
	ProjectNumber int
	StateFile     string
}

func (cfg *ReleaseConfig) Sanitize() error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	if !semver.IsValid(cfg.TargetVer) {
		return fmt.Errorf("invalid --target-version=%s. Expected form 'vX.Y.Z(-rc.W|-pre.N)'", cfg.TargetVer)
	}
	return nil
}

type Step interface {
	Name() string
	Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *gh.Client) error
	Revert(ctx context.Context, dryRun bool, ghClient *gh.Client) error
}

func Command(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the release process",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg.TargetVer = semver.Canonical(cfg.TargetVer)
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}

			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))

			steps := []Step{
				NewCheckReleaseBlockers(&cfg),
				NewImageCVE(&cfg),
				NewProjectsManagement(&cfg),
				NewPrepareCommit(&cfg),
				NewSubmitPR(&cfg),
			}

			for i, step := range steps {
				io.Fprintf(0, os.Stdout, "🏃 Running step %q\n", step.Name())
				err := step.Run(ctx, cfg.Force, cfg.DryRun, ghClient)
				if err != nil {
					io.Fprintf(0, os.Stdout, "😩 Error while running step %q: %s. Reverting previous steps...\n", step.Name(), err)
					revertSteps := steps[:i]
					slices.Reverse(revertSteps)
					for _, revertStep := range revertSteps {
						err := revertStep.Revert(ctx, cfg.DryRun, ghClient)
						if err != nil {
							io.Fprintf(0, os.Stdout, "😩 Unrecoverable error while reverting step %q: %s\n", revertStep.Name(), err)
						}
					}
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cfg.TargetVer, "target-version", "", "Target version to release")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Print the template, but do not open an issue on GitHub")
	cmd.Flags().BoolVar(&cfg.Force, "force", false, "Say yes to all prompts.")
	cmd.Flags().StringVar(&cfg.QuayOrg, "quay-org", "cilium", "Quay.io organization to check for image vulnerabilities")
	cmd.Flags().StringVar(&cfg.QuayRepo, "quay-repo", "cilium-ci", "Quay.io repository to check for image vulnerabilities")
	cmd.Flags().StringVar(&cfg.RepoDirectory, "repo-dir", "../cilium", "Directory with the source code of Cilium")
	cmd.Flags().StringVar(&cfg.StateFile, "state-file", "release-state.json", "When set, it will use the already fetched information from a previous run")

	for _, flag := range []string{"target-version", "template"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}
