// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	io2 "github.com/cilium/release/pkg/io"
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
	// TODO create a flag for charts repo?
	chartRemoteName, err := getRemote(pc.cfg.HelmRepoDirectory, pc.cfg.Owner, "charts")
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
	_, err = execCommand(pc.cfg.HelmRepoDirectory, helmRepoFullPath, pc.cfg.RepoDirectory, pc.cfg.TargetVer)
	if err != nil {
		return err
	}

	if yesToPrompt {
		fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
	} else {
		err := io2.ContinuePrompt(
			fmt.Sprintf("Push chart for %q to %q?", pc.cfg.TargetVer, chartRemoteName),
			"Stopping release preparation.",
		)
		if err != nil {
			return err
		}
	}

	_, err = execCommand(pc.cfg.RepoDirectory, "git", "push", chartRemoteName)
	if err != nil {
		return err
	}

	io2.Fprintf(2, os.Stdout, "✅ Changes pushed to helm chart repository.\n")
	io2.Fprintf(2, os.Stdout, "⚠️ Don't forget to manually check if the workflow was successful!\n")
	io2.Fprintf(2, os.Stdout, " - https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml?query=branch%%3Amaster\n")

	return nil
}
