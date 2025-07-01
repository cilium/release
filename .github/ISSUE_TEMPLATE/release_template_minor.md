---
name: Release a new minor version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.0 release'
labels: kind/release
assignees: ''

---

## Setup preparation

- Ensure Docker is installed and running
- Ensure a setup is in place for [signing tags] with Git (GPG, SSH, S/MIME)
- Install [gh](https://cli.github.com).
- Make sure the [Cilium helm charts][Cilium charts] and [release][Cilium release-notes tool] repositories are installed locally:
  - Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`
  - Run `git clone https://github.com/cilium/release.git "$GOPATH/src/github.com/cilium/release"`
- [ ] Make sure the `release` binary is up to date:
      `git checkout master && git pull && make`
- Read the documentation of `release start --help` tool to understand what
  each automated step does.

## Pre-check (run ~1 week before targeted publish date)

- [ ] When you create a GitHub issue using this issue template, GitHub Slack app posts a
      message in #launchpad Slack channel. Create a thread for that message and ping the
      current backporter to merge the outstanding [backport PRs] and stop merging any new
      backport PRs until the GitHub issue is closed (to avoid generating incomplete
      release notes).
- [ ] Run `./release start --steps 1-pre-check --target-version vX.Y.0`
  - [ ] Check that there are no [release blockers] for the targeted release
        version.
  - [ ] Ensure that outstanding [backport PRs] are merged (these may be
        skipped on case by case basis in coordination with the backporter).

## Preparation PR (run ~1 day before targeted publish date. It can be re-run multiple times.)

- [ ] Run `./release start --steps 2-prepare-release --target-version vX.Y.0`
- [ ] Manually fix the following:
  - [ ] Add the 'stable' tag as part of the GitHub workflow and remove the
        'stable' tag from the last stable branch (vX.Y-1).
- [ ] Manually submit a PR that removes the 'stable' tag from the last stable 
      branch.
- [ ] Merge PR

## Tagging

- [ ] Ask a maintainer if there are any known issues that should hold up the release
- [ ] Run `./release start --steps 3-tag --target-version vX.Y.0`
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)

## Post Tagging (run after docker images are published)

- [ ] Go to [release workflow] and Run the workflow from "Branch: main", for
  step "4-post-release" and version for vX.Y.0
    - [ ] Check if the workflow was successful and check the PR opened by the
      Release bot.
- [ ] Merge PR

## Publish helm (run after docker images are published)

- [ ] Update helm charts `./release start --steps 5-publish-helm --target-version vX.Y.0`
- [ ] Open [chart workflow] and check if the workflow run is successful.

## Publish docs

- [ ] Check [read the docs] configuration:
  - [ ] Set a new build as active and hidden in [active versions].
  - [ ] Set the [default version] and mark the EOL version as active, and
        hidden and configure the new minor version as active and **not**
        hidden in [active versions].
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
        _Set as the latest release_ and _Create a discussion for this release_ in
        the "Announcements" category.
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (Use [@]channel for vX.Y.0)
- [ ] For new minor version update [security policy]
- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`.
  - [ ] `echo vX.Y.0 > stable.txt`.
  - [ ] `echo '{"results":[{"slug":"vX.Y"}]}' > Documentation/_static/stable-version.json`.
  - [ ] Commit / amend the commit to add all of the changes above and push the PR.
  - [ ] Merge the post-release PR.

[release workflow]: https://github.com/cilium/cilium/actions/workflows/release.yaml
[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Abackport%2FX.Y
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/RELEASE.md
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/?version_filter=vX.Y
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[chart workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
[Cilium charts]: https://github.com/cilium/charts
[default version]: https://readthedocs.org/dashboard/cilium/advanced/
[security policy]: https://github.com/cilium/cilium/security/policy
