// SPDX-License-Identifier: Apache-2.0
//
// tests/test-inactivity-bot.js
//
// Integration tests for bot-inactivity.js.
// Run with: node .github/scripts/tests/test-inactivity-bot.js

const { runTestSuite } = require('./test-utils');
const script = require('../bot-inactivity.js');
const { LABELS } = require('../helpers/constants');

// =============================================================================
// TIME HELPERS
// =============================================================================

// Fixed "now" for all tests
const NOW = new Date('2024-01-15T12:00:00Z').getTime();

function daysAgo(n) {
  return new Date(NOW - n * 24 * 60 * 60 * 1000).toISOString();
}

// =============================================================================
// MOCK GITHUB FACTORY
// =============================================================================

/**
 * Creates a mock GitHub API for bot-inactivity.js tests.
 *
 * @param {object} opts
 * @param {object[]} opts.assignedIssues    - Items returned by issues.listForRepo (assignee:*)
 * @param {object[]} opts.openPRs           - Items returned by pulls.list
 * @param {object}   opts.commentsByNumber  - number -> comment[]  (issues.listComments)
 * @param {object}   opts.commitsByPRNumber - prNumber -> commit[] (pulls.listCommits)
 * @param {object}   opts.issuesByNumber    - number -> issue data (issues.get)
 * @param {object}   opts.eventsByNumber    - number -> event[]    (issues.listEvents)
 */
function createMockGithub(opts = {}) {
  const {
    assignedIssues = [],
    openPRs = [],
    commentsByNumber = {},
    commitsByPRNumber = {},
    issuesByNumber = {},
    eventsByNumber = {},
  } = opts;

  const calls = {
    itemsClosed: [],        // issue_numbers that were closed
    commentsCreated: [],    // { issue_number, body }
    commentsUpdated: [],    // { comment_id, body }
    labelsAdded: [],        // { issue_number, labels }
    labelsRemoved: [],      // { issue_number, name }
    assigneesRemoved: [],   // { issue_number, assignees }
    commentsList: [],       // { issue_number } — tracked for verification
  };

  const perPage = 100;

  // All comments keyed by issue/PR number (includes any pre-seeded ones used
  // by postOrUpdateComment to detect existing marker comments).
  const allComments = { ...commentsByNumber };

  const mock = {
    calls,
    rest: {
      issues: {
        listForRepo: async (params) => {
          const page = params.page || 1;
          const start = (page - 1) * perPage;
          const slice = assignedIssues.slice(start, start + perPage);
          return { data: slice };
        },

        listEvents: async (params) => {
          const num = params.issue_number;
          const all = eventsByNumber[num] || [];
          const page = params.page || 1;
          const start = (page - 1) * perPage;
          return { data: all.slice(start, start + perPage) };
        },

        listComments: async (params) => {
          const num = params.issue_number;
          calls.commentsList.push({ issue_number: num });
          const all = allComments[num] || [];
          const page = params.page || 1;
          const start = (page - 1) * perPage;
          return { data: all.slice(start, start + perPage) };
        },

        createComment: async (params) => {
          calls.commentsCreated.push({ issue_number: params.issue_number, body: params.body });
          console.log(`\n📝 COMMENT CREATED on #${params.issue_number}:\n${'─'.repeat(50)}\n${params.body}\n${'─'.repeat(50)}`);
        },

        updateComment: async (params) => {
          calls.commentsUpdated.push({ comment_id: params.comment_id, body: params.body });
          console.log(`\n✏️  COMMENT UPDATED (id=${params.comment_id}):\n${'─'.repeat(50)}\n${params.body}\n${'─'.repeat(50)}`);
        },

        update: async (params) => {
          if (params.state === 'closed') {
            calls.itemsClosed.push(params.issue_number);
            console.log(`\n🔒 CLOSED #${params.issue_number}`);
          }
        },

        addLabels: async (params) => {
          calls.labelsAdded.push({ issue_number: params.issue_number, labels: params.labels });
          console.log(`\n🏷️  LABELS ADDED on #${params.issue_number}: ${params.labels.join(', ')}`);
        },

        removeLabel: async (params) => {
          calls.labelsRemoved.push({ issue_number: params.issue_number, name: params.name });
          console.log(`\n🏷️  LABEL REMOVED on #${params.issue_number}: ${params.name}`);
        },

        removeAssignees: async (params) => {
          calls.assigneesRemoved.push({ issue_number: params.issue_number, assignees: params.assignees });
          console.log(`\n👤 ASSIGNEES REMOVED on #${params.issue_number}: ${params.assignees.join(', ')}`);
        },

        get: async (params) => {
          const data = issuesByNumber[params.issue_number] || {
            number: params.issue_number,
            state: 'open',
            assignees: [],
            labels: [],
            created_at: daysAgo(1),
          };
          return { data };
        },
      },

      pulls: {
        list: async (params) => {
          const page = params.page || 1;
          const start = (page - 1) * perPage;
          const slice = openPRs.slice(start, start + perPage);
          return { data: slice };
        },

        listCommits: async (params) => {
          const all = commitsByPRNumber[params.pull_number] || [];
          const page = params.page || 1;
          const start = (page - 1) * perPage;
          return { data: all.slice(start, start + perPage) };
        },
      },
    },
  };

  return mock;
}

