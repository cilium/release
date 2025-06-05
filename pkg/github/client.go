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
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/gofri/go-github-pagination/githubpagination"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"

	gh "github.com/google/go-github/v62/github"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
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

func NewClient(logger *log.Logger) *gh.Client {
	cache := diskcache.New(".github_cache")
	cacheTransport := httpcache.NewTransport(cache)

	rateLimiter := github_ratelimit.New(cacheTransport,
		github_primary_ratelimit.WithLimitDetectedCallback(func(ctx *github_primary_ratelimit.CallbackContext) {
			logger.Printf("Primary rate limit detected: category %s, reset time: %v\n", ctx.Category, ctx.ResetTime)
		}),
		github_secondary_ratelimit.WithLimitDetectedCallback(func(ctx *github_secondary_ratelimit.CallbackContext) {
			logger.Printf("Secondary rate limit detected: reset time: %v, total sleep time: %v\n", ctx.ResetTime, ctx.TotalSleepTime)
		}),
	)

	paginator := githubpagination.NewClient(rateLimiter,
		githubpagination.WithPerPage(100), // default to 100 results per page
	)

	return gh.NewClient(paginator).WithAuthToken(Token())
}
