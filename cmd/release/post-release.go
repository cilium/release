// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	io2 "github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
)

type PostRelease struct {
	cfg *ReleaseConfig
}

func NewPostRelease(cfg *ReleaseConfig) *PostRelease {
	return &PostRelease{
		cfg: cfg,
	}
}

func (pc *PostRelease) Name() string {
	return "post release step"
}

func (pc *PostRelease) Run(ctx context.Context, _, _ bool, ghClient *gh.Client) error {
	io2.Fprintf(1, os.Stdout, "📤 Fetching image digests and updating helm charts\n")

	buildURL := getWFRunForTag(ctx, ghClient, pc.cfg.Owner, pc.cfg.Repo, "build-images-releases.yaml", pc.cfg.TargetVer)
	if buildURL == "" {
		return fmt.Errorf("unable to find GitHub workflow run for %s", pc.cfg.TargetVer)
	}

	// Fetch remote branch
	io2.Fprintf(2, os.Stdout, "⬇️ Fetching branch\n")
	remoteName, err := getRemote(pc.cfg.RepoDirectory, pc.cfg.Owner, pc.cfg.Repo)
	if err != nil {
		return err
	}

	branch := pc.cfg.RemoteBranchName
	if !pc.cfg.HasStableBranch() {
		branch, err = getDefaultBranch(ctx, ghClient, pc.cfg.Owner, pc.cfg.Repo)
		if err != nil {
			return err
		}
	}

	localBranch := fmt.Sprintf("pr/%s-digests", pc.cfg.TargetVer)
	remoteBranch := fmt.Sprintf("%s/%s", remoteName, branch)

	// Pull docker manifests from RUN URL
	_, err = execCommand(pc.cfg.RepoDirectory, "git", "fetch", "-q", remoteName)
	if err != nil {
		return err
	}

	_, err = execCommand(pc.cfg.RepoDirectory, "git", "checkout", "-b", localBranch, remoteBranch)
	if err != nil {
		return err
	}

	// Pull docker manifests from RUN URL
	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch. Checking out to commit with release...\n")
		commitTitle := fmt.Sprintf("^Prepare for release %s$", pc.cfg.TargetVer)
		o, err := execCommand(pc.cfg.RepoDirectory, "git", "log", "--format=%H", "--grep", commitTitle, remoteBranch)
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

		_, err = execCommand(pc.cfg.RepoDirectory, "git", "checkout", commitSha)
		if err != nil {
			return err
		}
	}

	io2.Fprintf(2, os.Stdout, "⬇️ Fetching docker digests from workflow run\n")
	_, err = execCommand(pc.cfg.RepoDirectory, "../release/internal/pull-docker-manifests.sh", buildURL, pc.cfg.TargetVer)
	if err != nil {
		return err
	}

	io2.Fprintf(2, os.Stdout, "✍️ Updating helm values with image digests\n")
	_, err = execCommand(pc.cfg.RepoDirectory, "make", "-C", "Documentation", "update-helm-values")
	if err != nil {
		return err
	}

	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "✍️ Helm values successfully generated. Since there is not a stable branch, the files will not"+
			" be committed as they are only intended to be used on helm charts repository.\n")
		return nil
	}

	// Commit all changes
	io2.Fprintf(2, os.Stdout, "Committing files\n")
	commitFiles := []string{
		"Documentation/helm-values.rst",
		"install/kubernetes/Makefile.digests",
		"install/kubernetes/cilium/README.md",
		"install/kubernetes/cilium/values.yaml",
	}
	_, err = execCommand(pc.cfg.RepoDirectory, "git", append([]string{"add"}, commitFiles...)...)
	if err != nil {
		return err
	}

	digestFileName := fmt.Sprintf("digest-%s.txt", pc.cfg.TargetVer)
	digestFile := filepath.Join(pc.cfg.RepoDirectory, digestFileName)
	digests, err := os.ReadFile(digestFile)
	if err != nil {
		return fmt.Errorf("unable to read digest file %q: %w", digestFile, err)
	}
	commitMsg := fmt.Sprintf("install: Update image digests for %s\n\n"+
		"Generated from %s\n"+
		string(digests), pc.cfg.TargetVer, buildURL)
	_, err = execCommand(pc.cfg.RepoDirectory, "git", "commit", "-sm", commitMsg)

	return err
}

func (pc *PostRelease) commitInUpstream(ctx context.Context, commitSha, branch string) (bool, error) {
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

func (pc *PostRelease) Revert(ctx context.Context, dryRun bool, ghClient *gh.Client) error {
	return fmt.Errorf("Not implemented")
}
