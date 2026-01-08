// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package helm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OCIRegistry represents an OCI registry for Helm charts
type OCIRegistry struct {
	URL string
}

// PushChart pushes a Helm chart package to the OCI registry and returns the digest
func (r *OCIRegistry) PushChart(chartPath string) (string, error) {
	if r.URL == "" {
		return "", fmt.Errorf("OCI registry URL is required")
	}

	// Ensure the chart path exists
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		return "", fmt.Errorf("chart package not found: %s", chartPath)
	}

	cmd := exec.Command("helm", "push", chartPath, r.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to push chart to OCI registry: %w\nOutput: %s", err, string(output))
	}

	// Print the output to stdout
	fmt.Print(string(output))

	// Parse the output to extract the digest
	digest := ""
	outputStr := strings.TrimSpace(string(output))
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Digest:") {
			parts := strings.Split(line, "Digest:")
			if len(parts) > 1 {
				digest = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	return digest, nil
}
