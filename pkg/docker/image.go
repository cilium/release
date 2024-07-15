// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type ImageInfo struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
		Platform  struct {
			Architecture string `json:"architecture"`
			Os           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
}

var ErrImageNotFound = errors.New("error image not found")

// ImageSHA256AMD64 returns the image digest. It returns the digest in hexadecimal
// form with the sha256 prefix for the amd64 architecture
func ImageSHA256AMD64(image, tag string) (string, error) {
	cmd := exec.Command(
		"docker",
		"buildx",
		"imagetools",
		"inspect",
		image+":"+tag,
		"--raw")

	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// If the image is not found then default to the 'latest'
			// as this can be an image that is being released from the "main"
			// branch.
			if strings.TrimSpace(string(ee.Stderr)) == fmt.Sprintf("ERROR: %s:%s: not found", image, tag) {
				return "", ErrImageNotFound
			}
		}
		return "", fmt.Errorf("Error executing command %q: %w\n%s", cmd.Args, err, out)
	}
	var imageInfo ImageInfo
	err = json.Unmarshal(out, &imageInfo)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal the docker image info: %w", err)
	}

	for _, manifest := range imageInfo.Manifests {
		if manifest.Platform.Architecture == "amd64" && manifest.Platform.Os == "linux" {
			return manifest.Digest, nil
		}
	}

	return "", fmt.Errorf("digest not found")
}
