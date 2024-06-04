// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"

	"github.com/cilium/release/pkg/github"
	gh "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"
)

// Returns all tags for the given owner and repo.
func getTags(ctx context.Context, ghClient *gh.Client, owner, repo string) ([]string, error) {
	nextPage := 0
	var repositoryTags []string
	for {
		tags, resp, err := ghClient.Repositories.ListTags(ctx, owner, repo, &gh.ListOptions{
			Page: nextPage,
		})
		if err != nil {
			return nil, err
		}
		nextPage = resp.NextPage
		if nextPage == 0 {
			break
		}
		for _, t := range tags {
			repositoryTags = append(repositoryTags, t.GetName())
		}
	}
	return repositoryTags, nil
}

func getRemoteBranch(ctx context.Context, ghClient *gh.Client, owner, repo, targetVer string) (string, error) {
	page := 0
	for {
		branches, resp, err := ghClient.Repositories.ListBranches(ctx, owner, repo, &gh.BranchListOptions{
			Protected: func() *bool { a := true; return &a }(),
			ListOptions: gh.ListOptions{
				Page: page,
			},
		})
		if err != nil {
			return "", err
		}
		for _, br := range branches {
			majMinor := semver.MajorMinor(targetVer)
			if majMinor == br.GetName() {
				return br.GetName(), nil
			}
		}
		page = resp.NextPage
		if page == 0 {
			return "", nil
		}
	}
}

func previousVersion(ctx context.Context, ghClient *gh.Client, owner, repo, currentVersion string) (string, error) {
	allTags, err := getTags(ctx, ghClient, owner, repo)
	if err != nil {
		return "", err
	}

	allTags = append(allTags, currentVersion)

	sortedTags, err := github.SortTags(allTags)
	if err != nil {
		return "", err
	}

	return github.PreviousTagOf(sortedTags, currentVersion), nil
}
