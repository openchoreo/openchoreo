---
name: "Release"
about: "Checklist for creating a new release"
title: "Release: v<MAJOR>.<MINOR>.<PATCH>"
labels: "Type/Release"
type: Task
---

### Release Steps

The following checklist will guide you through the necessary steps to create a new release of OpenChoreo.
This checklist assumes you already have push access to the OpenChoreo repository.

### Prepare for Release

- [ ] Check [existing releases](https://github.com/openchoreo/openchoreo/releases) for the desired version number.
- [ ] Export the environment variables for use in subsequent steps:
    ```shell
    export MAJOR_VERSION=<MAJOR>
    export MINOR_VERSION=<MINOR>
    export PATCH_VERSION=<PATCH>
    export GIT_REMOTE=upstream # This should be the upstream remote name: github.com/openchoreo/openchoreo
    ```

### Prepare for Major/Minor Release (example: v1.4.0)

Skip these steps if you are creating a patch release (example: v1.4.1).

- [ ] Checkout the `main` branch, ensure it is up to date, and your local branch is clean:
    ```shell
    git checkout -b release-prep-v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION} ${GIT_REMOTE}/main
    ```
- [ ] Update the `VERSION` file with the new version number:
    ```shell
    echo "${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}" > VERSION
    ```
- [ ] Verify whether there are no changes to the `VERSION` file:
    ```shell
    git diff VERSION
    ```
  - [ ] If there are changes, commit, submit a PR to the `main` branch:
      ```shell
      git add VERSION
      git commit -m "Bump version to ${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}"
      ```
  - [ ] Get the PR merged into the `main` branch, wait for [Build and Test](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml) to pass
  - [ ] Reset your local branch again with the `main` branch:
      ```shell
      git fetch ${GIT_REMOTE}
      git reset --hard ${GIT_REMOTE}/main
      ```
- [ ] Create a new release branch:
    ```shell
    git checkout -b release-v${MAJOR_VERSION}.${MINOR_VERSION}
    ```
- [ ] Push the release branch to the OpenChoreo repository:
    ```shell
    git push ${GIT_REMOTE} release-v${MAJOR_VERSION}.${MINOR_VERSION}
    ```
- [ ] Wait for [Build and Test](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml) to pass on the **new release branch**.
- [ ] Update the OpenChoreo `main` branch to next major/minor version:
    ```shell
    git checkout -b release-next-v${MAJOR_VERSION}.$((MINOR_VERSION + 1)).${PATCH_VERSION} ${GIT_REMOTE}/main
    echo "${MAJOR_VERSION}.$((MINOR_VERSION + 1)).${PATCH_VERSION}" > VERSION
    git add VERSION
    git commit -m "Bump version to ${MAJOR_VERSION}.$((MINOR_VERSION + 1)).${PATCH_VERSION}"
    ```
- [ ] Submit a PR to the `main` branch and get it merged:

### Prepare for Patch Release (example: v1.4.1)

Skip these steps if you are creating a major or minor release (example: v1.4.0).

- [ ] Checkout to the release branch, ensure it is up to date, and your local branch is clean:
    ```shell
    git checkout -b release-prep-v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION} ${GIT_REMOTE}/release-v${MAJOR_VERSION}.${MINOR_VERSION}
    ```
- [ ] Update the `VERSION` file with the new version number:
    ```shell
    echo "${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}" > VERSION
    ```
- [ ] Verify whether there are no changes to the `VERSION` file:
    ```shell
    git diff VERSION
    ```
    - [ ] If there are changes, commit, submit a PR to the `release-v${MAJOR_VERSION}.${MINOR_VERSION}` branch:
        ```shell
        git add VERSION
        git commit -m "Bump version to ${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}"
        ```
    - [ ] Get the PR merged into the `release-v${MAJOR_VERSION}.${MINOR_VERSION}`, wait for [Build and Test](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml) to pass
    - [ ] Reset your local branch again with the `release-v${MAJOR_VERSION}.${MINOR_VERSION}` branch:
        ```shell
        git fetch ${GIT_REMOTE}
        git reset --hard ${GIT_REMOTE}/release-v${MAJOR_VERSION}.${MINOR_VERSION}
        ```
- [ ] Verify or wait for [Build and Test](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml) to pass on the release branch.

### Tag the Release

- [ ] Ensure that you are in the `release-prep-v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}` branch:
    ```shell
    git branch
    ```
- [ ] Create a new tag for the release:
    ```shell
    git tag -a v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION} -m "Release v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}"
    ```
- [ ] Push the tag to the OpenChoreo repository:
    ```shell
    git push ${GIT_REMOTE} v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}
    ```
- [ ] Wait for the [Release](https://github.com/openchoreo/openchoreo/actions/workflows/release.yml) to pass.
- [ ] Verify the [draft release](https://github.com/openchoreo/openchoreo/releases) created by the above workflow.
- [ ] Mark as **Latest** if this is the latest release. (If the current release is v1.3.2 while v1.4.0 exists, then skip marking as latest)
- [ ] Publish the release.
