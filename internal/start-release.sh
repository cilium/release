#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

DIR=$(dirname $(readlink -ne $BASH_SOURCE))
source $DIR/lib/k8s-common.sh
source $DIR/lib/common.sh

VERSION_GLOB='v[0-9]*\.[0-9]*\.[0-9]*'
BRANCH_REGEX='s/\(v[0-9]*\.[0-9]*\).*/\1/'
REMOTE="$(get_remote)"

usage() {
    logecho "usage: $0 <VERSION> [OLD-BRANCH]"
    logecho "VERSION    Target release version (format: X.Y.Z)"
    logecho "OLD-BRANCH Branch of the previous release version if VERSION is "
    logecho "           a new minor version"
    logecho
    logecho "--help     Print this help message"
}

handle_args() {
    if [ "$#" -gt 2 ]; then
        usage 2>&1
        common::exit 1
    fi

    if [[ "$1" = "--help" ]] || [[ "$1" = "-h" ]]; then
        usage
        common::exit 0
    fi

    if ! echo "$1" | grep -q "$RELEASE_REGEX"; then
        usage 2>&1
        common::exit 1 "Invalid VERSION ARG \"$1\"; $RELEASE_FORMAT_MSG"
    fi

    if [ "$#" -eq 2 ] && ! echo "$2" | grep -q "[0-9]\+\.[0-9]\+"; then
        usage 2>&1
        common::exit 1 "Invalid OLD-BRANCH ARG \"$2\"; Expected X.Y"
    fi

    if [[ ! -e VERSION ]]; then
        common::exit 1 "VERSION file not found. Is this directory a Cilium repository?"
    fi

    if [[ "$(git status -s | grep -v "^??" | wc -l)" -gt 0 ]]; then
        git status -s | grep -v "^??"
        common::exit 1 "Unmerged changes in tree prevent preparing release PR."
    fi

    if ! gh auth status >/dev/null; then
        common::exit 1 "Failed to authenticate with GitHub"
    fi
}

main() {
    handle_args "$@"

    local ersion="$(echo $1 | sed 's/^v//')"
    local version="v$ersion"
    local branch="$(get_branch_from_version $REMOTE $version)"
    local old_branch="$2"
    local old_version=""

    git fetch -q $REMOTE
    if [ "$branch" = "main" ]; then
        git checkout -b pr/prepare-$version $REMOTE/$branch
        if ! version_is_prerelease "$version"; then
            old_version="$(git tag -l "$VERSION_GLOB" | grep -v 'pre\|rc\|snapshot' | sort -V | tail -n 1)"
        else
            old_version="$(git tag -l "$VERSION_GLOB" | sort -V | tail -n 1)"
        fi
    else
        git checkout -b pr/prepare-$version $REMOTE/$branch
        old_version="$(cat VERSION)"
    fi

    logecho "Updating VERSION, AUTHORS.md, helm templates"
    echo $ersion > VERSION
    sed -i 's/"[^"]*"/""/g' install/kubernetes/Makefile.digests
    logrun make RELEASE=yes -C install/kubernetes all USE_DIGESTS=false
    if grep -q update-helm-values Documentation/Makefile; then
        logrun make -C Documentation update-helm-values
    fi
    logrun make update-authors

    target_branch=$(echo "$version" | sed "$BRANCH_REGEX")
    if ! git ls-remote --exit-code --heads $REMOTE $target_branch; then
        target_branch=$(echo "$old_version" | sed "$BRANCH_REGEX")
        old_branch="$target_branch"
    fi
    Documentation/check-crd-compat-table.sh "$target_branch" --update
    if [ "${old_branch}" != "" ]; then
      $DIR/prep-changelog.sh "$old_version" "$version" "$old_branch"
    else
      $DIR/prep-changelog.sh "$old_version" "$version"
    fi

    logecho "Next steps:"
    logecho "* Check all changes and add to a new commit"
    logecho "  * If this is a prerelease, create a revert commit"
    logecho "* Push the PR to Github for review ('submit-release.sh')"
    logecho "* (After PR merge) Use 'tag-release.sh' to prepare tags/release"
}

main "$@"
