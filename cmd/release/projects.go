// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	semver2 "github.com/Masterminds/semver"
	"github.com/cilium/release/pkg/io"
	gh "github.com/google/go-github/v62/github"
)

const (
	needsBackport         = "Needs backport from main"
	pendingBackportPrefix = "Backport pending to v"
	doneBackportPrefix    = "Backport done to v"

	needsBackportLbl   = "needs-backport/"
	pendingBackportLbl = "backport-pending/"
	doneBackportLbl    = "backport-done/"

	projectURL = "https://github.com/%s/%s/projects/%d"
)

func columnName(prefix, version string) string {
	lastIdx := strings.LastIndex(version, ".")
	ver := version[:lastIdx]
	return prefix + ver
}

func labelName(prefix, version string) string {
	lastIdx := strings.LastIndex(version, ".")
	ver := version[:lastIdx]
	return prefix + ver
}

type ProjectManagement struct {
	cfg *ReleaseConfig
}

func (pm *ProjectManagement) findProjects(ctx context.Context, ghClient *gh.Client, curr, next string) (int64, int, int64, int, error) {
	projs, _, err := ghClient.Repositories.ListProjects(ctx, pm.cfg.Owner, pm.cfg.Repo, &gh.ProjectListOptions{State: "open"})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	currentProjID := int64(-1)
	currentProjNumber := int(-1)
	nextProjID := int64(-1)
	nextProjNumber := int(-1)
	for _, proj := range projs {
		if proj.GetName() == curr {
			currentProjID = proj.GetID()
			currentProjNumber = proj.GetNumber()
		}
		if proj.GetName() == next {
			nextProjID = proj.GetID()
			nextProjNumber = proj.GetNumber()
		}
	}
	return currentProjID, currentProjNumber, nextProjID, nextProjNumber, nil
}

func (pm *ProjectManagement) findColumnIDs(ctx context.Context, ghClient *gh.Client, projID int64, ver string) (needs, pending, done int64, err error) {
	var columns []*gh.ProjectColumn
	needs = int64(-1)
	pending = int64(-1)
	done = int64(-1)

	columns, _, err = ghClient.Projects.ListProjectColumns(ctx, projID, &gh.ListOptions{})
	if err != nil {
		return
	}

	pendingColumnName := columnName(pendingBackportPrefix, ver)
	doneColumnName := columnName(doneBackportPrefix, ver)
	for _, column := range columns {
		switch column.GetName() {
		case needsBackport:
			needs = column.GetID()
		case pendingColumnName:
			pending = column.GetID()
		case doneColumnName:
			done = column.GetID()
		}
	}
	return
}

func (pm *ProjectManagement) createProject(ctx context.Context, ghClient *gh.Client, name string) (int64, int, error) {
	proj, _, err := ghClient.Repositories.CreateProject(ctx, pm.cfg.Owner, pm.cfg.Repo, &gh.ProjectOptions{
		Name: &name,
	})
	if err != nil {
		return 0, 0, err
	}
	return proj.GetID(), proj.GetNumber(), nil
}

func (pm *ProjectManagement) createColumn(ctx context.Context, ghClient *gh.Client, projID int64, name string) (int64, error) {
	column, _, err := ghClient.Projects.CreateProjectColumn(ctx, projID, &gh.ProjectColumnOptions{
		Name: name,
	})
	if err != nil {
		return 0, err
	}
	return column.GetID(), nil
}

