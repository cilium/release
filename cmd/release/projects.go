// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package release

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cilium/release/cmd/changelog"
	"github.com/cilium/release/pkg/io"
	"github.com/schollz/progressbar/v3"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/semaphore"
)

const (
	// projTemplateName is the string used to derive the template name that
	// should be available as a "template" in the organization's projects.
	projTemplateName = `[TEMPLATE] %s - vX.Y.Z`
)

func projTemplateNameGenerator(project string) string {
	return fmt.Sprintf(projTemplateName, project)
}

type ProjectManagement struct {
	cfg *ReleaseConfig
}

func NewProjectsManagement(cfg *ReleaseConfig) *ProjectManagement {
	return &ProjectManagement{
		cfg: cfg,
	}
}

func (pm *ProjectManagement) Name() string {
	return "managing projects"
}

func (pm *ProjectManagement) projectName() string {
	return fmt.Sprintf("%s %s", pm.cfg.Repo, pm.cfg.TargetVer)
}

func (pm *ProjectManagement) Run(ctx context.Context, yesToPrompt, dryRun bool, ghClient *GHClient) error {
	if semver.Prerelease(pm.cfg.TargetVer) != "" {
		io.Fprintf(1, os.Stdout, "Pre-Releases don't have a tracking project."+
			" Continuing with the release process.\n")
		return nil
	}

	io.Fprintf(1, os.Stdout, "Regenerating CHANGELOG to track which PRs belong to this release.\n")
	lg := &Logger{
		depth: 3,
	}
	// Generate the CHANGELOG from previous release to current release.
	clCfg := changelog.ChangeLogConfig{
		CommonConfig: pm.cfg.CommonConfig,
		Base:         pm.cfg.PreviousVer,
		Head:         pm.cfg.TargetVer,
		StateFile:    pm.cfg.StateFile,
	}
	err := clCfg.Sanitize()
	if err != nil {
		return err
	}

	releaseNotes, err := changelog.GenerateReleaseNotes(ctx, ghClient.ghClient, lg, clCfg)
	if err != nil {
		return err
	}

	allPRs, nodeIDs := releaseNotes.AllPRs()

	io.Fprintf(2, os.Stdout, "All PRs, including backports, that belong to %s:\n", pm.cfg.TargetVer)
	for prNumber := range allPRs {
		io.Fprintf(3, os.Stdout, "- https://github.com/%s/%s/pull/%d\n", pm.cfg.Owner, pm.cfg.Repo, prNumber)
	}

	io.Fprintf(1, os.Stdout, "Finding project for %s.\n", pm.cfg.TargetVer)
	currProjID, currProjNumber, err := pm.findProject(ctx, ghClient, pm.projectName())
	if err != nil {
		return err
	}

	if currProjID == nil {
		io.Fprintf(2, os.Stdout, "Project for %s not found, creating from the template.\n", pm.cfg.TargetVer)
		templateName := projTemplateNameGenerator(pm.cfg.Repo)
		if !dryRun {
			currProjID, err = pm.createProjectFromTemplate(ctx, ghClient, templateName, pm.projectName())
		}
		if err != nil {
			return fmt.Errorf("unable to create project: %w", err)
		}
		currProjID, currProjNumber, err = pm.findProject(ctx, ghClient, pm.projectName())
		if err != nil {
			return err
		}
	}
	var (
		statusFieldId, releaseOptionID graphql.ID
		releaseOptionIDStr             githubv4.String
	)
	if !dryRun {
		statusFieldId, releaseOptionID, err = pm.getProject(ctx, ghClient, currProjNumber)
		if err != nil {
			return err
		}
		releaseOptionIDStr = githubv4.String(releaseOptionID.(string))
	}

	io.Fprintf(1, os.Stdout, "Adding PRs to the project https://github.com/orgs/%s/projects/%d\n", pm.cfg.Owner, currProjNumber)

	bar := progressbar.Default(int64(len(nodeIDs)), "Adding PRs to project")
	// Update Project with PRs
	sem := semaphore.NewWeighted(5)
	var wg sync.WaitGroup
	errCh := make(chan error, len(nodeIDs))
	defer close(errCh)

	for prNumber, prNodeID := range nodeIDs {
		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("unable to acquire semaphore: %w", err)
		}
		wg.Add(1)

		go func(prNumber int, prNodeID string) {
			defer bar.Add(1)
			defer wg.Done()
			defer sem.Release(1)

			if !dryRun {
				err := pm.addPRToProject(ctx, ghClient, prNodeID, currProjID, statusFieldId, releaseOptionIDStr)
				if err != nil {
					// retry once if fails
					time.Sleep(5 * time.Second)
					err := pm.addPRToProject(ctx, ghClient, prNodeID, currProjID, statusFieldId, releaseOptionIDStr)
					if err != nil {
						errCh <- fmt.Errorf("unable to add PR %d to project %s: %w", prNumber, pm.projectName(), err)
					}
				}
			}
		}(prNumber, prNodeID)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}

	bar.Finish()

	io.Fprintf(1, os.Stdout, "Publishing project\n")
	if !dryRun {
		err = pm.publishProject(ctx, ghClient, currProjID)
		if err != nil {
			// TODO this is a confirmed limitation of GH as it doesn't allow an
			//  app to publish the projects. They are addressing this bug.
			io.Fprintf(1, os.Stdout, "⚠️⚠️ ERR: %s. Unable to publish the project!\n", err)
			io.Fprintf(1, os.Stdout, "⚠️⚠️ You need to manually close it and mark it as public/private depending if\n")
			io.Fprintf(1, os.Stdout, "⚠️⚠️ the repository is public or private.\n")
			io.Fprintf(1, os.Stdout, "⚠️⚠️ The project is under https://github.com/orgs/%s/projects/%d\n", pm.cfg.Owner, currProjNumber)
			// return fmt.Errorf("unable to publish project: %w", err)
		}
	}
	return nil
}

