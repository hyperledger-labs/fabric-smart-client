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

Maintainers may also apply a priority label — `priority: critical`, `priority: high`,
`priority: medium`, or `priority: low` — at their discretion. Priority is otherwise
optional: an issue with no priority label is perfectly normal. It becomes required only
when finalizing through `/finalize`, which refuses to run without exactly one.

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

#### Writing a Good Description

The template's fields *are* the expected structure: keep the `### ` headings it
generates and write your content underneath them.

> [!IMPORTANT]
> `/finalize` rebuilds the issue body by parsing `### ` headings, and it discards
> anything written **above the first one**. Do not open with a preamble, and do not
> demote the headings to `##` or `####` — only `### ` is recognized.

Keep each section brief and concrete: contributors read these to decide whether they
can pick the issue up, and maintainers to decide whether it is ready. Relevant file
paths, package names, and symbol names are worth more than narrative.

| Type      | What the description has to establish                                                       |
|-----------|---------------------------------------------------------------------------------------------|
| `Bug`     | What you observed vs. what you expected, numbered steps that reproduce it, and the environment |
| `Feature` | The problem first, then the proposed API or behavior; alternatives if you weighed any        |
| `Task`    | The current state, the target state, and the implementation steps as a checklist             |

Leave `Additional Information` empty when you have nothing to add — `_No response_` and
`Optional.` are both treated as empty, so filler costs a reader time without telling
them anything.

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

### Writing a PR Description

There is no PR template; this is the convention:

1. **What and why** — a short paragraph. The diff already shows *what* changed, so the
   description carries the reasoning and whatever is not deducible from the code.
2. **The issue link** — `Fixes #123`, in the body (see [above](#linking-a-pr-to-an-issue)).
3. **How it was verified** — the commands you ran, or the test that now covers it.
4. **Notes for reviewers** — optional: where to look first, what is deliberately out of scope.

Keep it structured and short. A file-by-file walkthrough only duplicates the diff; omit
it. The PR title should read like the final commit subject below.

### Commit Hygiene

Every PR is merged as a **single squashed commit**, and the branch reaches that state in
four steps:

1. **Develop locally** on a branch. Commit as often as you find useful — WIP commits are
   fine here. You can push the branch and open a **draft PR** at any point, to get CI
   running and make the work visible; the history does not have to be tidy yet.
2. **Before requesting review**, squash the branch into one commit with a meaningful
   message, and take the PR out of draft.
3. **During review**, address comments with fixup commits, so reviewers see only what
   changed since their last pass.
4. **Once the PR is approved**, autosquash the fixups and force-push, leaving the single
   commit that gets merged.

Squashing locally rather than through GitHub keeps the message that lands on `main` the
one you wrote, instead of a concatenation of every subject on the branch.

#### The commit message

The message describes what the PR changes — not the path you took to get there.
`addressed reviewer comments`, `fix typo`, and `wip` are useful while a branch is under
review, but none of them belong in the merged message. Use the
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) format already in
use on `main`, appending `!` to the scope for a breaking change:

```
<type>(<scope>): <short imperative summary>
```

```
fix(fabricx): report the status of an already-final transaction
test(integration): run libp2p in four targets instead of nine
refactor(storage)!: replace squirrel with an internal SQL builder
```

#### Step 2: squash before requesting review

Rebase onto the current `main`, collapse the branch into one commit, and write the
message:

```bash
git rebase main -S
git reset --soft main
git commit -s -S
git push --force-with-lease
```

A branch that genuinely contains two independent changes is two PRs, not two commits.

#### Step 3: fixups during review

Address review comments with fixup commits rather than by amending and force-pushing.
Reviewers then see exactly what changed since their last pass, and their comment threads
stay anchored to the lines they were written against:

```bash
git commit --fixup <sha> -s -S
git push
```

Keep `-s` so the DCO check stays green while the fixups are on the branch.

The **Git Checks / block-fixup** job fails for as long as `fixup!` commits are present.
That is intentional — it marks the branch as not yet ready to merge.

#### Step 4: autosquash once approved

Once the PR is approved, collapse the fixups into their target commit and force-push, so
the branch is a single commit again and ready to merge:

```bash
git rebase --autosquash -S main
git push --force-with-lease
```

The check turns green and the final message is yours. Add `-i` to inspect the todo list
before it runs.

> [!IMPORTANT]
> Rebasing rewrites commits, which drops their signatures unless you pass `-S`. The
> `Signed-off-by` trailer lives in the message and survives, but verify both afterwards —
> see [Verify Sign Status](rebasing.md#verify-sign-status).

GitHub's *Squash and merge* button can also collapse a branch that carries only a base
commit plus fixups, but it composes the message out of the concatenated commit subjects,
so whoever merges has to rewrite it in the merge dialog. Squashing locally is preferred:
you keep control of the message, and the branch is mergeable without an override.

### Keeping the Guides Current

A change that leaves the documentation wrong is not finished. Update the affected guide
in the same PR:

- `make` targets, CI checks, or test prerequisites → [Development Guide](development.md)
- the contribution, review, or release process → this document
- conventions, framework snippets, or paths that coding agents are told to follow →
  [`AGENTS.md`](../../AGENTS.md) and [`docs/agents/`](../agents/)

Reviewers should ask for the doc update rather than filing a follow-up: the author has
the context now, and a stale guide costs every later reader.

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
| `/finalize` | Maintainers and core contributors | Validates that the issue carries `status: awaiting triage`, exactly one `skill:` label, exactly one `priority:` label, and a recognized type; rebuilds the body in the expected format, leaving the title unchanged; and transitions the status label to `status: ready for dev`. |

## References

- [Hiero SDK Python — GitHub Workflows](https://github.com/hiero-ledger/hiero-sdk-python/tree/main/.github/workflows)
- [Hiero SDK C++ — GitHub Workflows](https://github.com/hiero-ledger/hiero-sdk-cpp/tree/main/.github/workflows)
- [LFDT Maintainer Days — Contribution Workflow Presentation](https://www.youtube.com/watch?v=I87WCpiXOOs)
- [Original proposal issue — fabric-x#130](https://github.com/hyperledger/fabric-x/issues/130)
