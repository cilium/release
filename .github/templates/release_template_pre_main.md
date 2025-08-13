---
name: Release a new pre-release version of Cilium from the main branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z-pre.N release'
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

## Pre-check (run ~1 week before release date)

- [ ] When you create a GitHub issue using this issue template, GitHub Slack app posts a
      message in #launchpad Slack channel. Create a thread for that message and ping the
      current backporter to merge the outstanding [backport PRs] and stop merging any new
      backport PRs until the GitHub issue is closed (to avoid generating incomplete
      release notes).
- [ ] Run `./release start --steps 1-pre-check --target-version vX.Y.Z-pre.N`
  - [ ] Check that there are no [release blockers] for the targeted release
        version.
  - [ ] Ensure that outstanding [backport PRs] are merged (these may be
        skipped on case by case basis in coordination with the backporter).
  - [ ] Check with @cilium/security team in case there are any CVEs found in the
        docker image.
  - [ ] Check with @cilium/security team if there are any security fixes to
        include in the release.

## Preparation PR (run ~1 day before release date. This step can be re-run multiple times.)

- [ ] Go to [release workflow] and Run the workflow from "Branch: main", for
  step "2-prepare-release" and version for vX.Y.Z-pre.N
  - [ ] Check if the workflow was successful and check the PR opened by the
        Release bot.
- [ ] Merge PR

## Tagging

- [ ] Ask a maintainer if there are any known issues that should hold up the release
- [ ] FYI, do not wait too much time between a tag is created and the helm charts are published.
      Once the tags are published the documentation will be pointing to them. Until we release
      the helm chart, users will face issues while trying out our documentation.
- [ ] Run `./release start --steps 3-tag --target-version vX.Y.Z-pre.N`
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)

## Post Tagging (run after docker images are published. In case of failure, this step can be re-run multiple times.)

- [ ] Check if the image build process was successful and check if the workflow
      was successful. (There won't be a PR opened for this step)

## Publish helm (run after docker images are published. In case of failure, this step can be re-run multiple times.)

- [ ] Check if the image build process was successful and check if the helm
      chart was published by the Release bot under the [Cilium helm charts][Cilium charts].
      If that did not happen, you can re-run the workflow.
      - **IN CASE THE HELM CHART WAS NOT PUSHED** Go to [release workflow]
        and Run the workflow from "Branch: main", for step "5-publish-helm" and
        version for vX.Y.Z-pre.N
- [ ] Open [Charts Workflow] and check if the workflow run is successful for vX.Y.Z-pre.N.

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
  - [ ] Check whether the new release should be set as the _latest_ release
        (via the checkbox at the bottom of the page). It should be the new
        _latest_ if the version number is strictly superior to the current
        _latest_ release displayed on GitHub (e.g. 1.11.13 does not become the
        new latest release over 1.12.5, but version 1.12.6 will).
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (do not use [@]channel).
      See below for templates.
- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`.

---

## Slack example text templates

### Patch releases

```
:confetti_ball: :cilium-radiant: Release Announcement :cilium-radiant::confetti_ball:

Cilium vX.Y.Z-pre.N, vA.B.C, and vD.E.F have been released. Thanks all for your contributions! Please see the release notes below for details :cilium-gopher:

vX.Y.Z-pre.N: https://github.com/cilium/cilium/releases/tag/vX.Y.Z-pre.N
vA.B.C: https://github.com/cilium/cilium/releases/tag/vA.B.C
vD.E.F: https://github.com/cilium/cilium/releases/tag/vD.E.F
```

### First pre-release

```
:cilium-new: *Cilium vX.Y.Z-pre.N has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-pre.N

This is the first monthly snapshot for the vX.Y development cycle. There are [vX.Y.Z-pre.N OSS docs](https://docs.cilium.io/en/vX.Y.Z-pre.N) available if you want to pull this version & try it out.
```

### Subsequent pre-/rc- releases

```
*Announcement* :tada: :tada:

:cilium-new: *Cilium vX.Y.Z-pre.N has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-pre.N

Thank you for the testing and contributing to the previous pre-releases. There are [vX.Y.Z-pre.N OSS docs](https://docs.cilium.io/en/vX.Y.Z-pre.N) available if you want to pull this version & try it out.
```

[active versions]: https://readthedocs.org/projects/cilium/versions/?version_filter=vX.Y
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[release workflow]: https://github.com/cilium/cilium/actions/workflows/release.yaml
[GitHub PAT tracker]: https://github.com/orgs/community/discussions/36441
[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+draft%3Afalse+label%3Abackport%2FX.Y
[Cilium release-notes tool]: https://github.com/cilium/release
[Cilium charts]: https://github.com/cilium/charts
[Charts Workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/RELEASE.md
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[chart workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
[read the docs]: https://readthedocs.org/projects/cilium/
