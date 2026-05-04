// SPDX-License-Identifier: Apache-2.0
//
// tests/test-on-pr-update-bot.js
//
// Integration tests for bot-on-pr-update.js (synchronize + edited triggers).
// Run with: node .github/scripts/tests/test-on-pr-update-bot.js

const {
  runTestSuite,
  commitDCOAndGPG,
  commitDCOFail,
  commitMerge,
  createMockGithub,
} = require('./test-utils');
const script = require('../bot-on-pr-update.js');
const { LABELS } = require('../helpers/constants');
const { MARKER } = require('../helpers/comments');

// =============================================================================
// SYNCHRONIZE TRIGGER TESTS
// =============================================================================

/**
 * Full mock factory for synchronize tests. Builds both the GitHub API mock and
 * the context object (event = pull_request, no action/changes fields).
 */
function createSyncMock(options = {}) {
  const commits = options.commits || [];
  const mergeable = options.mergeable ?? true;
  const comments = options.comments || [];
  const prLabels = options.prLabels || [];
  const prUser = options.prUser || { login: 'alice', type: 'User' };
  const prBody = options.prBody || '';
  const assignees = options.assignees || [];
  const closingIssues = options.closingIssues || [];
  const issuesData = options.issues || {};

  const calls = {
    labelsAdded: [],
    labelsRemoved: [],
    createComment: [],
    updateComment: [],
    addAssignees: [],
  };

  const owner = 'test-owner';
  const repo = 'test-repo';
  const prNumber = options.prNumber ?? 1;

  const mockGithub = {
    rest: {
      pulls: {
        listCommits: async ({ owner: o, repo: r, pull_number, page = 1, per_page = 100 }) => {
          if (o !== owner || r !== repo || pull_number !== prNumber) {
            throw new Error('Invalid pulls.listCommits params');
          }
          const start = (page - 1) * per_page;
          return { data: commits.slice(start, start + per_page) };
        },
        get: async ({ owner: o, repo: r, pull_number }) => {
          if (o !== owner || r !== repo || pull_number !== prNumber) {
            throw new Error('Invalid pulls.get params');
          }
          return {
            data: { mergeable, mergeable_state: mergeable ? 'clean' : 'dirty' },
          };
        },
      },
      issues: {
        listComments: async ({ owner: o, repo: r, issue_number, page = 1, per_page = 100 }) => {
          if (o !== owner || r !== repo || issue_number !== prNumber) {
            throw new Error('Invalid issues.listComments params');
          }
          const start = (page - 1) * per_page;
          const slice = comments.slice(start, start + per_page);
          return { data: slice };
        },
        createComment: async (params) => {
          calls.createComment.push(params);
          return {};
        },
        updateComment: async (params) => {
          calls.updateComment.push(params);
          return {};
        },
        addLabels: async (params) => {
          calls.labelsAdded.push(params.labels);
          return {};
        },
        removeLabel: async (params) => {
          calls.labelsRemoved.push(params.name);
          return {};
        },
        addAssignees: async (params) => {
          calls.addAssignees.push(params.assignees);
          return {};
        },
        get: async ({ owner: o, repo: r, issue_number }) => {
          if (o !== owner || r !== repo) {
            throw new Error('Invalid issues.get params');
          }
          const issue = issuesData[issue_number];
          if (!issue) throw new Error('Not Found');
          return { data: issue };
        },
      },
    },
    graphql:
      options.graphql ||
      (async () => ({
        repository: {
          pullRequest: {
            closingIssuesReferences: {
              nodes: closingIssues.map((n) => ({ number: n })),
            },
          },
        },
      })),
  };

  const context = {
    eventName: options.eventName || 'pull_request',
    repo: { owner, repo },
    payload: {
      pull_request: {
        number: prNumber,
        user: prUser,
        body: prBody,
        labels: prLabels,
        assignees,
      },
    },
  };

  return { github: mockGithub, context, calls };
}

