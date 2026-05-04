// SPDX-License-Identifier: Apache-2.0
//
// tests/test-finalize-bot.js
//
// Local test script for the /finalize command (bot-on-comment.js → commands/finalize.js).
// Run with: node .github/scripts/tests/test-finalize-bot.js
//
// Mocks the GitHub API and runs scenarios to verify /finalize behaves correctly
// without making real API calls.

const { LABELS, MAINTAINER_TEAM } = require('../helpers');
const script = require('../bot-on-comment.js');
const { runTestSuite, verifyComments } = require('./test-utils');
const {
  parseSections,
  isMeaningfulContent,
} = require('../commands/finalize-comments');

// =============================================================================
// MOCK GITHUB API
// =============================================================================

function createMockGithub(options = {}) {
  const {
    roleName = 'triage',            // role_name returned by getCollaboratorPermissionLevel
    permissionShouldFail = false,   // throw HTTP 500 on permission check
    permissionNotFound = false,     // throw HTTP 404 (non-collaborator)
    updateShouldFail = false,       // throw on issues.update
    removeLabelShouldFail = false,
    addLabelShouldFail = false,
  } = options;

  const calls = {
    comments: [],
    reactions: [],
    labelsAdded: [],
    labelsRemoved: [],
    issueUpdates: [],
    permissionChecks: [],
  };

  return {
    calls,
    rest: {
      reactions: {
        createForIssueComment: async (params) => {
          calls.reactions.push({ commentId: params.comment_id, content: params.content });
          console.log(`\n👍 REACTION ADDED: ${params.content}`);
        },
      },
      repos: {
        getCollaboratorPermissionLevel: async (params) => {
          calls.permissionChecks.push(params.username);
          console.log(`\n🔐 PERMISSION CHECK: @${params.username}`);
          if (permissionNotFound) {
            const err = new Error('Not Found');
            err.status = 404;
            throw err;
          }
          if (permissionShouldFail) {
            const err = new Error('Simulated permission check failure');
            err.status = 500;
            throw err;
          }
          console.log(`   → role_name: ${roleName}`);
          return { data: { role_name: roleName, permission: roleName } };
        },
      },
      issues: {
        createComment: async (params) => {
          calls.comments.push(params.body);
          console.log('\n📝 COMMENT POSTED:');
          console.log('─'.repeat(60));
          console.log(params.body);
          console.log('─'.repeat(60));
        },
        update: async (params) => {
          if (updateShouldFail) {
            throw new Error('Simulated issue update failure');
          }
          calls.issueUpdates.push({ title: params.title, body: params.body });
          console.log(`\n✏️  ISSUE UPDATED: title="${params.title}"`);
        },
        addLabels: async (params) => {
          if (addLabelShouldFail) {
            throw new Error('Simulated add label failure');
          }
          calls.labelsAdded.push(...params.labels);
          console.log(`\n🏷️  LABEL ADDED: ${params.labels.join(', ')}`);
        },
        removeLabel: async (params) => {
          if (removeLabelShouldFail) {
            throw new Error('Simulated remove label failure');
          }
          calls.labelsRemoved.push(params.name);
          console.log(`\n🏷️  LABEL REMOVED: ${params.name}`);
        },
      },
    },
  };
}

// =============================================================================
// HELPERS
// =============================================================================

function makeIssue(overrides = {}) {
  return {
    number: 42,
    title: 'Fix something',
    state: 'open',
    body: '### 👾 Description of the Issue\n\nThis thing is broken.\n\n### ✔️ Acceptance Criteria\n\n- [ ] Fixed',
    labels: [
      { name: LABELS.AWAITING_TRIAGE },
      { name: LABELS.BEGINNER },
      { name: 'priority: medium' },
    ],
    type: { name: 'Task' },
    assignees: [],
    ...overrides,
  };
}

function makeContext(issue, commentBody = '/finalize', commenter = 'maintainer') {
  return {
    eventName: 'issue_comment',
    payload: {
      issue,
      comment: {
        id: 9001,
        body: commentBody,
        user: { login: commenter, type: 'User' },
      },
    },
    repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
  };
}

