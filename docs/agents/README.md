# Agent Guides

Detailed, task-specific guidance for coding agents working in this repository.
Start from the root [`AGENTS.md`](../../AGENTS.md), which links here on demand.

- [Architecture & dependency injection](architecture.md) — platform layout, SDK
  composition, the `dig` Install/Start pattern, multi-network access.
- [Conventions](conventions.md) — code organization, error handling, logging,
  storage drivers, identity, security.
- [Testing](testing.md) — unit-test conventions and Ginkgo/Gomega patterns.
- [Integration tests](integration-tests.md) — authoring a new integration test
  and the network-topology API.

For the view/session programming model (how views, sessions, and initiator/
responder protocols work), see
[`docs/platform/view/programming-model.md`](../platform/view/programming-model.md).
