# Standardize build conditions for build workflow

**Authors**:  
@chalindukodikara

**Reviewers**:  
@Mirage20

**Created Date**:  
2025-04-07

**Status**:  
Implemented

**Related Issues/PRs**:  
https://github.com/openchoreo/openchoreo/issues/142
https://github.com/openchoreo/openchoreo/pull/149

---

## Summary
Currently, the build controller adds conditions only when a step is completed, resulting in three possible states: `InProgress`, `Succeeded`, and `Failed`. This approach makes it difficult for clients—such as the CLI and UI—to determine the full set of build workflow steps ahead of time.

As of now, the workflow consists of three steps: Clone, Build, and Push. However, additional steps may be introduced in the future. This would require client-side code changes to accommodate new steps, which is not ideal. To improve usability and make the system more extensible and predictable, we propose standardizing how workflow steps are represented in the resource status.

---

## Motivation

The current condition model only reflects steps that have already occurred, making it challenging for clients to understand the complete workflow structure. This tightly couples clients with the current set of steps, requiring changes when the workflow evolves.

By standardizing step representation:
- Clients can always expect a complete and consistent list of workflow steps.
- It eliminates the need for clients to change their logic when steps are added or removed.
- It improves maintainability and aligns with long-term goals like switching CI systems (e.g., Argo to GitHub Actions).
---

## Goals

- Define a standardized way to represent workflow/pipeline steps in the resource status.
- Update the build controller to work for the new model.

---

## Non-Goals

None

---

## Impact


- **Build Controller**: Requires changes to populate and update workflow step conditions.
- **CLI / UI**: Should be updated to rely on the standardized condition names and structure, rather than dynamically checking for specific conditions.
---


## Design

### Overview

The controller will initialize all workflow steps in the resource status with a default condition when the build starts. Each step will have four possible states: `Queued`, `InProgress`, `Succeeded`, and `Failed`.

Each step condition will use a consistent naming pattern: `Step<StepName>Succeeded`, and will include a clear `Reason`:

- **Queued**: Step is pending execution
- **InProgress**: Step is currently running
- **Succeeded**: Step completed successfully
- **Failed**: Step execution failed

To clearly differentiate these from other types of conditions, the prefix `Step` will be used.

This model ensures long-term compatibility, as new steps can be introduced without requiring client changes. It also supports future CI engine changes, as the representation remains stable.

---

### Naming Convention and Examples

| Step       | Condition Name        | State       | Reason        | Status |
|------------|-----------------------|-------------|----------------|--------|
| Clone      | `StepCloneSucceeded`  | Queued      | `Queued`       | False  |
|            |                       | InProgress  | `Progressing`  | False  |
|            |                       | Succeeded   | `Succeeded`    | True   |
|            |                       | Failed      | `Failed`       | False  |
| Build      | `StepBuildSucceeded`  | Queued      | `Queued`       | False  |
|            |                       | InProgress  | `Progressing`  | False  |
|            |                       | Succeeded   | `Succeeded`    | True   |
|            |                       | Failed      | `Failed`       | False  |
| Push       | `StepPushSucceeded`   | Queued      | `Queued`       | False  |
|            |                       | InProgress  | `Progressing`  | False  |
|            |                       | Succeeded   | `Succeeded`    | True   |
|            |                       | Failed      | `Failed`       | False  |

> **Note:** New steps introduced in the future should follow the same `Step<StepName>Succeeded` convention for consistency.
