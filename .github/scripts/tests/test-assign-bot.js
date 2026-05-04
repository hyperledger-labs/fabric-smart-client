// SPDX-License-Identifier: Apache-2.0
//
// tests/test-assign-bot.js
//
// Local test script for bot-on-comment.js
// Run with: node .github/scripts/tests/test-assign-bot.js
//
// This script mocks the GitHub API and runs various test scenarios
// to verify the on-comment (assign) bot behaves correctly without making real API calls.

const { LABELS, PROJECT_NAME, MAINTAINER_TEAM } = require("../helpers");
const script = require("../bot-on-comment.js");

// =============================================================================
// MOCK GITHUB API
// =============================================================================

function createMockGithub(options = {}, issueFromPayload = null) {
  const {
    completedIssueCount = 0,
    completedIssueCounts = {},
    openAssignmentCount = 0,
    openAssignmentCountExcludingBlocked = openAssignmentCount,
    blockedIssueCount = 0,
    restListClosedShouldFail = false,
    restListOpenShouldFail = false,
    assignShouldFail = false,
    removeLabelShouldFail = false,
    addLabelShouldFail = false,
    reactionShouldFail = false,
    issueGetShouldFail = false,
    issueAssignees = null,
    issueLabels = null,
    openAssignedIssues = null,
    searchResults = null,
    searchShouldFail = false,
    restListOpenFailOnCall = null,
  } = options;

  let openListForRepoCallCount = 0;

  const calls = {
    comments: [],
    assignees: [],
    labelsAdded: [],
    labelsRemoved: [],
    restCalls: [],
    reactions: [],
  };

  return {
    calls,
    rest: {
      reactions: {
        createForIssueComment: async (params) => {
          if (reactionShouldFail) {
            throw new Error(
              "Simulated reaction failure: comment not found (404)",
            );
          }
          calls.reactions.push({
            commentId: params.comment_id,
            content: params.content,
          });
          console.log(`\n👍 REACTION ADDED: ${params.content}`);
        },
      },
      issues: {
        createComment: async (params) => {
          calls.comments.push(params.body);
          console.log("\n📝 COMMENT POSTED:");
          console.log("─".repeat(60));
          console.log(params.body);
          console.log("─".repeat(60));
        },
        addAssignees: async (params) => {
          if (assignShouldFail) {
            throw new Error("Simulated assignment failure");
          }
          calls.assignees.push(...params.assignees);
          console.log(`\n✅ ASSIGNED: ${params.assignees.join(", ")}`);
        },
        addLabels: async (params) => {
          if (addLabelShouldFail) {
            throw new Error("Simulated add label failure");
          }
          calls.labelsAdded.push(...params.labels);
          console.log(`\n🏷️  LABEL ADDED: ${params.labels.join(", ")}`);
        },
        removeLabel: async (params) => {
          if (removeLabelShouldFail) {
            throw new Error("Simulated remove label failure");
          }
          calls.labelsRemoved.push(params.name);
          console.log(`\n🏷️  LABEL REMOVED: ${params.name}`);
        },
        get: async (params) => {
          console.log(
            `\n🔍 REST API CALL: get issue_number=${params.issue_number}`,
          );
          if (issueGetShouldFail) {
            throw new Error("Simulated issues.get failure");
          }
          const freshLabels = Array.isArray(issueLabels)
            ? issueLabels
            : Array.isArray(issueFromPayload?.labels)
              ? issueFromPayload.labels
              : [];
          if (Array.isArray(issueAssignees)) {
            return {
              data: {
                assignees: issueAssignees.map((login) => ({ login })),
                labels: freshLabels,
              },
            };
          }
          if (options.issueAlreadyAssignedTo) {
            return {
              data: {
                assignees: [{ login: options.issueAlreadyAssignedTo }],
                labels: freshLabels,
              },
            };
          }
          return { data: { assignees: [], labels: freshLabels } };
        },
        listForRepo: async (params) => {
          calls.restCalls.push(
            `REST listForRepo: state=${params.state} assignee=${params.assignee}`,
          );
          console.log(
            `\n🔍 REST API CALL: listForRepo state=${params.state} assignee=${params.assignee}`,
          );

          if (params.state === "open") {
            openListForRepoCallCount++;
            if (restListOpenShouldFail) {
              throw new Error(
                "Simulated REST API failure for open assignments",
              );
            }
            if (restListOpenFailOnCall === openListForRepoCallCount) {
              throw new Error(
                "Simulated REST API failure for open assignments",
              );
            }
            if (openAssignedIssues) {
              if (params.labels) {
                return {
                  data: openAssignedIssues.filter((issue) =>
                    issue.labels?.some((l) => l.name === params.labels),
                  ),
                };
              }
              return { data: openAssignedIssues };
            }
            const effectiveCount = openAssignmentCountExcludingBlocked;
            const issues = [];
            for (let i = 0; i < effectiveCount; i++) {
              issues.push({
                number: 9000 + i,
                labels: [{ name: "status: ready for dev" }],
              });
            }
            const blockedToGenerate = Math.max(
              blockedIssueCount,
              openAssignmentCount - openAssignmentCountExcludingBlocked,
            );
            for (let i = 0; i < blockedToGenerate; i++) {
              issues.push({
                number: 9100 + i,
                labels: [{ name: LABELS.BLOCKED }],
              });
            }
            const difference =
              openAssignmentCount -
              (openAssignmentCountExcludingBlocked + blockedToGenerate);
            if (difference > 0) {
              for (let i = 0; i < difference; i++) {
                issues.push({ number: 9200 + i, labels: [] });
              }
            }
            if (params.labels) {
              return {
                data: issues.filter((issue) =>
                  issue.labels?.some((l) => l.name === params.labels),
                ),
              };
            }
            return { data: issues };
          }

          if (params.state === "closed") {
            if (restListClosedShouldFail) {
              throw new Error("Simulated REST API failure");
            }
            const issues = [];
            for (const [labelName, count] of Object.entries(
              completedIssueCounts,
            )) {
              for (let i = 0; i < count; i++) {
                issues.push({ labels: [{ name: labelName }] });
              }
            }
            for (let i = 0; i < completedIssueCount; i++) {
              issues.push({ labels: [] });
            }
            if (params.labels) {
              return {
                data: issues.filter((issue) =>
                  issue.labels?.some((l) => l.name === params.labels),
                ),
              };
            }
            return { data: issues };
          }

          return { data: [] };
        },
      },
      search: {
        issuesAndPullRequests: async (params) => {
          console.log(`\n🔍 SEARCH API CALL: q=${params.q}`);
          if (searchShouldFail) {
            throw new Error("Simulated search API failure");
          }
          const linkedMatch = params.q.match(/linked:(\d+)/);
          const issueNumber = linkedMatch
            ? parseInt(linkedMatch[1], 10)
            : null;
          if (
            searchResults &&
            issueNumber !== null &&
            issueNumber in searchResults
          ) {
            return { data: searchResults[issueNumber] };
          }
          return { data: { total_count: 0 } };
        },
      },
    },
    graphql: async (query, vars) => {
      // Stubbed just in case other things call it, though we rely on REST now
      if (searchShouldFail) {
        throw new Error("Simulated GraphQL failure");
      }
      
      if (query.includes("closedByPullRequestsReferences")) {
        const issueNumber = vars.number;
        const totalCount = (searchResults && issueNumber !== null && issueNumber in searchResults)
          ? searchResults[issueNumber].total_count
          : 0;

        let nodes = [];
        if (totalCount > 0) {
          // In tests, if totalCount > 0, we just return a matching PR
          // The hasNeedsReviewPR function checks author matching `username`,
          // but we don't have the explicit username passed into the mock. 
          // So we use a special placeholder, or infer from context.
          // Since the test just wants a match, we can return the author as options.mockUsername
          // or fallback to 'bypass-review-user-1'.
          const authorLogin = options.mockUsername || "bypass-review-user-1";
          nodes.push({
            state: "OPEN",
            author: { login: authorLogin },
            labels: { nodes: [{ name: "status: needs review" }] }
          });
        }

        return {
          repository: {
            issue: {
              closedByPullRequestsReferences: {
                nodes
              }
            }
          }
        };
      }
      
      return { search: { issueCount: 0 } };
    },
  };
}

