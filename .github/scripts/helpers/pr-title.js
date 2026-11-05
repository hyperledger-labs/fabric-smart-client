// SPDX-License-Identifier: Apache-2.0
//
// helpers/pr-title.js
//
// Parses a PR title as a Conventional Commits header and maps it to the one
// changelog label that .github/release.yml groups the generated release notes by.
// A PR merges as a single commit whose header is the title, so the title decides
// where the PR appears in the release notes.

/** Types a PR title may start with. */
const TYPES = ['feat', 'fix', 'perf', 'refactor', 'docs', 'test', 'ci', 'build', 'chore', 'style', 'revert'];

/** The form a title must have, as shown in error messages. */
const TITLE_FORM = '<type>(<scope>): <summary>';

/** Every label this check sets or removes. Other labels on a PR are never touched. */
const CHANGELOG_LABELS = ['breaking', 'feature', 'fix', 'perf', 'dependencies', 'skip-changelog'];

// <type>, an optional lowercase (<scope>), an optional ! for a breaking change,
// ": " and a summary that starts with a non-space character.
const HEADER = new RegExp(`^(${TYPES.join('|')})(\\(([a-z0-9/._-]+)\\))?(!)?: \\S`);

// The header git writes for `git revert`.
const GIT_REVERT = /^Revert ".+"$/;

const SKIP_CHANGELOG_TYPES = new Set(['test', 'ci', 'build', 'style']);
const DEPENDENCY_TYPES = new Set(['chore', 'build']);

/**
 * Parses a PR title.
 *
 * @param {string} title
 * @returns {{ ok: true, type: string, scope: string|null, breaking: boolean } | { ok: false, error: string }}
 */
function parseTitle(title) {
  if (typeof title !== 'string') {
    return { ok: false, error: `The PR title must have the form ${TITLE_FORM}.` };
  }
  if (GIT_REVERT.test(title)) {
    return { ok: true, type: 'revert', scope: null, breaking: false };
  }
  const match = HEADER.exec(title);
  if (!match) {
    return {
      ok: false,
      error:
        `The PR title "${title}" must have the form ${TITLE_FORM}, with one of the types ` +
        `${TYPES.join(' ')}; add ! before the colon for a breaking change.`,
    };
  }
  return { ok: true, type: match[1], scope: match[3] || null, breaking: match[4] === '!' };
}

/**
 * Returns the changelog label of a parsed title, or null when the PR goes under
 * "Other changes".
 *
 * @param {{ type: string, scope: string|null, breaking: boolean }} parsed
 * @returns {string|null}
 */
function labelFor(parsed) {
  if (parsed.breaking) return 'breaking';
  if (parsed.type === 'feat') return 'feature';
  if (parsed.type === 'fix') return 'fix';
  if (parsed.type === 'perf') return 'perf';
  if (DEPENDENCY_TYPES.has(parsed.type) && parsed.scope === 'deps') return 'dependencies';
  if (SKIP_CHANGELOG_TYPES.has(parsed.type)) return 'skip-changelog';
  return null;
}

/**
 * Returns the label changes that leave exactly the wanted changelog label on a PR,
 * or none when wanted is null. Labels outside CHANGELOG_LABELS are left alone.
 *
 * @param {string[]} currentLabels - Names of the labels the PR has.
 * @param {string|null} wanted - The changelog label the PR should have.
 * @returns {{ add: string[], remove: string[] }}
 */
function labelChanges(currentLabels, wanted) {
  const current = new Set(currentLabels);
  const add = wanted && !current.has(wanted) ? [wanted] : [];
  const remove = CHANGELOG_LABELS.filter((l) => l !== wanted && current.has(l));
  return { add, remove };
}

module.exports = {
  CHANGELOG_LABELS,
  TITLE_FORM,
  parseTitle,
  labelFor,
  labelChanges,
};