type project struct {
	Closed githubv4.Boolean
	ID     githubv4.ID
	Number githubv4.Int
	Title  githubv4.String
}

type ProjectsGraphQL struct {
	TotalCount githubv4.Int
	Nodes      []project
	PageInfo   struct {
		EndCursor   githubv4.String
		HasNextPage githubv4.Boolean
	}
}

// queryResultProjects was derived from
//
//	query organization {
//	  organization(login: "$organization") {
//	    projectsV2(first: 50, after: $projectsWithRoleCursor) {
//	      nodes {
//	        closed
//	        id
//	        title
//	      }
//	    }
//	  }
//	}
type queryResultProjects struct {
	Organization struct {
		ProjectsV2 ProjectsGraphQL `graphql:"projectsV2(first: 50, after: $projectsWithRoleCursor)"`
	} `graphql:"organization(login: $organization)"`
}

// queryOrganization was derived from
//
//	query organization {
//	  organization(login: "$organization") {
//	    id
//	  }
//	}
type queryOrganization struct {
	Organization struct {
		ID graphql.ID
	} `graphql:"organization(login: $organization)"`
}

func (pm *ProjectManagement) queryOrgProjects(ctx context.Context, gqlGHClient *GHClient, additionalVariables map[string]interface{}) (queryResultProjects, error) {
	var q queryResultProjects
	variables := map[string]interface{}{
		"organization":           githubv4.String(pm.cfg.Owner),
		"projectsWithRoleCursor": (*githubv4.String)(nil), // Null after argument to get first page.
	}

	for k, v := range additionalVariables {
		variables[k] = v
	}

	err := gqlGHClient.ghGQLClient.Query(ctx, &q, variables)
	if err != nil {
		return queryResultProjects{}, err
	}

	return q, nil
}

func (pm *ProjectManagement) findProject(ctx context.Context, ghClient *GHClient, curr string) (any, int, error) {
	var currentProjID any
	currentProjNumber := -1

	variables := map[string]interface{}{}

	resultProjects, err := pm.queryOrgProjects(ctx, ghClient, variables)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query org projects github api: %w", err)
	}

	requeryProjects := false
	for {
		if requeryProjects {
			resultProjects, err = pm.queryOrgProjects(ctx, ghClient, variables)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to requery org projects github api: %w", err)
			}
			requeryProjects = false
		}

		for _, proj := range resultProjects.Organization.ProjectsV2.Nodes {
			if string(proj.Title) == curr {
				currentProjID = proj.ID
				currentProjNumber = int(proj.Number)
			}
		}

		if !resultProjects.Organization.ProjectsV2.PageInfo.HasNextPage {
			break
		}

		requeryProjects = true
		variables["projectsWithRoleCursor"] = githubv4.NewString(resultProjects.Organization.ProjectsV2.PageInfo.EndCursor)
	}
	return currentProjID, currentProjNumber, nil
}

func (pm *ProjectManagement) createProjectFromTemplate(ctx context.Context, gqlGHClient *GHClient, templateName string, projectName string) (any, error) {
	templateProjID, _, err := pm.findProject(ctx, gqlGHClient, templateName)
	if err != nil {
		return nil, err
	}

	if templateProjID == nil {
		return nil, fmt.Errorf("template not found. Make sure the project template %q exists", projTemplateNameGenerator(pm.cfg.Repo))
	}

	var q queryOrganization
	variables := map[string]interface{}{
		"organization": githubv4.String(pm.cfg.Owner),
	}

	err = gqlGHClient.ghGQLClient.Query(ctx, &q, variables)
	if err != nil {
		return nil, err
	}

	// Create Project
	var m struct {
		CreateProjectV2 struct {
			ProjectV2 struct {
				// OrgID
				ID githubv4.ID
			}
		} `graphql:"copyProjectV2(input: $input)"`
	}

	input := githubv4.CopyProjectV2Input{
		ProjectID: templateProjID,
		OwnerID:   q.Organization.ID,
		Title:     githubv4.String(projectName),
	}

	err = gqlGHClient.ghGQLClient.Mutate(ctx, &m, input, nil)
	if err != nil {
		return nil, err
	}

	return m.CreateProjectV2.ProjectV2.ID, err
}