func (pm *ProjectManagement) reCreateProjectColumns(ctx context.Context, ghClient *gh.Client, projID int64, ver string, needs, pending, done int64) (int64, int64, int64, error) {
	var err error
	if needs == -1 {
		needs, err = pm.createColumn(ctx, ghClient, projID, needsBackport)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if pending == -1 {
		pendingBackportColumnName := columnName(pendingBackportPrefix, ver)
		pending, err = pm.createColumn(ctx, ghClient, projID, pendingBackportColumnName)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if done == -1 {
		doneBackportColumnName := columnName(doneBackportPrefix, ver)
		done, err = pm.createColumn(ctx, ghClient, projID, doneBackportColumnName)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return needs, pending, done, nil
}

func (pm *ProjectManagement) moveCard(ctx context.Context, ghClient *gh.Client, cardID, columnID int64) error {
	_, err := ghClient.Projects.MoveProjectCard(ctx, cardID, &gh.ProjectCardMoveOptions{
		Position: "top",
		ColumnID: columnID,
	})
	return err
}

func (pm *ProjectManagement) createCard(ctx context.Context, ghClient *gh.Client, columnID, prID int64) error {
	_, _, err := ghClient.Projects.CreateProjectCard(ctx, columnID, &gh.ProjectCardOptions{
		ContentID:   prID,
		ContentType: "PullRequest",
	})
	return err
}

func (pm *ProjectManagement) syncCards(ctx context.Context, ghClient *gh.Client, dryRun bool, currVer, nextVer string, currColumnID, nextColumnID, currDoneColumnID, nextPendingColumnID int64) error {
	var dryRunStrPrefix string
	if dryRun {
		dryRunStrPrefix = "[🙅 🙅 DRY RUN - OPERATION WILL NOT BE DONE 🙅 🙅] "
	}
	var page int
	for {
		// get current cards
		currCards, resp, err := ghClient.Projects.ListProjectCards(ctx, currColumnID, &gh.ProjectCardListOptions{
			ArchivedState: nil,
			ListOptions: gh.ListOptions{
				Page: page,
			},
		})
		if err != nil {
			return err
		}

		for _, currCard := range currCards {
			contentURL := currCard.GetContentURL()
			idStr := filepath.Base(contentURL)
			prNumber, err := strconv.Atoi(idStr)
			if err != nil {
				return err
			}
			pr, _, err := ghClient.PullRequests.Get(ctx, pm.cfg.Owner, pm.cfg.Repo, prNumber)
			if err != nil {
				return err
			}
			prID := pr.GetID()
			// it's not a PR, it's a GH issue
			if prID == 0 {
				continue
			}
			// If the PR is not merged, and it's closed then drop it from this
			// project.
			if pr.GetState() == "closed" && !pr.GetMerged() {
				io.Fprintf(3, os.Stdout, "%s🗑️ Removing PR %s from project %s since it is closed and not merged.\n", dryRunStrPrefix, pr.GetHTMLURL(), currVer)
				if !dryRun {
					_, err = ghClient.Projects.DeleteProjectCard(ctx, currCard.GetID())
					if err != nil {
						return err
					}
				}
				continue
			}

			// If it is not backported then move it to the right column in the
			// next project.
			moveToColumnID := nextColumnID

			var labelFound bool
			for _, lbl := range pr.Labels {
				if lbl.GetName() == labelName(doneBackportLbl, currVer) {
					labelFound = true
					// If it is already backported them move it to the right column
					// in the current project.
					io.Fprintf(3, os.Stdout, "%s✅ Backport Done for PR %d moving to - %q\n", dryRunStrPrefix, prNumber, columnName(doneBackportPrefix, currVer))
					if !dryRun {
						err := pm.moveCard(ctx, ghClient, currCard.GetID(), currDoneColumnID)
						if err != nil {
							return err
						}
					}
					goto endForLoop
				} else if lbl.GetName() == labelName(pendingBackportLbl, currVer) {
					labelFound = true
					// If it is pending, them move it to the right column in the new
					// project.
					io.Fprintf(3, os.Stdout, "%s➡️ Backport Pending for PR %d moving to the new project - %q\n", dryRunStrPrefix, prNumber, columnName(pendingBackportPrefix, nextVer))
					moveToColumnID = nextPendingColumnID
				}
				if labelFound {
					break
				}
			}
			if !labelFound {
				io.Fprintf(3, os.Stdout, "%s➡️ Backport not started for PR %d moving to the new project - %q\n", dryRunStrPrefix, prNumber, needsBackport)
			}
			if !dryRun {
				err = pm.createCard(ctx, ghClient, moveToColumnID, prID)
				if err != nil {
					return err
				}
				// Since the project card was moved we can delete it from the current project.
				_, err = ghClient.Projects.DeleteProjectCard(ctx, currCard.GetID())
				if err != nil {
					return err
				}
			}
		endForLoop:
		}

		if resp.NextPage == 0 {
			return nil
		}

		page = resp.NextPage

	}
}

func NewProjectsManagement(cfg *ReleaseConfig) *ProjectManagement {
	return &ProjectManagement{
		cfg: cfg,
	}
}

func (pm *ProjectManagement) Name() string {
	return "managing projects"
}

func (pm *ProjectManagement) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *gh.Client) error {
	var dryRunStrPrefix string
	if dryRun {
		dryRunStrPrefix = "[🙅 🙅 DRY RUN - OPERATION WILL NOT BE DONE 🙅 🙅] "
	}

	io.Fprintf(1, os.Stdout, "👀 Syncing and creating GH projects projects to the next release project\n")

	semVerTarget := semver2.MustParse(pm.cfg.TargetVer)
	targetVersion := semVerTarget.String()
	// Increment the patch version by one.
	nextSemVerTargetVersion := semVerTarget.IncPatch()
	nextTargetVersion := nextSemVerTargetVersion.String()

	currProjID, currProjNumber, nextProjID, nextProjNumber, err := pm.findProjects(ctx, ghClient, targetVersion, nextTargetVersion)
	if err != nil {
		return err
	}

	if currProjID == -1 {
		return fmt.Errorf("current project %q not found", targetVersion)
	}
	targetProjectURL := fmt.Sprintf(projectURL, pm.cfg.Owner, pm.cfg.Repo, currProjNumber)
	io.Fprintf(2, os.Stdout, "Project for %s - %s\n", targetVersion, targetProjectURL)

	// get columns for the target version
	currNeedsColumnID, currPendingColumnID, currDoneColumnID, err := pm.findColumnIDs(ctx, ghClient, currProjID, targetVersion)
	if err != nil {
		return err
	}
	if currNeedsColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", needsBackport, targetVersion)
	}
	if currPendingColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", columnName(pendingBackportPrefix, targetVersion), targetVersion)
	}
	if currDoneColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", columnName(doneBackportPrefix, targetVersion), targetVersion)
	}

	// get columns for the next version
	var (
		nextNeedsColumnID   = int64(-1)
		nextPendingColumnID = int64(-1)
		nextDoneColumnID    = int64(-1)
	)
	if nextProjID == -1 {
		io.Fprintf(2, os.Stdout, "%sNext Project %s not found, creating it...\n", dryRunStrPrefix, nextTargetVersion)
		// create project
		if !dryRun {
			nextProjID, nextProjNumber, err = pm.createProject(ctx, ghClient, nextTargetVersion)
			if err != nil {
				return err
			}
			// create all 3 columns
			nextNeedsColumnID,
				nextPendingColumnID,
				nextDoneColumnID,
				err = pm.reCreateProjectColumns(ctx, ghClient, nextProjID, nextTargetVersion, -1, -1, -1)
			if err != nil {
				return err
			}
		}

		nextTargetProjectURL := fmt.Sprintf(projectURL, pm.cfg.Owner, pm.cfg.Repo, nextProjNumber)
		io.Fprintf(2, os.Stdout, "%sProject for %s - %s\n", dryRunStrPrefix, nextTargetVersion, nextTargetProjectURL)
		io.Fprintf(2, os.Stdout, "%sCommand for release: start-release.sh %s %d\n", dryRunStrPrefix, targetVersion, nextProjNumber)
	} else {
		// get columns
		nextNeedsColumnID, nextPendingColumnID, nextDoneColumnID, err = pm.findColumnIDs(ctx, ghClient, nextProjID, nextTargetVersion)
		if err != nil {
			return err
		}
		if !dryRun {
			// re-create all 3 columns if necessary
			nextNeedsColumnID,
				nextPendingColumnID,
				nextDoneColumnID,
				err = pm.reCreateProjectColumns(
				ctx,
				ghClient,
				nextProjID,
				nextTargetVersion,
				nextNeedsColumnID,
				nextPendingColumnID,
				nextDoneColumnID,
			)
			if err != nil {
				return err
			}
		}
	}
	cfg.ProjectNumber = nextProjNumber

	// Move needs backport column cards to the correct columns
	err = pm.syncCards(ctx, ghClient, dryRun, targetVersion, nextTargetVersion, currNeedsColumnID, nextNeedsColumnID, currDoneColumnID, nextPendingColumnID)
	if err != nil {
		return err
	}
	// Move pending backport column cards to the correct columns
	err = pm.syncCards(ctx, ghClient, dryRun, targetVersion, nextTargetVersion, currPendingColumnID, nextPendingColumnID, currDoneColumnID, nextPendingColumnID)
	if err != nil {
		return err
	}
	// Close the current project
	io.Fprintf(2, os.Stdout, "%sClosing project %q\n", dryRunStrPrefix, targetVersion)
	if !dryRun {
		_, _, err = ghClient.Projects.UpdateProject(ctx, currProjID, &gh.ProjectOptions{
			State: func() *string { a := "closed"; return &a }(),
		})
	}
	return err
}

func (pm *ProjectManagement) Revert(ctx context.Context, dryRun bool, ghClient *gh.Client) error {
	return fmt.Errorf("Not implemented")
}
