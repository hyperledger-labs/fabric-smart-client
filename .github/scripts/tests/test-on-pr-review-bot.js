// SPDX-License-Identifier: Apache-2.0
//
// tests/test-on-pr-review-bot.js
//
// Integration tests for bot-on-pr-review.js (pull_request_review trigger).
// Run with: node .github/scripts/tests/test-on-pr-review-bot.js

const { runTestSuite, createMockGithub } = require('./test-utils');
const script = require('../bot-on-pr-review.js');
const { LABELS } = require('../helpers/constants');

function defaultContext(overrides = {}) {
  return {
    eventName: 'pull_request_review',
    repo: { owner: 'test-owner', repo: 'test-repo' },
    payload: {
      pull_request: {
        number: 1,
        user: { login: 'contributor' },
        labels: [],
      },
      review: {
        state: 'changes_requested',
      },
    },
    ...overrides,
  };
}

const scenarios = [
  {
    name: 'Changes requested, PR has needs-review label',
    setup: {
      state: 'changes_requested',
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
    },
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVIEW) &&
      calls.labelsAdded.includes(LABELS.NEEDS_REVISION),
  },
  {
    name: 'Changes requested, PR does not have needs-review label',
    setup: {
      state: 'changes_requested',
      prLabels: [],
    },
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 &&
      calls.labelsAdded.includes(LABELS.NEEDS_REVISION),
  },
  {
    name: 'Review approved (ignored)',
    setup: {
      state: 'approved',
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
    },
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 &&
      calls.labelsAdded.length === 0,
  },
  {
    name: 'Review commented (ignored)',
    setup: {
      state: 'commented',
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
    },
    verify: ({ calls }) =>
      calls.labelsRemoved.length === 0 &&
      calls.labelsAdded.length === 0,
  },
  {
    name: 'Uppercase state CHANGES_REQUESTED is handled',
    setup: {
      state: 'CHANGES_REQUESTED',
      prLabels: [{ name: LABELS.NEEDS_REVIEW }],
    },
    verify: ({ calls }) =>
      calls.labelsRemoved.includes(LABELS.NEEDS_REVIEW) &&
      calls.labelsAdded.includes(LABELS.NEEDS_REVISION),
  },
];

async function runScenario(scenario, index) {
  const opts = scenario.setup;

  const mock = createMockGithub();
  const github = {
    rest: mock.rest,
    graphql: mock.graphql.bind(mock),
  };

  const context = defaultContext({
    payload: {
      pull_request: {
        number: 1,
        user: { login: 'contributor' },
        labels: opts.prLabels || [],
      },
      review: {
        state: opts.state,
      },
    },
  });

  try {
    await script({ github, context });
  } catch (error) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log(`   Script threw: ${error.message}`);
    return false;
  }

  if (!scenario.verify({ calls: mock.calls })) {
    console.log(`\n❌ Scenario ${index + 1}: ${scenario.name}`);
    console.log('   Verification failed');
    console.log('   labelsAdded:', JSON.stringify(mock.calls.labelsAdded));
    console.log('   labelsRemoved:', JSON.stringify(mock.calls.labelsRemoved));
    return false;
  }

  return true;
}

if (require.main === module) {
  runTestSuite('ON-PR-REVIEW BOT TEST SUITE', scenarios, runScenario);
} else {
  module.exports = { runTestSuite: () => runTestSuite('ON-PR-REVIEW BOT TEST SUITE', scenarios, runScenario) };
}
