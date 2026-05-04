// SPDX-License-Identifier: Apache-2.0
//
// commands/assign.js
//
// /assign command: assigns the commenter to the issue. Enforces skill-level
// prerequisites, assignment limits, and status labels. See bot-on-comment.js
// for high-level docs (limits, skill levels, required labels).

const {
  LABELS,
  ISSUE_STATE,
  createDelegatingLogger,
  isNonNegativeInteger,
  hasLabel,
  swapLabels,
  addAssignees,
  postComment,
  acknowledgeComment,
  getHighestIssueSkillLevel,
  countIssuesByAssignee,
  listAssignedIssues,
  hasNeedsReviewPR,
} = require('../helpers');

const {
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
} = require("./assign-comments");

// Delegate to the active logger set by the dispatcher (bot-on-comment.js).
// This ensures the correct prefix is used after command parsing.
const logger = createDelegatingLogger();

/**
 * Formats a count for user-facing text when a threshold short-circuit may have
 * capped the value. If count reaches the short-circuit threshold, return an
 * "at least" style display (e.g. "3+") rather than implying an exact value.
 *
 * @param {number} count - Count returned by countIssuesByAssignee.
 * @param {number} threshold - Threshold used for short-circuiting.
 * @returns {string} User-facing display string.
 */
function formatThresholdedCount(count, threshold) {
  return count >= threshold ? `${threshold}+` : String(count);
}

/**
 * Returns the number of open issues assigned to the user that carry the
 * "status: blocked" label. Used to provide context in the assignment-limit-exceeded
 * comment (blocked issues don't count toward the limit).
 *
 * @param {object} github - Octokit GitHub API client.
 * @param {string} owner - Repository owner.
 * @param {string} repo - Repository name.
 * @param {string} username - GitHub username to search for.
 * @returns {Promise<number>} The blocked-issue count (defaults to 0 on any error).
 */
async function getBlockedCount(github, owner, repo, username) {
  const count = await countIssuesByAssignee(
    github,
    owner,
    repo,
    username,
    ISSUE_STATE.OPEN,
    LABELS.BLOCKED,
    1,
  );
  if (count !== null && isNonNegativeInteger(count)) return Math.floor(count);
  return 0;
}

/**
 * Checks skill-level prerequisites for the requesting user. Looks up the
 * required number of completed issues at the prerequisite level (e.g. a
 * Beginner issue requires 2 completed Good First Issues). Posts a comment
 * and returns false when the check fails or an API error occurs; returns
 * true when prerequisites are satisfied (or not required for the level).
 *
 * @param {object} botContext - Bot context from buildBotContext (github, owner, repo, number).
 * @param {string} skillLevel - The skill-level label on the issue (a LABELS constant).
 * @param {string} requesterUsername - The GitHub username requesting assignment.
 * @returns {Promise<boolean>} True if prerequisites are met or none required; false otherwise.
 */
async function checkPrerequisites(botContext, skillLevel, requesterUsername) {
  const prereq = SKILL_PREREQUISITES[skillLevel];
  if (!prereq.requiredLabel || prereq.requiredCount <= 0) return true;

  // Bypass: If the user already has worked on any level or higher then bypass them.
  const skillIndex = SKILL_HIERARCHY.indexOf(skillLevel);
  if (skillIndex !== -1) {
    for (let i = skillIndex; i < SKILL_HIERARCHY.length; i++) {
      const checkCurrLevel = SKILL_HIERARCHY[i];
      const countAtLevel = await countIssuesByAssignee(
        botContext.github,
        botContext.owner,
        botContext.repo,
        requesterUsername,
        ISSUE_STATE.CLOSED,
        checkCurrLevel,
        1,
      );

      if (countAtLevel !== null && countAtLevel > 0) {
        logger.log(
          `Bypassing prerequisites: user has completed ${countAtLevel} issues with label "${checkCurrLevel}"`,
        );
        return true;
      }
    }
  }

  // Normal validation
  const completedCount = await countIssuesByAssignee(
    botContext.github,
    botContext.owner,
    botContext.repo,
    requesterUsername,
    ISSUE_STATE.CLOSED,
    prereq.requiredLabel,
    prereq.requiredCount,
  );
  if (completedCount === null) {
    logger.log("Exit: could not verify prerequisites due to API error");
    await postComment(botContext, buildApiErrorComment(requesterUsername));
    logger.log("Posted API error comment, tagged maintainers");
    return false;
  }
  if (completedCount < prereq.requiredCount) {
    logger.log("Exit: prerequisites not met", {
      required: prereq.requiredCount,
      completed: completedCount,
    });
    await postComment(
      botContext,
      buildPrerequisiteNotMetComment(
        requesterUsername,
        skillLevel,
        completedCount,
        botContext.owner,
        botContext.repo,
      ),
    );
    logger.log("Posted prerequisite-not-met comment");
    return false;
  }
  logger.log("Prerequisites met:", {
    required: prereq.requiredCount,
    completed: completedCount,
  });
  return true;
}

