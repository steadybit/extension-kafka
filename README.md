# Steadybit Extension Scaffold

This repository contains a scaffold with a sample implementation of a [Steadybit extension](https://docs.steadybit.com/integrate-with-steadybit/extensions). You may find this repository helpful…

 - [When you want to understand what Steadybit extensions are](#understanding-the-extension-mechanism).
 - [When you want to build a Steadybit extension](#for-extension-authors)

Please follow one of the links above to move to the appropriate documentation sections.

## Understanding the Extension Mechanism

One of the best ways to understand the extension mechanism is to run an extension and experiment with its APIs. We have prepared Gitpod and GitHub codespaces setups to make this as easy as possible for you.

When you click one of these buttons, you will be directed to an online editor with a locally running extension, and the file `README.http` will open. This file contains documentation and HTTP calls you can execute to learn about extensions and this specific sample implementation.

[![Open in Gitpod](https://gitpod.io/button/open-in-gitpod.svg)](http://gitpod.io/#https://github.com/steadybit/extension-scaffold/blob/main/README.http)


[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://github.com/codespaces/new?hide_repo_select=true&ref=main&repo=595972094)


## For Extension Authors

**Note:** We recommend that you [understand the extension mechanism](#understanding-the-extension-mechanism) before following these instructions.

This repository ships with everything Steadybit extensions might need:
 - Basic usage of and initialization for ActionKit, DiscoveryKit, EventKit and ExtensionKit.
 - Extension configuration support.
 - Dockerfile and Helm chart.
 - GitHub actions for building, testing and publishing Docker images and Helm charts.
 - and more.

To use this scaffold, you need to:

 1. Get a copy of this scaffold. [Use GitHub's repository template feature](https://docs.github.com/en/repositories/creating-and-managing-repositories/creating-a-repository-from-a-template), [fork the repository](https://github.com/steadybit/extension-scaffold/fork) or [download it](https://github.com/steadybit/extension-scaffold/archive/refs/heads/main.zip).
 2. Execute `make eject` within the copy to replace the readme, license etc. files with some more appropriate starting points.
 3. Delete the `.github/workflows/cla.yml` workflow or allow access to the access for CLA verification.
 4. Rename all occurrences of `extension-scaffold` to `extension-{{other name}}`
 5. Verify that the Docker and Helm installation instructions are correct in the `README.md`
 6. Create an empty branch named "gh-pages"
 7. After the first build, ensure that you make the Docker image public through `packages -> {{your package name}} -> Package settings -> Change visibility`

