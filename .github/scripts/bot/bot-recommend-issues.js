// SPDX-License-Identifier: Apache-2.0
//
// commands/recommend-issues.js
//
// Issue recommendation command: suggests relevant issues to contributors
// after a PR is closed. Uses a history-based eligibility model to recommend issues strictly
// at the highest level the contributor is currently allowed to claim.

const {
    MAINTAINER_TEAM,
    LABELS,
    SKILL_HIERARCHY,
    SKILL_PREREQUISITES,
    ISSUE_STATE,
    hasLabel,
    postComment,
    getHighestIssueSkillLevel,
    countIssuesByAssignee,
    PRIORITY_HIERARCHY,
    createDelegatingLogger,
} = require('../helpers');

const logger = createDelegatingLogger();

/**
 * Groups issues by their matching difficulty level.
 *
 * Each issue is assigned to the first matching level in levelsPriority.
 *
 * @param {Array<object>} issues
 * @param {string[]} levelsPriority
 * @returns {Object<string, Array<object>>} Issues grouped by level.
 */
function groupIssuesByLevel(issues, levelsPriority) {
    const grouped = Object.fromEntries(
        levelsPriority.map(level => [level, []])
    );

    for (const issue of issues) {
        const level = levelsPriority.find(l => hasLabel(issue, l));
        if (level) grouped[level].push(issue);
    }

    return grouped;
}

/**
 * Returns issues from the highest-priority level with results.
 *
 * Limits output to 5 issues.
 *
 * @param {Object<string, Array<object>>} grouped
 * @param {string[]} levelsPriority
 * @returns {Array<object>} Selected issues or empty array.
 */
function pickFirstAvailableLevel(grouped, levelsPriority) {
    for (const level of levelsPriority) {
        if (grouped[level].length > 0) {
            return grouped[level].slice(0, 5);
        }
    }
    return [];
}

/**
 * Fetches issues for multiple levels in a single query.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @returns {Promise<Array<{ title: string, html_url: string, labels: Array }> | null>}
 */
async function fetchIssuesBatch(github, owner, repo) {
    try {

        const query = [
            `repo:${owner}/${repo}`,
            'is:issue',
            'is:open',
            'no:assignee',
            `label:"${LABELS.READY_FOR_DEV}"`
        ].join(' ');

        const result = await github.rest.search.issuesAndPullRequests({
            q: query,
            per_page: 50,
            sort: 'created',
            order: 'asc',
        });

        return result.data.items || [];
    } catch (error) {
        logger.error('Failed to fetch issues:', {
            message: error.message,
            status: error.status,
        });
        return null;
    }
}

/**
 * Builds the success comment listing recommended issues.
 *
 * When `unlockedLevel` is provided, a congratulatory block is prepended to
 * acknowledge that this contribution crossed the threshold into a new skill
 * level. The display name is sourced from SKILL_PREREQUISITES so it stays
 * consistent with the rest of the bot.
 *
 * @param {string} username
 * @param {Array<{ title: string, html_url: string }>} issues
 * @param {string|null} unlockedLevel - LABELS constant for the newly unlocked
 *   level, or null if no threshold was crossed.
 * @returns {string}
 */
function buildRecommendationComment(username, issues, unlockedLevel = null) {
    const list = issues.map(
        (issue) => `- [${issue.title}](${issue.html_url})`
    );

    const congratsBlock = unlockedLevel
        ? [
            `🏆 Milestone unlocked: you've just reached **${SKILL_PREREQUISITES[unlockedLevel].displayName}** level!`,
            `That's a big step — well done. 🎊`,
            '',
        ]
        : [];

    return [
        `👋 Hi @${username}! Great work on your recent contribution! 🎉`,
        '',
        ...congratsBlock,
        `Here are some issues you might want to explore next:`,
        '',
        ...list,
        '',
        `Happy coding! 🚀`,
    ].join('\n');
}

/**
 * Builds an error comment when recommendations fail.
 *
 * @param {string} username
 * @returns {string}
 */
function buildRecommendationErrorComment(username) {
    return [
        `👋 Hi @${username}!`,
        '',
        `I ran into an issue while generating recommendations for you.`,
        '',
        `${MAINTAINER_TEAM} — could you please take a look?`,
        '',
        `Sorry for the inconvenience — feel free to explore open issues in the meantime!`,
    ].join('\n');
}

/**
 * Checks whether a contributor qualifies for a level via bypass.
 *
 * A contributor passes the bypass check if they have at least one closed
 * issue at the candidate level or any higher level. This indicates prior
 * experience at that difficulty, so prerequisite counts are not required.
 *
 * Iterates through all labels at or above the candidate level and stops
 * as soon as a qualifying issue is found.
 *
 * API failures are treated as non-qualifying and skipped.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {string} username
 * @param {string} candidate - Skill level being evaluated
 * @returns {Promise<boolean>} True if bypass condition is satisfied
 */
