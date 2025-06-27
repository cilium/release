---
name: Release a new RC version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z-rc.W release'
labels: kind/release
assignees: ''

---

## Setup preparation

- [ ] Depending on your OS, make sure Docker is running
- [ ] Export a [`GITHUB_TOKEN`](https://github.com/settings/tokens/new?description=Cilium%20Release%20Script&scopes=write:org,public_repo) that has access to the repository
- [ ] Make sure a setup (GPG, SSH, S/MIME) is in place for [signing tags] with Git
- [ ] Make sure the `GOPATH` environment variable is set and pointing to the relevant path
- [ ] Make sure the [Cilium helm charts][Cilium charts] repository is installed locally:
  - [ ] Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`

## Pre-release

- [ ] When you create a GitHub issue using this issue template, GitHub Slack app posts a
      message in #launchpad Slack channel. Create a thread for that message and ping the
      current backporter to merge the outstanding [backport PRs] and stop merging any new
      backport PRs until the GitHub issue is closed (to avoid generating incomplete
      release notes).
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Ensure that outstanding [backport PRs] are merged
- [ ] If stable branch is not created yet. Run:
  - `git fetch upstream && git checkout -b upstream/vX.Y upstream/main`
  - [ ] Push that branch to GitHub:
    - `git push upstream vX.Y`
  - [ ] On the main branch, create a PR with a change in the `VERSION` file to
        start the next development cycle as well as creating the necessary GH
        workflows (renovate configuration, MLH configuration, etc.
        see [24143732b616](https://github.com/cilium/cilium/commit/24143732b616bb6cd308564b0be33f13fc5613e6)
        for reference):
    - [ ] Check for any other .github workflow references to the current stable
          branch `X.Y-1`, and update those to include the new stable `X.Y`
          version as well.
        - `git grep "X.Y-1" .github/`
    - [ ] Ensure that the `CustomResourceDefinitionSchemaVersion` uses a new minor schema version compared to the new `X.Y` release.
    - `echo "X.Y+1.0-dev" > VERSION`
    - `make -C install/kubernetes`
    - `git add .github/ Documentation/contributing/testing/ci.rst`
    - `git commit -sam "Prepare for vX.Y+1 development cycle"`
  - [ ] Merge the main PR so that the stable branch protection can be properly
        set up with the right status checks requirements.
  - [ ] Sync the `vX.Y` branch up to the commit before preparing for the `X.Y+1` development cycle.
    - `git fetch upstream && git checkout vX.Y && git merge --ff-only upstream/main~1 && git log -5`
    - `git push upstream vX.Y`
  - [ ] Protect the new stable branch with GitHub Settings [here](https://github.com/cilium/cilium/settings/branches)
      - Use the settings of the previous stable branch and main as sane defaults
  - [ ] On the `vX.Y` branch, prepare for stable release development:
    - [ ] Remove any GitHub workflows from the stable branch that are only
          relevant for the main branch (Read the following before running
          this step).
      - Remove workflows that are exclusively triggered by cron job and
        workflows triggered by `issue_comment` triggers, as they do not run on
        stable branches. These can be identified with commands like this:
        - `git rm $(git grep -B 7 'cron:' .github/ | grep '\-name:' | sed 's/-name.*//g')`
        - `git rm $(git grep -l issue_comment .github/)`
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
      - Rewrite the CODEOWNERS file and docs. Keep the team descriptions from main
        and the previous stable branch. See [97daf56221](https://github.com/cilium/cilium/commit/97daf5622197d0cdda003a3f693e6e5a61038884)
        - `sed -i '/^\//,$d' CODEOWNERS`
        - `grep -v '#' ../cilium-X.Y-1/CODEOWNERS >> CODEOWNERS`
        - `make -C Documentation update-codeowners`
      - Delete unnecessary GitHub configurations from the stable branch
        - `git rm .github/{labeler,pull_request,renovate}*`
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

## Preparation PR (run ~1 day before targeted publish date. It can be re-run multiple times.)

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

## Post Tagging (run after docker images are published)

- [ ] Go to [release workflow] and Run the workflow from "Branch: main", for
  step "4-post-release" and version for vX.Y.Z-rc.W
    - [ ] Check if the workflow was successful and check the PR opened by the
      Release bot.
- [ ] Merge PR

## Publish helm (run after docker images are published)

- [ ] Update helm charts `./release start --steps 5-publish-helm --target-version vX.Y.Z-rc.W`
- [ ] Open [chart workflow] and check if the workflow run is successful.

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
- [ ] Bump the development snapshot version in `README.rst` on the main branch
      to point to this release
- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`.
- [ ] Update the upgrade guide and [roadmap](https://github.com/cilium/cilium/blob/main/Documentation/community/roadmap.rst)
      for any features that changed status. Usually do it after the RC1, once the
      stability of features is known.

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
