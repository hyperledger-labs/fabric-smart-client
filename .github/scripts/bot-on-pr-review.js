// SPDX-License-Identifier: Apache-2.0
//
// bot-on-pr-review.js
//
// Triggers on pull_request_review: submitted.
// When a maintainer requests changes, automatically swaps the needs-review label
// to needs-revision.

const {
  createLogger,
  buildBotContext,
  swapStatusLabel,
} = require('./helpers');

const logger = createLogger('on-pr-review');

module.exports = async ({ github, context }) => {
  try {
    const botContext = buildBotContext({ github, context });

    const state = context.payload.review?.state?.toLowerCase();

    if (state !== 'changes_requested') {
      logger.log(`Review state is '${state}', ignoring`);
      return;
    }

    // Force swap to needs-revision (allPassed = false)
    await swapStatusLabel(botContext, false, { force: true });
    logger.log(`Successfully swapped status to needs revision`);
  } catch (error) {
    logger.error('Error:', {
      message: error.message,
      number: context?.payload?.pull_request?.number,
    });
    throw error;
  }
};
