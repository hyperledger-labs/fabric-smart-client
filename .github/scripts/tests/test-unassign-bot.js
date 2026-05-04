// SPDX-License-Identifier: Apache-2.0
//
// tests/test-unassign-bot.js
//
// Local test script for the /unassign command in bot-on-comment.js.
// Run with: node .github/scripts/tests/test-unassign-bot.js
//
// This script mocks the GitHub API and runs various test scenarios
// to verify the /unassign command behaves correctly.

const { LABELS, ISSUE_STATE } = require('../helpers');
const script = require('../bot-on-comment.js');
const { verifyComments, runTestSuite } = require('./test-utils'); 

// =============================================================================
// MOCK GITHUB API
// =============================================================================

/**
 * Creates a mock GitHub API client for testing.
 * Tracks all calls made and optionally simulates API failures.
 *
 * @param {object} options
 * @param {boolean} [options.removeAssigneesShouldFail=false]
 * @param {boolean} [options.removeLabelShouldFail=false]
 * @param {boolean} [options.addLabelShouldFail=false]
 * @returns {{ calls: object, rest: object }}
 */
function createMockGithub(options = {}) {
  const {
    removeAssigneesShouldFail = false,
    removeLabelShouldFail     = false,
    addLabelShouldFail        = false,
  } = options;

  const calls = {
    comments:         [],
    assigneesRemoved: [],
    labelsAdded:      [],
    labelsRemoved:    [],
    reactions:        [],
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
      issues: {
        createComment: async (params) => {
          calls.comments.push(params.body);
          console.log('\n📝 COMMENT POSTED:');
          console.log('─'.repeat(60));
          console.log(params.body);
          console.log('─'.repeat(60));
        },
        removeAssignees: async (params) => {
          if (removeAssigneesShouldFail) throw new Error('Simulated remove assignees failure');
          calls.assigneesRemoved.push(...params.assignees);
          console.log(`\n✅ UNASSIGNED: ${params.assignees.join(', ')}`);
        },
        addLabels: async (params) => {
          if (addLabelShouldFail) throw new Error('Simulated add label failure');
          calls.labelsAdded.push(...params.labels);
          console.log(`\n🏷️  LABEL ADDED: ${params.labels.join(', ')}`);
        },
        removeLabel: async (params) => {
          if (removeLabelShouldFail) throw new Error('Simulated remove label failure');
          calls.labelsRemoved.push(params.name);
          console.log(`\n🏷️  LABEL REMOVED: ${params.name}`);
        },
      },
    },
  };
}

// =============================================================================
// TEST SCENARIOS
// =============================================================================

