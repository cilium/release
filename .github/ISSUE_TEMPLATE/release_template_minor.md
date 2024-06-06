---
name: Release a new minor version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.0 release'
labels: kind/release
assignees: ''

---

## Setup preparation

- [ ] Depending on your OS, make sure Docker is running
- [ ] Export a [`GITHUB_TOKEN`](https://github.com/settings/tokens/new?description=Cilium%20Release%20Script&scopes=write:org,public_repo) that has access to the repository
- [ ] Make sure a setup (GPG, SSH, S/MIME) is in place for [signing tags] with Git
- [ ] Make sure the `GOPATH` environment variable is set and pointing to the relevant path
- [ ] Make sure the [Cilium helm charts][Cilium charts] and [release][Cilium release-notes tool] repositories are installed locally:
  - [ ] Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`
  - [ ] Run `git clone https://github.com/cilium/release.git "$GOPATH/src/github.com/cilium/release"`
    - [ ] If you already have the repo checked out, make sure the `release` binary is up to date:

          git checkout master && git pull && make

## Pre-release

- [ ] Announce in Cilium slack channel #launchpad: `Starting vX.Y.0 release process :ship:`
- [ ] Create a thread for that message and ping current top-hat to not merge any
      PRs until the release process is complete.
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Ensure that outstanding [backport PRs] are merged
- [ ] Check with @cilium/security team if there are any security fixes to include
      in the release.
- [ ] Execute `release --current-version X.Y.0 --next-dev-version X.Y.1` to
      automatically move any unresolved issues/PRs from old release project
      into the new project. The `release` binary is located in the
      [current repository][Cilium release-notes tool].
- [ ] Push a PR including the changes necessary for the new release:
  - [ ] Pull latest changes from the branch being released
  - [ ] The next step will generate a `CHANGELOG.md` that will not be correct.
        That is expected, and it is fixed with a follow-up step. Don't worry.
  - [ ] Run `../release/internal/start-release.sh X.Y.0 <GH-PROJECT> X.Y-1`
        Note that this script produces some files at the root of the Cilium
        repository, and that these files are required at a later step for
        tagging the release.
  - [ ] `rm CHANGELOG.md`
  - [ ] Regenerate the log since the previous release with `prep-changelog.sh <last-patch-release> vX.Y.0`
  - [ ] Check and edit the `CHANGELOG.md` to ensure all PRs have proper release notes
  - [ ] Edit the `vX.Y.0-changes.txt` files locally to replace the text with "See CHANGELOG.md for more details"
  - [ ] Add the 'stable' tag as part of the GitHub workflow and remove the
        'stable' tag from the last stable branch.
  - [ ] Commit all changes with title `Prepare for release vX.Y.0`
  - [ ] Submit PR (`../release/internal/submit-release.sh`)
  - [ ] Submit a PR that removes the 'stable' tag from the last stable branch.
- [ ] Merge PR
- [ ] Create and push *both* tags to GitHub (`vX.Y.0`, `X.Y.0`)
  - [ ] Pull latest branch locally and run `../release/internal/tag-release.sh`.
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
        [cilium helm release tool] for the `vX.Y.0` release.
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
- [ ] Check [read the docs] configuration:
    - [ ] Set the [default version] and mark the EOL version as active, and
          hidden and configure the new minor version as active and **not**
          hidden in [active versions].
    - [ ] Update algolia configuration search in [docsearch-scraper-webhook].
- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (Use [@]channel for vX.Y.0)
- [ ] Update Grafana dashboards

## Post-release

- [ ] Update the upgrade guide and [roadmap](https://github.com/cilium/cilium/blob/main/Documentation/community/roadmap.rst) for any features that changed status.
- [ ] For new minor version update [security policy]
- [ ] Prepare post-release changes to main branch using `../release/internal/bump-readme.sh`
  - [ ] Make sure to update the `.github/maintainers-little-helper.yaml` so that
        upcoming PRs are tracked correctly for the next release.
  - [ ] Bump the main testsuite to upgrade from vX.Y branch to main
  - [ ] `echo X.Y.0 > stable.txt`.
  - [ ] `echo '{"results":[{"slug":"vX.Y"}]}' > Documentation/_static/stable-version.json`.
  - [ ] Commit / amend the commit to add all of the changes above and push the PR.
  - [ ] Merge the post-release PR.
- [ ] Notify #development on Slack that deprecated features may now be removed.
- [ ] This is the list of links for known external installers that depend on
      the release process. Ideally, work toward updating external tools and
      guides to point to the new Cilium version. If you find where to submit
      the update, please add the relevant links to this template.
  - [kops]
  - [kubespray]
  - [network policy]
  - [cluster administration networking]
  - [cluster administration addons]


[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Abackport%2FX.Y
[Cilium release-notes tool]: https://github.com/cilium/release
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[kops]: https://github.com/kubernetes/kops/
[kubespray]: https://github.com/kubernetes-sigs/kubespray/
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/RELEASE.md
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/
[default version]: https://readthedocs.org/dashboard/cilium/advanced/
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[security policy]: https://github.com/cilium/cilium/security/policy
[network policy]: https://kubernetes.io/docs/tasks/administer-cluster/network-policy-provider/cilium-network-policy/
[cluster administration networking]: https://kubernetes.io/docs/concepts/cluster-administration/networking/
[cluster administration addons]: https://kubernetes.io/docs/concepts/cluster-administration/addons/
[chart workflow]: https://github.com/cilium/charts/actions/workflows/validate-cilium-chart.yaml