// =============================================================================
// TEST SCENARIOS
// =============================================================================

const scenarios = [
  {
    name: "Race Condition - Issue Snatched While Queued (Case 2)",
    description:
      "Fresh fetch shows another user was assigned between queue and execution",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 120,
          assignees: [], // stale payload: appears unassigned
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1021,
          body: "/assign",
          user: { login: "second-requester", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueAlreadyAssignedTo: "first-requester" },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @second-requester! This issue is already assigned to @first-requester.\n\n👉 **Find another issue to work on:**\n[Browse unassigned issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)\n\nOnce you find one you like, comment \`/assign\` to get started!`,
    ],
  },
  {
    name: "Race Condition - Requester Already Assigned In Fresh State",
    description:
      "Fresh fetch shows the requester is already assigned; handler must exit without addAssignees",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 122,
          assignees: [], // stale payload: appears unassigned
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1023,
          body: "/assign",
          user: { login: "already-assigned-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueAlreadyAssignedTo: "already-assigned-user" },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @already-assigned-user! You're already assigned to this issue. You're all set to start working on it!\n\nIf you have any questions, feel free to ask here or reach out to the team.`,
    ],
  },
  {
    name: "Race Condition - Fresh Issue Has Multiple Assignees",
    description:
      "Fresh fetch reveals a corrupted multi-assignee issue; bot must reject and list all assignees",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 123,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1024,
          body: "/assign",
          user: { login: "third-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueAssignees: ["first-user", "second-user"] },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @third-user! This issue is already assigned to @first-user, @second-user.\n\n👉 **Find another issue to work on:**\n[Browse unassigned issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)\n\nOnce you find one you like, comment \`/assign\` to get started!`,
    ],
  },
  {
    name: "Race Condition - Ready Label Removed While Queued",
    description:
      "Fresh fetch no longer has ready-for-dev label; bot must abort assignment",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 124,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1025,
          body: "/assign",
          user: { login: "stale-ready-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueLabels: [{ name: LABELS.GOOD_FIRST_ISSUE }] },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @stale-ready-user! This issue is not ready for development yet.\n\nIssues must have the \`status: ready for dev\` label before they can be assigned.\n\n👉 **Find an issue that's ready:**\n[Browse ready issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)\n\nOnce you find one you like, comment \`/assign\` to get started!`,
    ],
  },
  {
    name: "Race Condition - Skill Label Removed While Queued",
    description:
      "Fresh fetch no longer has any skill label; bot must abort and tag maintainers",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 125,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1026,
          body: "/assign",
          user: { login: "stale-skill-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueLabels: [{ name: LABELS.READY_FOR_DEV }] },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @stale-skill-user! This issue doesn't have a skill level label yet.\n\n${MAINTAINER_TEAM} — could you please add one of the following labels?\n- \`skill: good first issue\`\n- \`skill: beginner\`\n- \`skill: intermediate\`\n- \`skill: advanced\`\n\n@stale-skill-user, once a maintainer adds the label, comment \`/assign\` again to request assignment.`,
    ],
  },
  {
    name: "Race Condition - Skill Label Changed to Different Level While Queued",
    description:
      "Fresh fetch shows a different skill level; bot must abort since prerequisite gates were run against stale level",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 126,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1027,
          body: "/assign",
          user: { login: "skill-changed-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      issueLabels: [{ name: LABELS.READY_FOR_DEV }, { name: LABELS.BEGINNER }],
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @skill-changed-user! The skill level for this issue changed while your assignment request was being processed.\n\n**Current label:** \`${LABELS.BEGINNER}\`\n**Previous label:** \`${LABELS.GOOD_FIRST_ISSUE}\`\n\nPlease comment \`/assign\` again to request assignment with the updated skill requirements.`,
    ],
  },
  {
    name: "Error - Fresh Issue Fetch API Failure",
    // Note: This failure fires from inside assignAndFinalize(), unlike the
    // other API failure tests which catch errors during precondition checks.
    description:
      "Tags maintainers when issues.get fails inside assignAndFinalize",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 121,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1022,
          body: "/assign",
          user: { login: "unlucky-user-4", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { issueGetShouldFail: true },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @unlucky-user-4! I encountered an error while trying to verify your eligibility for this issue.\n\n${MAINTAINER_TEAM} — could you please help with this assignment request?\n\n@unlucky-user-4, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
    ],
  },
  // ---------------------------------------------------------------------------
  // HAPPY PATHS (4 tests)
  // Successful assignment for each skill level
  // ---------------------------------------------------------------------------

  {
    name: "Happy Path - Good First Issue",
    description: "New contributor successfully assigned to GFI",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 100,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1001,
          body: "/assign",
          user: { login: "new-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: "new-contributor",
    expectedComments: [
      `👋 Hi @new-contributor, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉

The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.

Good luck, and welcome aboard! 🚀`,
    ],
  },

  {
    name: "Happy Path - Beginner Issue",
    description: "Contributor with 2 completed GFIs assigned to Beginner",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 101,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 1002,
          body: "/assign",
          user: { login: "experienced-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.GOOD_FIRST_ISSUE]: 2 } },
    expectedAssignee: "experienced-contributor",
    expectedComments: [
      `👋 Hi @experienced-contributor, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Beginner** issue. 🙌

If this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.

Good luck! 🚀`,
    ],
  },

  {
    name: "Happy Path - Intermediate Issue",
    description:
      "Contributor with 3 completed Beginners assigned to Intermediate",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 102,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: intermediate" },
          ],
        },
        comment: {
          id: 1003,
          body: "/assign",
          user: { login: "growing-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.BEGINNER]: 3 } },
    expectedAssignee: "growing-contributor",
    expectedComments: [
      `👋 Hi @growing-contributor, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Intermediate** issue. 🙌

If this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.

Good luck! 🚀`,
    ],
  },

  {
    name: "Happy Path - Advanced Issue",
    description:
      "Contributor with 3 completed Intermediates assigned to Advanced",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 103,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: advanced" },
          ],
        },
        comment: {
          id: 1004,
          body: "/assign",
          user: { login: "senior-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.INTERMEDIATE]: 3 } },
    expectedAssignee: "senior-contributor",
    expectedComments: [
      `👋 Hi @senior-contributor, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Advanced** issue. 🙌

If this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.

Good luck! 🚀`,
    ],
  },

  // ---------------------------------------------------------------------------
  // BYPASS LOGIC TESTS
  // ---------------------------------------------------------------------------

  {
    name: "Bypass - Same Level Completed",
    description:
      "User with 1 Beginner can take another Beginner (bypasses 2 GFI prereq)",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 200,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 2001,
          body: "/assign",
          user: { login: "bypass-user-1", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      completedIssueCounts: {
        [LABELS.BEGINNER]: 1,
        [LABELS.GOOD_FIRST_ISSUE]: 1, // Only 1 (Not enough to pass normal prereq)
      },
    },
    expectedAssignee: "bypass-user-1",
    expectedComments: [
      `👋 Hi @bypass-user-1, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Beginner** issue. 🙌\n\nIf this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.\n\nIf you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.\n\nGood luck! 🚀`,
    ],
  },

  {
    name: "Bypass - Higher Level Completed",
    description:
      "User with 1 Intermediate can take a Beginner (bypasses 2 GFI prereq)",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 201,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 2002,
          body: "/assign",
          user: { login: "bypass-user-2", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      completedIssueCounts: {
        [LABELS.INTERMEDIATE]: 1,
      },
    },
    expectedAssignee: "bypass-user-2",
    expectedComments: [
      `👋 Hi @bypass-user-2, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Beginner** issue. 🙌\n\nIf this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.\n\nIf you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.\n\nGood luck! 🚀`,
    ],
  },

  // ---------------------------------------------------------------------------
  // GFI COMPLETION CAP TESTS (3 tests)
  // Gate added in enforceGfiCompletionLimit
  // ---------------------------------------------------------------------------

  {
    name: "GFI Cap - Exactly At Limit (5 Completed)",
    description:
      "Contributor with 5 completed GFIs is rejected with encouraging redirect",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 300,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 3001,
          body: "/assign",
          user: { login: "veteran-gfi-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.GOOD_FIRST_ISSUE]: 5 } },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @veteran-gfi-user! You've completed **5 Good First Issues** — that's a fantastic achievement, and it shows you know the workflow inside and out. 🎉

Good First Issues are designed to help new contributors get comfortable with the process, and you've clearly mastered it. We believe you're more than ready to take on bigger challenges!

👉 **Find Beginner and higher issues to work on:**
[Browse available Beginner issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20no%3Aassignee%20label%3A%22skill%3A%20beginner%22%20label%3A%22status%3A%20ready%20for%20dev%22)

Come take on something more challenging — we're excited to see what you'll build next! 🚀`,
    ],
  },

  {
    name: "GFI Cap - Below Limit (4 Completed)",
    description:
      "Contributor with 4 completed GFIs is still allowed to take another GFI",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 301,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 3002,
          body: "/assign",
          user: { login: "almost-capped-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.GOOD_FIRST_ISSUE]: 4 } },
    expectedAssignee: "almost-capped-user",
    expectedComments: [
      `👋 Hi @almost-capped-user, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉

The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.

Good luck, and welcome aboard! 🚀`,
    ],
  },

  {
    name: "GFI Cap - Does Not Apply to Beginner Issues",
    description:
      "Contributor with 5 completed GFIs can still take a Beginner issue",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 302,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 3003,
          body: "/assign",
          user: { login: "gfi-graduated-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCounts: { [LABELS.GOOD_FIRST_ISSUE]: 5 } },
    expectedAssignee: "gfi-graduated-user",
    expectedComments: [
      `👋 Hi @gfi-graduated-user, thanks for continuing to contribute to the ${PROJECT_NAME}! You've been assigned this **Beginner** issue. 🙌

If this task involves any design decisions or you'd like early feedback, feel free to share your plan here before diving into the code.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the pool.

Good luck! 🚀`,
    ],
  },

  // ---------------------------------------------------------------------------
  // VALIDATION FAILURES (9 tests)
  // Bot rejects assignment with helpful message
  // ---------------------------------------------------------------------------

  {
    name: "Validation - Already Assigned to Someone Else",
    description: "Issue is taken by another contributor",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 104,
          assignees: [{ login: "other-user" }],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1005,
          body: "/assign",
          user: { login: "late-arrival", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @late-arrival! This issue is already assigned to @other-user.

👉 **Find another issue to work on:**
[Browse unassigned issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)

Once you find one you like, comment \`/assign\` to get started!`,
    ],
  },

  {
    name: "Validation - Already Assigned to Self",
    description: "Contributor already owns the issue",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 105,
          assignees: [{ login: "forgetful-user" }],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1006,
          body: "/assign",
          user: { login: "forgetful-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @forgetful-user! You're already assigned to this issue. You're all set to start working on it!

If you have any questions, feel free to ask here or reach out to the team.`,
    ],
  },

  {
    name: "Validation - Not Ready for Dev",
    description: "Issue missing status: ready for dev label",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 106,
          assignees: [],
          labels: [{ name: "skill: good first issue" }],
        },
        comment: {
          id: 1007,
          body: "/assign",
          user: { login: "eager-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @eager-user! This issue is not ready for development yet.

Issues must have the \`status: ready for dev\` label before they can be assigned.

👉 **Find an issue that's ready:**
[Browse ready issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)

Once you find one you like, comment \`/assign\` to get started!`,
    ],
  },

  {
    name: "Validation - No Labels At All",
    description: "Issue has no labels",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 107,
          assignees: [],
          labels: [],
        },
        comment: {
          id: 1007,
          body: "/assign",
          user: { login: "eager-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @eager-user! This issue is not ready for development yet.

Issues must have the \`status: ready for dev\` label before they can be assigned.

👉 **Find an issue that's ready:**
[Browse ready issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue+is%3Aopen+no%3Aassignee+label%3A%22status%3A+ready+for+dev%22)

Once you find one you like, comment \`/assign\` to get started!`,
    ],
  },

  {
    name: "Validation - No Skill Level Label",
    description: "Issue missing skill level label",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 107,
          assignees: [],
          labels: [{ name: "status: ready for dev" }],
        },
        comment: {
          id: 1008,
          body: "/assign",
          user: { login: "confused-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @confused-user! This issue doesn't have a skill level label yet.

${MAINTAINER_TEAM} — could you please add one of the following labels?
- \`skill: good first issue\`
- \`skill: beginner\`
- \`skill: intermediate\`
- \`skill: advanced\`

@confused-user, once a maintainer adds the label, comment \`/assign\` again to request assignment.`,
    ],
  },

  {
    name: "Validation - Prerequisites Not Met",
    description: "Contributor lacks required experience",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 108,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 1009,
          body: "/assign",
          user: { login: "eager-newbie", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { completedIssueCount: 0, openAssignmentCount: 0 },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @eager-newbie! Thanks for your interest in contributing!

This is a **Beginner** issue. Before taking it on, you need to complete at least **2 Good First Issues** to build familiarity with the codebase.

📊 **Your Progress:** You've completed **0** so far.

👉 **Find Good First Issues to work on:**
[Browse available Good First Issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20no%3Aassignee%20label%3A%22skill%3A%20good%20first%20issue%22%20label%3A%22status%3A%20ready%20for%20dev%22)

Once you've completed 2, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Validation - Too Many Open Assignments (at limit)",
    description: "Contributor already has 2 open issues assigned",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 114,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1015,
          body: "/assign",
          user: { login: "busy-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { openAssignmentCount: 2 },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @busy-contributor! Thanks for your enthusiasm to contribute!

To help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.

📊 **Your Current Assignments:** You're currently assigned to **2** open issues.

👉 **View your assigned issues:**
[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Abusy-contributor%20-label%3A%22status%3A%20blocked%22)

💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.

Once you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Validation - Too Many Open Assignments (over limit)",
    description: "Contributor has more than 2 open issues assigned",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 115,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1016,
          body: "/assign",
          user: { login: "very-busy-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { openAssignmentCount: 5 },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @very-busy-contributor! Thanks for your enthusiasm to contribute!

To help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.

📊 **Your Current Assignments:** You're currently assigned to **3+** open issues.

👉 **View your assigned issues:**
[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Avery-busy-contributor%20-label%3A%22status%3A%20blocked%22)

💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.

Once you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Validation - Over Limit After Issues Unblocked",
    description:
      "User has 3 open issues; some were blocked when they got the third. Now 3 count (excluding blocked), so over limit and cannot be assigned",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 118,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1019,
          body: "/assign",
          user: { login: "now-over-limit-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      openAssignmentCount: 3,
      openAssignmentCountExcludingBlocked: 3,
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @now-over-limit-user! Thanks for your enthusiasm to contribute!

To help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.

📊 **Your Current Assignments:** You're currently assigned to **3+** open issues.

👉 **View your assigned issues:**
[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Anow-over-limit-user%20-label%3A%22status%3A%20blocked%22)

💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.

Once you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Validation - At Limit With Blocked Issues (shows blocked link)",
    description:
      "User at 2 open (excluding blocked) and has 1 blocked issue; comment includes link to blocked issues",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 119,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1020,
          body: "/assign",
          user: { login: "at-limit-with-blocked", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      openAssignmentCount: 2,
      openAssignmentCountExcludingBlocked: 2,
      blockedIssueCount: 1,
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @at-limit-with-blocked! Thanks for your enthusiasm to contribute!

To help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.

📊 **Your Current Assignments:** You're currently assigned to **2** open issues.

👉 **View your assigned issues:**
[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Aat-limit-with-blocked%20-label%3A%22status%3A%20blocked%22)

👉 **View your blocked issues:**
[Your blocked issues](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Aat-limit-with-blocked%20label%3A%22status%3A%20blocked%22)

💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.

Once you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Validation - Under Assignment Limit (1 open issue)",
    description: "Contributor with 1 open issue can take another",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 116,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1017,
          body: "/assign",
          user: { login: "active-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { openAssignmentCount: 1 },
    expectedAssignee: "active-contributor",
    expectedComments: [
      `👋 Hi @active-contributor, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉

The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.

Good luck, and welcome aboard! 🚀`,
    ],
  },

  {
    name: "Validation - Open Assignments Exclude Blocked",
    description:
      "Contributor with 2 open issues both status: blocked can be assigned (blocked not counted)",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 117,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1018,
          body: "/assign",
          user: { login: "blocked-contributor", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      openAssignmentCount: 2,
      openAssignmentCountExcludingBlocked: 0,
    },
    expectedAssignee: "blocked-contributor",
    expectedComments: [
      `👋 Hi @blocked-contributor, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉

The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.

Good luck, and welcome aboard! 🚀`,
    ],
  },

  // ---------------------------------------------------------------------------
  // ERROR HANDLING (4 tests)
  // API failures
  // ---------------------------------------------------------------------------

  {
    name: "Error - Open Assignments API Failure",
    description: "Tags maintainers when open assignments check fails",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 117,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1018,
          body: "/assign",
          user: { login: "unlucky-user-3", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { restListOpenShouldFail: true },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @unlucky-user-3! I encountered an error while trying to verify your eligibility for this issue.

${MAINTAINER_TEAM} — could you please help with this assignment request?

@unlucky-user-3, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
    ],
  },

  {
    name: "Error - Prerequisite Check API Failure",
    description: "Tags maintainers when prerequisite check fails",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 109,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: beginner" },
          ],
        },
        comment: {
          id: 1010,
          body: "/assign",
          user: { login: "unlucky-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { restListClosedShouldFail: true },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @unlucky-user! I encountered an error while trying to verify your eligibility for this issue.

${MAINTAINER_TEAM} — could you please help with this assignment request?

@unlucky-user, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
    ],
  },

  {
    name: "Error - Assignment API Failure",
    description: "Tags maintainers when assignment fails",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 110,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1011,
          body: "/assign",
          user: { login: "unlucky-user-2", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { assignShouldFail: true },
    expectedAssignee: null,
    expectedComments: [
      `⚠️ Hi @unlucky-user-2! I tried to assign you to this issue, but encountered an error.

${MAINTAINER_TEAM} — could you please manually assign @unlucky-user-2 to this issue?

Error details: Simulated assignment failure`,
    ],
  },

  {
    name: "Error - Label Update Failure",
    description: "Tags maintainers when labels cannot be updated",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 111,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1012,
          body: "/assign",
          user: { login: "partially-lucky", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { removeLabelShouldFail: true, addLabelShouldFail: true },
    expectedAssignee: "partially-lucky",
    expectedComments: [
      `👋 Hi @partially-lucky, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉

The issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.

If you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.

Good luck, and welcome aboard! 🚀`,
      `⚠️ @partially-lucky has been successfully assigned to this issue, but I encountered an error updating the labels.

${MAINTAINER_TEAM} — please manually:
- Remove the \`status: ready for dev\` label
- Add the \`status: in progress\` label

Error details: Failed to remove 'status: ready for dev': Simulated remove label failure; Failed to add 'status: in progress': Simulated add label failure`,
    ],
  },

  // ---------------------------------------------------------------------------
  // DELETED COMMENT ABORT (1 test)
  // Bot aborts /assign when the triggering comment has been deleted
  // ---------------------------------------------------------------------------

  {
    name: "Abort - Triggering Comment Deleted",
    description:
      "Bot aborts /assign flow when acknowledgeComment fails (comment deleted)",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 400,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 4001,
          body: "/assign",
          user: { login: "deleted-comment-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: { reactionShouldFail: true },
    expectedAssignee: null,
    expectedComments: [],
    expectedNoSideEffects: true,
  },

  // ---------------------------------------------------------------------------
  // NO ACTION (2 tests)
  // Bot stays silent and takes no action
  // ---------------------------------------------------------------------------

  {
    name: "No Action - Comment Without /assign",
    description: "Regular comment ignored",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 112,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1013,
          body: "This looks interesting, can someone help me understand it?",
          user: { login: "curious-user", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [],
  },

  {
    name: "No Action - Bot Comment",
    description: "Bot users ignored to prevent loops",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 113,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 1014,
          body: "/assign",
          user: { login: "github-actions[bot]", type: "Bot" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {},
    expectedAssignee: null,
    expectedComments: [],
  },

  // ---------------------------------------------------------------------------
  // NEEDS-REVIEW BYPASS TESTS (6 tests)
  // Assignment cap bypass when all assigned issues have linked needs-review PRs
  // ---------------------------------------------------------------------------

  {
    name: "Bypass Cap - All Issues Have Needs-Review PRs (over limit)",
    description:
      "Contributor over the cap but all 3 assigned issues have linked needs-review PRs — bypass allows assignment",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 600,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6001,
          body: "/assign",
          user: { login: "bypass-review-user-1", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      mockUsername: "bypass-review-user-1",
      openAssignedIssues: [
        { number: 500, labels: [{ name: "status: in progress" }] },
        { number: 501, labels: [{ name: "status: in progress" }] },
        { number: 502, labels: [{ name: "status: in progress" }] },
      ],
      searchResults: {
        500: { total_count: 1 },
        501: { total_count: 1 },
        502: { total_count: 1 },
      },
    },
    expectedAssignee: "bypass-review-user-1",
    expectedComments: [
      `👋 Hi @bypass-review-user-1, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉\n\nThe issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.\n\nIf you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.\n\nGood luck, and welcome aboard! 🚀`,
    ],
  },

  {
    name: "Bypass Cap - One Issue Missing Needs-Review PR",
    description:
      "One assigned issue has a needs-review PR but the other does not — bypass fails, limit-exceeded comment posted",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 601,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6002,
          body: "/assign",
          user: { login: "bypass-review-user-2", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      mockUsername: "bypass-review-user-2",
      openAssignedIssues: [
        { number: 503, labels: [{ name: "status: in progress" }] },
        { number: 504, labels: [{ name: "status: in progress" }] },
      ],
      searchResults: {
        503: { total_count: 1 },
        504: { total_count: 0 },
      },
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @bypass-review-user-2! Thanks for your enthusiasm to contribute!\n\nTo help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.\n\n📊 **Your Current Assignments:** You're currently assigned to **2** open issues.\n\n👉 **View your assigned issues:**\n[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Abypass-review-user-2%20-label%3A%22status%3A%20blocked%22)\n\n💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.\n\nOnce you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Bypass Cap - No Issues Have Needs-Review PRs",
    description:
      "Neither assigned issue has a needs-review PR — bypass fails, limit-exceeded comment posted",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 602,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6003,
          body: "/assign",
          user: { login: "bypass-review-user-3", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      mockUsername: "bypass-review-user-3",
      openAssignedIssues: [
        { number: 505, labels: [{ name: "status: in progress" }] },
        { number: 506, labels: [{ name: "status: in progress" }] },
      ],
      searchResults: {
        505: { total_count: 0 },
        506: { total_count: 0 },
      },
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @bypass-review-user-3! Thanks for your enthusiasm to contribute!\n\nTo help contributors stay focused and ensure issues remain available for others, we limit assignments to **2 open issues** at a time. Issues labeled \`status: blocked\` are not counted toward this limit.\n\n📊 **Your Current Assignments:** You're currently assigned to **2** open issues.\n\n👉 **View your assigned issues:**\n[Your open assignments](https://github.com/hiero-ledger/hiero-sdk-cpp/issues?q=is%3Aissue%20is%3Aopen%20assignee%3Abypass-review-user-3%20-label%3A%22status%3A%20blocked%22)\n\n💡 **Tip:** If all of your open assigned issues have a linked PR with \`status: needs review\`, the limit is automatically bypassed — you can request a new assignment right away.\n\nOnce you complete or unassign from one of your current issues, come back and we'll be happy to assign this to you! 🎯`,
    ],
  },

  {
    name: "Bypass Cap - Search API Error",
    description:
      "Search API fails when checking for needs-review PRs — API error comment posted, no bypass",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 603,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6004,
          body: "/assign",
          user: { login: "bypass-review-user-4", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      openAssignedIssues: [
        { number: 507, labels: [{ name: "status: in progress" }] },
        { number: 508, labels: [{ name: "status: in progress" }] },
      ],
      searchShouldFail: true,
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @bypass-review-user-4! I encountered an error while trying to verify your eligibility for this issue.\n\n${MAINTAINER_TEAM} — could you please help with this assignment request?\n\n@bypass-review-user-4, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
    ],
  },

  {
    name: "Bypass Cap - Exactly At Limit With Needs-Review PRs",
    description:
      "Contributor at exactly MAX_OPEN_ASSIGNMENTS with all issues having needs-review PRs — bypass succeeds",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 604,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6005,
          body: "/assign",
          user: { login: "bypass-review-user-5", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      mockUsername: "bypass-review-user-5",
      openAssignedIssues: [
        { number: 509, labels: [{ name: "status: in progress" }] },
        { number: 510, labels: [{ name: "status: in progress" }] },
      ],
      searchResults: {
        509: { total_count: 1 },
        510: { total_count: 1 },
      },
    },
    expectedAssignee: "bypass-review-user-5",
    expectedComments: [
      `👋 Hi @bypass-review-user-5, welcome to the ${PROJECT_NAME} community! Thank you for choosing to contribute — we're thrilled to have you here! 🎉\n\nThe issue description above has everything you need: implementation steps, contribution workflow, and links to guides. If anything is unclear, just ask — we're happy to help.\n\nIf you realize you cannot complete this issue, simply comment \`/unassign\` to return it to the community pool.\n\nGood luck, and welcome aboard! 🚀`,
    ],
  },

  {
    name: "Bypass Cap - listAssignedIssues API Error",
    description:
      "listForRepo fails on the second call (listAssignedIssues) — API error comment posted, no bypass",
    context: {
      eventName: "issue_comment",
      payload: {
        issue: {
          number: 605,
          assignees: [],
          labels: [
            { name: "status: ready for dev" },
            { name: "skill: good first issue" },
          ],
        },
        comment: {
          id: 6006,
          body: "/assign",
          user: { login: "bypass-review-user-6", type: "User" },
        },
      },
      repo: { owner: "hiero-ledger", repo: "hiero-sdk-cpp" },
    },
    githubOptions: {
      openAssignmentCount: 2,
      restListOpenFailOnCall: 2,
    },
    expectedAssignee: null,
    expectedComments: [
      `👋 Hi @bypass-review-user-6! I encountered an error while trying to verify your eligibility for this issue.\n\n${MAINTAINER_TEAM} — could you please help with this assignment request?\n\n@bypass-review-user-6, a maintainer will review your request and assign you manually if appropriate. Sorry for the inconvenience!`,
    ],
  },
];

// =============================================================================
// TEST RUNNER
// =============================================================================

const { verifyComments, runTestSuite } = require("./test-utils");

async function runTest(scenario, index) {
  console.log("\n" + "=".repeat(70));
  console.log(`TEST ${index + 1}: ${scenario.name}`);
  console.log(`Description: ${scenario.description}`);
  console.log("=".repeat(70));

  const mockGithub = createMockGithub(
    scenario.githubOptions,
    scenario.context?.payload?.issue,
  );

  try {
    await script({ github: mockGithub, context: scenario.context });
  } catch (error) {
    console.log(`\n❌ SCRIPT THREW ERROR: ${error.message}`);
  }

  const results = {
    passed: true,
    details: [],
  };

  if (scenario.expectedAssignee) {
    if (mockGithub.calls.assignees.includes(scenario.expectedAssignee)) {
      results.details.push(
        `✅ Correctly assigned to ${scenario.expectedAssignee}`,
      );
    } else {
      results.passed = false;
      results.details.push(
        `❌ Expected assignee ${scenario.expectedAssignee}, got: ${mockGithub.calls.assignees.join(", ") || "none"}`,
      );
    }
  } else {
    if (mockGithub.calls.assignees.length === 0) {
      results.details.push("✅ Correctly did not assign anyone");
    } else {
      results.passed = false;
      results.details.push(
        `❌ Should not have assigned, but assigned: ${mockGithub.calls.assignees.join(", ")}`,
      );
    }
  }

  const commentResult = verifyComments(
    scenario.expectedComments || [],
    mockGithub.calls.comments,
  );
  if (!commentResult.passed) results.passed = false;
  results.details.push(...commentResult.details);

  console.log("\n📊 RESULT:");
  results.details.forEach((d) => console.log(`   ${d}`));

  return results.passed;
}

runTestSuite("BOT-ASSIGN-ON-COMMENT TEST SUITE", scenarios, runTest);
