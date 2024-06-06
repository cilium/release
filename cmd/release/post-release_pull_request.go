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

func (pc *PustPostPullRequest) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *gh.Client) error {
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

	var preRelease string
	if semver.Prerelease(pc.cfg.TargetVer) != "" {
		preRelease = "-p"
	}

	ersion := strings.TrimPrefix(pc.cfg.TargetVer, "v")
	_, err = execCommand(pc.cfg.RepoDirectory,
		"gh",
		"release",
		"create",
		"--draft",
		preRelease,
		"--notes-file",
		releaseSummaryFileName,
		pc.cfg.TargetVer,
		"--title",
		ersion,
	)

	if err != nil {
		return err
	}

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

	o, err := execCommand(pc.cfg.RepoDirectory, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	commitShaRaw, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	remoteBranchName := strings.TrimSpace(string(commitShaRaw))

	// hub api user --flat | awk '/.login/ {print $2}'
	user, err := pipeCommands(ctx, false, pc.cfg.RepoDirectory,
		"hub", []string{"api", "user", "--flat"},
		"awk", []string{"/.login/ {print $2}"},
	)
	if err != nil {
		return err
	}
	userRaw, err := io.ReadAll(user)
	if err != nil {
		return err
	}
	userRemote := strings.TrimSpace(string(userRaw))

	remoteName, err := getRemote(pc.cfg.RepoDirectory, userRemote, pc.cfg.Repo)
	if err != nil {
		return err
	}

	_, err = execCommand(pc.cfg.RepoDirectory, "git", "push", remoteName, remoteBranchName)
	if err != nil {
		return err
	}

	_, err = execCommand(pc.cfg.RepoDirectory,
		"hub",
		"pull-request",
		"-o",
		"--no-edit",
		"-b", baseBranch,
		"-l", "backport/"+github.MajorMinorErsion(baseBranch))
	if err != nil {
		return err
	}

	return nil
}

func (pc *PustPostPullRequest) Revert(ctx context.Context, dryRun bool, ghClient *gh.Client) error {
	return fmt.Errorf("Not implemented")
}
