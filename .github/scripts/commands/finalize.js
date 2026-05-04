// SPDX-License-Identifier: Apache-2.0
//
// commands/finalize.js
//
// /finalize command: transitions a triaged issue from "awaiting triage" to
// "ready for dev". Validates all required labels, updates the issue title with
// the appropriate skill-level prefix, reconstructs the body in the skill-level
// template format, and swaps status labels.
//
// Only contributors with triage-or-above repository permissions may run this command.

const {
  LABELS,
  createDelegatingLogger,
  hasLabel,
  getLabelsByPrefix,
  swapLabels,
  postComment,
  acknowledgeComment,
} = require('../helpers');

const {
  reconstructBody,
  buildUnauthorizedComment,
  buildValidationErrorComment,
  buildUpdateFailureComment,
  buildLabelSwapFailureComment,
  buildPermissionCheckErrorComment,
  buildSuccessComment,
} = require('./finalize-comments');

// Delegate to the active logger set by the dispatcher (bot-on-comment.js).
const logger = createDelegatingLogger();

// Permission levels that are allowed to run /finalize (triage and above).
const ALLOWED_ROLE_NAMES = new Set(['triage', 'write', 'maintain', 'admin']);

// Recognized GitHub issue type names set by our three templates.
const KNOWN_ISSUE_TYPES = new Set(['Bug', 'Feature', 'Task']);

// =============================================================================
// PERMISSION CHECK
// =============================================================================

/**
 * Checks whether the commenter has triage-or-above repository permissions.
 * Uses the GitHub REST API `getCollaboratorPermissionLevel` endpoint, which
 * returns a `role_name` of 'read' | 'triage' | 'write' | 'maintain' | 'admin'.
 * Non-collaborators (HTTP 404) are treated as unauthorized.
 *
 * @param {object} botContext - Bot context from buildBotContext.
 * @param {string} username - GitHub username to check.
 * @returns {Promise<'authorized'|'unauthorized'|'error'>}
 */
async function checkPermission(botContext, username) {
  try {
    const { data } = await botContext.github.rest.repos.getCollaboratorPermissionLevel({
      owner: botContext.owner,
      repo: botContext.repo,
      username,
    });
    const roleName = data?.role_name;
    logger.log(`Permission check for @${username}: role_name="${roleName}"`);
    if (ALLOWED_ROLE_NAMES.has(roleName)) return 'authorized';
    return 'unauthorized';
  } catch (error) {
    const status = error?.status ?? error?.response?.status;
    if (status === 404) {
      logger.log(`@${username} is not a collaborator (404) — unauthorized`);
      return 'unauthorized';
    }
    logger.error(`Permission check failed for @${username}:`, error.message);
    return 'error';
  }
}

// =============================================================================
// LABEL VALIDATION
// =============================================================================

/**
 * Returns the GitHub issue type name ('Bug', 'Feature', 'Task') from the issue
 * payload, or null if not set or unrecognized.
 *
 * @param {object} issue - The issue object from the GitHub payload.
 * @returns {string|null}
 */
function getIssueTypeName(issue) {
  const name = issue?.type?.name;
  if (typeof name === 'string' && KNOWN_ISSUE_TYPES.has(name)) return name;
  return null;
}

/**
 * Formats an array of label names as a comma-separated inline code list.
 * @param {string[]} labels
 * @returns {string} e.g. "`skill: beginner`, `skill: intermediate`"
 */
function formatLabelList(labels) {
  return labels.map((l) => `\`${l}\``).join(', ');
}

/**
 * Collects all label validation violations for /finalize. Returns an empty
 * array when everything is valid.
 *
 * Rules:
 *   - status: awaiting triage must be present
 *   - exactly 1 skill: label
 *   - exactly 1 priority: label
 *   - issue type must be Bug, Feature, or Task
 *
 * @param {object} issue - The issue object from the GitHub payload.
 * @returns {string[]} Array of human-readable error strings (one per violation).
 */
