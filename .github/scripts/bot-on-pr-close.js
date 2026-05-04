// SPDX-License-Identifier: Apache-2.0
//
// bot-on-pr-close.js
//
// Handles pull_request close events and triggers post-merge automation.
//
// Purpose:
//   When a PR is closed (and merged), trigger issue recommendation
//   to guide contributors to their next task.
//
// Security:
//   - Only runs on merged PRs
//   - Ignores bot users to prevent loops

const { createLogger, buildBotContext, resolveLinkedIssue } = require('./helpers');
const { handleRecommendIssues } = require('./bot/bot-recommend-issues');

let logger = createLogger('on-pr-close');

// =============================================================================
// ENTRY POINT
// =============================================================================

/**
 * Entry point for PR close event.
 *
 * Validates:
 *   - PR is merged
 *   - Actor is not a bot
 *
 * Then triggers issue recommendation flow.
 */
module.exports = async ({ github, context }) => {
    try {
        const botContext = buildBotContext({ github, context });

        const pr = botContext.pr;

        if (!pr) {
            logger.log('Exit: no pull_request payload');
            return;
        }

        if (!pr.merged) {
            logger.log('Exit: PR closed but not merged');
            return;
        }

        const username = pr.user?.login;

        if (!username) {
            logger.log('Exit: missing PR author');
            return;
        }

        if (pr.user?.type === 'Bot') {
            logger.log('Exit: PR authored by bot');
            return;
        }

        logger.log('Recommendation context:', {
            username,
            prNumber: pr.number,
        });

        const linkedIssue = await resolveLinkedIssue(botContext);

        if (!linkedIssue) {
            logger.log('Skipping recommendation (no resolvable issue)', {
                prNumber: pr.number,
                username,
            });
            return;
        }

        await handleRecommendIssues({
            ...botContext,
            issue: linkedIssue,
            number: pr.number,
            sender: pr.user,
        });

    } catch (error) {
        logger.error('Error:', {
            message: error.message,
            status: error.status,
            pr: context.payload.pull_request?.number,
        });
        throw error;
    }
};