function passingCommits(count = 1) {
  return Array.from({ length: count }, (_, i) => ({
    sha: `abc${i}234567890`,
    commit: {
      message: `feat: commit ${i}\n\nSigned-off-by: Test <test@test.com>`,
      verification: { verified: true },
    },
  }));
}

function dcoFailingCommits() {
  return [
    {
      sha: 'bad1234567890',
      commit: {
        message: 'feat: forgot to sign off',
        verification: { verified: true },
      },
    },
  ];
}

function gpgFailingCommits() {
  return [
    {
      sha: 'nogpg1234567',
      commit: {
        message: 'feat: no gpg\n\nSigned-off-by: Test <test@test.com>',
        verification: { verified: false },
      },
    },
  ];
}

function issueWithAssignee(num, title, assigneeLogin) {
  return {
    number: num,
    title: title || 'Bug report',
    assignees: [{ login: assigneeLogin }],
  };
}

const syncScenarios = [
  {
    name: '[sync] Label swap: revision → review (all pass)',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVISION }],
      prUser: { login: 'alice', type: 'User' },
      prBody: 'Fixes #42',
      closingIssues: [42],
      issues: { 42: issueWithAssignee(42, 'Bug', 'alice') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVISION) &&
      calls.labelsAdded.some((arr) => arr.includes(LABELS.NEEDS_REVIEW)),
    commentIncludes: ['All checks passed', ':white_check_mark:'],
  },
  {
    name: '[sync] Label swap: review → revision (DCO fails)',
    setup: () => ({
      commits: dcoFailingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
      prUser: { login: 'bob', type: 'User' },
      prBody: 'Fixes #1',
      closingIssues: [1],
      issues: { 1: issueWithAssignee(1, 'Fix', 'bob') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVIEW) &&
      calls.labelsAdded.some((arr) => arr.includes(LABELS.NEEDS_REVISION)),
    commentIncludes: [':x:', 'DCO Sign-off'],
  },
  {
    name: '[sync] No-op: all pass, already has needs-review',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
      prUser: { login: 'carol', type: 'User' },
      prBody: 'Fixes #10',
      closingIssues: [10],
      issues: { 10: issueWithAssignee(10, 'Feature', 'carol') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0,
    commentIncludes: ['All checks passed'],
  },
  {
    name: '[sync] No-op: fail, already has needs-revision',
    setup: () => ({
      commits: dcoFailingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVISION }],
      prUser: { login: 'dave', type: 'User' },
      prBody: 'Fixes #20',
      closingIssues: [20],
      issues: { 20: issueWithAssignee(20, 'Fix', 'dave') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0,
    commentIncludes: [':x:', 'DCO Sign-off'],
  },
  {
    name: '[sync] No-op: all pass, no status labels',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: 'bug' }],
      prUser: { login: 'eve', type: 'User' },
      prBody: 'Fixes #30',
      closingIssues: [30],
      issues: { 30: issueWithAssignee(30, 'Bug', 'eve') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0,
    commentIncludes: ['All checks passed'],
  },
  {
    name: '[sync] No-op: fail, no status labels',
    setup: () => ({
      commits: gpgFailingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [],
      prUser: { login: 'frank', type: 'User' },
      prBody: 'Fixes #40',
      closingIssues: [40],
      issues: { 40: issueWithAssignee(40, 'Fix', 'frank') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0,
    commentIncludes: [':x:', 'GPG Signature'],
  },
  {
    name: '[sync] Cross-check: DCO/GPG/merge pass but issue not linked',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
      prUser: { login: 'grace', type: 'User' },
      prBody: 'No issue linked',
      closingIssues: [],
      issues: {},
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVIEW) &&
      calls.labelsAdded.some((arr) => arr.includes(LABELS.NEEDS_REVISION)),
    commentIncludes: [':x:', 'Issue Link', 'not linked to any issue'],
  },
  {
    name: '[sync] Comment updated after new commits fix issue',
    setup: () => ({
      commits: passingCommits(2),
      mergeable: true,
      comments: [
        { id: 999, body: `${MARKER}\nOld content with DCO fail` },
      ],
      prLabels: [{ name: LABELS.NEEDS_REVISION }],
      prUser: { login: 'henry', type: 'User' },
      prBody: 'Fixes #50',
      closingIssues: [50],
      issues: { 50: issueWithAssignee(50, 'Fix', 'henry') },
    }),
    verify: ({ calls }) =>
      calls.updateComment.length === 1 &&
      calls.createComment.length === 0 &&
      calls.labelsRemoved.includes(LABELS.NEEDS_REVISION) &&
      calls.labelsAdded.some((arr) => arr.includes(LABELS.NEEDS_REVIEW)),
    commentIncludes: ['All checks passed', ':white_check_mark:'],
    expectUpdate: true,
  },
  {
    name: '[sync] Bot user skip',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
      prUser: { login: 'dependabot', type: 'Bot' },
      prBody: 'Fixes #60',
      closingIssues: [60],
      issues: { 60: issueWithAssignee(60, 'Dep', 'dependabot') },
    }),
    verify: ({ calls }) =>
      calls.createComment.length === 0 &&
      calls.updateComment.length === 0 &&
      calls.labelsAdded.length === 0 &&
      calls.labelsRemoved.length === 0,
    commentIncludes: null,
    skipComment: true,
  },
  {
    name: '[sync] No auto-assign',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVISION }],
      prUser: { login: 'ivan', type: 'User' },
      prBody: 'Fixes #70',
      closingIssues: [70],
      issues: { 70: issueWithAssignee(70, 'Fix', 'ivan') },
    }),
    verify: ({ calls }) => calls.addAssignees.length === 0,
    commentIncludes: ['All checks passed'],
  },
  {
    name: '[sync] Comment already exists',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [
        { id: 111, body: `${MARKER}\nPrevious bot comment` },
      ],
      prLabels: [],
      prUser: { login: 'jane', type: 'User' },
      prBody: 'Fixes #80',
      closingIssues: [80],
      issues: { 80: issueWithAssignee(80, 'Fix', 'jane') },
    }),
    verify: ({ calls }) =>
      calls.updateComment.length === 1 &&
      calls.createComment.length === 0 &&
      calls.updateComment[0].comment_id === 111,
    commentIncludes: ['All checks passed'],
    expectUpdate: true,
  },
  {
    name: '[sync] New comment if none exists',
    setup: () => ({
      commits: passingCommits(),
      mergeable: true,
      comments: [],
      prLabels: [],
      prUser: { login: 'kate', type: 'User' },
      prBody: 'Fixes #90',
      closingIssues: [90],
      issues: { 90: issueWithAssignee(90, 'Fix', 'kate') },
    }),
    verify: ({ calls }) =>
      calls.createComment.length === 1 &&
      calls.updateComment.length === 0,
    commentIncludes: ['All checks passed'],
    expectCreate: true,
  },
  {
    name: '[sync] Merge commit without DCO sign-off is skipped',
    setup: () => ({
      commits: [
        ...passingCommits(),
        {
          sha: 'merge1234567890',
          parents: [{}, {}],
          commit: {
            message: 'Merge branch \'main\' into feat/my-feature',
            verification: { verified: true },
          },
        },
      ],
      mergeable: true,
      comments: [],
      prLabels: [{ name: LABELS.NEEDS_REVISION }],
      prUser: { login: 'larry', type: 'User' },
      prBody: 'Fixes #100',
      closingIssues: [100],
      issues: { 100: issueWithAssignee(100, 'Fix', 'larry') },
    }),
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVISION) &&
      calls.labelsAdded.some((arr) => arr.includes(LABELS.NEEDS_REVIEW)),
    commentIncludes: ['All checks passed', ':white_check_mark:'],
  },
];

