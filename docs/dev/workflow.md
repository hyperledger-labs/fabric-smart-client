# Contribution Workflow

The Fabric Smart Client project adopts the contribution workflow developed by the [Hiero community](https://github.com/hiero-ledger/hiero-sdk-python), presented at LFDT Maintainer Days ([recording](https://www.youtube.com/watch?v=I87WCpiXOOs)).

The process is designed to:
- Reduce duplicate work and assignment conflicts.
- Give external contributors a clear path to claim and deliver work.
- Keep issues and PRs moving without requiring constant manual intervention from maintainers.

## Issue Lifecycle

### Issue Types

Every issue should have a type:

| Type      | Meaning                           |
|-----------|-----------------------------------|
| `Bug`     | Something is broken               |
| `Feature` | New feature or improvement        |
| `Task`    | A specific, bounded piece of work |

And one of the following status labels:

| Label                     | Meaning                                                            |
|---------------------------|--------------------------------------------------------------------|
| `status: awaiting triage` | New issue that needs to be reviewed and categorized by maintainers |
| `status: ready for dev`   | Fully defined and ready for a contributor to pick up               |
| `status: in progress`     | A contributor is actively working on this issue                    |

An issue without a `status: ready for dev` label is **not ready for contribution**.

### Skill System

Every issue carries a skill-level label that determines who can claim it:

| Label                     | Prerequisite                                           |
|---------------------------|--------------------------------------------------------|
| `skill: good first issue` | None — open to all (max 5 completions per contributor) |
| `skill: beginner`         | 2 completed `skill: good first issue` issues           |
| `skill: intermediate`     | 3 completed `skill: beginner` issues                   |
| `skill: advanced`         | 3 completed `skill: intermediate` issues               |

When you comment `/assign`, the bot verifies your prerequisite count. If the check fails, it posts a comment showing your current progress and links to issues you can work on first.

A contributor who has already completed any issue at a given level or higher automatically satisfies prerequisites for lower levels.

### Creating an Issue

Anyone may open an issue using the `bug`, `feature`, or `task` template. New issues start untriaged.

Maintainers and core contributors review new issues and apply `status: ready for dev` once the issue is well-defined, scoped, and accepted.

#### Parent and Child Issues

For larger efforts, a **parent issue** may be opened to capture the overall goal, with individual **child issues** that are well scoped and actionable.
Each child issue is the unit of assignment, `status: ready for dev` labeling, and PR linking.
Note that dependencies between children can be expressed using the `Marked as blocked by` relationship.

### Claiming an Issue

Contributors must claim an issue **before** opening a PR:

1. Comment `/assign` on the issue.
2. The bot checks:
    - The issue carries `status: ready for dev`.
    - The issue has a skill-level label and the contributor meets the prerequisite (see [Skill System](#skill-system)).
    - The contributor has **no more than two open assigned issues** (limit across all issues in the repository).
3. If all conditions are met, the bot assigns the contributor and confirms in a comment.
4. To release an issue voluntarily, comment `/unassign`.

Maintainers may assign any contributor directly — this bypasses all bot eligibility checks.

### Issue Inactivity

Once assigned, the bot monitors activity (comments, linked PR events):

| Threshold                                | Action                                                                                               |
|------------------------------------------|------------------------------------------------------------------------------------------------------|
| 5 days of no activity                    | Bot posts a reminder tagging the assignee                                                            |
| 7 days of no activity                    | Bot unassigns the contributor with an explanatory comment; issue becomes available for re-assignment |

Issues carrying `status: blocked` are exempt from the above timeline. Instead, the bot posts a check-in comment every 30 days asking whether the issue is still blocked. The label is applied by maintainers when progress is gated on an external factor (e.g. a dependency or upstream fix).

The 7-day unassignment window is intentionally short to keep the queue moving. For issues of higher complexity, a maintainer may manually extend the window or re-assign as appropriate.

## Pull Request (PR) Lifecycle

### Linking a PR to an Issue

Every PR opened by a contributor must:
1. Reference an open issue carrying `status: ready for dev`, using a closing keyword in the PR description (e.g. `Fixes #123`).
2. Have the PR author **assigned to the linked issue**.

PRs by maintainers, core contributors, and `dependabot` are exempt from both requirements.

If either condition is not met for a contributor's PR, the bot posts a warning comment.

### PR Labels

| Label                    | Meaning                                                                               |
|--------------------------|---------------------------------------------------------------------------------------|
| `status: needs review`   | The pull request is ready for maintainer review                                       |
| `status: needs revision` | The pull request requires changes from the author before it can be reviewed or merged |

### PR Inactivity

| Threshold                                                   | Action                                                                     |
|-------------------------------------------------------------|----------------------------------------------------------------------------|
| PR labeled `status: needs review`                           | Skipped — the bot does not flag PRs that are waiting for maintainer review |
| PR labeled `status: blocked`                                | Exempt from close/warn; bot posts a check-in comment every 30 days instead |
| 5 days of no activity (commits, review responses, comments) | Bot posts a reminder tagging the author                                    |
| 7 days of no activity                                       | Bot closes the PR with an explanatory comment                              |

When a PR is auto-closed, the linked issue is also reset: the assignee is removed and the label reverts to `status: ready for dev`. The contributor may re-claim the issue by commenting `/assign`.

### PR Checks

Standard automated checks run on every PR:

- DCO sign-off
- Unit tests
- Integration tests
- Linter / static analysis

The full set of checks is defined in the repository's CI configuration.

## Roles

| Role                 | Definition                                          |
|----------------------|-----------------------------------------------------|
| **Maintainer**       | Has `admin` or `write` permission on the repository |
| **Core contributor** | Has `triage` permission on the repository           |
| **Contributor**      | Everyone else                                       |

Maintainers and core contributors are exempt from all assignment and PR-linking rules described below. PRs from `dependabot` are also exempt. Maintainers may directly assign any contributor to any issue at any time, bypassing eligibility checks.

### Maintainer and Core Contributor Commands

| Command     | Who can use                       | Effect                                                                                                                                                                  |
|-------------|-----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/finalize` | Maintainers and core contributors | Validates that the issue has a skill-level label, updates the issue title and body to the expected format, and transitions the status label to `status: ready for dev`. |

## References

- [Hiero SDK Python — GitHub Workflows](https://github.com/hiero-ledger/hiero-sdk-python/tree/main/.github/workflows)
- [Hiero SDK C++ — GitHub Workflows](https://github.com/hiero-ledger/hiero-sdk-cpp/tree/main/.github/workflows)
- [LFDT Maintainer Days — Contribution Workflow Presentation](https://www.youtube.com/watch?v=I87WCpiXOOs)
- [Original proposal issue — fabric-x#130](https://github.com/hyperledger/fabric-x/issues/130)
