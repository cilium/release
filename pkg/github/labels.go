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
	"strconv"
	"strings"

	gh "github.com/google/go-github/v50/github"
)

const (
	releaseNoteBlock = "```release-note"
	upstreamPRsBlock = "```upstream-prs"
	commentTag       = "<!--"
)

func textBlockBetween(body, str string) string {
	lines := strings.Split(body, "\n")
	beginning, end := -1, -1
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == str {
			beginning = idx
		}
		if beginning != -1 && line == "```" {
			end = idx
			break
		}
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
	// typical block contains something among these lines:
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