/**
 * Transitions issue status labels after a successful assignment: removes
 * "status: ready for dev" and adds "status: in progress". If either label
 * operation fails, posts a comment tagging maintainers with manual instructions.
 *
 * @param {object} botContext - Bot context from buildBotContext (github, owner, repo, number).
 * @param {string} requesterUsername - The username being assigned (for the failure comment).
 * @returns {Promise<void>}
 */
async function updateLabels(botContext, requesterUsername) {
  const { success, errorDetails } = await swapLabels(
    botContext,
    LABELS.READY_FOR_DEV,
    LABELS.IN_PROGRESS,
  );
  if (!success) {
    await postComment(
      botContext,
      buildLabelUpdateFailureComment(requesterUsername, errorDetails),
    );
    logger.log("Posted label update failure comment, tagged maintainers");
  }
}

/**
 * Checks whether the requester has reached the GFI completion cap. Only applies
 * when skillLevel is LABELS.GOOD_FIRST_ISSUE. Posts an encouraging comment and
 * returns false when the cap is reached; returns true otherwise.
 *
 * @param {object} botContext - Bot context from buildBotContext.
 * @param {string} skillLevel - The skill-level label on the issue (a LABELS constant).
 * @param {string} requesterUsername - GitHub username requesting assignment.
 * @returns {Promise<boolean>} True if under the cap or not a GFI; false otherwise.
 */
async function enforceGfiCompletionLimit(
  botContext,
  skillLevel,
  requesterUsername,
) {
  if (skillLevel !== LABELS.GOOD_FIRST_ISSUE) return true;

  const completedCount = await countIssuesByAssignee(
    botContext.github,
    botContext.owner,
    botContext.repo,
    requesterUsername,
    ISSUE_STATE.CLOSED,
    LABELS.GOOD_FIRST_ISSUE,
    MAX_GFI_COMPLETIONS + 1,
  );
  if (completedCount === null) {
    logger.log("Exit: could not verify GFI completion count due to API error");
    await postComment(botContext, buildApiErrorComment(requesterUsername));
    logger.log("Posted API error comment, tagged maintainers");
    return false;
  }
  if (completedCount >= MAX_GFI_COMPLETIONS) {
    logger.log("Exit: contributor has reached GFI completion cap", {
      maxAllowed: MAX_GFI_COMPLETIONS,
      completedCount,
    });
    await postComment(
      botContext,
      buildGfiLimitExceededComment(
        requesterUsername,
        formatThresholdedCount(completedCount, MAX_GFI_COMPLETIONS + 1),
        botContext.owner,
        botContext.repo,
      ),
    );
    logger.log("Posted GFI-limit-exceeded comment");
    return false;
  }
  logger.log("GFI completion count OK:", {
    maxAllowed: MAX_GFI_COMPLETIONS,
    completedCount,
  });
  return true;
}

