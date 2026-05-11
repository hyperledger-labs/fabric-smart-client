// SPDX-License-Identifier: Apache-2.0
//
// helpers/constants.js
//
// Shared constants for bot scripts: maintainer team, labels, issue state.

const PROJECT_NAME = 'Fabric Smart Client';

/**
 * Team to tag when manual intervention is needed.
 */
const MAINTAINER_TEAM = '@hyperledger-labs/fabric-smart-client-committers';

/**
 * Common label constants used across bot scripts.
 */
const LABELS = Object.freeze({
  // Status labels
  AWAITING_TRIAGE: 'status: awaiting triage',
  READY_FOR_DEV: 'status: ready for dev',
  IN_PROGRESS: 'status: in progress',
  BLOCKED: 'status: blocked',
  NEEDS_REVIEW: 'status: needs review',
  NEEDS_REVISION: 'status: needs revision',

  // Skill level labels
  GOOD_FIRST_ISSUE: 'skill: good first issue',
  BEGINNER: 'skill: beginner',
  INTERMEDIATE: 'skill: intermediate',
  ADVANCED: 'skill: advanced',

  // Priority labels
  PRIORITY_CRITICAL: 'priority: critical',
  PRIORITY_HIGH: 'priority: high',
  PRIORITY_MEDIUM: 'priority: medium',
  PRIORITY_LOW: 'priority: low',
});

/**
 * Skill hierarchy used to determine progression for recommendations.
 */
const SKILL_HIERARCHY = Object.freeze([
    LABELS.GOOD_FIRST_ISSUE,
    LABELS.BEGINNER,
    LABELS.INTERMEDIATE,
    LABELS.ADVANCED,
]);

/**
 * Priority hierarchy for issue recommendations.
 */
const PRIORITY_HIERARCHY = Object.freeze([
    LABELS.PRIORITY_CRITICAL,
    LABELS.PRIORITY_HIGH,
    LABELS.PRIORITY_MEDIUM,
    LABELS.PRIORITY_LOW,
]);

/**
 * Issue state values for GitHub search queries.
 */
const ISSUE_STATE = Object.freeze({
  OPEN: 'open',
  CLOSED: 'closed',
});

/**
 * Skill-level prerequisite map. Each key is a LABELS skill-level constant.
 * - requiredLabel: the prerequisite skill label the user must have completed, or null if none.
 * - requiredCount: how many closed issues with requiredLabel the user needs.
 * - displayName: human-readable name for the current skill level.
 * - prerequisiteDisplayName: human-readable plural name for the prerequisite level (used in comments).
 *
 * Progression: Good First Issue (no prereqs) -> Beginner (2 GFI) -> Intermediate (3 Beginner) -> Advanced (3 Intermediate).
 * @type {Object<string, { requiredLabel: string|null, requiredCount: number, displayName: string, prerequisiteDisplayName?: string }>}
 */
const SKILL_PREREQUISITES = {
  [LABELS.GOOD_FIRST_ISSUE]: {
    requiredLabel: null,
    requiredCount: 0,
    displayName: "Good First Issue",
  },
  [LABELS.BEGINNER]: {
    requiredLabel: LABELS.GOOD_FIRST_ISSUE,
    requiredCount: 2,
    displayName: "Beginner",
    prerequisiteDisplayName: "Good First Issues",
  },
  [LABELS.INTERMEDIATE]: {
    requiredLabel: LABELS.BEGINNER,
    requiredCount: 3,
    displayName: "Intermediate",
    prerequisiteDisplayName: "Beginner Issues",
  },
  [LABELS.ADVANCED]: {
    requiredLabel: LABELS.INTERMEDIATE,
    requiredCount: 3,
    displayName: "Advanced",
    prerequisiteDisplayName: "Intermediate Issues",
  },
};

module.exports = {
  PROJECT_NAME,
  MAINTAINER_TEAM,
  LABELS,
  ISSUE_STATE,
  SKILL_HIERARCHY,
  SKILL_PREREQUISITES,
  PRIORITY_HIERARCHY,
};
