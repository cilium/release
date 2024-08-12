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
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	gh "github.com/google/go-github/v62/github"
	"github.com/schollz/progressbar/v3"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/persistence"
	"github.com/cilium/release/pkg/types"
)

var releaseNotes = map[string]string{
	"release-note/security": "**Important Security Updates:**",
	"release-note/major":    "**Major Changes:**",
	"release-note/minor":    "**Minor Changes:**",
	"release-note/bug":      "**Bugfixes:**",
	"release-note/ci":       "**CI Changes:**",
	"release-note/misc":     "**Misc Changes:**",
	"release-note/none":     "**Other Changes:**",
}

var defaultReleaseNotesOrder = []string{
	"release-note/security",
	"release-note/major",
	"release-note/minor",
	"release-note/bug",
	"release-note/ci",
	"release-note/misc",
	"release-note/none",
}

type ChangeLog struct {
	ChangeLogConfig
	Logger Printer

	prsWithUpstream types.BackportPRs
	listOfPrs       types.PullRequests
	graphQLNodeIDs  types.NodeIDs
}

type Printer interface {
	Printf(format string, v ...any)
	Println(v ...any)
}

func GenerateReleaseNotes(globalCtx context.Context, ghClient *gh.Client, logger Printer, cfg ChangeLogConfig) (*ChangeLog, error) {
	var (
		backportPRs = types.BackportPRs{}
		listOfPRs   = types.PullRequests{}
		nodeIDs     = types.NodeIDs{}
		shas        []string
	)

	if _, err := os.Stat(cfg.StateFile); err == nil {
		logger.Printf("Found state file, resuming from stored state\n")

		var err error
		backportPRs, listOfPRs, nodeIDs, shas, err = persistence.LoadState(cfg.StateFile)
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
				if sha != "" {
					shas = append(shas, sha)
				}
			}
			cfg.Head = shas[len(shas)-1]
			cont = true
			prevHead = cc.Commits[len(cc.Commits)-1].GetSHA()
		}
	}

	logger.Printf("Found %d commits!\n", len(shas))
	bar := progressbar.Default(int64(len(shas)), "Preparing Changelog file")
	defer bar.Finish()

	output := func(foo string) { logger.Println(foo) }
	prsWithUpstream, listOfPrs, nodeIDs, leftShas, err :=
		github.GeneratePatchRelease(globalCtx, ghClient, cfg.Owner, cfg.Repo, bar, output, backportPRs, listOfPRs, nodeIDs, shas)
	logger.Println()
	if err != nil {
		logger.Printf("Storing state in %s before exiting due to error...\n", cfg.StateFile)
	}
	err2 := persistence.StoreState(cfg.StateFile, prsWithUpstream, listOfPrs, nodeIDs, leftShas)
	if err2 == nil {
		logger.Printf("State stored successful in %s, please use --state-file=%s in the next run to continue\n", cfg.StateFile, cfg.StateFile)
	} else {
		logger.Printf("Unable to store state: %s + \n", err2)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve PRs for commits: %w\n", err)
	}

	logger.Printf("\n")
	logger.Printf("Found %d PRs and %d backport PRs!\n\n", len(listOfPrs), len(prsWithUpstream))

	return &ChangeLog{
		ChangeLogConfig: cfg,
		Logger:          logger,
		prsWithUpstream: prsWithUpstream,
		listOfPrs:       listOfPrs,
		graphQLNodeIDs:  nodeIDs,
	}, nil
}

