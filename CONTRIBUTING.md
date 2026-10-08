# Contributing to OpenChoreo

Thank you for your interest in contributing to OpenChoreo! OpenChoreo is a CNCF Sandbox project, and we welcome contributions of all kinds — code, documentation, issue reports, reviews, and design discussions.

This document gives you a quick overview of the contribution process. For step-by-step instructions, follow the links to the detailed guides under [`docs/contributors/`](./docs/contributors/README.md).

## Code of Conduct

OpenChoreo follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). By participating in this project, you agree to abide by its terms. See [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Ways to Contribute

- **Report a bug or request a feature** — open an [issue](https://github.com/openchoreo/openchoreo/issues).
- **Ask a question or start a discussion** — use [GitHub Discussions](https://github.com/openchoreo/openchoreo/discussions) or join our [Slack channel](https://cloud-native.slack.com/archives/C0ABYRG1MND).
- **Submit a code or documentation change** — follow the pull request workflow described below.
- **Review pull requests** — reviews from the community are highly valued.

## Submitting Changes

1. **Fork** the repository and **clone** your fork. See [GitHub Workflow Guide](./docs/contributors/github_workflow.md) for details.
2. **Set up your development environment.** The full setup, including prerequisites, k3d cluster bootstrap, and build/test commands, is documented in the [Contribution Guide](./docs/contributors/contribute.md).
3. **Create a feature branch** off of `main`.
4. **Make your changes.** Before opening a PR:
   - Run `make lint` and `make lint-fix` to address lint issues.
   - Run `make code.gen-check` to ensure generated code is up to date (run `make code.gen` if not).
   - Run `make test` to make sure tests pass.
   - Add or update tests for your change.
5. **Sign your commits** with the [Developer Certificate of Origin (DCO)](./docs/contributors/github_workflow.md#dco-sign-off) using `git commit -s`.
6. **Open a pull request** against `main`. The PR title must follow [Conventional Commits](https://www.conventionalcommits.org/) (e.g., `feat(api): add endpoint for listing components`). Fill in the [pull request template](./.github/pull_request_template.md).
7. **Respond to review feedback.** A maintainer will review your PR and may request changes. Once approved, a maintainer will merge it.

## Reporting Security Issues

Please do **not** report security vulnerabilities through public GitHub issues. Follow the private disclosure process described in [SECURITY.md](./SECURITY.md).

## Project Governance

OpenChoreo follows a consensus-driven governance model. See [GOVERNANCE.md](./GOVERNANCE.md) for details on roles, decision-making, and how to become a maintainer. The current maintainers are listed in [MAINTAINERS.md](./MAINTAINERS.md).

## Getting Help

If you have questions about contributing, the easiest place to ask is the [`#openchoreo` Slack channel](https://cloud-native.slack.com/archives/C0ABYRG1MND) on CNCF Slack, or [GitHub Discussions](https://github.com/openchoreo/openchoreo/discussions). Maintainers and community members are happy to help you get started.

---

For deeper guides — including how to add a new CRD, add MCP tools, or cut a release — see the full [Contributor Documentation](./docs/contributors/README.md).
