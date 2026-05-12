// SPDX-License-Identifier: Apache-2.0
//
// tests/test-recommend-issues-bot.js
//
// Unit tests for bot/bot-recommend-issues.js
// Run with: node .github/scripts/tests/test-recommend-issues-bot.js

const { runTestSuite } = require('./test-utils');
const { LABELS, SKILL_HIERARCHY, SKILL_PREREQUISITES, MAINTAINER_TEAM, PRIORITY_HIERARCHY } = require('../helpers/constants');
const {
  handleRecommendIssues,
  getRecommendedIssues,
  resolveEligibleLevel,
  detectUnlockedLevel,
} = require('../bot/bot-recommend-issues');

// =============================================================================
// MOCK FACTORY
// =============================================================================

function createMockBotContext(overrides = {}) {
  const calls = { comments: [] };

  // searchItems: null  → simulate API failure for issue search
  // searchItems: []    → API succeeds, no results
  // searchItems: [...] → API succeeds with items
  const searchItems = overrides.searchItems !== undefined ? overrides.searchItems : [];

  // closedCounts controls how many issues exist for each (label, user).
  // null simulates API failure for all calls.
  const closedCounts = overrides.closedCounts !== undefined ? overrides.closedCounts : {};

  return {
    botContext: {
      github: {
        rest: {
          issues: {
            createComment: async (params) => {
              calls.comments.push(params.body);
            },
            listForRepo: buildListForRepo(closedCounts),
          },
          search: {
            issuesAndPullRequests: async () => {
              if (searchItems === null) throw new Error('Search API error');
              return { data: { items: searchItems } };
            },
          },
        },
      },
      owner: 'test',
      repo: 'repo',
      number: 99,
      sender: overrides.sender !== undefined ? overrides.sender : { login: 'contributor' },
      issue: overrides.issue !== undefined ? overrides.issue : null,
    },
    calls,
  };
}

/**

 * Builds a mock of `github.rest.issues.listForRepo` used by
 * countIssuesByAssignee.
 *
 * For a given (assignee, label), the mock returns a list of synthetic
 * issues whose size is determined by `closedCounts["label:username"]`.
 *
 * Behavior:
 *   - `closedCounts === null` → all calls throw (simulates API failure)
 *   - `closedCounts[key] === null` → specific call throws
 *   - otherwise → returns an array of matching issues
 *
 * The returned items mimic real API responses:
 *   - include labels
 *   - exclude pull_request field (so they count as issues)
 *
 * The number of returned items is capped by `per_page`, matching how the real
 * function respects pagination limits.
 */
function buildListForRepo(closedCounts) {
  return async ({ assignee, labels, per_page }) => {
    // Simulate full API outage.
    if (closedCounts === null) {
      throw new Error('listForRepo API error');
    }

    // Only the closed + label path is modeled, which is what the eligibility
    // and unlock logic rely on.
    const label = labels ?? null;
    const key = label ? `${label}:${assignee}` : null;

    // Allow individual keys to signal failure.
    if (key !== null && closedCounts[key] === null) {
      throw new Error(`listForRepo API error for key ${key}`);
    }

    const count = (key !== null ? closedCounts[key] : 0) ?? 0;

    // Build synthetic issue objects that pass the `!item.pull_request` filter
    // and carry the requested label.
    const items = Array.from({ length: count }, (_, i) => ({
      number: i + 1,
      title: `Issue ${i + 1}`,
      html_url: `https://github.com/test/repo/issues/${i + 1}`,
      pull_request: undefined,   // marks it as a real issue, not a PR
      labels: label ? [{ name: label }] : [],
    }));

    // The real implementation paginates but the mock returns everything at
    // once since counts are small. Mimic the `{ data }` envelope.
    return { data: items.slice(0, per_page) };
  };
}

/**
 * Creates a minimal issue object with the given labels.
 * Matches the structure expected by recommendation logic.
 */
function makeIssue(labels, number = 1, createdAt = '2026-03-01T00:00:00Z') {
  return {
    number,
    title: `Issue ${number}`,
    html_url: `https://github.com/test/repo/issues/${number}`,
    labels: labels.map((name) => ({ name })),
    created_at: createdAt,
  };
}

// Skill levels derived from hierarchy order
const GFI      = SKILL_HIERARCHY[0]; // good first issue (or equivalent)
const BEGINNER = SKILL_HIERARCHY[1];
const MID      = SKILL_HIERARCHY[2];
const TOP      = SKILL_HIERARCHY[SKILL_HIERARCHY.length - 1];