async function runScenario(scenario, index) {
  console.log('\n' + '='.repeat(70));
  console.log(`TEST ${index + 1}: ${scenario.name}`);
  console.log(`DESC: ${scenario.description}`);
  console.log('='.repeat(70));

  const github = createMockGithub(scenario.githubOptions || {});
  const context = scenario.context;

  let threw = null;
  try {
    await script({ github, context });
  } catch (e) {
    threw = e;
  }

  let passed = true;
  const failures = [];

  // Snapshot verification for comment text
  if (scenario.expectedComments !== undefined) {
    const commentResult = verifyComments(scenario.expectedComments, github.calls.comments);
    if (!commentResult.passed) {
      passed = false;
      failures.push(...commentResult.details.filter((d) => d.startsWith('❌')));
    }
  }

  // Other behavioural assertions (labels, reactions, issue updates, body content)
  for (const assertion of scenario.assertions || []) {
    const result = assertion(github.calls, threw);
    if (result !== true) {
      passed = false;
      failures.push(result);
    }
  }

  if (passed) {
    console.log('\n✅ PASSED\n');
  } else {
    console.log('\n❌ FAILED');
    for (const f of failures) console.log('  -', f);
    console.log();
  }

  return passed;
}

// =============================================================================
// ASSERTION HELPERS
// =============================================================================

const assert = {
  commentContains: (text) => (calls) => {
    const found = calls.comments.some((c) => c.includes(text));
    return found || `Expected a comment containing: "${text}"`;
  },
  noComments: () => (calls) => calls.comments.length === 0 || `Expected no comments, got ${calls.comments.length}`,
  labelAdded: (label) => (calls) => calls.labelsAdded.includes(label) || `Expected label added: "${label}"`,
  labelRemoved: (label) => (calls) => calls.labelsRemoved.includes(label) || `Expected label removed: "${label}"`,
  noLabelsAdded: () => (calls) => calls.labelsAdded.length === 0 || `Expected no labels added, got: ${calls.labelsAdded}`,
  noLabelsRemoved: () => (calls) => calls.labelsRemoved.length === 0 || `Expected no labels removed, got: ${calls.labelsRemoved}`,
  noIssueUpdate: () => (calls) => calls.issueUpdates.length === 0 || `Expected no issue update, got ${calls.issueUpdates.length}`,
  issueUpdated: () => (calls) => calls.issueUpdates.length > 0 || 'Expected issue to be updated',
  titleContains: (text) => (calls) => {
    const found = calls.issueUpdates.some((u) => u.title && u.title.includes(text));
    return found || `Expected updated title to contain: "${text}"`;
  },
  bodyContains: (text) => (calls) => {
    const found = calls.issueUpdates.some((u) => u.body && u.body.includes(text));
    return found || `Expected updated body to contain: "${text}"`;
  },
  bodyNotContains: (text) => (calls) => {
    const found = calls.issueUpdates.some((u) => u.body && u.body.includes(text));
    return !found || `Expected updated body NOT to contain: "${text}"`;
  },
  reactionAdded: () => (calls) => calls.reactions.length > 0 || 'Expected thumbs-up reaction to be added',
};

// =============================================================================
// SCENARIOS
// =============================================================================

// =============================================================================
// EXPECTED COMMENT SNAPSHOTS
// =============================================================================
// Defined as constants so the same text can be reused across scenarios that
// produce the same comment (e.g. both unauthorized scenarios).

const COMMENT_UNAUTHORIZED = `👋 Hi @maintainer! The \`/finalize\` command is reserved for maintainers and contributors with **triage** (or higher) repository permissions.

If you believe you should have access, please reach out to a maintainer.`;

const COMMENT_PERMISSION_ERROR = `👋 Hi @maintainer! I encountered an error while trying to verify your permissions.

${MAINTAINER_TEAM} — could you please verify @maintainer's permissions and complete the finalization manually if appropriate?

Sorry for the inconvenience!`;

