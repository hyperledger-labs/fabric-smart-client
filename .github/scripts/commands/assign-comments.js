// SPDX-License-Identifier: Apache-2.0
//
// commands/assign-comments.js
//
// Comment builders for the /assign command. Pure formatting functions
// separated from assignment logic for readability.

const {
  PROJECT_NAME,
  MAINTAINER_TEAM,
  LABELS,
  ISSUE_STATE,
  SKILL_HIERARCHY,
  SKILL_PREREQUISITES,
} = require("../helpers");

/**
 * Maximum number of open (non-blocked) issues a contributor can be assigned to
 * at the same time. Enforced by handleAssign in assign.js.
 * @type {number}
 */
const MAX_OPEN_ASSIGNMENTS = 2;

/**
 * Maximum number of Good First Issues a contributor may complete before being
 * redirected to Beginner and higher-level issues. Enforced by handleAssign in assign.js.
 * @type {number}
 */
const MAX_GFI_COMPLETIONS = 5;

/**
 * Builds the welcome comment posted after a successful assignment. Returns a
 * special first-timer welcome for Good First Issues (mentioning the support team),
 * or a shorter returning-contributor message for all other skill levels.
 *
 * @param {string} username - The GitHub username being assigned.
 * @param {string} skillLevel - The skill-level label on the issue (a LABELS constant).
 * @returns {string} The formatted Markdown comment body.
 */
function buildWelcomeComment(username, skillLevel) {
  const isGoodFirstIssue = skillLevel === LABELS.GOOD_FIRST_ISSUE;
  const skillDisplayName =
    SKILL_PREREQUISITES[skillLevel]?.displayName || "issue";
  if (isGoodFirstIssue) {
    return [
      `👋 Hi @${username}, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉`,
      "",
      "The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.",
      "",
      "If you realize you cannot complete this issue, simply comment `/unassign` to return it to the community pool.",
      "",
      "Good luck, and welcome aboard! 🚀",
    ].join("\n");
  }
  return [
    `👋 Hi @${username}, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **${skillDisplayName}** issue. 🙌`,
    "",
    "If this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.",
    "",
    "If you realize you cannot complete this issue, simply comment `/unassign` to return it to the pool.",
    "",
    "Good luck! 🚀",
  ].join("\n");
}

/**
 * Builds the comment posted when an issue is already assigned. If the requester
 * is the current assignee, confirms they're already set; otherwise names the
 * current assignee and links to unassigned ready issues.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {{ assignees?: Array<{ login: string }> }} issue - The issue object from the payload.
 * @param {string} owner - Repository owner (for the browse-issues URL).
 * @param {string} repo - Repository name (for the browse-issues URL).
 * @returns {string} The formatted Markdown comment body.
 */
function buildAlreadyAssignedComment(requesterUsername, issue, owner, repo) {
  const isAssignedToSelf = issue?.assignees?.some(
    (a) =>
      (a.login || "").toLowerCase() === (requesterUsername || "").toLowerCase(),
  );
  if (isAssignedToSelf) {
    const otherAssignees = (issue?.assignees || [])
      .map((assignee) => assignee?.login)
      .filter(
        (login) =>
          login &&
          login.toLowerCase() !== (requesterUsername || "").toLowerCase(),
      );
    const currentAssignmentLine =
      otherAssignees.length > 0
        ? `You're already assigned to this issue, which is also assigned to ${otherAssignees.map((login) => `@${login}`).join(", ")}.`
        : "You're already assigned to this issue. You're all set to start working on it!";
    return [
      `👋 Hi @${requesterUsername}! ${currentAssignmentLine}`,
      "",
      "If you have any questions, feel free to ask here or reach out to the team.",
    ].join("\n");
  }
  const assigneeLogins = (issue?.assignees || [])
    .map((assignee) => assignee?.login)
    .filter(Boolean);
  const currentAssignee =
    assigneeLogins.length === 0
      ? "someone"
      : assigneeLogins.map((login) => `@${login}`).join(", ");
  return [
    `👋 Hi @${requesterUsername}! This issue is already assigned to ${currentAssignee}.`,
    "",
    "👉 **Find another issue to work on:**",
    `[Browse unassigned issues](https://github.com/${owner}/${repo}/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)`,
    "",
    "Once you find one you like, comment `/assign` to get started!",
  ].join("\n");
}

