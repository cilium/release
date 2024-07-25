// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	io2 "github.com/cilium/release/pkg/io"
)

type TagCommit struct {
	cfg *ReleaseConfig
}

func NewTagCommit(cfg *ReleaseConfig) *TagCommit {
	return &TagCommit{
		cfg: cfg,
	}
}

func (pc *TagCommit) Name() string {
	return "tagging release commit"
}

func (pc *TagCommit) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *GHClient) error {
	var dryRunStrPrefix string
	if dryRun {
		dryRunStrPrefix = "[🙅 🙅 DRY RUN - OPERATION WILL NOT BE DONE 🙅 🙅] "
	}

	io2.Fprintf(1, os.Stdout, "📤 Tagging a release\n")

	// Fetch remote branch
	io2.Fprintf(2, os.Stdout, "⬇️ Fetching branch\n")
	remoteName, err := getRemote(pc.cfg.RepoDirectory, pc.cfg.Owner, pc.cfg.Repo)
	if err != nil {
		return err
	}

	_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "fetch", "-q", remoteName)
	if err != nil {
		return err
	}

	// Find release commit in the remote branch
	branch := pc.cfg.RemoteBranchName
	if !pc.cfg.HasStableBranch() {
		branch = pc.cfg.DefaultBranch
	}
	remoteBranch := fmt.Sprintf("%s/%s", remoteName, branch)

	commitTitle := fmt.Sprintf("^Prepare for release %s$", pc.cfg.TargetVer)
	o, err := execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "log", "--format=%H", "--grep", commitTitle, remoteBranch)
	if err != nil {
		return err
	}
	commitShaRaw, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	commitSha := strings.TrimSpace(string(commitShaRaw))
	if len(commitSha) == 0 {
		return fmt.Errorf("commit not merged into branch %s. Refusing to tag release", remoteBranch)
	}

	_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "checkout", commitSha)
	if err != nil {
		return err
	}

	o, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "log", "-1", commitSha)
	if err != nil {
		return err
	}
	commitLog, err := io.ReadAll(o)
	if err != nil {
		return err
	}

	io2.Fprintf(3, os.Stdout, "Current HEAD is: %s", commitLog)

	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
	} else {
		err := io2.ContinuePrompt(
			fmt.Sprintf("Create git tags for %s with this commit?", pc.cfg.TargetVer),
			"Stopping release preparation.",
		)
		if err != nil {
			return err
		}
	}

	ersion := strings.TrimPrefix(pc.cfg.TargetVer, "v")
	_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "tag", "-a", ersion, "-s", "-m", "Release "+pc.cfg.TargetVer)
	if err != nil {
		return err
	}
	_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "tag", "-a", pc.cfg.TargetVer, "-s", "-m", "Release "+pc.cfg.TargetVer)
	if err != nil {
		return err
	}

	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
	} else {
		err := io2.ContinuePrompt(
			fmt.Sprintf("%sPush tags %q and %q to %s?", dryRunStrPrefix, pc.cfg.TargetVer, ersion, remoteName),
			"Stopping release preparation.",
		)
		if err != nil {
			return err
		}
	}

	if !dryRun {
		_, err = execCommand(pc.cfg.DryRun, pc.cfg.RepoDirectory, "git", "push", remoteName, ersion, pc.cfg.TargetVer)
		if err != nil {
			return err
		}
	}

	return nil
}

func (pc *TagCommit) commitInUpstream(ctx context.Context, commitSha, branch string) (bool, error) {
	o, err := pipeCommands(ctx, false, pc.cfg.RepoDirectory,
		"git", []string{"branch", "-q", "-r", "--contains", commitSha, branch, "2>/dev/null"},
		"grep", []string{"-q", ".*" + branch},
	)
	if err != nil {
		return false, err
	}
	commitShaRaw, err := io.ReadAll(o)
	if err != nil {
		return false, err
	}
	io2.Fprintf(3, os.Stdout, "%s\n", commitShaRaw)
	return true, nil
}