/**
 * Validates that the issue is in a state that allows assignment. Checks three
 * gates in order: no existing assignees, "status: ready for dev" label present,
 * and a skill-level label present. Posts an informative comment and returns null
 * when any gate fails.
 *
 * @param {object} botContext - Bot context from buildBotContext.
 * @param {string} requesterUsername - GitHub username requesting assignment.
 * @returns {Promise<string|null>} The skill-level label, or null if a gate failed.
 */
async function validateIssueState(botContext, requesterUsername) {
  if (botContext.issue.assignees?.length > 0) {
    logger.log(
      "Exit: issue already assigned to",
      botContext.issue.assignees.map((a) => a.login),
    );
    await postComment(
      botContext,
      buildAlreadyAssignedComment(
        requesterUsername,
        botContext.issue,
        botContext.owner,
        botContext.repo,
      ),
    );
    logger.log("Posted already-assigned comment");
    return null;
  }

  if (!hasLabel(botContext.issue, LABELS.READY_FOR_DEV)) {
    logger.log("Exit: issue missing ready for dev label");
    await postComment(
      botContext,
      buildNotReadyComment(
        requesterUsername,
        botContext.owner,
        botContext.repo,
      ),
    );
    logger.log("Posted not-ready comment");
    return null;
  }

  const skillLevel = getHighestIssueSkillLevel(botContext.issue);
  if (!skillLevel) {
    logger.log("Exit: issue has no skill level label");
    await postComment(botContext, buildNoSkillLevelComment(requesterUsername));
    logger.log("Posted no-skill-level comment");
    return null;
  }
  logger.log("Issue skill level:", skillLevel);
  return skillLevel;
}

/**
 * Verifies the requester has not reached the open-assignment limit. Posts an
 * API-error or limit-exceeded comment and returns false when the check fails;
 * returns true when the user is within limits.
 *
 * @param {object} botContext - Bot context from buildBotContext.
 * @param {string} requesterUsername - GitHub username requesting assignment.
 * @returns {Promise<boolean>} True if within assignment limits; false otherwise.
 */
async function enforceAssignmentLimit(botContext, requesterUsername) {
  const openAssignmentCount = await countIssuesByAssignee(
    botContext.github,
    botContext.owner,
    botContext.repo,
    requesterUsername,
    ISSUE_STATE.OPEN,
    null,
    MAX_OPEN_ASSIGNMENTS + 1,
  );
  if (openAssignmentCount === null) {
    logger.log("Exit: could not verify open assignments due to API error");
    await postComment(botContext, buildApiErrorComment(requesterUsername));
    logger.log("Posted API error comment, tagged maintainers");
    return false;
  }

  if (openAssignmentCount >= MAX_OPEN_ASSIGNMENTS) {
    logger.log("Contributor at or above assignment cap", {
      maxAllowed: MAX_OPEN_ASSIGNMENTS,
      currentCount: openAssignmentCount,
    });

    // --- Needs-review bypass check ---
    // If every open (non-blocked) assigned issue has an open PR authored by
    // the contributor with "status: needs review", allow the bypass.
    const assignedIssues = await listAssignedIssues(
      botContext.github,
      botContext.owner,
      botContext.repo,
      requesterUsername,
    );

    if (assignedIssues === null) {
      logger.log("Exit: could not fetch assigned issues due to API error");
      await postComment(botContext, buildApiErrorComment(requesterUsername));
      logger.log("Posted API error comment, tagged maintainers");
      return false;
    }

    // Guard against vacuous truth: [].every(fn) === true in JavaScript.
    // An empty array can occur from eventual consistency or race conditions;
    // it must NOT silently trigger the bypass.
    if (assignedIssues.length > 0) {
      let allHaveNeedsReviewPR = true;
      for (const issue of assignedIssues) {
        const result = await hasNeedsReviewPR(
          botContext.github,
          botContext.owner,
          botContext.repo,
          requesterUsername,
          issue.number,
        );
        if (result === null) {
          logger.log(
            `Exit: API error checking needs-review PR for issue #${issue.number}`,
          );
          await postComment(
            botContext,
            buildApiErrorComment(requesterUsername),
          );
          logger.log("Posted API error comment, tagged maintainers");
          return false;
        }
        if (!result) {
          allHaveNeedsReviewPR = false;
          break;
        }
      }

      if (allHaveNeedsReviewPR) {
        logger.log(
          "Bypass: all assigned issues have linked needs-review PRs",
          { issueCount: assignedIssues.length },
        );
        return true;
      }
    } else {
      logger.log(
        "Bypass skipped: listAssignedIssues returned 0 issues despite count >= limit",
      );
    }

    // Fall through to the existing limit-exceeded path
    logger.log("Exit: contributor has too many open assignments", {
      maxAllowed: MAX_OPEN_ASSIGNMENTS,
      currentCount: openAssignmentCount,
    });
    const blockedCount = await getBlockedCount(
      botContext.github,
      botContext.owner,
      botContext.repo,
      requesterUsername,
    );
    await postComment(
      botContext,
      buildAssignmentLimitExceededComment(
        requesterUsername,
        formatThresholdedCount(openAssignmentCount, MAX_OPEN_ASSIGNMENTS + 1),
        botContext.owner,
        botContext.repo,
        blockedCount,
      ),
    );
    logger.log("Posted assignment-limit-exceeded comment");
    return false;
  }

  logger.log("Open assignment count OK:", {
    maxAllowed: MAX_OPEN_ASSIGNMENTS,
    currentCount: openAssignmentCount,
  });
  return true;
}

