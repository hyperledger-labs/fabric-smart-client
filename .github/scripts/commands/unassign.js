// SPDX-License-Identifier: Apache-2.0
//
// commands/unassign.js
//
// /unassign command: allows a currently assigned contributor to unassign themselves.
// Enforces authorization (only assignees can unassign themselves) and reverts
// status labels back to the community pool.

const {
  LABELS,
  ISSUE_STATE,
  createDelegatingLogger,
  swapLabels,
  removeAssignees,
  postComment,
  acknowledgeComment,
} = require('../helpers');

const {
  buildSuccessfulUnassignComment,
  buildNotAssignedToUserComment,
  buildNoAssigneeComment,
  buildIssueClosedComment,
  buildUnassignFailureComment,
} = require('./unassign-comments');

// Delegate to the active logger set by the dispatcher.
const logger = createDelegatingLogger();

/**
 * Main handler for the /unassign command. Runs the following gates in order:
 *
 * 1. Acknowledge the comment with a thumbs-up reaction.
 * 2. Is the issue already closed? -> issue-closed comment.
 * 3. Does the issue have no assignees? -> no-assignee comment.
 * 4. Is the commenter not the current assignee? -> unauthorized comment.
 *
 * On success: removes the user as an assignee, reverts the "in progress"
 * label to "ready for dev", and posts an acknowledgment comment.
 *
 * @param {{ github: object, owner: string, repo: string, number: number,
 * issue: object, comment: { user: { login: string } } }} botContext
 * @returns {Promise<void>}
 */
async function handleUnassign(botContext) {
  const requesterUsername = botContext.comment.user.login;
  const issue = botContext.issue;

  await acknowledgeComment(botContext, botContext.comment.id);

  // GATE 1: Issue is closed
  if (issue.state === ISSUE_STATE.CLOSED) {
    logger.log('Exit: issue is closed');
    await postComment(botContext, buildIssueClosedComment(requesterUsername));
    return;
  }

  const assignees = issue.assignees || [];

  // GATE 2: No one is assigned at all
  if (assignees.length === 0) {
    logger.log('Exit: issue has no assignees');
    await postComment(botContext, buildNoAssigneeComment(requesterUsername));
    return;
  }

  // GATE 3: Authorization check (case-insensitive)
  const isAssigned = assignees.some(
    (a) => (a?.login || '').toLowerCase() === requesterUsername.toLowerCase()
  );
  if (!isAssigned) {
    logger.log(`Exit: @${requesterUsername} is not assigned to this issue`);
    const currentAssignee = assignees[0]?.login; // Grab the actual assignee for the message
    await postComment(botContext, buildNotAssignedToUserComment(requesterUsername, currentAssignee));
    return;
  }

  // ACTION 1: Remove the assignee
  logger.log(`Unassigning @${requesterUsername}`);
  const removeResult = await removeAssignees(botContext, [requesterUsername]);
  if (!removeResult.success) {
    await postComment(botContext, buildUnassignFailureComment(requesterUsername));
    return;
  }

  // ACTION 2: Label Swapping (Mirroring assign.js style - no stale checks)
  logger.log(`Swapping labels: removing ${LABELS.IN_PROGRESS}, adding ${LABELS.READY_FOR_DEV}`);
  const { success: swapSuccess, errorDetails: swapError } = await swapLabels(botContext, LABELS.IN_PROGRESS, LABELS.READY_FOR_DEV);
  if (!swapSuccess) {
    logger.error(`Label swap failed: ${swapError}`);
  }

  // ACTION 3: Post success acknowledgment
  await postComment(botContext, buildSuccessfulUnassignComment(requesterUsername));
  logger.log(`Successfully unassigned @${requesterUsername} and reverted labels`);
}

module.exports = { handleUnassign };