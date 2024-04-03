// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package types

import (
	"fmt"
	"strings"
)

type CommonConfig struct {
	RepoName string
	Owner    string
	Repo     string
}

func (cfg *CommonConfig) Sanitize() error {
	ownerRepo := strings.Split(cfg.RepoName, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("Invalid repo name: %s\n", cfg.RepoName)
	}
	cfg.Owner = ownerRepo[0]
	cfg.Repo = ownerRepo[1]
	return nil
}
