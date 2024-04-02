// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package changelog

import (
	"context"
	"log"
	"os"

	"github.com/cilium/release/pkg/github"
	"github.com/spf13/cobra"
)

func Command(ctx context.Context, logger *log.Logger, cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "changelog",
		Short: "Generate release notes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))
			cl, err := GenerateReleaseNotes(ctx, ghClient, logger, *cfg)
			if err != nil {
				logger.Fatalf("%s\n", err)
			}
			cl.PrintReleaseNotes()
			return nil
		},
	}
}
