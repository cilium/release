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

	flag "github.com/spf13/pflag"

	"github.com/cilium/release/cmd/changelog"
	"github.com/cilium/release/cmd/projects"
	"github.com/cilium/release/pkg/github"
)

var (
	cfg               changelog.Config
	globalCtx, cancel = context.WithCancel(context.Background())
)

func init() {
	flag.StringVar(&cfg.CurrVer, "current-version", "", "Current version - the one being released")
	flag.StringVar(&cfg.NextVer, "next-dev-version", "", "Next version - the next development cycle")
	flag.StringVar(&cfg.Base, "base", "", "Base commit / tag used to generate release notes")
	flag.StringVar(&cfg.Head, "head", "", "Head commit used to generate release notes")
	flag.StringVar(&cfg.LastStable, "last-stable", "", "When last stable version is set, it will be used to detect if a bug was already backported or not to that particular branch (e.g.: '1.5', '1.6')")
	flag.StringVar(&cfg.StateFile, "state-file", "release-state.json", "When set, it will use the already fetched information from a previous run")
	flag.StringVar(&cfg.RepoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	flag.BoolVar(&cfg.ForceMovePending, "force-move-pending-backports", false, "Force move pending backports to the next version's project")
	flag.Parse()

	if err := cfg.Sanitize(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		flag.Usage()
		os.Exit(-1)
	}
	go signals()
}

func signals() {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	cancel()
}

func main() {
	ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))

	if len(cfg.CurrVer) != 0 {
		pm := projects.NewProjectManagement(ghClient, cfg.Owner, cfg.Repo)
		err := pm.SyncProjects(globalCtx, cfg.CurrVer, cfg.NextVer, cfg.ForceMovePending)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to manage project: %s\n", err)
			os.Exit(-1)
		}
		return
	}

	logger := log.New(os.Stderr, "", 0)
	cl, err := changelog.GenerateReleaseNotes(globalCtx, ghClient, logger, cfg)
	if err != nil {
		logger.Fatalf("%s\n", err)
	}
	cl.PrintReleaseNotes()
}
