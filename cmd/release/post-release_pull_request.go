// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"io"
	"os"
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