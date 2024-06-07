---
name: Release a new patch version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z release'
labels: kind/release
assignees: ''

---

## Setup preparation

- [ ] Depending on your OS, make sure Docker is running
- [ ] Export a [`GITHUB_TOKEN`](https://github.com/settings/tokens/new?description=Cilium%20Release%20Script&scopes=write:org,public_repo) that has access to the repository. Only classic tokens are
      [supported at the moment][GitHub PAT tracker], the needed scope is `public_repo`.
- [ ] Make sure a setup (GPG, SSH, S/MIME) is in place for [signing tags] with Git and install [Hub](https://github.com/github/hub).
- [ ] Make sure the `GOPATH` environment variable is set and pointing to the relevant path
- [ ] Make sure the [Cilium helm charts][Cilium charts] and [release][Cilium release-notes tool] repositories are installed locally:
  - [ ] Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`
  - [ ] Run `git clone https://github.com/cilium/release.git "$GOPATH/src/github.com/cilium/release"`
    - [ ] If you already have the repo checked out, make sure the `release` binary is up to date:

          git checkout master && git pull && make

## Pre-release

- [ ] When you create a GitHub issue using this issue template, GitHub Slack app posts a
      message in #launchpad Slack channel. Create a thread for that message and ping the
      current backporter to merge the outstanding [backport PRs] and stop merging any new
      backport PRs until the GitHub issue is closed (to avoid generating incomplete
      release notes).
- [ ] Check that there are no [release blockers] for the targeted release version.
- [ ] Ensure that outstanding [backport PRs] are merged (these may be skipped on
      case by case basis in coordination with the backporter).
- [ ] Check with @cilium/security team if there are any security fixes to include
      in the release.
- [ ] Push a PR to cilium/cilium including the changes necessary for the new release:
  - [ ] Change directory to the local copy of cilium/cilium repository and pull latest
        changes from the branch being released
  - [ ] Run `../release/internal/start-release.sh X.Y.Z`. You can ignore
        warnings about commits with no related PR found.
        Note that this script produces some files at the root of the Cilium
        repository, and that these files are required at a later step for
        tagging the release. Do not commit them.
  - [ ] Commit all changed files with title `Prepare for release vX.Y.Z`. New
        generated files, such as release-state.json and vX.Y.Z-changes.txt
        should not be committed. Tip: use `git add -p` to review the changes and
        compare them with the previous release PR.
  - [ ] Submit PR (`../release/internal/submit-release.sh`). Note that only the smoke tests
        need to succeed in order to merge this PR. Full e2e test runs are not required.
- [ ] Merge PR
- [ ] Ask a maintainer if there are any known issues that should hold up the release
- [ ] FYI, do not wait too much time between a tag is created and the helm charts are published.
      Once the tags are published the documentation will be pointing to them. Until we release
      the helm chart, users will face issues while trying out our documentation.
- [ ] Create and push *both* tags to GitHub (`vX.Y.Z`, `X.Y.Z`)
  - [ ] Pull latest `upstream/vX.Y` branch locally
  - [ ] Run `../release/internal/tag-release.sh`.
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)
  - [ ] Check if all docker images are available before announcing the release:
        `make -C install/kubernetes/ check-docker-images`
- [ ] Get the image digests from the build process and make a commit and PR with
      these digests.
  - [ ] Run `../release/internal/post-release.sh URL` to fetch the image
        digests and submit a PR to update these, use the `URL` of the GitHub
        run here
  - [ ] Get someone to review the PR. Do not trigger the full CI suite, but
        wait for the automatic checks to complete. Merge the PR.
- [ ] Update helm charts
  - [ ] Create helm charts artifacts in [Cilium charts] repository using
        [cilium helm release tool] for the `vX.Y.Z` release
        and create a PR with these changes against the charts repository.
        Note: If you handle several patch releases at once,
        create one PR per release, to make sure that the corresponding workflow
        action run for each commit. Wait for your PR to be merged before
        creating the other ones for other patch releases, or they will
        conflict.
  - [ ] Have a maintainer review and merge your PR.
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
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
- [ ] Announce the release in #general on Slack (do not use [@]channel)

## Post-release

- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`.

[GitHub PAT tracker]: https://github.com/orgs/community/discussions/36441
[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+draft%3Afalse+label%3Abackport%2FX.Y
[Cilium release-notes tool]: https://github.com/cilium/release
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/RELEASE.md
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[chart workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
