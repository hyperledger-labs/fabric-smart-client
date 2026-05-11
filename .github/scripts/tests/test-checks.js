// SPDX-License-Identifier: Apache-2.0
//
// tests/test-checks.js
//
// Unit tests for helpers/checks.js (DCO, GPG, merge conflict, issue link).
// Run with: node .github/scripts/tests/test-checks.js

const { runTestSuite } = require('./test-utils');
const {
  hasDCOSignoff,
  hasVerifiedGPGSignature,
  isMergeCommit,
  checkDCO,
  checkGPG,
  checkMergeConflict,
  checkIssueLink,
} = require('../helpers/checks');

// =============================================================================
// UNIT TESTS
// =============================================================================

const unitTests = [
  // ---------------------------------------------------------------------------
  // hasDCOSignoff
  // ---------------------------------------------------------------------------
  {
    name: 'hasDCOSignoff: valid sign-off Signed-off-by: Name <email>',
    test: () => hasDCOSignoff('Signed-off-by: Jane Doe <jane@example.com>') === true,
  },
  {
    name: 'hasDCOSignoff: missing sign-off',
    test: () => hasDCOSignoff('Just a commit message') === false,
  },
  {
    name: 'hasDCOSignoff: sign-off without email',
    test: () => hasDCOSignoff('Signed-off-by: Jane Doe') === false,
  },
  {
    name: 'hasDCOSignoff: case insensitive signed-off-by',
    test: () => hasDCOSignoff('signed-off-by: Jane Doe <jane@example.com>') === true,
  },
  {
    name: 'hasDCOSignoff: null message',
    test: () => hasDCOSignoff(null) === false,
  },
  {
    name: 'hasDCOSignoff: empty string',
    test: () => hasDCOSignoff('') === false,
  },
  {
    name: 'hasDCOSignoff: multiline with sign-off in trailer',
    test: () =>
      hasDCOSignoff(
        'Fix bug\n\nSome description\n\nSigned-off-by: Jane Doe <jane@example.com>'
      ) === true,
  },

  // ---------------------------------------------------------------------------
  // hasVerifiedGPGSignature
  // ---------------------------------------------------------------------------
  {
    name: 'hasVerifiedGPGSignature: verified true',
    test: () =>
      hasVerifiedGPGSignature({
        commit: { verification: { verified: true } },
      }) === true,
  },
  {
    name: 'hasVerifiedGPGSignature: verified false',
    test: () =>
      hasVerifiedGPGSignature({
        commit: { verification: { verified: false } },
      }) === false,
  },
  {
    name: 'hasVerifiedGPGSignature: no verification object',
    test: () => hasVerifiedGPGSignature({ commit: {} }) === false,
  },
  {
    name: 'hasVerifiedGPGSignature: missing commit object',
    test: () => hasVerifiedGPGSignature({}) === false,
  },
  {
    name: 'hasVerifiedGPGSignature: null commit',
    test: () => hasVerifiedGPGSignature(null) === false,
  },

  // ---------------------------------------------------------------------------
  // isMergeCommit
  // ---------------------------------------------------------------------------
  {
    name: 'isMergeCommit: commit with two parents returns true',
    test: () => isMergeCommit({ parents: [{}, {}] }) === true,
  },
  {
    name: 'isMergeCommit: commit with one parent returns false',
    test: () => isMergeCommit({ parents: [{}] }) === false,
  },
  {
    name: 'isMergeCommit: commit with no parents field returns false',
    test: () => isMergeCommit({}) === false,
  },
  {
    name: 'isMergeCommit: null commit returns false',
    test: () => isMergeCommit(null) === false,
  },
  {
    name: 'isMergeCommit: undefined commit returns false',
    test: () => isMergeCommit(undefined) === false,
  },
  {
    name: 'isMergeCommit: commit with empty parents array returns false',
    test: () => isMergeCommit({ parents: [] }) === false,
  },

  // ---------------------------------------------------------------------------
  // checkDCO
  // ---------------------------------------------------------------------------
  {
    name: 'checkDCO: all pass',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix\n\nSigned-off-by: A <a@x.com>' } },
        { sha: 'def5678', commit: { message: 'Fix2\n\nSigned-off-by: B <b@x.com>' } },
      ];
      const r = checkDCO(commits);
      return r.passed === true && r.failures.length === 0;
    },
  },
  {
    name: 'checkDCO: some fail',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix\n\nSigned-off-by: A <a@x.com>' } },
        { sha: 'def5678', commit: { message: 'No sign-off here' } },
      ];
      const r = checkDCO(commits);
      return (
        r.passed === false &&
        r.failures.length === 1 &&
        r.failures[0].sha === 'def5678' &&
        r.failures[0].message === 'No sign-off here'
      );
    },
  },
  {
    name: 'checkDCO: all fail',
    test: () => {
      const commits = [
        { sha: 'aaa1111', commit: { message: 'No sign-off' } },
        { sha: 'bbb2222', commit: { message: 'Also no sign-off' } },
      ];
      const r = checkDCO(commits);
      return r.passed === false && r.failures.length === 2;
    },
  },
  {
    name: 'checkDCO: empty commits',
    test: () => {
      const r = checkDCO([]);
      return r.passed === true && r.failures.length === 0;
    },
  },
  {
    name: 'checkDCO: merge commit without sign-off is skipped (passes)',
    test: () => {
      const commits = [
        { sha: 'merge123', parents: [{}, {}], commit: { message: 'Merge branch main into feat' } },
      ];
      const r = checkDCO(commits);
      return r.passed === true && r.failures.length === 0;
    },
  },
  {
    name: 'checkDCO: merge commit skipped, regular commits still checked',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix\n\nSigned-off-by: A <a@x.com>' } },
        { sha: 'merge56', parents: [{}, {}], commit: { message: 'Merge branch main into feat' } },
        { sha: 'def5678', commit: { message: 'No sign-off here' } },
      ];
      const r = checkDCO(commits);
      return (
        r.passed === false &&
        r.failures.length === 1 &&
        r.failures[0].sha === 'def5678'
      );
    },
  },
  {
    name: 'checkDCO: all regular commits pass with merge commit present',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix\n\nSigned-off-by: A <a@x.com>' } },
        { sha: 'merge56', parents: [{}, {}], commit: { message: 'Merge branch main into feat' } },
      ];
      const r = checkDCO(commits);
      return r.passed === true && r.failures.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // checkGPG
  // ---------------------------------------------------------------------------
  {
    name: 'checkGPG: all pass',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix', verification: { verified: true } } },
        { sha: 'def5678', commit: { message: 'Fix2', verification: { verified: true } } },
      ];
      const r = checkGPG(commits);
      return r.passed === true && r.failures.length === 0;
    },
  },
  {
    name: 'checkGPG: some fail',
    test: () => {
      const commits = [
        { sha: 'abc1234', commit: { message: 'Fix', verification: { verified: true } } },
        { sha: 'def5678', commit: { message: 'Fix2', verification: { verified: false } } },
      ];
      const r = checkGPG(commits);
      return (
        r.passed === false &&
        r.failures.length === 1 &&
        r.failures[0].sha === 'def5678'
      );
    },
  },
  {
    name: 'checkGPG: all fail',
    test: () => {
      const commits = [
        { sha: 'aaa1111', commit: { message: 'Fix', verification: { verified: false } } },
        { sha: 'bbb2222', commit: { message: 'Fix2' } },
      ];
      const r = checkGPG(commits);
      return r.passed === false && r.failures.length === 2;
    },
  },
  {
    name: 'checkGPG: empty commits',
    test: () => {
      const r = checkGPG([]);
      return r.passed === true && r.failures.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // checkMergeConflict
  // ---------------------------------------------------------------------------
  {
    name: 'checkMergeConflict: mergeable true',
    test: async () => {
      const botContext = {
        github: {
          rest: {
            pulls: {
              get: async () => ({ data: { mergeable: true, mergeable_state: 'clean' } }),
            },
          },
        },
        owner: 'o',
        repo: 'r',
        number: 1,
      };
      const r = await checkMergeConflict(botContext);
      return r.passed === true;
    },
  },
  {
    name: 'checkMergeConflict: mergeable false',
    test: async () => {
      const botContext = {
        github: {
          rest: {
            pulls: {
              get: async () => ({ data: { mergeable: false, mergeable_state: 'dirty' } }),
            },
          },
        },
        owner: 'o',
        repo: 'r',
        number: 1,
      };
      const r = await checkMergeConflict(botContext);
      return r.passed === false;
    },
  },
  {
    name: 'checkMergeConflict: mergeable null then true on retry',
    test: async () => {
      let callCount = 0;
      const botContext = {
        github: {
          rest: {
            pulls: {
              get: async () => {
                callCount++;
                return {
                  data: {
                    mergeable: callCount === 1 ? null : true,
                    mergeable_state: callCount === 1 ? 'unknown' : 'clean',
                  },
                };
              },
            },
          },
        },
        owner: 'o',
        repo: 'r',
        number: 1,
      };
      const r = await checkMergeConflict(botContext);
      return r.passed === true && callCount === 2;
    },
  },

  // ---------------------------------------------------------------------------
  // checkIssueLink
  // ---------------------------------------------------------------------------
  {
    name: 'checkIssueLink: Fixes #123 in body, author assigned',
    test: async () => {
      const ctx = {
        pr: { body: 'Fixes #123', user: { login: 'alice' } },
      };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'alice' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true && r.reason === null;
    },
  },
  {
    name: 'checkIssueLink: fixes #123 lowercase',
    test: async () => {
      const ctx = { pr: { body: 'fixes #123', user: { login: 'bob' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'bob' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Fixed #123',
    test: async () => {
      const ctx = { pr: { body: 'Fixed #123', user: { login: 'carol' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'carol' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Closes #456',
    test: async () => {
      const ctx = { pr: { body: 'Closes #456', user: { login: 'dave' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'dave' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: close #789',
    test: async () => {
      const ctx = { pr: { body: 'close #789', user: { login: 'eve' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'eve' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Closed #789',
    test: async () => {
      const ctx = { pr: { body: 'Closed #789', user: { login: 'frank' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'frank' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Resolves #100',
    test: async () => {
      const ctx = { pr: { body: 'Resolves #100', user: { login: 'grace' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'grace' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: resolve #100',
    test: async () => {
      const ctx = { pr: { body: 'resolve #100', user: { login: 'henry' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'henry' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Resolved #100',
    test: async () => {
      const ctx = { pr: { body: 'Resolved #100', user: { login: 'ivy' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'ivy' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: Related to #200',
    test: async () => {
      const ctx = { pr: { body: 'Related to #200', user: { login: 'jack' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'jack' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: related to #200 lowercase',
    test: async () => {
      const ctx = { pr: { body: 'related to #200', user: { login: 'kate' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'kate' }] });
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: multiple keywords Fixes #1 and Closes #2',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1\nCloses #2', user: { login: 'lea' } } };
      let fetched = [];
      const fetchIssue = async (_, num) => {
        fetched.push(num);
        return { title: `Issue ${num}`, assignees: [{ login: 'lea' }] };
      };
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true && fetched.includes(1) && fetched.includes(2);
    },
  },
  {
    name: 'checkIssueLink: body with #123 but no keyword, GraphQL fallback called',
    test: async () => {
      let graphqlCalled = false;
      const ctx = { pr: { body: 'Addresses issue #123', user: { login: 'mike' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'mike' }] });
      const fetchClosing = async () => {
        graphqlCalled = true;
        return [123];
      };
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return graphqlCalled === true && r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: empty body',
    test: async () => {
      const ctx = { pr: { body: '', user: { login: 'nancy' } } };
      let graphqlCalled = false;
      const fetchIssue = async () => ({});
      const fetchClosing = async () => {
        graphqlCalled = true;
        return [];
      };
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === false && r.reason === 'no_issue_linked' && graphqlCalled === true;
    },
  },
  {
    name: 'checkIssueLink: null body',
    test: async () => {
      const ctx = { pr: { body: null, user: { login: 'oscar' } } };
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, {
        fetchIssue: async () => ({}),
        fetchClosingIssueNumbers: fetchClosing,
      });
      return r.passed === false && r.reason === 'no_issue_linked';
    },
  },
  {
    name: 'checkIssueLink: GraphQL fallback returns issue',
    test: async () => {
      const ctx = { pr: { body: 'No keyword here', user: { login: 'paul' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'paul' }] });
      const fetchClosing = async () => [999];
      const r = await checkIssueLink(ctx, { fetchIssue, fetchClosingIssueNumbers: fetchClosing });
      return r.passed === true && r.issues.some(i => i.number === 999);
    },
  },
  {
    name: 'checkIssueLink: GraphQL returns 0',
    test: async () => {
      const ctx = { pr: { body: 'No keywords', user: { login: 'quinn' } } };
      const fetchClosing = async () => [];
      const r = await checkIssueLink(ctx, {
        fetchIssue: async () => ({}),
        fetchClosingIssueNumbers: fetchClosing,
      });
      return r.passed === false && r.reason === 'no_issue_linked';
    },
  },
  {
    name: 'checkIssueLink: 1 linked issue, author assigned',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1', user: { login: 'rachel' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'rachel' }] });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: 1 linked issue, different user',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1', user: { login: 'sam' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'other-user' }] });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'not_assigned';
    },
  },
  {
    name: 'checkIssueLink: 1 linked issue, no assignees',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1', user: { login: 'tina' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [] });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'not_assigned';
    },
  },
  {
    name: 'checkIssueLink: multiple issues, author assigned to first only',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1\nCloses #2', user: { login: 'uma' } } };
      const fetchIssue = async (_, num) => ({
        title: `Issue ${num}`,
        assignees: num === 1 ? [{ login: 'uma' }] : [{ login: 'other' }],
      });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'not_assigned';
    },
  },
  {
    name: 'checkIssueLink: multiple issues, author assigned to second only',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1\nCloses #2', user: { login: 'vic' } } };
      const fetchIssue = async (_, num) => ({
        title: `Issue ${num}`,
        assignees: num === 2 ? [{ login: 'vic' }] : [{ login: 'other' }],
      });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'not_assigned';
    },
  },
  {
    name: 'checkIssueLink: multiple issues, author assigned to all',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1\nCloses #2', user: { login: 'zara' } } };
      const fetchIssue = async () => ({
        title: 'Bug',
        assignees: [{ login: 'zara' }],
      });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === true && r.issues.length === 2;
    },
  },
  {
    name: 'checkIssueLink: multiple issues, author assigned to none',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1\nCloses #2', user: { login: 'wade' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'other' }] });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'not_assigned';
    },
  },
  {
    name: 'checkIssueLink: case-insensitive author check',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1', user: { login: 'Alice' } } };
      const fetchIssue = async () => ({ title: 'Bug', assignees: [{ login: 'alice' }] });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === true;
    },
  },
  {
    name: 'checkIssueLink: fetchIssue throws 404, graceful no_issue_linked',
    test: async () => {
      const ctx = { pr: { body: 'Fixes #1', user: { login: 'xavier' } } };
      const fetchIssue = async () => {
        throw new Error('Not Found');
      };
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [],
      });
      return r.passed === false && r.reason === 'no_issue_linked' && r.issues.length === 0;
    },
  },
  {
    name: 'checkIssueLink: PR with a title containing an issue number and a body with none',
    test: async () => {
      const ctx = { pr: { title: 'Feature relates to #314', body: 'Fixes #', user: { login: 'monte' } } };
      const fetchIssue = async () => ({
        title: 'Feature request',
        assignees: [{ login: 'monte' }],
      });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [314],
      });
      return r.passed === false && r.reason === 'no_issue_linked' && r.issues.length === 0;
    },
  },
  {
    name: 'checkIssueLink: PR with a title that contains no issue references and an empty body',
    test: async () => {
      const ctx = { pr: { title: 'Fix', body: 'Fixing something', user: { login: 'mark' } } };
      const fetchIssue = async () => ({
        title: 'Bug',
        assignees: [{ login: 'mark' }],
      });
      const r = await checkIssueLink(ctx, {
        fetchIssue,
        fetchClosingIssueNumbers: async () => [12],
      });
      return r.passed === true && r.issues.length === 1 && r.issues[0].number === 12;
    },
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runUnitTests() {
  console.log('🔬 UNIT TESTS (checks)');
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

runTestSuite('CHECK HELPERS TEST SUITE', [], async () => true, [
  { label: 'Unit Tests', run: runUnitTests },
]);