/**
 * Builds the comment posted when the issue is missing the "status: ready for dev"
 * label. Explains the requirement and links to issues that are ready.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string} owner - Repository owner (for the browse-issues URL).
 * @param {string} repo - Repository name (for the browse-issues URL).
 * @returns {string} The formatted Markdown comment body.
 */
function buildNotReadyComment(requesterUsername, owner, repo) {
  return [
    `👋 Hi @${requesterUsername}! This issue is not ready for development yet.`,
    "",
    `Issues must have the \`${LABELS.READY_FOR_DEV}\` label before they can be assigned.`,
    "",
    "👉 **Find an issue that's ready:**",
    `[Browse ready issues](https://github.com/${owner}/${repo}/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)`,
    "",
    "Once you find one you like, comment `/assign` to get started!",
  ].join("\n");
}

/**
 * Builds the comment posted when a contributor hasn't completed enough prerequisite
 * issues. Shows the required count, current progress, and a search link to find
 * available prerequisite-level issues.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string} skillLevel - The skill-level label on the issue (a LABELS constant).
 * @param {number} completedCount - How many prerequisite issues the user has completed so far.
 * @param {string} owner - Repository owner (for the search URL).
 * @param {string} repo - Repository name (for the search URL).
 * @returns {string} The formatted Markdown comment body.
 */
function buildPrerequisiteNotMetComment(
  requesterUsername,
  skillLevel,
  completedCount,
  owner,
  repo,
) {
  const prereq = SKILL_PREREQUISITES[skillLevel];
  const { requiredLabel, requiredCount, prerequisiteDisplayName, displayName } =
    prereq;
  const searchQuery = `is:issue is:open no:assignee label:"${requiredLabel}" label:"${LABELS.READY_FOR_DEV}"`;
  const searchUrl = `https://github.com/${owner}/${repo}/issues?q=${encodeURIComponent(searchQuery)}`;
  return [
    `👋 Hi @${requesterUsername}! Thanks for your interest in contributing!`,
    "",
    `This is a **${displayName}** issue. Before taking it on, you need to complete at least **${requiredCount} ${prerequisiteDisplayName}** to build familiarity with the codebase.`,
    "",
    `📊 **Your Progress:** You've completed **${completedCount}** so far.`,
    "",
    `👉 **Find ${prerequisiteDisplayName} to work on:**`,
    `[Browse available ${prerequisiteDisplayName}](${searchUrl})`,
    "",
    `Once you've completed ${requiredCount}, come back and we'll be happy to assign this to you! 🎯`,
  ].join("\n");
}

/**
 * Builds the comment posted when the issue has no skill-level label. Tags the
 * maintainer team to add one of the four skill levels, and instructs the
 * requester to try /assign again once the label is added.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @returns {string} The formatted Markdown comment body.
 */
function buildNoSkillLevelComment(requesterUsername) {
  return [
    `👋 Hi @${requesterUsername}! This issue doesn't have a skill level label yet.`,
    "",
    `${MAINTAINER_TEAM} — could you please add one of the following labels?`,
    `- \`${LABELS.GOOD_FIRST_ISSUE}\``,
    `- \`${LABELS.BEGINNER}\``,
    `- \`${LABELS.INTERMEDIATE}\``,
    `- \`${LABELS.ADVANCED}\``,
    "",
    `@${requesterUsername}, once a maintainer adds the label, comment \`/assign\` again to request assignment.`,
  ].join("\n");
}

/**
 * Builds the comment posted when the issue's skill level changes while the
 * assignment request is queued. Tells the requester to retry so eligibility
 * checks run against the latest label state.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string} previousSkillLevel - The skill level from the event payload.
 * @param {string} currentSkillLevel - The skill level from the fresh issue snapshot.
 * @returns {string} The formatted Markdown comment body.
 */
