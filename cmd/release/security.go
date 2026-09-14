// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/cilium/release/pkg/github"
	"github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"
)

func NewCheckSecurityBlockers(cfg *ReleaseConfig) Step {
	return &CheckSecurityBlockers{
		cfg: cfg,
	}
}

type CheckSecurityBlockers struct {
	cfg *ReleaseConfig
}

func (c *CheckSecurityBlockers) Name() string {
	return "checking for security release blockers"
}

// Run looks for security fixes that are still open for the release. They are
// tracked in a private repository that most release managers cannot read, so
// losing access to it warns instead of stopping the release process.
func (c *CheckSecurityBlockers) Run(ctx context.Context, yesToPrompt, _ bool, ghClient *GHClient) error {
	if semver.Prerelease(c.cfg.TargetVer) != "" {
		io.Fprintf(1, os.Stdout, "On Pre-Releases and Release Candidates there aren't 'security"+
			" release blockers'. Continuing with the release process.\n")
		return nil
	}

	owner, repo := c.cfg.Security.Owner, c.cfg.Security.Repo
	releaseBlockerLabel := github.ReleaseBlockerLabel(c.cfg.TargetVer)
	query := securityBlockersQuery(releaseBlockerLabel, owner, repo)

	io.Fprintf(1, os.Stdout, "👀 Checking for security patches with the label %q that are still open in:\n"+
		"   https://github.com/%s/%s/issues?q=%s\n",
		releaseBlockerLabel, owner, repo, url.PathEscape(query))

	// A search cannot tell an invisible repository from a misspelled one, so ask
	// about the repository itself first.
	if _, _, err := ghClient.ghClient.Repositories.Get(ctx, owner, repo); err != nil {
		if !repoNotVisible(err) {
			return fmt.Errorf("unable to fetch repository %s/%s: %w", owner, repo, err)
		}
		c.warnNotChecked(fmt.Sprintf("No access to the private %s/%s repository", owner, repo), releaseBlockerLabel)
		return nil
	}

	found, truncated, err := c.searchSecurityBlockers(ctx, ghClient, query)
	if err != nil {
		if !searchRepoNotVisible(err) {
			return err
		}
		c.warnNotChecked(fmt.Sprintf("The %s/%s repository is visible but its issues cannot be searched with this token", owner, repo), releaseBlockerLabel)
		return nil
	}
	if truncated {
		io.Fprintf(1, os.Stdout, "⚠️ GitHub truncated the search results, there may be security release blockers that are not listed above.\n")
	}
	if !found && !truncated {
		io.Fprintf(1, os.Stdout, "✅ No security release blockers.\n")
		return nil
	}

	// A truncated search rules nothing out, so it asks for the same confirmation.
	prompt, declined := "⚠️ Found security release blockers. Do you want to continue the release process?",
		"✋ Security release blockers found, stopping the release process"
	if !found {
		prompt, declined = "⚠️ Unable to confirm there are no security release blockers. Do you want to continue the release process?",
			"✋ Security release blockers not ruled out, stopping the release process"
	}

	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
		io.Fprintf(1, os.Stdout, "Confirm with @cilium/security that %s is not affected.\n", c.cfg.TargetVer)
		return nil
	}
	return io.ContinuePrompt(prompt, declined)
}

// warnNotChecked reports that the security release blockers could not be checked
// and what has to be done by hand instead.
func (c *CheckSecurityBlockers) warnNotChecked(reason, releaseBlockerLabel string) {
	io.Fprintf(1, os.Stdout, "⚠️ %s, security release blockers were not checked.\n", reason)
	io.Fprintf(2, os.Stdout, "Ask @cilium/security to confirm there is no open %q before performing the release.\n", releaseBlockerLabel)
}

// searchSecurityBlockers prints every item returned by query and reports whether
// any was found and whether GitHub truncated the search results.
func (c *CheckSecurityBlockers) searchSecurityBlockers(ctx context.Context, ghClient *GHClient, query string) (bool, bool, error) {
	page := 0
	var found, truncated bool
	for {
		ghIssues, resp, err := ghClient.ghClient.Search.Issues(ctx, query, &gh.SearchOptions{
			TextMatch: true,
			ListOptions: gh.ListOptions{
				Page: page,
			},
		})
		if err != nil {
			return found, truncated, err
		}
		truncated = truncated || ghIssues.GetIncompleteResults()
		if len(ghIssues.Issues) != 0 && !found {
			found = true
			io.Fprintf(2, os.Stderr, "⚠️ Found security release blockers:\n")
		}
		for _, ghIssue := range ghIssues.Issues {
			io.Fprintf(2, os.Stderr, "%s - %s\n", ghIssue.GetHTMLURL(), ghIssue.GetTitle())
		}
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return found, truncated, nil
}

// securityBlockersQuery builds the GitHub search that returns every open item
// carrying the given release blocker label in the security repository. It is
// pure so it can be unit-tested without hitting the GitHub API.
func securityBlockersQuery(releaseBlockerLabel, owner, repo string) string {
	return fmt.Sprintf(
		"is:open "+
			"label:%s "+
			"repo:%s/%s",
		releaseBlockerLabel,
		owner,
		repo,
	)
}

// repoNotVisible reports whether err means the token cannot see the repository:
// GitHub answers 404 for a private repository it hides and 403 when an
// organization denies the token.
func repoNotVisible(err error) bool {
	var rateLimitErr *gh.RateLimitError
	var abuseRateLimitErr *gh.AbuseRateLimitError
	if errors.As(err, &rateLimitErr) || errors.As(err, &abuseRateLimitErr) {
		return false
	}
	var ghErr *gh.ErrorResponse
	if !errors.As(err, &ghErr) || ghErr.Response == nil {
		return false
	}
	switch ghErr.Response.StatusCode {
	case http.StatusNotFound:
		return true
	case http.StatusForbidden:
		// A secondary rate limit is a 403 as well, only the headers tell them apart.
		return ghErr.Response.Header.Get("Retry-After") == "" &&
			ghErr.Response.Header.Get("X-RateLimit-Remaining") != "0"
	}
	return false
}

// searchRepoNotVisible reports whether a search failed because a repository in
// the query cannot be searched with this token.
func searchRepoNotVisible(err error) bool {
	var ghErr *gh.ErrorResponse
	if !errors.As(err, &ghErr) || ghErr.Response == nil {
		return false
	}
	if ghErr.Response.StatusCode != http.StatusUnprocessableEntity {
		return false
	}
	for _, ghSearchErr := range ghErr.Errors {
		if ghSearchErr.Resource == "Search" && ghSearchErr.Field == "q" && ghSearchErr.Code == "invalid" {
			return true
		}
	}
	return false
}
