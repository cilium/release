// Copyright 2020 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package persistence

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/cilium/release/pkg/types"
)

func TestStoreState(t *testing.T) {
	name, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Error("Unable to create tmp directory:", err)
	}
	type args struct {
		file        string
		backportPRs types.BackportPRs
		prs         types.PullRequests
		nodeIDs     types.NodeIDs
		shas        []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "existing file but empty",
			args: args{
				file: name.Name(),
				backportPRs: types.BackportPRs{
					1: {
						2: types.PullRequest{
							ReleaseNote:  "BarFoo",
							ReleaseLabel: "release-note/major",
							AuthorName:   "@example",
						},
					},
				},
				prs: types.PullRequests{
					3: {
						ReleaseNote:  "FooBar",
						ReleaseLabel: "release-note/minor",
						AuthorName:   "@example",
						BackportBranches: []string{
							"backport-done/1.5",
						},
					},
				},
				nodeIDs: types.NodeIDs{
					3: "abcdef",
				},
				shas: []string{
					"9ba79ef2517ede0ece6c1d1a7798c57d33d24f77",
					"9ba79ef2517ede0ece6c1d1a7798c57d33d24f72",
					"9ba79ef2517ede0ece6c1d1a7798c57d33d24f71",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := StoreState(tt.args.file, tt.args.backportPRs, tt.args.prs, tt.args.nodeIDs, tt.args.shas); (err != nil) != tt.wantErr {
				t.Errorf("StoreState() error = %v, wantErr %v", err, tt.wantErr)
			}
			backportPRs, prs, nodeIDs, shas, err := LoadState(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(backportPRs, tt.args.backportPRs) {
				t.Errorf("LoadState() backportPRs = %v, want %v", backportPRs, tt.args.backportPRs)
			}
			if !reflect.DeepEqual(prs, tt.args.prs) {
				t.Errorf("LoadState() prs = %v, want %v", prs, tt.args.prs)
			}
			if !reflect.DeepEqual(nodeIDs, tt.args.nodeIDs) {
				t.Errorf("LoadState() nodeIDs = %v, want %v", nodeIDs, tt.args.nodeIDs)
			}
			if !reflect.DeepEqual(shas, tt.args.shas) {
				t.Errorf("LoadState() shas = %v, want %v", shas, tt.args.shas)
			}
		})
	}
}
