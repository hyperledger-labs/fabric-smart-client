# Contributing

We appreciate your contributions to the Fabric Smart Client project. Any help is welcome and there is still much to do.

This document describes the contribution process we follow in this repository so contributors and maintainers have a shared expectation for how work is proposed, claimed, reviewed, and merged.

## Communication

- Use the [issue tracker](https://github.com/hyperledger-labs/fabric-smart-client/issues) for bug reports, feature requests, and project tasks.
- Use [pull requests](https://github.com/hyperledger-labs/fabric-smart-client/pulls) for proposed code, test, and documentation changes.
- Use Discord in [#fabric-smart-client](https://discord.gg/hyperledger) for coordination questions and lightweight discussion.

## Roles

- `Maintainer`: has `admin` or `write` permission on the repository.
- `Core contributor`: has `triage` permission on the repository.
- `Contributor`: everyone else.

Maintainers and core contributors may bypass the assignment and PR-linking expectations below when needed to keep the project moving. The process is primarily meant to coordinate community contributions and reduce duplicate work.

## Issue Lifecycle

### Issue Types

Every issue should describe one of the following:

- `bug`: something is broken or behaves unexpectedly.
- `feature`: a new capability or improvement.
- `task`: a concrete piece of implementation, testing, or documentation work.

Maintainers may also use workflow labels to communicate whether an issue is ready for external contribution.

### Ready For Contribution

Contributors should prefer issues that are already triaged and ready for work. In practice, that means:

- the problem statement is clear,
- the expected outcome is understood,
- important design constraints are known,
- and a maintainer has indicated that the issue is ready to be picked up.

If an issue is not clearly ready, start by commenting on the issue instead of opening a PR immediately.

### Claiming An Issue

Before opening a PR for non-trivial work, claim the issue in a comment so others know it is in progress.

Recommended flow:

1. Comment on the issue that you would like to work on it.
2. Wait for confirmation or assignment from a maintainer when the issue needs coordination.
3. If you stop working on the issue, leave a short update so someone else can take it over.

Maintainers may assign issues directly when coordination is needed.

### Inactivity

If an issue has been claimed but there is no visible progress for several days, maintainers may ask for a status update and may reassign the issue so the work does not stall. A quick progress comment is enough to keep the issue active.

## Pull Request Lifecycle

### Open A PR Against An Issue

Whenever possible, link the PR to the issue it addresses.

- Use closing keywords in the PR description when appropriate, for example `Fixes #123`.
- If the PR is only part of the work, use a non-closing reference instead.
- If there is no issue yet for substantial work, open one first so design and scope can be discussed.

Small maintenance or follow-up changes may be accepted without a dedicated issue at maintainer discretion.

### Keep PRs Focused

To make review easier:

- prefer one logical change per PR,
- include tests whenever behavior changes,
- update documentation when the user or developer workflow changes,
- avoid mixing unrelated refactors with the actual fix.

### Review Follow-Up

After opening a PR:

- respond to review comments in a timely way,
- push follow-up commits or update the branch as requested,
- rebase or merge `main` when requested to resolve drift or failing checks.

Maintainers may close stale PRs that show no activity for an extended period. Contributors are welcome to reopen the work later with a refreshed branch.

## Quality Expectations

Before requesting review, contributors should run the relevant checks locally whenever possible.

Typical commands include:

```bash
make checks
make lint
make unit-tests
make integration-tests
```

For more details on the local setup, helper tooling, and test commands, see [`docs/development.md`](docs/development.md).

If a change only affects a subset of the project, run the targeted checks that cover that area and mention them in the PR description.

## Commit And PR Requirements

- Sign off commits to satisfy the DCO requirements, for example by using `git commit --signoff`.
- Make sure GitHub Actions checks pass before merge unless a maintainer explicitly decides otherwise.
- Write clear commit messages and PR descriptions so reviewers can understand the intent quickly.

## Licensing

We follow the [LFDT Charter](https://www.lfdecentralizedtrust.org/about/charter). All new inbound code contributions to the Fabric Smart Client project shall be made under the Apache License, Version 2.0. All outbound code is made available under the Apache License, Version 2.0.
