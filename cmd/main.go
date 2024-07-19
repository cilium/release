// Copyright 2020-2021 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/cilium/release/cmd/changelog"
	"github.com/cilium/release/cmd/checklist"
	"github.com/cilium/release/cmd/projects"
	"github.com/cilium/release/cmd/release"
	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/types"

	"github.com/spf13/cobra"
)

var (
	cfg               Config
	globalCtx, cancel = context.WithCancel(context.Background())
	logger            = log.New(os.Stderr, "", 0)

	rootCmd = &cobra.Command{
		Use:          "release",
		Short:        "release -- Prepare a Cilium release",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := cfg.Sanitize(); err != nil {
				cmd.Usage()
				logger.Fatalf("\nFailed to validate configuration: %s", err)
			}
			run(logger)
		},
	}
)

func init() {
	addFlags(rootCmd)
	rootCmd.AddCommand(
		changelog.Command(globalCtx, logger),
		projects.Command(globalCtx, logger),
		checklist.Command(globalCtx, logger),
		release.Command(globalCtx, logger),
	)
	go signals()
}

type Config struct {
	types.CommonConfig
	changelog.ChangeLogConfig
	projects.ProjectsConfig
}

// Sanitize runs the sanitization logic for the older functions that were
// always run as part of a bare './release' command. When we deprecate using
// this command directly in favour of using the subcommands, we can remove the
// extra hacks to sanitize the settings here.
func (cfg *Config) Sanitize() error {
	for _, ccfg := range []*types.CommonConfig{
		&cfg.ChangeLogConfig.CommonConfig,
		&cfg.ProjectsConfig.CommonConfig,
	} {
		*ccfg = cfg.CommonConfig
	}
	for _, fn := range []func() error{
		cfg.ChangeLogConfig.Sanitize,
		cfg.ProjectsConfig.Sanitize,
	} {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&cfg.CurrVer, "current-version", "", "Current version - the one being released")
	cmd.Flags().StringVar(&cfg.NextVer, "next-dev-version", "", "Next version - the next development cycle")
	cmd.Flags().StringVar(&cfg.Base, "base", "", "Base commit / tag used to generate release notes")
	cmd.Flags().StringVar(&cfg.Head, "head", "", "Head commit used to generate release notes")
	cmd.Flags().StringVar(&cfg.LastStable, "last-stable", "", "When last stable version is set, it will be used to detect if a bug was already backported or not to that particular branch (e.g.: '1.5', '1.6')")
	cmd.Flags().StringVar(&cfg.StateFile, "state-file", "release-state.json", "When set, it will use the already fetched information from a previous run")
	cmd.Flags().StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	cmd.Flags().BoolVar(&cfg.ForceMovePending, "force-move-pending-backports", false, "Force move pending backports to the next version's project")
	cmd.Flags().StringArrayVar(&cfg.LabelFilters, "label-filter", []string{}, "Filter pull requests by labels")
	cmd.Flags().BoolVar(&cfg.ExcludePRReferences, "exclude-pr-references", false, "If true, do not include references to the PR or PR author")
	cmd.Flags().BoolVar(&cfg.SkipHeader, "skip-header", false, "If true, do not print 'Summary of of changes' header")
}

func signals() {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	cancel()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(logger *log.Logger) {
	ghClient := github.NewClient()

	if len(cfg.CurrVer) != 0 {
		pm := projects.NewProjectManagement(ghClient, cfg.Owner, cfg.Repo)
		err := pm.SyncProjects(globalCtx, cfg.CurrVer, cfg.NextVer, cfg.ForceMovePending)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to manage project: %s\n", err)
			os.Exit(-1)
		}
		return
	}

	cl, err := changelog.GenerateReleaseNotes(globalCtx, ghClient, logger, cfg.ChangeLogConfig)
	if err != nil {
		logger.Fatalf("%s\n", err)
	}
	cl.PrintReleaseNotes()
}
