// SPDX-License-Identifier: Apache-2.0
//
// tests/test-api.js
//
// Unit tests for helpers/api.js (postOrUpdateComment, fetchPRCommits, swapStatusLabel, etc.).
// Run with: node .github/scripts/tests/test-api.js

const { runTestSuite } = require('./test-utils');
const {
  getBotComment,
  postOrUpdateComment,
  fetchPRCommits,
  fetchOpenPRs,
  fetchIssue,
  fetchClosingIssueNumbers,
  swapStatusLabel,
  hasLabel,
  resolveLinkedIssue,
  getHighestIssueSkillLevel,
} = require('../helpers/api');
const { LABELS } = require('../helpers/constants');
const { isSafeSearchToken } = require('../helpers/validation');

// =============================================================================
// MOCK FACTORY
// =============================================================================

function createMockBotContext(overrides = {}) {
  const calls = {
    created: [],
    updated: [],
    labelsAdded: [],
    labelsRemoved: [],
  };
  const comments = overrides.comments || [];
  return {
    botContext: {
      github: {
        rest: {
          issues: {
            listComments: async ({ page, per_page }) => {
              const start = (page - 1) * per_page;
              const slice = comments.slice(start, start + per_page);
              return { data: slice };
            },
            createComment: async (params) => {
              calls.created.push(params);
            },
            updateComment: async (params) => {
              calls.updated.push(params);
            },
            addLabels: async (params) => {
              calls.labelsAdded.push(params.labels);
            },
            removeLabel: async (params) => {
              calls.labelsRemoved.push(params.name);
            },
            get: async ({ issue_number }) => {
              const issue = (overrides.issues || {})[issue_number];
              if (!issue) throw new Error('Not Found');
              return { data: issue };
            },
          },
          pulls: {
            list: async ({ page, per_page }) => {
              const allPRs = overrides.openPRs || [];
              const start = (page - 1) * per_page;
              const slice = allPRs.slice(start, start + per_page);
              return { data: slice };
            },
            listCommits: async ({ page, per_page }) => {
              const allCommits = overrides.commits || [];
              const start = (page - 1) * per_page;
              const slice = allCommits.slice(start, start + per_page);
              return { data: slice };
            },
          },
        },
        graphql:
          overrides.graphql ||
          (async () => ({
            repository: {
              pullRequest: {
                closingIssuesReferences: { nodes: [] },
              },
            },
          })),
      },
      owner: 'test',
      repo: 'repo',
      number: 1,
      pr: overrides.pr || { labels: [] },
    },
    calls,
  };
}

// =============================================================================
// UNIT TESTS
// =============================================================================

