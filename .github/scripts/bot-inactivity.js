// SPDX-License-Identifier: Apache-2.0
//
// bot-inactivity.js
//
// Scheduled inactivity bot. Runs daily to detect stale assigned issues and PRs.
//
// Timeline (for non-blocked items):
//   5 days of inactivity → warning comment (idempotent via HTML marker)
//   7 days of inactivity → close, remove assignees, reset to "status: ready for dev"
//
// Activity signals that reset the 5-day clock:
//   - A non-bot comment on the item by the author or any assignee
//   - A commit pushed to a PR branch by the PR author
//   - Removal of the "status: blocked" label (unblocking counts as activity)
//
// PR review state:
//   - "status: needs review"   → skipped entirely (waiting on maintainers)
//   - "status: needs revision" → clock starts from when label was last applied
//
// Blocked items ("status: blocked" label):
//   - Exempt from the inactivity close/warn flow
//   - Receive a friendly check-in comment every 30 days instead
//
// Cross-referencing:
//   - Issues linked to open PRs with recent activity are not flagged.
//   - When a PR is closed for inactivity, its linked issues are also reset.

const { createLogger } = require('./helpers/logger');
const { LABELS } = require('./helpers/constants');
const {
  hasLabel,
  removeLabel,
  addLabels,
  removeAssignees,
  postComment,
  postOrUpdateComment,
  fetchComments,
  fetchIssueEvents,
  fetchPRCommits,
  closeItem,
} = require('./helpers/api');
const { parseIssueNumbers } = require('./helpers/checks');

// ─── Constants ───────────────────────────────────────────────────────────────

const WARN_AFTER_MS           = 5 * 24 * 60 * 60 * 1000;  // 5 days
const CLOSE_AFTER_MS          = 7 * 24 * 60 * 60 * 1000;  // 7 days
const BLOCKED_CHECKIN_AFTER_MS = 30 * 24 * 60 * 60 * 1000; // 30 days

const WARN_MARKER           = '<!-- bot:inactivity-warning -->';
const BLOCKED_CHECKIN_MARKER = '<!-- bot:blocked-checkin -->';

// ─── Context builder ─────────────────────────────────────────────────────────

/**
 * Builds a minimal context object compatible with shared API helpers.
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number
 * @returns {{ github: object, owner: string, repo: string, number: number }}
 */
function buildCtx(github, owner, repo, number) {
  return { github, owner, repo, number };
}

// ─── Activity helpers ─────────────────────────────────────────────────────────

/**
 * Returns the greater of two timestamps. If candidate is null or not later
 * than current, returns current unchanged.
 * @param {number} current
 * @param {number|null} candidate
 * @returns {number}
 */
function latestOf(current, candidate) {
  if (candidate === null || candidate <= current) return current;
  return candidate;
}

/**
 * Builds a Set of lowercased participant logins from an issue or PR object.
 * Includes the item's user (author) and all assignees.
 * @param {object} item
 * @returns {Set<string>}
 */
function collectParticipants(item) {
  const logins = (item.assignees || []).map(a => a.login.toLowerCase());
  if (item.user?.login) logins.push(item.user.login.toLowerCase());
  return new Set(logins);
}

/**
 * Returns true if the commit was authored by the given (lowercased) login.
 * @param {object} commit
 * @param {string} login - Lowercased login to match.
 * @returns {boolean}
 */
function isAuthorLogin(commit, login) {
  return commit.author?.login?.toLowerCase() === login;
}

/**
 * Extracts the best available date string from a commit object.
 * @param {object} commit
 * @returns {string|null}
 */
function extractCommitDate(commit) {
  return commit.commit?.committer?.date ?? commit.commit?.author?.date ?? null;
}

/**
 * Returns the timestamp (ms) of the most recent non-bot comment posted by any
 * of the given usernames, or null if none found.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number - Issue or PR number.
 * @param {Set<string>} relevantLogins - Lowercased usernames to consider.
 * @returns {Promise<number|null>}
 */
async function getLastRelevantCommentDate(github, owner, repo, number, relevantLogins) {
  const ctx = buildCtx(github, owner, repo, number);
  const comments = await fetchComments(ctx);

  let latest = null;
  for (const c of comments) {
    if (!c.user || c.user.type === 'Bot') continue;
    if (!relevantLogins.has(c.user.login.toLowerCase())) continue;
    const t = new Date(c.created_at).getTime();
    latest = latestOf(latest === null ? -Infinity : latest, t);
  }
  return latest === -Infinity ? null : latest;
}

