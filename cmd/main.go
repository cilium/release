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
	"os"
	"os/signal"
	"sort"
	"strings"

	gh "github.com/google/go-github/v50/github"
	flag "github.com/spf13/pflag"

	"github.com/cilium/release/cmd/projects"
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

var (
	base       string
	head       string
	lastStable string
	stateFile  string
	repoName   string
	currVer    string
	nextVer    string

	// forceMovePending lets "pending" backports be moved from one project
	// to another. By default this is set to false, since most commonly
	// this is a mistake and the PR should have been previously marked as
	// "backport-done".
	forceMovePending bool
)

func init() {
	flag.StringVar(&currVer, "current-version", "", "Current version - the one being released")
	flag.StringVar(&nextVer, "next-dev-version", "", "Next version - the next development cycle")
	flag.StringVar(&base, "base", "", "Base commit / tag used to generate release notes")
	flag.StringVar(&head, "head", "", "Head commit used to generate release notes")
	flag.StringVar(&lastStable, "last-stable", "", "When last stable version is set, it will be used to detect if a bug was already backported or not to that particular branch (e.g.: '1.5', '1.6')")
	flag.StringVar(&stateFile, "state-file", "release-state.json", "When set, it will use the already fetched information from a previous run")
	flag.StringVar(&repoName, "repo", "cilium/cilium", "GitHub organization and repository names separated by a slash")
	flag.BoolVar(&forceMovePending, "force-move-pending-backports", false, "Force move pending backports to the next version's project")
	flag.Parse()

	if len(base) == 0 && len(currVer) == 0 {
		fmt.Fprintf(os.Stderr, "--base can't be empty\n")
		flag.Usage()
		os.Exit(-1)

	}
	if len(head) == 0 && len(currVer) == 0 {
		fmt.Fprintf(os.Stderr, "--head can't be empty\n")
		flag.Usage()
		os.Exit(-1)
	}
	if len(stateFile) == 0 {
		fmt.Fprintf(os.Stderr, "--state-file can't be empty\n")
		flag.Usage()
		os.Exit(-1)
	}
	if strings.Contains(lastStable, "v") {
		fmt.Fprintf(os.Stderr, "--last-stable can't contain letters, should be of the format 'x.y'\n")
		flag.Usage()
		os.Exit(-1)
	}
	go signals()
}

var globalCtx, cancel = context.WithCancel(context.Background())

func signals() {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	cancel()
}

func main() {
	ghClient := github.NewClient(os.Getenv("GITHUB_TOKEN"))

	var (
		backportPRs = types.BackportPRs{}
		listOfPRs   = types.PullRequests{}
		shas        []string
	)
	ownerRepo := strings.Split(repoName, "/")
	if len(ownerRepo) != 2 {
		fmt.Fprintf(os.Stderr, "Invalid repo name: %s\n", repoName)
		os.Exit(-1)
	}
	owner := ownerRepo[0]
	repo := ownerRepo[1]

	if len(currVer) != 0 {
		pm := projects.NewProjectManagement(ghClient, owner, repo)
		err := pm.SyncProjects(globalCtx, currVer, nextVer, forceMovePending)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to manage project: %s\n", err)
			os.Exit(-1)
		}
		return
	}

	if _, err := os.Stat(stateFile); err == nil {
		fmt.Fprintf(os.Stderr, "Found state file, resuming from stored state\n")
		var err error
		backportPRs, listOfPRs, shas, err = persistence.LoadState(stateFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to read persistence file: %s", err)
			os.Exit(-1)
			return
		}
	} else {
		cont := false
		prevHead := ""

		for {
			fmt.Fprintf(os.Stderr, "Comparing %s...%s\n", base, head)
			cc, _, err := ghClient.Repositories.CompareCommits(globalCtx, owner, repo, base, head, &gh.ListOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to compare commits %s %s: %s\n", base, head, err)
				os.Exit(-1)
				return
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
				if sha != "" {
					shas = append(shas, sha)
				}
			}
			head = shas[len(shas)-1]
			cont = true
			prevHead = cc.Commits[len(cc.Commits)-1].GetSHA()
		}
	}

	fmt.Fprintf(os.Stderr, "Found %d commits!\n", len(shas))

	printer := func(msg string) {
		fmt.Fprintf(os.Stderr, msg)
	}

	prsWithUpstream, listOfPrs, leftShas, err := github.GeneratePatchRelease(globalCtx, ghClient, owner, repo, printer, backportPRs, listOfPRs, shas)
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to retrieve PRs for commits: %s\n", err)
		fmt.Fprintf(os.Stderr, "Storing state in %s before existing!\n", stateFile)
	}
	err2 := persistence.StoreState(stateFile, prsWithUpstream, listOfPrs, leftShas)
	if err2 == nil {
		fmt.Fprintf(os.Stderr, "State stored successful in %s, please use --state-file=%s in the next run to continue\n", stateFile, stateFile)
	} else {
		fmt.Fprintf(os.Stderr, "Unable to store state: %s\n", err2)
	}
	if err != nil {
		os.Exit(-1)
		return
	}

	fmt.Fprintf(os.Stderr, "\nFound %d PRs and %d backport PRs!\n\n", len(listOfPrs), len(prsWithUpstream))

	printReleaseNotes(prsWithUpstream, listOfPrs)

}

func printReleaseNotes(prsWithUpstream types.BackportPRs, listOfPrs types.PullRequests) {
	fmt.Println("Summary of Changes")
	fmt.Println("------------------")

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for backportPR, listOfPrs := range prsWithUpstream {
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
		for prID, pr := range listOfPrs {
			if pr.ReleaseLabel != releaseLabel {
				continue
			}
			if len(lastStable) != 0 {
				var backported bool
				for _, bb := range pr.BackportBranches {
					if strings.Contains(bb, lastStable) {
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
			delete(listOfPrs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			fmt.Println(changeLogItem)
		}
	}

	if len(listOfPrs) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\n\033[1mNOTICE\033[0m: The following PRs were not included in the "+
		"changelog as they were backported to branch %s and assumed to be already released.\n", lastStable)

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for prID, pr := range listOfPrs {
			if pr.ReleaseLabel != releaseLabel {
				continue
			}
			if !printedReleaseNoteHeader {
				fmt.Fprintf(os.Stderr, releaseNotes[releaseLabel])
				printedReleaseNoteHeader = true
			}
			changelogItems = append(
				changelogItems,
				fmt.Sprintf("* %s (#%d, @%s)", pr.ReleaseNote, prID, pr.AuthorName),
			)
			delete(listOfPrs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			fmt.Fprintf(os.Stderr, changeLogItem)
		}
	}
}
