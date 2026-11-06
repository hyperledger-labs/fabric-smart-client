# Writing Documentation

FSC's documentation has two audiences: the people who use and develop FSC, and the coding
agents that work in the repository. This page says where each kind of documentation lives,
what belongs to which audience, how documentation is written, and when it is updated.

## Two audiences

**Documentation for people** is complete and self-contained. It is everything under
`docs/` except `docs/agents/`, plus the `README.md` files and
[`CONTRIBUTING.md`](../../CONTRIBUTING.md). A reader who knows only these pages can use FSC,
build it, test it and contribute to it. They never link into `docs/agents/`; `make checks`
fails when one does.

**Documentation for agents** is [`AGENTS.md`](../../AGENTS.md) (`CLAUDE.md` is a symlink to
it) and `docs/agents/`. It holds only what helps an agent produce consistent work on top of
the documentation for people: the rules to follow without exception, mistakes agents tend
to make, and the checks to run before a change is finished. It links to the pages for
people for everything else and is never the only place a fact is written down. If a page
under `docs/agents/` explains how something works, that explanation belongs in a page for
people, and the agent page links to it.

## Where documentation goes

| Topic | Location |
|---|---|
| What FSC is and how its parts fit together | [`docs/core-concepts.md`](../core-concepts.md) |
| A platform: its services, programming model, drivers and configuration | `docs/platform/<platform>/` (`view`, `fabric`, `fabric-x`) |
| Configuration keys | [`docs/configuration.md`](../configuration.md) and the platform's `configuration.md`; the reference tests require it, see [Conventions → Configuration](conventions.md#configuration) |
| Metrics | [`docs/platform/view/services/monitoring_metrics.md`](../platform/view/services/monitoring_metrics.md) |
| Command-line tools | `docs/reference/` |
| What an operator does when upgrading from one release to the next | `docs/upgrade/<from>-to-<to>.md` |
| Setting up a development environment, building, running tests, CI | [`docs/dev/development.md`](development.md) |
| How the code is organised and the conventions it follows | [`docs/dev/architecture.md`](architecture.md), [`docs/dev/conventions.md`](conventions.md) |
| Writing unit tests and fuzz targets | [`docs/dev/testing.md`](testing.md) |
| Writing and running integration tests | [`docs/dev/integration-tests/`](integration-tests/README.md) |
| Contribution process: issues, commits, pull requests, review | [`docs/dev/workflow.md`](workflow.md), [`CONTRIBUTING.md`](../../CONTRIBUTING.md) |
| Cutting a release | [`docs/dev/releasing.md`](releasing.md) |
| The contract of an exported package, type or function | its Godoc comment |
| An example or a test suite | a `README.md` next to its code, linking to the guide it belongs to |

New pages are linked from [`docs/README.md`](../README.md) or from the section page they
belong to, so a reader can reach every page from the index.

### Upgrade guides

The upgrade guide of a minor release, `docs/upgrade/v<from>-to-v<to>.md`, says what an
operator or a developer has to do to upgrade from the previous minor. It starts with opening
paragraphs between its title and its first heading: what the release changes, and whether
organizations upgrade together. It ends with a `# Checklist` section that lists every step for
operators and for developers. The release notes link to the guide on `main` instead of quoting
it (see [`docs/upgrade/releases/`](../upgrade/releases/README.md)), so a fix to the guide after
the release reaches everyone who follows the link. A guide therefore stays in the repository
after later releases, and `go run ./scripts/release notes` refuses a minor whose guide does not
exist.

## How documentation is written

Documentation, Godoc comments and standalone pages alike, describes the **current
implementation as a self-contained system**. Write for a developer who cloned the
repository today: they never saw the previous implementation, do not know the git history,
and have not read the PR or issue. If a page only makes sense to someone who has, rewrite
it.

- **Not a changelog.** Never write "previously", "before this change", "we changed/moved
  from X to Y", "formerly", "now instead of", "this replaces", "the old implementation" or
  "after the refactor". Don't explain a design by describing what the code used to do. The
  upgrade guides under `docs/upgrade/` are the exception: describing what changed between
  two releases is their purpose.
- **Explain why the current design exists**, in terms of today's requirements
  (correctness, concurrency, ordering, lifecycle, security, performance, resource
  ownership, API guarantees, compatibility, failure handling), and only when that
  rationale helps understand or maintain the code. Prefer *"The client uses X to
  coordinate concurrent requests and preserve ordering."* over *"We previously used Y, but
  changed it to X."*
- **History is an input, not a subject.** Inspect commits, PRs and issues freely while
  investigating why the code looks the way it does; document the resulting design, not the
  investigation. Linking an issue for extra context is fine as long as the page still
  explains the behaviour on its own.
- **Godoc** covers what an exported package, type or function represents, its
  responsibilities, guarantees, preconditions, concurrency and lifecycle requirements,
  error behaviour, and semantics not obvious from the signature. It never says why the code
  changed, what the old version did or which PR introduced it. Comments must stay useful
  with no git history available. Style: [Conventions → Godoc](conventions.md#godoc).
- **Write for maintainers**: responsibilities and ownership, component relationships,
  important execution flows, invariants, lifecycle, concurrency model, error handling,
  resource management, extension points, and assumptions that must remain true. Don't
  narrate obvious code.
- **Style**: precise, concise, technical, factual, present tense; no narrative.

## When documentation is updated

A change that leaves the documentation wrong is not finished. Update it in the same PR,
in the same commit as the code:

- Godoc when behaviour or semantics change.
- The page for people from the table above when architecture, configuration, metrics,
  commands, `make` targets, CI checks or test prerequisites change.
- `AGENTS.md` or `docs/agents/` only when a rule agents must follow changes, after the page
  for people that explains it.
- Documentation for behaviour that no longer exists is deleted. Historical wording is not
  kept just because it was once accurate.

Reviewers ask for the documentation update rather than filing a follow-up: the author has
the context now, and a stale guide costs every later reader.
