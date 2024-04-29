// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"io"
	"os"

	io2 "github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
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

func (pc *PushPullRequest) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *gh.Client) error {
	io2.Fprintf(1, os.Stdout, "📤 Submitting changes to a PR\n")

	o, err := execCommand(pc.cfg.RepoDirectory, "contrib/release/submit-release.sh")
	if err != nil {
		return err
	}
	output, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	io2.Fprintf(2, os.Stdout, "%s\n", string(output))

	return nil
}

func (pc *PushPullRequest) Revert(ctx context.Context, dryRun bool, ghClient *gh.Client) error {
	return fmt.Errorf("Not implemented")
}
