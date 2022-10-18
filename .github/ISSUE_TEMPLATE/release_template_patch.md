---
name: Release a new patch version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z release'
labels: kind/release
assignees: ''

---

## Setup preparation

- [ ] Depending on your OS, make sure Docker is running
- [ ] Export a `GITHUB_TOKEN` that has access to the repository
- [ ] Make sure a setup (GPG, SSH, S/MIME) is in place for [signing tags] with Git
- [ ] Make sure the `GOPATH` environment variable is set and pointing to the relevant path
- [ ] Make sure the [Cilium helm charts][Cilium charts] and [release][Cilium release-notes tool] repositories are installed locally:
  - [ ] Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`
  - [ ] Run `git clone https://github.com/cilium/release.git "$GOPATH/src/github.com/cilium/release"`

## Pre-release

- [ ] Announce in Cilium slack channel #launchpad: `Starting vX.Y.Z release process :ship:`
- [ ] Create a thread for that message and ping current top-hat to not merge any
      PRs until the release process is complete.
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Ensure that outstanding [backport PRs] are merged
- [ ] Consider building new [cilium-runtime images] and bumping the base image
      versions on this branch:
  - [ ] Modify the `FORCE_BUILD` environment value in the
    `images/runtime/Dockerfile` to force a rebuild
    [Instructions](https://docs.cilium.io/en/latest/contributing/development/images/#update-cilium-builder-and-cilium-runtime-images).
- [ ] Execute `release --current-version X.Y.Z --next-dev-version X.Y.W` to
      automatically move any unresolved issues/PRs from old release project
      into the new project (`W` should be calculation of `Z+1`). The `release`
      binary is located in the [current repository][Cilium release-notes tool].
- [ ] Push a PR including the changes necessary for the new release:
  - [ ] Pull latest changes from the branch being released
  - [ ] Run `contrib/release/start-release.sh X.Y.Z N`, where `N` is the id of
        the GitHub project created at the previous step.
        Note that this script produces some files at the root of the Cilium
        repository, and that these files are required at a later step for
        tagging the release.
  - [ ] Commit all changes with title `Prepare for release vX.Y.Z`
  - [ ] Submit PR (`contrib/release/submit-release.sh`)
- [ ] Merge PR
- [ ] Ask a maintainer if there are any known issues that should hold up the release
- [ ] Create and push *both* tags to GitHub (`vX.Y.Z`, `X.Y.Z`)
  - [ ] Pull latest branch locally
  - [ ] Run `contrib/release/tag-release.sh`.
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)
  - [ ] Check if all docker images are available before announcing the release:
        `make -C install/kubernetes/ check-docker-images`
- [ ] Get the image digests from the build process and make a commit and PR with
      these digests.
  - [ ] Run `contrib/release/post-release.sh URL` to fetch the image
        digests and submit a PR to update these, use the `URL` of the GitHub
        run here
  - [ ] Merge PR
- [ ] Update helm charts
  - [ ] Pull latest branch locally into the cilium repository.
  - [ ] Create helm charts artifacts in [Cilium charts] repository using
        [cilium helm release tool] for both the `vX.Y.Z` release and `vX.Y`
        branch and push these changes into the helm repository. Make sure the
        generated helm charts point to the commit that contains the image
        digests.
  - [ ] Push the charts
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
  - [ ] Copy the text from `digest-vX.Y.Z.txt` to the end of the release text.
        This text was previously generated with
        `contrib/release/post-release.sh`, or is otherwise available in the
        GitHub workflow run that built the images.
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (do not use [@]channel)

## Post-release

- [ ] Prepare post-release changes to master branch using `contrib/release/bump-readme.sh`


[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Abackport%2FX.Y
[Cilium release-notes tool]: https://github.com/cilium/release
[Docker Hub]: https://hub.docker.com/orgs/cilium/repositories
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[Stable releases]: https://github.com/cilium/cilium#stable-releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/prepare_artifacts.sh
[Quick Install]: https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default.html
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/
[default version]: https://readthedocs.org/dashboard/cilium/advanced/
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[security policy]: https://github.com/cilium/cilium/security/policy
[chart workflow]: https://github.com/cilium/charts/actions/workflows/conformance-gke.yaml
