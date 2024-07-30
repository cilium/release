<img title="Repository logo" src=".github/assets/logo.svg" width=200px />

# Cilium release

This repository will generate changelog for cilium releases

```bash
$ make release
$ export GITHUB_TOKEN=<token_with_repo_public_access>
```

### For a x.y.z release, a.k.a patch release

```bash
$ ./release --base <base-commit>  \
            --head <head-commit>
```

Where:
 - `<base-commit>` is `x.y.z-1`
 - `<head-commit>` should be the last commit available for the `x.y` branch.

### For a x.y.0 release, a.k.a minor release

```bash
$ ./release --base <base-commit>  \
            --head <head-commit> \
            --last-stable x.y-1
```

Where:
 - `<base-commit>` can be found with `git merge-base origin/vx.y-1 origin/vx.y`
 - `<head-commit>` should be the last commit available for the `x.y` branch.
