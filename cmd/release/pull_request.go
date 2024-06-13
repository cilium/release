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
	"time"

	"github.com/cilium/release/pkg/github"
	io2 "github.com/cilium/release/pkg/io"
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
		var err error
		baseBranch, err = ghClient.getDefaultBranch(ctx, pc.cfg.Owner, pc.cfg.Repo)
		if err != nil {
			return err
		}
	}

	user, err := execCommand(pc.cfg.RepoDirectory, "gh", "api", "user", "--jq", ".login")
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

	localBranch := fmt.Sprintf("pr/prepare-%s", pc.cfg.TargetVer)

	io2.Fprintf(2, os.Stdout, "📤 Pushing branch %q to remote %q\n", localBranch, remoteName)

	// Revert the "Prepare for release" commit since that commit will only be
	// used for a tag.
	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch, pushing HEAD^ changes before creating PR\n")
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "push", remoteName, "HEAD^:refs/heads/"+localBranch)
	} else {
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "push", remoteName, localBranch)
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
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "push", remoteName, localBranch)
		if err != nil {
			return err
		}
	}

	labels := []string{"kind/release", "release-note/misc"}
	if pc.cfg.HasStableBranch() {
		labels = append(labels, github.BackportLabel(pc.cfg.TargetVer))
	}

	io2.Fprintf(2, os.Stdout, "📤 Creating PR...\n")
	// Sleep 10 seconds, otherwise we are too fast for github to detect there's
	// a branch already created and use that branch to create the PR.
	time.Sleep(10 * time.Second)
	_, err = execCommand(pc.cfg.RepoDirectory,
		"gh",
		"pr",
		"create",
		"--base",
		baseBranch,
		"--label", strings.Join(labels, ","),
		"--body-file", prBodyFile,
		"--title", prTitle)

	return err
}

func (pc *PushPullRequest) Revert(ctx context.Context, dryRun bool, ghClient *GHClient) error {
	return fmt.Errorf("Not implemented")
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

	prBodyFileContent.WriteString(prTitle + "\n")

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
