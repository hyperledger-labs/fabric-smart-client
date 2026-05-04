// SPDX-License-Identifier: Apache-2.0
//
// helpers/comments.js
//
// Builds the unified PR Helper Bot dashboard comment. Designed with a layered
// structure so future sections (commands, instructions) can be added alongside
// checks without changing the overall shape.

const { MAINTAINER_TEAM } = require('./constants');

const MARKER = '<!-- bot:pr-helper -->';

const SIGNING_GUIDE = 'https://github.com/hyperledger-labs/fabric-smart-client/blob/main/docs/dev/signing.md';
const MERGE_CONFLICTS_GUIDE = 'https://github.com/hyperledger-labs/fabric-smart-client/blob/main/docs/dev/merge-conflicts.md';

/**
 * Determines the display state of a check result.
 * @param {{ passed?: boolean, error?: boolean }} result
 * @returns {'pass'|'fail'|'error'}
 */
function checkState(result) {
  if (result.error) return 'error';
  return result.passed ? 'pass' : 'fail';
}

/**
 * Shared renderer for the error and pass states of a check section.
 * Returns null for the fail state so callers can supply their own content.
 * @param {{ title: string, result: object, passMessage: string }} opts
 * @returns {string|null}
 */
function buildSection({ title, result, passMessage }) {
  const state = checkState(result);
  if (state === 'error') {
    return [
      `:warning: **${title}** -- This check encountered an internal error. ${MAINTAINER_TEAM} please review manually.`,
      '',
      `Error: ${result.errorMessage || 'Unknown error'}`,
    ].join('\n');
  }
  if (state === 'pass') {
    return `:white_check_mark: **${title}** -- ${passMessage}`;
  }
  return null;
}

/**
 * @param {{ passed: boolean, failures?: Array<{ sha: string, message: string }>, error?: boolean, errorMessage?: string }} dco
 * @returns {string}
 */
function buildDCOSection(dco) {
  const common = buildSection({ title: 'DCO Sign-off', result: dco, passMessage: 'All commits have valid sign-offs. Nice work!' });
  if (common) return common;

  const failList = (dco.failures || []).map(f => `- \`${f.sha}\` ${f.message}`).join('\n');
  return [
    ':x: **DCO Sign-off** -- Uh oh! The following commits are missing the required DCO sign-off:',
    failList,
    '',
    `No worries, this is an easy fix! Add \`Signed-off-by: Your Name <email>\` to each commit (e.g. \`git commit -s\`). See the [Signing Guide](${SIGNING_GUIDE}).`,
  ].join('\n');
}

/**
 * @param {{ passed: boolean, failures?: Array<{ sha: string, message: string }>, error?: boolean, errorMessage?: string }} gpg
 * @returns {string}
 */
function buildGPGSection(gpg) {
  const common = buildSection({ title: 'GPG Signature', result: gpg, passMessage: 'All commits have verified GPG signatures. Locked and loaded!' });
  if (common) return common;

  const failList = (gpg.failures || []).map(f => `- \`${f.sha}\` ${f.message}`).join('\n');
  return [
    ':x: **GPG Signature** -- Heads up! The following commits don\'t have a verified GPG signature:',
    failList,
    '',
    `You'll need to sign your commits with GPG (e.g. \`git commit -S\`). See the [Signing Guide](${SIGNING_GUIDE}) for a step-by-step walkthrough.`,
  ].join('\n');
}

/**
 * @param {{ passed: boolean, error?: boolean, errorMessage?: string }} merge
 * @returns {string}
 */
function buildMergeSection(merge) {
  const common = buildSection({ title: 'Merge Conflicts', result: merge, passMessage: 'No merge conflicts detected. Smooth sailing!' });
  if (common) return common;

  return [
    ':x: **Merge Conflicts** -- Oh no, this PR has merge conflicts with the base branch.',
    '',
    `Let's get this sorted! Update your branch (e.g. rebase or merge from base) and push. See the [Merge Conflicts Guide](${MERGE_CONFLICTS_GUIDE}) if you need a hand.`,
  ].join('\n');
}

