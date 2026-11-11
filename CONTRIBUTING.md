# Contributing

We appreciate your contributions to the Fabric Smart Client project. Any help is welcome and there is still much to do.

**Jump To:**
- [Code Contributions](#code-contributions)
- [Submitting an Issue](#submitting-an-issue)

## Code contributions

Read these in order:

1. [Contribution Workflow](docs/dev/workflow.md): how issues are claimed and how a pull request
   gets merged. Pick an issue labeled `status: ready for dev` from the
   [open, unassigned issues](https://github.com/hyperledger-labs/fabric-smart-client/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)
   and claim it by commenting `/assign`. Each issue has a skill level, and new contributors
   start with `skill: good first issue` (see the [Skill System](docs/dev/workflow.md#skill-system)).
2. [Development Guide](docs/dev/development.md): fork and clone the repository, install the
   tools, and run the checks and tests.
3. [Commit Signing Guide](docs/dev/signing.md): sign every commit (`-s -S`).
4. [Tests](docs/dev/workflow.md#tests) and the [Testing Guide](docs/dev/testing.md): add or
   extend the tests that cover your change.
5. [Commit Hygiene](docs/dev/workflow.md#commit-hygiene) and
   [Writing a PR Description](docs/dev/workflow.md#writing-a-pr-description): open a draft
   pull request early, squash it into one commit before requesting review, then mark it
   ready for review.

## Submitting an issue

### Bug reports

**Ensure you are using the latest release of the Fabric Smart Client.** The bug may already be fixed.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. **Check the bug is not already reported.** If it is, comment to confirm you're also experiencing it.
3. Click **New Issue** and choose the **Bug Report** template.

The template will guide you through providing a description, steps to reproduce, expected vs. actual behavior, and
environment details. The more reproducible the report, the faster a fix can land.

Security vulnerabilities should be disclosed responsibly. Please see [SECURITY.md](SECURITY.md).

### Feature requests

**Note:** If you intend to implement a feature yourself, please submit the request _before_ writing any code and
ask to be assigned. Features are for user-facing capabilities: new platforms, services, API methods, etc.
Improvements to tooling, CI, or the contribution process are [Tasks](#tasks) instead.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. Verify the feature has not already been proposed.
3. Click **New Issue** and choose the **Feature** template.

### Tasks

Use a Task for maintenance, improvement, or operational work: refactoring, dependency updates, documentation
improvements, CI/CD changes, test coverage, or enhancements to existing features.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. Verify the work has not already been proposed.
3. Click **New Issue** and choose the **Task** template.

### After submitting

All three templates automatically apply `status: awaiting triage`. A maintainer will review the issue and prepare it for contributors.
Once finalized, the issue will have `status: ready for dev` and can be claimed via `/assign`.

---

Note that we follow the [LFDT Charter](https://www.lfdecentralizedtrust.org/about/charter), particularly, all new inbound code contributions to the Fabric Smart Client project shall be made under the Apache License, Version 2.0. All outbound code will be made available under the Apache License, Version 2.0.

You can also reach us on Discord in [#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012).
