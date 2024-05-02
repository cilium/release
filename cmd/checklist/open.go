// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/types"

	gh "github.com/google/go-github/v50/github"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var cfg ChecklistConfig

type ChecklistConfig struct {
	types.CommonConfig

	TargetVer    string
	TemplatePath string
	DryRun       bool
}

func (cfg *ChecklistConfig) Sanitize() error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	if !semver.IsValid(cfg.TargetVer) {
		return fmt.Errorf("invalid --target-version=%s. Expected form 'vX.Y.Z(-rc.W|-pre.N)'", cfg.TargetVer)
	}
	return nil
}

func OpenCommand(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open a new release checklist",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg.TargetVer = semver.Canonical(cfg.TargetVer)
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}

			tmpl, err := prepareChecklist(cfg)
			if err != nil {
				return fmt.Errorf("Failed to apply template configuration to template: %s", err)
			}
			if cfg.DryRun {
				fmt.Printf("%s", tmpl)
				return nil
			}

			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))
			return CreateIssue(ctx, ghClient, cfg, tmpl)
		},
	}
	cmd.Flags().StringVar(&cfg.TemplatePath, "template", "", "Template path to create release checklist")
	cmd.Flags().StringVar(&cfg.TargetVer, "target-version", "", "Target version to release")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/release", "GitHub organization and repository names separated by a slash")
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
