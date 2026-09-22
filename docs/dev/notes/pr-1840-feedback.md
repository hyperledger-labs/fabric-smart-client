# PR #1840 — Marcus's review, explained

Status: **CHANGES_REQUESTED** by @mbrandenburger, 2026-09-16. Four inline comments.

His summary line:

> I strongly believe that the classify-instead-of-stop idea is right — but I am not
> sure if the delivery service is the right place.

**Read that twice, because it is the whole review.** He is not saying the fix is
wrong. He is saying the *idea* is right and the *location* is wrong. Three of his
four comments are the same architectural point asked from different angles; only
one is a small independent thing.

---

## The one big point (comments 2 and 4 are the same question)

### What he actually said

On `delivery.go:98` (the `commitRetries` field):

> I am wondering if this is the correct naming. Should this be `callbackRetries`
> instead as the delivery service is actually independent from the committer?
> Should the committer contain the retry logic — as it better understands what
> transient/permanent errors are? What would mean that if the delivery received an
> error — it's already permanent or fatal?

On `failure.go:117` (the `classify` function):

> Are these error classes already wired into the committer? […] should the delivery
> component care about these error classes which are committer specific? Note that
> the committer has access to the vault and may understand what vault error is
> persistent or permanent, it has access to the membership service and can decide
> if there is a config tx updating failing that produces a fatal error.

### In plain English

The delivery service has one job: pull blocks off the peer's gRPC stream and hand
each one to a callback. It does not know or care who that callback is.

Our change taught it to look *inside* the errors coming back from that callback and
recognise things like "this is a Postgres deadlock" and "this is a rejected channel
configuration". Those are **committer** concepts. The delivery service now knows
about the vault and the membership service, which are not its business.

His proposed shape:

- **The committer classifies and retries**, because it is the component that owns
  the vault and the membership service and therefore actually knows which of its
  own errors are worth retrying.
- **By the time an error reaches the delivery service, it is already final.** The
  delivery service's job shrinks back to: got an error, stop the stream, report it.
  No classification, no retry loop.

His question *"What would mean that if the delivery received an error — it's already
permanent or fatal?"* is him describing that contract out loud. He is asking whether
we agree that should be the rule.

### He is right, and here is the evidence from our own PR

Two things in our own diff prove his point better than any argument:

**1. We had to special-case scans.** The delivery service is used by two completely
different kinds of caller:

| Caller | Callback | Retrying it is… |
| --- | --- | --- |
| the committer feed (`NewService`) | `Committer.Commit` | safe — the committer skips transactions it already committed |
| `Scan` / `ScanBlock` / `ScanFromBlock` | the *application's* own function | **not** safe — we know nothing about it |

Because the retry lives in the shared layer, it applied to both, and we had to
bolt on `deliveryService.commitRetries = 0` in `runBlockScan` to undo it. That line
is a patch over a layering mistake. If the retry lived in the committer, scans would
never have been affected and that line would not exist.

**2. Our own naming gives it away.** The field inside a component that is supposed
to be committer-agnostic is called `commitRetries`, and the metrics are
`commit_retries` and `commit_failures`. We could not name these things without
referring to the committer — which is a strong hint they belong to the committer.

### What this costs to change

Honest answer: it is real work, not a rename. Roughly:

- Move `classify` and the class constants from `delivery/` into `committer/`.
- Put the retry loop inside `Committer.Commit` — it already receives the whole
  block (`committer.go:293`) and holds the vault, membership service and channel
  config, so it has everything it needs.
- Move the two counters to the committer's existing `Metrics` struct.
- The delivery service goes back to roughly its old shape: on error, stop and
  report. The `ErrFatalCommit` / `FatalHandler` hook probably stays in delivery or
  moves up, since it is about the process, not about committing.
- Delete the `commitRetries = 0` line in `runBlockScan`. It becomes unnecessary,
  which is the clean signal that the move was correct.
- Config: `delivery.commitRetries` should probably become `committer.retries` or
  similar, since it would no longer be a delivery setting.

The good news: **the hard part is already done and does not get thrown away.** The
taxonomy, the four classes, the escalation-on-exhaustion behaviour, the tests, and
the reasoning about which errors are transient — all of that moves as-is. What
changes is which package it lives in.

### What to say to him

Agree, and confirm the contract before writing code:

> "Agreed — the classification is committer knowledge and it should live there. Can
> I confirm the contract: the committer classifies and retries internally, and any
> error that reaches the delivery callback is final, so delivery just stops and
> reports? If so I will move `classify` and the retry loop into the committer, move
> the counters onto its `Metrics`, and drop the `commitRetries = 0` workaround in
> `runBlockScan` — which only exists because the retry currently sits in the shared
> layer and leaks into application scans."