function collectLabelViolations(issue) {
  const errors = [];

  const skillLabels = getLabelsByPrefix(issue, 'skill:');
  const priorityLabels = getLabelsByPrefix(issue, 'priority:');

  const issueTypeName = getIssueTypeName(issue);

  // 1. status: awaiting triage must be present
  if (!hasLabel(issue, LABELS.AWAITING_TRIAGE)) {
    const statusLabels = getLabelsByPrefix(issue, 'status:');
    const currentStatus = statusLabels.length > 0 ? formatLabelList(statusLabels) : 'none';
    errors.push(
      `The \`${LABELS.AWAITING_TRIAGE}\` label must be present to run \`/finalize\`. Current status label(s): ${currentStatus}.`
    );
  }

  // 2. Exactly 1 skill: label
  if (skillLabels.length === 0) {
    errors.push(
      `Exactly one \`skill:\` label is required (e.g. \`skill: beginner\`). None found. Choose from: \`${LABELS.GOOD_FIRST_ISSUE}\`, \`${LABELS.BEGINNER}\`, \`${LABELS.INTERMEDIATE}\`, \`${LABELS.ADVANCED}\`.`
    );
  } else if (skillLabels.length > 1) {
    errors.push(
      `Exactly one \`skill:\` label is required. Found ${skillLabels.length}: ${formatLabelList(skillLabels)}. Please remove all but one.`
    );
  }

  // 3. Exactly 1 priority: label
  if (priorityLabels.length === 0) {
    errors.push(
      `Exactly one \`priority:\` label is required (e.g. \`priority: medium\`). None found.`
    );
  } else if (priorityLabels.length > 1) {
    errors.push(
      `Exactly one \`priority:\` label is required. Found ${priorityLabels.length}: ${formatLabelList(priorityLabels)}. Please remove all but one.`
    );
  }

  // 4. Issue type must be recognized
  if (!issueTypeName) {
    errors.push(
      `The issue type (Bug, Feature, or Task) could not be determined. Ensure the issue was submitted using one of the official issue templates.`
    );
  }

  return errors;
}

// =============================================================================
// MAIN HANDLER
// =============================================================================

/**
 * Main handler for the /finalize command. Runs the following steps in order,
 * posting an informative comment and returning early if any step fails:
 *
 *   1. Acknowledge the comment with a thumbs-up reaction.
 *   2. Check commenter has triage+ permissions → unauthorized / API error comment.
 *   3. Collect all label violations → validation error comment listing all issues.
 *   4. Determine skill level and build updated title + body.
 *   5. Update the issue via the GitHub API → update failure comment on error.
 *   6. Swap status labels: awaiting triage → ready for dev.
 *   7. Post success comment.
 *
 * @param {{ github: object, owner: string, repo: string, number: number,
 *           issue: object, comment: { id: number, user: { login: string } } }} botContext
 *   Bot context from buildBotContext (issue_comment event).
 * @returns {Promise<void>}
 */
async function handleFinalize(botContext) {
  const finalizerUsername = botContext.comment.user.login;

  // STEP 1: Acknowledge
  await acknowledgeComment(botContext, botContext.comment.id);

  // STEP 2: Permission check
  const permResult = await checkPermission(botContext, finalizerUsername);
  if (permResult === 'error') {
    logger.log('Exit: permission check API error');
    await postComment(botContext, buildPermissionCheckErrorComment(finalizerUsername));
    return;
  }
  if (permResult === 'unauthorized') {
    logger.log(`Exit: @${finalizerUsername} is not authorized`);
    await postComment(botContext, buildUnauthorizedComment(finalizerUsername));
    return;
  }

  // STEP 3: Label validation — collect ALL violations before posting
  const violations = collectLabelViolations(botContext.issue);
  if (violations.length > 0) {
    logger.log(`Exit: ${violations.length} label violation(s) found`);
    await postComment(botContext, buildValidationErrorComment(finalizerUsername, violations));
    return;
  }

  // STEP 4: Determine skill level + build new title and body
  const skillLabels = getLabelsByPrefix(botContext.issue, 'skill:');
  const skillLevel = skillLabels[0]; // validated above: exactly 1 exists

  const newTitle = botContext.issue.title;
  const newBody = reconstructBody(botContext.issue.body || '', skillLevel);

  logger.log(`Updating issue #${botContext.number}: title="${newTitle}", skillLevel="${skillLevel}"`);

  // STEP 5: Update issue title and body
  let updateError = null;
  try {
    await botContext.github.rest.issues.update({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
      title: newTitle,
      body: newBody,
    });
    logger.log('Issue updated successfully');
  } catch (error) {
    updateError = error instanceof Error ? error.message : String(error);
    logger.error('Issue update failed:', updateError);
  }

  if (updateError) {
    await postComment(botContext, buildUpdateFailureComment(finalizerUsername, updateError));
    return;
  }

  // STEP 6: Swap status labels: awaiting triage → ready for dev
  const swapResult = await swapLabels(botContext, LABELS.AWAITING_TRIAGE, LABELS.READY_FOR_DEV);
  if (!swapResult.success) {
    await postComment(botContext, buildLabelSwapFailureComment(finalizerUsername, swapResult.errorDetails));
    logger.log('Posted label swap failure comment, tagged maintainers');
  }

  // STEP 7: Post success comment
  const priorityLabel = getLabelsByPrefix(botContext.issue, 'priority:')[0];
  await postComment(botContext, buildSuccessComment(finalizerUsername, skillLevel, priorityLabel));
  logger.log('Finalize flow completed successfully');
}

module.exports = { handleFinalize };
