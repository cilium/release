// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package projects

import (
	"fmt"

	"github.com/cilium/release/pkg/types"
)

var cfg ProjectsConfig

type ProjectsConfig struct {
	types.CommonConfig

	CurrVer string
	NextVer string

	// ForceMovePending lets "pending" backports be moved from one project
	// to another. By default this is set to false, since most commonly
	// this is a mistake and the PR should have been previously marked as
	// "backport-done".
	ForceMovePending bool
}

func (cfg *ProjectsConfig) Sanitize() error {
	if err := cfg.CommonConfig.Sanitize(); err != nil {
		return err
	}

	if len(cfg.CurrVer) == 0 {
		return fmt.Errorf("--current-version must be specified\n")
	}

	return nil
}
