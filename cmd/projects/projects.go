// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package projects

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

func Command(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Manage projects",
	}

	cmd.AddCommand(SyncCommand(ctx, logger))

	return cmd
}
