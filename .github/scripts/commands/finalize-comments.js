// SPDX-License-Identifier: Apache-2.0
//
// commands/finalize-comments.js
//
// Comment builders and per-skill-level boilerplate for the /finalize command.
// Pure formatting functions separated from finalize logic for readability and testability.

const { PROJECT_NAME, MAINTAINER_TEAM, LABELS } = require('../helpers');

// =============================================================================
// SKILL-LEVEL BOILERPLATE BLOCKS
// =============================================================================
// Each entry mirrors the intro textarea and [!IMPORTANT] markdown block from
// the original skill-level issue YAML templates. Used by /finalize to prepend
// the appropriate skill context to the issue body.

/**
 * Per-skill-level boilerplate for body reconstruction.
 * - introLabel: the H3 header label for the intro section
 * - introContent: the body of the intro textarea
 * - importantBlock: the [!IMPORTANT] GitHub callout markdown
 *
 * @type {Object<string, { introLabel: string, introContent: string, importantBlock: string }>}
 */
const SKILL_BOILERPLATE = {
  [LABELS.GOOD_FIRST_ISSUE]: {
    introLabel: 'First-Time Friendly',
    introContent: [
      `This issue is especially welcoming for people who are new to contributing to the ${PROJECT_NAME}.`,
      '',
      'We know that opening your first pull request can feel like a big step. Issues labeled **Good First Issue** are designed to make that experience easier, clearer, and more comfortable.',
      '',
      'No prior knowledge of Fabric Smart Client or distributed ledger technology is required - just a basic familiarity with go and Git is more than enough to get started.',
    ].join('\n'),
    importantBlock: [
      '> [!IMPORTANT]',
      '> ### About Good First Issues',
      '>',
      '> Good First Issues are designed to make getting started as smooth and stress-free as possible.',
      '>',
      '> They usually focus on:',
      '> - Small, clearly scoped changes  ',
      '> - Straightforward updates to existing code or docs  ',
      '> - Simple refactors or clarity improvements  ',
      '>',
      '> Other kinds of contributions — like larger features, deeper technical changes, or design-focused work — are just as valuable and often use the beginner, intermediate, or advanced labels.',
    ].join('\n'),
  },

  [LABELS.BEGINNER]: {
    introLabel: '🐥 Beginner Friendly',
    introContent: [
      `This issue is a great fit for contributors who are ready to explore the ${PROJECT_NAME} codebase a little more and take on slightly more independent work.`,
      '',
      `Beginner Issues often involve reading existing code, understanding how different parts of the ${PROJECT_NAME} fit together, and making small, thoughtful updates that follow established patterns.`,
      '',
      'The goal is to support skill growth while keeping the experience approachable, well-scoped, and enjoyable.',
    ].join('\n'),
    importantBlock: [
      '> [!IMPORTANT]',
      '> ### About Beginner Issues',
      '>',
      '> Beginner Issues are a great next step for contributors who feel comfortable with the basic project workflow and want to explore the codebase a little more.',
      '>',
      '> These issues often involve:',
      '> - Reading existing code  ',
      `> - Understanding how different parts of the ${PROJECT_NAME} fit together  `,
      '> - Making small, thoughtful updates that follow established patterns  ',
      '>',
      '> You\'ll usually see Beginner Issues focused on things like:',
      '> - Small, well-scoped improvements to existing tests  ',
      '> - Narrow updates to `src` functionality (e.g. refining helpers or improving readability)  ',
      '> - Documentation or comment clarity  ',
      '> - Enhancements to existing examples  ',
      '>',
      '> Other types of contributions — such as brand-new features, broader system changes, or deeper technical work — are just as valuable and may use different labels.',
    ].join('\n'),
  },

  [LABELS.INTERMEDIATE]: {
    introLabel: 'Intermediate Friendly',
    introContent: [
      `This issue is a good fit for contributors who are already familiar with the ${PROJECT_NAME} and feel comfortable navigating the codebase.`,
      '',
      'Intermediate Issues often involve:',
      '- Exploring existing implementations  ',
      '- Understanding how different components work together  ',
      '- Making thoughtful changes that follow established patterns  ',
      '',
      'The goal is to support deeper problem-solving while keeping the task clear, focused, and enjoyable to work on.',
    ].join('\n'),
    importantBlock: [
      '> [!IMPORTANT]',
      '> ### About Intermediate Issues',
      '>',
      '> Intermediate Issues are a great next step for contributors who enjoy digging into the codebase and reasoning about how things work.',
      '>',
      '> These issues often:',
      '> - Involve multiple related files or components  ',
      '> - Encourage investigation and understanding of existing behavior  ',
      '> - Leave room for thoughtful implementation choices  ',
      '> - Stay focused on a clearly defined goal  ',
      '>',
      '> Other kinds of contributions — from beginner-friendly tasks to large system-level changes — are just as valuable and use different labels.',
    ].join('\n'),
  },

  [LABELS.ADVANCED]: {
    introLabel: '🧠 Advanced',
    introContent: [
      `This issue is well-suited for contributors who are very familiar with the ${PROJECT_NAME} and enjoy working with its core abstractions and design patterns.`,
      '',
      'Advanced Issues often involve:',
      `- Exploring and shaping ${PROJECT_NAME} architecture  `,
      '- Reasoning about trade-offs and long-term impact  ',
      '- Working across multiple modules or systems  ',
      '- Updating tests, examples, and documentation alongside code  ',
      '',
      'The goal is to support thoughtful, high-impact contributions in a clear and collaborative way.',
    ].join('\n'),
    importantBlock: [
      '> [!IMPORTANT]',
      '> ### 🧭 About Advanced Issues',
      '>',
      `> Advanced Issues usually focus on larger changes that influence how the ${PROJECT_NAME} works as a whole.`,
      '>',
      '> These issues often:',
      '> - Touch core abstractions or shared utilities  ',
      '> - Span multiple parts of the codebase  ',
      '> - Involve design decisions and trade-offs  ',
      '> - Consider long-term maintainability and compatibility  ',
      '>',
      '> Smaller fixes, focused refactors, and onboarding-friendly tasks are just as valuable and often use different labels.',
    ].join('\n'),
  },
};