/**
 * Returns the timestamp (ms) of the most recent commit authored by the given
 * login on the PR, or null if none found.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} prNumber
 * @param {string} authorLogin
 * @returns {Promise<number|null>}
 */
async function getLastAuthorCommitDate(github, owner, repo, prNumber, authorLogin) {
  const ctx = buildCtx(github, owner, repo, prNumber);
  const commits = await fetchPRCommits(ctx);
  const login = authorLogin.toLowerCase();
  let latest = null;

  for (const c of commits) {
    if (!isAuthorLogin(c, login)) continue;
    const dateStr = extractCommitDate(c);
    latest = latestOf(latest === null ? -Infinity : latest, dateStr ? new Date(dateStr).getTime() : -Infinity);
  }
  return latest === null || latest === -Infinity ? null : latest;
}

/**
 * Returns the timestamp (ms) of the most recent event matching predicate,
 * or null if no matching event is found.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number
 * @param {function} predicate - Called with each event object; return true to include.
 * @returns {Promise<number|null>}
 */
async function getLastMatchingEventDate(github, owner, repo, number, predicate) {
  const ctx = buildCtx(github, owner, repo, number);
  const events = await fetchIssueEvents(ctx);

  let latest = null;
  for (const e of events) {
    if (!predicate(e)) continue;
    const t = new Date(e.created_at).getTime();
    latest = latestOf(latest === null ? -Infinity : latest, t);
  }
  return latest === null || latest === -Infinity ? null : latest;
}

/**
 * Returns the timestamp (ms) of the most recent time "status: blocked" was
 * removed from the item, or null if it has never been unblocked.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number
 * @returns {Promise<number|null>}
 */
async function getLastUnblockedDate(github, owner, repo, number) {
  return getLastMatchingEventDate(github, owner, repo, number,
    e => e.event === 'unlabeled' && e.label?.name === LABELS.BLOCKED);
}

/**
 * Returns the timestamp (ms) of the most recent time "status: needs revision"
 * was applied to the item, or null if it has never been applied.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number
 * @returns {Promise<number|null>}
 */
async function getLastNeedsRevisionLabeledDate(github, owner, repo, number) {
  return getLastMatchingEventDate(github, owner, repo, number,
    e => e.event === 'labeled' && e.label?.name === LABELS.NEEDS_REVISION);
}

/**
 * Returns the timestamp (ms) of the most recent time the item was assigned,
 * or null if no assigned event is found.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {number} number
 * @returns {Promise<number|null>}
 */
async function getLastAssignedDate(github, owner, repo, number) {
  return getLastMatchingEventDate(github, owner, repo, number,
    e => e.event === 'assigned');
}

// ─── Activity computation ────────────────────────────────────────────────────

/**
 * Computes the last meaningful activity timestamp (ms) for a PR.
 * Considers: relevant comments (by PR author/assignees), commits by the PR
 * author, and removal of the blocked label.
 *
 * Also used by computeIssueLastActivity when evaluating linked open PRs.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {object} pr - GitHub PR object.
 * @returns {Promise<number>}
 */
async function computePRLastActivity(github, owner, repo, pr) {
  let latest = new Date(pr.created_at).getTime();
  const participants = collectParticipants(pr);

  if (participants.size > 0) {
    const d = await getLastRelevantCommentDate(github, owner, repo, pr.number, participants);
    latest = latestOf(latest, d);
  }

  if (pr.user?.login) {
    const d = await getLastAuthorCommitDate(github, owner, repo, pr.number, pr.user.login);
    latest = latestOf(latest, d);
  }

  return latestOf(latest, await getLastUnblockedDate(github, owner, repo, pr.number));
}

/**
 * Computes the last meaningful activity timestamp (ms) for an issue.
 * Considers: the issue's own relevant comments, removal of the blocked label,
 * and activity on any linked open PRs.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {object} issue - GitHub issue object.
 * @param {object[]} linkedOpenPRs - Open PR objects whose bodies reference this issue.
 * @returns {Promise<number>}
 */
async function computeIssueLastActivity(github, owner, repo, issue, linkedOpenPRs) {
  const assignedAt = await getLastAssignedDate(github, owner, repo, issue.number);
  if (assignedAt === null) return null;

  let latest = assignedAt;
  const assigneeLogins = new Set((issue.assignees || []).map(a => a.login.toLowerCase()));

  if (assigneeLogins.size > 0) {
    const d = await getLastRelevantCommentDate(github, owner, repo, issue.number, assigneeLogins);
    latest = latestOf(latest, d);
  }

  latest = latestOf(latest, await getLastUnblockedDate(github, owner, repo, issue.number));

  for (const pr of linkedOpenPRs) {
    latest = latestOf(latest, await computePRLastActivity(github, owner, repo, pr));
  }

  return latest;
}

