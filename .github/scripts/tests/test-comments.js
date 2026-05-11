// SPDX-License-Identifier: Apache-2.0
//
// tests/test-comments.js
//
// Unit tests for helpers/comments.js (unified bot comment builder).
// Run with: node .github/scripts/tests/test-comments.js

const { runTestSuite } = require('./test-utils');
const { MARKER, buildBotComment, buildChecksSection, allChecksPassed, buildMergeConflictNotificationComment } = require('../helpers/comments');
const { MAINTAINER_TEAM } = require('../helpers');

// =============================================================================
// TEST DATA HELPERS
// =============================================================================

function allPassing() {
  return {
    dco: { passed: true, failures: [] },
    gpg: { passed: true, failures: [] },
    merge: { passed: true },
    issueLink: {
      passed: true,
      reason: null,
      issues: [{ number: 42, title: 'test', isAssigned: true }],
    },
  };
}

function withDCOFailed(data = allPassing()) {
  return {
    ...data,
    dco: { passed: false, failures: [{ sha: 'abc123', message: 'Fix bug' }] },
  };
}

function withDCOError(data = allPassing()) {
  return {
    ...data,
    dco: { passed: false, error: true, errorMessage: 'API timeout' },
  };
}

function withGPGFailed(data = allPassing()) {
  return {
    ...data,
    gpg: { passed: false, failures: [{ sha: 'def456', message: 'Another fix' }] },
  };
}

function withGPGError(data = allPassing()) {
  return {
    ...data,
    gpg: { passed: false, error: true, errorMessage: 'Network error' },
  };
}

function withMergeFailed(data = allPassing()) {
  return {
    ...data,
    merge: { passed: false },
  };
}


function withIssueLinkNoIssue(data = allPassing()) {
  return {
    ...data,
    issueLink: { passed: false, reason: 'no_issue_linked', issues: [] },
  };
}

function withIssueLinkNotAssigned(data = allPassing()) {
  return {
    ...data,
    issueLink: {
      passed: false,
      reason: 'not_assigned',
      issues: [{ number: 99, title: 'Bug', isAssigned: false }],
    },
  };
}


// =============================================================================
// UNIT TESTS
// =============================================================================