// =============================================================================
// CONTRIBUTION GUIDE BOILERPLATE
// =============================================================================

/**
 * Standard contribution guide section appended to all finalized issue bodies.
 * Mirrors the "Step-by-Step Contribution Guide" textarea from the skill-level templates.
 */
const CONTRIBUTION_GUIDE_LABEL = 'Step-by-Step Contribution Guide';

const CONTRIBUTION_GUIDE_CONTENT = [
  'To help keep contributions consistent and easy to review, we recommend following these steps:',
  '',
  '- [ ] Comment `/assign` to request the issue',
  '- [ ] Wait for assignment',
  '- [ ] Fork the repository and create a branch',
  '- [ ] Set up the project using the instructions in `README.md`',
  '- [ ] Make the requested changes',
  '- [ ] Sign each commit using `-s -S`',
  '- [ ] Push your branch and open a pull request',
  '',
  'Read [Workflow Guide](https://github.com/hyperledger-labs/fabric-smart-client/blob/main/docs/dev/workflow.md) for step-by-step workflow guidance.',
  'Read [docs/dev/development.md](https://github.com/hyperledger-labs/fabric-smart-client/blob/main/docs/dev/development.md) for setup instructions.',
  '',
  '❗ Pull requests **cannot be merged** without `S` and `s` signed commits. See the [Signing Guide](https://github.com/hyperledger-labs/fabric-smart-client/blob/main/docs/dev/signing.md).',
].join('\n');

// =============================================================================
// DEFAULT ADDITIONAL INFORMATION CONTENT
// =============================================================================

/**
 * Default additional information content used when the submitter left the field
 * empty or with the default placeholder ("Optional.").
 */
const DEFAULT_ADDITIONAL_INFO_LABEL = 'Additional Information';

const DEFAULT_ADDITIONAL_INFO_CONTENT = [
  'If you have questions while working on this issue, feel free to ask!',
  '',
  `You can reach the community and maintainers here: LFDT Discord [#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012)`,
  '',
  'Whether you need help finding the right file, understanding existing code, or confirming your approach — we\'re happy to help.',
].join('\n');

// =============================================================================
// BODY PARSING & RECONSTRUCTION
// =============================================================================

