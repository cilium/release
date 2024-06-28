// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/cilium/release/pkg/io"
	"github.com/cilium/release/pkg/types"
	"github.com/docker/docker/client"
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
	Steps             []string
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
	name        string
	steps       []Step
	permissions map[Location]Permission
}

var (
	groups             []GroupStep
	allGroupStepsNames []string
)

func init() {
	groups = []GroupStep{
		{
			name: "1-pre-check",
			steps: []Step{
				NewCheckReleaseBlockers(&cfg),
				NewImageCVE(&cfg),
			},
			permissions: map[Location]Permission{
				LocationGitHubUpstream: PermissionRead,
				LocationQuayIO:         PermissionRead,
			},
		},
		{
			name: "2-prepare-release",
			steps: []Step{
				NewPrepareCommit(&cfg),
				NewSubmitPR(&cfg),
			},
			permissions: map[Location]Permission{
				LocationLocalDisk:      PermissionRead | PermissionWrite,
				LocationGitHubUpstream: PermissionRead | PermissionPullRequest,
				LocationGitHubFork:     PermissionWrite,
			},
		},
		{
			name: "3-tag",
			steps: []Step{
				NewTagCommit(&cfg),
			},
			permissions: map[Location]Permission{
				LocationLocalDisk:      PermissionRead | PermissionWrite,
				LocationGitHubUpstream: PermissionRead | PermissionWrite,
			},
		},
		{
			name: "4-post-release",
			steps: []Step{
				NewPostRelease(&cfg),
				NewSubmitPostReleasePR(&cfg),
				NewProjectsManagement(&cfg),
			},
			permissions: map[Location]Permission{
				LocationLocalDisk:      PermissionRead | PermissionWrite,
				LocationGitHubUpstream: PermissionRead | PermissionPullRequest,
				LocationGitHubFork:     PermissionWrite,
				LocationGitHubProjects: PermissionRead | PermissionWrite,
			},
		},
		{
			name: "5-publish-helm",
			steps: []Step{
				NewHelmChart(&cfg),
			},
			permissions: map[Location]Permission{
				LocationLocalDisk:       PermissionRead | PermissionWrite,
				LocationGitHubHelmChart: PermissionRead | PermissionWrite,
			},
		},
	}

	for _, group := range groups {
		allGroupStepsNames = append(allGroupStepsNames, group.name)
	}
}

func Command(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the release process",
		Long: func() string {
			var buf bytes.Buffer
			buf.WriteString(
				`Release Process
This tool is designed to perform Cilium releases.

Each step in the release process should be run separately. These steps perform
logical operations and can be executed independently. Each step has a soft
dependency on the previous one.

This tool handles pre-releases, release candidates (RCs), and patch releases.

1. pre-check:
Checks for any release blockers and fixable CVEs in quay.io.

2. prepare-release:
Prepares a commit in the local git repository by modifying necessary files and
pushing a PR once the files are committed.

3. tag:
Fetches the repository from upstream and tags the release commit with the
appropriate tag.

4. post-release:
Populates the Helm charts with the image digests and creates a Pull Request with
these changes.
Creates a GitHub release in draft mode for later publishing.
Moves all Pull Requests that are part of this release to their respective
project.

5. publish-helm:
Prepares the Helm chart in the local repository and pushes the changes directly
to the main branch.

Below is a table summarizing the permissions required for this tool.
`)
			PrintPermTable(&buf)
			buf.WriteString(`

Requirements:
- docker
- gh CLI tool
- GITHUB_TOKEN with:
    - Read access to actions, issues, and metadata
    - Read and write access to code, organization projects, and pull requests
	- Direct link in https://github.com/settings/tokens/new?scopes=project,write:org,repo
- Local Cilium repository
- Local Cilium Chart repository

To start, run
./release start --target-version vX.Y.Z[-(pre|rc).W] --steps 1
`)
			return buf.String()
		}(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg.TargetVer = semver.Canonical(cfg.TargetVer)
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}
			if defaultStateFileValue == cfg.StateFile {
				cfg.StateFile = fmt.Sprintf("release-state-%s-%s-%s.json", cfg.Repo, cfg.Owner, cfg.TargetVer)
			}

			// check if docker is running before starting the release process
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return fmt.Errorf("Error creating Docker client: %w", err)
			}

			ctx := context.Background()
			_, err = cli.Ping(ctx)
			if err != nil {
				return fmt.Errorf("Docker is not running or not accessible: %w", err)
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
			cfg.DefaultBranch, err = ghClient.getDefaultBranch(ctx, cfg.Owner, cfg.Repo)
			if err != nil {
				return err
			}

			remoteBranchName, err := ghClient.getRemoteBranch(ctx, cfg.Owner, cfg.Repo, cfg.TargetVer)
			if err != nil {
				return err
			}

			cfg.RemoteBranchName = remoteBranchName

			for _, group := range groups {
				run := false
				for _, runGroup := range cfg.Steps {
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
	cmd.Flags().StringSliceVar(&cfg.Steps, "steps", []string{"1"},
		fmt.Sprintf("Specify which steps should be executed for the release. Steps numbers are also allowed, e.g. '1,2'. Accepted values: %s", strings.Join(allGroupStepsNames, ", ")),
	)

	for _, flag := range []string{"target-version", "template"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}
