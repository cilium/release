// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_assembleVersionSubstitutions(t *testing.T) {
	tests := []struct {
		name  string
		err   string
		input string
		want  []string
	}{
		{
			name:  "Invalid input",
			input: "foo",
			want:  nil,
			err:   "unexpected version string foo",
		},
		{
			name:  "Stable release",
			input: "v1.10.0",
			want: []string{
				"X.Y.Z-rc.W", "",
				"X.Y.Z-pre.N", "",
				"X.Y.Z", "1.10.0",
				"X.Y.W", "1.10.1",
				"X.Y-1", "1.9",
				"X.Y+1", "1.11",
				"X.Y", "1.10",
			},
		},
		{
			name:  "Release candidate",
			input: "v1.10.0-rc.0",
			want: []string{
				"X.Y.Z-rc.W", "1.10.0-rc.0",
				"X.Y.Z-pre.N", "1.10.0-rc.0",
				"X.Y.Z", "1.10.0",
				"X.Y.W", "1.10.1",
				"X.Y-1", "1.9",
				"X.Y+1", "1.11",
				"X.Y", "1.10",
			},
		},
		{
			name:  "Prerelease",
			input: "v1.10.0-pre.0",
			want: []string{
				"X.Y.Z-rc.W", "1.10.0-pre.0",
				"X.Y.Z-pre.N", "1.10.0-pre.0",
				"X.Y.Z", "1.10.0",
				"X.Y.W", "1.10.1",
				"X.Y-1", "1.9",
				"X.Y+1", "1.11",
				"X.Y", "1.10",
			},
		},
	}

	for _, tt := range tests {
		sub, err := assembleVersionSubstitutions(tt.input)
		if tt.err != "" {
			assert.ErrorContains(t, err, tt.err, tt.name+" encountered unexpected error")
		}
		assert.Equal(t, sub, tt.want, tt.name+" generated unexpected substitutions")
	}
}
