// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/cilium/release/pkg/io"
	"github.com/cilium/release/pkg/types"
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
	Groups            []string
	DefaultBranch     string
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
	Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *GHClient) error
	Revert(ctx context.Context, dryRun bool, ghClient *GHClient) error
}

// GroupStep contains a list of steps that should run.
type GroupStep struct {
	name  string
	steps []Step
}

var (
	groups        []GroupStep
	allGroupNames []string
)

func init() {
	groups = []GroupStep{
		{
			name: "1-pre-release",
			steps: []Step{
				// Audited
				// Tested for pre-release
				NewCheckReleaseBlockers(&cfg),
				// Audited
				// Tested for pre-release
				NewImageCVE(&cfg),
			},
		},
		{
			name: "2-prepare-commit",
			steps: []Step{
				// Audited
				// Tested for pre-release
				NewPrepareCommit(&cfg),
				// Audited
				// Tested for pre-release
				NewSubmitPR(&cfg),
			},
		},
		{
			name: "3-tag",
			steps: []Step{
				// Audited
				// Tested for pre-release
				NewTagCommit(&cfg),
			},
		},
		{
			name: "4-post-release",
			steps: []Step{
				// Audited
				// Tested for pre-release
				NewPostRelease(&cfg),
				// Audited
				// Tested for pre-release
				NewSubmitPostReleasePR(&cfg),
				// Audited
				// Tested for pre-release
				NewProjectsManagement(&cfg),
			},
		},
		{
			name: "5-publish-helm",
			steps: []Step{
				NewHelmChart(&cfg),
			},
		},
	}

	for _, group := range groups {
		allGroupNames = append(allGroupNames, group.name)
	}
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

			ghClient := NewGHClient(os.Getenv("GITHUB_TOKEN"))

			// Auto detect previous version
			if cfg.PreviousVer == "" {
				previousVer, err := ghClient.previousVersion(ctx, cfg.Owner, cfg.Repo, cfg.TargetVer)
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

			// Auto detect default branch
			var err error
			cfg.DefaultBranch, err = ghClient.getDefaultBranch(ctx, cfg.Owner, cfg.Repo)
			if err != nil {
				return err
			}

			remoteBranchName, err := ghClient.getRemoteBranch(ctx, cfg.Owner, cfg.Repo, cfg.TargetVer)
			if err != nil {
				return err
			}

			cfg.RemoteBranchName = remoteBranchName

			// FIXME: check if docker is running before starting the release process

			for _, group := range groups {
				run := false
				for _, runGroup := range cfg.Groups {
					if group.name == runGroup ||
						// Also accept alias based on the number
						(len(runGroup) == 1 && strings.HasPrefix(group.name, runGroup)) {
						run = true
						break
					}
				}
				if !run {
					continue
				}
				io.Fprintf(0, os.Stdout, "🏃 Running group %q\n", group.name)
				steps := group.steps
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
				io.Fprintf(0, os.Stdout, "All groups successfully ran\n")
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
	cmd.Flags().StringSliceVar(&cfg.Groups, "groups", allGroupNames, "Specify which groups should be executed for the release. You can also simply pass the numbers of the groups '1,2'")

	for _, flag := range []string{"target-version", "template"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}
