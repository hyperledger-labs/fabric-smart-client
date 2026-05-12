// SPDX-License-Identifier: Apache-2.0
//
// commands/unassign-comments.js
//
// Comment builders for the /unassign command. Pure formatting functions
// separated from unassignment logic for readability.

const { MAINTAINER_TEAM, LABELS } = require('../helpers');

/**
 * Builds the comment posted after a successful unassignment.
 *
 * @param {string} username - The GitHub username being unassigned.
 * @returns {string} The formatted Markdown comment body.
 */
function buildSuccessfulUnassignComment(username) {
  return [
    `👋 Hi @${username}! You have been successfully unassigned from this issue.`,
    '',
    `The \`${LABELS.IN_PROGRESS}\` label has been removed, and it is now back to \`${LABELS.READY_FOR_DEV}\` for others to claim. Thanks for letting us know!`,
  ].join('\n');
}

/**
 * Builds the comment posted when a user tries to unassign an issue they don't own.
 *
 * @param {string} requesterUsername - The GitHub username who commented /unassign.
 * @param {string} currentAssignee - The GitHub username of the actual assignee.
 * @returns {string} The formatted Markdown comment body.
 */
function buildNotAssignedToUserComment(requesterUsername, currentAssignee) {
  const assigneeText = currentAssignee ? `@${currentAssignee}` : 'someone else';
  return [
    `⚠️ Hi @${requesterUsername}! You cannot unassign this issue because it is currently assigned to ${assigneeText}.`,
    '',
    'Only the current assignee can unassign themselves.',
  ].join('\n');
}

/**
 * Builds the comment posted when the issue has no assignees.
 *
 * @param {string} requesterUsername - The GitHub username who commented /unassign.
 * @returns {string} The formatted Markdown comment body.
 */
function buildNoAssigneeComment(requesterUsername) {
  return [
    `👋 Hi @${requesterUsername}! This issue doesn't currently have any assignees.`,
  ].join('\n');
}

/**
 * Builds the comment posted when the issue is already closed.
 *
 * @param {string} requesterUsername - The GitHub username who commented /unassign.
 * @returns {string} The formatted Markdown comment body.
 */
function buildIssueClosedComment(requesterUsername) {
  return [
    `👋 Hi @${requesterUsername}! This issue is already closed, so the \`/unassign\` command cannot be used here.`,
  ].join('\n');
}

/**
 * Builds the comment posted when the unassign API call fails.
 *
 * @param {string} requesterUsername - The GitHub username who commented /unassign.
 * @returns {string} The formatted Markdown comment body.
 */
function buildUnassignFailureComment(requesterUsername) {
  return [
    `⚠️ Hi @${requesterUsername}! I tried to unassign you, but encountered an unexpected error.`,
    '',
    `${MAINTAINER_TEAM} — could you please manually unassign @${requesterUsername}?`,
  ].join('\n');
}

module.exports = {
  buildSuccessfulUnassignComment,
  buildNotAssignedToUserComment,
  buildNoAssigneeComment,
  buildIssueClosedComment,
  buildUnassignFailureComment,
};