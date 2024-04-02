// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package projects

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/types"

	"github.com/spf13/cobra"
)

var cfg ProjectsConfig

type ProjectsConfig struct {
	types.CommonConfig

	CurrVer string
	NextVer string

	// ForceMovePending lets "pending" backports be moved from one project
	// to another. By default this is set to false, since most commonly
	// this is a mistake and the PR should have been previously marked as
	// "backport-done".
	ForceMovePending bool
}

func (cfg *ProjectsConfig) Sanitize() error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	return nil
}

func SyncCommand(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}

			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))
			pm := NewProjectManagement(ghClient, cfg.Owner, cfg.Repo)
			err := pm.SyncProjects(ctx, cfg.CurrVer, cfg.NextVer, cfg.ForceMovePending)
			if err != nil {
				return fmt.Errorf("Unable to manage project: %s\n", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cfg.CurrVer, "current-version", "", "Current version - the one being released")
	cmd.Flags().StringVar(&cfg.NextVer, "next-dev-version", "", "Next version - the next development cycle")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	cmd.Flags().BoolVar(&cfg.ForceMovePending, "force-move-pending-backports", false, "Force move pending backports to the next version's project")

	for _, flag := range []string{"current-version", "repo"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}

	return cmd
}
