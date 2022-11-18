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

package projects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v28/github"
)

const (
	needsBackport         = "Needs backport from master"
	pendingBackportPrefix = "Backport pending to v"
	doneBackportPrefix    = "Backport done to v"

	needsBackportLbl   = "needs-backport/"
	pendingBackportLbl = "backport-pending/"
	doneBackportLbl    = "backport-done/"
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
	owner    string
	repo     string
	ghClient *gh.Client
}

func (pm *ProjectManagement) findProjects(ctx context.Context, curr, next string) (int64, int64, error) {
	projs, _, err := pm.ghClient.Repositories.ListProjects(ctx, pm.owner, pm.repo, &gh.ProjectListOptions{State: "open"})
	if err != nil {
		return 0, 0, err
	}
	currentProjID := int64(-1)
	nextProjID := int64(-1)
	for _, proj := range projs {
		if proj.GetName() == curr {
			currentProjID = proj.GetID()
		}
		if proj.GetName() == next {
			nextProjID = proj.GetID()
		}
	}
	return currentProjID, nextProjID, nil
}

func (pm *ProjectManagement) findColumnIDs(ctx context.Context, projID int64, ver string) (needs, pending, done int64, err error) {
	var columns []*gh.ProjectColumn
	needs = int64(-1)
	pending = int64(-1)
	done = int64(-1)

	columns, _, err = pm.ghClient.Projects.ListProjectColumns(ctx, projID, &gh.ListOptions{})
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

func (pm *ProjectManagement) createProject(ctx context.Context, name string) (int64, int, error) {
	proj, _, err := pm.ghClient.Repositories.CreateProject(ctx, pm.owner, pm.repo, &gh.ProjectOptions{
		Name: &name,
	})
	if err != nil {
		return 0, 0, err
	}
	return proj.GetID(), proj.GetNumber(), nil
}

func (pm *ProjectManagement) createColumn(ctx context.Context, projID int64, name string) (int64, error) {
	column, _, err := pm.ghClient.Projects.CreateProjectColumn(ctx, projID, &gh.ProjectColumnOptions{
		Name: name,
	})
	if err != nil {
		return 0, err
	}
	return column.GetID(), nil
}

func (pm *ProjectManagement) reCreateProjectColumns(ctx context.Context, projID int64, ver string, needs, pending, done int64) (int64, int64, int64, error) {
	var err error
	if needs == -1 {
		needs, err = pm.createColumn(ctx, projID, needsBackport)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if pending == -1 {
		pendingBackportColumnName := columnName(pendingBackportPrefix, ver)
		pending, err = pm.createColumn(ctx, projID, pendingBackportColumnName)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if done == -1 {
		doneBackportColumnName := columnName(doneBackportPrefix, ver)
		done, err = pm.createColumn(ctx, projID, doneBackportColumnName)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return needs, pending, done, nil
}

func (pm *ProjectManagement) moveCard(ctx context.Context, cardID, columnID int64) error {
	_, err := pm.ghClient.Projects.MoveProjectCard(ctx, cardID, &gh.ProjectCardMoveOptions{
		Position: "top",
		ColumnID: columnID,
	})
	return err
}

func (pm *ProjectManagement) createCard(ctx context.Context, columnID, prID int64) error {
	_, _, err := pm.ghClient.Projects.CreateProjectCard(ctx, columnID, &gh.ProjectCardOptions{
		ContentID:   prID,
		ContentType: "PullRequest",
	})
	return err
}

func (pm *ProjectManagement) syncCards(ctx context.Context, currVer, nextVer string, currColumnID, nextColumnID, currDoneColumnID, nextPendingColumnID int64, forceMovePending bool) error {
	// get base cards
	currCards, _, err := pm.ghClient.Projects.ListProjectCards(ctx, currColumnID, &gh.ProjectCardListOptions{})
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
		pr, _, err := pm.ghClient.PullRequests.Get(ctx, pm.owner, pm.repo, prNumber)
		if err != nil {
			return err
		}
		prID := pr.GetID()
		// it's not a PR, it's a GH issue
		if prID == 0 {
			continue
		}
		// If it is not backported them move it to the right column in the
		// next project.
		moveToColumnID := nextColumnID
		var labelFound bool
		for _, lbl := range pr.Labels {
			if lbl.GetName() == labelName(doneBackportLbl, currVer) {
				labelFound = true
				// If it is already backported them move it to the right column
				// in the current project.
				fmt.Fprintf(os.Stdout, "moving PR %d to %q\n", prNumber, columnName(doneBackportPrefix, currVer))
				err := pm.moveCard(ctx, currCard.GetID(), currDoneColumnID)
				if err != nil {
					return err
				}
				goto endForLoop
			} else if lbl.GetName() == labelName(pendingBackportLbl, currVer) {
				labelFound = true
				// If it is pending, them move it to the right column in the new
				// project.
				if !forceMovePending {
					return fmt.Errorf("Found unexpected pending PR https://github.com/%s/%d in project. Please ensure that all backported PRs have been moved to the done column.",
						repoName, prNmuber)
				}
				moveToColumnID = nextPendingColumnID
				fmt.Fprintf(os.Stdout, "moving PR %d to %q\n", prNumber, columnName(pendingBackportPrefix, nextVer))
			}
			if labelFound {
				break
			}
		}
		if !labelFound {
			fmt.Fprintf(os.Stdout, "moving PR %d to %q in the project %q\n", prNumber, needsBackport, nextVer)
		}
		err = pm.createCard(ctx, moveToColumnID, prID)
		if err != nil {
			return err
		}
		// Since the project card was moved we can delete it from the current project.
		_, err = pm.ghClient.Projects.DeleteProjectCard(ctx, currCard.GetID())
		if err != nil {
			return err
		}
	endForLoop:
	}
	return err
}

func NewProjectManagement(ghClient *gh.Client, owner, repo string) *ProjectManagement {
	return &ProjectManagement{
		owner:    owner,
		repo:     repo,
		ghClient: ghClient,
	}
}

func (pm *ProjectManagement) SyncProjects(ctx context.Context, currVer, nextVer string, forceMovePending bool) error {
	currProjID, nextProjID, err := pm.findProjects(ctx, currVer, nextVer)
	if err != nil {
		return err
	}

	if currProjID == -1 {
		return fmt.Errorf("current project %q not found", currVer)
	}

	// get columns for the current version
	currNeedsColumnID, currPendingColumnID, currDoneColumnID, err := pm.findColumnIDs(ctx, currProjID, currVer)
	if err != nil {
		return err
	}
	if currNeedsColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", needsBackport, currVer)
	}
	if currPendingColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", columnName(pendingBackportPrefix, currVer), currVer)
	}
	if currDoneColumnID == -1 {
		return fmt.Errorf("required columns %q in current version %q not found", columnName(doneBackportPrefix, currVer), currVer)
	}

	// get columns for the next version
	var (
		nextNeedsColumnID   = int64(-1)
		nextPendingColumnID = int64(-1)
		nextDoneColumnID    = int64(-1)
	)
	if nextProjID == -1 {
		var projNumber int
		fmt.Fprintf(os.Stdout, "Next project %q not found, creating it...\n", nextVer)
		// create project
		nextProjID, projNumber, err = pm.createProject(ctx, nextVer)
		fmt.Fprintf(os.Stdout, "Project created for %q, command for release: start-release.sh %s %d\n", nextVer, currVer, projNumber)

		// create all 3 columns
		nextNeedsColumnID,
			nextPendingColumnID,
			nextDoneColumnID,
			err = pm.reCreateProjectColumns(ctx, nextProjID, nextVer, -1, -1, -1)
		if err != nil {
			return err
		}
	} else {
		// get columns
		nextNeedsColumnID, nextPendingColumnID, nextDoneColumnID, err = pm.findColumnIDs(ctx, nextProjID, nextVer)
		if err != nil {
			return err
		}
		// re-create all 3 columns if necessary
		nextNeedsColumnID,
			nextPendingColumnID,
			nextDoneColumnID,
			err = pm.reCreateProjectColumns(
			ctx,
			nextProjID,
			nextVer,
			nextNeedsColumnID,
			nextPendingColumnID,
			nextDoneColumnID,
		)
		if err != nil {
			return err
		}
	}

	// Move needs backport column cards to the correct columns
	err = pm.syncCards(ctx, currVer, nextVer, currNeedsColumnID, nextNeedsColumnID, currDoneColumnID, nextPendingColumnID, true)
	if err != nil {
		return err
	}
	// Move pending backport column cards to the correct columns
	err = pm.syncCards(ctx, currVer, nextVer, currPendingColumnID, nextPendingColumnID, currDoneColumnID, nextPendingColumnID, forceMovePending)
	if err != nil {
		return err
	}

	// Close the current project
	fmt.Fprintf(os.Stdout, "Closing project %q\n", currVer)
	_, _, err = pm.ghClient.Projects.UpdateProject(ctx, currProjID, &gh.ProjectOptions{
		State: func() *string { a := "closed"; return &a }(),
	})

	return err
}
