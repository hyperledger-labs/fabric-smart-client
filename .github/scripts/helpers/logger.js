// SPDX-License-Identifier: Apache-2.0
//
// helpers/logger.js
//
// Logger factory and getter for bot scripts. Helpers use getLogger() so they
// log with the current bot's prefix.

/** @private Logger instance used by getLogger(); set when createLogger() is called. */
let _logger = null;

/**
 * Creates a logger with a consistent prefix for bot scripts.
 * Also sets the module logger so getLogger() returns this instance.
 * @param {string} botName - The name of the bot (e.g., 'on-commit', 'on-pr', 'on-comment').
 * @returns {object} - Logger object with log and error methods.
 */
function createLogger(botName) {
  const prefix = `[${botName}]`;
  const logger = {
    log: (...args) => console.log(prefix, ...args),
    error: (...args) => console.error(prefix, ...args),
  };
  _logger = logger;
  return logger;
}

/**
 * Returns the logger used by helper functions (writeGithubOutput, postComment, etc.).
 * Returns the logger last set by createLogger(), or a default logger if none set yet.
 * @returns {{ log: function, error: function }}
 */
function getLogger() {
  if (!_logger) _logger = createLogger('bot-helpers');
  return _logger;
}

/**
 * Returns a logger proxy that always delegates to whichever logger is currently
 * active (i.e., the one last set by createLogger). Use this in command modules
 * so log calls automatically pick up the prefix set by the dispatcher.
 * @returns {{ log: function, error: function }}
 */
function createDelegatingLogger() {
  return {
    log: (...args) => getLogger().log(...args),
    error: (...args) => getLogger().error(...args),
  };
}

module.exports = {
  createLogger,
  getLogger,
  createDelegatingLogger,
};