// =============================================================================
// HELPERS — ITEM BUILDERS
// =============================================================================

function makeIssue(number, { createdAt = daysAgo(1), assignees = [], labels = [] } = {}) {
  return {
    number,
    state: 'open',
    created_at: createdAt,
    assignees: assignees.map(l => ({ login: l })),
    labels: labels.map(l => ({ name: l })),
    pull_request: undefined,
  };
}

function makePR(number, { createdAt = daysAgo(1), assignees = [], labels = [], body = '', authorLogin = 'contributor' } = {}) {
  return {
    number,
    state: 'open',
    created_at: createdAt,
    user: { login: authorLogin, type: 'User' },
    assignees: assignees.map(l => ({ login: l })),
    labels: labels.map(l => ({ name: l })),
    body,
  };
}

function makeUnlabeledEvent(labelName, createdAt) {
  return { event: 'unlabeled', created_at: createdAt, label: { name: labelName } };
}

function makeLabeledEvent(labelName, createdAt) {
  return { event: 'labeled', created_at: createdAt, label: { name: labelName } };
}

function makeAssignedEvent(createdAt) {
  return { event: 'assigned', created_at: createdAt };
}

function makeComment(userLogin, createdAt, { isBot = false } = {}) {
  return {
    id: Math.floor(Math.random() * 100000),
    user: { login: userLogin, type: isBot ? 'Bot' : 'User' },
    body: `Comment from ${userLogin}`,
    created_at: createdAt,
  };
}

function makeCommit(authorLogin, date) {
  return {
    author: { login: authorLogin },
    commit: {
      author: { date },
      committer: { date },
      message: 'chore: some work',
      verification: { verified: true },
    },
  };
}

const defaultContext = {
  repo: { owner: 'test-org', repo: 'test-repo' },
};

// =============================================================================
// SCENARIOS
// =============================================================================

