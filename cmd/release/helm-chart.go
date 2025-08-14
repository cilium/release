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
	github2 "github.com/google/go-github/v62/github"
)

type HelmChart struct {
	cfg *ReleaseConfig
}

func NewHelmChart(cfg *ReleaseConfig) *HelmChart {
	return &HelmChart{
		cfg: cfg,
	}
}

func (pc *HelmChart) Name() string {
	return "helm chart"
}

func (pc *HelmChart) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *GHClient) error {
	io2.Fprintf(1, os.Stdout, "☸️ Generating helm charts\n")

	_, err := execCommand(pc.cfg.HelmRepoDirectory, "git", "diff", "--quiet", "HEAD")
	if err != nil {
		return fmt.Errorf("the git repository %q contains uncommitted files, stash them before continuing", pc.cfg.HelmRepoDirectory)
	}

	// Fetch remote branch
	io2.Fprintf(2, os.Stdout, "⬇️ Fetching helm chart\n")
	// Default to "owner" if we can't get the user from gh api
	userRemote := pc.cfg.Owner

	user, err := execCommand(pc.cfg.HelmRepoDirectory, "gh", "api", "user", "--jq", ".login")
	if err == nil {
		userRaw, err := io.ReadAll(user)
		if err != nil {
			return err
		}
		userRemote = strings.TrimSpace(string(userRaw))
	} else {
		io2.Fprintf(3, os.Stdout, "⚠️ Unable to get GH user, falling back to %q\n", userRemote)
	}

	remoteName, err := getRemote(pc.cfg.HelmRepoDirectory, userRemote, "charts")
	if err != nil {
		return err
	}

	// Fetch remote branch
	io2.Fprintf(2, os.Stdout, "⬇️ Fetching Cilium\n")

	helmRepoFullPath := pc.cfg.HelmRepoDirectory
	if pc.cfg.HelmRepoDirectory == "../charts" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		helmRepoFullPath = filepath.Join(wd, pc.cfg.HelmRepoDirectory, "generate_helm_release.sh")
	} else {
		helmRepoFullPath = filepath.Join(pc.cfg.HelmRepoDirectory, "generate_helm_release.sh")
	}
	_, err = execCommand(pc.cfg.HelmRepoDirectory, helmRepoFullPath, "cilium", pc.cfg.TargetVer)
	if err != nil {
		return err
	}

	localBranch := fmt.Sprintf("pr/prepare-%s", pc.cfg.TargetVer)

	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
	} else {
		err := io2.ContinuePrompt(
			fmt.Sprintf("Push chart for %q to branch %q and create PR?", pc.cfg.TargetVer, localBranch),
			"Stopping release preparation.",
		)
		if err != nil {
			return err
		}
	}

	io2.Fprintf(2, os.Stdout, "📤 Pushing branch %q to remote %q\n", localBranch, remoteName)

	// Push to the PR branch
	_, err = execCommand(pc.cfg.HelmRepoDirectory, "git", "push", "-f", remoteName, "HEAD:refs/heads/"+localBranch)
	if err != nil {
		return err
	}

	// Check if PR already exists for this branch
	// TODO: add flag to specify the chart repository
	defaultBranch, err := ghClient.getDefaultBranch(ctx, cfg.Owner, "charts")
	if err != nil {
		return err
	}
	prs, _, err := ghClient.ghClient.PullRequests.List(ctx, pc.cfg.Owner, "charts", &github2.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", userRemote, localBranch),
		Base:  defaultBranch,
	})
	if err != nil {
		return err
	}

	if len(prs) > 0 {
		io2.Fprintf(2, os.Stdout, "📤 Pull request is already open: %s\n", prs[0].GetHTMLURL())
	} else {
		io2.Fprintf(2, os.Stdout, "📤 Creating PR for helm chart...\n")
		prTitle := fmt.Sprintf("Prepare helm chart for release %s", pc.cfg.TargetVer)
		prBody := fmt.Sprintf("Automated helm chart update for Cilium release %s", pc.cfg.TargetVer)

		labels := []string{"kind/release"}

		_, err = execCommand(pc.cfg.HelmRepoDirectory,
			"gh",
			"pr",
			"create",
			"--base",
			defaultBranch,
			"--head",
			fmt.Sprintf("%s:%s", userRemote, localBranch),
			"--label", strings.Join(labels, ","),
			"--body", prBody,
			"--title", prTitle)
		if err != nil {
			return err
		}
	}

	io2.Fprintf(2, os.Stdout, "✅ Changes pushed to helm chart repository.\n")
	io2.Fprintf(2, os.Stdout, "⚠️ Don't forget to manually check if the workflow was successful!\n")
	io2.Fprintf(2, os.Stdout, " - https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml?query=branch%%3Amaster\n")

	return nil
}