function buildSkillLevelChangedComment(
  requesterUsername,
  previousSkillLevel,
  currentSkillLevel,
) {
  return [
    `👋 Hi @${requesterUsername}! The skill level for this issue changed while your assignment request was being processed.`,
    "",
    `**Current label:** \`${currentSkillLevel}\``,
    `**Previous label:** \`${previousSkillLevel}\``,
    "",
    "Please comment `/assign` again to request assignment with the updated skill requirements.",
  ].join("\n");
}

/**
 * Builds a GitHub Issues search URL for the given repository and query string.
 *
 * @param {string} owner - Repository owner.
 * @param {string} repo - Repository name.
 * @param {string} searchQuery - Raw GitHub search query (will be URI-encoded).
 * @returns {string} The full HTTPS search URL.
 */
function buildIssuesSearchUrl(owner, repo, searchQuery) {
  return `https://github.com/${owner}/${repo}/issues?q=${encodeURIComponent(searchQuery)}`;
}

/**
 * Internal formatter for the assignment-limit-exceeded comment. Accepts pre-built
 * URLs so it can be tested independently of URL construction logic. Explains the
 * limit, shows the user's current count, and links to their assigned and
 * (optionally) blocked issues.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string|number} openCount - User-facing open-assignment count display (for example 2 or "3+").
 * @param {string} assignedIssuesUrl - URL to the user's open assigned issues search.
 * @param {string|null} blockedIssuesUrl - URL to the user's blocked issues search, or null if none.
 * @returns {string} The formatted Markdown comment body.
 */
function formatAssignmentLimitExceededComment(
  requesterUsername,
  openCount,
  assignedIssuesUrl,
  blockedIssuesUrl,
) {
  const lines = [
    `👋 Hi @${requesterUsername}! Thanks for your enthusiasm to contribute!`,
    "",
    `To help contributors stay focused and ensure issues remain available for others, we limit assignments to **${MAX_OPEN_ASSIGNMENTS} open issues** at a time. Issues labeled \`${LABELS.BLOCKED}\` are not counted toward this limit.`,
    "",
    `📊 **Your Current Assignments:** You're currently assigned to **${openCount}** open issues.`,
    "",
    "👉 **View your assigned issues:**",
    `[Your open assignments](${assignedIssuesUrl})`,
  ];
  if (blockedIssuesUrl) {
    lines.push(
      "",
      "👉 **View your blocked issues:**",
      `[Your blocked issues](${blockedIssuesUrl})`,
    );
  }
  lines.push(
    "",
    `💡 **Tip:** If all of your open assigned issues have a linked PR with \`${LABELS.NEEDS_REVIEW}\`, the limit is automatically bypassed — you can request a new assignment right away.`,
    "",
    "Once you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯",
  );
  return lines.join("\n");
}

/**
 * Builds the comment posted when a contributor is at or above the maximum number
 * of open assignments (MAX_OPEN_ASSIGNMENTS). Constructs search URLs for the
 * user's assigned and blocked issues, then delegates to formatAssignmentLimitExceededComment.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string|number} openCount - User-facing open-assignment count display (for example 2 or "3+").
 * @param {string} owner - Repository owner (for search URLs).
 * @param {string} repo - Repository name (for search URLs).
 * @param {number} [blockedCount=0] - Number of blocked issues assigned to the user (shows link if > 0).
 * @returns {string} The formatted Markdown comment body.
 */
function buildAssignmentLimitExceededComment(
  requesterUsername,
  openCount,
  owner,
  repo,
  blockedCount = 0,
) {
  const assignedQuery = `is:issue is:${ISSUE_STATE.OPEN} assignee:${requesterUsername} -label:"${LABELS.BLOCKED}"`;
  const assignedIssuesUrl = buildIssuesSearchUrl(owner, repo, assignedQuery);
  const blockedIssuesUrl =
    blockedCount > 0
      ? buildIssuesSearchUrl(
          owner,
          repo,
          `is:issue is:${ISSUE_STATE.OPEN} assignee:${requesterUsername} label:"${LABELS.BLOCKED}"`,
        )
      : null;
  return formatAssignmentLimitExceededComment(
    requesterUsername,
    openCount,
    assignedIssuesUrl,
    blockedIssuesUrl,
  );
}

