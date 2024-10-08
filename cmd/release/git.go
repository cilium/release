// Copyright 2021 Authors of Cilium
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

package release

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/release/cmd/changelog"
	"github.com/cilium/release/pkg/github"
	io2 "github.com/cilium/release/pkg/io"
	progressbar "github.com/schollz/progressbar/v3"
	"golang.org/x/mod/semver"
)

type PrepareCommit struct {
	cfg *ReleaseConfig
}

func NewPrepareCommit(cfg *ReleaseConfig) *PrepareCommit {
	return &PrepareCommit{
		cfg: cfg,
	}
}

func (pc *PrepareCommit) Name() string {
	return "preparing release commit"
}

func (pc *PrepareCommit) Run(ctx context.Context, _, _ bool, ghClient *GHClient) error {
	io2.Fprintf(1, os.Stdout, "📤 Submitting changes to a PR\n")

	// Fetch remote branch
	io2.Fprintf(2, os.Stdout, "⬇️ Fetching branch\n")
	remoteName, err := getRemote(pc.cfg.RepoDirectory, pc.cfg.Owner, pc.cfg.Repo)
	if err != nil {
		return err
	}

	// If we are doing a pre-release from the main branch then the remote
	// branch doesn't exist.
	branch := pc.cfg.RemoteBranchName
	crdBranch := semver.MajorMinor(pc.cfg.TargetVer)
	if !pc.cfg.HasStableBranch() {
		branch = pc.cfg.DefaultBranch
		crdBranch = semver.MajorMinor(pc.cfg.PreviousVer)
	}

	localBranch := fmt.Sprintf("pr/prepare-%s", pc.cfg.TargetVer)
	remoteBranch := fmt.Sprintf("%s/%s", remoteName, branch)

	shallowRepo, err := isShallowRepo(pc.cfg.RepoDirectory)
	if err != nil {
		io2.Fprintf(3, os.Stdout, "Unable to detect if repository is shallow, assuming it's not: %s\n", err)
	}
	if shallowRepo {
		io2.Fprintf(3, os.Stdout, "Fetching and unshallowing repository to generate AUTHORS file properly\n")
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "fetch", "-q", "--unshallow", remoteName)
		if err != nil {
			return err
		}
	} else {
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "fetch", "-q", remoteName)
		if err != nil {
			return err
		}
	}

	_, err = execCommand(pc.cfg.RepoDirectory, "git", "checkout", "-b", localBranch, remoteBranch)
	if err != nil {
		return err
	}

	// Update VERSION file
	newErsion := strings.TrimPrefix(pc.cfg.TargetVer, "v")
	io2.Fprintf(2, os.Stdout, "Updating VERSION file with %q\n", newErsion)
	err = writeFile(filepath.Join(pc.cfg.RepoDirectory, "VERSION"), []byte(newErsion+"\n"))
	if err != nil {
		return err
	}

	// Clean digests from Makefile.digests
	digestsMakefile := "install/kubernetes/Makefile.digests"
	io2.Fprintf(2, os.Stdout, "Removing image digests from %q\n", digestsMakefile)
	makefileContent, err := os.ReadFile(filepath.Join(pc.cfg.RepoDirectory, digestsMakefile))
	if err != nil {
		return fmt.Errorf("error reading Makefile.digests file: %w", err)
	}
	re := regexp.MustCompile(`"[^"]*"`)
	output := re.ReplaceAll(makefileContent, []byte(`""`))
	err = writeFile(filepath.Join(pc.cfg.RepoDirectory, digestsMakefile), output)
	if err != nil {
		return err
	}

	// Update helm values file
	io2.Fprintf(2, os.Stdout, "Updating helm values files\n")
	ciliumBranch := fmt.Sprintf("CILIUM_BRANCH=%s", branch)
	_, err = execCommand(pc.cfg.RepoDirectory, "make", "RELEASE=yes", ciliumBranch, "-C", "install/kubernetes", "all", "USE_DIGESTS=false")
	if err != nil {
		return err
	}

	// Update helm documentation
	io2.Fprintf(2, os.Stdout, "Updating Documentation\n")
	_, err = execCommand(pc.cfg.RepoDirectory, "make", "-C", "Documentation", "update-helm-values")
	if err != nil {
		return err
	}

	// Update authors
	io2.Fprintf(2, os.Stdout, "Updating authors\n")
	authorsPath := filepath.Join(pc.cfg.RepoDirectory, "AUTHORS")
	authorsAux := filepath.Join(pc.cfg.RepoDirectory, ".authors.aux")
	err = pc.updateAuthors(ctx, authorsPath, authorsAux)
	if err != nil {
		return err
	}

	// $DIR/../../Documentation/check-crd-compat-table.sh "$target_branch" --update
	//
	io2.Fprintf(2, os.Stdout, "Updating check-crd-compat-table.sh\n")
	_, err = execCommand(pc.cfg.RepoDirectory, "Documentation/check-crd-compat-table.sh", crdBranch, "--update")
	if err != nil {
		return err
	}

	// $DIR/prep-changelog.sh "$old_version" "$version"
	io2.Fprintf(2, os.Stdout, "Preparing Changelog\n")
	err = pc.generateChangeLog(ctx, ghClient)
	if err != nil {
		return err
	}

	// Commit all changes
	io2.Fprintf(2, os.Stdout, "Committing files\n")
	commitFiles := []string{
		"AUTHORS",
		"Documentation/network/kubernetes/compatibility-table.rst",
		"pkg/k8s/apis/cilium.io/register.go",
	}
	// Create an "update authors and update docs" commit but only pre-releases
	// created from the main branch. The pre-releases that are done from stable
	// branches will have everything in a single commit.
	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch, creating a separate commit for AUTHORS and Documentation files\n")
		_, err = execCommand(pc.cfg.RepoDirectory, "git", append([]string{"add"}, commitFiles...)...)
		if err != nil {
			return err
		}

		commitMsg := fmt.Sprintf("update AUTHORS and Documentation")
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "commit", "-sm", commitMsg)
		if err != nil {
			return err
		}
	}

	// Commit the remaining files for patch releases
	commitFiles = append(commitFiles,
		".github/maintainers-little-helper.yaml",
		"CHANGELOG.md",
		"Documentation/helm-values.rst",
		"VERSION",
		"install/kubernetes/cilium/Chart.yaml",
		"install/kubernetes/cilium/README.md",
		"install/kubernetes/cilium/values.yaml",
	)
	// If this release has a stable then add the branch-specific files.
	if pc.cfg.HasStableBranch() {
		commitFiles = append(commitFiles,
			"install/kubernetes/Makefile.digests",
		)
	}
	_, err = execCommand(pc.cfg.RepoDirectory, "git", append([]string{"add"}, commitFiles...)...)
	if err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("Prepare for release %s", pc.cfg.TargetVer)
	_, err = execCommand(pc.cfg.RepoDirectory, "git", "commit", "-sm", commitMsg)
	if err != nil {
		return err
	}

	// Revert the "Prepare for release" commit since that commit will only be
	// used for a tag.
	if !pc.cfg.HasStableBranch() {
		io2.Fprintf(2, os.Stdout, "🧪 Detected pre-release from default branch, reverting commit with helm changes.\n")
		_, err = execCommand(pc.cfg.RepoDirectory, "git", "revert", "-s", "--no-edit", "HEAD")
		if err != nil {
			return err
		}
	}

	return nil
}

