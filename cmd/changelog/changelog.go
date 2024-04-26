// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package changelog

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/types"
	"github.com/spf13/cobra"
)

var cfg ChangeLogConfig

type ChangeLogConfig struct {
	types.CommonConfig

	Base        string
	Head        string
	LastStable  string
	StateFile   string
	FileFilters []string
}

func (cfg *ChangeLogConfig) Sanitize() error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	if len(cfg.StateFile) == 0 {
		return fmt.Errorf("--state-file can't be empty\n")
	}
	if strings.Contains(cfg.LastStable, "v") {
		return fmt.Errorf("--last-stable can't contain letters, should be of the format 'x.y'\n")
	}
	return nil
}

func Command(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Generate release notes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				return fmt.Errorf("Failed to validate configuration: %s", err)
			}

			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))
			cl, err := GenerateReleaseNotes(ctx, ghClient, logger, cfg)
			if err != nil {
				return err
			}
			cl.PrintReleaseNotes()
			return nil
		},
	}
	cmd.Flags().StringVar(&cfg.Base, "base", "", "Base commit / tag used to generate release notes")
	cmd.Flags().StringVar(&cfg.Head, "head", "", "Head commit used to generate release notes")
	cmd.Flags().StringVar(&cfg.LastStable, "last-stable", "", "When last stable version is set, it will be used to detect if a bug was already backported or not to that particular branch (e.g.: '1.5', '1.6')")
	cmd.Flags().StringVar(&cfg.StateFile, "state-file", "release-state.json", "When set, it will use the already fetched information from a previous run")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	cmd.Flags().StringArrayVar(&cfg.FileFilters, "file-filter", []string{}, "stuff")

	for _, flag := range []string{"base", "head", "repo"} {
		cobra.MarkFlagRequired(cmd.Flags(), flag)
	}
	return cmd
}