const unitTests = [
  // ---------------------------------------------------------------------------
  // hasLabel
  // ---------------------------------------------------------------------------
  {
    name: 'hasLabel: PR with matching label object → true',
    test: () => {
      const pr = {
        labels: [{ name: 'status: needs review' }],
      };
      return hasLabel(pr, LABELS.NEEDS_REVIEW) === true;
    },
  },
  {
    name: 'hasLabel: PR with no matching label → false',
    test: () => {
      const pr = {
        labels: [{ name: 'bug' }, { name: 'enhancement' }],
      };
      return hasLabel(pr, LABELS.NEEDS_REVIEW) === false;
    },
  },
  {
    name: 'hasLabel: PR with no labels → false',
    test: () => {
      const pr = { labels: [] };
      return hasLabel(pr, LABELS.NEEDS_REVIEW) === false;
    },
  },
  {
    name: 'hasLabel: PR with null/undefined labels → false',
    test: () => {
      return (
        hasLabel({ labels: null }, LABELS.NEEDS_REVIEW) === false &&
        hasLabel({}, LABELS.NEEDS_REVIEW) === false
      );
    },
  },
  {
    name: 'hasLabel: case insensitive match → true',
    test: () => {
      const pr = {
        labels: [{ name: 'STATUS: NEEDS REVIEW' }],
      };
      return hasLabel(pr, 'status: needs review') === true;
    },
  },
  {
    name: 'hasLabel: string labels → true',
    test: () => {
      const pr = {
        labels: ['status: needs review', 'bug'],
      };
      return hasLabel(pr, LABELS.NEEDS_REVIEW) === true;
    },
  },

  // ---------------------------------------------------------------------------
  // getBotComment
  // ---------------------------------------------------------------------------
  {
    name: 'getBotComment: returns comment matched by marker',
    test: async () => {
      const marker = '<!-- bot:test -->';
      const { botContext } = createMockBotContext({
        comments: [
          { id: 1, body: 'User comment 1' },
          { id: 2, body: '<!-- bot:test -->\nBot comment' },
        ],
      });
      const result = await getBotComment(botContext, marker);
      return result !== null && result.id === 2;
    },
  },
  {
    name: 'getBotComment: returns null if no marker match',
    test: async () => {
      const marker = '<!-- bot:test -->';
      const { botContext } = createMockBotContext({
        comments: [
          { id: 1, body: 'User comment 1' },
        ],
      });
      const result = await getBotComment(botContext, marker);
      return result === null;
    },
  },
  {
    name: 'getBotComment: searches across pages',
    test: async () => {
      const marker = '<!-- bot:paged -->';
      const page1 = Array(100).fill(null).map((_, i) => ({ id: i + 1, body: `Comment ${i}` }));
      const page2 = [{ id: 101, body: '<!-- bot:paged -->\nFound on page 2' }];
      const { botContext } = createMockBotContext({
        comments: [...page1, ...page2],
      });
      const result = await getBotComment(botContext, marker);
      return result !== null && result.id === 101;
    },
  },

  // ---------------------------------------------------------------------------
  // postOrUpdateComment
  // ---------------------------------------------------------------------------
  {
    name: 'postOrUpdateComment: no existing comment → creates new',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        comments: [],
      });
      const marker = '<!-- bot:test -->';
      const body = '<!-- bot:test -->\nHello';
      const result = await postOrUpdateComment(botContext, marker, body);
      return (
        result.success === true &&
        calls.created.length === 1 &&
        calls.updated.length === 0 &&
        calls.created[0].body === body
      );
    },
  },
  {
    name: 'postOrUpdateComment: existing comment with marker → updates',
    test: async () => {
      const marker = '<!-- bot:test -->';
      const { botContext, calls } = createMockBotContext({
        comments: [
          { id: 999, body: '<!-- bot:test -->\nOld content' },
        ],
      });
      const body = '<!-- bot:test -->\nNew content';
      const result = await postOrUpdateComment(botContext, marker, body);
      return (
        result.success === true &&
        calls.created.length === 0 &&
        calls.updated.length === 1 &&
        calls.updated[0].comment_id === 999 &&
        calls.updated[0].body === body
      );
    },
  },
  {
    name: 'postOrUpdateComment: multiple comments, one has marker → updates correct one',
    test: async () => {
      const marker = '<!-- bot:test -->';
      const { botContext, calls } = createMockBotContext({
        comments: [
          { id: 1, body: 'User comment 1' },
          { id: 2, body: '<!-- bot:test -->\nBot comment' },
          { id: 3, body: 'User comment 2' },
        ],
      });
      const body = '<!-- bot:test -->\nUpdated bot';
      const result = await postOrUpdateComment(botContext, marker, body);
      return (
        result.success === true &&
        calls.updated.length === 1 &&
        calls.updated[0].comment_id === 2
      );
    },
  },
  {
    name: 'postOrUpdateComment: empty comment list → creates new',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        comments: [],
      });
      const result = await postOrUpdateComment(
        botContext,
        '<!-- bot:x -->',
        '<!-- bot:x -->\nEmpty'
      );
      return result.success === true && calls.created.length === 1 && calls.updated.length === 0;
    },
  },
  {
    name: 'postOrUpdateComment: comment on second page → finds and updates',
    test: async () => {
      const marker = '<!-- bot:paged -->';
      const page1 = Array(100)
        .fill(null)
        .map((_, i) => ({ id: i + 1, body: `Comment ${i}` }));
      const page2 = [
        { id: 101, body: '<!-- bot:paged -->\nFound on page 2' },
      ];
      const { botContext, calls } = createMockBotContext({
        comments: [...page1, ...page2],
      });
      const body = '<!-- bot:paged -->\nUpdated';
      const result = await postOrUpdateComment(botContext, marker, body);
      return (
        result.success === true &&
        calls.updated.length === 1 &&
        calls.updated[0].comment_id === 101
      );
    },
  },

  // ---------------------------------------------------------------------------
  // fetchPRCommits
  // ---------------------------------------------------------------------------
  {
    name: 'fetchPRCommits: single page (< 100 commits) → returns all',
    test: async () => {
      const commits = [
        { sha: 'a1', commit: { message: 'First' } },
        { sha: 'b2', commit: { message: 'Second' } },
      ];
      const { botContext } = createMockBotContext({ commits });
      const result = await fetchPRCommits(botContext);
      return (
        Array.isArray(result) &&
        result.length === 2 &&
        result[0].sha === 'a1' &&
        result[1].sha === 'b2'
      );
    },
  },
  {
    name: 'fetchPRCommits: multiple pages → paginates and returns all',
    test: async () => {
      const commits = Array(150)
        .fill(null)
        .map((_, i) => ({ sha: `c${i}`, commit: { message: `Commit ${i}` } }));
      const { botContext } = createMockBotContext({ commits });
      const result = await fetchPRCommits(botContext);
      return result.length === 150;
    },
  },
  {
    name: 'fetchPRCommits: empty PR → returns []',
    test: async () => {
      const { botContext } = createMockBotContext({ commits: [] });
      const result = await fetchPRCommits(botContext);
      return Array.isArray(result) && result.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // fetchOpenPRs
  // ---------------------------------------------------------------------------
  {
    name: 'fetchOpenPRs: single page (< 100 PRs) → returns all',
    test: async () => {
      const openPRs = [
        { number: 1, title: 'First PR' },
        { number: 2, title: 'Second PR' },
      ];
      const { botContext } = createMockBotContext({ openPRs });
      const result = await fetchOpenPRs(botContext);
      return (
        Array.isArray(result) &&
        result.length === 2 &&
        result[0].number === 1 &&
        result[1].number === 2
      );
    },
  },
  {
    name: 'fetchOpenPRs: multiple pages → paginates and returns all',
    test: async () => {
      const openPRs = Array(150)
        .fill(null)
        .map((_, i) => ({ number: i + 1, title: `PR ${i + 1}` }));
      const { botContext } = createMockBotContext({ openPRs });
      const result = await fetchOpenPRs(botContext);
      return result.length === 150;
    },
  },
  {
    name: 'fetchOpenPRs: zero PRs → returns []',
    test: async () => {
      const { botContext } = createMockBotContext({ openPRs: [] });
      const result = await fetchOpenPRs(botContext);
      return Array.isArray(result) && result.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // fetchIssue
  // ---------------------------------------------------------------------------
  {
    name: 'fetchIssue: valid issue → returns issue data',
    test: async () => {
      const issueData = { number: 5, title: 'Bug report', state: 'open' };
      const { botContext } = createMockBotContext({
        issues: { 5: issueData },
      });
      const result = await fetchIssue(botContext, 5);
      return result.number === 5 && result.title === 'Bug report';
    },
  },
  {
    name: 'fetchIssue: missing issue (API throws) → throws',
    test: async () => {
      const { botContext } = createMockBotContext({ issues: {} });
      try {
        await fetchIssue(botContext, 999);
        return false;
      } catch (err) {
        return err.message === 'Not Found';
      }
    },
  },

  // ---------------------------------------------------------------------------
  // fetchClosingIssueNumbers
  // ---------------------------------------------------------------------------
  {
    name: 'fetchClosingIssueNumbers: 1 closing reference → returns [number]',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 42 }],
              },
            },
          },
        }),
      });
      const result = await fetchClosingIssueNumbers(botContext);
      return result.length === 1 && result[0] === 42;
    },
  },
  {
    name: 'fetchClosingIssueNumbers: 0 references → returns []',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: { nodes: [] },
            },
          },
        }),
      });
      const result = await fetchClosingIssueNumbers(botContext);
      return Array.isArray(result) && result.length === 0;
    },
  },
  {
    name: 'fetchClosingIssueNumbers: multiple references → returns all',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 1 }, { number: 2 }, { number: 3 }],
              },
            },
          },
        }),
      });
      const result = await fetchClosingIssueNumbers(botContext);
      return (
        result.length === 3 &&
        result[0] === 1 &&
        result[1] === 2 &&
        result[2] === 3
      );
    },
  },
  {
    name: 'fetchClosingIssueNumbers: GraphQL fails → returns [] (graceful)',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => {
          throw new Error('GraphQL error');
        },
      });
      const result = await fetchClosingIssueNumbers(botContext);
      return Array.isArray(result) && result.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // resolveLinkedIssue
  // ---------------------------------------------------------------------------
  {
    name: 'resolveLinkedIssue: no linked issues → returns null',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: { nodes: [] },
            },
          },
        }),
      });
      const result = await resolveLinkedIssue(botContext);
      return result === null;
    },
  },
  {
    name: 'resolveLinkedIssue: single linked issue with skill label → returns it',
    test: async () => {
      const issueData = { number: 10, title: 'Fix bug', labels: [{ name: LABELS.BEGINNER }] };
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: { nodes: [{ number: 10 }] },
            },
          },
        }),
        issues: { 10: issueData },
      });
      const result = await resolveLinkedIssue(botContext);
      return result !== null && result.number === 10;
    },
  },
  {
    name: 'resolveLinkedIssue: single linked issue with no skill label → returns null',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 7 }],
              },
            },
          },
        }),
        issues: {
          7: { number: 7, title: 'Issue 7', labels: [{ name: 'bug' }] },
        },
      });
      const result = await resolveLinkedIssue(botContext);
      return result === null;
    },
  },
  {
    name: 'resolveLinkedIssue: multiple linked issues with skill label → returns highest skill level',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 1 }, { number: 2 }, { number: 3 }],
              },
            },
          },
        }),
        issues: {
          1: { number: 1, title: 'GFI issue',          labels: [{ name: LABELS.GOOD_FIRST_ISSUE }] },
          2: { number: 2, title: 'Intermediate issue',  labels: [{ name: LABELS.INTERMEDIATE }] },
          3: { number: 3, title: 'Beginner issue',      labels: [{ name: LABELS.BEGINNER }] },
        },
      });
      const result = await resolveLinkedIssue(botContext);
      return result !== null && result.number === 2; // INTERMEDIATE is highest
    },
  },
  {
    name: 'resolveLinkedIssue: multiple linked issues with no skill label → returns null',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 4 }, { number: 5 }],
              },
            },
          },
        }),
        issues: {
          4: { number: 4, title: 'Issue 4', labels: [{ name: 'bug' }] },
          5: { number: 5, title: 'Issue 5', labels: [{ name: 'enhancement' }] },
        },
      });
      const result = await resolveLinkedIssue(botContext);
      return result === null;
    },
  },
  {
    name: 'resolveLinkedIssue: GraphQL fails → returns null gracefully',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => { throw new Error('GraphQL error'); },
      });
      const result = await resolveLinkedIssue(botContext);
      return result === null;
    },
  },
  {
    name: 'resolveLinkedIssue: issue fetch fails for all linked issues → returns null',
    test: async () => {
      const { botContext } = createMockBotContext({
        graphql: async () => ({
          repository: {
            pullRequest: {
              closingIssuesReferences: {
                nodes: [{ number: 999 }, { number: 998 }],
              },
            },
          },
        }),
        issues: {}, // no issues → fetchIssue throws for all
      });
      const result = await resolveLinkedIssue(botContext);
      return result === null;
    },
  },

  // ---------------------------------------------------------------------------
  // swapStatusLabel
  // ---------------------------------------------------------------------------
  {
    name: 'swapStatusLabel: allPassed true, has NEEDS_REVISION → removes revision, adds review',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [{ name: LABELS.NEEDS_REVISION }] },
      });
      await swapStatusLabel(botContext, true);
      return (
        calls.labelsRemoved.length === 1 &&
        calls.labelsRemoved[0] === LABELS.NEEDS_REVISION &&
        calls.labelsAdded.length === 1 &&
        Array.isArray(calls.labelsAdded[0]) &&
        calls.labelsAdded[0][0] === LABELS.NEEDS_REVIEW
      );
    },
  },
  {
    name: 'swapStatusLabel: allPassed true, has NEEDS_REVIEW → no-op',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [{ name: LABELS.NEEDS_REVIEW }] },
      });
      await swapStatusLabel(botContext, true);
      return calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0;
    },
  },
  {
    name: 'swapStatusLabel: allPassed true, no status label → no-op',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [{ name: 'bug' }] },
      });
      await swapStatusLabel(botContext, true);
      return calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0;
    },
  },
  {
    name: 'swapStatusLabel: allPassed false, has NEEDS_REVIEW → removes review, adds revision',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [{ name: LABELS.NEEDS_REVIEW }] },
      });
      await swapStatusLabel(botContext, false);
      return (
        calls.labelsRemoved.length === 1 &&
        calls.labelsRemoved[0] === LABELS.NEEDS_REVIEW &&
        calls.labelsAdded.length === 1 &&
        Array.isArray(calls.labelsAdded[0]) &&
        calls.labelsAdded[0][0] === LABELS.NEEDS_REVISION
      );
    },
  },
  {
    name: 'swapStatusLabel: allPassed false, has NEEDS_REVISION → no-op',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [{ name: LABELS.NEEDS_REVISION }] },
      });
      await swapStatusLabel(botContext, false);
      return calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0;
    },
  },
  {
    name: 'swapStatusLabel: allPassed false, no status label → no-op',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        pr: { labels: [] },
      });
      await swapStatusLabel(botContext, false);
      return calls.labelsRemoved.length === 0 && calls.labelsAdded.length === 0;
    },
  },

  // ---------------------------------------------------------------------------
  // SafeSearchToken
  // ---------------------------------------------------------------------------
  {
    name: 'isSafeSearchToken: dependabot[bot] → true',
    test: () => isSafeSearchToken('dependabot[bot]') === true,
  },
  {
    name: 'isSafeSearchToken: string with spaces → false',
    test: () => isSafeSearchToken('bad username') === false,
  },
  {
    name: 'isSafeSearchToken: string with angle brackets → false',
    test: () => isSafeSearchToken('bad<username>') === false,
  },
  {
    name: 'isSafeSearchToken: string with semicolon → false',
    test: () => isSafeSearchToken('bad;username') === false,
  },
  {
    name: 'isSafeSearchToken: string with brackets but not bot inside → false',
    test: () => isSafeSearchToken('bad[admin]') === false,
  },
  {
    name: 'isSafeSearchToken: string with multiple brackets → false',
    test: () => isSafeSearchToken('bad[[admin]') === false,
  },

  // ---------------------------------------------------------------------------
  // getHighestIssueSkillLevel
  // ---------------------------------------------------------------------------
  {
    name: 'getHighestIssueSkillLevel: issue with one skill label → returns that level',
    test: () => {
      const issue = { number: 1, title: 'Test', labels: [{ name: LABELS.BEGINNER }, { name: LABELS.READY_FOR_DEV }] };
      return getHighestIssueSkillLevel(issue) === LABELS.BEGINNER;
    },
  },
  {
    name: 'getHighestIssueSkillLevel: issue with no skill labels → returns null',
    test: () => {
      const issue = { number: 1, title: 'Test', labels: [{ name: 'bug' }, { name: 'enhancement' }] };
      return getHighestIssueSkillLevel(issue) === null;
    },
  },
  {
    name: 'getHighestIssueSkillLevel: issue with multiple skill labels → returns highest',
    test: () => {
      const issue = { number: 1, title: 'Test', labels: [{ name: LABELS.GOOD_FIRST_ISSUE }, { name: LABELS.BEGINNER }, { name: LABELS.INTERMEDIATE }] };
      return getHighestIssueSkillLevel(issue) === LABELS.INTERMEDIATE;
    },
  },
  {
    name: 'getHighestIssueSkillLevel: issue with empty labels → returns null',
    test: () => {
      const issue = { number: 1, title: 'Test', labels: [] };
      return getHighestIssueSkillLevel(issue) === null;
    },
  },
];
// =============================================================================
// TEST RUNNER
// =============================================================================

async function runUnitTests() {
  console.log('🔬 UNIT TESTS (api)');
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

runTestSuite('API HELPERS TEST SUITE', [], async () => true, [
  { label: 'Unit Tests', run: runUnitTests },
]);
