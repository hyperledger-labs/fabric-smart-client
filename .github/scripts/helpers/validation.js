// SPDX-License-Identifier: Apache-2.0
//
// helpers/validation.js
//
// Validation helpers for bot scripts. Only functions that need two or more checks
// (e.g. not null and type, or type and format) are provided here.

/**
 * Returns true if value is a non-null object (including arrays).
 * Two checks: not null and type object.
 * @param {*} value
 * @returns {boolean}
 */
function isObject(value) {
  return value !== null && typeof value === 'object';
}

/**
 * Returns true if value is an integer >= 0.
 * Two checks: is integer and non-negative.
 * @param {*} value
 * @returns {boolean}
 */
function isNonNegativeInteger(value) {
  return Number.isInteger(value) && value >= 0;
}

/**
 * Returns true if value is a string safe for GitHub search queries (type and format).
 * Allows standard alphanumeric characters, `-`, `_`, `/`, `.`, and the optional
 * GitHub bot suffix `[bot]` (e.g. `dependabot[bot]`).
 * Two checks: is string and matches safe character set.
 * @param {*} value
 * @returns {boolean}
 */
function isSafeSearchToken(value) {
  return typeof value === 'string' && /^[a-zA-Z0-9._/-]+(\[bot\])?$/.test(value);
}

/**
 * Throws if value is not a non-null object.
 * @param {*} value - Value to check.
 * @param {string} label - Label for error message.
 * @throws {Error}
 */
function requireObject(value, label) {
  if (!isObject(value)) {
    throw new Error(`Bot context invalid: missing or invalid ${label}`);
  }
}

/**
 * Throws if value is not a non-empty string (type and non-empty after trim).
 * @param {*} value - Value to check.
 * @param {string} label - Label for error message.
 * @throws {Error}
 */
function requireNonEmptyString(value, label) {
  if (typeof value !== 'string' || !value.trim()) {
    throw new Error(`Bot context invalid: missing or invalid ${label}`);
  }
}

/**
 * Throws if value is not a positive integer.
 * @param {*} value - Value to check.
 * @param {string} label - Label for error message.
 * @throws {Error}
 */
function requirePositiveInt(value, label) {
  if (!Number.isInteger(value) || value < 1) {
    throw new Error(`Bot context invalid: missing or invalid ${label}`);
  }
}

/**
 * Throws if value is not a non-empty string safe for use as a GitHub username (e.g. in API calls or search).
 * @param {*} value - Value to check.
 * @param {string} label - Label for error message.
 * @throws {Error}
 */
function requireSafeUsername(value, label) {
  requireNonEmptyString(value, label);
  if (!isSafeSearchToken(value)) {
    throw new Error(`Bot context invalid: ${label} contains invalid characters`);
  }
}

module.exports = {
  isNonNegativeInteger,
  isSafeSearchToken,
  requireObject,
  requireNonEmptyString,
  requirePositiveInt,
  requireSafeUsername,
};