// ─── Comment builders ─────────────────────────────────────────────────────────

/**
 * Builds the inactivity warning comment body.
 * @param {string[]} assigneeLogins
 * @param {string} itemType - 'issue' or 'PR'
 * @returns {string}
 */
function buildWarningComment(assigneeLogins, itemType) {
  const mentions = assigneeLogins.length > 0
    ? assigneeLogins.map(l => `@${l}`).join(', ')
    : 'there';

  return [
    WARN_MARKER,
    `👋 Hey ${mentions}! This ${itemType} has been inactive for 5 days.`,
    '',
    'Are you still working on this? We will close this in 2 days if we see no further activity.',
    '',
    "If you're still on it, leave a comment to let us know! If you'd like to step down, comment `/unassign`.",
  ].join('\n');
}

/**
 * Builds the closure comment body.
 * @param {string} itemType - 'issue' or 'PR'
 * @returns {string}
 */
function buildClosureComment(itemType) {
  if (itemType === 'issue') {
    return [
      '⏱️ This issue has been unassigned and reset to `status: ready for dev` due to 7 days of inactivity.',
      '',
      "If you'd like to continue working on this, feel free to comment `/assign` to be reassigned.",
    ].join('\n');
  }
  return [
    '⏱️ This PR has been closed due to 7 days of inactivity.',
    '',
    'It has been unassigned and reset to `status: ready for dev` so another contributor can pick it up.',
    '',
    "If you'd like to continue working on this, feel free to comment `/assign` to be reassigned.",
  ].join('\n');
}

/**
 * Builds the comment posted on an issue when its linked PR was closed for inactivity.
 * @returns {string}
 */
function buildLinkedPRClosedComment() {
  return [
    '🔗 The pull request linked to this issue was closed due to inactivity.',
    '',
    'This issue has been unassigned and reset to `status: ready for dev`.',
    '',
    "If you'd like to continue working on this, feel free to comment `/assign` to be reassigned.",
  ].join('\n');
}

/**
 * Builds the 30-day blocked check-in comment body.
 * @param {string[]} assigneeLogins
 * @param {string} itemType - 'issue' or 'PR'
 * @returns {string}
 */
function buildBlockedCheckinComment(assigneeLogins, itemType) {
  const mentions = assigneeLogins.length > 0
    ? assigneeLogins.map(l => `@${l}`).join(', ')
    : 'there';

  return [
    BLOCKED_CHECKIN_MARKER,
    `👋 Hey ${mentions}, just checking in! Is this ${itemType} still blocked?`,
    '',
    'If it has been unblocked, please remove the `status: blocked` label so we can track progress.',
  ].join('\n');
}

// ─── State mutation ───────────────────────────────────────────────────────────

/**
 * Resets an item back to "ready for dev": removes assignees, removes
 * "status: in progress", adds "status: ready for dev".
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {object} item - Issue or PR object with .number, .assignees, .labels.
 * @returns {Promise<void>}
 */
async function resetItem(github, owner, repo, item) {
  const ctx = buildCtx(github, owner, repo, item.number);
  const assigneeLogins = (item.assignees || []).map(a => a.login);

  if (assigneeLogins.length > 0) {
    await removeAssignees(ctx, assigneeLogins);
  }
  if (hasLabel(item, LABELS.IN_PROGRESS)) {
    await removeLabel(ctx, LABELS.IN_PROGRESS);
  }
  await addLabels(ctx, [LABELS.READY_FOR_DEV]);
}

// ─── Stale and blocked handlers ───────────────────────────────────────────────

/**
 * Formats a one-line per-item activity summary log.
 *
 * @param {object} item - Issue or PR object with .number and .assignees.
 * @param {string} itemType - 'issue' or 'PR'
 * @param {number} lastActivityMs - Timestamp of last meaningful activity.
 * @param {number} nowMs - Current time in ms.
 * @param {'closed'|'warned'|'none'} result
 * @returns {string}
 */
function formatActivitySummary(item, itemType, lastActivityMs, nowMs, result) {
  const days = Math.floor((nowMs - lastActivityMs) / (24 * 60 * 60 * 1000));
  const assigneeLogins = (item.assignees || []).map(a => a.login);
  const assignees = assigneeLogins.length > 0 ? assigneeLogins.join(', ') : 'none';

  let action = 'no action needed';
  if (result === 'warned') {
    action = 'posting inactivity warning';
  } else if (result === 'closed') {
    action = itemType === 'PR' ? 'closing PR' : 'unassigning and resetting issue';
  }

  return `#${item.number} (${itemType}): last activity ${days}d ago (assigned: ${assignees}), ${action}`;
}

