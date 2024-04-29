// Copyright 2020 Authors of Cilium
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

package github

import (
	"context"
	"fmt"
	"slices"
	"time"

	gh "github.com/google/go-github/v62/github"
	"github.com/schollz/progressbar/v3"

	"github.com/cilium/release/pkg/types"
)

func filterByLabels(labels []string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, label := range labels {
		if slices.Contains(filters, label) {
			return true
		}
	}
	return false
}

// GeneratePatchRelease will returns a map that maps the backport PR number to
// the upstream PR number and a map that maps the backport PR number to the PR
// if no upstream PR was found.
// In case of an error, a list of non-processed commits will be returned.
func GeneratePatchRelease(
	ctx context.Context,
	ghClient *gh.Client,
	owner string,
	repo string,
	bar *progressbar.ProgressBar,
	printer func(msg string),
	backportPRs types.BackportPRs,
	listOfPRs types.PullRequests,
	commits []string,
	labelFilters []string,
) (
	types.BackportPRs,
	types.PullRequests,
	[]string,
	error,
) {

	for i, sha := range commits {
		bar.Add(1)
		page := 0
		foundPR := false
		for {
			ctxWithTimeout, cancel := context.WithTimeout(ctx, 45*time.Second)
			prs, resp, err := ghClient.PullRequests.ListPullRequestsWithCommit(ctxWithTimeout, owner, repo, sha, &gh.ListOptions{
				Page: page,
			})
			cancel()
			if err != nil {
				return backportPRs, listOfPRs, commits[i:], err
			}

			for _, pr := range prs {
				_, ok := listOfPRs[pr.GetNumber()]
				_, ok2 := backportPRs[pr.GetNumber()]
				if ok || ok2 {
					foundPR = true
					continue
				}
				if pr.GetState() != "closed" {
					continue
				}
				foundPR = true
				upstreamPRs := getUpstreamPRs(pr.GetBody())
				if upstreamPRs == nil {
					lbls := parseGHLabels(pr.Labels)
					if !filterByLabels(lbls, labelFilters) {
						continue
					}
					listOfPRs[pr.GetNumber()] = types.PullRequest{
						ReleaseNote:      getReleaseNote(pr.GetTitle(), pr.GetBody()),
						ReleaseLabel:     getReleaseLabel(lbls),
						AuthorName:       pr.GetUser().GetLogin(),
						BackportBranches: getBackportBranches(lbls),
					}
					continue
				}
				backportPRs[pr.GetNumber()] = map[int]types.PullRequest{}
				for _, upstreamPRNumber := range upstreamPRs {
					_, ok := backportPRs[pr.GetNumber()][upstreamPRNumber]
					if ok {
						continue
					}
					ctxWithTimeout, cancel := context.WithTimeout(ctx, 45*time.Second)
					upstreamPR, _, err := ghClient.PullRequests.Get(ctxWithTimeout, owner, repo, upstreamPRNumber)
					cancel()
					if err != nil {
						delete(backportPRs, pr.GetNumber())
						return backportPRs, listOfPRs, commits[i:], err
					}
					lbls := parseGHLabels(upstreamPR.Labels)
					if !filterByLabels(lbls, labelFilters) {
						continue
					}
					backportPRs[pr.GetNumber()][upstreamPRNumber] = types.PullRequest{
						ReleaseNote:  getReleaseNote(upstreamPR.GetTitle(), upstreamPR.GetBody()),
						ReleaseLabel: getReleaseLabel(lbls),
						AuthorName:   upstreamPR.GetUser().GetLogin(),
					}
				}
			}

			page = resp.NextPage
			if page == 0 {
				break
			}
		}
		if !foundPR {
			printer(fmt.Sprintf("WARNING: PR not found for commit %s!\n", sha))
		}
	}
	return backportPRs, listOfPRs, nil, nil
}
