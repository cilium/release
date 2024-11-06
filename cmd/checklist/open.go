// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/types"

	gh "github.com/google/go-github/v62/github"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var cfg ChecklistConfig

type ChecklistConfig struct {
	types.CommonConfig

	// TargetRepoName differs from the commmon RepoName to create the checklist in - it's where we check whether the
	// target version exists already.
	TargetRepoName string
	TargetVer      string
	TemplatePath   string
	DryRun         bool
}

func (cfg *ChecklistConfig) Sanitize(ctx context.Context, gh *gh.Client) error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	targetOwnerRepo := strings.Split(cfg.TargetRepoName, "/")
	if len(targetOwnerRepo) != 2 {
		return fmt.Errorf("Invalid target repo name: %s\n", cfg.TargetRepoName)
	}

	if !semver.IsValid(cfg.TargetVer) {
		return fmt.Errorf("invalid --target-version=%s. Expected form 'vX.Y.Z(-rc.W|-pre.N)'", cfg.TargetVer)
	}

	// Check that the target version isn't published already.
	exists, err := releaseExists(ctx, gh, targetOwnerRepo[0], targetOwnerRepo[1], cfg.TargetVer)
	if err != nil {
		return fmt.Errorf("failed to ensure release doesn't exist yet: %w", err)
	}
	if exists {
		return fmt.Errorf("release %v already exists", cfg.TargetVer)
	}

	return nil
}

func OpenCommand(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open a new release checklist",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ghClient := github.NewClient()
			cfg.TargetVer = semver.Canonical(cfg.TargetVer)
			if err := cfg.Sanitize(ctx, ghClient); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}

			tmpl, err := fetchTemplate(cfg)
			if err != nil {
				return fmt.Errorf("Failed to fetch template: %w", err)
			}
			cl, err := prepareChecklist(tmpl, cfg)
			if err != nil {
				return fmt.Errorf("Failed to apply template configuration to template: %s", err)
			}
			if cfg.DryRun {
				fmt.Printf("%s", cl)
				return nil
			}

			return CreateIssue(ctx, ghClient, cfg, cl)
		},
	}
	cmd.Flags().StringVar(&cfg.TemplatePath, "template", "", "Template path to create release checklist")
	cmd.Flags().StringVar(&cfg.TargetVer, "target-version", "", "Target version to release")
	cmd.Flags().StringVar(&cfg.TargetRepoName, "target-repo", "cilium/cilium", "Github repository which target versions refer to")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/release", "GitHub organization and repository names separated by a slash to create the checklist in")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Print the template, but do not open an issue on GitHub")

	for _, flag := range []string{"target-version", "template"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}

func CreateIssue(ctx context.Context, ghClient *gh.Client, cfg ChecklistConfig, tmpl string) error {
	req, err := templateToRequest(tmpl)
	if err != nil {
		return fmt.Errorf("Issue template metadata unreadable: %w", err)
	}

	res, _, err := ghClient.Issues.Create(ctx, cfg.Owner, cfg.Repo, req)
	if err != nil {
		return fmt.Errorf("Failed to create release checklist: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created issue at %s\n", *res.HTMLURL)

	return nil
}

func releaseExists(ctx context.Context, ghClient *gh.Client, owner, repo, tag string) (bool, error) {
	_, res, err := ghClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if res == nil && err != nil {
		return false, fmt.Errorf("failed to fetch release by tag: %w", err)
	}

	switch res.StatusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK:
		return true, nil
	default:
		return false, fmt.Errorf("unexpected status fetching release: %v", res.StatusCode)
	}
}