const COMMENT_UPDATE_FAILURE = `⚠️ Hi @maintainer! I encountered an error while trying to update the issue title or body.

${MAINTAINER_TEAM} — could you please complete the finalization manually?

Error details: Simulated issue update failure`;

const COMMENT_SWAP_FAILURE_REMOVE = `⚠️ The issue was updated successfully, but I encountered an error swapping the status labels.

${MAINTAINER_TEAM} — please manually:
- Remove the \`status: awaiting triage\` label
- Add the \`status: ready for dev\` label

Error details: Failed to remove 'status: awaiting triage': Simulated remove label failure`;

const COMMENT_SWAP_FAILURE_ADD = `⚠️ The issue was updated successfully, but I encountered an error swapping the status labels.

${MAINTAINER_TEAM} — please manually:
- Remove the \`status: awaiting triage\` label
- Add the \`status: ready for dev\` label

Error details: Failed to add 'status: ready for dev': Simulated add label failure`;

const COMMENT_SUCCESS_GFI_LOW = `✅ Issue finalized by @maintainer!

**Skill level:** \`skill: good first issue\`
**Priority:** \`priority: low\`

The issue body has been updated with the appropriate skill-level context and contribution guide. This issue is now ready for contributors to pick up via \`/assign\`.`;

const COMMENT_SUCCESS_BEGINNER_MEDIUM = `✅ Issue finalized by @maintainer!

**Skill level:** \`skill: beginner\`
**Priority:** \`priority: medium\`

The issue body has been updated with the appropriate skill-level context and contribution guide. This issue is now ready for contributors to pick up via \`/assign\`.`;

const COMMENT_SUCCESS_INTERMEDIATE_HIGH = `✅ Issue finalized by @maintainer!

**Skill level:** \`skill: intermediate\`
**Priority:** \`priority: high\`

The issue body has been updated with the appropriate skill-level context and contribution guide. This issue is now ready for contributors to pick up via \`/assign\`.`;

const COMMENT_SUCCESS_ADVANCED_MEDIUM = `✅ Issue finalized by @maintainer!

**Skill level:** \`skill: advanced\`
**Priority:** \`priority: medium\`

The issue body has been updated with the appropriate skill-level context and contribution guide. This issue is now ready for contributors to pick up via \`/assign\`.`;

/** Builds the standard validation-error comment for one or more violations. */
function validationComment(...errors) {
  const errorList = errors.map((e) => `- ${e}`).join('\n');
  return `👋 Hi @maintainer! The issue isn't quite ready to finalize yet. Please fix the following labeling issue(s) and then comment \`/finalize\` again:\n\n${errorList}\n\nIf you have questions about which labels to apply, see the maintainer documentation or ask in the team channel.`;
}

// Pre-built validation error strings matching the exact output of collectLabelViolations.
const ERR_MISSING_TRIAGE = `The \`status: awaiting triage\` label must be present to run \`/finalize\`. Current status label(s): \`status: ready for dev\`.`;
const ERR_NO_SKILL = `Exactly one \`skill:\` label is required (e.g. \`skill: beginner\`). None found. Choose from: \`skill: good first issue\`, \`skill: beginner\`, \`skill: intermediate\`, \`skill: advanced\`.`;
const ERR_MULTIPLE_SKILLS = `Exactly one \`skill:\` label is required. Found 2: \`skill: beginner\`, \`skill: intermediate\`. Please remove all but one.`;
const ERR_NO_PRIORITY = `Exactly one \`priority:\` label is required (e.g. \`priority: medium\`). None found.`;

const ERR_UNKNOWN_TYPE = `The issue type (Bug, Feature, or Task) could not be determined. Ensure the issue was submitted using one of the official issue templates.`;

// =============================================================================
// SCENARIOS
// =============================================================================