That last sentence is worth saying: it shows you found the same smell he did.

**One thing to raise, not to argue:** who stops the channel? If the committer
retries internally and only returns final errors, delivery still needs to know
whether to stop the channel or keep going. Ask him whether he wants:

- (a) the committer to return a typed/sentinel error that delivery reads for the
  stop decision, or
- (b) delivery to stop on *any* error, full stop, and the committer never returns
  one it wants ignored.

Option (b) is simpler and matches "already permanent or fatal". Let him pick.

---

## The small independent one

### Comment 3 — `metrics.go:40`

> Should we add a channel label for these metrics?

**Plain English:** an FSC node can run several channels at once. Every channel gets
its own `Delivery`, but they all report into one process-wide metrics registry, and
our counters carry only a `class` label. So if `commit_failures` goes up, an
operator sees *a* channel stopped but cannot tell *which one*.

**He is right, and it is a genuine gap** — I checked. The whole point of these
counters is to make a silently-stalled channel visible, and without a channel label
you learn "something stopped" and then have to go read logs to find out what. That
undercuts the feature.

Precedent in the repo: `fsc_fabric_core_generic_ordering_ordered_transactions`
already uses a `network` label (`core/generic/metrics/metrics.go:17`). The `network`
label is registered in the shared label table in `monitoring_metrics.md`; `channel`
is not yet.

**What to do:** yes, add it. Add `channel` (and probably `network`, since a node can
be on several networks too — worth asking him). Also register the new label(s) in
the label table in `monitoring_metrics.md`.

**Note:** if the metrics move to the committer per the big point above, this comes
almost free — the committer is already constructed per channel and knows both names
(`committer.go:157` builds its logger from exactly `network:channel`).

### Comment 1 — `configuration.md:656`

> should this comment sit above `sleepAfterFailure`?

**Plain English:** a formatting nit. My long explanatory comment sits above
`commitRetries`, but it talks about how `sleepAfterFailure` and `commitRetries`
combine to set the recovery window. He is asking whether it is in the right spot.

**What to do:** just fix it. Split it — put the "wait between attempts" part above
`sleepAfterFailure` and the "how many attempts" part above `commitRetries`. Or put
one short note above the whole `delivery:` block. Thirty seconds of work, no
discussion needed.

---

## How to run the meeting

**Lead with agreement, not defence.** He opened by saying the idea is right. The
disagreement is narrow and he is correct on it. Going in defensive would waste the
conversation.

Suggested order:

1. **"You're right about the layering."** Say it first and plainly.
2. **Give him the evidence he does not have yet** — the `commitRetries = 0`
   workaround in `runBlockScan`. He suspected the layering was wrong; that line is
   proof, because it exists purely to undo the retry for application scans. This is
   the strongest thing you can bring, and it is something you found, not something
   he pointed out.
3. **Confirm the contract** — the two options above for who decides to stop the
   channel. Get him to choose so the rework is not guesswork.
4. **Agree the metrics label**, and note it gets easier if the metrics move.
5. **Mention the docs nit is already fixed.**
6. **Set expectations on scope.** This is a package move plus rewiring, not a
   rename. The taxonomy and tests survive; be clear it is a few hours, not minutes,
   so nobody expects a push in ten minutes.

**One thing worth flagging to him that his comments do not cover:** #1624 asked
whether a failed config update should stall *all* transaction types on the channel
or only config processing. Our answer was "stall everything", which argues against
what the issue suggested — the reasoning is that a config we failed to apply may
have rotated MSPs or changed endorsement policy, so continuing to commit would
validate transactions against rules the network has already replaced. He asked for
that strategy originally on #1611, and moving the logic into the committer makes it
*easier* to revisit, since the committer is where "this was a config tx" is known.
Worth raising, because it is the one decision in this PR that still needs a
maintainer, and he is the maintainer.

---

## Summary table

| # | Comment | What he wants | Size | Verdict |
| --- | --- | --- | --- | --- |
| 2 + 4 | classify/retry in the wrong component | move classification and retry into the committer; delivery only stops and reports | hours | **he is right** — our own scan workaround proves it |
| 3 | no channel label on the metrics | add `channel` (ask about `network`), register in the label table | small | **he is right** — real gap |
| 1 | comment placement in `configuration.md` | move/split the comment | trivial | just do it |

Nothing he raised is wrong, and nothing he raised invalidates the work — the
taxonomy, the escalation behaviour and the tests all move to a better home.
