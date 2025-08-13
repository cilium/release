// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package checklist

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v62/github"
	"golang.org/x/mod/semver"

	templates "github.com/cilium/release/.github/templates"
)

var (
	orderedSubstitutions = []string{
		"X.Y.Z-rc.W",
		"X.Y.Z-pre.N",
		"X.Y.Z",
		"X.Y.W",
		"X.Y-1",
		"X.Y+1",
		"X.Y",
	}
)

func fetchTemplate(cfg ChecklistConfig) ([]byte, error) {
	if len(cfg.TemplatePath) > 0 {
		return os.ReadFile(cfg.TemplatePath)
	}

	prerelease := semver.Prerelease(cfg.TargetVer)
	if strings.HasPrefix(prerelease, "-pre") {
		return templates.ReleaseTemplatePre, nil
	} else if strings.HasPrefix(prerelease, "-rc") {
		return templates.ReleaseTemplateRC, nil
	} else if len(prerelease) == 0 {
		canonical := semver.Canonical(cfg.TargetVer)
		ver := strings.Split(canonical, "-")
		if strings.HasSuffix(ver[0], ".0") {
			return templates.ReleaseTemplateMinor, nil
		} else if len(ver[0]) > 0 {
			return templates.ReleaseTemplatePatch, nil
		}
	}

	return nil, fmt.Errorf("No template configuration found. Specify '--template'?")
}

func assembleVersionSubstitutions(version string) ([]string, error) {
	var patchVer string

	versionSub := make(map[string]string)

	if semver.Prerelease(version) == "" {
		versionSub["X.Y.Z"] = version
		patchVer = version
	} else {
		versionSub["X.Y.Z-rc.W"] = version
		versionSub["X.Y.Z-pre.N"] = version
		patchVer, _, _ = strings.Cut(version, "-")
		versionSub["X.Y.Z"] = patchVer
	}

	splits := strings.Split(patchVer, ".")
	if len(splits) != 3 {
		return nil, fmt.Errorf("unexpected version string %s", version)
	}
	minor, err := strconv.Atoi(splits[1])
	if err != nil {
		return nil, err
	}
	patch, err := strconv.Atoi(splits[2])
	if err != nil {
		return nil, err
	}

	versionSub["X.Y.W"] = semver.MajorMinor(version) + "." + strconv.Itoa(patch+1)
	versionSub["X.Y-1"] = semver.Major(version) + "." + strconv.Itoa(minor-1)
	versionSub["X.Y+1"] = semver.Major(version) + "." + strconv.Itoa(minor+1)
	versionSub["X.Y"] = semver.MajorMinor(version)

	// Now we have a nice map of keys -> values, convert into a list of
	// key,value pairs that will be respected by strings.NewReplacer().
	result := make([]string, 0, len(orderedSubstitutions)*2)
	for _, p := range orderedSubstitutions {
		version = versionSub[p]
		result = append(result, p)
		result = append(result, strings.TrimPrefix(version, "v"))
	}

	return result, nil
}

func prepareChecklist(tmpl []byte, cfg ChecklistConfig) (string, error) {
	patterns, err := assembleVersionSubstitutions(cfg.TargetVer)
	if err != nil {
		return "", fmt.Errorf("Error while parsing version %q: %w", cfg.TargetVer, err)
	}

	// Now, substitute version strings into the template
	replacer := strings.NewReplacer(patterns...)
	checklist := replacer.Replace(string(tmpl))

	return checklist, nil
}

func templateToRequest(tmpl string) (*gh.IssueRequest, error) {
	segments := strings.Split(tmpl, "---")
	if len(segments) < 3 {
		return nil, fmt.Errorf("unable to find metadata, body in issue template, check form of %s", cfg.TemplatePath)
	}
	meta := segments[1]
	body := strings.Join(segments[2:], "---")

	titleRe := regexp.MustCompile(`title: '(.*)'\n`)
	match := titleRe.FindStringSubmatch(meta)
	if len(match) < 2 {
		return nil, fmt.Errorf("unable to find title in issue metadata:\n%s", meta)
	}
	title := string(match[1])

	labelsRe := regexp.MustCompile(`labels: (.*)\n`)
	match = labelsRe.FindStringSubmatch(meta)
	if len(match) < 2 {
		return nil, fmt.Errorf("unable to find labels in issue metadata:\n%s", meta)
	}
	labels := strings.Split(match[1], ",")

	return &gh.IssueRequest{
		Title:  &title,
		Body:   &body,
		Labels: &labels,
	}, nil
}
