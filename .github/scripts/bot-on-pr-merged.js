// SPDX-License-Identifier: Apache-2.0
//
// bot-on-pr-merged.js
//
// Handles pull_request closed events where the PR was merged.
// Updates the dashboard comments of sibling PRs when a newly merged PR 
// introduces or resolves merge conflicts in them.

const { 
  createLogger, 
  buildBotContext, 
  fetchOpenPRs, 
  getBotComment, 
  runAllChecksAndComment, 
  swapStatusLabel,
  postComment 
} = require('./helpers');
const { checkMergeConflict } = require('./helpers/checks');
const { MARKER, buildMergeConflictNotificationComment } = require('./helpers/comments');

const logger = createLogger('on-pr-merged');

/**
 * Iterates through open PRs, checking for merge conflicts.
 * If a PR's conflict state has changed (compared to its dashboard comment),
 * it runs the checks again and updates the PR dashboard.
 *
 * @param {object} globalBotContext - Base context scoped to the repo.
 */
async function checkSiblingConflictsOnMerge(globalBotContext) {
  const openPRs = await fetchOpenPRs(globalBotContext);
  const mergedPRNumber = globalBotContext.number;

  for (const pr of openPRs) {
    if (pr.draft || pr.number === mergedPRNumber) {
      continue;
    }

    // Context specific to the sibling PR
    const prContext = { ...globalBotContext, number: pr.number, pr };

    // Check actual merge conflict status
    const mergeResult = await checkMergeConflict(prContext);
    const actuallyHasConflict = !mergeResult.passed;

    // Determine current dashboard status shown to the user
    const existingComment = await getBotComment(prContext, MARKER);
    const currentlyShowsConflict = existingComment
      ? existingComment.body.includes(':x: **Merge Conflicts**')
      : false;

    if (currentlyShowsConflict !== actuallyHasConflict) {
      logger.log(`PR #${pr.number} conflict status changed to ${actuallyHasConflict ? 'conflicted' : 'clean'}, updating...`);
      const { allPassed } = await runAllChecksAndComment(prContext, { merge: mergeResult });
      await swapStatusLabel(prContext, allPassed);

      // Post a standalone notification when a conflict is newly introduced
      if (!currentlyShowsConflict && actuallyHasConflict) {
        const prAuthor = pr.user?.login;
        if (prAuthor) {
          const notification = buildMergeConflictNotificationComment(prAuthor, mergedPRNumber);
          await postComment(prContext, notification);
        }
      }
    } else {
      logger.log(`PR #${pr.number} conflict status unchanged (${actuallyHasConflict ? 'conflicted' : 'clean'}), skipping...`);
    }
  }
}

module.exports = async ({ github, context }) => {
  try {
    const botContext = buildBotContext({ github, context });
    
    if (!botContext.pr || !botContext.pr.merged) {
      logger.log('PR is not merged, exiting sibling conflict check.');
      return;
    }

    // Decision: Sibling-conflict check is placed in a dedicated `bot-on-pr-merged.js` script
    // rather than appending it to `bot-on-pr-close.js`. This separates the core PR status
    // updates (conflicts/labels/dashboard) from the contributor workflow automation 
    // (issue recommendation) handled by `bot-on-pr-close.js`.
    await checkSiblingConflictsOnMerge(botContext);

  } catch (error) {
    logger.error('Error in sibling conflict check:', error);
    throw error;
  }
};