async function passesBypassCheck(github, owner, repo, username, candidate) {
    const candidateIndex = SKILL_HIERARCHY.indexOf(candidate);
    const atOrAboveLabels = SKILL_HIERARCHY.slice(candidateIndex);

    for (const label of atOrAboveLabels) {
        const count = await countIssuesByAssignee(
            github, owner, repo, username,
            ISSUE_STATE.CLOSED,
            label,
            1,
        );
        // Skip on API failure
        if (count === null) continue;

        if (count >= 1) return true;
    }
    return false;
}

/**
 * Checks whether a contributor qualifies for a level via prerequisite count.
 *
 * A contributor passes the normal check if they have completed at least
 * `requiredCount` issues at the prerequisite level.
 *
 * The threshold is passed to countIssuesByAssignee so the query can stop
 * early once the requirement is satisfied.
 *
 * @param {object} github
 * @param {string} owner
 * @param {string} repo
 * @param {string} username
 * @param {{ requiredLabel: string, requiredCount: number }} prereq
 * @returns {Promise<boolean|null>}
 *   - true  → requirement met
 *   - false → requirement not met
 *   - null  → API failure
 */
async function passesNormalCheck(github, owner, repo, username, prereq) {
    const count = await countIssuesByAssignee(
        github, owner, repo, username,
        ISSUE_STATE.CLOSED,
        prereq.requiredLabel,
        prereq.requiredCount,
    );

    if (count === null) return null;

    return count >= prereq.requiredCount;
}

/**
 * Determines the highest skill level a contributor is eligible for
 * based on their completed issues.
 *
 * The hierarchy is evaluated from highest to lowest level, returning
 * the first level that satisfies either of the following:
 *
 *   - Bypass check:
 *       The contributor has at least one closed issue at the candidate
 *       level or any higher level. This indicates prior experience, so
 *       prerequisite counts are not required.
 *
 *   - Normal check:
 *       The contributor has completed at least `requiredCount` issues
 *       at the prerequisite level defined in SKILL_PREREQUISITES.
 *
 * The two checks are delegated to helper functions:
 *   - passesBypassCheck
 *   - passesNormalCheck
 *
 * Levels without a prerequisite label (e.g. Good First Issue) act as
 * the floor and are always considered eligible.
 *
 * If an API call fails during evaluation, that candidate level is skipped
 * and the next lower level is considered.
 *
 * @param {object} botContext
 * @param {string} username
 * @returns {Promise<string>} The highest eligible skill level label
 */
async function resolveEligibleLevel(botContext, username) {
    const { github, owner, repo } = botContext;
    const topDown = [...SKILL_HIERARCHY].reverse();

    for (const candidate of topDown) {
        const prereq = SKILL_PREREQUISITES[candidate];

        if (!prereq) continue;

        // Floor level
        if (!prereq.requiredLabel) return candidate;

        // Bypass check
        const bypass = await passesBypassCheck(
            github, owner, repo, username, candidate
        );
        if (bypass) return candidate;

        // Normal check
        const normal = await passesNormalCheck(
            github, owner, repo, username, prereq
        );

        // Skip candidate if API failed
        if (normal === null) continue;

        if (normal) return candidate;
    }

    return LABELS.GOOD_FIRST_ISSUE;
}

/**
 * Checks whether the just-completed issue caused the contributor to reach
 * the threshold required to unlock the level directly above `currentLevel`.
 *
 * The count is evaluated using a capped query (threshold = requiredCount + 1),
 * allowing the function to distinguish between:
 *   - reaching the threshold exactly for the first time
 *   - having already exceeded it earlier
 *
 * Only the immediate next level is considered.
 *
 * @param {object} botContext
 * @param {string} username
 * @param {string} currentLevel
 * @returns {Promise<string|null>}
 */
async function detectUnlockedLevel(botContext, username, currentLevel) {
    const { github, owner, repo } = botContext;
    const currentIndex = SKILL_HIERARCHY.indexOf(currentLevel);
    const immediateNextLevel = SKILL_HIERARCHY[currentIndex + 1] ?? null;

    if (!immediateNextLevel) return null;

    const nextPrereq = SKILL_PREREQUISITES[immediateNextLevel];

    // The next level's prerequisite must be the current level for this
    // completion to be the relevant crossing event.
    if (!nextPrereq?.requiredLabel || nextPrereq.requiredLabel !== currentLevel) return null;

    const count = await countIssuesByAssignee(
        github, owner, repo, username,
        ISSUE_STATE.CLOSED,
        currentLevel,
        nextPrereq.requiredCount + 1,
    );

    if (count === null) {
        logger.log('detectUnlockedLevel: countIssuesByAssignee failed', {
            user: username, currentLevel,
        });
        return null;
    }

    if (count === nextPrereq.requiredCount) {
        logger.log('detectUnlockedLevel: threshold crossed', {
            user: username, unlockedLevel: immediateNextLevel, count,
        });
        return immediateNextLevel;
    }

    return null;
}

