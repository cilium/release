// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

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
	if semver.Prerelease(c.cfg.TargetVer) != "" {
		io.Fprintf(1, os.Stdout, "On Pre-Releases there aren't 'release blockers'."+
			" Continuing with the release process.\n")
		return nil
	}

	releaseBlockerLabel := github.ReleaseBlockerLabel(c.cfg.TargetVer)
	backportDoneLabel := github.BackportDoneLabel(c.cfg.TargetVer)

	baseBranch := c.cfg.DefaultBranch

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

	if err := c.checkBackportOrdering(ctx, yesToPrompt, ghClient); err != nil {
		return err
	}

	return nil
}

// checkBackportOrdering enforces the invariant that a fix present in a given
// stable branch must also be present in every newer active stable branch.
// Upgrades always move from an older to a newer minor version, so a fix that
// lands in an older branch while still missing from a newer one is an upgrade
// regression.
//
// The check is performed relative to the branch being released (TargetVer),
// covering both directions of the invariant:
//
//   - the released branch is behind an OLDER branch: a PR is backport-done on
//     an older branch but still needs-backport / backport-pending on the
//     released branch; and
//   - the released branch is ahead of a NEWER branch: a PR is backport-done on
//     the released branch but still needs-backport / backport-pending on a
//     newer branch.
//
// Any violation is a hard block on the release process, overridable with
// --force (yesToPrompt).
func (c *CheckReleaseBlockers) checkBackportOrdering(ctx context.Context, yesToPrompt bool, ghClient *GHClient) error {
	allStableBranches, err := ghClient.getStableBranches(ctx, c.cfg.Owner, c.cfg.Repo)
	if err != nil {
		return err
	}

	// Cilium only actively maintains the most-recent minors. Comparing against
	// EOL branches (which may still carry stale needs-backport labels) would
	// produce false positives, so restrict the comparison to the maintained set
	// (plus the branch being released).
	stableBranches := maintainedStableBranches(allStableBranches, c.cfg.TargetVer, c.cfg.MaintainedMinors)

	queries := backportOrderingQueries(c.cfg.TargetVer, stableBranches, c.cfg.Owner, c.cfg.Repo)
	if len(queries) == 0 {
		io.Fprintf(1, os.Stdout, "✅ No other actively-maintained stable branches to compare backport ordering against.\n")
		return nil
	}

	relMM := semver.MajorMinor(c.cfg.TargetVer)
	var otherBranches []string
	for _, branch := range stableBranches {
		if semver.MajorMinor(branch) != relMM {
			otherBranches = append(otherBranches, branch)
		}
	}
	io.Fprintf(1, os.Stdout,
		"👀 Checking that every backport in %s is consistent with the other actively-maintained stable branches (%s)\n",
		relMM, strings.Join(otherBranches, ", "))

	found, err := c.runBackportOrderingQueries(ctx, ghClient, queries)
	if err != nil {
		return err
	}
	if !found {
		io.Fprintf(1, os.Stdout, "✅ Backport ordering invariant satisfied across all active stable branches.\n")
		return nil
	}

	if yesToPrompt {
		io.Fprintf(1, os.Stdout, "⏩ --force set, continuing despite backport ordering violations.\n")
		return nil
	}
	return fmt.Errorf("found backport ordering violations. A fix present in one stable branch must also be present in every newer active stable branch. " +
		"Please ensure the listed pull requests are backported (or that their backport candidates are frozen consistently) before continuing the release process")
}

// maintainedStableBranches restricts allBranches (bare major.minor branch names
// such as "v1.15") to the maintainedMinors most-recent minors, always including
// the branch being released (relVer) even if it falls outside that window (e.g.
// when cutting a patch for an about-to-be-EOL branch). The returned slice is
// sorted ascending by version and de-duplicated by major.minor. A
// maintainedMinors <= 0 disables the limit and keeps every branch. It is pure so
// it can be unit-tested without hitting the GitHub API.
func maintainedStableBranches(allBranches []string, relVer string, maintainedMinors int) []string {
	relMM := semver.MajorMinor(relVer)

	// Keep only valid major.minor branches, de-duplicated.
	seen := make(map[string]struct{})
	var branches []string
	for _, b := range allBranches {
		mm := semver.MajorMinor(b)
		if mm == "" || !semver.IsValid(mm) {
			continue
		}
		if _, ok := seen[mm]; ok {
			continue
		}
		seen[mm] = struct{}{}
		branches = append(branches, mm)
	}

	// Ensure the released branch is always part of the set.
	if relMM != "" {
		if _, ok := seen[relMM]; !ok {
			branches = append(branches, relMM)
		}
	}

	// Sort ascending by semver (v1.9 < v1.10).
	sort.Slice(branches, func(i, j int) bool {
		return semver.Compare(branches[i], branches[j]) < 0
	})

	// Keep everything when the limit is disabled.
	if maintainedMinors <= 0 {
		return branches
	}

	// Keep the maintainedMinors newest branches, then re-add the released branch
	// if the window dropped it.
	if len(branches) > maintainedMinors {
		branches = branches[len(branches)-maintainedMinors:]
	}
	if relMM != "" {
		found := false
		for _, b := range branches {
			if b == relMM {
				found = true
				break
			}
		}
		if !found {
			branches = append(branches, relMM)
			sort.Slice(branches, func(i, j int) bool {
				return semver.Compare(branches[i], branches[j]) < 0
			})
		}
	}
	return branches
}

