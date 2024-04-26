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

package changelog

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	gh "github.com/google/go-github/v50/github"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/persistence"
	"github.com/cilium/release/pkg/types"
)

var releaseNotes = map[string]string{
	"release-note/major": "**Major Changes:**",
	"release-note/minor": "**Minor Changes:**",
	"release-note/bug":   "**Bugfixes:**",
	"release-note/ci":    "**CI Changes:**",
	"release-note/misc":  "**Misc Changes:**",
	"release-note/none":  "**Other Changes:**",
}

var releaseNotesOrder = []string{
	"release-note/major",
	"release-note/minor",
	"release-note/bug",
	"release-note/ci",
	"release-note/misc",
	"release-note/none",
}

type ChangeLog struct {
	ChangeLogConfig
	Logger *log.Logger

	prsWithUpstream types.BackportPRs
	listOfPrs       types.PullRequests
}

func filterCommit(cfg ChangeLogConfig, commit *gh.RepositoryCommit) bool {
	for _, filter := range cfg.FileFilters {
		for _, file := range commit.Files {
			if strings.HasPrefix(file.GetFilename(), filter) {
				return true
			}
		}
	}
	return false
}

func GenerateReleaseNotes(globalCtx context.Context, ghClient *gh.Client, logger *log.Logger, cfg ChangeLogConfig) (*ChangeLog, error) {
	var (
		backportPRs = types.BackportPRs{}
		listOfPRs   = types.PullRequests{}
		shas        []string
	)

	if _, err := os.Stat(cfg.StateFile); err == nil {
		logger.Printf("Found state file, resuming from stored state\n")

		var err error
		backportPRs, listOfPRs, shas, err = persistence.LoadState(cfg.StateFile)
		if err != nil {
			return nil, fmt.Errorf("Unable to read persistence file: %w", err)
		}
	} else {
		cont := false
		prevHead := ""

		for {
			logger.Printf("Comparing " + cfg.Base + "..." + cfg.Head + "\n")
			cc, _, err := ghClient.Repositories.CompareCommits(globalCtx, cfg.Owner, cfg.Repo, cfg.Base, cfg.Head, &gh.ListOptions{})
			if err != nil {
				return nil, fmt.Errorf("Unable to compare commits %s %s: %w\n", cfg.Base, cfg.Head, err)
			}
			if prevHead == cc.Commits[len(cc.Commits)-1].GetSHA() {
				sha := cc.Commits[0].GetSHA()
				if sha != "" {
					shas = append(shas, sha)
				}
				break
			}
			start := len(cc.Commits) - 1
			if cont {
				// We want to ignore the last sha for if the number of commits
				// returned by github are throttled. If they are throttled
				// we will keep comparing commits until the last commit
				// points to the base commit.
				start = start - 1
			}
			// List of commits are ordered from base to head
			// so we want to order them from head to base
			// For example, assuming commit SHAs are integers:
			// compare 1...10 will return [6,7,8,9,10]
			// We will store [10,9,8,7,6] and ask for compare 1...6
			// This will return [6,5,4,3,2,1] which we will ignore 6
			// since it's already stored in the list of SHAs and continue
			for i := start; i != 0; i-- {
				sha := cc.Commits[i].GetSHA()
				includeCommit := false
				if len(cfg.FileFilters) == 0 {
					includeCommit = true
				} else {
					commit, _, err := ghClient.Repositories.GetCommit(globalCtx, cfg.Owner, cfg.Repo, cc.Commits[i].GetSHA(), &gh.ListOptions{})
					if err != nil {
						includeCommit = true
						fmt.Println("Failed to get commit. struggle", err)
					} else {
						includeCommit = filterCommit(cfg, commit)
					}
				}
				if sha != "" && includeCommit {
					shas = append(shas, sha)
				}
			}
			cfg.Head = shas[len(shas)-1]
			cont = true
			prevHead = cc.Commits[len(cc.Commits)-1].GetSHA()
		}
	}

	logger.Printf("Found %d commits!\n", len(shas))

	output := func(foo string) { logger.Println(foo) }
	prsWithUpstream, listOfPrs, leftShas, err := github.GeneratePatchRelease(globalCtx, ghClient, cfg.Owner, cfg.Repo, output, backportPRs, listOfPRs, shas)
	fmt.Println()
	if err != nil {
		logger.Printf("Storing state in %s before exiting due to error...\n", cfg.StateFile)
	}
	err2 := persistence.StoreState(cfg.StateFile, prsWithUpstream, listOfPrs, leftShas)
	if err2 == nil {
		logger.Printf("State stored successful in %s, please use --state-file=%s in the next run to continue\n", cfg.StateFile, cfg.StateFile)
	} else {
		logger.Printf("Unable to store state: %s + \n", err2)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve PRs for commits: %w\n", err)
	}

	logger.Printf("\nFound %d PRs and %d backport PRs!\n\n", len(listOfPrs), len(prsWithUpstream))

	return &ChangeLog{
		ChangeLogConfig: cfg,
		Logger:          logger,
		prsWithUpstream: prsWithUpstream,
		listOfPrs:       listOfPrs,
	}, nil
}

func (cl *ChangeLog) PrintReleaseNotes() {
	fmt.Println("Summary of Changes")
	fmt.Println("------------------")

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for backportPR, listOfPrs := range cl.prsWithUpstream {
			for prID, pr := range listOfPrs {
				if pr.ReleaseLabel != releaseLabel {
					continue
				}
				if !printedReleaseNoteHeader {
					fmt.Println()
					fmt.Println(releaseNotes[releaseLabel])
					printedReleaseNoteHeader = true
				}

				changelogItems = append(
					changelogItems,
					fmt.Sprintf("* %s (Backport PR #%d, Upstream PR #%d, @%s)",
						pr.ReleaseNote, backportPR, prID, pr.AuthorName),
				)
				delete(listOfPrs, prID)
			}
		}
		for prID, pr := range cl.listOfPrs {
			if pr.ReleaseLabel != releaseLabel {
				continue
			}
			if len(cl.LastStable) != 0 {
				var backported bool
				for _, bb := range pr.BackportBranches {
					if strings.Contains(bb, cl.LastStable) {
						backported = true
					}
				}
				if backported {
					continue
				}
			}
			if !printedReleaseNoteHeader {
				fmt.Println()
				fmt.Println(releaseNotes[releaseLabel])
				printedReleaseNoteHeader = true
			}

			changelogItems = append(
				changelogItems,
				fmt.Sprintf("* %s (#%d, @%s)", pr.ReleaseNote, prID, pr.AuthorName),
			)
			delete(cl.listOfPrs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			fmt.Println(changeLogItem)
		}
	}

	if len(cl.listOfPrs) == 0 {
		return
	}
	cl.Logger.Printf("\n\033[1mNOTICE\033[0m: The following PRs were not included in the "+
		"changelog as they were backported to branch %s and assumed to be already released.\n", cl.LastStable)

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for prID, pr := range cl.listOfPrs {
			if pr.ReleaseLabel != releaseLabel {
				continue
			}
			if !printedReleaseNoteHeader {
				cl.Logger.Printf(releaseNotes[releaseLabel])
				printedReleaseNoteHeader = true
			}
			changelogItems = append(
				changelogItems,
				fmt.Sprintf("* %s (#%d, @%s)", pr.ReleaseNote, prID, pr.AuthorName),
			)
			delete(cl.listOfPrs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			cl.Logger.Printf(changeLogItem)
		}
	}
}
