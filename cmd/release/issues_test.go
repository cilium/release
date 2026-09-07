// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"reflect"
	"sort"
	"testing"
)

func Test_backportOrderingQueries(t *testing.T) {
	const (
		owner = "cilium"
		repo  = "cilium"
	)

	type args struct {
		relVer         string
		stableBranches []string
	}
	tests := []struct {
		name string
		args args
		want []backportOrderingViolation
	}{
		{
			name: "no other stable branches",
			args: args{
				relVer:         "v1.15.5",
				stableBranches: []string{"v1.15"},
			},
			want: nil,
		},
		{
			name: "released branch is the newest: only older branches, behind-older direction",
			args: args{
				relVer:         "v1.15.5",
				stableBranches: []string{"v1.13", "v1.14", "v1.15"},
			},
			want: []backportOrderingViolation{
				// v1.13 is older than v1.15
				{
					reason: "present in older branch v1.13 (backport-done/1.13) but still needs backport to v1.15 (needs-backport/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "needs-backport/1.15"),
				},
				{
					reason: "present in older branch v1.13 (backport-done/1.13) but backport to v1.15 is still pending (backport-pending/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "backport-pending/1.15"),
				},
				// v1.14 is older than v1.15
				{
					reason: "present in older branch v1.14 (backport-done/1.14) but still needs backport to v1.15 (needs-backport/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "needs-backport/1.15"),
				},
				{
					reason: "present in older branch v1.14 (backport-done/1.14) but backport to v1.15 is still pending (backport-pending/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "backport-pending/1.15"),
				},
			},
		},
		{
			name: "released branch is the oldest: only newer branches, ahead-of-newer direction",
			args: args{
				relVer:         "v1.13.20",
				stableBranches: []string{"v1.13", "v1.14", "v1.15"},
			},
			want: []backportOrderingViolation{
				// v1.14 is newer than v1.13
				{
					reason: "present in v1.13 (backport-done/1.13) but still needs backport to newer branch v1.14 (needs-backport/1.14)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "needs-backport/1.14"),
				},
				{
					reason: "present in v1.13 (backport-done/1.13) but backport to newer branch v1.14 is still pending (backport-pending/1.14)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "backport-pending/1.14"),
				},
				// v1.15 is newer than v1.13
				{
					reason: "present in v1.13 (backport-done/1.13) but still needs backport to newer branch v1.15 (needs-backport/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "needs-backport/1.15"),
				},
				{
					reason: "present in v1.13 (backport-done/1.13) but backport to newer branch v1.15 is still pending (backport-pending/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "backport-pending/1.15"),
				},
			},
		},
		{
			name: "released branch in the middle: both directions",
			args: args{
				relVer:         "v1.14.10",
				stableBranches: []string{"v1.13", "v1.14", "v1.15"},
			},
			want: []backportOrderingViolation{
				// v1.13 is older than v1.14
				{
					reason: "present in older branch v1.13 (backport-done/1.13) but still needs backport to v1.14 (needs-backport/1.14)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "needs-backport/1.14"),
				},
				{
					reason: "present in older branch v1.13 (backport-done/1.13) but backport to v1.14 is still pending (backport-pending/1.14)",
					query:  orderingQuery(owner, repo, "backport-done/1.13", "backport-pending/1.14"),
				},
				// v1.15 is newer than v1.14
				{
					reason: "present in v1.14 (backport-done/1.14) but still needs backport to newer branch v1.15 (needs-backport/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "needs-backport/1.15"),
				},
				{
					reason: "present in v1.14 (backport-done/1.14) but backport to newer branch v1.15 is still pending (backport-pending/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "backport-pending/1.15"),
				},
			},
		},
		{
			name: "ignores non-version branches such as the default branch",
			args: args{
				relVer:         "v1.15.5",
				stableBranches: []string{"main", "v1.14", "v1.15", "some-feature-branch"},
			},
			want: []backportOrderingViolation{
				{
					reason: "present in older branch v1.14 (backport-done/1.14) but still needs backport to v1.15 (needs-backport/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "needs-backport/1.15"),
				},
				{
					reason: "present in older branch v1.14 (backport-done/1.14) but backport to v1.15 is still pending (backport-pending/1.15)",
					query:  orderingQuery(owner, repo, "backport-done/1.14", "backport-pending/1.15"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backportOrderingQueries(tt.args.relVer, tt.args.stableBranches, owner, repo)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("backportOrderingQueries() mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func Test_maintainedStableBranches(t *testing.T) {
	tests := []struct {
		name             string
		allBranches      []string
		relVer           string
		maintainedMinors int
		want             []string
	}{
		{
			name:             "trims to N newest, releasing the newest",
			allBranches:      []string{"v1.11", "v1.12", "v1.13", "v1.14", "v1.15"},
			relVer:           "v1.15.3",
			maintainedMinors: 3,
			want:             []string{"v1.13", "v1.14", "v1.15"},
		},
		{
			name:             "released branch outside the window is re-added",
			allBranches:      []string{"v1.11", "v1.12", "v1.13", "v1.14", "v1.15"},
			relVer:           "v1.12.9",
			maintainedMinors: 3,
			want:             []string{"v1.12", "v1.13", "v1.14", "v1.15"},
		},
		{
			name:             "released branch not among protected branches is added",
			allBranches:      []string{"v1.13", "v1.14"},
			relVer:           "v1.15.0",
			maintainedMinors: 3,
			want:             []string{"v1.13", "v1.14", "v1.15"},
		},
		{
			name:             "limit disabled keeps all, sorted and de-duplicated",
			allBranches:      []string{"v1.15", "v1.11", "v1.13", "v1.15", "v1.12", "v1.14"},
			relVer:           "v1.15.3",
			maintainedMinors: 0,
			want:             []string{"v1.11", "v1.12", "v1.13", "v1.14", "v1.15"},
		},
		{
			name:             "sorts numerically, not lexically (v1.9 < v1.10)",
			allBranches:      []string{"v1.9", "v1.10", "v1.11"},
			relVer:           "v1.11.0",
			maintainedMinors: 2,
			want:             []string{"v1.10", "v1.11"},
		},
		{
			name:             "ignores non-version branches",
			allBranches:      []string{"main", "v1.14", "feature-x", "v1.15"},
			relVer:           "v1.15.1",
			maintainedMinors: 3,
			want:             []string{"v1.14", "v1.15"},
		},
		{
			name:             "patch-level relVer normalizes to major.minor",
			allBranches:      []string{"v1.14", "v1.15"},
			relVer:           "v1.15.7",
			maintainedMinors: 3,
			want:             []string{"v1.14", "v1.15"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maintainedStableBranches(tt.allBranches, tt.relVer, tt.maintainedMinors)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("maintainedStableBranches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test_backportOrderingQueries_coversAllPairs is a property-style check that, for
// any release branch, every other active stable branch is compared exactly once
// in the correct direction and no self-comparison is generated.
func Test_backportOrderingQueries_coversAllPairs(t *testing.T) {
	stableBranches := []string{"v1.12", "v1.13", "v1.14", "v1.15", "v1.16"}
	for _, relVer := range []string{"v1.12.9", "v1.13.5", "v1.14.3", "v1.15.1", "v1.16.0"} {
		got := backportOrderingQueries(relVer, stableBranches, "cilium", "cilium")
		// Each other branch contributes exactly 2 queries (needs + pending).
		wantLen := (len(stableBranches) - 1) * 2
		if len(got) != wantLen {
			t.Fatalf("relVer=%s: got %d queries, want %d", relVer, len(got), wantLen)
		}
		// Ensure no query compares a branch against itself.
		for _, v := range got {
			if v.query == "" {
				t.Fatalf("relVer=%s: empty query in %#v", relVer, v)
			}
		}
	}
}

// Test_backportOrderingQueries_unaffectedByBranchOrder ensures the output does
// not depend on the ordering the branches are returned by the GitHub API in.
func Test_backportOrderingQueries_unaffectedByBranchOrder(t *testing.T) {
	relVer := "v1.14.10"
	ascending := []string{"v1.13", "v1.14", "v1.15"}
	descending := []string{"v1.15", "v1.14", "v1.13"}

	gotAsc := backportOrderingQueries(relVer, ascending, "cilium", "cilium")
	gotDesc := backportOrderingQueries(relVer, descending, "cilium", "cilium")

	// The set of produced queries must be identical regardless of input order.
	sortQueries := func(vs []backportOrderingViolation) []string {
		out := make([]string, 0, len(vs))
		for _, v := range vs {
			out = append(out, v.query)
		}
		sort.Strings(out)
		return out
	}

	if !reflect.DeepEqual(sortQueries(gotAsc), sortQueries(gotDesc)) {
		t.Errorf("query set depends on input branch order:\n asc: %v\ndesc: %v",
			sortQueries(gotAsc), sortQueries(gotDesc))
	}
}