async function runSyncTest(scenario, index) {
  const opts = typeof scenario.setup === 'function' ? scenario.setup() : scenario.setup;
  const { github, context, calls } = createSyncMock(opts);

  try {
    await script({ github, context });
  } catch (error) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log(`   Script threw: ${error.message}`);
    return false;
  }

  let passed = true;

  if (scenario.verify && !scenario.verify({ calls })) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log('   Verification failed');
    console.log('   labelsAdded:', JSON.stringify(calls.labelsAdded));
    console.log('   labelsRemoved:', JSON.stringify(calls.labelsRemoved));
    console.log('   createComment:', calls.createComment.length);
    console.log('   updateComment:', calls.updateComment.length);
    console.log('   addAssignees:', calls.addAssignees.length);
    passed = false;
  }

  if (!scenario.skipComment && scenario.commentIncludes) {
    const body = calls.updateComment[0]?.body ?? calls.createComment[0]?.body;
    if (!body) {
      console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
      console.log('   No comment body to verify');
      passed = false;
    } else {
      for (const needle of scenario.commentIncludes) {
        if (!body.includes(needle)) {
          console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
          console.log(`   Comment should include "${needle}"`);
          passed = false;
          break;
        }
      }
    }
  }

  if (scenario.expectUpdate && calls.updateComment.length === 0) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log('   Expected comment update, got create');
    passed = false;
  }

  if (scenario.expectCreate && calls.createComment.length === 0) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log('   Expected new comment, got update or none');
    passed = false;
  }

  return passed;
}

