// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"
)

func NewCheckReleaseBlockers(cfg *ReleaseConfig) Step {
	return &CheckReleaseBlockers{
		cfg: cfg,
	}
}

type CheckReleaseBlockers struct {
	cfg *ReleaseConfig
}

func (c *CheckReleaseBlockers) Name() string {
	return "checking for release blockers"
}

func (c *CheckReleaseBlockers) Run(ctx context.Context, yesToPrompt, _ bool, ghClient *GHClient) error {
	if !c.cfg.HasStableBranch() {
		io.Fprintf(1, os.Stdout, "On Pre-Releases there aren't 'release blockers'."+
			" Continuing with the release process.\n")
		return nil
	}

	releaseBlockerLabel := github.ReleaseBlockerLabel(c.cfg.TargetVer)
	backportDoneLabel := github.BackportDoneLabel(c.cfg.TargetVer)

	baseBranch, err := ghClient.getDefaultBranch(ctx, c.cfg.Owner, c.cfg.Repo)
	if err != nil {
		return err
	}

	releaseDate, err := ghClient.getTagDate(ctx, c.cfg.Owner, c.cfg.Repo, c.cfg.PreviousVer)
	if err != nil {
		return err
	}

	prBlockedQuery := releaseBlockerPRsQuery(releaseDate, baseBranch, backportDoneLabel, releaseBlockerLabel, c.cfg.Owner, c.cfg.Repo)

	io.Fprintf(1, os.Stdout, "👀 Checking for opened GH issues and pull requests with the label %q "+
		"and for closed GH Pull Requests with that same label that are not backported yet but got merged "+
		"in the '%s' branch after '%s': \n"+
		"   https://github.com/%s/%s/labels/%s\n"+
		"   https://github.com/%s/%s/issues?q=is%%3Aopen+label:%s+-is%%3Adraft\n"+
		"   https://github.com/%s/%s/issues?q=%s\n",
		releaseBlockerLabel,
		baseBranch,
		releaseDate,
		c.cfg.Owner, c.cfg.Repo, releaseBlockerLabel,
		c.cfg.Owner, c.cfg.Repo, releaseBlockerLabel,
		c.cfg.Owner, c.cfg.Repo,
		url.PathEscape(prBlockedQuery))

	found, err := c.checkGHBlockers(ctx, ghClient, releaseBlockerLabel, prBlockedQuery)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("Found outstanding release blockers. Please resolve them before continuing release process")
	} else {
		io.Fprintf(1, os.Stdout, "✅ All release blockers merged.\n")
	}

	branchName := semver.MajorMinor(c.cfg.TargetVer)
	backportLabel := github.BackportLabel(c.cfg.TargetVer)
	openedBackportPRsQuery := openedBackportPRsQuery(branchName, backportLabel, c.cfg.Owner, c.cfg.Repo)

	io.Fprintf(1, os.Stdout,
		"👀 Checking for outstanding backport PRs in: "+
			"https://github.com/%s/%s/issues?q=%s\n",
		c.cfg.Owner, c.cfg.Repo,
		url.PathEscape(openedBackportPRsQuery))

	found, err = c.checkBackports(ctx, ghClient, openedBackportPRsQuery)
	if err != nil {
		return err
	}
	if found {
		if yesToPrompt {
			fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
		} else {
			err := io.ContinuePrompt(
				"⚠️ Found opened backports. Do you want to continue the release process?",
				"✋ Backports found, stopping the release process",
			)
			if err != nil {
				return err
			}
		}
	} else {
		io.Fprintf(1, os.Stdout, "✅ All backports merged.\n")
	}

	return nil
}

func (c *CheckReleaseBlockers) checkBackports(ctx context.Context, ghClient *GHClient, query string) (bool, error) {
	page := 0
	var found bool
	for {
		ghIssues, resp, err := ghClient.ghClient.Search.Issues(ctx, query, &gh.SearchOptions{
			TextMatch: true,
			ListOptions: gh.ListOptions{
				Page: page,
			},
		})
		if err != nil {
			return found, nil
		}
		if len(ghIssues.Issues) != 0 && !found {
			found = true
			io.Fprintf(2, os.Stderr, "⚠️ Found opened backports:\n")
		}
		for _, ghIssue := range ghIssues.Issues {
			io.Fprintf(2, os.Stderr, " %s - %s\n", ghIssue.GetHTMLURL(), ghIssue.GetTitle())
		}
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return found, nil
}

func (c *CheckReleaseBlockers) checkGHBlockers(ctx context.Context, ghClient *GHClient, releaseBlockerLabel, prQuery string) (bool, error) {
	page := 0
	var found bool
	queries := []string{
		// Check all GH issues and PRs that are opened with the release blocker label
		fmt.Sprintf("is:open is:issue label:%s repo:%s/%s", releaseBlockerLabel, c.cfg.Owner, c.cfg.Repo),
		fmt.Sprintf("is:open is:pull-request -is:draft label:%s repo:%s/%s", releaseBlockerLabel, c.cfg.Owner, c.cfg.Repo),
		// Check all PRs that are closed to main, marked as a release blocked
		// and haven't been backported yet.
		prQuery,
	}
	for _, q := range queries {
		for {
			ghIssues, resp, err := ghClient.ghClient.Search.Issues(ctx, q, &gh.SearchOptions{
				TextMatch: true,
				ListOptions: gh.ListOptions{
					Page: page,
				},
			})
			if err != nil {
				return found, err
			}
			if len(ghIssues.Issues) != 0 && !found {
				found = true
				io.Fprintf(2, os.Stderr, "⚠️ Found release blockers:\n")
			}
			for _, ghIssue := range ghIssues.Issues {
				io.Fprintf(2, os.Stderr, "%s - %s\n", ghIssue.GetHTMLURL(), ghIssue.GetTitle())
			}
			if resp.NextPage == 0 {
				break
			}
			page = resp.NextPage
		}
	}
	return found, nil
}

func releaseBlockerPRsQuery(stableReleaseDate, baseBranchName, backportDoneLabel, releaseBlockerLabel, owner, repo string) string {
	return fmt.Sprintf(
		"is:pull-request "+
			"state:closed "+
			"is:merged "+
			"merged:>=%s "+
			"base:%s "+
			"-label:%s "+
			"label:%s "+
			"repo:%s/%s",
		stableReleaseDate,
		baseBranchName,
		backportDoneLabel,
		releaseBlockerLabel,
		owner,
		repo,
	)
}

func openedBackportPRsQuery(branchName, backportLabel, owner, repo string) string {
	return fmt.Sprintf(
		"is:pull-request "+
			"state:open "+
			"-is:draft "+
			"base:%s "+
			"label:%s "+
			"repo:%s/%s",
		branchName,
		backportLabel,
		owner,
		repo,
	)
}

func (c *CheckReleaseBlockers) Revert(ctx context.Context, dryRun bool, ghClient *GHClient) error {
	return nil
}