/**
 * Builds a standalone notification comment to alert a PR author that a
 * recently merged PR has introduced a merge conflict in their PR.
 * This is posted once when the conflict state changes from clean to
 * conflicted — it does NOT replace the dashboard comment.
 *
 * @param {string} prAuthor - GitHub username of the PR author.
 * @param {number} mergedPRNumber - The PR number whose merge caused the conflict.
 * @returns {string}
 */
function buildMergeConflictNotificationComment(prAuthor, mergedPRNumber) {
  return [
    `Hi @${prAuthor} :wave: — the recent merge of PR #${mergedPRNumber} has introduced a merge conflict in this PR.`,
    `Please resolve the merge conflict so that this PR can be reviewed again. Thank you!`,
  ].join(' ');
}

/**
 * @param {{ passed: boolean, reason?: string, issues?: Array<{ number: number, title: string, isAssigned: boolean }>, error?: boolean, errorMessage?: string }} issueLink
 * @returns {string}
 */
function buildIssueLinkSection(issueLink) {
  const linked = (issueLink.issues || [])
    .filter(i => i.isAssigned)
    .map(i => `#${i.number}`)
    .join(', ');
  const common = buildSection({ title: 'Issue Link', result: issueLink, passMessage: `Linked to ${linked} (assigned to you).` });
  if (common) return common;

  if (issueLink.reason === 'not_assigned') {
    const unassigned = (issueLink.issues || []).filter(i => !i.isAssigned).map(i => `#${i.number}`).join(', ');
    return [
      `:x: **Issue Link** -- Almost there! You are not assigned to the following linked issues: ${unassigned}.`,
      '',
      'Please ensure you are assigned to all linked issues before opening a PR. You can comment `/assign` on the issue to grab it!',
    ].join('\n');
  }
  return [
    ':x: **Issue Link** -- This PR is not linked to any issue.',
    '',
    'Please reference an issue using a closing keyword (e.g. `Fixes #123`) and ensure the issue is assigned to you. Every PR needs a home!',
  ].join('\n');
}

/**
 * Builds the ### PR Checks section of the dashboard comment.
 * @param {{ dco: object, gpg: object, merge: object, issueLink: object }} results
 * @returns {string}
 */
function buildChecksSection({ dco, gpg, merge, issueLink }) {
  return [
    '### PR Checks',
    '',
    buildDCOSection(dco),
    '',
    '---',
    '',
    buildGPGSection(gpg),
    '',
    '---',
    '',
    buildMergeSection(merge),
    '',
    '---',
    '',
    buildIssueLinkSection(issueLink),
  ].join('\n');
}

/**
 * Determines whether all checks passed (errors count as not passed).
 * @param {{ dco: object, gpg: object, merge: object, issueLink: object }} results
 * @returns {boolean}
 */
function allChecksPassed({ dco, gpg, merge, issueLink }) {
  return (
    !dco.error && dco.passed &&
    !gpg.error && gpg.passed &&
    !merge.error && merge.passed &&
    !issueLink.error && issueLink.passed
  );
}

/**
 * Builds the full unified bot comment.
 * @param {{ prAuthor: string, dco: object, gpg: object, merge: object, issueLink: object }} params
 * @returns {{ marker: string, body: string, allPassed: boolean }}
 */
function buildBotComment({ prAuthor, dco, gpg, merge, issueLink }) {
  const greeting = [
    `Hey @${prAuthor} :wave: thanks for the PR!`,
    "I'm your friendly **PR Helper Bot** :robot: and I'll be riding shotgun on this one, keeping track of your PR's status to help you get it approved and merged.",
    '',
    "This comment updates automatically as you push changes -- think of it as your PR's live scoreboard!",
    "Here's the latest:",
  ].join('\n');

  const checksSection = buildChecksSection({ dco, gpg, merge, issueLink });
  const passed = allChecksPassed({ dco, gpg, merge, issueLink });

  const footer = passed
    ? ':tada: *All checks passed! Your PR is ready for review. Great job!*'
    : ':hourglass_flowing_sand: *All checks must pass before this PR can be reviewed. You\'ve got this!*';

  const body = [MARKER, greeting, '', '---', '', checksSection, '', '---', '', footer].join('\n');
  return { marker: MARKER, body, allPassed: passed };
}

module.exports = {
  MARKER,
  buildBotComment,
  buildChecksSection,
  allChecksPassed,
  buildMergeConflictNotificationComment,
};