func (pc *PrepareCommit) generateChangeLog(ctx context.Context, ghClient *GHClient) error {
	// Retrieve the SHA for the previous release.
	previousPatchVersion := pc.cfg.PreviousVer

	o, err := execCommand(pc.cfg.RepoDirectory, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	commitShaRaw, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	commitSha := strings.TrimSpace(string(commitShaRaw))

	// If we are doing a pre-release from the main branch then the remote
	// branch doesn't exist.
	var lastStable string
	if semver.MajorMinor(pc.cfg.TargetVer) != semver.MajorMinor(pc.cfg.PreviousVer) {
		lastStable = github.MajorMinorErsion(pc.cfg.PreviousVer)
	}

	// Generate the CHANGELOG from previous release to current release.
	io2.Fprintf(3, os.Stdout, "✍️ Generating CHANGELOG.md from %s to %s\n", previousPatchVersion, commitSha)
	io2.Fprintf(4, os.Stdout, "Previous and current version are from different branches, using last stable %q for release notes\n", lastStable)
	clCfg := changelog.ChangeLogConfig{
		CommonConfig: pc.cfg.CommonConfig,
		Base:         previousPatchVersion,
		Head:         commitSha,
		StateFile:    pc.cfg.StateFile,
		LastStable:   lastStable,
	}
	err = clCfg.Sanitize()
	if err != nil {
		return err
	}
	lg := &Logger{
		depth: 3,
	}
	releaseNotes, err := changelog.GenerateReleaseNotes(ctx, ghClient.ghClient, lg, clCfg)
	if err != nil {
		return err
	}

	var changeLogBuf bytes.Buffer
	changeLogBuf.WriteString(fmt.Sprintf("# Changelog\n\n## %s\n\n", pc.cfg.TargetVer))
	releaseNotes.PrintReleaseNotesForWriter(&changeLogBuf)
	changeLogBuf.WriteRune('\n')

	versionChangesFileName := fmt.Sprintf("%s-changes.txt", pc.cfg.TargetVer)
	versionChanges := filepath.Join(pc.cfg.RepoDirectory, versionChangesFileName)
	err = writeFile(versionChanges, changeLogBuf.Bytes())
	if err != nil {
		return err
	}

	changelogFile := filepath.Join(pc.cfg.RepoDirectory, "CHANGELOG.md")
	changelogContent, err := os.Open(changelogFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error reading CHANGELOG.md file: %w", err)
		} else {
			changelogContent, err = os.Create(changelogFile)
		}
	}
	defer changelogContent.Close()

	scanner := bufio.NewScanner(changelogContent)
	for i := 0; scanner.Scan(); i++ {
		// Ignore the first two lines
		if i < 2 {
			continue
		}
		changeLogBuf.Write(append(scanner.Bytes(), byte('\n')))
	}
	return writeFile(changelogFile, changeLogBuf.Bytes())
}

