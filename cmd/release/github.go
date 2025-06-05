// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cilium/release/pkg/github"
	gh "github.com/google/go-github/v62/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
)

type GHClient struct {
	ghClient    *gh.Client
	ghGQLClient *githubv4.Client
}

func NewGHClient(logger *log.Logger) *GHClient {
	return &GHClient{
		ghClient: github.NewClient(logger),
		ghGQLClient: githubv4.NewClient(
			oauth2.NewClient(
				context.Background(),
				oauth2.StaticTokenSource(
					&oauth2.Token{
						AccessToken: github.Token(),
					},
				),
			),
		),
	}
}

// Returns all tags for the given owner and repo.
func (ghClient *GHClient) getTags(ctx context.Context, owner, repo string) ([]string, error) {
	nextPage := 0
	var repositoryTags []string
	for {
		tags, resp, err := ghClient.ghClient.Repositories.ListTags(ctx, owner, repo, &gh.ListOptions{
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

func (ghClient *GHClient) getRemoteBranch(ctx context.Context, owner, repo, targetVer string) (string, error) {
	page := 0
	for {
		branches, resp, err := ghClient.ghClient.Repositories.ListBranches(ctx, owner, repo, &gh.BranchListOptions{
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

func (ghClient *GHClient) previousVersion(ctx context.Context, owner, repo, currentVersion string) (string, error) {
	allTags, err := ghClient.getTags(ctx, owner, repo)
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

// getTagDate returns the release date in YYYY-MM-DD format of the target tag
// version.
func (ghClient *GHClient) getTagDate(ctx context.Context, owner, repo, tagVersion string) (string, error) {
	ref, _, err := ghClient.ghClient.Git.GetRef(ctx, owner, repo, "refs/tags/"+tagVersion)
	if err != nil {
		return "", err
	}
	tagSHA := ref.GetObject().GetSHA()

	tag, _, err := ghClient.ghClient.Git.GetTag(ctx, owner, repo, tagSHA)
	if err != nil {
		return "", err
	}

	minorReleaseDate := tag.GetTagger().GetDate().Format(time.DateOnly)
	return minorReleaseDate, nil
}

// getDefaultBranch returns the base branch for the repository in the configuration.
func (ghClient *GHClient) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repository, _, err := ghClient.ghClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("unable to fetch repository for %s/%s: %s", owner, repo, err)
	}
	baseBranch := repository.GetDefaultBranch()
	if baseBranch == "" {
		return "", fmt.Errorf("unable to get base branch for repository %s/%s. The base branch is empty", owner, repo)
	}
	return baseBranch, nil
}

// getWFRunForTag returns the WF run HTMLURL of the given tag for the given
// workflowFileName.
func (ghClient *GHClient) getWFRunForTag(ctx context.Context, owner, repo, workflowFileName, targetVersion string) string {
	page := 0
	for {
		runs, resp, err := ghClient.ghClient.Actions.ListWorkflowRunsByFileName(ctx, owner, repo, workflowFileName, &gh.ListWorkflowRunsOptions{
			ExcludePullRequests: true,
			ListOptions: gh.ListOptions{
				Page: page,
			},
		})
		if err != nil {
			log.Fatalf("Error listing workflow runs: %v", err)
		}

		for _, run := range runs.WorkflowRuns {
			if run.GetHeadBranch() == targetVersion {
				return run.GetHTMLURL()
			}
		}
		page = resp.NextPage
		if page == 0 {
			break
		}
	}
	return ""
}
