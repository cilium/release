// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type Permission int

const (
	PermissionNone        = 0
	PermissionRead        = 1
	PermissionPullRequest = 2
	PermissionWrite       = 4
)

type Location int

const (
	LocationLocalDisk = iota
	LocationQuayIO
	LocationGitHubUpstream
	LocationGitHubFork
	LocationGitHubProjects
	LocationGitHubHelmChart
)

var locationNames = []string{
	LocationLocalDisk:       "Local Disk",
	LocationQuayIO:          "quay.io",
	LocationGitHubUpstream:  "GitHub - Upstream",
	LocationGitHubFork:      "GitHub - Fork",
	LocationGitHubProjects:  "GitHub - Org Projects",
	LocationGitHubHelmChart: "GitHub - Helm Chart Repo",
}

type PermissionLocation struct {
	permission Permission
	location   Location
}

func permissionString(p Permission) string {
	if p == PermissionNone {
		return "-"
	}
	var perm strings.Builder
	if p&PermissionRead != 0 {
		perm.WriteString("[R] ")
	}
	if p&PermissionWrite != 0 {
		perm.WriteString("[W] ")
	}
	if p&PermissionPullRequest != 0 {
		perm.WriteString("[PR]")
	}
	return perm.String()
}

func PrintPermTable(writer io.Writer) {
	fmt.Fprint(writer, "\n[R] - Read\n")
	fmt.Fprint(writer, "[W] - Write\n")
	fmt.Fprint(writer, "[PR] - Pull Request\n\n")
	w := tabwriter.NewWriter(writer, 0, 0, 1, ' ', 0)

	headers := append([]string{"Step"}, locationNames...)
	header := strings.Join(headers, "\t") + "\t\n"
	fmt.Fprint(w, header)
	for _, loc := range headers {
		for i := 0; i < len(loc); i++ {
			fmt.Fprint(w, "-")
		}
		fmt.Fprint(w, "\t")
	}
	fmt.Fprint(w, "\n")

	for _, group := range groups {
		row := fmt.Sprintf("%s\t", group.name)
		for location := LocationLocalDisk; location <= LocationGitHubHelmChart; location++ {
			perm := permissionString(group.permissions[Location(location)])
			row += fmt.Sprintf("%s\t", perm)
		}
		fmt.Fprint(w, row+"\n")
	}

	w.Flush()
}
