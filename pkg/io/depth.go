// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package io

import (
	"fmt"
	"io"
	"strings"
)

func Fprintf(depth int, w io.Writer, format string, a ...any) {
	tabs := strings.Repeat("  ", depth)
	fmt.Fprintf(w, tabs+format, a...)
}
