# Architecture: Agent Checklist

Read [Architecture and Dependency Injection](../dev/architecture.md) first. When
a change adds, wires or replaces a service:

- Register it in the platform's `sdk/dig/sdk.go`, and keep the call to the
  parent `p.SDK.Install()` in `Install()`.
- In `Stop()`, stop the SDK's own components first, then call the wrapped SDK's
  `Stop()`. Never tear down in a goroutine waiting on `ctx.Done()`.
- Replace a listener by decorating its constructor, never the built server, so a
  disabled listener is never built.
- Keep backend constructors free of side effects: the SDK constructs every
  backend in the group just to read its name.
- Never add a field with security meaning to an existing view-API message; add a
  new command type ([Conventions → Security](../dev/conventions.md#security)).
- Align with the Fabric-x committer only on the wire protocol, not on its Go
  style.