/**
 * Sorts issues based on the PRIORITY_HIERARCHY.
 * If two issues share the same priority, the oldest (earlier `created_at`)
 * comes first; issues without a recognized priority label fall to the end.
 *
 * @param {Array<object>} issues
 * @returns {Array<object>} Sorted copy of the issues.
 */
function sortByPriority(issues) {
    return [...issues].sort((a, b) => {
        const ai = PRIORITY_HIERARCHY.findIndex(p => hasLabel(a, p));
        const bi = PRIORITY_HIERARCHY.findIndex(p => hasLabel(b, p));
        const aRank = ai === -1 ? PRIORITY_HIERARCHY.length : ai;
        const bRank = bi === -1 ? PRIORITY_HIERARCHY.length : bi;
        if (aRank !== bRank) {
            return aRank - bRank;
        }
        // Tiebreaker: Use the creation date (Oldest first)
        return new Date(a.created_at) - new Date(b.created_at);
    });
}

/**
 * Returns recommended issues for the contributor based on their true
 * eligibility level, determined by a history-based top-down walk.
 *
 * Issues are fetched in a single batch and filtered locally.
 * Recommendations are restricted to the contributor's resolved eligible level,
 * so all suggested issues are immediately assignable.
 *
 * Also runs a focused threshold check (detectUnlockedLevel) to determine
 * whether this specific completion crossed a new level boundary, so the caller
 * can include a congratulatory message when appropriate.
 *
 * @param {object} botContext
 * @param {string} username
 * @param {string} currentLevel - Skill label from the triggering PR/issue.
 * @returns {Promise<{ issues: Array<{ title: string, html_url: string }>, unlockedLevel: string|null } | null>}
 *   Returns null on API failure (error comment already posted).
 */
async function getRecommendedIssues(botContext, username, currentLevel) {
    // Run both async operations concurrently — they're independent.
    const [eligibleLevel, unlockedLevel] = await Promise.all([
        resolveEligibleLevel(botContext, username),
        detectUnlockedLevel(botContext, username, currentLevel),
    ]);

    logger.log('getRecommendedIssues: resolved', {
        user: username,
        currentLevel,
        eligibleLevel,
        unlockedLevel,
    });

    const issues = await fetchIssuesBatch(
        botContext.github,
        botContext.owner,
        botContext.repo,
    );

    if (issues === null) {
        await postComment(
            botContext,
            buildRecommendationErrorComment(username)
        );
        return null;
    }
    // Sort issues by priority.
    const sorted = sortByPriority(issues);
    // Filter issues to the resolved eligible level so all results are actionable.
    const grouped = groupIssuesByLevel(sorted, [eligibleLevel]);
    return {
        issues: pickFirstAvailableLevel(grouped, [eligibleLevel]),
        unlockedLevel,
    };
}

/**
 * Main handler for issue recommendations after a PR is merged.
 *
 * - Determines skill level
 * - Fetches recommended issues
 * - Posts a comment if results exist, with a congratulatory prefix when the
 *   just-completed issue crossed a new level threshold
 *
 * Skips silently if context is incomplete or no results found.
 * Returns early on API failure.
 *
 * @param {{
 *   github: object,
 *   owner: string,
 *   repo: string,
 *   issue: object,
 *   sender: { login: string }
 * }} botContext
 * @returns {Promise<void>}
 */
async function handleRecommendIssues(botContext) {
    const username = botContext.sender?.login;
    if (!username) {
        logger.log('Missing sender login, skipping recommendation');
        return;
    }

    if (!botContext.issue) {
        logger.log('Missing issue in context, skipping recommendation');
        return;
    }

    const skillLevel = getHighestIssueSkillLevel(botContext.issue);
    if (!skillLevel) {
        logger.log('No skill level found, skipping recommendation', {
            issueNumber: botContext.issue?.number,
        });
        return;
    }

    logger.log('recommendation.context', {
        user: username,
        level: skillLevel,
        issue: botContext.issue?.number,
    });

    const result = await getRecommendedIssues(
        botContext,
        username,
        skillLevel,
    );

    if (result === null) return;

    const { issues, unlockedLevel } = result;

    if (issues.length === 0) {
        logger.log('recommendation.empty', { user: username });
        return;
    }

    const comment = buildRecommendationComment(username, issues, unlockedLevel);
    logger.log('recommendation.postComment', {
        target: botContext.number,
        issueSource: botContext.issue?.number,
        recommendations: issues.length,
        unlockedLevel,
    });
    const postResult = await postComment(botContext, comment);

    if (!postResult.success) {
        logger.error('recommendation.postCommentFailed', {
            error: postResult.error,
        });
        return;
    }

    logger.log('recommendation.posted');
}

module.exports = { 
    handleRecommendIssues,
    getRecommendedIssues,
    resolveEligibleLevel,
    detectUnlockedLevel,
};
