// SPDX-License-Identifier: Apache-2.0
//
// helpers/api.js
//
// Bot context builder and GitHub API wrappers (labels, assignees, comments,
// commit/issue fetching, and label swap helpers).

const { getLogger } = require('./logger');
const {
  isSafeSearchToken,
  requireObject,
  requireNonEmptyString,
  requirePositiveInt,
  requireSafeUsername,
} = require('./validation');
const { LABELS, SKILL_HIERARCHY, ISSUE_STATE } = require('./constants');
const { checkDCO, checkGPG, checkMergeConflict, checkIssueLink } = require('./checks');
const { buildBotComment } = require('./comments');

/**
 * Builds the bot context for any bot. Validates github, context, and payload; throws if invalid.
 * Returned object always includes eventType; then event-specific fields (number, pr/issue, and comment for issue_comment).
 *
 * @param {{ github: object, context: object }} args - The arguments from the workflow.
 * @returns {{ github: object, owner: string, repo: string, eventType: string, ... }}
 *   - pull_request / pull_request_target / pull_request_review: also number, pr
 *   - issues: also number, issue
 *   - issue_comment: also number, issue, comment
 * @throws {Error} If input is invalid or event type is unsupported.
 */
function buildBotContext({ github, context }) {
  requireObject(github, 'github');
  requireObject(context, 'context');
  requireObject(context.repo, 'context.repo');
  requireObject(context.payload, 'context.payload');

  const owner = context.repo.owner;
  const repo = context.repo.repo;
  requireNonEmptyString(owner, 'context.repo.owner');
  requireNonEmptyString(repo, 'context.repo.repo');
  if (!isSafeSearchToken(owner) || !isSafeSearchToken(repo)) {
    throw new Error('Bot context invalid: owner or repo contains invalid characters');
  }

  const base = { github, owner, repo };

  requireNonEmptyString(context.eventName, 'context.eventName');
  const eventType = context.eventName;

  const { payload } = context;
  let payloadPart;
  switch (eventType) {
    case 'pull_request':
    case 'pull_request_target':
    case 'pull_request_review': {
      const pr = payload.pull_request;
      requireObject(pr, 'context.payload.pull_request');
      requirePositiveInt(pr.number, 'pull_request.number');

      if (pr.user) {
        requireNonEmptyString(pr.user.login, 'pull_request.user.login');
        if (!isSafeSearchToken(pr.user.login)) {
          throw new Error('Bot context invalid: pull_request.user.login contains invalid characters');
        }
      }

      payloadPart = { number: pr.number, pr };
      break;
    }

    case 'issues':
    case 'issue_comment': {
      const issue = payload.issue;
      requireObject(issue, 'context.payload.issue');
      requirePositiveInt(issue.number, 'issue.number');
      payloadPart = { number: issue.number, issue };

      if (eventType === 'issue_comment') {
        const comment = payload.comment;
        requireObject(comment, 'context.payload.comment');
        requireObject(comment.user, 'context.payload.comment.user');
        requireNonEmptyString(comment.user.login, 'context.payload.comment.user.login');

        // Flag bot users early so callers can skip processing without
        // hitting the stricter username validation below.
        const isBot = comment.user.type === 'Bot';
        if (!isBot && !isSafeSearchToken(comment.user.login)) {
          throw new Error('Bot context invalid: comment.user.login contains invalid characters');
        }
        if (typeof comment.body !== 'string') {
          throw new Error('Bot context invalid: comment.body must be a string');
        }
        
        payloadPart = { ...payloadPart, comment, isBot };
      }
      break;
    }
    default:
      throw new Error(`Bot context invalid: unsupported event type "${eventType}"`);
  }
  return { ...base, eventType, ...payloadPart };
}

/**
 * Safely adds labels to an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string[]} labels - Array of label names to add.
 * @returns {Promise<{success: boolean, error?: string}>} - Result object.
 */
