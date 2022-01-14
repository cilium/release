---
name: Release a new minor version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.0 release'
labels: kind/release
assignees: ''

---

## Pre-release

- [ ] Announce in Cilium slack channel #launchpad: `Starting vX.Y.0 release process :ship:`
- [ ] Create a thread for that message and ping current top-hat to not merge any
      PRs until the release process is complete.
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Make sure docker is running.
- [ ] Export a `GITHUB_TOKEN` that has access to the repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Ensure that outstanding [backport PRs] are merged
- [ ] Consider building new [cilium-runtime images] and bumping the base image
      versions on this branch:
  - Modify the `FORCE_BUILD` environment value in the `images/runtime/Dockerfile` to force a rebuild.
     [Instructions](https://docs.cilium.io/en/latest/contributing/development/images/#update-cilium-builder-and-cilium-runtime-images)
- [ ] Execute `release --current-version X.Y.0 --next-dev-version X.Y.1` to automatically
  move any unresolved issues/PRs from old release project into the new
  project.
- [ ] Push a PR including the changes necessary for the new release:
  - [ ] Pull latest changes from the branch being released
  - [ ] Run `contrib/release/start-release.sh`
  - [ ] Run `Documentation/check-crd-compat-table.sh vX.Y` and if needed, follow the
        instructions.
  - [ ] `rm CHANGELOG.md`
  - [ ] Regenerate the log since the previous release with `prep-changelog.sh <last-patch-release> vX.Y.0`
  - [ ] Check and edit the `CHANGELOG.md` to ensure all PRs have proper release notes
  - [ ] Edit the `vX.Y.0-changes.txt` files locally to replace the text with "See CHANGELOG.md for more details"
  - [ ] Commit all changes with title `Prepare for release vX.Y.0`
  - [ ] Submit PR (`contrib/release/submit-release.sh`)
  - [ ] Add the 'stable' tag as part of the GitHub workflow and remove the
        'stable' tag from the last stable branch.
- [ ] Merge PR https://github.com/cilium/cilium/pull/18126
- [ ] Create and push *both* tags to GitHub (`vX.Y.0`, `X.Y.0`)
  - Pull latest branch locally and run `contrib/release/tag-release.sh`.
- [ ] Ask a maintainer to approve the build in the following links (keep the URL
      of the GitHub run to be used later):
  - [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)
  - Check if all docker images are available before announcing the release
    `make -C install/kubernetes/ check-docker-images`
- [ ] Get the image digests from the build process and make a commit and PR with
      these digests.
  - [ ] Run `contrib/release/post-release.sh` to fetch the image
        digests and submit a PR to update these, use the URL of the GitHub run here.
  - [ ] Merge PR https://github.com/cilium/cilium/pull/18136
- [ ] Update helm charts
  - [ ] Pull latest branch locally into the cilium repository.
  - [ ] Create helm charts artifacts for `vX.Y-dev` by following the
      `README.md` in the charts repo and push these changes into the
        helm repository
  - [ ] Create helm charts artifacts in [Cilium charts] repository using
        [cilium helm release tool] for the `vX.Y.0` release. Make sure the
        generated helm charts point to the commit that contains the image
        digests.
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
- [ ] Check [read the docs] configuration:
    - [ ] Set the [default version] and mark the EOL version as active, and
          hidden and configure the new minor version as active and **not**
          hidden in [active versions].
    - [ ] Update algolia configuration search in [docsearch-scraper-webhook].
- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
  - [ ] Copy the text from `digest-vX.Y.0.txt` to the end of the release text.
        This text was previously generated with
        `contrib/release/post-release.sh`, or is otherwise available in the
        GitHub workflow run that built the images.
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack (Use [@]channel for vX.Y.0)
- [ ] Update Grafana dashboards
  - [ ] Install the dashboards into a live cluster by following the
        [Grafana install] steps.
  - [ ] Export the dashboards by following the [Grafana export] guide.
        Enable the "Export for sharing externally" option during export.
  - [ ] Upload the dashboards to Grafana.com and populate the description,
        README, icons, etc. by copying them from the previous release.

## Post-release

- [ ] For new minor version update [security policy]
- [ ] Prepare post-release changes to master branch using `contrib/release/bump-readme.sh`
  - [ ] Make sure to update the `.github/maintainers-little-helper.yaml` so that
        upcoming PRs are tracked correctly for the next release.
  - [ ] Bump the master testsuite to upgrade from vX.Y branch to master
- [ ] Notify #development on Slack that deprecated features may now be removed.
- [ ] Update external tools and guides to point to the new Cilium version:
  - [ ] [kops]
  - [ ] [kubespray]
  - [ ] [network policy]
  - [ ] [cluster administration networking]
  - [ ] [cluster administration addons]


[release blockers]: https://github.com/cilium/cilium/labels/release-blocker%2FX.Y
[backport PRs]: https://github.com/cilium/cilium/pulls?q=is%3Aopen+is%3Apr+label%3Abackport%2FX.Y
[Cilium release-notes tool]: https://github.com/cilium/release
[Docker Hub]: https://hub.docker.com/orgs/cilium/repositories
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[Stable releases]: https://github.com/cilium/cilium#stable-releases
[kops]: https://github.com/kubernetes/kops/
[kubespray]: https://github.com/kubernetes-sigs/kubespray/
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/prepare_artifacts.sh
[Quick Install]: https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default.html
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/
[default version]: https://readthedocs.org/dashboard/cilium/advanced/
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[security policy]: https://github.com/cilium/cilium/security/policy
[network policy]: https://kubernetes.io/docs/tasks/administer-cluster/network-policy-provider/cilium-network-policy/
[cluster administration networking]: https://kubernetes.io/docs/concepts/cluster-administration/networking/
[cluster administration addons]: https://kubernetes.io/docs/concepts/cluster-administration/addons/
[chart workflow]: https://github.com/cilium/charts/actions/workflows/conformance-gke.yaml
[Grafana install]: https://docs.cilium.io/en/stable/gettingstarted/grafana/#install-metrics
[Grafana export]: https://grafana.com/docs/grafana/latest/dashboards/export-import/
