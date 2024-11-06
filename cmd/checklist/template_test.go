// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"os"
	"path/filepath"
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

func Test_prepareChecklist(t *testing.T) {
	cfg := ChecklistConfig{
		TargetVer: "v1.10.0-pre.0",
	}
	testdataPath := filepath.Join("..", "..", "testdata", "checklist")

	paths, err := filepath.Glob(filepath.Join(testdataPath, "*.input"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		_, filename := filepath.Split(path)
		testname := filename[:len(filename)-len(filepath.Ext(path))]

		t.Run(testname, func(t *testing.T) {
			source, err := os.ReadFile(path)
			assert.Nil(t, err, "failed to read input template: ", err)

			output, err := prepareChecklist(source, cfg)
			assert.Nil(t, err, "failed to render checklist: ", err)

			golden := filepath.Join(testdataPath, testname+".golden")
			want, err := os.ReadFile(golden)
			assert.Nil(t, err, "error reading golden output: ", err)

			assert.Equal(t, output, string(want), "template processing did not match golden output. Check for bugs or run 'make generate-golden' to update the golden tests.")
		})
	}
}