async function addLabels(botContext, labels) {
  if (!Array.isArray(labels)) {
    return { success: false, error: 'labels must be an array' };
  }
  
  try {
    for (let i = 0; i < labels.length; i++) {
      requireNonEmptyString(labels[i], `labels[${i}]`);
    }

    await botContext.github.rest.issues.addLabels({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      labels,
    });

    getLogger().log(`Added labels: ${labels.join(', ')}`);
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not add labels "${labels.join(', ')}": ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Safely removes a label from an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string} labelName - The label name to remove.
 * @returns {Promise<{success: boolean, error?: string}>} - Result object.
 */
async function removeLabel(botContext, labelName) {
  try {
    requireNonEmptyString(labelName, 'labelName');

    await botContext.github.rest.issues.removeLabel({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      name: labelName,
    });

    getLogger().log(`Removed label: ${labelName}`);
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not remove label "${labelName}": ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Safely adds assignees to an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string[]} assignees - Array of usernames to assign.
 * @returns {Promise<{success: boolean, error?: string}>} - Result object.
 */
async function addAssignees(botContext, assignees) {
  if (!Array.isArray(assignees)) {
    return { success: false, error: 'assignees must be an array' };
  }

  try {
    for (let i = 0; i < assignees.length; i++) {
      requireSafeUsername(assignees[i], `assignees[${i}]`);
    }

    await botContext.github.rest.issues.addAssignees({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      assignees,
    });

    getLogger().log(`Added assignees: ${assignees.join(', ')}`);
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not add assignees "${assignees.join(', ')}": ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Safely removes assignees from an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string[]} assignees - Array of usernames to remove.
 * @returns {Promise<{success: boolean, error?: string}>} - Result object.
 */
async function removeAssignees(botContext, assignees) {
  if (!Array.isArray(assignees)) {
    return { success: false, error: 'assignees must be an array' };
  }

  try {
    for (let i = 0; i < assignees.length; i++) {
      requireSafeUsername(assignees[i], `assignees[${i}]`);
    }

    await botContext.github.rest.issues.removeAssignees({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      assignees,
    });

    getLogger().log(`Removed assignees: ${assignees.join(', ')}`);
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not remove assignees "${assignees.join(', ')}": ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Safely posts a comment on an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string} body - The comment body.
 * @returns {Promise<{success: boolean, error?: string}>} - Result object.
 */
async function postComment(botContext, body) {
  try {
    requireNonEmptyString(body, 'comment body');

    await botContext.github.rest.issues.createComment({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      body,
    });
    getLogger().log('Posted comment');
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not post comment: ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Checks if an issue or PR has a specific label.
 * @param {object} issueOrPr - The issue or PR object.
 * @param {string} labelName - The label name to check for.
 * @returns {boolean} - True if the label is present.
 */
function hasLabel(issueOrPr, labelName) {
  if (!issueOrPr?.labels?.length) {
    return false;
  }

  return issueOrPr.labels.some((label) => {
    const name = typeof label === 'string' ? label : label?.name;
    return typeof name === 'string' && name.toLowerCase() === labelName.toLowerCase();
  });
}

/**
 * Returns all label names on an issue or PR that start with the given prefix.
 * The comparison is case-insensitive.
 *
 * @param {object} issueOrPr - The issue or PR object.
 * @param {string} prefix - Label group prefix (e.g. 'skill:').
 * @returns {string[]}
 */
function getLabelsByPrefix(issueOrPr, prefix) {
  return (issueOrPr.labels || [])
    .map((l) => (typeof l === 'string' ? l : l?.name || ''))
    .filter((name) => name.toLowerCase().startsWith(prefix.toLowerCase()));
}

/**
 * Removes `fromLabel` and adds `toLabel` on the issue/PR. Both operations are
 * always attempted; errors are collected and returned rather than thrown.
 *
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @param {string} fromLabel - Label to remove.
 * @param {string} toLabel - Label to add.
 * @returns {Promise<{ success: boolean, errorDetails: string }>}
 */
async function swapLabels(botContext, fromLabel, toLabel) {
  const errors = [];

  const removeResult = await removeLabel(botContext, fromLabel);
  if (!removeResult.success) {
    errors.push(`Failed to remove '${fromLabel}': ${removeResult.error}`);
  }

  const addResult = await addLabels(botContext, [toLabel]);
  if (!addResult.success) {
    errors.push(`Failed to add '${toLabel}': ${addResult.error}`);
  }

  return { success: errors.length === 0, errorDetails: errors.join('; ') };
}

/**
 * Fetches an existing comment identified by an HTML marker.
 * Paginates through all comments to find a match.
 * @param {object} botContext
 * @param {string} marker - HTML comment marker (e.g. '<!-- bot:pr-helper -->').
 * @returns {Promise<object|null>}
 */
async function getBotComment(botContext, marker) {
  let page = 1;
  const perPage = 100;

  while (true) {
    const { data: comments } = await botContext.github.rest.issues.listComments({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      per_page: perPage,
      page,
    });

    for (const c of comments) {
      if (c.body && c.body.startsWith(marker)) {
        return c;
      }
    }

    if (comments.length < perPage) break;
    page++;
  }

  return null;
}

/**
 * Posts a new comment or updates an existing one identified by an HTML marker.
 * Prevents unnecessary updates if the comment body has not changed.
 * @param {object} botContext
 * @param {string} marker - HTML comment marker (e.g. '<!-- bot:pr-helper -->').
 * @param {string} body - Full comment body (must include the marker).
 * @returns {Promise<{success: boolean, error?: string}>}
 */
async function postOrUpdateComment(botContext, marker, body) {
  try {
    requireNonEmptyString(marker, 'marker');
    requireNonEmptyString(body, 'comment body');

    const existingComment = await getBotComment(botContext, marker);

    if (existingComment) {
      if (existingComment.body.trim() === body.trim()) {
        getLogger().log('Existing bot comment is up-to-date');
      } else {
        await botContext.github.rest.issues.updateComment({
          owner: botContext.owner,
          repo: botContext.repo,
          comment_id: existingComment.id,
          body,
        });
        getLogger().log('Updated existing bot comment');
      }
    } else {
      await botContext.github.rest.issues.createComment({
        owner: botContext.owner,
        repo: botContext.repo,
        issue_number: botContext.number,
        body,
      });
      getLogger().log('Created new bot comment');
    }
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not post/update comment: ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Fetches all commits for a pull request via the GitHub API (paginated).
 * @param {object} botContext
 * @returns {Promise<Array>}
 */
async function fetchPRCommits(botContext) {
  const commits = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const response = await botContext.github.rest.pulls.listCommits({
      owner: botContext.owner,
      repo: botContext.repo,
      pull_number: botContext.number,
      per_page: perPage,
      page,
    });

    commits.push(...response.data);

    if (response.data.length < perPage) break;
    page++;
  }

  getLogger().log(`Fetched ${commits.length} commits for PR #${botContext.number}`);
  return commits;
}

/**
 * Fetches all open pull requests for the repository via the GitHub API (paginated).
 * @param {object} botContext
 * @returns {Promise<Array>}
 */
async function fetchOpenPRs(botContext) {
  const prs = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const response = await botContext.github.rest.pulls.list({
      owner: botContext.owner,
      repo: botContext.repo,
      state: 'open',
      per_page: perPage,
      page,
    });

    prs.push(...response.data);

    if (response.data.length < perPage) break;
    page++;
  }

  getLogger().log(`Fetched ${prs.length} open PRs`);
  return prs;
}

/**
 * Fetches a single issue by number.
 * @param {object} botContext
 * @param {number} issueNumber
 * @returns {Promise<object>}
 */
async function fetchIssue(botContext, issueNumber) {
  const { data: issue } = await botContext.github.rest.issues.get({
    owner: botContext.owner,
    repo: botContext.repo,
    issue_number: issueNumber,
  });
  return issue;
}

/**
 * Fetches issue numbers linked to a PR via GitHub's closingIssuesReferences GraphQL field.
 * @param {object} botContext
 * @returns {Promise<number[]>}
 */
async function fetchClosingIssueNumbers(botContext) {
  try {
    const query = `query($owner:String!,$repo:String!,$number:Int!){
      repository(owner:$owner,name:$repo){
        pullRequest(number:$number){
          closingIssuesReferences(first:10){
            nodes { number }
          }
        }
      }
    }`;
    const result = await botContext.github.graphql(query, {
      owner: botContext.owner,
      repo: botContext.repo,
      number: botContext.number,
    });
    const nodes = result.repository.pullRequest.closingIssuesReferences.nodes || [];
    return nodes.map(n => n.number);
  } catch (error) {
    getLogger().error(`GraphQL closingIssuesReferences failed: ${error.message}`);
    return [];
  }
}

/**
 * Swaps between needs-review and needs-revision labels based on check results.
 * By default only changes the label if the opposite label is currently applied.
 * When force is true, unconditionally applies the target label (used on PR open
 * to guarantee a status label is always present).
 * @param {object} botContext
 * @param {boolean} allPassed
 * @param {{ force?: boolean }} [options]
 * @returns {Promise<void>}
 */
async function swapStatusLabel(botContext, allPassed, { force = false } = {}) {
  const pr = botContext.pr;
  const labelToAdd = allPassed ? LABELS.NEEDS_REVIEW : LABELS.NEEDS_REVISION;
  const labelToRemove = allPassed ? LABELS.NEEDS_REVISION : LABELS.NEEDS_REVIEW;

  if (force) {
    if (hasLabel(pr, labelToRemove)) {
      await removeLabel(botContext, labelToRemove);
    }
    await addLabels(botContext, [labelToAdd]);
  } else {
    if (hasLabel(pr, labelToRemove)) {
      await removeLabel(botContext, labelToRemove);
      await addLabels(botContext, [labelToAdd]);
    }
  }
}

/**
 * Adds a thumbs-up (+1) reaction to a comment as visual acknowledgement that
 * a bot command was received. Returns { success: false } when the reaction
 * cannot be added (e.g. the comment was deleted before the bot ran).
 *
 * @param {object} botContext - Bot context from buildBotContext (github, owner, repo).
 * @param {number} commentId - The ID of the comment to react to.
 * @returns {Promise<{ success: boolean }>}
 */
async function acknowledgeComment(botContext, commentId) {
  try {
    await botContext.github.rest.reactions.createForIssueComment({
      owner: botContext.owner,
      repo: botContext.repo,
      comment_id: commentId,
      content: '+1',
    });
    getLogger().log('Added thumbs-up reaction to comment');
    return { success: true };
  } catch (error) {
    getLogger().log('Could not add reaction:', error.message);
    return { success: false };
  }
}

/**
 * Runs all 4 PR checks (DCO, GPG, merge conflict, issue link) with error
 * resilience, builds the unified dashboard comment, and posts/updates it.
 * Returns { allPassed } so callers can decide on label handling.
 * @param {object} botContext
 * @returns {Promise<{ allPassed: boolean }>}
 */
async function runAllChecksAndComment(botContext, precomputed = {}) {
  let { dco, gpg, merge, issueLink } = precomputed;
  let commits = [];

  try {
    commits = await fetchPRCommits(botContext);
  } catch (e) {
    getLogger().error('Failed to fetch PR commits:', e.message);
    dco = { error: true, errorMessage: e.message };
    gpg = { error: true, errorMessage: e.message };
  }

  if (!dco) {
    try { dco = checkDCO(commits); }
    catch (e) { dco = { error: true, errorMessage: e.message }; }
  }

  if (!gpg) {
    try { gpg = checkGPG(commits); }
    catch (e) { gpg = { error: true, errorMessage: e.message }; }
  }

  if (!merge) {
    try { merge = await checkMergeConflict(botContext); }
    catch (e) { merge = { error: true, errorMessage: e.message }; }
  }

  if (!issueLink) {
    try { issueLink = await checkIssueLink(botContext, { fetchIssue, fetchClosingIssueNumbers }); }
    catch (e) { issueLink = { error: true, errorMessage: e.message }; }
  }

  const prAuthor = botContext.pr?.user?.login;
  const { marker, body, allPassed } = buildBotComment({ prAuthor, dco, gpg, merge, issueLink });
  await postOrUpdateComment(botContext, marker, body);

  return { allPassed };
}

/**
 * Fetches all issue/PR events (paginated). Useful for detecting label changes
 * (e.g. when "status: blocked" was removed).
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @returns {Promise<Array>}
 */
async function fetchIssueEvents(botContext) {
  const events = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const { data } = await botContext.github.rest.issues.listEvents({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      per_page: perPage,
      page,
    });

    events.push(...data);

    if (data.length < perPage) break;
    page++;
  }

  getLogger().log(`Fetched ${events.length} events for #${botContext.number}`);
  return events;
}

/**
 * Fetches all comments for an issue or PR (paginated).
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @returns {Promise<Array>}
 */
async function fetchComments(botContext) {
  const comments = [];
  let page = 1;
  const perPage = 100;

  while (true) {
    const { data } = await botContext.github.rest.issues.listComments({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      per_page: perPage,
      page,
    });

    comments.push(...data);

    if (data.length < perPage) break;
    page++;
  }

  getLogger().log(`Fetched ${comments.length} comments for #${botContext.number}`);
  return comments;
}

/**
 * Closes an issue or PR.
 * @param {object} botContext - Bot context (github, owner, repo, number).
 * @returns {Promise<{success: boolean, error?: string}>}
 */
async function closeItem(botContext) {
  try {
    await botContext.github.rest.issues.update({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      state: 'closed',
    });

    getLogger().log(`Closed #${botContext.number}`);
    return { success: true };
  } catch (error) {
    getLogger().error(`Could not close #${botContext.number}: ${error.message}`);
    return { success: false, error: error.message };
  }
}

/**
 * Resolves the primary issue linked to a PR.
 *
 * Strategy:
 *   - Fetch closing issue references via GraphQL
 *   - If multiple issues, return the one with the highest skill level
 *   - Return null if no linked issues found
 *
 * Notes:
 *   - Logs informational messages for traceability
 *   - Does NOT throw — failures are handled gracefully
 *
 * @param {object} botContext
 * @returns {Promise<object|null>}
 */
async function resolveLinkedIssue(botContext) {
    try {
        const issueNumbers = await fetchClosingIssueNumbers(botContext);

        if (!issueNumbers.length) {
            getLogger().log('No linked issue found', {
                prNumber: botContext.number,
            });
            return null;
        }

        if (issueNumbers.length === 1) {
            const issue = await fetchIssue(botContext, issueNumbers[0]);
            if (!issue || SKILL_HIERARCHY.findIndex(level => hasLabel(issue, level)) === -1) {
                getLogger().log('Single linked issue has no skill label', { issueNumber: issueNumbers[0] });
                return null;
            }
            return issue;
        }

        const issues = await Promise.all(
            issueNumbers.map(n => fetchIssue(botContext, n))
        );
        const valid = issues.filter(Boolean);

        if (!valid.length) {
            getLogger().log('All linked issue fetches returned empty', { issueNumbers });
            return null;
        }

        const selected = valid.reduce((best, issue) => {
            const bestIndex = SKILL_HIERARCHY.findIndex(level => hasLabel(best, level));
            const currIndex = SKILL_HIERARCHY.findIndex(level => hasLabel(issue, level));
            return currIndex > bestIndex ? issue : best;
        });

        const selectedIndex = SKILL_HIERARCHY.findIndex(level => hasLabel(selected, level));
        if (selectedIndex === -1) {
            getLogger().log('No linked issues have a skill label', { issueNumbers });
            return null;
        }

        getLogger().log('Multiple linked issues found (using highest level)', {
            issueNumbers,
            selected: selected.number,
        });

        return selected;

    } catch (error) {
        getLogger().error('Failed to resolve linked issue:', {
            message: error.message,
        });
        return null;
    }
}

/**
 * Returns the highest difficulty level of an issue based on its labels.
 *
 * Checks labels against SKILL_HIERARCHY in descending order and returns the first match.
 *
 * @param {{ labels: Array<string|{ name: string }> }} issue
 * @returns {string|null} Matching level or null if none found.
 */
function getHighestIssueSkillLevel(issue) {
  for (const level of [...SKILL_HIERARCHY].reverse()) {
    if (hasLabel(issue, level)) return level;
  }
  return null;
}

/**
 * Fetches the number of issues assigned to a specific user that match a given
 * state and optional label using the GitHub REST API.
 *
 * Note: When state is OPEN and no label filter is provided, issues with the
 * "status: blocked" label are explicitly EXCLUDED from the count.
 * The search is constrained to the repo specified in the context.
 *
 * @param {object} github - Octokit GitHub API client (must support github.rest).
 * @param {string} owner - Repository owner (e.g. 'hiero-ledger').
 * @param {string} repo - Repository name (e.g. 'hiero-sdk-cpp').
 * @param {string} username - GitHub username to search for.
 * @param {string} state - Issue state filter: ISSUE_STATE.OPEN or ISSUE_STATE.CLOSED.
 * @param {string|null} [label=null] - Optional label filter (e.g. 'skill: good first issue').
 * @param {number|null} [threshold=null] - Optional threshold to short-circuit pagination.
 *   When provided, the function returns a capped count (the threshold value)
 *   once that threshold is reached.
 * @returns {Promise<number|null>} Matching issue count, or null if inputs are invalid or the API call fails.
 *   When threshold is provided and reached, returns the threshold value (capped),
 *   not necessarily the exact total.
 */
async function countIssuesByAssignee(
  github,
  owner,
  repo,
  username,
  state,
  label = null,
  threshold = null,
) {
  if (
    !isSafeSearchToken(owner) ||
    !isSafeSearchToken(repo) ||
    !isSafeSearchToken(username)
  ) {
    getLogger().log("[assign] Invalid search inputs:", {
      owner,
      repo,
      username,
      label,
    });
    return null;
  }
  if (state !== ISSUE_STATE.OPEN && state !== ISSUE_STATE.CLOSED) {
    getLogger().log("[assign] Invalid state:", { state });
    return null;
  }
  if (
    label &&
    (typeof label !== "string" || !label.trim() || label.includes('"'))
  ) {
    getLogger().log("[assign] Invalid label parameter:", { label });
    return null;
  }

  try {
    let page = 1;
    let matchingIssuesCount = 0;
    const perPage = 100;

    getLogger().log(`[assign] Fetching ${state} assigned issues via REST...`);
    while (true) {
      const params = {
        owner,
        repo,
        state: state.toLowerCase(),
        assignee: username,
        per_page: perPage,
        page,
      };
      if (label) params.labels = label;

      const result = await github.rest.issues.listForRepo(params);
      // Filter out Pull Requests (which are returned by the issues endpoint)
      const actualIssues = result.data.filter((item) => !item.pull_request);

      let pageMatchCount = 0;
      if (state === ISSUE_STATE.OPEN && !label) {
        pageMatchCount = actualIssues.filter(
          (issue) =>
            !issue.labels?.some((l) => (l.name || l) === LABELS.BLOCKED),
        ).length;
      } else {
        pageMatchCount = actualIssues.length;
      }

      matchingIssuesCount += pageMatchCount;

      if (threshold !== null && matchingIssuesCount >= threshold) {
        getLogger().log(
          `[assign] Reached threshold (${threshold}), short-circuiting fetch.`,
        );
        matchingIssuesCount = threshold; // Cap at threshold logically for callers
        break;
      }

      // Pagination must evaluate the raw result size, not the filtered size.
      if (result.data.length < perPage) break;
      page++;
    }

    if (label) {
      getLogger().log(
        `[assign] ${state} assigned issues for ${username} with label ${label}: ${matchingIssuesCount}`,
      );
    } else {
      getLogger().log(
        `[assign] ${state} assigned issues for ${username}${state === ISSUE_STATE.OPEN ? " (excluding blocked)" : ""}: ${matchingIssuesCount}`,
      );
    }
    return matchingIssuesCount;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    getLogger().log(
      `[assign] Failed to count ${state} issues for ${username}: ${message}`,
    );
    return null;
  }
}

/**
 * Returns the actual open non-blocked issue objects assigned to the given user.
 * Reuses the same listForRepo pagination pattern as countIssuesByAssignee,
 * filtering out pull requests and issues with the "status: blocked" label.
 *
 * @param {object} github - Octokit GitHub API client (must support github.rest).
 * @param {string} owner - Repository owner (e.g. 'hiero-ledger').
 * @param {string} repo - Repository name (e.g. 'hiero-sdk-cpp').
 * @param {string} username - GitHub username to search for.
 * @returns {Promise<Array|null>} Array of issue objects, or null if inputs are invalid or the API call fails.
 */
async function listAssignedIssues(github, owner, repo, username) {
  if (
    !isSafeSearchToken(owner) ||
    !isSafeSearchToken(repo) ||
    !isSafeSearchToken(username)
  ) {
    getLogger().log("[assign] Invalid search inputs for listAssignedIssues:", {
      owner,
      repo,
      username,
    });
    return null;
  }

  try {
    let page = 1;
    const perPage = 100;
    const issues = [];

    getLogger().log("[assign] Fetching open assigned issues via REST (objects)...");
    while (true) {
      const result = await github.rest.issues.listForRepo({
        owner,
        repo,
        state: ISSUE_STATE.OPEN,
        assignee: username,
        per_page: perPage,
        page,
      });

      // Filter out Pull Requests (which are returned by the issues endpoint)
      const actualIssues = result.data.filter((item) => !item.pull_request);

      // Exclude issues with the "status: blocked" label
      const nonBlocked = actualIssues.filter(
        (issue) =>
          !issue.labels?.some((l) => (l.name || l) === LABELS.BLOCKED),
      );

      issues.push(...nonBlocked);

      // Pagination must evaluate the raw result size, not the filtered size.
      if (result.data.length < perPage) break;
      page++;
    }

    getLogger().log(
      `[assign] Open non-blocked assigned issues for ${username}: ${issues.length}`,
    );
    return issues;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    getLogger().log(
      `[assign] Failed to list assigned issues for ${username}: ${message}`,
    );
    return null;
  }
}

/**
 * Checks whether the given issue has an open PR authored by the specified user
 * that carries the "status: needs review" label and is linked to the issue.
 * Uses the GitHub GraphQL API to check closedByPullRequestsReferences.
 *
 * @param {object} github - Octokit GitHub API client.
 * @param {string} owner - Repository owner.
 * @param {string} repo - Repository name.
 * @param {string} username - GitHub username (PR author).
 * @param {number} issueNumber - Issue number to check for linked PRs.
 * @returns {Promise<boolean|null>} true if a matching PR exists, false if none, null on API error.
 */
async function hasNeedsReviewPR(github, owner, repo, username, issueNumber) {
  if (
    !isSafeSearchToken(owner) ||
    !isSafeSearchToken(repo) ||
    !isSafeSearchToken(username)
  ) {
    getLogger().log("[assign] Invalid search inputs for hasNeedsReviewPR:", {
      owner,
      repo,
      username,
      issueNumber,
    });
    return null;
  }
  if (!Number.isInteger(issueNumber) || issueNumber < 1) {
    getLogger().log("[assign] Invalid issue number for hasNeedsReviewPR:", {
      issueNumber,
    });
    return null;
  }

  try {
    getLogger().log(`[assign] Querying linked PRs for issue #${issueNumber}`);
    const query = `query($owner:String!,$repo:String!,$number:Int!){
      repository(owner:$owner,name:$repo){
        issue(number:$number){
          closedByPullRequestsReferences(first:50){
            nodes {
              state
              author { login }
              labels(first:50) {
                nodes { name }
              }
            }
          }
        }
      }
    }`;
    const result = await github.graphql(query, {
      owner,
      repo,
      number: issueNumber,
    });

    const nodes = result.repository?.issue?.closedByPullRequestsReferences?.nodes || [];
    const hasMatch = nodes.some(pr => {
      const isAuthor = pr.author && pr.author.login === username;
      const isOpen = pr.state === 'OPEN';
      const hasLabel = pr.labels && pr.labels.nodes && pr.labels.nodes.some(l => l.name === LABELS.NEEDS_REVIEW);
      return isAuthor && isOpen && hasLabel;
    });

    getLogger().log(
      `[assign] Needs-review PR search for issue #${issueNumber}: ${hasMatch ? 1 : 0} match(es)`,
    );
    return hasMatch;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    getLogger().log(
      `[assign] Failed to search for needs-review PRs for issue #${issueNumber}: ${message}`,
    );
    return null;
  }
}

module.exports = {
  buildBotContext,
  addLabels,
  removeLabel,
  addAssignees,
  removeAssignees,
  postComment,
  hasLabel,
  getLabelsByPrefix,
  swapLabels,
  getBotComment,
  postOrUpdateComment,
  fetchPRCommits,
  fetchOpenPRs,
  fetchIssue,
  fetchClosingIssueNumbers,
  swapStatusLabel,
  runAllChecksAndComment,
  resolveLinkedIssue,
  acknowledgeComment,
  fetchComments,
  fetchIssueEvents,
  closeItem,
  getHighestIssueSkillLevel,
  countIssuesByAssignee,
  listAssignedIssues,
  hasNeedsReviewPR,
};