const unitTests = [
  // ---------------------------------------------------------------------------
  // Structure and greeting
  // ---------------------------------------------------------------------------
  {
    name: 'Comment starts with <!-- bot:pr-helper --> marker',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'alice', ...allPassing() });
      return body.startsWith('<!-- bot:pr-helper -->');
    },
  },
  {
    name: 'Greeting contains @prAuthor mention',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'bob-smith', ...allPassing() });
      return body.includes('@bob-smith');
    },
  },
  {
    name: 'Greeting contains "PR Helper Bot"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'carol', ...allPassing() });
      return body.includes('PR Helper Bot');
    },
  },
  {
    name: 'marker field equals MARKER constant',
    test: () => {
      const { marker } = buildBotComment({ prAuthor: 'dave', ...allPassing() });
      return marker === MARKER;
    },
  },
  {
    name: 'allPassed: true when all checks pass',
    test: () => {
      const { allPassed } = buildBotComment({ prAuthor: 'eve', ...allPassing() });
      return allPassed === true;
    },
  },
  {
    name: 'allPassed: false when any check fails',
    test: () => {
      const data = withDCOFailed();
      const { allPassed } = buildBotComment({ prAuthor: 'frank', ...data });
      return allPassed === false;
    },
  },

  // ---------------------------------------------------------------------------
  // DCO section
  // ---------------------------------------------------------------------------
  {
    name: 'DCO all pass: body contains :white_check_mark: and "All commits have valid sign-offs"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'gina', ...allPassing() });
      return body.includes(':white_check_mark:') && body.includes('All commits have valid sign-offs');
    },
  },
  {
    name: 'DCO failures: body contains :x: and "DCO Sign-off" and lists failing commits',
    test: () => {
      const data = withDCOFailed();
      const { body } = buildBotComment({ prAuthor: 'henry', ...data });
      return (
        body.includes(':x:') &&
        body.includes('DCO Sign-off') &&
        body.includes('abc123') &&
        body.includes('Fix bug')
      );
    },
  },
  {
    name: 'DCO failures include link to Signing Guide',
    test: () => {
      const data = withDCOFailed();
      const { body } = buildBotComment({ prAuthor: 'ivy', ...data });
      return body.includes('signing.md');
    },
  },

  // ---------------------------------------------------------------------------
  // GPG section
  // ---------------------------------------------------------------------------
  {
    name: 'GPG all pass: body contains "All commits have verified GPG signatures"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'jack', ...allPassing() });
      return body.includes('All commits have verified GPG signatures');
    },
  },
  {
    name: 'GPG failures: body contains :x: and "GPG Signature" and lists failing commits',
    test: () => {
      const data = withGPGFailed();
      const { body } = buildBotComment({ prAuthor: 'kate', ...data });
      return (
        body.includes(':x:') &&
        body.includes('GPG Signature') &&
        body.includes('def456') &&
        body.includes('Another fix')
      );
    },
  },
  {
    name: 'GPG failures include link to Signing Guide',
    test: () => {
      const data = withGPGFailed();
      const { body } = buildBotComment({ prAuthor: 'lea', ...data });
      return body.includes('signing.md');
    },
  },

  // ---------------------------------------------------------------------------
  // Merge conflict section
  // ---------------------------------------------------------------------------
  {
    name: 'Merge no conflicts: body contains "No merge conflicts detected"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'mike', ...allPassing() });
      return body.includes('No merge conflicts detected');
    },
  },
  {
    name: 'Merge conflicts: body contains :x: and "merge conflicts" and guide link',
    test: () => {
      const data = withMergeFailed();
      const { body } = buildBotComment({ prAuthor: 'nancy', ...data });
      return (
        body.includes(':x:') &&
        body.includes('merge conflicts') &&
        body.includes('merge-conflicts.md')
      );
    },
  },

  // ---------------------------------------------------------------------------
  // Issue link section
  // ---------------------------------------------------------------------------
  {
    name: 'Issue link passed: body contains "Linked to #42 (assigned to you)"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'oscar', ...allPassing() });
      return body.includes('Linked to #42') && body.includes('assigned to you');
    },
  },
  {
    name: 'Issue link reason no_issue_linked: body contains "not linked to any issue"',
    test: () => {
      const data = withIssueLinkNoIssue();
      const { body } = buildBotComment({ prAuthor: 'paul', ...data });
      return body.includes('not linked to any issue');
    },
  },
  {
    name: 'Issue link reason not_assigned: body shows unassigned issues',
    test: () => {
      const data = withIssueLinkNotAssigned();
      const { body } = buildBotComment({ prAuthor: 'quinn', ...data });
      return body.includes('not assigned to the following linked issues') && body.includes('#99');
    },
  },

  // ---------------------------------------------------------------------------
  // Footer
  // ---------------------------------------------------------------------------
  {
    name: 'Footer all pass: body contains :tada: and "All checks passed"',
    test: () => {
      const { body } = buildBotComment({ prAuthor: 'rachel', ...allPassing() });
      return body.includes(':tada:') && body.includes('All checks passed');
    },
  },
  {
    name: 'Footer any fail: body contains "All checks must pass"',
    test: () => {
      const data = withDCOFailed();
      const { body } = buildBotComment({ prAuthor: 'sam', ...data });
      return body.includes('All checks must pass');
    },
  },

  // ---------------------------------------------------------------------------
  // Error state
  // ---------------------------------------------------------------------------
  {
    name: 'DCO errored: body contains :warning: and maintainer team tag',
    test: () => {
      const data = withDCOError();
      const { body } = buildBotComment({ prAuthor: 'tina', ...data });
      return (
        body.includes(':warning:') &&
        body.includes(MAINTAINER_TEAM) &&
        body.includes('internal error')
      );
    },
  },
  {
    name: 'Multiple errors: each shows warning',
    test: () => {
      const data = withDCOError(withGPGError());
      const { body } = buildBotComment({ prAuthor: 'uma', ...data });
      const dcoWarning = body.includes(':warning: **DCO Sign-off**');
      const gpgWarning = body.includes(':warning: **GPG Signature**');
      return dcoWarning && gpgWarning;
    },
  },
  {
    name: 'Mix of error + fail + pass: correct icons for each',
    test: () => {
      const data = withDCOError(withGPGFailed(withMergeFailed(allPassing())));
      const { body } = buildBotComment({ prAuthor: 'vic', ...data });
      const hasDCOWarning = body.includes(':warning: **DCO Sign-off**');
      const hasGPGX = body.includes(':x: **GPG Signature**');
      const hasMergeX = body.includes(':x: **Merge Conflicts**');
      const hasIssueCheck = body.includes(':white_check_mark: **Issue Link**');
      return hasDCOWarning && hasGPGX && hasMergeX && hasIssueCheck;
    },
  },
  {
    name: 'allPassed false when any check errored',
    test: () => {
      const data = withDCOError();
      const { allPassed } = buildBotComment({ prAuthor: 'wade', ...data });
      return allPassed === false;
    },
  },

  // ---------------------------------------------------------------------------
  // Combinations
  // ---------------------------------------------------------------------------
  {
    name: 'Single failure (DCO only)',
    test: () => {
      const data = withDCOFailed();
      const { allPassed, body } = buildBotComment({ prAuthor: 'xander', ...data });
      return allPassed === false && body.includes(':x: **DCO Sign-off**');
    },
  },
  {
    name: 'Two failures (DCO + GPG)',
    test: () => {
      const data = withGPGFailed(withDCOFailed());
      const { allPassed, body } = buildBotComment({ prAuthor: 'yara', ...data });
      return (
        allPassed === false &&
        body.includes(':x: **DCO Sign-off**') &&
        body.includes(':x: **GPG Signature**')
      );
    },
  },
  {
    name: 'All four fail',
    test: () => {
      const data = withIssueLinkNoIssue(
        withMergeFailed(withGPGFailed(withDCOFailed()))
      );
      const { allPassed, body } = buildBotComment({ prAuthor: 'zara', ...data });
      return (
        allPassed === false &&
        body.includes(':x: **DCO Sign-off**') &&
        body.includes(':x: **GPG Signature**') &&
        body.includes(':x: **Merge Conflicts**') &&
        body.includes(':x: **Issue Link**')
      );
    },
  },
  {
    name: 'Issue link fail + all others pass',
    test: () => {
      const data = withIssueLinkNotAssigned();
      const { allPassed, body } = buildBotComment({ prAuthor: 'adam', ...data });
      const othersPass =
        body.includes(':white_check_mark: **DCO Sign-off**') &&
        body.includes(':white_check_mark: **GPG Signature**') &&
        body.includes(':white_check_mark: **Merge Conflicts**');
      const issueFails = body.includes(':x: **Issue Link**');
      return allPassed === false && othersPass && issueFails;
    },
  },

  // ---------------------------------------------------------------------------
  // buildChecksSection and allChecksPassed (direct)
  // ---------------------------------------------------------------------------
  {
    name: 'buildChecksSection: contains PR Checks heading',
    test: () => {
      const section = buildChecksSection(allPassing());
      return section.includes('### PR Checks');
    },
  },
  {
    name: 'allChecksPassed: true when all pass',
    test: () => allChecksPassed(allPassing()) === true,
  },
  {
    name: 'allChecksPassed: false when DCO fails',
    test: () => allChecksPassed(withDCOFailed()) === false,
  },
  {
    name: 'allChecksPassed: false when DCO errors',
    test: () => allChecksPassed(withDCOError()) === false,
  },
  {
    name: 'Issue link assigned: multiple issues format',
    test: () => {
      const data = allPassing();
      data.issueLink.issues = [
        { number: 1, title: 'Bug 1', isAssigned: true },
        { number: 2, title: 'Bug 2', isAssigned: true },
      ];
      const { body } = buildBotComment({ prAuthor: 'multi', ...data });
      return body.includes('#1') && body.includes('#2') && body.includes('assigned to you');
    },
  },

  // ---------------------------------------------------------------------------
  // buildMergeConflictNotificationComment
  // ---------------------------------------------------------------------------
  {
    name: 'Notification comment contains @prAuthor mention',
    test: () => {
      const comment = buildMergeConflictNotificationComment('alice', 42);
      return comment.includes('@alice');
    },
  },
  {
    name: 'Notification comment references merged PR number',
    test: () => {
      const comment = buildMergeConflictNotificationComment('bob', 123);
      return comment.includes('#123');
    },
  },
  {
    name: 'Notification comment includes wave emoji',
    test: () => {
      const comment = buildMergeConflictNotificationComment('carol', 7);
      return comment.includes(':wave:');
    },
  },
  {
    name: 'Notification comment asks to resolve merge conflict',
    test: () => {
      const comment = buildMergeConflictNotificationComment('dave', 99);
      return comment.includes('resolve the merge conflict');
    },
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runUnitTests() {
  console.log('🔬 UNIT TESTS (comments)');
  console.log('='.repeat(70));
  let passed = 0;
  let failed = 0;
  for (const test of unitTests) {
    try {
      const result = await Promise.resolve(test.test());
      if (result) {
        console.log(`✅ ${test.name}`);
        passed++;
      } else {
        console.log(`❌ ${test.name}`);
        failed++;
      }
    } catch (error) {
      console.log(`❌ ${test.name} - Error: ${error.message}`);
      failed++;
    }
  }
  console.log('\n' + '-'.repeat(70));
  console.log(`Unit Tests: ${passed} passed, ${failed} failed`);
  return { total: unitTests.length, passed, failed };
}

runTestSuite('COMMENT HELPERS TEST SUITE', [], async () => true, [
  { label: 'Unit Tests', run: runUnitTests },
]);