type Logger struct {
	depth int
}

func (l *Logger) Printf(format string, v ...any) {
	io2.Fprintf(l.depth, os.Stdout, format, v...)
}

func (l *Logger) Println(v ...any) {
	s := fmt.Sprintln(v...)
	io2.Fprintf(l.depth, os.Stdout, "%s", s)
}

func execCommand(dir, name string, args ...string) (io.Reader, error) {
	io2.Fprintf(3, os.Stdout, "🧑‍💻 Running command: %s\n",
		strings.Join(append([]string{name}, args...), " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("unable to run command: %w\n\n%s\n\n", err, stderr.String())
	}
	return &stdout, nil
}

func getRemote(gitRepoDir, org, repo string) (string, error) {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = gitRepoDir

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	remoteLines := strings.Split(out.String(), "\n")

	var remote string
	for _, line := range remoteLines {
		reg := regexp.MustCompile(`github.com[/:]` + org + `/` + repo + `(\.git)? `)
		if reg.MatchString(line) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				remote = fields[0]
				break
			}
		}
	}

	if remote == "" {
		return "", fmt.Errorf("No remote git@github.com:%s/%s.git or https://github.com/%s/%s found", org, repo, org, repo)
	}

	return remote, nil
}

func writeFile(fileName string, content []byte) error {
	fileInfo, err := os.Stat(fileName)
	permissions := os.FileMode(0644)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error getting file information for %q: %v", fileName, err)
		}
	} else {
		permissions = fileInfo.Mode()
	}

	err = os.WriteFile(fileName, content, permissions)
	if err != nil {
		return fmt.Errorf("error writing to %q file: %v", fileName, err)
	}
	return nil
}

