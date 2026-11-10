// SPDX-License-Identifier: Apache-2.0
//
// bot-pr-title.js
//
// Runs when a PR is opened, edited, reopened or gets new commits. Checks that the
// title follows Conventional Commits, and, once the PR is squashed to one commit,
// that the commit's header equals the title, because that header is what lands on
// main. Reports the result as the commit status "pr-title" and sets the one
// changelog label the title maps to (see helpers/pr-title.js). Runs for every
// author, including dependabot, whose dependency PRs get the "dependencies" label.

const {
  createLogger,
  buildBotContext,
  addLabels,
  removeLabel,
  fetchPRCommits,
  parseTitle,
  labelFor,
  labelChanges,
} = require('./helpers');

const logger = createLogger('pr-title');

const STATUS_CONTEXT = 'pr-title';

// GitHub truncates commit status descriptions longer than this.
const MAX_DESCRIPTION = 140;

/**
 * Sets the pr-title commit status on the PR's head commit.
 * @param {object} botContext
 * @param {'success'|'failure'} state
 * @param {string} description
 */
async function setStatus(botContext, state, description) {
  const text = description.length > MAX_DESCRIPTION ? description.slice(0, MAX_DESCRIPTION - 3) + '...' : description;
  await botContext.github.rest.repos.createCommitStatus({
    owner: botContext.owner,
    repo: botContext.repo,
    sha: botContext.pr.head.sha,
    state,
    context: STATUS_CONTEXT,
    description: text,
  });
}

// The title Mergify gives a backport, "<title> (backport #N)" (.mergify.yml).
const BACKPORT_TITLE = /^(.+) \(backport (#\d+)\)$/;

/**
 * Returns an error when the PR has exactly one commit and its header differs from
 * the title, or null otherwise. While a PR has several commits (fixups during
 * review) the header is not compared. A backport to a release branch, titled
 * "<title> (backport #N)", cherry-picks the squash commit of PR N, whose header is
 * "<title> (#N)".
 * @param {object} botContext
 * @returns {Promise<string|null>}
 */
async function headerMismatch(botContext) {
  if (botContext.pr.commits !== 1) return null;
  const commits = await fetchPRCommits(botContext);
  if (commits.length !== 1) return null;
  const header = (commits[0].commit?.message || '').split('\n')[0];
  if (header === botContext.pr.title) return null;
  const backport = botContext.pr.base?.ref?.startsWith('release-') && BACKPORT_TITLE.exec(botContext.pr.title);
  if (backport && header === `${backport[1]} (${backport[2]})`) return null;
  return `The commit header "${header}" must equal the PR title.`;
}

/**
 * Leaves exactly the changelog label of the title on the PR.
 * @param {object} botContext
 * @param {string|null} wanted
 */
async function applyLabel(botContext, wanted) {
  const current = (botContext.pr.labels || []).map((l) => l.name);
  const { add, remove } = labelChanges(current, wanted);
  if (add.length > 0) await addLabels(botContext, add);
  for (const name of remove) await removeLabel(botContext, name);
}

module.exports = async ({ github, context }) => {
  const botContext = buildBotContext({ github, context });

  const parsed = parseTitle(botContext.pr.title);
  if (!parsed.ok) {
    logger.log(`Invalid title: ${botContext.pr.title}`);
    await setStatus(botContext, 'failure', parsed.error);
    return;
  }

  await applyLabel(botContext, labelFor(parsed));

  const mismatch = await headerMismatch(botContext);
  if (mismatch) {
    logger.log(mismatch);
    await setStatus(botContext, 'failure', mismatch);
    return;
  }

  await setStatus(botContext, 'success', 'The PR title follows Conventional Commits.');
};
