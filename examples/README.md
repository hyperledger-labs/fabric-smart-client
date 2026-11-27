# Examples

This module holds FSC demos: applications that show how to build a use case
with FSC, written to be read. A demo's dependencies stay in this module's
`go.mod`, out of the root and `integration` modules and out of everything that
depends on them.

Each demo has a directory of its own. That directory holds the demo's
integration suite and a `README.md` that walks through it; its subpackages hold
the views, states and rules, with their unit tests. A demo's suite runs with
its own target, `make integration-tests-examples-<name>`, the same way a suite
under `integration/` runs with `make integration-tests-<name>`, and
`make unit-tests-examples` runs the unit tests of every demo.

This module is never released or tagged: it `replace`s the root and
`integration` modules with local paths, so it always builds against the
checked-out tree, not a published version.

## Adding a demo

1. Put the demo in `examples/<name>/`, with its subpackages below it.
1. Add its port range to `integration/ports.go`.
1. Add `examples-<name>` to `INTEGRATION_TARGETS` in the `Makefile`, with
   `INTEGRATION_DIR_examples-<name> = ../examples/<name>`, and its subpackages
   to `EXAMPLES_UNIT_PACKAGES`.
1. If the demo needs Chaguar or another network, add its target to the matching
   setup step of the integration job in `.github/workflows/tests.yml`.
1. List it below.

## Demos

- [`tickets/`](tickets/README.md): ticket booking on Chaguar between a
  promoter and two resellers, each FSC node serving a REST API and a web page,
  with the ledger deciding when two resellers sell the same seat
  (`make integration-tests-examples-tickets`, `make run-examples-tickets`).