// Convenience: priority levels
const CRITICAL = PRIORITY_HIERARCHY[0];
const HIGH     = PRIORITY_HIERARCHY[1];
const MEDIUM   = PRIORITY_HIERARCHY[2];
const LOW      = PRIORITY_HIERARCHY[3];

// =============================================================================
// UNIT TESTS
// =============================================================================

const unitTests = [

  // ---------------------------------------------------------------------------
  // resolveEligibleLevel
  // ---------------------------------------------------------------------------
  {
    name: 'resolveEligibleLevel: no closed issues → defaults to GFI (floor level)',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {},
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === GFI;
    },
  },
  {
    name: 'resolveEligibleLevel: meets normal prereq for BEGINNER → returns BEGINNER',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      // Enough closed GFIs to satisfy the normal check for BEGINNER.
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${prereq.requiredLabel}:contributor`]: prereq.requiredCount,
        },
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === BEGINNER;
    },
  },
  {
    name: 'resolveEligibleLevel: has 1 closed BEGINNER issue → bypass check passes for BEGINNER',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${BEGINNER}:contributor`]: 1,
        },
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === BEGINNER;
    },
  },
  {
    name: 'resolveEligibleLevel: has 1 closed MID issue → bypass check resolves to MID',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${MID}:contributor`]: 1,
        },
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === MID;
    },
  },
  {
    name: 'resolveEligibleLevel: countIssuesByAssignee API failure → degrades to GFI',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: null, // signal all API calls fail
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === GFI;
    },
  },
  {
    name: 'resolveEligibleLevel: API failure at top level → falls through to next eligible level',
    test: async () => {
      const midPrereq = SKILL_PREREQUISITES[MID];
      const { botContext } = createMockBotContext({
        closedCounts: {
          // Simulate failure for the TOP-level check, pass for MID.
          [`${TOP}:contributor`]: null,
          [`${midPrereq.requiredLabel}:contributor`]: midPrereq.requiredCount,
        },
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === MID;
    },
  },
  {
    name: 'resolveEligibleLevel: meets prereq for MID but not TOP → returns MID',
    test: async () => {
      const midPrereq = SKILL_PREREQUISITES[MID];
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${midPrereq.requiredLabel}:contributor`]: midPrereq.requiredCount,
        },
      });
      const level = await resolveEligibleLevel(botContext, 'contributor');
      return level === MID;
    },
  },

  // ---------------------------------------------------------------------------
  // detectUnlockedLevel
  // ---------------------------------------------------------------------------
  {
    name: 'detectUnlockedLevel: count exactly equals threshold → returns next level',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${GFI}:contributor`]: prereq.requiredCount,
        },
      });
      const unlocked = await detectUnlockedLevel(botContext, 'contributor', GFI);
      return unlocked === BEGINNER;
    },
  },
  {
    name: 'detectUnlockedLevel: count below threshold → returns null',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${GFI}:contributor`]: prereq.requiredCount - 1,
        },
      });
      const unlocked = await detectUnlockedLevel(botContext, 'contributor', GFI);
      return unlocked === null;
    },
  },
  {
    name: 'detectUnlockedLevel: count above threshold → returns null (already crossed before)',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          // The mock returns raw counts, so values greater than requiredCount
          // represent cases where the threshold was already exceeded.
          [`${GFI}:contributor`]: prereq.requiredCount + 1,
        },
      });
      const unlocked = await detectUnlockedLevel(botContext, 'contributor', GFI);
      return unlocked === null;
    },
  },
  {
    name: 'detectUnlockedLevel: currentLevel is TOP → returns null (no higher level)',
    test: async () => {
      const { botContext } = createMockBotContext();
      const unlocked = await detectUnlockedLevel(botContext, 'contributor', TOP);
      return unlocked === null;
    },
  },
  {
    name: 'detectUnlockedLevel: API failure → returns null',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: null,
      });
      const unlocked = await detectUnlockedLevel(botContext, 'contributor', GFI);
      return unlocked === null;
    },
  },

  // ---------------------------------------------------------------------------
  // getRecommendedIssues — returns { issues, unlockedLevel }
  // ---------------------------------------------------------------------------
  {
    name: 'getRecommendedIssues: eligible for BEGINNER, BEGINNER issues exist → returns them',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${prereq.requiredLabel}:contributor`]: prereq.requiredCount,
        },
        searchItems: [
          makeIssue([BEGINNER, LABELS.READY_FOR_DEV], 1),
          makeIssue([MID,      LABELS.READY_FOR_DEV], 2),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return (
        result !== null &&
        result.issues.length === 1 &&
        result.issues[0].number === 1
      );
    },
  },
  {
    name: 'getRecommendedIssues: eligible for GFI → only GFI issues returned',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {},
        searchItems: [
          makeIssue([GFI,      LABELS.READY_FOR_DEV], 3),
          makeIssue([BEGINNER, LABELS.READY_FOR_DEV], 4),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return (
        result !== null &&
        result.issues.length === 1 &&
        result.issues[0].number === 3
      );
    },
  },
  {
    name: 'getRecommendedIssues: no issues at eligible level → returns empty array',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {},
        searchItems: [
          makeIssue([BEGINNER, LABELS.READY_FOR_DEV], 5),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return result !== null && result.issues.length === 0;
    },
  },
  {
    name: 'getRecommendedIssues: API failure on issue search → posts error comment, returns null',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        sender: { login: 'user' },
        issue: makeIssue([BEGINNER]),
        closedCounts: {},
        searchItems: null,
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      const expected = [
        '👋 Hi @user!',
        '',
        'I ran into an issue while generating recommendations for you.',
        '',
        `${MAINTAINER_TEAM} — could you please take a look?`,
        '',
        'Sorry for the inconvenience — feel free to explore open issues in the meantime!',
      ].join('\n');
      return (
        result === null &&
        calls.comments.length === 1 &&
        calls.comments[0] === expected
      );
    },
  },
  {
    name: 'getRecommendedIssues: caps results at 5',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          [`${prereq.requiredLabel}:contributor`]: prereq.requiredCount,
        },
        searchItems: Array.from({ length: 10 }, (_, i) =>
          makeIssue([BEGINNER, LABELS.READY_FOR_DEV], i + 1)
        ),
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return result !== null && result.issues.length <= 5;
    },
  },
  {
    name: 'getRecommendedIssues: threshold crossed → unlockedLevel is set',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext } = createMockBotContext({
        closedCounts: {
          // Exactly meets normal prereq for BEGINNER (resolveEligibleLevel) AND
          // exactly equals threshold for detectUnlockedLevel.
          [`${GFI}:contributor`]: prereq.requiredCount,
        },
        searchItems: [makeIssue([BEGINNER, LABELS.READY_FOR_DEV], 1)],
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return result !== null && result.unlockedLevel === BEGINNER;
    },
  },
  {
    name: 'getRecommendedIssues: threshold not crossed → unlockedLevel is null',
    test: async () => {
      const { botContext } = createMockBotContext({
        closedCounts: {},
        searchItems: [makeIssue([GFI, LABELS.READY_FOR_DEV], 1)],
      });
      const result = await getRecommendedIssues(botContext, 'contributor', GFI);
      return result !== null && result.unlockedLevel === null;
    },
  },

  // ---------------------------------------------------------------------------
  // getRecommendedIssues — Priority and Tiebreaker logic
  // ---------------------------------------------------------------------------
  {
    name: 'getRecommendedIssues: Critical > High priority at same level',
    test: async () => {
      const { botContext } = createMockBotContext({
        sender: { login: 'user' },
        closedCounts: { [`${MID}:user`]: 1 },
        searchItems: [
          makeIssue([MID, LABELS.READY_FOR_DEV, HIGH], 1),
          makeIssue([MID, LABELS.READY_FOR_DEV, CRITICAL], 2),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      // Even though Issue 1 was "found" first by search, Issue 2 should be recommended first
      return result.issues[0].number === 2 && result.issues[1].number === 1;
    },
  },
  {
    name: 'getRecommendedIssues: High > Medium priority at same level',
    test: async () => {
      const { botContext } = createMockBotContext({
        sender: { login: 'user' },
        closedCounts: { [`${MID}:user`]: 1 },
        searchItems: [
          makeIssue([MID, LABELS.READY_FOR_DEV, MEDIUM], 1),
          makeIssue([MID, LABELS.READY_FOR_DEV, HIGH], 2),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      return result.issues[0].number === 2 && result.issues[1].number === 1;
    },
  },
  {
    name: 'getRecommendedIssues: Medium > Low priority at same level',
    test: async () => {
      const { botContext } = createMockBotContext({
        sender: { login: 'user' },
        closedCounts: { [`${MID}:user`]: 1 },
        searchItems: [
          makeIssue([MID, LABELS.READY_FOR_DEV, LOW], 1),
          makeIssue([MID, LABELS.READY_FOR_DEV, MEDIUM], 2),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      return result.issues[0].number === 2 && result.issues[1].number === 1;
    },
  },
  {
    name: 'getRecommendedIssues: Unlabeled issues appear after all priority labels',
    test: async () => {
      const { botContext } = createMockBotContext({
        sender: { login: 'user' },
        closedCounts: { [`${MID}:user`]: 1 },
        searchItems: [
          makeIssue([MID, LABELS.READY_FOR_DEV], 1), // No priority label
          makeIssue([MID, LABELS.READY_FOR_DEV, LOW], 2),
        ],
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      return result.issues[0].number === 2 && result.issues[1].number === 1;
    },
  },
  {
    name: 'getRecommendedIssues: Same priority tiebreaker → older issue first',
    test: async () => {
      const { botContext } = createMockBotContext({
        sender: { login: 'user' },
        closedCounts: { [`${MID}:user`]: 1 },
        searchItems: [
          makeIssue([MID, LABELS.READY_FOR_DEV, HIGH], 1, '2026-04-01T00:00:00Z'), // Newer
          makeIssue([MID, LABELS.READY_FOR_DEV, HIGH], 2, '2026-01-01T00:00:00Z'), // Older
        ],
      });
      const result = await getRecommendedIssues(botContext, 'user', BEGINNER);
      // Issue 2 is older, so it should be the top recommendation
      return result.issues[0].number === 2 && result.issues[1].number === 1;
    },
  },

  // ---------------------------------------------------------------------------
  // handleRecommendIssues — short-circuit cases
  // ---------------------------------------------------------------------------
  {
    name: 'handleRecommendIssues: missing sender → skips silently',
    test: async () => {
      const { botContext, calls } = createMockBotContext({ sender: null });
      await handleRecommendIssues(botContext);
      return calls.comments.length === 0;
    },
  },
  {
    name: 'handleRecommendIssues: missing issue → skips silently',
    test: async () => {
      const { botContext, calls } = createMockBotContext({ issue: null });
      await handleRecommendIssues(botContext);
      return calls.comments.length === 0;
    },
  },
  {
    name: 'handleRecommendIssues: issue has no skill label → skips silently',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        issue: makeIssue(['bug', 'enhancement']),
      });
      await handleRecommendIssues(botContext);
      return calls.comments.length === 0;
    },
  },
  {
    name: 'handleRecommendIssues: no matching issues at eligible level → no comment',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        issue: makeIssue([BEGINNER, LABELS.READY_FOR_DEV]),
        closedCounts: {},
        searchItems: [],
      });
      await handleRecommendIssues(botContext);
      return calls.comments.length === 0;
    },
  },
  {
    name: 'handleRecommendIssues: valid context, no unlock → posts standard recommendation comment',
    test: async () => {
      const { botContext, calls } = createMockBotContext({
        sender: { login: 'user' },
        issue: makeIssue([GFI, LABELS.READY_FOR_DEV]),
        closedCounts: {},
        searchItems: [makeIssue([GFI, LABELS.READY_FOR_DEV], 1)],
      });
      await handleRecommendIssues(botContext);
      const expected = [
        '👋 Hi @user! Great work on your recent contribution! 🎉',
        '',
        'Here are some issues you might want to explore next:',
        '',
        '- [Issue 1](https://github.com/test/repo/issues/1)',
        '',
        'Happy coding! 🚀',
      ].join('\n');
      return calls.comments.length === 1 && calls.comments[0] === expected;
    },
  },
  {
    name: 'handleRecommendIssues: level unlocked → comment includes congratulatory block',
    test: async () => {
      const prereq = SKILL_PREREQUISITES[BEGINNER];
      const { botContext, calls } = createMockBotContext({
        sender: { login: 'user' },
        issue: makeIssue([GFI, LABELS.READY_FOR_DEV]),
        closedCounts: {
          [`${GFI}:user`]: prereq.requiredCount,
        },
        searchItems: [makeIssue([BEGINNER, LABELS.READY_FOR_DEV], 2)],
      });
      await handleRecommendIssues(botContext);
      return (
        calls.comments.length === 1 &&
        calls.comments[0].includes('Milestone unlocked') &&
        calls.comments[0].includes(SKILL_PREREQUISITES[BEGINNER].displayName)
      );
    },
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runUnitTests() {
  console.log('🔬 UNIT TESTS (recommend-issues)');
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

runTestSuite('RECOMMEND ISSUES TEST SUITE', [], async () => true, [
  { label: 'Unit Tests', run: runUnitTests },
]);
