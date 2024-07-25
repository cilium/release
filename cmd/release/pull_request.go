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
	"strings"

	"github.com/cilium/release/pkg/github"
	io2 "github.com/cilium/release/pkg/io"
	github2 "github.com/google/go-github/v62/github"
)

type PushPullRequest struct {
	cfg *ReleaseConfig
}

func NewSubmitPR(cfg *ReleaseConfig) *PushPullRequest {
	return &PushPullRequest{
		cfg: cfg,
	}
}

func (pc *PushPullRequest) Name() string {
	return "Creating Pull Request"
}

func (pc *PushPullRequest) Run(ctx context.Context, _, _ bool, ghClient *GHClient) error {
	io2.Fprintf(1, os.Stdout, "📤 Submitting changes to a PR\n")

	baseBranch := pc.cfg.RemoteBranchName
	if !pc.cfg.HasStableBranch() {
		baseBranch = pc.cfg.DefaultBranch
	}

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

	localBranch := fmt.Sprintf("pr/prepare-%s", pc.cfg.TargetVer)

	io2.Fprintf(2, os.Stdout, "📤 Pushing branch %q to remote %q\n", localBranch, remoteName)

	// Revert the "Prepare for release" commit since that commit will only be
	// used for a tag.
	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch, pushing HEAD^ changes before creating PR\n")
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "push", "-f", remoteName, "HEAD^:refs/heads/"+localBranch)
	} else {
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "push", "-f", remoteName, localBranch)
	}
	if err != nil {
		return err
	}

	io2.Fprintf(2, os.Stdout, "📜 Generating summary file for PR\n")
	prTitle, prBodyFile, err := pc.generateSummaryFile()
	if err != nil {
		return err
	}

	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch, pushing remaining changes into PR\n")
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "push", remoteName, localBranch)
		if err != nil {
			return err
		}
	}

	labels := []string{"kind/release", "release-note/misc"}
	if pc.cfg.HasStableBranch() {
		labels = append(labels, github.BackportLabel(pc.cfg.TargetVer))
	}

	// Check if PR already exists for this branch.
	prs, _, err := ghClient.ghClient.PullRequests.List(ctx, pc.cfg.Owner, pc.cfg.Repo, &github2.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", userRemote, localBranch),
		Base:  baseBranch,
	})
	if err != nil {
		return err
	}
	if len(prs) > 0 {
		io2.Fprintf(2, os.Stdout, "📤 Pull request is already open: %s\n", prs[0].GetHTMLURL())
	} else {
		io2.Fprintf(2, os.Stdout, "📤 Creating PR...\n")
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory,
			"gh",
			"pr",
			"create",
			"--base",
			baseBranch,
			"--head",
			fmt.Sprintf("%s:%s", userRemote, localBranch),
			"--label", strings.Join(labels, ","),
			"--body-file", prBodyFile,
			"--title", prTitle)
	}

	return err
}

func (pc *PushPullRequest) generateSummaryFile() (string, string, error) {
	prTitle := fmt.Sprintf("Prepare for release %s", pc.cfg.TargetVer)

	changesFileName := fmt.Sprintf("%s-changes.txt", pc.cfg.TargetVer)
	changesFile := filepath.Join(pc.cfg.RepoDirectory, changesFileName)
	changesContent, err := os.Open(changesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("error reading %s file: %w", changesFileName, err)
		} else {
			return "", "", fmt.Errorf("%s file not found, it needs to be present to create a PR on GitHub", changesFileName)
		}
	}
	defer changesContent.Close()

	prBodyFileName := fmt.Sprintf("%s-pr-body.txt", pc.cfg.TargetVer)
	prBodyFile := filepath.Join(pc.cfg.RepoDirectory, prBodyFileName)
	prBodyFileContent, err := os.Create(prBodyFile)
	if err != nil {
		return "", "", fmt.Errorf("unable to create summary file: %w", err)
	}
	defer prBodyFileContent.Close()

	if !pc.cfg.HasStableBranch() {
		prBodyFileContent.WriteString("\nSee the included CHANGELOG.md for a full list of changes.\n")
	} else {
		scanner := bufio.NewScanner(changesContent)
		for i := 0; scanner.Scan(); i++ {
			// Ignore the first four lines
			if i < 4 {
				continue
			}
			prBodyFileContent.Write(append(scanner.Bytes(), byte('\n')))
		}
	}

	return prTitle, prBodyFileName, err
}
