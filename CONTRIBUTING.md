# Contributing

We appreciate your contributions to the Fabric Smart Client project. Any help is welcome and there is still much to do.

**Jump To:**
- [Code Contributions](#code-contributions)
- [Submitting an Issue](#submitting-an-issue)

## Code Contributions

Browse [open, unassigned issues](https://github.com/hyperledger-labs/fabric-smart-client/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22) to find one that matches your interest.

Look for issues with the `status: ready for dev` label — these have been triaged and are ready to be worked on.
Issues also carry a skill-level label (`skill: good first issue`, `skill: beginner`, `skill: intermediate`, `skill: advanced`).
Start with `skill: good first issue` if you are new to the project. See the [Skill System](docs/dev/workflow.md#skill-system) for progression requirements.

### Getting Assigned

To claim an issue, comment on it with:

```
/assign
```

The bot will automatically:
1. Assign you to the issue
2. Update the issue labels
3. Post a welcome message

If you don't meet the prerequisites, the bot will show your progress and link to issues you can work on first.

### Submitting Your Work

Once assigned, follow the [Development Guide](docs/dev/development.md) to:
1. Fork the repository and create a branch
2. Make your changes
3. Sign your commits (`-s -S`). See the [Commit Signing Guide](docs/dev/signing.md)
4. Open a pull request

## Submitting an Issue

### Bug Reports 🐛

⚠️ **Ensure you are using the latest release of the Fabric Smart Client** — it's possible the bug is already fixed.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. ⚠️ **Check the bug is not already reported.** If it is, comment to confirm you're also experiencing it.
3. Click **New Issue** and choose the **Bug Report** template.

The template will guide you through providing a description, steps to reproduce, expected vs. actual behavior, and
environment details. The more reproducible the report, the faster a fix can land.

Security vulnerabilities should be disclosed responsibly. Please see [SECURITY.md](SECURITY.md)

### Feature Requests 💡

**Note:** If you intend to implement a feature yourself, please submit the request _before_ writing any code and
ask to be assigned. Features are for user-facing capabilities — new platforms, services, API methods, etc.
Improvements to tooling, CI, or the contribution process are [Tasks](#tasks) instead.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. Verify the feature has not already been proposed.
3. Click **New Issue** and choose the **Feature Request** template.

### Tasks 🔧

Use a Task for maintenance, improvement, or operational work: refactoring, dependency updates, documentation
improvements, CI/CD changes, test coverage, or enhancements to existing features.

1. Visit the [Issues page](https://github.com/hyperledger-labs/fabric-smart-client/issues).
2. Verify the work has not already been proposed.
3. Click **New Issue** and choose the **Task** template.

### After Submitting

All three templates automatically apply `status: awaiting triage`. A maintainer will review the issue and prepare it for contributors.
Once finalized, the issue will have `status: ready for dev` and can be claimed via `/assign`.

---

Note that we follow the [LFDT Charter](https://www.lfdecentralizedtrust.org/about/charter), particularly, all new inbound code contributions to the Fabric Smart Client project shall be made under the Apache License, Version 2.0. All outbound code will be made available under the Apache License, Version 2.0.

You can also reach us on Discord in [#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012).
