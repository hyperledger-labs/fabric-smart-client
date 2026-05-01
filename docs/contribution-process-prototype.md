# Contribution Process Prototype

This document describes the prototype automation added for the Fabric Smart Client contribution process.

The goal is to provide a small, runnable showcase on a fork before the full workflow is enabled or expanded in the main repository.

## Included Prototype Pieces

- `/assign` on issues marked `ready`
- `/unassign` for the current assignee
- `/working` as a lightweight activity signal
- a scheduled PR enforcer that checks whether external contributors link a PR to an open issue and are assigned to that issue

## Implemented Rules

### Issue Claiming

- The `/assign` command only works on open issues.
- The issue must carry the `ready` label.
- A contributor may hold at most 2 open assigned issues at a time.
- Repository collaborators with `triage`, `write`, or `admin` permission are exempt from the assignment limit.

### Issue Release

- The `/unassign` command removes the commenter from the issue, but only when that commenter is already assigned.

### Activity Signal

- The `/working` command reacts to the comment with `eyes`.
- It is accepted from the issue assignee or the PR author.
- This is intended as a simple signal that can later be consumed by inactivity workflows.

### Pull Request Enforcement

- A scheduled workflow checks open PRs from non-collaborators.
- After 12 hours, the workflow closes PRs that:
  - do not link an open issue, or
  - link an open issue that the PR author is not assigned to.

Maintainers, core contributors, and bot-authored PRs are exempt from this enforcement.

## How To Showcase On A Fork

1. Label an issue with `ready`.
2. Comment `/assign` from a non-collaborator account.
3. Open a PR from that account with and without `Fixes #<issue>` in the description.
4. Run the `Bot: Linked Issue Enforcer` workflow manually with `dry_run=false` to verify the close-and-comment behavior.
5. Comment `/unassign` and `/working` to verify the issue command handlers.

## Files Added

- `.github/workflows/bot-assign-on-comment.yml`
- `.github/workflows/unassign-on-comment.yml`
- `.github/workflows/working-on-comment.yml`
- `.github/workflows/cron-enforcer-pr-linked-issue.yml`
- `.github/scripts/bot-assign-on-comment.js`
- `.github/scripts/bot-unassign-on-comment.js`
- `.github/scripts/bot-working-on-comment.js`
- `.github/scripts/cron-enforcer-pr-linked-issue.js`

## Next Steps

- add inactivity reminder and unassignment workflows,
- integrate additional labels such as `good-first-issue`,
- refine contributor messaging,
- and decide which parts should remain prototype-only versus move into the default repository workflow.