// backportOrderingViolation is a single GitHub search that surfaces pull
// requests violating the backport ordering invariant, together with a
// human-readable reason.
type backportOrderingViolation struct {
	reason string
	query  string
}

// backportOrderingQueries builds the set of GitHub searches that detect
// backport ordering violations relative to relVer, given the list of active
// stable branches (bare major.minor branch names, e.g. "v1.15"). It is pure so
// it can be unit-tested without hitting the GitHub API.
func backportOrderingQueries(relVer string, stableBranches []string, owner, repo string) []backportOrderingViolation {
	relMM := semver.MajorMinor(relVer)

	relDone := github.BackportDoneLabel(relVer)
	relNeeds := github.NeedsBackportLabel(relVer)
	relPending := github.BackportPendingLabel(relVer)

	var violations []backportOrderingViolation
	for _, branch := range stableBranches {
		branchMM := semver.MajorMinor(branch)
		// Skip the branch being released and anything that is not a valid
		// major.minor stable branch.
		if branchMM == "" || branchMM == relMM {
			continue
		}

		branchDone := github.BackportDoneLabel(branch)
		branchNeeds := github.NeedsBackportLabel(branch)
		branchPending := github.BackportPendingLabel(branch)

		switch {
		case semver.Compare(branchMM, relMM) < 0:
			// The released branch is behind an OLDER branch: the fix is already
			// in the older branch but is still missing from the branch we are
			// about to release.
			violations = append(violations,
				backportOrderingViolation{
					reason: fmt.Sprintf("present in older branch %s (%s) but still needs backport to %s (%s)", branchMM, branchDone, relMM, relNeeds),
					query:  orderingQuery(owner, repo, branchDone, relNeeds),
				},
				backportOrderingViolation{
					reason: fmt.Sprintf("present in older branch %s (%s) but backport to %s is still pending (%s)", branchMM, branchDone, relMM, relPending),
					query:  orderingQuery(owner, repo, branchDone, relPending),
				},
			)
		default:
			// The released branch is ahead of a NEWER branch: the fix is in the
			// branch we are about to release but is still missing from a newer
			// branch.
			violations = append(violations,
				backportOrderingViolation{
					reason: fmt.Sprintf("present in %s (%s) but still needs backport to newer branch %s (%s)", relMM, relDone, branchMM, branchNeeds),
					query:  orderingQuery(owner, repo, relDone, branchNeeds),
				},
				backportOrderingViolation{
					reason: fmt.Sprintf("present in %s (%s) but backport to newer branch %s is still pending (%s)", relMM, relDone, branchMM, branchPending),
					query:  orderingQuery(owner, repo, relDone, branchPending),
				},
			)
		}
	}
	return violations
}

// orderingQuery builds a GitHub issue search that returns merged pull requests
// carrying both presentLabel (the branch where the fix already landed) and
// missingLabel (a branch where the fix is still absent).
func orderingQuery(owner, repo, presentLabel, missingLabel string) string {
	return fmt.Sprintf(
		"is:pull-request "+
			"is:merged "+
			"label:%s "+
			"label:%s "+
			"repo:%s/%s",
		presentLabel,
		missingLabel,
		owner,
		repo,
	)
}

func (c *CheckReleaseBlockers) runBackportOrderingQueries(ctx context.Context, ghClient *GHClient, violations []backportOrderingViolation) (bool, error) {
	var found bool
	for _, v := range violations {
		page := 0
		var headerPrinted bool
		for {
			ghIssues, resp, err := ghClient.ghClient.Search.Issues(ctx, v.query, &gh.SearchOptions{
				TextMatch: true,
				ListOptions: gh.ListOptions{
					Page: page,
				},
			})
			if err != nil {
				return found, err
			}
			if len(ghIssues.Issues) != 0 && !headerPrinted {
				headerPrinted = true
				if !found {
					io.Fprintf(2, os.Stderr, "⚠️ Found backport ordering violations:\n")
				}
				found = true
				io.Fprintf(2, os.Stderr, " • %s:\n", v.reason)
				io.Fprintf(3, os.Stderr, "https://github.com/%s/%s/issues?q=%s\n",
					c.cfg.Owner, c.cfg.Repo, url.PathEscape(v.query))
			}
			for _, ghIssue := range ghIssues.Issues {
				io.Fprintf(3, os.Stderr, "%s - %s\n", ghIssue.GetHTMLURL(), ghIssue.GetTitle())
			}
			if resp.NextPage == 0 {
				break
			}
			page = resp.NextPage
		}
	}
	return found, nil
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