/**
 * Applies the appropriate stale action to an item.
 * - If >= 7 days inactive: close, post closure comment, reset.
 * - If >= 5 days inactive: post or update warning comment.
 * - Otherwise: no action.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {object} item - Issue or PR object.
 * @param {number} lastActivityMs - Timestamp of last meaningful activity.
 * @param {string} itemType - 'issue' or 'PR'
 * @param {number} nowMs - Current time in ms (injectable for testing).
 * @returns {Promise<'closed'|'warned'|'none'>}
 */
async function handleStaleItem(github, owner, repo, item, lastActivityMs, itemType, nowMs) {
  const ctx = buildCtx(github, owner, repo, item.number);
  const elapsed = nowMs - lastActivityMs;
  const assigneeLogins = (item.assignees || []).map(a => a.login);

  if (elapsed >= CLOSE_AFTER_MS) {
    if (itemType === 'PR') {
      await closeItem(ctx);
    }
    await resetItem(github, owner, repo, item);
    await postComment(ctx, buildClosureComment(itemType));
    return 'closed';
  }

  if (elapsed >= WARN_AFTER_MS) {
    await postOrUpdateComment(ctx, WARN_MARKER, buildWarningComment(assigneeLogins, itemType));
    return 'warned';
  }

  return 'none';
}

/**
 * Handles a blocked item: posts or refreshes a check-in comment if 30 days
 * have passed since the last one.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {object} item - Issue or PR object.
 * @param {string} itemType - 'issue' or 'PR'
 * @param {number} nowMs - Current time in ms (injectable for testing).
 * @param {object} logger
 * @returns {Promise<void>}
 */
async function handleBlockedItem(github, owner, repo, item, itemType, nowMs, logger) {
  const ctx = buildCtx(github, owner, repo, item.number);
  const comments = await fetchComments(ctx);

  // Find the most recently updated check-in comment (there should be at most one).
  const checkinComment = comments
    .filter(c => c.body && c.body.startsWith(BLOCKED_CHECKIN_MARKER))
    .sort((a, b) => new Date(b.updated_at || b.created_at) - new Date(a.updated_at || a.created_at))[0];

  const lastCheckinMs = checkinComment
    ? new Date(checkinComment.updated_at || checkinComment.created_at).getTime()
    : null;

  if (lastCheckinMs === null || (nowMs - lastCheckinMs) >= BLOCKED_CHECKIN_AFTER_MS) {
    logger.log(`#${item.number} (${itemType}): posting blocked check-in`);
    const assigneeLogins = (item.assignees || []).map(a => a.login);
    await postOrUpdateComment(ctx, BLOCKED_CHECKIN_MARKER, buildBlockedCheckinComment(assigneeLogins, itemType));
  } else {
    logger.log(`#${item.number} (${itemType}): blocked check-in not due yet`);
  }
}

// ─── Data fetching ────────────────────────────────────────────────────────────

/**
 * Fetches all open assigned issues (paginated). Includes issues with any label
 * (blocked or in-progress) so both are handled in the main loop.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @returns {Promise<object[]>}
 */
async function fetchAssignedIssues(github, owner, repo) {
  const items = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const { data } = await github.rest.issues.listForRepo({
      owner,
      repo,
      state: 'open',
      assignee: '*',
      per_page: perPage,
      page,
    });
    // issues.listForRepo returns both issues and PRs — filter to issues only
    items.push(...data.filter(item => !item.pull_request));
    if (data.length < perPage) break;
    page++;
  }

  return items;
}

/**
 * Fetches all open PRs (paginated).
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @returns {Promise<object[]>}
 */
async function fetchOpenPRs(github, owner, repo) {
  const prs = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const { data } = await github.rest.pulls.list({
      owner,
      repo,
      state: 'open',
      per_page: perPage,
      page,
    });
    prs.push(...data);
    if (data.length < perPage) break;
    page++;
  }

  return prs;
}

/**
 * Builds a map from issue number → array of open PR objects whose body
 * references that issue via closing keywords or "related to".
 *
 * @param {object[]} openPRs
 * @returns {Map<number, object[]>}
 */
function buildIssueToPRsMap(openPRs) {
  const map = new Map();
  for (const pr of openPRs) {
    const issueNums = parseIssueNumbers(pr.body || '');
    for (const num of issueNums) {
      if (!map.has(num)) map.set(num, []);
      map.get(num).push(pr);
    }
  }
  return map;
}