/**
 * Parses an issue body into sections by splitting on H3 (###) markdown headers.
 * GitHub form templates render each field's `label` as a `### ` header in the
 * submitted issue body, making this a reliable split point.
 *
 * @param {string} body - The raw issue body string.
 * @returns {Array<{ header: string|null, content: string }>} Ordered section list.
 *   The first entry may have `header: null` if content appears before the first header.
 */
function parseSections(body) {
  if (!body || typeof body !== 'string') return [];

  const lines = body.split('\n');
  const sections = [];
  let currentHeader = null;
  let currentLines = [];

  function flush() {
    if (currentHeader !== null || currentLines.some((l) => l.trim())) {
      sections.push({ header: currentHeader, content: currentLines.join('\n').trim() });
    }
  }

  for (const line of lines) {
    const match = line.match(/^### (.+)$/);
    if (match) {
      flush();
      currentHeader = match[1].trim();
      currentLines = [];
    } else {
      currentLines.push(line);
    }
  }

  flush();
  return sections;
}

/**
 * Determines whether the content of a section is non-trivial (i.e., the user
 * actually filled it in rather than leaving the default placeholder).
 *
 * @param {string|undefined} content - The section content string.
 * @returns {boolean} True if the content is meaningful.
 */
function isMeaningfulContent(content) {
  if (!content || typeof content !== 'string') return false;
  const trimmed = content.trim();
  return trimmed.length > 0 && trimmed !== 'Optional.' && trimmed !== '_No response_';
}

/**
 * Reconstructs the issue body in the skill-level template format. The existing
 * body is parsed into sections; skill-level boilerplate is prepended; the
 * Contribution Guide boilerplate is appended; Additional Information is moved
 * to the very end.
 *
 * @param {string} existingBody - The current issue body (from the submitted template).
 * @param {string} skillLevel - A LABELS skill-level constant (e.g. LABELS.BEGINNER).
 * @returns {string} The fully reconstructed issue body.
 */
function reconstructBody(existingBody, skillLevel) {
  const boilerplate = SKILL_BOILERPLATE[skillLevel];
  const sections = parseSections(existingBody);

  // Extract "Additional Information" section so it can be placed last
  const additionalInfoIdx = sections.findIndex(
    (s) => s.header && s.header.toLowerCase().replace(/[^\w\s]/g, '').trim().includes('additional information')
  );
  let additionalInfoContent = null;
  if (additionalInfoIdx !== -1) {
    const [aiSection] = sections.splice(additionalInfoIdx, 1);
    if (isMeaningfulContent(aiSection.content)) {
      additionalInfoContent = aiSection.content;
    }
  }

  const parts = [];

  // 1. Skill-level intro block
  parts.push(`### ${boilerplate.introLabel}`);
  parts.push('');
  parts.push(boilerplate.introContent);
  parts.push('');
  parts.push(boilerplate.importantBlock);
  parts.push('');

  // 2. User's sections in original order (skip null-header leading content)
  for (const section of sections) {
    if (!section.header) continue;
    parts.push(`### ${section.header}`);
    if (section.content) {
      parts.push('');
      parts.push(section.content);
    }
    parts.push('');
  }

  // 3. Contribution guide boilerplate
  parts.push('---');
  parts.push('');
  parts.push(`### ${CONTRIBUTION_GUIDE_LABEL}`);
  parts.push('');
  parts.push(CONTRIBUTION_GUIDE_CONTENT);
  parts.push('');

  // 4. Additional information (user-provided or default)
  parts.push(`### ${DEFAULT_ADDITIONAL_INFO_LABEL}`);
  parts.push('');
  parts.push(additionalInfoContent !== null ? additionalInfoContent : DEFAULT_ADDITIONAL_INFO_CONTENT);

  return parts.join('\n').trimEnd();
}

// =============================================================================
// COMMENT BUILDERS
// =============================================================================

/**
 * Builds the comment posted when the commenter does not have triage-or-above
 * permissions. Lists the required permission level.
 *
 * @param {string} username - The GitHub username who commented /finalize.
 * @returns {string} The formatted Markdown comment body.
 */
function buildUnauthorizedComment(username) {
  return [
    `👋 Hi @${username}! The \`/finalize\` command is reserved for maintainers and contributors with **triage** (or higher) repository permissions.`,
    '',
    'If you believe you should have access, please reach out to a maintainer.',
  ].join('\n');
}

/**
 * Builds the comment posted when one or more label validation rules are violated.
 * Lists every violation found so the reviewer can fix them all in one pass.
 *
 * @param {string} username - The GitHub username who commented /finalize.
 * @param {string[]} errors - Array of human-readable error strings (one per violation).
 * @returns {string} The formatted Markdown comment body.
 */
function buildValidationErrorComment(username, errors) {
  const errorList = errors.map((e) => `- ${e}`).join('\n');
  return [
    `👋 Hi @${username}! The issue isn't quite ready to finalize yet. Please fix the following labeling issue(s) and then comment \`/finalize\` again:`,
    '',
    errorList,
    '',
    'If you have questions about which labels to apply, see the maintainer documentation or ask in the team channel.',
  ].join('\n');
}

/**
 * Builds the comment posted when the GitHub API call to update the issue
 * (title or body) fails. Tags the maintainer team for manual intervention.
 *
 * @param {string} username - The GitHub username who commented /finalize.
 * @param {string} error - The error message from the failed API call.
 * @returns {string} The formatted Markdown comment body.
 */
function buildUpdateFailureComment(username, error) {
  return [
    `⚠️ Hi @${username}! I encountered an error while trying to update the issue title or body.`,
    '',
    `${MAINTAINER_TEAM} — could you please complete the finalization manually?`,
    '',
    `Error details: ${error}`,
  ].join('\n');
}

/**
 * Builds the comment posted when the label swap after a successful update
 * fails. Tags the maintainer team with explicit manual steps.
 *
 * @param {string} username - The GitHub username who commented /finalize.
 * @param {string} error - The error message(s) from the failed label operations.
 * @returns {string} The formatted Markdown comment body.
 */
function buildLabelSwapFailureComment(username, error) {
  return [
    `⚠️ The issue was updated successfully, but I encountered an error swapping the status labels.`,
    '',
    `${MAINTAINER_TEAM} — please manually:`,
    `- Remove the \`${LABELS.AWAITING_TRIAGE}\` label`,
    `- Add the \`${LABELS.READY_FOR_DEV}\` label`,
    '',
    `Error details: ${error}`,
  ].join('\n');
}

/**
 * Builds the comment posted when the permission check API call itself fails.
 * Tags the maintainer team for manual assistance.
 *
 * @param {string} username - The GitHub username who commented /finalize.
 * @returns {string} The formatted Markdown comment body.
 */
function buildPermissionCheckErrorComment(username) {
  return [
    `👋 Hi @${username}! I encountered an error while trying to verify your permissions.`,
    '',
    `${MAINTAINER_TEAM} — could you please verify @${username}'s permissions and complete the finalization manually if appropriate?`,
    '',
    'Sorry for the inconvenience!',
  ].join('\n');
}

/**
 * Builds the success comment posted after a successful /finalize run.
 *
 * @param {string} username - The GitHub username who ran /finalize.
 * @param {string} skillLevel - The skill-level label that was applied (a LABELS constant).
 * @param {string} priorityLabel - The priority label on the issue (e.g. 'priority: medium').
 * @returns {string} The formatted Markdown comment body.
 */
function buildSuccessComment(username, skillLevel, priorityLabel) {
  return [
    `✅ Issue finalized by @${username}!`,
    '',
    `**Skill level:** \`${skillLevel}\``,
    `**Priority:** \`${priorityLabel}\``,
    '',
    'The issue body has been updated with the appropriate skill-level context and contribution guide. This issue is now ready for contributors to pick up via `/assign`.',
  ].join('\n');
}

module.exports = {
  CONTRIBUTION_GUIDE_LABEL,
  CONTRIBUTION_GUIDE_CONTENT,
  DEFAULT_ADDITIONAL_INFO_LABEL,
  DEFAULT_ADDITIONAL_INFO_CONTENT,
  parseSections,
  isMeaningfulContent,
  reconstructBody,
  buildUnauthorizedComment,
  buildValidationErrorComment,
  buildUpdateFailureComment,
  buildLabelSwapFailureComment,
  buildPermissionCheckErrorComment,
  buildSuccessComment,
};