async function runSyncTests() {
  console.log('🔬 SYNCHRONIZE TRIGGER TESTS');
  console.log('='.repeat(70));
  let passed = 0;
  let failed = 0;
  for (let i = 0; i < syncScenarios.length; i++) {
    const ok = await runSyncTest(syncScenarios[i], i);
    if (ok) passed++;
    else failed++;
  }
  console.log('\n' + '-'.repeat(70));
  console.log(`Synchronize Tests: ${passed} passed, ${failed} failed`);
  return { total: syncScenarios.length, passed, failed };
}

// =============================================================================
// EDITED TRIGGER TESTS
// =============================================================================

function defaultEditContext(overrides = {}) {
  return {
    eventName: 'pull_request_target',
    payload: {
      action: 'edited',
      pull_request: {
        number: 1,
        user: { login: 'contributor', type: 'User' },
        body: 'Fixes #42',
        labels: [],
        assignees: [],
      },
      changes: { body: { from: 'old body' } },
    },
    repo: { owner: 'test', repo: 'repo' },
    ...overrides,
  };
}

function mergePayload(base, extras) {
  const merged = JSON.parse(JSON.stringify(base));
  const payload = merged.payload || {};
  const pr = { ...payload.pull_request, ...(extras.pull_request || {}), ...(extras.payload?.pull_request || {}) };
  const payloadOverrides = extras.payload || extras;
  const keys = Object.keys(payloadOverrides).filter((k) => k !== 'pull_request');
  keys.forEach((k) => { payload[k] = payloadOverrides[k]; });
  merged.payload = { ...payload, pull_request: pr };
  return merged;
}

