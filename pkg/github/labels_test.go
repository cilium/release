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

package github

import (
	"reflect"
	"testing"
)

func Test_getReleaseNote(t *testing.T) {
	type args struct {
		title string
		body  string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "",
			args: args{
				title: "Fooo",
				body: "This PR implements BPF feature probes for the kernel config with bpftool. Based on #8094, but now I'm trying to submit in smaller pieces with hope that CI is not going to complain. Please review per commit.\n" +
					"\n" +
					"```release-note\n" +
					"BPF kernel probes based on bpftool\n" +
					"```\n",
			},
			want: "BPF kernel probes based on bpftool",
		},
		{
			name: "",
			args: args{
				title: "Fooo",
				body: "This PR implements BPF feature probes for the kernel config with bpftool. Based on #8094, but now I'm trying to submit in smaller pieces with hope that CI is not going to complain. Please review per commit.\n" +
					"\n" +
					"```release-note\n" +
					"BPF kernel probes based on bpftool\n" +
					"BPF kernel probes based on bpftool\n" +
					"```\n",
			},
			want: "BPF kernel probes based on bpftool BPF kernel probes based on bpftool",
		},
		{
			name: "",
			args: args{
				title: "Fooo",
				body: "This PR implements BPF feature probes for the kernel config with bpftool. Based on #8094, but now I'm trying to submit in smaller pieces with hope that CI is not going to complain. Please review per commit.\n" +
					"```\n",
			},
			want: "Fooo",
		},
		{
			name: "",
			args: args{
				title: "[v1.6] golang: update to 1.12.15",
				body:  "```release-note\r\ngolang: update to 1.12.15\r\n```\n\n<!-- Reviewable:start -->\n",
			},
			want: "golang: update to 1.12.15",
		},
		{
			name: "",
			args: args{
				title: "Pineapple pizza",
				body:  "```release-note\r\n<!-- Enter the release note text here if needed or remove this section! -->\n",
			},
			want: "Pineapple pizza",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getReleaseNote(tt.args.title, tt.args.body); got != tt.want {
				t.Errorf("getReleaseNote() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_getBackportPRs(t *testing.T) {
	type args struct {
		body string
	}
	testsV1 := []struct {
		name string
		args args
		want []int
	}{
		{
			name: "get all three PRs in v1 block",
			args: args{
				body: "```upstream-prs\r\n$ for pr in 9959 9982 10005; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: []int{
				9959,
				9982,
				10005,
			},
		},
		{
			name: "get all three PRs in v1 block (no prompt symbol)",
			args: args{
				body: "```upstream-prs\r\nfor pr in 9959 9982 10005; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: []int{
				9959,
				9982,
				10005,
			},
		},
		{
			name: "get single PR in v1 block",
			args: args{
				body: "```upstream-prs\r\n$ for pr in 9959 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: []int{
				9959,
			},
		},
		{
			name: "get single PR in v1 block (no prompt symbol)",
			args: args{
				body: "```upstream-prs\r\n$ for pr in 9959 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: []int{
				9959,
			},
		},
		{
			name: "command line pattern missing in v1 block",
			args: args{
				body: "```upstream-prs\r\n$ 9 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: nil,
		},
		{
			name: "command line pattern missing in v1 block (no prompt symbol)",
			args: args{
				body: "```upstream-prs\r\n9 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: nil,
		},
		{
			name: "command line pattern incomplete in v1 block",
			args: args{
				body: "```upstream-prs\r\npr in 99 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\n```",
			},
			want: nil,
		},
		{
			name: "unfinished quote section in v1 block",
			args: args{
				body: "```upstream-prs\n$ for pr in 9959 ; do contrib/backporting/set-labels.py $pr done 1.6; done\r\nfoo\nbar",
			},
			want: []int{
				9959,
			},
		},
		{
			name: "get all three PRs in v2 block",
			args: args{
				body: "```upstream-prs\r\n 9959 9982 10005\r\n```",
			},
			want: []int{
				9959,
				9982,
				10005,
			},
		},
		{
			name: "get single PR in v2 block",
			args: args{
				body: "```upstream-prs\r\n 9959\r\n```",
			},
			want: []int{
				9959,
			},
		},
		{
			name: "empty block in v2 block",
			args: args{
				body: "```upstream-prs\r\n\r\n```",
			},
			want: nil,
		},
		{
			name: "unfinished quote section in v2 block",
			args: args{
				body: "```upstream-prs\n 9959\r\nfoo\nbar",
			},
			want: []int{
				9959,
			},
		},
	}
	for _, tt := range testsV1 {
		t.Run(tt.name, func(t *testing.T) {
			if got := getUpstreamPRs(tt.args.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getUpstreamPRs() = %v, want %v", got, tt.want)
			}
		})
	}
}
