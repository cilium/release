// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

// Assets provides access to some top-level files into the tree.
package templates

import (
	_ "embed"
)

var (
	//go:embed release_template_minor.md
	ReleaseTemplateMinor []byte

	//go:embed release_template_patch.md
	ReleaseTemplatePatch []byte

	//go:embed release_template_pre_main.md
	ReleaseTemplatePre []byte

	//go:embed release_template_rc_branch.md
	ReleaseTemplateRC []byte
)
