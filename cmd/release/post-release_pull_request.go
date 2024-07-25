// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cilium/release/pkg/github"
	io2 "github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
	github2 "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"
)

type PustPostPullRequest struct {
	cfg *ReleaseConfig
}

func NewSubmitPostReleasePR(cfg *ReleaseConfig) *PustPostPullRequest {
	return &PustPostPullRequest{
		cfg: cfg,
	}
}

func (pc *PustPostPullRequest) Name() string {
	return "Creating Pull Request"
}

func (pc *PustPostPullRequest) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *GHClient) error {
	io2.Fprintf(1, os.Stdout, "📜 Generating a DRAFT GitHub Release\n")
	// Generate release summary
	changelogFile := filepath.Join(pc.cfg.RepoDirectory, "CHANGELOG.md")
	changelogContent, err := os.Open(changelogFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error reading CHANGELOG.md file: %w", err)
		} else {
			return fmt.Errorf("CHANGELOG.md file not found, it needs to be present to create a release on GitHub")
		}
	}
	defer changelogContent.Close()

	digestFileName := fmt.Sprintf("digest-%s.txt", pc.cfg.TargetVer)
	digestFile := filepath.Join(pc.cfg.RepoDirectory, digestFileName)
	digestFileContent, err := os.Open(digestFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error reading %s file: %w", digestFileName, err)
		} else {
			return fmt.Errorf("%s file not found, it needs to be present to create a release on GitHub", digestFileName)
		}
	}
	defer digestFileContent.Close()

	releaseSummaryFileName := fmt.Sprintf("%s-release-summary.txt", pc.cfg.TargetVer)
	releaseSummaryFile := filepath.Join(pc.cfg.RepoDirectory, releaseSummaryFileName)
	releaseSummaryFileContent, err := os.Create(releaseSummaryFile)
	if err != nil {
		return fmt.Errorf("unable to create summary file: %w", err)
	}
	defer releaseSummaryFileContent.Close()

	detectVersion := regexp.MustCompile(`^## v.*$`)
	scanner := bufio.NewScanner(changelogContent)
	for i := 0; scanner.Scan(); i++ {
		// Ignore the first four lines
		if i < 4 {
			continue
		}
		// Stop when we hit the previous version in the CHANGELOG.md
		if detectVersion.Match(scanner.Bytes()) {
			break
		}
		releaseSummaryFileContent.Write(append(scanner.Bytes(), byte('\n')))
	}

	if _, err := io.Copy(releaseSummaryFileContent, digestFileContent); err != nil {
		return fmt.Errorf("Unable to copy the digest file conent into the release summary file")
	}
	releaseSummaryFileContent.Close()

	releaseSummaryFileContentBytes, err := os.ReadFile(releaseSummaryFile)
	if err != nil {
		return err
	}

	releaseSummaryFileContentStr := string(releaseSummaryFileContentBytes)

	ersion := strings.TrimPrefix(pc.cfg.TargetVer, "v")
	_, _, err = ghClient.ghClient.Repositories.CreateRelease(
		ctx,
		pc.cfg.Owner,
		pc.cfg.Repo,
		&gh.RepositoryRelease{
			TagName:              &pc.cfg.TargetVer,
			Name:                 &ersion,
			Body:                 &releaseSummaryFileContentStr,
			Draft:                func() *bool { a := true; return &a }(),
			Prerelease:           func() *bool { a := semver.Prerelease(pc.cfg.TargetVer) != ""; return &a }(),
			MakeLatest:           func() *string { a := "false"; return &a }(),
			GenerateReleaseNotes: func() *bool { a := false; return &a }(),
		},
	)

	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(1, os.Stdout, "Pre-Releases don't need to have a post pull request done "+
			"as we don't commit the digests.\n")
		return nil
	}
	io2.Fprintf(1, os.Stdout, "📤 Submitting changes to a PR\n")

	baseBranch := semver.MajorMinor(pc.cfg.TargetVer)
	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
	} else {
		err := io2.ContinuePrompt(
			fmt.Sprintf("Create PR for %s with these changes?", baseBranch),
			"Stopping release preparation.",
		)
		if err != nil {
			return err
		}
	}

	o, err := execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	commitShaRaw, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	remoteBranchName := strings.TrimSpace(string(commitShaRaw))

	// Default to "owner" if we can't get the user from gh api
	userRemote := pc.cfg.Owner

	user, err := execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "gh", "api", "user", "--jq", ".login")
	if err == nil {
		userRaw, err := io.ReadAll(user)
		if err != nil {
			return err
		}
		userRemote = strings.TrimSpace(string(userRaw))
	} else {
		io2.Fprintf(3, os.Stdout, "⚠️ Unable to get GH user, falling back to %q\n", userRemote)
	}

	remoteName, err := getRemote(pc.cfg.RepoDirectory, userRemote, pc.cfg.Repo)
	if err != nil {
		return err
	}

	_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "push", "-f", remoteName, remoteBranchName)
	if err != nil {
		return err
	}

	labels := []string{"kind/release"}
	if pc.cfg.HasStableBranch() {
		labels = append(labels, github.BackportLabel(pc.cfg.TargetVer))
	}
	// Check if PR already exists for this branch.
	prs, _, err := ghClient.ghClient.PullRequests.List(ctx, pc.cfg.Owner, pc.cfg.Repo, &github2.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", userRemote, remoteBranchName),
		Base:  baseBranch,
	})
	if err != nil {
		return err
	}
	if len(prs) > 0 {
		io2.Fprintf(2, os.Stdout, "📤 Pull request is already open: %s\n", prs[0].GetHTMLURL())
	} else {
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory,
			"gh",
			"pr",
			"create",
			"--fill",
			"--base",
			baseBranch,
			"--head",
			fmt.Sprintf("%s:%s", userRemote, remoteBranchName),
			"-l", strings.Join(labels, ","))
	}

	return err
}
