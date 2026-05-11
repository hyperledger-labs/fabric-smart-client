// SPDX-License-Identifier: Apache-2.0
//
// tests/test-on-pr-open-bot.js
//
// Integration tests for bot-on-pr-open.js (opened/reopened/ready_for_review).
// Run with: node .github/scripts/tests/test-on-pr-open-bot.js

const {
  runTestSuite,
  commitDCOAndGPG,
  commitDCOFail,
  commitGPGFail,
  createMockGithub,
} = require('./test-utils');
const script = require('../bot-on-pr-open.js');
const { LABELS } = require('../helpers/constants');
const { MARKER } = require('../helpers/comments');

// =============================================================================
// DEFAULT CONTEXT
// =============================================================================

function defaultContext(overrides = {}) {
  return {
    eventName: 'pull_request_target',
    payload: {
      pull_request: {
        number: 1,
        user: { login: 'contributor', type: 'User' },
        body: 'Fixes #42',
        labels: [],
        assignees: [],
      },
    },
    repo: { owner: 'test', repo: 'repo' },
    ...overrides,
  };
}

// =============================================================================
// SCENARIOS
// =============================================================================

const scenarios = [
  // ---------------------------------------------------------------------------
  // 1. Happy path - all pass
  // ---------------------------------------------------------------------------
  {
    name: 'Happy path - all pass',
    description: 'All commits have DCO+GPG, no conflicts, issue linked and assigned.',
    context: defaultContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'Add feature')],
      mergeable: true,
      issues: {
        42: { title: 'Bug fix', assignees: [{ login: 'contributor' }] },
      },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVIEW],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentUpdated: false,
      commentIncludes: [':white_check_mark:', 'All checks passed', '@contributor'],
      commentExcludes: [':x:'],
    },
  },

  // ---------------------------------------------------------------------------
  // 2. DCO fail only
  // ---------------------------------------------------------------------------
  {
    name: 'DCO fail only',
    description: 'One commit missing DCO.',
    context: defaultContext(),
    githubOptions: {
      commits: [
        commitDCOAndGPG('abc1234', 'OK'),
        commitDCOFail('def5678', 'No sign-off here'),
      ],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **DCO Sign-off**', 'def5678', 'No sign-off here'],
    },
  },

  // ---------------------------------------------------------------------------
  // 3. GPG fail only
  // ---------------------------------------------------------------------------
  {
    name: 'GPG fail only',
    description: 'One commit missing GPG.',
    context: defaultContext(),
    githubOptions: {
      commits: [
        commitDCOAndGPG('abc1234', 'OK'),
        commitGPGFail('def5678', 'Fix bug'),
      ],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **GPG Signature**', 'def5678'],
    },
  },

  // ---------------------------------------------------------------------------
  // 4. Merge conflict only
  // ---------------------------------------------------------------------------
  {
    name: 'Merge conflict only',
    description: 'mergeable=false.',
    context: defaultContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: false,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **Merge Conflicts**', 'merge conflicts'],
    },
  },

  // ---------------------------------------------------------------------------
  // 5. Issue link not linked
  // ---------------------------------------------------------------------------
  {
    name: 'Issue link not linked',
    description: 'No issue in body, no GraphQL results.',
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Just some changes',
          labels: [],
          assignees: [],
        },
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
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **Issue Link**', 'not linked to any issue'],
    },
  },

  // ---------------------------------------------------------------------------
  // 6. Issue link not assigned
  // ---------------------------------------------------------------------------
  {
    name: 'Issue link not assigned',
    description: 'Issue linked but author not assigned.',
    context: defaultContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: {
        42: { title: 'Bug', assignees: [{ login: 'other-user' }] },
      },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **Issue Link**', 'not assigned to the following linked issues'],
    },
  },

  // ---------------------------------------------------------------------------
  // 7. Multiple failures (DCO + GPG)
  // ---------------------------------------------------------------------------
  {
    name: 'Multiple failures (DCO + GPG)',
    description: 'Both DCO and GPG fail.',
    context: defaultContext(),
    githubOptions: {
      commits: [
        commitDCOFail('abc1234', 'No DCO'),
        commitGPGFail('def5678', 'No GPG'),
      ],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **DCO Sign-off**', ':x: **GPG Signature**'],
    },
  },

  // ---------------------------------------------------------------------------
  // 8. All 4 fail
  // ---------------------------------------------------------------------------
  {
    name: 'All 4 fail',
    description: 'DCO, GPG, merge, issue link all fail.',
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: '',
          labels: [],
          assignees: [],
        },
      },
    }),
    githubOptions: {
      commits: [
        commitDCOFail('abc1234', 'No sign-off'),
        commitGPGFail('def5678', 'No GPG'),
      ],
      mergeable: false,
      issues: {},
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [
        ':x: **DCO Sign-off**',
        ':x: **GPG Signature**',
        ':x: **Merge Conflicts**',
        ':x: **Issue Link**',
      ],
    },
  },

  // ---------------------------------------------------------------------------
  // 9. Bot user skip
  // ---------------------------------------------------------------------------
  {
    name: 'Bot user skip',
    description: "PR author type='Bot'. Auto-assigned, but no checks, comments, or labels.",
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'dependabot', type: 'Bot' },
          body: 'Fixes #42',
          labels: [],
          assignees: [],
        },
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
      assignees: ['dependabot'],
      commentCreated: false,
      commentUpdated: false,
    },
  },

  // ---------------------------------------------------------------------------
  // 10. Label cleanup on reopen - was needs-revision, now all pass
  // ---------------------------------------------------------------------------
  {
    name: 'Label cleanup on reopen - was needs-revision, now all pass',
    description: 'PR has needs-revision label, all checks pass.',
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Fixes #42',
          labels: [{ name: LABELS.NEEDS_REVISION }],
          assignees: [],
        },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVIEW],
      labelsRemoved: [LABELS.NEEDS_REVISION],
      assignees: ['contributor'],
      commentCreated: true,
    },
  },

  // ---------------------------------------------------------------------------
  // 11. Label cleanup on reopen - was needs-review, now fails
  // ---------------------------------------------------------------------------
  {
    name: 'Label cleanup on reopen - was needs-review, now fails',
    description: 'PR has needs-review label, DCO fails.',
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Fixes #42',
          labels: [{ name: LABELS.NEEDS_REVIEW }],
          assignees: [],
        },
      },
    }),
    githubOptions: {
      commits: [commitDCOFail('abc1234', 'No sign-off')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVISION],
      labelsRemoved: [LABELS.NEEDS_REVIEW],
      assignees: ['contributor'],
      commentCreated: true,
      commentIncludes: [':x: **DCO Sign-off**'],
    },
  },

  // ---------------------------------------------------------------------------
  // 12. Author already assigned
  // ---------------------------------------------------------------------------
  {
    name: 'Author already assigned',
    description: 'Author in assignees list. No addAssignees call.',
    context: defaultContext({
      payload: {
        pull_request: {
          number: 1,
          user: { login: 'contributor', type: 'User' },
          body: 'Fixes #42',
          labels: [],
          assignees: [{ login: 'contributor' }],
        },
      },
    }),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVIEW],
      labelsRemoved: [],
      assignees: [],
      commentCreated: true,
    },
  },

  // ---------------------------------------------------------------------------
  // 13. Comment already exists
  // ---------------------------------------------------------------------------
  {
    name: 'Comment already exists',
    description: 'Existing bot comment. Updated, not duplicated.',
    context: defaultContext(),
    githubOptions: {
      commits: [commitDCOAndGPG('abc1234', 'OK')],
      mergeable: true,
      issues: { 42: { title: 'Bug', assignees: [{ login: 'contributor' }] } },
      graphqlClosingIssues: [],
      existingComments: [
        {
          id: 999,
          body: `${MARKER}\n\nOld comment content`,
        },
      ],
    },
    expect: {
      labelsAdded: [LABELS.NEEDS_REVIEW],
      labelsRemoved: [],
      assignees: ['contributor'],
      commentCreated: false,
      commentUpdated: true,
      commentIncludes: [':white_check_mark:', 'All checks passed'],
    },
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runTest(scenario, index) {
  console.log('\n' + '='.repeat(70));
  console.log(`TEST ${index + 1}: ${scenario.name}`);
  console.log(`Description: ${scenario.description}`);
  console.log('='.repeat(70));

  const mock = createMockGithub(scenario.githubOptions);

  // Wrap for buildBotContext: github.rest.issues, etc.
  const github = {
    rest: mock.rest,
    graphql: mock.graphql.bind(mock),
  };

  const context = scenario.context;

  try {
    await script({ github, context });
  } catch (error) {
    if (!scenario.expectError) {
      console.log(`\n❌ SCRIPT THREW ERROR: ${error.message}`);
      if (error.stack) console.log(error.stack);
      return false;
    }
  }

  const expect = scenario.expect || {};
  let passed = true;

  // Check labels added
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
  } else if (expectedLabelsAdded.length > 0) {
    console.log(`\n✅ labelsAdded: [${actualLabelsAdded.join(', ')}]`);
  }

  // Check labels removed
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
  } else if (expectedLabelsRemoved.length > 0) {
    console.log(`\n✅ labelsRemoved: [${actualLabelsRemoved.join(', ')}]`);
  }

  // Check assignees
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
  } else if (expectedAssignees.length > 0) {
    console.log(`\n✅ assignees: [${actualAssignees.join(', ')}]`);
  }

  // Check comment created
  if (expect.commentCreated === true && mock.calls.commentsCreated.length === 0) {
    console.log('\n❌ Expected comment to be created');
    passed = false;
  }
  if (expect.commentCreated === false && mock.calls.commentsCreated.length > 0) {
    console.log(`\n❌ Expected no comment created, got ${mock.calls.commentsCreated.length}`);
    passed = false;
  }

  // Check comment updated
  if (expect.commentUpdated === true && mock.calls.commentsUpdated.length === 0) {
    console.log('\n❌ Expected comment to be updated');
    passed = false;
  }
  if (expect.commentUpdated === false && mock.calls.commentsUpdated.length > 0) {
    console.log(`\n❌ Expected no comment updated, got ${mock.calls.commentsUpdated.length}`);
    passed = false;
  }

  // Check comment body content (commentsCreated stores body strings; commentsUpdated stores { body } objects)
  const commentBody =
    mock.calls.commentsCreated[0] || mock.calls.commentsUpdated[0]?.body || '';
  if (expect.commentIncludes) {
    for (const str of expect.commentIncludes) {
      if (!commentBody.includes(str)) {
        console.log(`\n❌ Comment body missing expected string: "${str}"`);
        passed = false;
      }
    }
    if (passed && expect.commentIncludes.length > 0) {
      console.log(`\n✅ Comment includes expected strings`);
    }
  }
  if (expect.commentExcludes) {
    for (const str of expect.commentExcludes) {
      if (commentBody.includes(str)) {
        console.log(`\n❌ Comment body should not contain: "${str}"`);
        passed = false;
      }
    }
  }

  if (passed) {
    console.log('\n✅ PASSED');
  }

  return passed;
}

runTestSuite('ON-PR-OPEN BOT TEST SUITE', scenarios, runTest);
