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
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"

	blang_semver "github.com/blang/semver/v4"
	gh "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"
)

const (
	releaseNoteBlock = "```release-note"
	upstreamPRsBlock = "```upstream-prs"
	commentTag       = "<!--"
	endBlock         = "```"
)

// Get the text between startBlock and endBlock
func textBlockBetween(body, startBlock string) string {
	// Use a bufio.Scanner to scan line by line because it handles both `\n` and `\r\n`.
	scanner := bufio.NewScanner(strings.NewReader(body))
	beginning, end := -1, -1
	idx := 0
	gotBlock := false
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		lines = append(lines, line)
		if line == startBlock {
			beginning = idx
		}
		if !gotBlock && beginning != -1 && line == endBlock {
			end = idx
			// Got the block, but continue scanning so we can accumulate all the lines
			gotBlock = true
		}
		if scanner.Err() != nil {
			return ""
		}
		idx++
	}

	if beginning == end {
		return ""
	}
	if end == -1 {
		end = len(lines)
	}

	return strings.TrimSpace(strings.Join(lines[beginning+1:end], " "))
}

func getUpstreamPRs(body string) []int {
	if !strings.Contains(body, upstreamPRsBlock) {
		return nil
	}
	block := textBlockBetween(body, upstreamPRsBlock)
	if len(block) == 0 {
		return nil
	}

	// Look for substrings that should be present in a Backport PR body in v1 format
	if strings.Contains(block, "for pr in") || strings.Contains(block, "contrib/backporting/set-labels.py") {
		return getUpstreamPRsV1(body, block)
	}

	// otherwise assume that the body is in v2 format
	return getUpstreamPRsV2(block)
}

func getUpstreamPRsV1(body, block string) []int {
	// v1 of a Backport PR body should follow this format:
	//
	// for pr in 9959 9982 10005; do contrib/backporting/set-labels.py $pr done 1.6; done
	if !strings.Contains(body, "for pr in") {
		return nil
	}
	// blocks may contain a prompt symbol before the "for" loop
	block = strings.TrimPrefix(block, "$ ")
	block = strings.TrimPrefix(block, "for pr in")
	bashLines := strings.Split(block, ";")
	if len(bashLines) < 1 {
		return nil
	}
	strNumbers := strings.Split(bashLines[0], " ")
	var prNumbers []int
	for _, strNumber := range strNumbers {
		prNumber, err := strconv.Atoi(strNumber)
		if err != nil {
			continue
		}
		prNumbers = append(prNumbers, prNumber)
	}
	return prNumbers
}

func getUpstreamPRsV2(block string) []int {
	// v2 of a Backport PR body should follow this format:
	//
	// 9959 9982 10005
	var prNumbers []int
	for _, strNumber := range strings.Fields(block) {
		prNumber, err := strconv.Atoi(strNumber)
		if err != nil {
			continue
		}
		prNumbers = append(prNumbers, prNumber)
	}
	return prNumbers
}

// getReleaseNote returns the release node if it is present in the given body
// otherwise it will fallback to the title.
func getReleaseNote(title, body string) string {
	if strings.Contains(body, releaseNoteBlock) {
		block := textBlockBetween(body, releaseNoteBlock)
		if len(block) != 0 && !strings.Contains(block, commentTag) {
			return block
		}
	}
	return strings.TrimSpace(title)
}

// getReleaseLabel returns the release label found in the slice of labels.
func getReleaseLabel(lbls []string) string {
	for _, lbl := range lbls {
		if strings.HasPrefix(lbl, "release-note/") {
			return lbl
		}
	}
	return "release-note/none"
}

// getBackportBranches returns a slice of labels that have the prefix
// `backport-done/`
func getBackportBranches(lbls []string) []string {
	var bb []string
	for _, lbl := range lbls {
		if strings.HasPrefix(lbl, "backport-done/") {
			bb = append(bb, lbl)
		}
	}
	return bb
}

// parseGHLabels parses the github labels into
func parseGHLabels(ghLabels []*gh.Label) []string {
	var lbls []string
	for _, prLabel := range ghLabels {
		lbls = append(lbls, prLabel.GetName())
	}
	return lbls
}

const (
	releaseBlockerPrefix = "release-blocker/"
	backportDonePrefix   = "backport-done/"
	backportPrefix       = "backport/"
)

func ReleaseBlockerLabel(version string) string {
	return fmt.Sprintf("%s%s", releaseBlockerPrefix, MajorMinorErsion(version))
}

func BackportDoneLabel(version string) string {
	return fmt.Sprintf("%s%s", backportDonePrefix, MajorMinorErsion(version))
}

func BackportLabel(version string) string {
	return fmt.Sprintf("%s%s", backportPrefix, MajorMinorErsion(version))
}

func MajorMinorErsion(version string) string {
	majorMinorVersion := semver.MajorMinor(version)
	return strings.TrimPrefix(majorMinorVersion, "v")
}

// Custom version type that includes semver.Version
type customVersion struct {
	Version  blang_semver.Version
	Original string
}

func SortTags(tags []string) ([]string, error) {
	versions := make([]customVersion, len(tags))

	for i, tag := range tags {
		if !strings.HasPrefix(tag, "v") {
			continue
		}
		v, err := blang_semver.ParseTolerant(tag[1:])
		if err != nil {
			return nil, fmt.Errorf("failed to parse tag %s: %w", tag, err)
		}
		versions[i] = customVersion{Version: v, Original: tag}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version.LT(versions[j].Version)
	})

	sortedTags := make([]string, len(tags))
	for i, v := range versions {
		sortedTags[i] = v.Original
	}

	return sortedTags, nil
}

func PreviousTagOf(tags []string, tag string) string {
	for i := range tags {
		if tag == tags[i] {
			if i == 0 {
				return ""
			}
			return tags[i-1]
		}
	}
	return ""
}