const scenarios = [
  // ---------------------------------------------------------------------------
  // HAPPY PATHS
  // ---------------------------------------------------------------------------
  {
    name:        'Happy Path - Unassign Success',
    description: 'Current assignee successfully unassigns themselves; labels swap correctly',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    100,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1001,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: 'active-contributor',
    expectedLabelRemoved:    LABELS.IN_PROGRESS,
    expectedLabelAdded:      LABELS.READY_FOR_DEV,
    expectedComments: [
      `👋 Hi @active-contributor! You have been successfully unassigned from this issue.\n\nThe \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
    ],
  },
  {
    name:        'Happy Path - Case-Insensitive Username',
    description: 'Unassign succeeds even when stored login casing differs from commenter casing',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    106,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'Active-Contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1007,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: 'active-contributor',
    expectedLabelRemoved:    LABELS.IN_PROGRESS,
    expectedLabelAdded:      LABELS.READY_FOR_DEV,
    expectedComments: [
      `👋 Hi @active-contributor! You have been successfully unassigned from this issue.\n\nThe \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
    ],
  },
  {
    name:        'Happy Path - Command Casing and Whitespace',
    description: 'Command successfully routes even with trailing spaces and uppercase letters',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    110,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1011,
          body: '   /UNASSIGN   ',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: 'active-contributor',
    expectedLabelRemoved:    LABELS.IN_PROGRESS,
    expectedLabelAdded:      LABELS.READY_FOR_DEV,
    expectedComments: [
      `👋 Hi @active-contributor! You have been successfully unassigned from this issue.\n\nThe \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
    ],
  },
  {
    name:        'Happy Path - IN_PROGRESS Label Already Missing',
    description: 'Unassign succeeds if a maintainer manually removed IN_PROGRESS; READY_FOR_DEV is still added',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    107,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [],
        },
        comment: {
          id:   1008,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: 'active-contributor',
    expectedLabelRemoved:    LABELS.IN_PROGRESS, 
    expectedLabelAdded:      LABELS.READY_FOR_DEV,
    expectedComments: [
      `👋 Hi @active-contributor! You have been successfully unassigned from this issue.\n\nThe \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
    ],
  },

  // ---------------------------------------------------------------------------
  // VALIDATION FAILURES
  // ---------------------------------------------------------------------------
  {
    name:        'Validation - Not Assigned to Requester',
    description: 'User tries to unassign an issue assigned to someone else; gets auth error',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    101,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'other-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1002,
          body: '/unassign',
          user: { login: 'sneaky-user', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments: [
      `⚠️ Hi @sneaky-user! You cannot unassign this issue because it is currently assigned to @other-contributor.\n\nOnly the current assignee can unassign themselves.`,
    ],
  },
  {
    name:        'Validation - No Assignees on Issue',
    description: 'User tries to unassign an issue that has no assignees at all',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    102,
          state:     ISSUE_STATE.OPEN,
          assignees: [],
          labels:    [{ name: LABELS.READY_FOR_DEV }],
        },
        comment: {
          id:   1003,
          body: '/unassign',
          user: { login: 'confused-user', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments: [
      `👋 Hi @confused-user! This issue doesn't currently have any assignees.`,
    ],
  },
  {
    name:        'Validation - Issue Already Closed',
    description: 'User tries to unassign from a closed issue; command is rejected',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    103,
          state:     ISSUE_STATE.CLOSED,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1004,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          true,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments: [
      `👋 Hi @active-contributor! This issue is already closed, so the \`/unassign\` command cannot be used here.`,
    ],
  },

  // ---------------------------------------------------------------------------
  // NO ACTION
  // ---------------------------------------------------------------------------
  {
    name:        'No Action - Regular Comment Ignored',
    description: 'Non-command comments on assigned issues produce zero API calls',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    109,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1010,
          body: 'Looking good, will review tomorrow!',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          false,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments:        [],
  },
  {
    name:        'No Action - Bot Comment Ignored',
    description: 'Bot-authored comments are silently ignored to prevent feedback loops',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    108,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'github-actions[bot]' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1009,
          body: '/unassign',
          user: { login: 'github-actions[bot]', type: 'Bot' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           {},
    expectReaction:          false,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments:        [],
  },

  // ---------------------------------------------------------------------------
  // ERROR HANDLING
  // ---------------------------------------------------------------------------
  {
    name:        'Error - removeAssignees API Failure',
    description: 'GitHub API throws on removeAssignees; failure comment posted, no label changes made',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    104,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1005,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           { removeAssigneesShouldFail: true },
    expectReaction:          true,
    expectedAssigneeRemoved: null,
    expectedLabelRemoved:    null,
    expectedLabelAdded:      null,
    expectedComments: [
      `⚠️ Hi @active-contributor! I tried to unassign you, but encountered an unexpected error.\n\n@hiero-ledger/hiero-sdk-cpp-maintainers — could you please manually unassign @active-contributor?`,
    ],
  },
  {
    name:        'Error - Label Swap API Failure',
    description: 'Label swap fails after successful unassignment; error logged, success comment still posted',
    context: {
      eventName: 'issue_comment',
      payload: {
        issue: {
          number:    105,
          state:     ISSUE_STATE.OPEN,
          assignees: [{ login: 'active-contributor' }],
          labels:    [{ name: LABELS.IN_PROGRESS }],
        },
        comment: {
          id:   1006,
          body: '/unassign',
          user: { login: 'active-contributor', type: 'User' },
        },
      },
      repo: { owner: 'hiero-ledger', repo: 'hiero-sdk-cpp' },
    },
    githubOptions:           { removeLabelShouldFail: true, addLabelShouldFail: true },
    expectReaction:          true,
    expectedAssigneeRemoved: 'active-contributor',
    expectedLabelRemoved:    null,  
    expectedLabelAdded:      null, 
    expectedComments: [
      `👋 Hi @active-contributor! You have been successfully unassigned from this issue.\n\nThe \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
    ],
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

/**
 * Runs a single test scenario and returns whether it passed.
 *
 * @param {object} scenario - The scenario definition.
 * @param {number} index - Zero-based scenario index.
 * @returns {Promise<boolean>}
 */
async function runTest(scenario, index) {
  console.log('\n' + '='.repeat(70));
  console.log(`TEST ${index + 1}: ${scenario.name}`);
  console.log(`Description: ${scenario.description}`);
  console.log('='.repeat(70));

  const mockGithub = createMockGithub(scenario.githubOptions);

  try {
    await script({ github: mockGithub, context: scenario.context });
  } catch (error) {
    console.log(`\n❌ SCRIPT THREW ERROR: ${error.message}`);
  }

  const results = { passed: true, details: [] };

  // Verify: assignee removed
  if (scenario.expectedAssigneeRemoved) {
    if (mockGithub.calls.assigneesRemoved.includes(scenario.expectedAssigneeRemoved)) {
      results.details.push(`✅ Correctly removed assignee: ${scenario.expectedAssigneeRemoved}`);
    } else {
      results.passed = false;
      results.details.push(`❌ Expected to remove assignee ${scenario.expectedAssigneeRemoved}, got: ${mockGithub.calls.assigneesRemoved.join(', ') || 'none'}`);
    }
  } else {
    if (mockGithub.calls.assigneesRemoved.length === 0) {
      results.details.push('✅ Correctly did not remove any assignees');
    } else {
      results.passed = false;
      results.details.push(`❌ Should not have removed assignees, but removed: ${mockGithub.calls.assigneesRemoved.join(', ')}`);
    }
  }

  // Verify: label removed
  if ('expectedLabelRemoved' in scenario) {
    if (scenario.expectedLabelRemoved === null) {
      if (mockGithub.calls.labelsRemoved.length === 0) {
        results.details.push('✅ Correctly did not remove any labels');
      } else {
        results.passed = false;
        results.details.push(`❌ Expected NO labels removed, got: ${mockGithub.calls.labelsRemoved.join(', ')}`);
      }
    } else {
      if (mockGithub.calls.labelsRemoved.includes(scenario.expectedLabelRemoved)) {
        results.details.push(`✅ Correctly removed label: ${scenario.expectedLabelRemoved}`);
      } else {
        results.passed = false;
        results.details.push(`❌ Expected to remove label "${scenario.expectedLabelRemoved}", got: ${mockGithub.calls.labelsRemoved.join(', ') || 'none'}`);
      }
    }
  }

  // Verify: label added
  if ('expectedLabelAdded' in scenario) {
    if (scenario.expectedLabelAdded === null) {
      if (mockGithub.calls.labelsAdded.length === 0) {
        results.details.push('✅ Correctly did not add any labels');
      } else {
        results.passed = false;
        results.details.push(`❌ Expected NO labels added, got: ${mockGithub.calls.labelsAdded.join(', ')}`);
      }
    } else {
      if (mockGithub.calls.labelsAdded.includes(scenario.expectedLabelAdded)) {
        results.details.push(`✅ Correctly added label: ${scenario.expectedLabelAdded}`);
      } else {
        results.passed = false;
        results.details.push(`❌ Expected to add label "${scenario.expectedLabelAdded}", got: ${mockGithub.calls.labelsAdded.join(', ') || 'none'}`);
      }
    }
  }

  // Verify: comments
  const commentResult = verifyComments(scenario.expectedComments || [], mockGithub.calls.comments);
  if (!commentResult.passed) results.passed = false;
  results.details.push(...commentResult.details);

  // Verify: reaction
  if (scenario.expectReaction) {
    const reacted = mockGithub.calls.reactions.some(r => r.content === '+1');
    if (reacted) {
      results.details.push('✅ Correctly added thumbs-up reaction');
    } else {
      results.passed = false;
      results.details.push('❌ Expected thumbs-up reaction but none was added');
    }
  } else {
    if (mockGithub.calls.reactions.length === 0) {
      results.details.push('✅ Correctly did not add any reactions');
    } else {
      results.passed = false;
      results.details.push(`❌ Expected NO reactions, but added: ${mockGithub.calls.reactions.map(r => r.content).join(', ')}`);
    }
  }

  console.log('\n📊 RESULT:');
  results.details.forEach((d) => console.log(`   ${d}`));

  return results.passed;
}

runTestSuite('BOT-UNASSIGN-ON-COMMENT TEST SUITE', scenarios, runTest);