func (pm *ProjectManagement) publishProject(ctx context.Context, gqlGHClient *GHClient, id any) error {
	repository, _, err := gqlGHClient.ghClient.Repositories.Get(ctx, pm.cfg.Owner, pm.cfg.Repo)
	if err != nil {
		return err
	}

	upi := githubv4.UpdateProjectV2Input{
		ProjectID: id,
		Closed:    func() *githubv4.Boolean { a := githubv4.Boolean(true); return &a }(),
		// Set the project public only the repository is also public.
		Public: func() *githubv4.Boolean { a := githubv4.Boolean(!repository.GetPrivate()); return &a }(),
	}

	var m struct {
		UpdateProjectV2 struct {
			ProjectV2 struct {
				ID githubv4.ID
			}
		} `graphql:"updateProjectV2(input: $input)"`
	}

	var publicOrPrivate string
	if repository.GetPrivate() {
		publicOrPrivate = "private"
	} else {
		publicOrPrivate = "public"
	}
	io.Fprintf(2, os.Stdout, "Publishing project as 'closed' and marking it as %q\n", publicOrPrivate)

	return gqlGHClient.ghGQLClient.Mutate(ctx, &m, upi, nil)
}

func (pm *ProjectManagement) addPRToProject(ctx context.Context, gqlGHClient *GHClient, prID string, projectID, fieldID githubv4.ID, optionID githubv4.String) error {
	// Add PR to Project
	var addItemMutation struct {
		AddProjectV2ItemById struct {
			Item struct {
				ID graphql.ID
			}
		} `graphql:"addProjectV2ItemById(input: $input)"`
	}

	input := githubv4.AddProjectV2ItemByIdInput{
		ProjectID: projectID,
		ContentID: prID,
	}

	err := gqlGHClient.ghGQLClient.Mutate(ctx, &addItemMutation, input, nil)
	if err != nil {
		return err
	}

	// Update PR with project item v2
	var updateMutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID graphql.ID
			} `graphql:"projectV2Item"`
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}

	updateItemInput := githubv4.UpdateProjectV2ItemFieldValueInput{
		ProjectID: projectID,
		ItemID:    addItemMutation.AddProjectV2ItemById.Item.ID,
		FieldID:   fieldID,
		Value: githubv4.ProjectV2FieldValue{
			SingleSelectOptionID: &optionID,
		},
	}

	return gqlGHClient.ghGQLClient.Mutate(ctx, &updateMutation, updateItemInput, nil)
}

// ProjectV2SingleSelectField was derived from
//
//	query organization {
//	 organization(login: "$login") {
//	   projectV2(number: $number) {
//	     field(name: "Status") {
//	       ... on ProjectV2SingleSelectField {
//	         id
//	         options {
//	           id
//	           name
//	         }
//	       }
//	     }
//	   }
//	 }
//	}
type ProjectV2SingleSelectField struct {
	ID      graphql.ID `graphql:"id"`
	Options []struct {
		ID   graphql.ID     `graphql:"id"`
		Name graphql.String `graphql:"name"`
	} `graphql:"options"`
}

type Project struct {
	Field struct {
		ProjectV2SingleSelectField `graphql:"... on ProjectV2SingleSelectField"`
	} `graphql:"field(name: \"Status\")"`
}

type Organization struct {
	ProjectV2 Project `graphql:"projectV2(number: $number)"`
}

func (pm *ProjectManagement) getProject(ctx context.Context, gqlGHClient *GHClient, projectNumber int) (graphql.ID, graphql.ID, error) {
	const statusName = "Released"

	// Fetch the field ID and options for the "Status" field
	var orgQuery struct {
		Organization struct {
			ProjectV2 Project `graphql:"projectV2(number: $number)"`
		} `graphql:"organization(login: $login)"`
	}

	orgVars := map[string]any{
		"login":  graphql.String(pm.cfg.Owner),
		"number": graphql.Int(projectNumber),
	}

	err := gqlGHClient.ghGQLClient.Query(ctx, &orgQuery, orgVars)
	if err != nil {
		return nil, nil, err
	}

	stateField := orgQuery.Organization.ProjectV2.Field.ProjectV2SingleSelectField
	if stateField.ID == "" {
		return nil, nil, fmt.Errorf("status field not found. Make sure the project template %q contains the 'Status' field", projTemplateNameGenerator(pm.cfg.Repo))
	}

	var stateOptionID graphql.ID

	for _, option := range stateField.Options {
		if option.Name == statusName {
			stateOptionID = option.ID
			break
		}
	}

	if stateOptionID == "" {
		return nil, 0, fmt.Errorf("option %s not found. Make sure the project template %q contains the 'Status' field with the option %s", statusName, projTemplateNameGenerator(pm.cfg.Repo), statusName)
	}

	return stateField.ID, stateOptionID, nil
}
