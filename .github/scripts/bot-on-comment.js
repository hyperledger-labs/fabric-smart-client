// SPDX-License-Identifier: Apache-2.0
//
// bot-on-comment.js
//
// Handles issue comment events: reads the comment body, parses commands, and dispatches
// to the appropriate handler. Implemented commands: /assign, /unassign, /finalize.
//
// /assign:   see commands/assign.js (skill levels, assignment limits, required labels).
// /unassign: see commands/unassign.js (authorization, label reversion).
// /finalize: see commands/finalize.js (triage permission required; validates labels,
//            updates issue title/body with skill-level format, swaps status labels).

const { createLogger, buildBotContext } = require('./helpers');
const { handleAssign } = require('./commands/assign');
const { handleUnassign } = require('./commands/unassign');
const { handleFinalize } = require('./commands/finalize');

const KNOWN_COMMANDS = ['assign', 'unassign', 'finalize'];

let logger = createLogger('on-comment');

// =============================================================================
// COMMAND PARSING
// =============================================================================

/**
 * Parses the comment body and returns the list of commands to run.
 * Commands are recognized by exact match (with optional surrounding whitespace).
 *
 * @param {string} body - The comment body.
 * @returns {{ commands?: string[], nearMiss?: string }} - List of command names (e.g. ['assign'] or []).
 */

function parseComment(body) {
  if (typeof body !== 'string') {
    return { commands: [] };
  }

  const trimmed = body.trim();

  for (const command of KNOWN_COMMANDS) {
    // exact match
    if (new RegExp(`^/${command}$`, 'i').test(trimmed)) {
      logger.log(`parseComment: detected /${command}`);
      return { commands: [command] };
    }

    // near miss
    if (new RegExp(`^/${command}\\s+`, 'i').test(trimmed)) {
      logger.log(`parseComment: near miss /${command}`);
      return { nearMiss: command };
    }
  }

  logger.log('parseComment: no known command', { body: body.substring(0, 80) });
  return { commands: [] };
}

// =============================================================================
// ENTRY POINT
// =============================================================================

/**
 * Entry point: read comment, parse commands, dispatch to handlers.
 * Validates that the event is a comment from a human; then runs each detected command.
 */
module.exports = async ({ github, context }) => {
  try {
    const botContext = buildBotContext({ github, context });

    if (!botContext.comment?.user?.login) {
      logger.log('Exit: missing comment user login');
      return;
    }

    if (botContext.comment.user.type === 'Bot') {
      logger.log('Exit: comment authored by bot');
      return;
    }

    const parsed = parseComment(botContext.comment.body);
    if (parsed.nearMiss) {
      logger = createLogger(`on-${parsed.nearMiss}`);

      await botContext.postComment(
        `⚠️ The command \`/${parsed.nearMiss}\` must be used alone.\n\nPlease comment exactly:\n\`/${parsed.nearMiss}\``
      );

      return;
    }

    if (!parsed.commands || parsed.commands.length === 0) {
      logger.log('Exit: no known command');
      return;
    }


    for (const command of parsed.commands) {
      // Update logger prefix to the command name so helper functions
      // (postComment, addLabels, etc.) log with the correct tag.
      logger = createLogger(`on-${command}`);

      if (command === 'assign') {
        await handleAssign(botContext);
      } else if (command === 'unassign') {
        await handleUnassign(botContext);
      } else if (command === 'finalize') {
        await handleFinalize(botContext);
      } else {
        logger.log('Unknown command:', command);
      }
    }
  } catch (error) {
    logger.error('Error:', {
      message: error.message,
      status: error.status,
      number: context.payload.issue?.number,
      commenter: context.payload.comment?.user?.login,
    });
    throw error;
  }
};

module.exports.parseComment = parseComment;