const editScenarios = [
  {
    name: '[edit] Title-edit optimization - only title changed',
    description: 'changes: { title: { from: "old" } } only. No body change. Early exit.',
    context: mergePayload(defaultEditContext(), {
      changes: { title: { from: 'old' } },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'Add feature')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: false,
      commentUpdated: false,
    },
  },
  {
    name: '[edit] Body changed - all checks run',
    description: 'changes: { body: { from: "old" } }. All checks run normally.',
    context: defaultEditContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'Add feature')],
      mergeable: true,
      issues: { 42: { title: 'Bug fix', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
      commentUpdated: false,
      commentIncludes: [':white_check_mark:', 'All checks passed'],
    },
  },
  {
    name: '[edit] Label swap: revision → review',
    description: 'Body edited to add Fixes #42 (assigned). All pass. PR has needs-revision → swap to review.',
    context: mergePayload(defaultEditContext(), {
      pull_request: {
        labels: [{ name: LABELS.NEEDS_REVISION }],
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'Add feature')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVIEW],
      labelsRemoved: [LABELS.NEEDS_REVISION],
      assignees: [],
      commentCreated: true,
    },
  },
  {
    name: '[edit] Label swap: review → revision',
    description: 'Body edited to remove issue link. PR has needs-review → swap to revision.',
    context: mergePayload(defaultEditContext(), {
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Just some changes',
          labels: [{ name: LABELS.NEEDS_REVIEW }],
          assignees: [],
        },
        changes: { body: { from: 'Fixes #42' } },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: {},
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [LABELS.NEEDS_REVIEW],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':x: **Issue Link**', 'not linked to any issue'],
    },
  },
  {
    name: '[edit] No-op: already correct label',
    description: 'All pass. Already has needs-review → no label change.',
    context: mergePayload(defaultEditContext(), {
      pull_request: {
        labels: [{ name: LABELS.NEEDS_REVIEW }],
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
    },
  },
  {
    name: '[edit] No-op: fail, already has needs-revision',
    description: 'DCO fails. PR already has needs-revision → no label change.',
    context: mergePayload(defaultEditContext(), {
      pull_request: {
        labels: [{ name: LABELS.NEEDS_REVISION }],
      },
    }),
    githubOptions: {
      commits: [commitDCOFail('abc1234', 'No sign-off')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':x: **DCO Sign-off**'],
    },
  },
  {
    name: '[edit] Cross-check: issue link passes but DCO fails',
    description: 'Body adds issue link. DCO fails → stays needs-revision.',
    context: mergePayload(defaultEditContext(), {
      pull_request: {
        labels: [{ name: LABELS.NEEDS_REVISION }],
      },
    }),
    githubOptions: {
      commits: [commitDCOFail('abc1234', 'No sign-off')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':x: **DCO Sign-off**', ':white_check_mark: **Issue Link**'],
    },
  },
  {
    name: '[edit] Bot user skip',
    description: "PR author type='Bot'. No checks, no comment, no labels.",
    context: mergePayload(defaultEditContext(), {
      pull_request: {
        user: { login: 'dependabot', type: 'Bot' },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'dependabot' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: false,
      commentUpdated: false,
    },
  },
  {
    name: '[edit] No auto-assign',
    description: 'Verify no addAssignees calls on pr-edit. Issue link fails (not assigned).',
    context: mergePayload(defaultEditContext(), {
      pull_request: { labels: [{ name: LABELS.NEEDS_REVIEW }] },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [LABELS.NEEDS_REVIEW],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':x: **Issue Link**', 'not assigned to the following linked issues'],
    },
  },
  {
    name: '[edit] Comment updated',
    description: 'Existing bot comment updated, not duplicated.',
    context: defaultEditContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
      existingComments: [
        { id: 999, body: `${MARKER}\n\nOld comment content` },
      ],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: false,
      commentUpdated: true,
      commentIncludes: [':white_check_mark:', 'All checks passed'],
    },
  },
  {
    name: '[edit] Body changed from Fixes #1 to Fixes #2 (still assigned)',
    description: 'Body has Fixes #2. Author assigned to #2. Passed.',
    context: mergePayload(defaultEditContext(), {
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Fixes #2',
          labels: [],
          assignees: [],
        },
        changes: { body: { from: 'Fixes #1' } },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 2: { title: 'Issue 2', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':white_check_mark:', 'All checks passed'],
    },
  },
  {
    name: '[edit] Body changed to empty',
    description: 'Body emptied. Issue link fails (no_issue_linked). PR has needs-review → swap to revision.',
    context: mergePayload(defaultEditContext(), {
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: '',
          labels: [{ name: LABELS.NEEDS_REVIEW }],
          assignees: [],
        },
        changes: { body: { from: 'Fixes #42' } },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: {},
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [LABELS.NEEDS_REVIEW],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':x: **Issue Link**', 'not linked to any issue'],
    },
  },
  {
    name: '[edit] Merge commit without DCO sign-off is skipped',
    description: 'Mix of passing commit and merge commit (no sign-off). DCO still passes.',
    context: defaultEditContext(),
    githubOptions: {
      commits: [
        commitDCOAndGPG('abc1234', 'Add feature'),
        commitMerge('merge567', 'Merge branch \'main\' into feat/my-feature'),
      ],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
      commentIncludes: [':white_check_mark:', 'All checks passed'],
    },
  },
  {
    name: '[edit] Empty changes - early exit',
    description: 'changes: {} or no body. Early exit.',
    context: mergePayload(defaultEditContext(), {
      changes: {},
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [],
      labelsRemoved: [],
      assignees: [],
      commentCreated: false,
      commentUpdated: false,
    },
  },
];

async function runEditTest(scenario) {
  const mock = createMockGithub(scenario.githubOptions);

  const github = {
    rest: mock.rest,
    graphql: mock.graphql.bind(mock),
  };

  try {
    await script({ github, context: scenario.context });
  } catch (error) {
    if (!scenario.expectError) {
      console.log(`\n❌ SCRIPT THREW ERROR: ${error.message}`);
      if (error.stack) console.log(error.stack);
      return false;
    }
  }

  const expect = scenario.expect || {};
  let passed = true;

  const expectedLabelsAdded = expect.labelsAdded || [];
  const actualLabelsAdded = mock.calls.labelsAdded;
  if (
    expectedLabelsAdded.length !== actualLabelsAdded.length ||
    expectedLabelsAdded.some((l, i) => l !== actualLabelsAdded[i])
  ) {
    console.log(
      `\n❌ labelsAdded: expected [${expectedLabelsAdded.join(', ')}], got [${actualLabelsAdded.join(', ')}]`
    );
    passed = false;
  }

  const expectedLabelsRemoved = expect.labelsRemoved || [];
  const actualLabelsRemoved = mock.calls.labelsRemoved;
  if (
    expectedLabelsRemoved.length !== actualLabelsRemoved.length ||
    expectedLabelsRemoved.some((l, i) => l !== actualLabelsRemoved[i])
  ) {
    console.log(
      `\n❌ labelsRemoved: expected [${expectedLabelsRemoved.join(', ')}], got [${actualLabelsRemoved.join(', ')}]`
    );
    passed = false;
  }

  const expectedAssignees = expect.assignees || [];
  const actualAssignees = mock.calls.assignees;
  if (
    expectedAssignees.length !== actualAssignees.length ||
    expectedAssignees.some((a, i) => a !== actualAssignees[i])
  ) {
    console.log(
      `\n❌ assignees: expected [${expectedAssignees.join(', ')}], got [${actualAssignees.join(', ')}]`
    );
    passed = false;
  }

  if (expect.commentCreated === true && mock.calls.commentsCreated.length === 0) {
    console.log('\n❌ Expected comment to be created');
    passed = false;
  }
  if (expect.commentCreated === false && mock.calls.commentsCreated.length > 0) {
    console.log(`\n❌ Expected no comment created, got ${mock.calls.commentsCreated.length}`);
    passed = false;
  }

  if (expect.commentUpdated === true && mock.calls.commentsUpdated.length === 0) {
    console.log('\n❌ Expected comment to be updated');
    passed = false;
  }
  if (expect.commentUpdated === false && mock.calls.commentsUpdated.length > 0) {
    console.log(`\n❌ Expected no comment updated, got ${mock.calls.commentsUpdated.length}`);
    passed = false;
  }

  const commentBody =
    mock.calls.commentsCreated[0] || mock.calls.commentsUpdated[0]?.body || '';
  if (expect.commentIncludes) {
    for (const str of expect.commentIncludes) {
      if (!commentBody.includes(str)) {
        console.log(`\n❌ Comment body missing expected string: "${str}"`);
        passed = false;
      }
    }
  }

  if (passed) {
    console.log(`\n✅ PASSED`);
  }

  return passed;
}

// =============================================================================
// COMBINED RUNNER
// =============================================================================

runTestSuite('ON-PR-UPDATE BOT TEST SUITE', editScenarios, runEditTest, [
  { label: 'Synchronize Tests', run: runSyncTests },
]);
