// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

func Command(ctx context.Context, logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checklist",
		Short: "Manage release checklists",
	}

	cmd.AddCommand(OpenCommand(ctx, logger))

	return cmd
}