// ─── Main entrypoint ──────────────────────────────────────────────────────────

/**
 * Main entrypoint for the inactivity bot.
 *
 * @param {{ github: object, context: object, getNow?: () => number }} args
 *   - getNow is injectable for testing; defaults to Date.now.
 */
module.exports = async function ({ github, context, getNow = () => Date.now() }) {
  const logger = createLogger('[inactivity-bot]');
  const { owner, repo } = context.repo;
  const nowMs = getNow();

  logger.log(`Starting inactivity check (now=${new Date(nowMs).toISOString()})`);

  // ── Fetch data ────────────────────────────────────────────────────────────

  const [assignedIssues, allOpenPRs] = await Promise.all([
    fetchAssignedIssues(github, owner, repo),
    fetchOpenPRs(github, owner, repo),
  ]);
 
  const processedIssueNumbers = new Set();

  const assignedOpenPRs = allOpenPRs.filter(pr => (pr.assignees || []).length > 0);
  const issueToOpenPRs  = buildIssueToPRsMap(allOpenPRs);

  logger.log(
    `Found ${assignedIssues.length} assigned issues, ` +
    `${assignedOpenPRs.length} assigned open PRs`
  );

  // ── Process assigned PRs ──────────────────────────────────────────────────

  for (const pr of assignedOpenPRs) {
    if (hasLabel(pr, LABELS.BLOCKED)) {
      await handleBlockedItem(github, owner, repo, pr, 'PR', nowMs, logger);
      continue;
    }

    if (hasLabel(pr, LABELS.NEEDS_REVIEW)) {
      logger.log(`#${pr.number} (PR): skipping (status: needs review)`);
      continue;
    }

    let lastActivity;
    if (hasLabel(pr, LABELS.NEEDS_REVISION)) {
      const revisionLabeledAt = await getLastNeedsRevisionLabeledDate(github, owner, repo, pr.number);
      const prActivity = await computePRLastActivity(github, owner, repo, pr);
      lastActivity = latestOf(prActivity, revisionLabeledAt);
    } else {
      lastActivity = await computePRLastActivity(github, owner, repo, pr);
    }

    const result = await handleStaleItem(github, owner, repo, pr, lastActivity, 'PR', nowMs);
    logger.log(formatActivitySummary(pr, 'PR', lastActivity, nowMs, result));

    if (result === 'closed') {
      // Clean up issues linked to this PR immediately
      const linkedIssueNums = parseIssueNumbers(pr.body || '');
      for (const issueNum of linkedIssueNums) {
        try {
          const { data: linkedIssue } = await github.rest.issues.get({
            owner, repo, issue_number: issueNum,
          });
          if (linkedIssue.state === 'open' && (linkedIssue.assignees || []).length > 0) {
            logger.log(`Cleaning up linked issue #${issueNum} (PR #${pr.number} closed for inactivity)`);
            await resetItem(github, owner, repo, linkedIssue);
            const ctx = buildCtx(github, owner, repo, issueNum);
            await postComment(ctx, buildLinkedPRClosedComment());
            processedIssueNumbers.add(issueNum); 
          }
        } catch (err) {
          logger.error(`Could not clean up linked issue #${issueNum}: ${err.message}`);
        }
      }
    }
  }

  // ── Process assigned issues ───────────────────────────────────────────────

  for (const issue of assignedIssues) {
    if (processedIssueNumbers.has(issue.number)) {
      logger.log(`#${issue.number}: skipping (already handled via PR loop)`);
       continue;
    }
    if (hasLabel(issue, LABELS.BLOCKED)) {
      await handleBlockedItem(github, owner, repo, issue, 'issue', nowMs, logger);
      continue;
    }

    if (!hasLabel(issue, LABELS.IN_PROGRESS)) {
      logger.log(`#${issue.number}: skipping (no in-progress or blocked label)`);
      continue;
    }

    const linkedPRs = issueToOpenPRs.get(issue.number) || [];
    const lastActivity = await computeIssueLastActivity(github, owner, repo, issue, linkedPRs);
    if (lastActivity === null) {
      logger.log(`#${issue.number}: skipping (no assigned event found)`);
      continue;
    }
    const result = await handleStaleItem(github, owner, repo, issue, lastActivity, 'issue', nowMs);
    logger.log(formatActivitySummary(issue, 'issue', lastActivity, nowMs, result));
  }

  logger.log('Inactivity check complete');
};
