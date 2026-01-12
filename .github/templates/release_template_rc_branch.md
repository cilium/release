---
name: Release a new RC version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z-rc.W release'
labels: kind/release
assignees: ''

---

## Setup preparation

- Ensure Docker is installed and running
- Ensure a setup is in place for [signing tags] with Git (GPG, SSH, S/MIME)
- Install [gh](https://cli.github.com).
- Make sure the [release][Cilium release-notes tool] repository is installed
  locally:
  - Run `git clone https://github.com/cilium/release.git "$GOPATH/src/github.com/cilium/release"`
- [ ] Make sure the `release` binary is up to date:
      `git checkout master && git pull && make`
- Read the documentation of `release start --help` tool to understand what
  each automated step does.

## Lead up to feature freeze (run ~1 week before feature freeze date)

- [ ] Announce on Slack #development channel that the feature freeze is one
      week away and that contributors should coordinate with reviewers to
      assess whether their PRs are on track for merging prior to feature
      freeze. Example announcement:

      :cilium-gopher: :mega: Feature freeze is in *one week*.

      If you are planning on getting a PR into the upcoming stable release,
      please coordinate with your reviewers to ensure that you have aligned
      expectations for bandwidth in order to develop, test, review and merge
      the PR.

## Feature freeze day

- [ ] Announce feature freeze is in effect on Slack. Example announcement:

      :cilium-gopher: :mega: Feature freeze is *in effect*.

      The `main` branch is now closed for merging feature changes into the
      branch for the upcoming stable release. You may continue to develop
      and update your feature PRs, however as a general rule they will not
      be merged until the feature branching is complete in around one week.

      The feature freeze does not restrict bugfixes or improvements to
      testing and documentation. We welcome these submissions at any time.
      :cilium-bounce:

- [ ] [Review feature PRs] to determine whether they are critical for the
      upcoming release or not. Discuss with other committers any potential
      exceptions to the feature freeze.

      Criteria for granting an exception:
      - Multiple committers consider that the benefit of merging the PR in
        violation of the freeze outweighs the risks introduced by merging
        the proposal.
      - The PR is self-contained or requires minimal subsequent changes in
        order to complete the functionality outlined in the PR.
      - The author is actively working on the PR to drive it to completion.
      - The testsuite has been run against the PR and there are no major
        shortfalls in testing.
      - A committer has already reviewed the overall goals of the PR and
        it is not expected to be controversial.

      General exemptions to feature freeze:
      - Bug fixes
      - Testing enhancements
      - Documentation enhancements

- [ ] Mark all PRs that are not granted an exception with the GitHub label
      `dont-merge/wait-until-release`.

   - [ ] Review and merge open [renovate PRs]. Use the [dependency dashboard] to
         ensure that dependency updates are current. This reduces duplicate effort
         to update the upcoming stable branch as well as main for each of the
         corresponding dependencies.

## Pre-release

- [ ] Change directory to the local copy of Cilium repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] If stable branch is not created yet. Run:
  - `git fetch upstream && git checkout -b vX.Y upstream/main`
  - [ ] Push that branch to GitHub:
    - `git push upstream vX.Y`
  - [ ] On the main branch, create a PR with a change in the `VERSION` file to
        start the next development cycle as well as creating the necessary GH
        workflows (renovate configuration, etc.
        see [24143732b616](https://github.com/cilium/cilium/commit/24143732b616bb6cd308564b0be33f13fc5613e6)
        for reference):
    - [ ] Check for any other .github workflow references to the current stable
          branch `X.Y-1`, and update those to include the new stable `X.Y`
          version as well.
      - `git grep "X.Y-1" .github/`
    - [ ] Ensure that the `CustomResourceDefinitionSchemaVersion` uses a new minor schema version compared to the new `X.Y` release.
      - `git grep 'CustomResourceDefinitionSchemaVersion =' -- pkg/`
    - `echo "X.Y+1.0-dev" > VERSION`
    - `make -C install/kubernetes`
    - `git add .github/ install/ pkg/k8s`
    - `git commit -sam "Prepare for vX.Y+1 development cycle"`
  - [ ] Merge the main PR so that the stable branch protection can be properly
        set up with the right status checks requirements.
  - [ ] Sync the `vX.Y` branch up to the commit before preparing for the `X.Y+1` development cycle.
    - `git fetch upstream && git checkout vX.Y && git merge --ff-only upstream/main~1 && git log -5`
    - `git push upstream vX.Y`
  - [ ] Push a copy of the latest CI image as a temporary vX.Y CI image version
        so that the upgrade workflow on `main` can upgrade from it.
    - ```
      skopeo login quay.io

      REPOS=$(yq '.jobs.build-and-push-prs.strategy.matrix.include[] | select(.name != "cilium-cli") | .name' .github/workflows/build-images-ci.yaml)
      COMMIT=$(git rev-parse vX.Y)
      for repo in $REPOS; do
          skopeo copy -a docker://quay.io/cilium/$repo-ci:$COMMIT docker://quay.io/cilium/$repo-ci:vX.Y;
      done

      skopeo logout
      ```
  - [ ] Protect the new stable branch with GitHub Settings [here](https://github.com/cilium/cilium/settings/branches)
    - Use the settings of the previous stable branch and main as sane defaults
  - [ ] On the `vX.Y` branch, prepare for stable release development:
    - [ ] Remove any GitHub workflows from the stable branch that are only
          relevant for the main branch (Read the following before running
          this step).
      - Remove workflows that are exclusively triggered by cron job and
        workflows triggered by `issue_comment` triggers, as they do not run on
        stable branches.
        - ```
          for f in .github/workflows/*yaml; do
              if [ $(yq '.on | pick(["push", "pull_request", "pull_request_target", "merge_group", "workflow_call", "workflow_dispatch"]) | length' $f) == '0' ]; then
                  git rm $f;
              fi;
          done
          ```
      - Replace references to `main` branch with `X.Y` in the workflows.
        - `sed -i 's/- \(ft\/\)\?main/- \1vX.Y/g' .github/workflows/*`
        - `sed -i 's/@main/@vX.Y/g' .github/workflows/*`
        - `sed -i 's/\/main\//\/vX.Y\//g' .github/workflows/*`
        - `sed -i 's/\(renovate\/\)main/\1vX.Y/g' .github/workflows/*`
        - `sed -i 's/- v\[0-9\]+\.\[0-9\]+/- vX.Y/g' .github/workflows/build-images-releases.yaml`
      - Double-check if there are any other new references to `main` in the
        workflows, and update them as needed.
        - `git grep 'main' .github/workflows/`
      - Remove cilium-cli references in the tree.
        - `git rm -r ./cilium-cli/`
        - `git rm .github/workflows/cilium-cli.yaml`
        - `sed -i 's/ cilium-cli$//' Makefile`
        - `git rm Documentation/cmdref/index_cilium_cli.rst`
        - `sed -i '/cilium_cli$/d' Documentation/cmdref/index.rst`
        - `sed -i '/cilium-cli/d' Documentation/update-cmdref.sh`
        - `make -C Documentation update-cmdref`
        - `go mod vendor && go mod tidy`
      - [ ] Pick the latest cilium CLI version and place it into the action for setting environment variables.
        - `export CLI_RELEASE=$(gh release list --repo cilium/cilium-cli --json tagName,isLatest --jq '.[] | select(.isLatest)|.tagName')`
        - `sed -i 's/^\([ ]*\)\(CILIUM_CLI_VERSION=\)""$/\1# renovate: datasource=github-releases depName=cilium\/cilium-cli\n\1\2"'$CLI_RELEASE'"/g' .github/actions/set-env-variables/action.yml`
      - Update `install/kubernetes/Makefile*`, following the changes made
        during the previous stable branch preparation commit on the previous
        stable branch.
        - `sed -i 's/\(CI_ORG ?=.*$\)/\1\nexport RELEASE := yes/' install/kubernetes/Makefile.values`
        - `make -C install/kubernetes`
        - `make -C Documentation update-helm-values`
      - Ensure that the `CustomResourceDefinitionSchemaVersion` uses a new
        minor schema version compared to the previous stable release.
        - `vim $(git grep -l CustomResourceDefinitionSchemaVersion)`
      - Remove `stable.txt` file
        - `git rm stable.txt`
      - Adjust `./.github/maintainers-little-helper.yaml` to set labels based
        on the new stable branch version. See [5b4934284d](https://github.com/cilium/cilium/commit/5b4934284dd525399aacec17c137811df9cf0f8b)
        for reference.
        - `cp {../cilium-X.Y-1/,}.github/maintainers-little-helper.yaml`
        - `sed -i 's/X.Y-1/X.Y/g' .github/maintainers-little-helper.yaml`
      - Copy CODEOWNERS to TESTOWNERS.
        - `cp CODEOWNERS TESTOWNERS`
        - `git add TESTOWNERS`
      - Rewrite the CODEOWNERS file and docs. Keep the team descriptions from main
        and the previous stable branch. See [97daf56221](https://github.com/cilium/cilium/commit/97daf5622197d0cdda003a3f693e6e5a61038884)
        - `sed -i '/^\//,$d' CODEOWNERS`
        - `grep -v '#' ../cilium-X.Y-1/CODEOWNERS >> CODEOWNERS`
        - `make -C Documentation update-codeowners`
      - Delete unnecessary GitHub configurations from the stable branch
        - `git rm .github/{pull_request,renovate}*`
        - `git rm -r .github/ISSUE_TEMPLATE/`
        - `git rm .github/workflows/lint-codeowners.yaml`
        - `git rm .github/workflows/release.yaml`
        - `git rm .github/workflows/renovate*`
      - Replace references to `bpf-next-*` lvh images in workflows with the
        newest LTS kernel from [quay.io](https://quay.io/repository/lvh-images/kind?tab=tags&tag=latest).
        If there is no newer LTS, delete the corresponding matrix entries.
        - `grep -R bpf-next- .github/workflows/`
      - You may want to initially commit the state up until now before the next
        step, so that it's easier to compare the diff vs. the previous stable
        release.
        - `git commit -s -m "Prepare vX.Y stable branch"`
      - Copy-paste the `.github` directory from the previous stable branch and
        manually check the diff between the files from the current stable branch
        and modify the workflows to match the target stable branch. See
        [8bbae9cb43](https://github.com/cilium/cilium/commit/8bbae9cb4323bf3dd94936e355b0c2aad96d0df8)
        for reference.
        - `cp -R ../cilium-X.Y-1/.github/* .github/`
        - `git diff --stat`
        - Ignore all stable branch changes under the `.github/actions` directory.
          `git checkout .github/actions`
        - `git diff`
        - Yes this step is horribly painful. It's unrealistic for us to make
          reasonable decisions here when scanning thousands of lines of random
          CI changes for the past six months. Suggestions welcome: please
          update these instructions if you find anything we can do better.
    - [ ] Review the diff for this commit compared to the preparation commit
          for the previous stable branch.
    - [ ] Push a PR with those changes:
      - `git commit -sam "Prepare vX.Y stable branch"`
      - `gh pr create -B vX.Y`
    - [ ] Merge the stable branch PR
- [ ] Remove the `dont-merge/wait-until-release` label from [Blocked PRs].
- [ ] Announce on Slack #development channel that the stable branch is
      created and developers must use `release-note/X.Y` labels in order to
      nominate any subsequent changes for the target stable branch. Example
      announcement:

      :cilium-gopher: :mega: We have cut the vX.Y stable branch.

      Any pull request merged into `main` will now NOT be part of the `vX.Y`
      release unless it has the `needs-backport/X.Y` label. The feature
      freeze is now lifted. We encourage you to continue to focus on changes
      that will improve the quality of the upcoming stable release such as
      bugfixes and improvements to testing or documentation. Where possible,
      consider deferring significant refactors until the final vX.Y.0 release
      as this will help with backporting during this period.

      Thank you to all who contribute to this release!

## Pre-check (run ~1 week before targeted publish date)

- [ ] When you create a GitHub issue using this issue template, GitHub Slack app posts a
      message in #launchpad Slack channel. Create a thread for that message and ping the
      current backporter to merge the outstanding [backport PRs] and stop merging any new
      backport PRs until the GitHub issue is closed (to avoid generating incomplete
      release notes).
- [ ] Run `./release start --steps 1-pre-check --target-version vX.Y.Z-rc.W`
  - [ ] Check that there are no [release blockers] for the targeted release
        version.
  - [ ] Ensure that outstanding [backport PRs] are merged (these may be
        skipped on case by case basis in coordination with the backporter).

## Preparation PR (run ~1 day before targeted publish date. This step can be re-run multiple times.)

- [ ] Go to [release workflow] and Run the workflow from "Branch: main", for
  step "2-prepare-release" and version for vX.Y.Z-rc.W
  - [ ] Check if the workflow was successful and check the PR opened by the
        Release bot.
- [ ] Merge PR

## Tagging

- [ ] Ask a maintainer if there are any known issues that should hold up the release
- [ ] FYI, do not wait too much time between a tag is created and the helm charts are published.
      Once the tags are published the documentation will be pointing to them. Until we release
      the helm chart, users will face issues while trying out our documentation.
- [ ] Run `./release start --steps 3-tag --target-version vX.Y.Z-rc.W`
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)

## Post Tagging (run after docker images are published. In case of failure, this step can be re-run multiple times.)

- [ ] Check if the image build process was successful and check the PR opened
      by the Release bot. If the PR was not opened, you can re-run the workflow
  -  **IN CASE THE PR WAS NOT OPENED** Go to [release workflow] and Run the
     workflow from "Branch: main", for step "4-post-release" and version for
     vX.Y.Z-rc.W
- [ ] Merge PR

## Publish helm (run after docker images are published. In case of failure, this step can be re-run multiple times.)

- [ ] Ask a maintainer to approve the build in the following link:
      [Release Tool](https://github.com/cilium/cilium/actions/workflows/release.yaml)
- [ ] Check if the image build process was successful and check if the helm
      chart was published by the Release bot under the [Cilium helm charts][Cilium charts].
      If that did not happen, you can re-run the workflow.
      - **IN CASE THE HELM CHART WAS NOT PUSHED** Go to [release workflow]
        and Run the workflow from "Branch: main", for step "5-publish-helm" and
        version for vX.Y.Z-rc.W
- [ ] Open [Charts Workflow] and check if the workflow run is successful for vX.Y.Z-rc.W.

## Publish docs (only for pre/rc releases)

- [ ] Check [read the docs] configuration:
  - [ ] Set a new build as active and hidden in [active versions].
  - [ ] Deactivate previous RCs.
  - [ ] Update algolia configuration search in [docsearch-scraper-webhook].
    - Update the versions in `docsearch.config.json`, commit them and push a
      trigger the workflow [here](https://github.com/cilium/docsearch-scraper-webhook/actions/workflows/update-algolia-index.yaml)

## Post-release

- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
  - [ ] Check with @cilium/security if the release addresses any open security
        advisory. If it does, include the list of security advisories at the
        top of the release notes.
  - [ ] Check if the GitHub release page with the options:
        _Set as a pre-release_ and _Create a discussion for this release_ in
        the "Announcements" category.
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (do not use [@]channel).
      See below for templates.
- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`.
- [ ] Update the upgrade guide and [roadmap](https://github.com/cilium/cilium/blob/main/Documentation/community/roadmap.rst)
      for any features that changed status. Usually do it after the RC1, once the
      stability of features is known.

---
Text template for the first RC:
```
*Announcement* :tada: :tada:

:cilium-new: *Cilium release candidate vX.Y.Z-rc.W has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-rc.W

This is the first monthly snapshot for the vX.Y development cycle. There are [vX.Y.Z-rc.W OSS docs](https://docs.cilium.io/en/vX.Y.Z-rc.W) available if you want to pull this version & try it out.
```
Text template for the next RCs:
```
*Announcement* :tada: :tada:

:cilium-new: *Cilium release candidate vX.Y.Z-rc.W has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-rc.W

Thank you for the testing and contributing to the previous pre-releases. There are [vX.Y.Z-rc.W OSS docs](https://docs.cilium.io/en/vX.Y.Z-rc.W) available if you want to pull this version & try it out.
```

[release workflow]: https://github.com/cilium/cilium/actions/workflows/release.yaml
[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Abackport%2FX.Y
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/RELEASE.md
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/?version_filter=vX.Y.Z-rc.W
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[chart workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
[Cilium charts]: https://github.com/cilium/charts
[Review feature PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+-label%3Arelease-note%2Fbug+-label%3Arelease-note%2Fci+-author%3Aapp%2Fcilium-renovate+-label%3Adont-merge%2Fwait-until-release+-label%3Adont-merge%2Fpreview-only+-label%3Aarea%2Fdocumentation+-label%3Acilium-cli-exclusive+-label%3Arelease-blocker%2FX.Y
[Renovate PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+author%3Aapp%2Fcilium-renovate+base%3Amain
[Blocked PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Adont-merge%2Fwait-until-release+base%3Amain
[dependency dashboard]: https://github.com/cilium/cilium/issues/33550
