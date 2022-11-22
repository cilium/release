---
name: Release a new RC version of Cilium from a stable branch
about: Create a checklist for an upcoming release
title: 'vX.Y.Z-rcW release'
labels: kind/release
assignees: ''

---

## Setup preparation

- [ ] Depending on your OS, make sure Docker is running
- [ ] Export a `GITHUB_TOKEN` that has access to the repository
- [ ] Make sure a setup (GPG, SSH, S/MIME) is in place for [signing tags] with Git
- [ ] Make sure the `GOPATH` environment variable is set and pointing to the relevant path
- [ ] Make sure the [Cilium helm charts][Cilium charts] repository is installed locally:
  - [ ] Run `git clone https://github.com/cilium/charts.git "$GOPATH/src/github.com/cilium/charts"`

## Pre-release


- [ ] Announce in Cilium slack channel #launchpad: `Starting vX.Y.Z-rcW release process :ship:`
- [ ] Create a thread for that message and ping current top-hat to not merge any
  PRs until the release process is complete.
- [ ] Change directory to the local copy of Cilium repository.
- [ ] Check that there are no [release blockers] for the targeted release version
- [ ] Ensure that outstanding [backport PRs] are merged
- [ ] Consider building new [cilium-runtime images] and bumping the base image
      versions on this branch:
  - [ ] Modify the `FORCE_BUILD` environment value in the
        `images/runtime/Dockerfile` to force a rebuild
        ([Instructions](https://docs.cilium.io/en/latest/contributing/development/images/#update-cilium-builder-and-cilium-runtime-images))
- [ ] If stable branch is not created yet. Run:
  - `git fetch origin && git checkout -b origin/vX.Y origin/master`
  - [ ] Update the VERSION file with the last RC released for this stable version
    - `echo "X.Y.Z-rcW-1" > VERSION`
  - [ ] Commit that change into the `vX.Y` branch with the title `Update vX.Y VERSION`
  - [ ] Push that branch and the commit with the updated `VERSION` to GitHub:
    - `git push origin vX.Y`
  - [ ] Create a new GH project for the `X.Y+1.0` version and keep the project ID
        to update the MLH configuration in the next step.
  - [ ] On the master branch, create a PR with a change in the `VERSION` file to
        start the next development cycle as well as creating the necessary GH
        workflows (conformance tests, dependabot configuration, MLH configuration,
        see [4d52791d27](https://github.com/cilium/cilium/commit/4d52791d27de836d2fb1190230769e32ad813c25)
        for reference):
    - [ ] Create the specific GH workflow that are only triggered via comment in
          the master branch for the stable version going to be released.
    - [ ] Remove all GH workflow that are only triggered via comment from the
          stable branch that is going to be released.
    - [ ] Adjust `maintainers-little-helper.yaml` accordingly the new stable
          branch.
    - `echo "X.Y.90" > VERSION`
    - `make -C install/kubernetes`
    - `git add .github/ Documentation/contributing/testing/ci.rst`
    - `git commit -sam "Prepare for X.Y+1 development cycle"`
  - [ ] Merge the master PR so that the next step can be properly done with the
        right status checks requirements.
  - [ ] Protect the new stable branch with GitHub Settings [here](https://github.com/cilium/cilium/settings/branches)
      - Use the settings of the previous stable branch and master as sane defaults
      - Some of the branch-specific status checks might only appear after they
        were triggered at least one time in the stable branch.
  - [ ] Remove any GitHub workflows from the stable branch that are only
        relevant for the master branch.
    - Copy-paste the `.github` directory from the previous stable branch and
      manually check the diff between the files from the current stable branch
      and make changes accordingly. Heuristically this means removing all GH
      workflows that are triggered by `issue_comment` and the ones that
      exclusively cron jobs, and modify the remaining workflows to be specific
      for the stable branch. See [8bbae9cb43](https://github.com/cilium/cilium/commit/8bbae9cb4323bf3dd94936e355b0c2aad96d0df8)
      for reference.
    - Remove the `labels-unset` field from the MLH configuration and add
      the `auto-label` field. See [5b4934284d](https://github.com/cilium/cilium/commit/5b4934284dd525399aacec17c137811df9cf0f8b)
      for reference.
    - Rewrite the CODEOWNERS file. See [97daf56221](https://github.com/cilium/cilium/commit/97daf5622197d0cdda003a3f693e6e5a61038884)
  - [ ] Ping CI team to prepare all necessary jenkins configuration for this
        branch.
  - [ ] Push a PR with those changes:
    - `git commit -sam "Prepare v1.12 stable branch`
- [ ] Push a PR including the changes necessary for the new release:
  - [ ] Run `./contrib/release/start-release.sh vX.Y.Z-rcW`
        Note that this script produces some files at the root of the Cilium
        repository, and that these files are required at a later step for
        tagging the release.
  - [ ] Check the modified schema file(s) in `Documentation` as it will be
        necessary to fix them manually. Add a new line for this RC and remove
        unsupported versions.
  - [ ] Fix any duplicate `AUTHORS` entries and verify if it is possible to
        get the real names instead of GitHub usernames.
  - [ ] Add the generated `CHANGELOG.md` file and commit all remaining changes
        with the title `Prepare for release vX.Y.Z-rcW`
  - [ ] Submit PR (`contrib/release/submit-release.sh`)
- [ ] Merge PR
- [ ] Ping current top-hat that PRs can be merged again.
- [ ] Create and push *both* tags to GitHub (`vX.Y.Z-rcW`, `X.Y.Z-rcW`)
  - Pull latest branch locally.
  - Check out the commit before the revert and run `contrib/release/tag-release.sh`
    against that commit.
- [ ] Ask a maintainer to approve the build in the following link (keep the URL
      of the GitHub run to be used later):
      [Cilium Image Release builds](https://github.com/cilium/cilium/actions?query=workflow:%22Image+Release+Build%22)
  - [ ] Check if all docker images are available before announcing the release:
        `make -C install/kubernetes/ RELEASE=yes CILIUM_BRANCH=vX.Y check-docker-images`
- [ ] Get the image digests from the build process and make a commit and PR with
      these digests.
  - [ ] Run `contrib/release/post-release.sh URL` to fetch the image
        digests and submit a PR to update these, use the `URL` of the GitHub
        run here
  - [ ] Merge PR
- [ ] Update helm charts
  - [ ] Pull latest branch locally into the cilium repository.
  - [ ] Create helm charts artifacts in [Cilium charts] repository using
        [cilium helm release tool] for the `vX.Y.Z` release and push these
        changes into the helm repository. Make sure the generated helm charts
        point to the commit that contains the image digests.
  - [ ] Check the output of the [chart workflow] and see if the test was
        successful.
- [ ] Check [read the docs] configuration:
    - [ ] Set a new build as active and hidden in [active versions].
    - [ ] Deactivate previous RCs.
    - [ ] Update algolia configuration search in [docsearch-scraper-webhook].
      - Update the versions in `docsearch.config.json`, commit them and push a trigger the workflow [here](https://github.com/cilium/docsearch-scraper-webhook/actions/workflows/update-algolia-index.yaml)
- [ ] Check draft release from [releases] page
  - [ ] Update the text at the top with 2-3 highlights of the release
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

[signing tags]: https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-tags
[Cilium charts]: https://github.com/cilium/charts
[releases]: https://github.com/cilium/cilium/releases
[cilium helm release tool]: https://github.com/cilium/charts/blob/master/prepare_artifacts.sh
[cilium-runtime images]: https://quay.io/repository/cilium/cilium-runtime
[read the docs]: https://readthedocs.org/projects/cilium/
[active versions]: https://readthedocs.org/projects/cilium/versions/?version_filter=vX.Y.Z-rcW
[docsearch-scraper-webhook]: https://github.com/cilium/docsearch-scraper-webhook
[chart workflow]: https://github.com/cilium/charts/actions/workflows/conformance-gke.yaml
