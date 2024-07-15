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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	gh "github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

func execCommand(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		fullCmd := strings.Join(append([]string{name}, args...), " ")
		return "", fmt.Errorf("unable to run command %q: %w\n%s",
			fullCmd, err, stderr.String())
	}
	return stdout.String(), nil
}

func Token() string {
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		t, err := execCommand("gh", "auth", "token")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot fetch GITHUB_TOKEN: %s", err)
		}
		ghToken = strings.TrimSpace(t)
	}
	return ghToken
}

func NewClient() *gh.Client {
	return gh.NewClient(
		oauth2.NewClient(
			context.Background(),
			oauth2.StaticTokenSource(
				&oauth2.Token{
					AccessToken: Token(),
				},
			),
		),
	)
}
