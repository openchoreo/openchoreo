# Contributing to OpenChoreo

Welcome! We're glad you're interested in contributing to OpenChoreo. Whether you're fixing a bug, improving documentation, or proposing a feature, every contribution makes a difference.

OpenChoreo spans several repositories. This page helps you find the right place to make your change and points to the detailed guides that already exist.

## Find your first contribution

New to the project? Browse the open [good first issues](https://github.com/openchoreo/openchoreo/issues?q=is%3Aissue%20state%3Aopen%20label%3A%22good%20first%20issue%22), then introduce yourself on the issue before starting work.

## Where should I contribute?

| Repository | What belongs there |
| --- | --- |
| [openchoreo/openchoreo](https://github.com/openchoreo/openchoreo) | Core APIs and CRDs, controllers, API and MCP servers, `occ`, built-in agents, Helm charts, and platform samples |
| [openchoreo/backstage-plugins](https://github.com/openchoreo/backstage-plugins) | The Backstage portal, including frontend and backend plugins, catalog integration, and scaffolding |
| [openchoreo/community-modules](https://github.com/openchoreo/community-modules) | Optional integrations such as gateways, observability backends, workflows, and GitOps modules |
| [openchoreo/openchoreo.github.io](https://github.com/openchoreo/openchoreo.github.io) | The OpenChoreo website, public documentation, blog, and marketplace |
| [openchoreo/sample-workloads](https://github.com/openchoreo/sample-workloads) | Application source code used by build-from-source samples |
| [openchoreo/sample-gitops](https://github.com/openchoreo/sample-gitops) | The reference GitOps repository, including Flux configuration, platform resources, and GitOps workflows |
| [openchoreo/skills](https://github.com/openchoreo/skills) | Agent skills for installing, developing on, and operating OpenChoreo |

Changes to a core API or resource may affect the portal, documentation, samples, modules, or skills. When work spans repositories, link the related issues and pull requests.

## Start here

1. Use the [development guide](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/contribute.md) to set up the core repository and learn its build, test, and generation commands.
2. Before submitting a change, follow the [GitHub workflow](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/github_workflow.md) and review the [AI policy](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/AI-POLICY.md).
3. If your change introduces a new CRD, breaks a public contract, or affects multiple parts of the platform, open a [proposal in GitHub Discussions](https://github.com/openchoreo/openchoreo/discussions/categories/proposals) before starting implementation.

## Go deeper

<details>
<summary>Understand the platform</summary>

- [Architecture overview](https://github.com/openchoreo/openchoreo#how-does-openchoreo-work)
- [Resource kind reference](https://github.com/openchoreo/openchoreo/blob/main/docs/resource-kind-reference-guide.md)

</details>

<details>
<summary>Extend the core</summary>

- [Add a CRD](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/adding-new-crd.md)
- [Add an MCP tool](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/adding-new-mcp-tools.md)
- [Add a build engine](https://github.com/openchoreo/openchoreo/blob/main/docs/contributors/build-engines.md)
- [Templating reference](https://github.com/openchoreo/openchoreo/tree/main/docs/templating)

</details>

<details>
<summary>Run and verify OpenChoreo</summary>

- [Local k3d environments](https://github.com/openchoreo/openchoreo/tree/main/install/k3d)
- [Test suites](https://github.com/openchoreo/openchoreo/blob/main/test/README.md)
- [Samples](https://github.com/openchoreo/openchoreo/blob/main/samples/README.md)

</details>

## Before starting a change

1. Find the repository that owns the behavior or contract.
2. Search the project's [issues](https://github.com/openchoreo/openchoreo/issues) and [discussions](https://github.com/openchoreo/openchoreo/discussions) before starting a new design.
3. Verify current behavior against the code and API definitions.
4. Link related issues and pull requests when a change affects more than one repository.

For help, join the [OpenChoreo CNCF Slack channel](https://cloud-native.slack.com/archives/C0ABYRG1MND) or start a [GitHub Discussion](https://github.com/openchoreo/openchoreo/discussions).
