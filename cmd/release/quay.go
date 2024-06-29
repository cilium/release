// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/cilium/release/pkg/docker"
	"github.com/cilium/release/pkg/io"
	"golang.org/x/mod/semver"
)

const quayURL = "https://quay.io/api/v1/repository/%s/%s/manifest/%s/security?vulnerabilities=true"

func NewImageCVE(cfg *ReleaseConfig) Step {
	return &ImageCVEChecker{
		cfg: cfg,
	}
}

type ImageCVEChecker struct {
	cfg *ReleaseConfig
}

func (c *ImageCVEChecker) Name() string {
	return "checking for quay.io image vulnerabilities"
}

func (c *ImageCVEChecker) Run(ctx context.Context, yesToPrompt, _ bool, _ *GHClient) error {
	majorMinor := semver.MajorMinor(c.cfg.TargetVer)
	imageURL := fmt.Sprintf("quay.io/%s/%s", c.cfg.QuayOrg, c.cfg.QuayRepo)

	var imageDigest string
	for {
		var err error
		imageDigest, err = docker.ImageSHA256(imageURL, majorMinor)
		if majorMinor != "latest" && errors.Is(err, docker.ErrImageNotFound) {
			majorMinor = "latest"
			continue
		}
		if err != nil {
			return fmt.Errorf("unable to get digest for %s: %w", imageURL, err)
		}
		break
	}
	humanURL := fmt.Sprintf("https://%s:%s", imageURL, majorMinor)
	io.Fprintf(1, os.Stdout, "👀 Checking current branch image for vulnerabilities in: %s\n", humanURL)

	url := quayImageURL(c.cfg.QuayOrg, c.cfg.QuayRepo, imageDigest)

	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Error making GET request:", err)
	}
	defer resp.Body.Close()

	var vulnInfo ImageVulnerabilitiesInfo
	if err := json.NewDecoder(resp.Body).Decode(&vulnInfo); err != nil {
		log.Fatal("Error decoding JSON response:", err)
	}

	var w *tabwriter.Writer
	tableRows := getFixedVulnerabilities(vulnInfo)
	if len(tableRows) != 0 {
		io.Fprintf(2, os.Stdout, "☢️ Image contains vulnerabilities that are fixable:\n")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
		io.Fprintf(3, w, "ADVISORY\tSEVERITY\tPACKAGE\tCURRENT VERSION\tFIXED IN VERSION\n")

		for _, row := range tableRows {
			io.Fprintf(3, w, "%s\t%s\t%s\t%s\t%s\n", row.Advisory, row.Severity, row.Package, row.CurrentVersion, row.FixedInVersion)
		}
		w.Flush()
		if yesToPrompt {
			fmt.Printf("⏩ Skipping prompts, continuing with the release process.\n")
		} else {
			err := io.ContinuePrompt(
				"☢️ Image contains vulnerabilities. Do you want to continue the release process?",
				"✋ Vulnerabilities found, stopping the release process",
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getFixedVulnerabilities(data ImageVulnerabilitiesInfo) []tableRow {
	var rows []tableRow
	for _, feature := range data.Data.Layer.Features {
		for _, vuln := range feature.Vulnerabilities {
			if vuln.FixedBy != "" {
				rows = append(rows, tableRow{
					Advisory:       vuln.Name,
					Severity:       vuln.Severity,
					Package:        feature.Name,
					CurrentVersion: feature.Version,
					FixedInVersion: vuln.FixedBy,
				})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Severity < rows[j].Severity
	})
	return rows
}

func (c *ImageCVEChecker) Revert(ctx context.Context, dryRun bool, ghClient *GHClient) error {
	return nil
}

func quayImageURL(org, imageName, sha256sum string) string {
	return fmt.Sprintf(
		quayURL,
		org,
		imageName,
		sha256sum,
	)
}

type tableRow struct {
	Advisory       string
	Severity       string
	Package        string
	CurrentVersion string
	FixedInVersion string
}

type ImageVulnerabilitiesInfo struct {
	Status string `json:"status"`
	Data   struct {
		Layer struct {
			Name             string `json:"Name"`
			ParentName       string `json:"ParentName"`
			NamespaceName    string `json:"NamespaceName"`
			IndexedByVersion int    `json:"IndexedByVersion"`
			Features         []struct {
				Name            string        `json:"Name"`
				VersionFormat   string        `json:"VersionFormat"`
				NamespaceName   string        `json:"NamespaceName"`
				AddedBy         string        `json:"AddedBy"`
				Version         string        `json:"Version"`
				BaseScores      []interface{} `json:"BaseScores"`
				CVEIds          []interface{} `json:"CVEIds"`
				Vulnerabilities []struct {
					Severity      string `json:"Severity"`
					NamespaceName string `json:"NamespaceName"`
					Link          string `json:"Link"`
					FixedBy       string `json:"FixedBy"`
					Description   string `json:"Description"`
					Name          string `json:"Name"`
					Metadata      struct {
						UpdatedBy     string `json:"UpdatedBy"`
						RepoName      string `json:"RepoName"`
						RepoLink      string `json:"RepoLink"`
						DistroName    string `json:"DistroName"`
						DistroVersion string `json:"DistroVersion"`
						NVD           struct {
							CVSSv3 struct {
								Vectors string      `json:"Vectors"`
								Score   interface{} `json:"Score"`
							} `json:"CVSSv3"`
						} `json:"NVD"`
					} `json:"Metadata"`
				} `json:"Vulnerabilities"`
			} `json:"Features"`
		} `json:"Layer"`
	} `json:"data"`
}