const scenarios = [
  // ---------------------------------------------------------------------------
  // AUTHORIZATION
  // ---------------------------------------------------------------------------

  {
    name: 'Unauthorized — read-only collaborator',
    description: 'A collaborator with "read" role is rejected',
    context: makeContext(makeIssue()),
    githubOptions: { roleName: 'read' },
    expectedComments: [COMMENT_UNAUTHORIZED],
    assertions: [
      assert.reactionAdded(),
      assert.noIssueUpdate(),
      assert.noLabelsAdded(),
    ],
  },

  {
    name: 'Unauthorized — non-collaborator (404)',
    description: 'A user who is not a repo collaborator is rejected',
    context: makeContext(makeIssue()),
    githubOptions: { permissionNotFound: true },
    expectedComments: [COMMENT_UNAUTHORIZED],
    assertions: [
      assert.reactionAdded(),
      assert.noIssueUpdate(),
    ],
  },

  {
    name: 'Permission check API error',
    description: 'When the permission API fails, posts an error comment and tags maintainers',
    context: makeContext(makeIssue()),
    githubOptions: { permissionShouldFail: true },
    expectedComments: [COMMENT_PERMISSION_ERROR],
    assertions: [
      assert.reactionAdded(),
      assert.noIssueUpdate(),
    ],
  },

  // ---------------------------------------------------------------------------
  // LABEL VALIDATION
  // ---------------------------------------------------------------------------

  {
    name: 'Validation — missing status: awaiting triage',
    description: 'Issue has a different status label — should fail validation',
    context: makeContext(makeIssue({
      labels: [
        { name: LABELS.READY_FOR_DEV },
        { name: LABELS.BEGINNER },
        { name: 'priority: medium' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [validationComment(ERR_MISSING_TRIAGE)],
    assertions: [
      assert.noIssueUpdate(),
      assert.noLabelsAdded(),
    ],
  },

  {
    name: 'Validation — no skill label',
    description: 'Issue is missing a skill: label',
    context: makeContext(makeIssue({
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: 'priority: medium' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [validationComment(ERR_NO_SKILL)],
    assertions: [
      assert.noIssueUpdate(),
    ],
  },

  {
    name: 'Validation — multiple skill labels',
    description: 'Issue has two skill: labels — exactly one is required',
    context: makeContext(makeIssue({
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.BEGINNER },
        { name: LABELS.INTERMEDIATE },
        { name: 'priority: medium' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [validationComment(ERR_MULTIPLE_SKILLS)],
    assertions: [
      assert.noIssueUpdate(),
    ],
  },

  {
    name: 'Validation — no priority label',
    description: 'Issue is missing a priority: label',
    context: makeContext(makeIssue({
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.BEGINNER },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [validationComment(ERR_NO_PRIORITY)],
    assertions: [
      assert.noIssueUpdate(),
    ],
  },

  {
    name: 'Validation — Multiple violations listed in one comment',
    description: 'All violations (no skill, no priority) reported together',
    context: makeContext(makeIssue({
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
      ],
      type: { name: 'Feature' },
    })),
    githubOptions: { roleName: 'admin' },
    expectedComments: [validationComment(ERR_NO_SKILL, ERR_NO_PRIORITY)],
    assertions: [
      assert.noIssueUpdate(),
    ],
  },

  {
    name: 'Validation — Unknown issue type',
    description: 'Issue created without a recognized type triggers a validation error',
    context: makeContext(makeIssue({ type: null })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [validationComment(ERR_UNKNOWN_TYPE)],
    assertions: [
      assert.noIssueUpdate(),
    ],
  },

  // ---------------------------------------------------------------------------
  // HAPPY PATHS
  // ---------------------------------------------------------------------------

  {
    name: 'Happy Path — Good First Issue (Feature)',
    description: 'Valid GFI feature issue is finalized successfully',
    context: makeContext(makeIssue({
      title: 'Add batch query support',
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.GOOD_FIRST_ISSUE },
        { name: 'priority: low' },
      ],
      type: { name: 'Feature' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [COMMENT_SUCCESS_GFI_LOW],
    assertions: [
      assert.reactionAdded(),
      assert.issueUpdated(),
      assert.bodyContains('First-Time Friendly'),
      assert.bodyContains('About Good First Issues'),
      assert.bodyContains('Step-by-Step Contribution Guide'),
      assert.labelAdded(LABELS.READY_FOR_DEV),
      assert.labelRemoved(LABELS.AWAITING_TRIAGE),
    ],
  },

  {
    name: 'Happy Path — Beginner Task',
    description: 'Valid beginner task is finalized; title prefix added, body reconstructed',
    context: makeContext(makeIssue()),
    githubOptions: { roleName: 'triage' },
    expectedComments: [COMMENT_SUCCESS_BEGINNER_MEDIUM],
    assertions: [
      assert.reactionAdded(),
      assert.issueUpdated(),
      assert.bodyContains('Beginner Friendly'),
      assert.bodyContains('About Beginner Issues'),
      assert.bodyContains('Step-by-Step Contribution Guide'),
      assert.labelAdded(LABELS.READY_FOR_DEV),
      assert.labelRemoved(LABELS.AWAITING_TRIAGE),
    ],
  },

  {
    name: 'Happy Path — Intermediate Bug',
    description: 'Valid intermediate bug report is finalized',
    context: makeContext(makeIssue({
      title: 'Client fee cap silently ignored',
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.INTERMEDIATE },
        { name: 'priority: high' },
      ],
      type: { name: 'Bug' },
    })),
    githubOptions: { roleName: 'write' },
    expectedComments: [COMMENT_SUCCESS_INTERMEDIATE_HIGH],
    assertions: [
      assert.issueUpdated(),
      assert.bodyContains('Intermediate Friendly'),
      assert.bodyContains('About Intermediate Issues'),
      assert.bodyContains('Step-by-Step Contribution Guide'),
      assert.labelAdded(LABELS.READY_FOR_DEV),
    ],
  },

  {
    name: 'Happy Path — Advanced Task',
    description: 'Valid advanced task is finalized',
    context: makeContext(makeIssue({
      title: 'Improve issue triage workflow',
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.ADVANCED },
        { name: 'priority: medium' },
        { name: 'scope: ci' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'admin' },
    expectedComments: [COMMENT_SUCCESS_ADVANCED_MEDIUM],
    assertions: [
      assert.issueUpdated(),
      assert.bodyContains('Advanced'),
      assert.bodyContains('About Advanced Issues'),
      assert.bodyContains('Step-by-Step Contribution Guide'),
      assert.labelAdded(LABELS.READY_FOR_DEV),
    ],
  },

  {
    name: 'Happy Path — existing prefix is replaced',
    description: 'An issue that was already finalized once gets the correct prefix when re-finalized',
    context: makeContext(makeIssue({
      title: 'Fix something',
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.ADVANCED },
        { name: 'priority: medium' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'maintain' },
    expectedComments: [COMMENT_SUCCESS_ADVANCED_MEDIUM],
    assertions: [
      assert.issueUpdated(),
      assert.titleContains('Fix something'),
    ],
  },

  // ---------------------------------------------------------------------------
  // API FAILURE PATHS
  // ---------------------------------------------------------------------------

  {
    name: 'API failure — issue update fails',
    description: 'When issues.update throws, a failure comment is posted and labels are NOT swapped',
    context: makeContext(makeIssue()),
    githubOptions: { roleName: 'triage', updateShouldFail: true },
    expectedComments: [COMMENT_UPDATE_FAILURE],
    assertions: [
      assert.noLabelsAdded(),
      assert.noLabelsRemoved(),
    ],
  },

  {
    name: 'API failure — remove label fails after successful update',
    description: 'When removeLabel throws, the swap failure comment and success comment are both posted',
    context: makeContext(makeIssue()),
    githubOptions: { roleName: 'triage', removeLabelShouldFail: true },
    expectedComments: [COMMENT_SWAP_FAILURE_REMOVE, COMMENT_SUCCESS_BEGINNER_MEDIUM],
    assertions: [
      assert.issueUpdated(),
      assert.labelAdded(LABELS.READY_FOR_DEV),   // add still runs and succeeds
      assert.noLabelsRemoved(),                   // remove failed
    ],
  },

  {
    name: 'API failure — add label fails after successful update and remove',
    description: 'When addLabels throws, the swap failure comment and success comment are both posted',
    context: makeContext(makeIssue()),
    githubOptions: { roleName: 'triage', addLabelShouldFail: true },
    expectedComments: [COMMENT_SWAP_FAILURE_ADD, COMMENT_SUCCESS_BEGINNER_MEDIUM],
    assertions: [
      assert.issueUpdated(),
      assert.labelRemoved(LABELS.AWAITING_TRIAGE), // remove succeeded
      assert.noLabelsAdded(),                       // add failed
    ],
  },

  {
    name: 'Body reconstruction — user-provided Additional Information is preserved',
    description: 'When the Additional Information section has real user content it should not be replaced with the default Discord link text',
    context: makeContext(makeIssue({
      body: [
        '### 👾 Description of the Issue\n\nSome bug.\n\n',
        '### ✔️ Acceptance Criteria\n\n- [ ] Fixed\n\n',
        '### 🤔 Additional Information\n\nSee the internal bug tracker for full repro steps.',
      ].join(''),
      labels: [
        { name: LABELS.AWAITING_TRIAGE },
        { name: LABELS.BEGINNER },
        { name: 'priority: medium' },
      ],
      type: { name: 'Task' },
    })),
    githubOptions: { roleName: 'triage' },
    expectedComments: [COMMENT_SUCCESS_BEGINNER_MEDIUM],
    assertions: [
      assert.issueUpdated(),
      assert.bodyContains('See the internal bug tracker for full repro steps.'),
      assert.bodyNotContains('If you have questions while working on this issue'),
    ],
  },
];

// =============================================================================
// UNIT TESTS — pure functions from finalize-comments.js
// =============================================================================

async function runUnitTests() {
  let total = 0;
  let passed = 0;
  let failed = 0;

  function check(name, actual, expected) {
    total++;
    const ok = JSON.stringify(actual) === JSON.stringify(expected);
    if (ok) {
      passed++;
      console.log(`  ✅ ${name}`);
    } else {
      failed++;
      console.log(`  ❌ ${name}`);
      console.log(`     expected: ${JSON.stringify(expected)}`);
      console.log(`     actual:   ${JSON.stringify(actual)}`);
    }
  }

  console.log('\n📐 UNIT TESTS — parseSections / isMeaningfulContent');
  console.log('─'.repeat(60));

  // parseSections
  check('parseSections: null → []', parseSections(null), []);
  check('parseSections: empty string → []', parseSections(''), []);
  check('parseSections: no headers → single null-header entry',
    parseSections('just some text'),
    [{ header: null, content: 'just some text' }]
  );
  check('parseSections: single section',
    parseSections('### My Header\n\nsome content'),
    [{ header: 'My Header', content: 'some content' }]
  );
  check('parseSections: two sections',
    parseSections('### First\n\ncontent one\n\n### Second\n\ncontent two'),
    [
      { header: 'First', content: 'content one' },
      { header: 'Second', content: 'content two' },
    ]
  );
  check('parseSections: section with no content',
    parseSections('### Header\n'),
    [{ header: 'Header', content: '' }]
  );
  check('parseSections: leading content then a header',
    parseSections('preamble\n### Section\n\nbody'),
    [
      { header: null, content: 'preamble' },
      { header: 'Section', content: 'body' },
    ]
  );

  // isMeaningfulContent
  check('isMeaningfulContent: null → false', isMeaningfulContent(null), false);
  check('isMeaningfulContent: empty string → false', isMeaningfulContent(''), false);
  check('isMeaningfulContent: whitespace only → false', isMeaningfulContent('   '), false);
  check('isMeaningfulContent: "Optional." → false', isMeaningfulContent('Optional.'), false);
  check('isMeaningfulContent: "_No response_" → false', isMeaningfulContent('_No response_'), false);
  check('isMeaningfulContent: real content → true', isMeaningfulContent('Some real text here.'), true);
  check('isMeaningfulContent: whitespace-padded real content → true', isMeaningfulContent('  Real content  '), true);

  return { total, passed, failed };
}

// =============================================================================
// RUNNER
// =============================================================================

runTestSuite('FINALIZE COMMAND TEST SUITE', scenarios, runScenario, [
  { label: 'Unit Tests', run: runUnitTests },
]);