func (pc *PrepareCommit) updateAuthors(ctx context.Context, authorsFilePath, appendFilePath string) error {
	appendFile, err := os.Open(appendFilePath)
	if err != nil {
		return fmt.Errorf("error opening %s file: %w", appendFilePath, err)
	}
	defer appendFile.Close()

	var output bytes.Buffer

	output.WriteString("The following people, in alphabetical order, have either authored or signed\n")
	output.WriteString("off on commits in the Cilium repository:\n")
	output.WriteString("\n")

	out, err := execCommand(pc.cfg.RepoDirectory,
		"git", "--no-pager", "shortlog", "--summary", "HEAD",
	)
	if err != nil {
		return err
	}

	// Process the output
	authors, err := extractAuthors(out)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	authorsEmails := make([][]byte, len(authors))
	wg.Add(len(authors))
	maxGoRoutines := runtime.NumCPU() - 2
	if maxGoRoutines < 1 {
		maxGoRoutines = 1
	}
	semaphore := make(chan struct{}, maxGoRoutines)
	bar := progressbar.Default(int64(len(authors)), "Preparing authors file")
	defer bar.Finish()
	// Iterate over authors
	for i, author := range authors {
		bar.Add(1)
		semaphore <- struct{}{}
		go func(i int, author string) {
			defer func() {
				<-semaphore
				wg.Done()
			}()
			// Execute git log --use-mailmap --author="$author" --format="%<|(40)%aN%aE" | head -1
			out, err := pipeCommands(ctx, true, pc.cfg.RepoDirectory,
				"git", []string{"log", "--use-mailmap", "--author=" + author, `--format=%<|(40)%aN%aE`, "HEAD"},
				"head", []string{"-1"},
			)

			if err != nil {
				io2.Fprintf(3, os.Stderr, "error getting author e-mail: %s\n", err)
				return
			}
			scanner := bufio.NewScanner(out)
			for scanner.Scan() {
				authorsEmails[i] = scanner.Bytes()
			}
		}(i, author)
	}

	wg.Wait()

	cmd := exec.Command("sort", "-u")
	cmd.Env = append(cmd.Env, "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")

	var stdin bytes.Buffer
	cmd.Stdin = &stdin
	cmd.Stdout = &output

	for _, authorsEmail := range authorsEmails {
		if len(authorsEmail) == 0 {
			continue
		}
		stdin.Write(authorsEmail)
		stdin.WriteRune('\n')
	}
	err = cmd.Run()
	if err != nil {
		return err
	}

	_, err = io.Copy(&output, appendFile)
	if err != nil {
		return fmt.Errorf("error writing from %s file: %w", appendFilePath, err)
	}

	err = writeFile(authorsFilePath, output.Bytes())
	if err != nil {
		return fmt.Errorf("error writing into %s file: %w", authorsFilePath, err)
	}

	return nil
}

func pipeCommands(ctx context.Context, quiet bool, directory string, cmd1Name string, cmd1Args []string, cmd2Name string, cmd2Args []string) (io.Reader, error) {
	ctx1, cancel1 := context.WithCancel(ctx)
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	cmd1 := exec.CommandContext(ctx1, cmd1Name, cmd1Args...)
	cmd1.Dir = directory
	cmd2 := exec.CommandContext(ctx2, cmd2Name, cmd2Args...)
	cmd2.Dir = directory

	if !quiet {
		io2.Fprintf(3, os.Stdout, "🧑‍💻 Running command: %s | %s\n",
			strings.Join(cmd1.Args, " "),
			strings.Join(cmd2.Args, " "))
	}

	r, w := io.Pipe()
	defer r.Close()
	cmd1.Stdout = w
	cmd2.Stdin = r

	var output bytes.Buffer
	var stderr bytes.Buffer
	cmd2.Stdout = &output
	cmd2.Stderr = &stderr

	// Execute commands
	if err := cmd1.Start(); err != nil {
		fmt.Println("Error starting cmd1:", err)
		return nil, err
	}
	if err := cmd2.Start(); err != nil {
		fmt.Println("Error starting cmd2:", err)
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// Wait for both commands to finish
		defer wg.Done()
		defer w.Close()
		err := cmd1.Wait()
		if err != nil {
			ps, ok := cmd1.ProcessState.Sys().(syscall.WaitStatus)
			if !(ok && ps.Signal() == syscall.SIGPIPE) {
				fmt.Println("Error waiting for cmd1:", err)
			}
		}
	}()

	if err := cmd2.Wait(); err != nil {
		fmt.Println("Error waiting for cmd2:", err)
		return nil, err
	}

	err := cmd1.Process.Signal(syscall.SIGPIPE)
	if err != nil && err.Error() != "os: process already finished" {
		panic(err)
	}
	w.Close()

	wg.Wait()

	return &output, nil
}

func extractAuthors(input io.Reader) ([]string, error) {
	var authors []string
	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Remove count part from the line
		author := strings.TrimSpace(strings.SplitN(line, "\t", 2)[1])
		if !strings.Contains(author, "vagrant") {
			authors = append(authors, author)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	return authors, nil
}

func isShallowRepo(repo string) (bool, error) {
	r, err := execCommand(repo, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, err
	}
	outputRaw, err := io.ReadAll(r)
	if err != nil {
		return false, err
	}
	output := strings.TrimSpace(string(outputRaw))

	return output == "true", nil
}
