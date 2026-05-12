// SPDX-License-Identifier: Apache-2.0
//
// bot-on-pr-open.js
//
// Runs when a PR is opened, reopened, or converted from draft (ready_for_review).
// Performs all 4 checks (DCO, GPG, merge conflict, issue link), posts/updates
// the unified dashboard comment, auto-assigns the author, and applies the
// appropriate status label.

const {
  createLogger,
  buildBotContext,
  addAssignees,
  requireSafeUsername,
  runAllChecksAndComment,
  swapStatusLabel,
} = require('./helpers');

const logger = createLogger('on-pr-open');

/**
 * Auto-assigns the PR author if not already assigned.
 * @param {object} botContext
 */
async function autoAssignAuthor(botContext) {
  const prAuthor = botContext.pr?.user?.login;
  if (!prAuthor) {
    logger.log('Exit: missing pull request author');
    return;
  }
  try {
    requireSafeUsername(prAuthor, 'pr.author');
  } catch (err) {
    logger.log('Exit: invalid pr.author', err.message);
    return;
  }

  const currentAssignees = botContext.pr?.assignees || [];
  const isAlreadyAssigned = currentAssignees.some(
    (a) => (a?.login || '').toLowerCase() === prAuthor.toLowerCase()
  );
  if (isAlreadyAssigned) {
    logger.log(`Author ${prAuthor} is already assigned`);
    return;
  }
  await addAssignees(botContext, [prAuthor]);
}

module.exports = async ({ github, context }) => {
  try {
    const botContext = buildBotContext({ github, context });

    await autoAssignAuthor(botContext);

    if (botContext.pr?.user?.type === 'Bot') {
      logger.log('Skipping bot-authored PR');
      return;
    }

    const { allPassed } = await runAllChecksAndComment(botContext);
    await swapStatusLabel(botContext, allPassed, { force: true });

    logger.log('On-PR-open bot completed');
  } catch (error) {
    logger.error('Error:', {
      message: error.message,
      number: context?.payload?.pull_request?.number,
    });
    throw error;
  }
};