/**
 * Performs the actual issue assignment with a same-issue race defense:
 *
 * 1. Pre-flight check: fetches fresh issue state via issues.get() to detect
 *    same-issue collisions (another user assigned while this run was queued;
 *    the workflow concurrency key is serialized per issue).
 *
 * On success, posts a welcome comment and transitions status labels.
 * Posts failure comments if any API call fails.
 *
 * @param {object} botContext - Bot context from buildBotContext.
 * @param {string} requesterUsername - GitHub username being assigned.
 * @param {string} skillLevel - The skill-level label on the issue.
 * @returns {Promise<void>}
 */
async function assignAndFinalize(botContext, requesterUsername, skillLevel) {
  logger.log("Assigning issue to", requesterUsername);

  // Pre-flight same-issue collision check: fetch live issue state BEFORE writing.
  // The per-issue concurrency key serializes runs on the same issue, so by the
  // time this run executes, another user may already be assigned.
  let freshIssue;
  try {
    const response = await botContext.github.rest.issues.get({
      owner: botContext.owner,
      repo: botContext.repo,
      issue_number: botContext.number,
    });
    freshIssue = response.data;
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    logger.error("Failed to pre-fetch fresh issue state:", errorMsg);
    await postComment(botContext, buildApiErrorComment(requesterUsername));
    return;
  }

  const alreadyAssigned = freshIssue.assignees?.some(
    (a) => a.login === requesterUsername,
  );

  if (alreadyAssigned) {
    logger.log("Exit: user is already assigned (caught during fresh fetch)");
    await postComment(
      botContext,
      buildAlreadyAssignedComment(
        requesterUsername,
        freshIssue,
        botContext.owner,
        botContext.repo,
      ),
    );
    return;
  }

  if (freshIssue.assignees?.length > 0) {
    logger.log("Exit: issue was assigned by another user while queued");
    await postComment(
      botContext,
      buildAlreadyAssignedComment(
        requesterUsername,
        freshIssue,
        botContext.owner,
        botContext.repo,
      ),
    );
    return;
  }

  // Revalidate labels from the fresh issue snapshot to prevent stale-event
  // payload eligibility checks when labels change while this run is queued.
  if (!hasLabel(freshIssue, LABELS.READY_FOR_DEV)) {
    logger.log("Exit: fresh issue state is missing ready for dev label");
    await postComment(
      botContext,
      buildNotReadyComment(
        requesterUsername,
        botContext.owner,
        botContext.repo,
      ),
    );
    return;
  }

  const freshSkillLevel = getHighestIssueSkillLevel(freshIssue);
  if (!freshSkillLevel) {
    logger.log("Exit: fresh issue state has no skill level label");
    await postComment(botContext, buildNoSkillLevelComment(requesterUsername));
    return;
  }

  // If the skill level changed while this run was queued, the earlier prerequisite
  // and GFI-cap gates were run against a stale label. Abort and ask user to retry.
  if (freshSkillLevel !== skillLevel) {
    logger.log("Exit: skill level changed while queued", {
      stalePayloadLevel: skillLevel,
      freshReadLevel: freshSkillLevel,
    });
    await postComment(
      botContext,
      buildSkillLevelChangedComment(
        requesterUsername,
        skillLevel,
        freshSkillLevel,
      ),
    );
    logger.log("Posted skill-level-changed comment");
    return;
  }

  const assignResult = await addAssignees(botContext, [requesterUsername]);
  if (!assignResult.success) {
    await postComment(
      botContext,
      buildAssignmentFailureComment(requesterUsername, assignResult.error),
    );
    logger.log("Posted assignment failure comment, tagged maintainers");
    return;
  }

  await postComment(
    botContext,
    buildWelcomeComment(requesterUsername, freshSkillLevel),
  );
  logger.log("Posted welcome comment");
  await updateLabels(botContext, requesterUsername);
  logger.log("Assignment flow completed successfully");
}

