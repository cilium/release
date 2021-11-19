---
name: Release a new RC version of Cilium from master branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z-rcW release'
labels: kind/release
assignees: ''

---

## Pre-release

- [ ] Announce in Cilium slack channel #launchpad: `Starting vX.Y.Z-rcW release process :ship:`
- [ ] Create a thread for that message and ping current top-hat to not merge any
      PRs until the release process is complete.
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Make sure docker is running.
- [ ] Export a `GITHUB_TOKEN` that has access to the repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Consider building new [cilium-runtime images] and bumping the base image
      versions on this branch:
    - Modify the `FORCE_BUILD` environment value in the `images/runtime/Dockerfile` to force a rebuild.
      [Instructions](https://docs.cilium.io/en/latest/contributing/development/images/#update-cilium-builder-and-cilium-runtime-images)
- [ ] Push a PR including the changes necessary for the new release:
  - [ ] Run `./contrib/release/start-release.sh vX.Y.Z-rcW`
  - [ ] Run `Documentation/check-crd-compat-table.sh vX.Y`
    - [ ] Check the modified file(s) in `Documentation` as it will be necessary
          to fix them manually. Add a new line for this RC and remove
          unsupported versions.
  - [ ] Fix any duplicate `AUTHORS` entries and verify if it is possible to
        get the real names instead of GitHub usernames.
  - [ ] Commit the `AUTHORS` as well as the documentation files changed by the
        previous step with title `update AUTHORS and Documentation`.
  - [ ] Add the generated `CHANGELOG.md` file and commit all remaining changes
        with the title `Prepare for release vX.Y.Z-rcW`
  - [ ] Submit PR (`contrib/release/submit-release.sh`)
  - [ ] Allow the CI to sanity-check the PR (GitHub actions are enough) and get review
  - [ ] Revert the release commit and re-push
- [ ] Merge PR
- [ ] Ping current top-hat that PRs can be merged again.
- [ ] Create and push *both* tags to GitHub (`vX.Y.Z-rcW`, `X.Y.Z-rcW`)
  - Pull latest branch locally.
  - Check out the commit before the revert and run `contrib/release/tag-release.sh`
    against that commit.
- [ ] Ask a maintainer to approve the build in the following links:
  - [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)
  - Check if all docker images are available before announcing the release
    `make -C install/kubernetes/ check-docker-images`
- [ ] Update helm charts
  - [ ] Create helm charts artifacts in [Cilium charts] repository using
        [cilium helm release tool] for the `vX.Y.Z-rcW` release and push
        these changes into the helm repository. Make sure the generated helm
        charts point to the commit that was tagged.
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
- [ ] Check [read the docs] configuration:
    - [ ] Set a new build as active and hidden in [active versions].
    - [ ] Deactivate previous RCs.
    - [ ] Update algolia configuration search in [docsearch-scraper-webhook].
      - Update the versions in `docsearch.config.json`, commit them and push a trigger the workflow [here](https://github.com/cilium/docsearch-scraper-webhook/actions/workflows/update-algolia-index.yaml)
- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
  - [ ] Mark the checkbox of "This is a pre-release"
  - [ ] Publish the release
- [ ] Announce the release in #general on Slack.
Text template for the first RC:
```
*Announcement* :tada: :tada:

:cilium-new: *Cilium release candidate vX.Y.Z-rcW has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-rcW

This kicks off the release train that leads us towards vX.Y final version in the coming weeks. There are [vX.Y.Z-rcW OSS docs](https://docs.cilium.io/en/vX.Y.Z-rcW) available if you want to pull this version & try it out.
```
Text template for the next RCs:
```
*Announcement* :tada: :tada:

:cilium-new: *Cilium release candidate vX.Y.Z-rcW has been released:*
https://github.com/cilium/cilium/releases/tag/vX.Y.Z-rcW

Thank you for the testing and contributing to the previous RC. There are [vX.Y.Z-rcW OSS docs](https://docs.cilium.io/en/vX.Y.Z-rcW) available if you want to pull this version & try it out.
```

[Cilium release-notes tool]: https://github.com/cilium/release
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/prepare_artifacts.sh
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/?version_filter=vX.Y.Z-rcW
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[chart workflow]: https://github.com/cilium/charts/actions/workflows/conformance-gke.yaml