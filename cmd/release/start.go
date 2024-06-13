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

const defaultStateFileValue = "release-state-${owner}-${repo}-${target-version}.json"

var cfg ReleaseConfig

type ReleaseConfig struct {
	types.CommonConfig

	QuayOrg  string
	QuayRepo string

	TargetVer         string
	PreviousVer       string
	RemoteBranchName  string
	DryRun            bool
	Force             bool
	RepoDirectory     string
	HelmRepoDirectory string
	StateFile         string
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

// HasStableBranch returns true if there is a major.minor branch for the release
// we are doing.
func (cfg *ReleaseConfig) HasStableBranch() bool {
	return cfg.RemoteBranchName != ""
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
			if defaultStateFileValue == cfg.StateFile {
				cfg.StateFile = fmt.Sprintf("release-state-%s-%s-%s.json", cfg.Repo, cfg.Owner, cfg.TargetVer)
			}

			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))

			// Auto detect previous version
			if cfg.PreviousVer == "" {
				previousVer, err := previousVersion(ctx, ghClient, cfg.Owner, cfg.Repo, cfg.TargetVer)
				if err != nil {
					return err
				}

				if !cfg.Force {
					err = io.ContinuePrompt(
						fmt.Sprintf("💡 The PREVIOUS released version was %s, continue?", previousVer),
						"✋ Wrong version detected, stopping the release process",
					)
					if err != nil {
						return err
					}
				} else {
					io.Fprintf(0, os.Stdout, "💡 The PREVIOUS released version was %s\n", previousVer)
				}
				cfg.PreviousVer = previousVer
			}

			remoteBranchName, err := getRemoteBranch(ctx, ghClient, cfg.Owner, cfg.Repo, cfg.TargetVer)
			if err != nil {
				return err
			}

			cfg.RemoteBranchName = remoteBranchName

			// FIXME: check if docker is running before starting the release process

			steps := []Step{
				// Pre-release
				// Audited
				// Tested for pre-release
				NewCheckReleaseBlockers(&cfg),
				// Audited
				// Tested for pre-release
				NewImageCVE(&cfg),
				// 1st part
				// Audited
				// Tested for pre-release
				NewPrepareCommit(&cfg),
				// Audited
				// Tested for pre-release
				NewSubmitPR(&cfg),
				// 2nd part
				// Audited
				// Tested for pre-release
				NewTagCommit(&cfg),
				// 3rd part
				// Audited
				// Tested for pre-release
				NewPostRelease(&cfg),
				// Audited
				// Tested for pre-release
				NewSubmitPostReleasePR(&cfg),
				// 4th part
				NewHelmChart(&cfg),
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
	cmd.Flags().StringVar(&cfg.PreviousVer, "previous-version", "", "Previous released version (manually specify if the auto detection doesn't work properly)")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "If enabled, it will not perform irreversible operations into GitHub such as: "+
		"syncing GH projects, pushing tags. All changes that are done locally as well as creating and pushing PRs "+
		"are also considered reversible and therefore not affected by this flag's value.")
	cmd.Flags().BoolVar(&cfg.Force, "force", false, "Say yes to all prompts.")
	cmd.Flags().StringVar(&cfg.QuayOrg, "quay-org", "cilium", "Quay.io organization to check for image vulnerabilities")
	cmd.Flags().StringVar(&cfg.QuayRepo, "quay-repo", "cilium-ci", "Quay.io repository to check for image vulnerabilities")
	cmd.Flags().StringVar(&cfg.RepoDirectory, "repo-dir", "../cilium", "Directory with the source code of Cilium")
	cmd.Flags().StringVar(&cfg.HelmRepoDirectory, "charts-repo-dir", "../charts", "Directory with the source code of Helm charts")
	cmd.Flags().StringVar(&cfg.StateFile, "state-file", defaultStateFileValue, "When set, it will use the already fetched information from a previous run")

	for _, flag := range []string{"target-version", "template"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}