const scenarios = [
  // ── 1 ──────────────────────────────────────────────────────────────────────
  {
    name: 'No in-progress items — no action taken',
    description: 'When there are no assigned issues or PRs, the bot should be silent.',
    github: createMockGithub(),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 2 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 3 days inactive — no action',
    description: 'An issue created 3 days ago with no comments should not be flagged.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(10, { createdAt: daysAgo(3), assignees: ['alice'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        10: [makeAssignedEvent(daysAgo(3))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
      summaryLogs: ['#10 (issue): last activity 3d ago (assigned: alice), no action needed'],
    },
  },

  // ── 3 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 6 days inactive — warning posted',
    description: 'An issue with no activity for 6 days should receive a warning comment.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(20, { createdAt: daysAgo(6), assignees: ['alice'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        20: [makeAssignedEvent(daysAgo(6))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreatedCount: 1,
      warningPostedOn: [20],
      labelsAdded: 0,
      assigneesRemoved: 0,
      summaryLogs: ['#20 (issue): last activity 6d ago (assigned: alice), posting inactivity warning'],
    },
  },

  // ── 4 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 8 days inactive — reset (not closed)',
    description: 'An issue with no activity for 8 days should be reset but remain open.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(30, { createdAt: daysAgo(8), assignees: ['alice'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        30: [makeAssignedEvent(daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      resetCommentOn: [30],
      labelsAdded: [{ issue_number: 30, labels: [LABELS.READY_FOR_DEV] }],
      labelsRemoved: [{ issue_number: 30, name: LABELS.IN_PROGRESS }],
      assigneesRemoved: [{ issue_number: 30, assignees: ['alice'] }],
      summaryLogs: ['#30 (issue): last activity 8d ago (assigned: alice), unassigning and resetting issue'],
    },
  },

  // ── 5 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 8 days old but assignee commented 2 days ago — no action',
    description: 'An assignee comment resets the inactivity clock.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(40, { createdAt: daysAgo(8), assignees: ['bob'], labels: [LABELS.IN_PROGRESS] }),
      ],
      commentsByNumber: {
        40: [makeComment('bob', daysAgo(2))],
      },
      eventsByNumber: {
        40: [makeAssignedEvent(daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 6 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 8 days old, non-assignee commented — clock not reset, reset (not closed)',
    description: 'A comment by a non-assignee (e.g. maintainer) should not reset the clock.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(50, { createdAt: daysAgo(8), assignees: ['alice'], labels: [LABELS.IN_PROGRESS] }),
      ],
      commentsByNumber: {
        50: [makeComment('maintainer', daysAgo(1))],
      },
      eventsByNumber: {
        50: [makeAssignedEvent(daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      resetCommentOn: [50],
      assigneesRemoved: [{ issue_number: 50, assignees: ['alice'] }],
    },
  },

  // ── 7 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: blocked label — skipped for inactivity, receives check-in',
    description: 'Blocked issues are exempt from close/warn, but get a 30-day check-in comment.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(60, {
          createdAt: daysAgo(8),
          assignees: ['alice'],
          labels: [LABELS.IN_PROGRESS, LABELS.BLOCKED],
        }),
      ],
    }),
    expect: {
      itemsClosed: [],
      checkinPostedOn: [60],
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 8 ──────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: 8 days inactive but linked open PR has recent author commit — no action',
    description: 'Activity on a linked PR (author commit 1 day ago) should protect the issue.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(70, { createdAt: daysAgo(8), assignees: ['carol'], labels: [LABELS.IN_PROGRESS] }),
      ],
      openPRs: [
        makePR(71, {
          createdAt: daysAgo(8),
          assignees: ['carol'],
          authorLogin: 'carol',
          body: 'Fixes #70',
        }),
      ],
      commitsByPRNumber: {
        71: [makeCommit('carol', daysAgo(1))],
      },
      eventsByNumber: {
        70: [makeAssignedEvent(daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 9 ──────────────────────────────────────────────────────────────────────
  {
    name: 'PR: 6 days inactive — warning posted',
    description: 'An assigned PR with no activity for 6 days should receive a warning.',
    github: createMockGithub({
      openPRs: [
        makePR(80, { createdAt: daysAgo(6), assignees: ['dave'], authorLogin: 'dave' }),
      ],
    }),
    expect: {
      itemsClosed: [],
      commentsCreatedCount: 1,
      warningPostedOn: [80],
      labelsAdded: 0,
      assigneesRemoved: 0,
      summaryLogs: ['#80 (PR): last activity 6d ago (assigned: dave), posting inactivity warning'],
    },
  },

  // ── 10 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: 8 days inactive — closed, reset, linked issue cleaned up',
    description: 'A stale PR should be closed and its linked issue unassigned and reset.',
    github: createMockGithub({
      openPRs: [
        makePR(90, {
          createdAt: daysAgo(8),
          assignees: ['eve'],
          authorLogin: 'eve',
          body: 'Fixes #91',
        }),
      ],
      issuesByNumber: {
        91: {
          number: 91,
          state: 'open',
          assignees: [{ login: 'eve' }],
          labels: [{ name: LABELS.IN_PROGRESS }],
          created_at: daysAgo(8),
        },
      },
    }),
    expect: {
      itemsClosed: [90],
      closureCommentOn: [90],
      linkedIssueCleaned: [91],
      assigneesRemovedOn: [90, 91],
      summaryLogs: ['#90 (PR): last activity 8d ago (assigned: eve), closing PR'],
    },
  },

  // ── 11 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: blocked label — skipped for inactivity, receives check-in',
    description: 'Blocked PRs are exempt from close/warn, but get a 30-day check-in comment.',
    github: createMockGithub({
      openPRs: [
        makePR(100, {
          createdAt: daysAgo(8),
          assignees: ['frank'],
          authorLogin: 'frank',
          labels: [LABELS.BLOCKED],
        }),
      ],
    }),
    expect: {
      itemsClosed: [],
      checkinPostedOn: [100],
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 12 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: 8 days old but author committed 1 day ago — no action',
    description: 'A recent commit by the PR author resets the inactivity clock.',
    github: createMockGithub({
      openPRs: [
        makePR(110, { createdAt: daysAgo(8), assignees: ['grace'], authorLogin: 'grace' }),
      ],
      commitsByPRNumber: {
        110: [makeCommit('grace', daysAgo(1))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 13 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: 8 days old, commit by different author — clock not reset, closed',
    description: 'A commit by someone other than the PR author must not reset the clock.',
    github: createMockGithub({
      openPRs: [
        makePR(120, { createdAt: daysAgo(8), assignees: ['henry'], authorLogin: 'henry' }),
      ],
      commitsByPRNumber: {
        // Commit is by a maintainer, not the PR author
        120: [makeCommit('maintainer', daysAgo(1))],
      },
    }),
    expect: {
      itemsClosed: [120],
      closureCommentOn: [120],
      assigneesRemoved: [{ issue_number: 120, assignees: ['henry'] }],
    },
  },

  // ── 14 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Unassigned PR — not tracked for inactivity',
    description: 'Open PRs without assignees should not be processed.',
    github: createMockGithub({
      openPRs: [
        makePR(130, { createdAt: daysAgo(8), assignees: [], authorLogin: 'ivan' }),
      ],
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 15 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: unblocked 3 days ago (was 8 days old) — no action',
    description: 'Removing the blocked label resets the 5-day clock.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(140, { createdAt: daysAgo(8), assignees: ['judy'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        140: [makeAssignedEvent(daysAgo(8)), makeUnlabeledEvent(LABELS.BLOCKED, daysAgo(3))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 16 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: unblocked 8 days ago — reset (not closed)',
    description: 'If the unblocked date is still more than 7 days ago, the issue should be reset but remain open.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(150, { createdAt: daysAgo(10), assignees: ['kate'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        150: [makeAssignedEvent(daysAgo(10)), makeUnlabeledEvent(LABELS.BLOCKED, daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      resetCommentOn: [150],
      assigneesRemoved: [{ issue_number: 150, assignees: ['kate'] }],
    },
  },

  // ── 17 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: blocked, no prior check-in — check-in comment posted',
    description: 'First time the bot sees a blocked item, it should post a check-in.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(160, {
          createdAt: daysAgo(35),
          assignees: ['liam'],
          labels: [LABELS.IN_PROGRESS, LABELS.BLOCKED],
        }),
      ],
    }),
    expect: {
      itemsClosed: [],
      checkinPostedOn: [160],
      assigneesRemoved: 0,
    },
  },

  // ── 18 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: blocked, check-in posted 35 days ago — new check-in posted',
    description: 'After 30 days the check-in comment should be refreshed.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(170, {
          createdAt: daysAgo(40),
          assignees: ['mia'],
          labels: [LABELS.IN_PROGRESS, LABELS.BLOCKED],
        }),
      ],
      commentsByNumber: {
        170: [{
          id: 9001,
          user: { login: 'github-actions[bot]', type: 'Bot' },
          body: '<!-- bot:blocked-checkin -->\n👋 Hey @mia, just checking in!...',
          created_at: daysAgo(35),
          updated_at: daysAgo(35),
        }],
      },
    }),
    expect: {
      itemsClosed: [],
      checkinPostedOn: [170],
      assigneesRemoved: 0,
    },
  },

  // ── 19 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: blocked, check-in posted 10 days ago — no action',
    description: 'If a check-in was posted within 30 days, the bot should stay quiet.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(180, {
          createdAt: daysAgo(40),
          assignees: ['noah'],
          labels: [LABELS.IN_PROGRESS, LABELS.BLOCKED],
        }),
      ],
      commentsByNumber: {
        180: [{
          id: 9002,
          user: { login: 'github-actions[bot]', type: 'Bot' },
          body: '<!-- bot:blocked-checkin -->\n👋 Hey @noah, just checking in!...',
          created_at: daysAgo(10),
          updated_at: daysAgo(10),
        }],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      commentsUpdated: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 20 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: blocked, no prior check-in — check-in posted',
    description: 'Blocked PRs also receive the 30-day check-in.',
    github: createMockGithub({
      openPRs: [
        makePR(190, {
          createdAt: daysAgo(35),
          assignees: ['olivia'],
          authorLogin: 'olivia',
          labels: [LABELS.BLOCKED],
        }),
      ],
    }),
    expect: {
      itemsClosed: [],
      checkinPostedOn: [190],
      assigneesRemoved: 0,
    },
  },

  // ── 21 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: created 30 days ago but assigned 2 days ago — no action',
    description: 'The inactivity clock starts from the assignment date, not creation date.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(200, { createdAt: daysAgo(30), assignees: ['pat'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        200: [makeAssignedEvent(daysAgo(2))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 22 ─────────────────────────────────────────────────────────────────────
  {
    name: 'Issue: no assigned event — skipped without error',
    description: 'If the events API returns no assigned event, the issue is skipped entirely.',
    github: createMockGithub({
      assignedIssues: [
        makeIssue(210, { createdAt: daysAgo(10), assignees: ['quinn'], labels: [LABELS.IN_PROGRESS] }),
      ],
      eventsByNumber: {
        210: [],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 23 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs review — skipped entirely',
    description: 'PRs waiting on maintainer review are exempt from inactivity tracking.',
    github: createMockGithub({
      openPRs: [
        makePR(220, {
          createdAt: daysAgo(8),
          assignees: ['rose'],
          authorLogin: 'rose',
          labels: [LABELS.NEEDS_REVIEW],
        }),
      ],
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },
  
  // ── 24 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs revision labeled 2 days ago — no action',
    description: 'Inactivity clock starts from when needs-revision was last applied; 2 days is under threshold.',
    github: createMockGithub({
      openPRs: [
        makePR(230, {
          createdAt: daysAgo(10),
          assignees: ['sam'],
          authorLogin: 'sam',
          labels: [LABELS.NEEDS_REVISION],
        }),
      ],
      eventsByNumber: {
        230: [makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(2))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 25 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs revision labeled 6 days ago — warning posted',
    description: 'Six days since needs-revision was applied triggers the 5-day warning.',
    github: createMockGithub({
      openPRs: [
        makePR(240, {
          createdAt: daysAgo(10),
          assignees: ['taylor'],
          authorLogin: 'taylor',
          labels: [LABELS.NEEDS_REVISION],
        }),
      ],
      eventsByNumber: {
        240: [makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(6))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreatedCount: 1,
      warningPostedOn: [240],
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 26 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs revision labeled 8 days ago — closed and reset',
    description: 'Eight days since needs-revision was applied exceeds the 7-day close threshold.',
    github: createMockGithub({
      openPRs: [
        makePR(250, {
          createdAt: daysAgo(10),
          assignees: ['uri'],
          authorLogin: 'uri',
          labels: [LABELS.NEEDS_REVISION],
        }),
      ],
      eventsByNumber: {
        250: [makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(8))], 
      },
    }),
    expect: {
      itemsClosed: [250],
      closureCommentOn: [250],
      assigneesRemoved: [{ issue_number: 250, assignees: ['uri'] }],
    },
  },

  // ── 27 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs revision labeled 8 days ago, author commented 2 days ago — no action',
    description: 'Author activity after the label is applied resets the clock; the bot should not close an actively-engaged PR.',
    github: createMockGithub({
      openPRs: [
        makePR(261, {
          createdAt: daysAgo(10),
          assignees: ['wren'],
          authorLogin: 'wren',
          labels: [LABELS.NEEDS_REVISION],
        }),
      ],
      commentsByNumber: {
        261: [makeComment('wren', daysAgo(2))],
      },
      eventsByNumber: {
        261: [makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(8))],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 28 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR: status: needs revision applied twice — clock uses most recent application',
    description: 'Back-and-forth review cycles must not penalize contributors; the clock resets on each new needs-revision application.',
    github: createMockGithub({
      openPRs: [
        makePR(260, {
          createdAt: daysAgo(12),
          assignees: ['vera'],
          authorLogin: 'vera',
          labels: [LABELS.NEEDS_REVISION],
        }),
      ],
      eventsByNumber: {
        260: [
          makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(10)),
          makeUnlabeledEvent(LABELS.NEEDS_REVISION, daysAgo(8)),
          makeLabeledEvent(LABELS.NEEDS_REVIEW, daysAgo(8)),
          makeUnlabeledEvent(LABELS.NEEDS_REVIEW, daysAgo(2)),
          makeLabeledEvent(LABELS.NEEDS_REVISION, daysAgo(2)),
        ],
      },
    }),
    expect: {
      itemsClosed: [],
      commentsCreated: 0,
      labelsAdded: 0,
      assigneesRemoved: 0,
    },
  },

  // ── 29 ─────────────────────────────────────────────────────────────────────
  {
    name: 'PR + issue both stale — issue not double-commented after PR loop resets it',
    description: 'When the PR loop resets a linked issue, the issues loop should skip it — no duplicate comment.',
    github: createMockGithub({
      openPRs: [
        makePR(90, {
          createdAt: daysAgo(8),
          assignees: ['eve'],
          authorLogin: 'eve',
          body: 'Fixes #91',
        }),
      ],
      assignedIssues: [
        makeIssue(91, {
          createdAt: daysAgo(8),
          assignees: ['eve'],
          labels: [LABELS.IN_PROGRESS],
        }),
      ],
      issuesByNumber: {
        91: {
          number: 91,
          state: 'open',
          assignees: [{ login: 'eve' }],
          labels: [{ name: LABELS.IN_PROGRESS }],
          created_at: daysAgo(8),
        },
      },
    }),
    expect: {
      itemsClosed: [90],
      closureCommentOn: [90],
      linkedIssueCleaned: [91],
      assigneesRemovedOn: [90, 91],
      commentsCreatedCount: 2,
    },
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runScenario(scenario, index) {
  const { name, description, github, expect: expected } = scenario;

  console.log(`\n${'─'.repeat(70)}`);
  console.log(`[${index}] ${name}`);
  if (description) console.log(`    ${description}`);

  const capturedLogs = [];
  const originalConsoleLog = console.log;
  const originalConsoleError = console.error;
  console.log = (...args) => {
    capturedLogs.push(args.map(a => String(a)).join(' '));
    originalConsoleLog(...args);
  };
  console.error = (...args) => {
    capturedLogs.push(args.map(a => String(a)).join(' '));
    originalConsoleError(...args);
  };

  try {
    await script({ github, context: defaultContext, getNow: () => NOW });
  } catch (err) {
    console.log = originalConsoleLog;
    console.error = originalConsoleError;
    console.error(`❌ Script threw: ${err.message}`);
    console.error(err.stack);
    return false;
  }
  console.log = originalConsoleLog;
  console.error = originalConsoleError;

  const { calls } = github;
  const failures = [];

  // itemsClosed
  if (expected.itemsClosed !== undefined) {
    const closed = calls.itemsClosed;
    if (JSON.stringify(closed.sort()) !== JSON.stringify(expected.itemsClosed.sort())) {
      failures.push(`itemsClosed: expected ${JSON.stringify(expected.itemsClosed)}, got ${JSON.stringify(closed)}`);
    }
  }

  // commentsCreated count
  if (expected.commentsCreated !== undefined) {
    const count = calls.commentsCreated.length + calls.commentsUpdated.length;
    if (count !== expected.commentsCreated) {
      failures.push(`comments posted: expected ${expected.commentsCreated}, got ${count}`);
    }
  }

  if (expected.commentsCreatedCount !== undefined) {
    const count = calls.commentsCreated.length;
    if (count !== expected.commentsCreatedCount) {
      failures.push(`commentsCreated count: expected ${expected.commentsCreatedCount}, got ${count}`);
    }
  }

  // warningPostedOn — check that warning marker appears in comments for these items
  if (expected.warningPostedOn) {
    for (const num of expected.warningPostedOn) {
      const MARKER = '<!-- bot:inactivity-warning -->';
      const found = calls.commentsCreated.some(c => c.issue_number === num && c.body.startsWith(MARKER))
                 || calls.commentsUpdated.some(c => c.body.startsWith(MARKER));
      if (!found) {
        failures.push(`Expected warning comment on #${num}`);
      }
    }
  }

  // closureCommentOn — check that PR closure comment appears for these items
  if (expected.closureCommentOn) {
    for (const num of expected.closureCommentOn) {
      const found = calls.commentsCreated.some(c => c.issue_number === num && c.body.includes('closed due to'));
      if (!found) {
        failures.push(`Expected closure comment on #${num}`);
      }
    }
  }

  // resetCommentOn — check that issue reset comment appears for these items (not closed)
  if (expected.resetCommentOn) {
    for (const num of expected.resetCommentOn) {
      const found = calls.commentsCreated.some(c => c.issue_number === num && c.body.includes('unassigned and reset'));
      if (!found) {
        failures.push(`Expected reset comment on #${num}`);
      }
    }
  }

  // labelsAdded
  if (expected.labelsAdded !== undefined) {
    if (typeof expected.labelsAdded === 'number') {
      if (calls.labelsAdded.length !== expected.labelsAdded) {
        failures.push(`labelsAdded count: expected ${expected.labelsAdded}, got ${calls.labelsAdded.length}`);
      }
    } else if (Array.isArray(expected.labelsAdded)) {
      for (const exp of expected.labelsAdded) {
        const found = calls.labelsAdded.some(
          a => a.issue_number === exp.issue_number &&
               JSON.stringify(a.labels) === JSON.stringify(exp.labels)
        );
        if (!found) {
          failures.push(`Expected labels ${JSON.stringify(exp.labels)} added on #${exp.issue_number}`);
        }
      }
    }
  }

  // labelsRemoved
  if (expected.labelsRemoved !== undefined) {
    if (typeof expected.labelsRemoved === 'number') {
      if (calls.labelsRemoved.length !== expected.labelsRemoved) {
        failures.push(`labelsRemoved count: expected ${expected.labelsRemoved}, got ${calls.labelsRemoved.length}`);
      }
    } else if (Array.isArray(expected.labelsRemoved)) {
      for (const exp of expected.labelsRemoved) {
        const found = calls.labelsRemoved.some(
          r => r.issue_number === exp.issue_number && r.name === exp.name
        );
        if (!found) {
          failures.push(`Expected label "${exp.name}" removed on #${exp.issue_number}`);
        }
      }
    }
  }

  // assigneesRemoved
  if (expected.assigneesRemoved !== undefined) {
    if (typeof expected.assigneesRemoved === 'number') {
      if (calls.assigneesRemoved.length !== expected.assigneesRemoved) {
        failures.push(`assigneesRemoved count: expected ${expected.assigneesRemoved}, got ${calls.assigneesRemoved.length}`);
      }
    } else if (Array.isArray(expected.assigneesRemoved)) {
      for (const exp of expected.assigneesRemoved) {
        const found = calls.assigneesRemoved.some(
          r => r.issue_number === exp.issue_number &&
               JSON.stringify(r.assignees.sort()) === JSON.stringify(exp.assignees.sort())
        );
        if (!found) {
          failures.push(`Expected assignees ${JSON.stringify(exp.assignees)} removed on #${exp.issue_number}`);
        }
      }
    }
  }

  // assigneesRemovedOn — just check items (not specific logins)
  if (expected.assigneesRemovedOn) {
    for (const num of expected.assigneesRemovedOn) {
      const found = calls.assigneesRemoved.some(r => r.issue_number === num);
      if (!found) {
        failures.push(`Expected assignees removed on #${num}`);
      }
    }
  }

  // linkedIssueCleaned — issue was reset (assignees removed AND ready-for-dev label added)
  if (expected.linkedIssueCleaned) {
    for (const num of expected.linkedIssueCleaned) {
      const unassigned = calls.assigneesRemoved.some(r => r.issue_number === num);
      const relabeled  = calls.labelsAdded.some(r => r.issue_number === num && r.labels.includes(LABELS.READY_FOR_DEV));
      if (!unassigned) failures.push(`Expected assignees removed on linked issue #${num}`);
      if (!relabeled)  failures.push(`Expected "${LABELS.READY_FOR_DEV}" added on linked issue #${num}`);
    }
  }

  // checkinPostedOn — blocked check-in marker appears in created or updated comments
  if (expected.checkinPostedOn) {
    const CHECKIN_MARKER = '<!-- bot:blocked-checkin -->';
    for (const num of expected.checkinPostedOn) {
      const found = calls.commentsCreated.some(c => c.issue_number === num && c.body.startsWith(CHECKIN_MARKER))
                 || calls.commentsUpdated.some(c => c.body.startsWith(CHECKIN_MARKER));
      if (!found) {
        failures.push(`Expected blocked check-in comment on #${num}`);
      }
    }
  }

  // commentsUpdated count
  if (expected.commentsUpdated !== undefined) {
    if (calls.commentsUpdated.length !== expected.commentsUpdated) {
      failures.push(`commentsUpdated count: expected ${expected.commentsUpdated}, got ${calls.commentsUpdated.length}`);
    }
  }

  if (expected.summaryLogs) {
    for (const expectedLine of expected.summaryLogs) {
      const found = capturedLogs.some(line => line.includes(expectedLine));
      if (!found) {
        failures.push(`Expected summary log line: ${expectedLine}`);
      }
    }
  }

  if (failures.length > 0) {
    for (const f of failures) console.error(`  ❌ ${f}`);
    return false;
  }

  console.log('  ✅ All assertions passed');
  return true;
}

// =============================================================================
// ENTRY POINT
// =============================================================================

runTestSuite('INACTIVITY BOT TEST SUITE', scenarios, runScenario);
