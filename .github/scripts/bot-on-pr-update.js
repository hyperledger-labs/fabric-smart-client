// SPDX-License-Identifier: Apache-2.0
//
// bot-on-pr-update.js
//
// Runs on new commits (synchronize) and PR body edits (edited). Performs all
// 4 checks (DCO, GPG, merge conflict, issue link), posts/updates the unified
// dashboard comment, and conditionally swaps the status label.
// For edited events, exits early if only the title or base branch changed.

const {
  createLogger,
  buildBotContext,
  swapStatusLabel,
  runAllChecksAndComment,
} = require('./helpers');

const logger = createLogger('on-pr-update');

module.exports = async ({ github, context }) => {
  try {
    const botContext = buildBotContext({ github, context });

    if (botContext.pr?.user?.type === 'Bot') {
      logger.log('Skipping bot-authored PR');
      return;
    }

    // Edits can be triggered by title changes, but we only care about body changes.
    if (context.payload.action === 'edited' && !context.payload.changes?.body) {
      logger.log('Body not changed, skipping');
      return;
    }

    const { allPassed } = await runAllChecksAndComment(botContext);
    await swapStatusLabel(botContext, allPassed);

    logger.log('On-PR-update bot completed');
  } catch (error) {
    logger.error('Error:', {
      message: error.message,
      number: context?.payload?.pull_request?.number,
    });
    throw error;
  }
};
