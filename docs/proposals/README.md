# OpenChoreo Proposals

This directory contains design proposals for OpenChoreo. Proposals are used to document significant changes to the platform before implementation.

## When to Write a Proposal

A proposal is required for:
- New CRDs or significant changes to existing CRDs
- Major architectural changes
- Breaking changes to APIs or behavior
- New features that impact multiple components
- Changes affecting the developer experience

## Proposal Process

### 1. Create a GitHub Issue
First, create a GitHub issue describing the problem or enhancement you want to address. This issue will be assigned a number that becomes the proposal identifier.

### 2. Open a Discussion
After creating the issue, open a [GitHub Discussion under the "Proposals" category](https://github.com/openchoreo/openchoreo/discussions/categories/proposals) to:
- Present your initial design ideas
- Gather feedback from the community
- Iterate on the design based on input
- Build consensus before formal documentation

### 3. Write the Proposal
Once the discussion reaches consensus:
1. Copy the `xxxx-proposal-template.md` file
2. Rename it to `<issue-number>-<descriptive-title>.md` where:
   - `<issue-number>` is zero-padded to 4 digits (e.g., 0245)
   - `<descriptive-title>` uses kebab-case (e.g., introduce-build-plane)
   - Example: `0245-introduce-build-plane.md`
3. If your proposal includes diagrams or images:
   - Create a directory `<issue-number>-assets/` (e.g., `0245-assets/`)
   - Place all images, diagrams, and other assets in this directory
   - Reference them from your proposal using relative paths (e.g., `![Architecture](0245-assets/architecture.png)`)
4. Fill out all sections of the template
5. Reference both the GitHub issue and discussion if applicable

### 4. Submit via Pull Request
1. Create a PR with your proposal file
2. Link the PR to the original GitHub issue and discussion
3. Request reviews from relevant maintainers

### 5. Proposal Status Lifecycle
Once merged into the repository, proposals have these statuses:
- **Approved**: Proposal accepted, implementation can begin
- **Rejected**: Proposal was not accepted (kept with rejection reasoning documented)

Note: Both approved and rejected proposals remain in the repository for historical reference and to document decision rationale.

## Improving the Proposal Process

Have suggestions to improve the proposal process itself? Please open a GitHub Discussion to share your ideas and gather feedback from the community.