func (cl *ChangeLog) PrintReleaseNotesForWriter(w io.Writer) {
	var (
		listOfPRs       = make(types.PullRequests)
		prsWithUpstream = make(types.BackportPRs)
	)

	// Filter the PRs by --label-filter
	for id, pr := range cl.listOfPrs.DeepCopy() {
		if !filterByLabels(pr.Labels, cl.LabelFilters) {
			continue
		}
		listOfPRs[id] = pr
	}

	// Filter the Backport PRs by --label-filter
	for prNumber, upstreamedPRs := range cl.prsWithUpstream.DeepCopy() {
		for upstreamPRNumber, upstreamPR := range upstreamedPRs {
			if !filterByLabels(upstreamPR.Labels, cl.LabelFilters) {
				continue
			}
			if prsWithUpstream[prNumber] == nil {
				prsWithUpstream[prNumber] = make(types.PullRequests)
			}
			prsWithUpstream[prNumber][upstreamPRNumber] = upstreamPR
		}
	}

	cl.Logger.Printf("Found %d PRs and %d backport PRs in %s based on --label-filter\n\n", len(listOfPRs), len(prsWithUpstream), cl.StateFile)

	if !cl.SkipHeader {
		fmt.Fprintln(w, "Summary of Changes")
		fmt.Fprintln(w, "------------------")
	}

	var releaseNotesOrder []string
	if len(cl.ReleaseLabels) != 0 {
		// Only add release notes for release labels specified by --release-labels
		for _, label := range defaultReleaseNotesOrder {
			if !slices.Contains(cl.ReleaseLabels, label) {
				continue
			}
			releaseNotesOrder = append(releaseNotesOrder, label)
		}
	} else {
		releaseNotesOrder = defaultReleaseNotesOrder
	}

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for backportPR, listOfPRsUpstream := range prsWithUpstream {
			for prID, pr := range listOfPRsUpstream {
				if pr.ReleaseLabel != releaseLabel {
					continue
				}
				if !printedReleaseNoteHeader {
					fmt.Fprintln(w)
					fmt.Fprintln(w, releaseNotes[releaseLabel])
					printedReleaseNoteHeader = true
				}

				changelogItems = append(changelogItems, cl.prReleaseNote(pr, backportPR, &prID))
				delete(listOfPRsUpstream, prID)
			}
		}
		for prID, pr := range listOfPRs {
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
				fmt.Fprintln(w)
				fmt.Fprintln(w, releaseNotes[releaseLabel])
				printedReleaseNoteHeader = true
			}

			changelogItems = append(changelogItems, cl.prReleaseNote(pr, prID, nil))
			delete(listOfPRs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			fmt.Fprintln(w, changeLogItem)
		}
	}

	if len(listOfPRs) == 0 {
		return
	}
	cl.Logger.Printf("\n\033[1mNOTICE\033[0m: The following PRs were not included in the "+
		"changelog as they were backported to branch %s and assumed to be already released.\n", cl.LastStable)

	for _, releaseLabel := range releaseNotesOrder {
		var changelogItems []string
		printedReleaseNoteHeader := false
		for prID, pr := range listOfPRs {
			if pr.ReleaseLabel != releaseLabel {
				continue
			}
			if !printedReleaseNoteHeader {
				cl.Logger.Printf(releaseNotes[releaseLabel])
				printedReleaseNoteHeader = true
			}
			changelogItems = append(changelogItems, cl.prReleaseNote(pr, prID, nil))
			delete(listOfPRs, prID)
		}
		sort.Slice(changelogItems, func(i, j int) bool {
			return strings.ToLower(changelogItems[i]) < strings.ToLower(changelogItems[j])
		})
		for _, changeLogItem := range changelogItems {
			cl.Logger.Printf(changeLogItem)
		}
	}
}

func (cl *ChangeLog) PrintReleaseNotes() {
	cl.PrintReleaseNotesForWriter(os.Stdout)
}

// AllPRs returns all PRs that are part the changelog.
func (cl *ChangeLog) AllPRs() (map[int]struct{}, types.NodeIDs) {
	setOfPRs := map[int]struct{}{}

	for prNumber := range cl.listOfPrs {
		setOfPRs[prNumber] = struct{}{}
	}
	for backportPR, upstreamedPRs := range cl.prsWithUpstream {
		for upstreamPR := range upstreamedPRs {
			setOfPRs[upstreamPR] = struct{}{}
		}
		setOfPRs[backportPR] = struct{}{}
	}

	return setOfPRs, cl.graphQLNodeIDs
}

// prReleaseNote returns the release note for a given pull request.
func (cl *ChangeLog) prReleaseNote(pr types.PullRequest, prNumber int, upstreamPRNumber *int) string {
	text := fmt.Sprintf("* %s", pr.ReleaseNote)
	if !cl.ExcludePRReferences {
		if upstreamPRNumber != nil {
			text += fmt.Sprintf(" (Backport PR #%d, Upstream PR #%d, @%s)", prNumber, *upstreamPRNumber, pr.AuthorName)
		} else {
			text += fmt.Sprintf(" (%s#%d, @%s)", cl.RepoName, prNumber, pr.AuthorName)
		}
	}
	return text
}