/**
 * Main handler for the /assign command. Runs the following gates in order, posting
 * an informative comment and returning early if any gate fails:
 *
 *   1. Acknowledge the comment with a thumbs-up reaction.
 *   2. Issue already assigned? -> already-assigned comment.
 *   3. Missing "status: ready for dev" label? -> not-ready comment.
 *   4. No skill-level label? -> no-skill-level comment (tags maintainers).
 *   5. Open-assignment count API error? -> API-error comment (tags maintainers).
 *   6. At or above MAX_OPEN_ASSIGNMENTS? -> limit-exceeded comment.
 *   7. GFI cap reached (skill: good first issue only)? -> GFI-limit-exceeded comment.
 *   8. Skill prerequisites not met? -> prerequisite-not-met comment.
 *   9. Issue snatched while queued (fresh fetch)? -> already-assigned comment.
 *   10. Fresh labels invalid while queued? -> not-ready/no-skill-level comment.
 *   11. Skill level changed while queued? -> skill-level-changed comment (ask to retry).
 *   12. Assignment API failure? -> assignment-failure comment (tags maintainers).
 *
 * If all checks pass, the bot assigns the issue, posts a welcome comment,
 * and swaps the "status: ready for dev" label with "status: in progress".
 *
 * @param {{ github: object, owner: string, repo: string, number: number,
 *           issue: object, comment: { id: number, user: { login: string } } }} botContext
 *   Bot context from buildBotContext (issue_comment event).
 * @returns {Promise<void>}
 */
async function handleAssign(botContext) {
  const requesterUsername = botContext.comment.user.login;

  const ack = await acknowledgeComment(botContext, botContext.comment.id);
  if (!ack.success) {
    logger.log(
      "Aborting /assign: triggering comment was deleted or could not be acknowledged",
    );
    return;
  }

  const skillLevel = await validateIssueState(botContext, requesterUsername);
  if (!skillLevel) return;

  const withinLimit = await enforceAssignmentLimit(
    botContext,
    requesterUsername,
  );
  if (!withinLimit) return;

  const withinGfiCap = await enforceGfiCompletionLimit(
    botContext,
    skillLevel,
    requesterUsername,
  );
  if (!withinGfiCap) return;

  const prereqsPassed = await checkPrerequisites(
    botContext,
    skillLevel,
    requesterUsername,
  );
  if (!prereqsPassed) return;

  await assignAndFinalize(botContext, requesterUsername, skillLevel);
}

module.exports = { handleAssign };