/**
 * Builds the comment posted when a GitHub API call fails during eligibility
 * verification (e.g. counting open assignments or checking prerequisites).
 * Tags the maintainer team for manual assistance.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @returns {string} The formatted Markdown comment body.
 */
function buildApiErrorComment(requesterUsername) {
  return [
    `👋 Hi @${requesterUsername}! I encountered an error while trying to verify your eligibility for this issue.`,
    "",
    `${MAINTAINER_TEAM} — could you please help with this assignment request?`,
    "",
    `@${requesterUsername}, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
  ].join("\n");
}

/**
 * Builds the comment posted when the user was successfully assigned but the
 * label transition ("ready for dev" -> "in progress") failed. Tags the maintainer
 * team with explicit manual steps and includes the error details.
 *
 * @param {string} username - The GitHub username who was assigned.
 * @param {string} error - The error message(s) from the failed label operations.
 * @returns {string} The formatted Markdown comment body.
 */
function buildLabelUpdateFailureComment(username, error) {
  return [
    `⚠️ @${username} has been successfully assigned to this issue, but I encountered an error updating the labels.`,
    "",
    `${MAINTAINER_TEAM} — please manually:`,
    `- Remove the \`${LABELS.READY_FOR_DEV}\` label`,
    `- Add the \`${LABELS.IN_PROGRESS}\` label`,
    "",
    `Error details: ${error}`,
  ].join("\n");
}

/**
 * Builds the comment posted when a contributor has already completed the maximum
 * number of Good First Issues (MAX_GFI_COMPLETIONS). Rejects the assignment
 * warmly and redirects them toward Beginner and higher-level issues.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string|number} completedCount - User-facing completed-count display (for example 5 or "5+").
 * @param {string} owner - Repository owner (for the search URL).
 * @param {string} repo - Repository name (for the search URL).
 * @returns {string} The formatted Markdown comment body.
 */
function buildGfiLimitExceededComment(
  requesterUsername,
  completedCount,
  owner,
  repo,
) {
  const searchQuery = `is:issue is:open no:assignee label:"${LABELS.BEGINNER}" label:"${LABELS.READY_FOR_DEV}"`;
  const searchUrl = buildIssuesSearchUrl(owner, repo, searchQuery);
  return [
    `👋 Hi @${requesterUsername}! You've completed **${completedCount} Good First Issues** — that's a fantastic achievement, and it shows you know the workflow inside and out. 🎉`,
    "",
    "Good First Issues are designed to help new contributors get comfortable with the process, and you've clearly mastered it. We believe you're more than ready to take on bigger challenges!",
    "",
    "👉 **Find Beginner and higher issues to work on:**",
    `[Browse available Beginner issues](${searchUrl})`,
    "",
    "Come take on something more challenging — we're excited to see what you'll build next! 🚀",
  ].join("\n");
}

/**
 * Builds the comment posted when the addAssignees API call itself fails.
 * Tags the maintainer team to manually assign the user and includes the error details.
 *
 * @param {string} requesterUsername - The GitHub username who commented /assign.
 * @param {string} error - The error message from the failed assignment call.
 * @returns {string} The formatted Markdown comment body.
 */
function buildAssignmentFailureComment(requesterUsername, error) {
  return [
    `⚠️ Hi @${requesterUsername}! I tried to assign you to this issue, but encountered an error.`,
    "",
    `${MAINTAINER_TEAM} — could you please manually assign @${requesterUsername} to this issue?`,
    "",
    `Error details: ${error}`,
  ].join("\n");
}

module.exports = {
  MAX_OPEN_ASSIGNMENTS,
  MAX_GFI_COMPLETIONS,
  SKILL_HIERARCHY,
  SKILL_PREREQUISITES,
  buildWelcomeComment,
  buildAlreadyAssignedComment,
  buildNotReadyComment,
  buildPrerequisiteNotMetComment,
  buildNoSkillLevelComment,
  buildSkillLevelChangedComment,
  buildAssignmentLimitExceededComment,
  buildGfiLimitExceededComment,
  buildApiErrorComment,
  buildLabelUpdateFailureComment,
  buildAssignmentFailureComment,
};
