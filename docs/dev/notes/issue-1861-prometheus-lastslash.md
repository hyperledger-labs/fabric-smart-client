Title: fix(metrics): guard the no-separator case in prometheus GetPackageName

Type: Task
Template: .github/ISSUE_TEMPLATE/task.yml
Parent: #1861

Labels applied automatically by the template: `status: awaiting triage`
Labels to request from a maintainer: `status: ready for dev`, `skill: good first issue`, `priority: low`

---

### Description

`GetPackageName` in `platform/view/services/metrics/prometheus/provider.go:147`
slices on the result of `strings.LastIndex(fullFuncName, "/")` without checking
for `-1`:

```go
lastSlash := strings.LastIndex(fullFuncName, "/")
dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
return fullFuncName[:lastSlash+dotAfterSlash]
```

When the resolved caller frame belongs to a package whose fully qualified
function name contains no path separator, `lastSlash` is `-1` and
`fullFuncName[lastSlash:]` panics with
`runtime error: slice bounds out of range [-1:]`.

Go's `main` package has no separator in its fully qualified function name
(`main.main`), so any FSC-based binary that creates a metric directly from `main`
crashes. Reproduced with a module that calls `prometheus.NewCounter` from `main`:
without the guard it panics with `slice bounds out of range [-1:]`, with the guard
it returns normally. This reaches `GetPackageName` through the ordinary
`NewCounter` → `applyNamespaceSubsystem` chain at the intended
`callerSkipFrames = 3` depth — no unusual call depth required.

`runtime` frames (`runtime.main`, `runtime.goexit`) are slash-free for the same
reason, so a direct call to the exported `GetPackageName`, which skips the three
frames of that chain, also panics.

Current state: of the three copies of this function in the repo, two guard the
`-1` case and one does not.

| Location | Guarded |
|---|---|
| `platform/view/services/tracing/config.go:108` | yes — returns `fullFuncName` |
| `platform/common/services/logging/logger.go:121` | yes — returns an error (added in #1844) |
| `platform/view/services/metrics/prometheus/provider.go:147` | **no** |

Target state: the prometheus copy behaves like its `tracing` sibling and returns
the unqualified `fullFuncName` when there is no separator.

Note on the parent checklist: #1861 lists this item as
`logging/logger.go:111 and metrics/prometheus/provider.go:147`. The
`logging/logger.go` half was already fixed by #1844, which merged before the
audit was filed, so only the prometheus copy remains.

### Implementation Steps

- [ ] Return `fullFuncName` when `lastSlash == -1` in
      `platform/view/services/metrics/prometheus/provider.go`, mirroring
      `platform/view/services/tracing/config.go:108`
- [ ] Add a spec to the existing Ginkgo suite in `provider_test.go` asserting
      that a direct `GetPackageName()` call does not panic and returns a name
      containing no `/`
- [ ] Verify: `go test -count=1 -race ./platform/view/services/metrics/prometheus/`
      and `make lint`

### Additional Information

Parent: #1861 — third item under "Latent landmines in exported APIs (no in-repo
caller today)". The "no in-repo caller today" heading undersells this one: there
is no in-repo caller, but any downstream binary creating a metric from `main`
hits it, which is the trigger the parent issue itself describes.

Removing the guard makes the new spec fail with
`runtime error: slice bounds out of range [-1:]`, confirming it covers the fix.

The parent issue cites `platform/view/services/tracing/utils.go` as the correctly
guarded copy. The guard is in `platform/view/services/tracing/config.go:108`;
`utils.go` exists but does not contain this function.

Out of scope: the two `panic` calls above the slicing (on `!ok` and `fn == nil`)
and the identical pair in `tracing/config.go`. Converting those to returned
errors changes the signature of an exported function and each of its callers;
`crypto/provider.go:31` and `state/rwsetextractor.go:65` are listed separately in
#1861 for the same class of problem and would be better handled together.
