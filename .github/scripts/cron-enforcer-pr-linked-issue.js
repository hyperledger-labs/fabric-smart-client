const dryRun = (process.env.DRY_RUN || "false").toLowerCase() === "true";
const hoursBeforeClose = parseInt(process.env.HOURS_BEFORE_CLOSE || "12", 10);
const requireAuthorAssigned = (process.env.REQUIRE_AUTHOR_ASSIGNED || "true").toLowerCase() === "true";
const MARKER_PREFIX = "<!-- linked-issue-enforcer:";

function getHoursOpen(pr) {
  return Math.floor((Date.now() - new Date(pr.created_at)) / (60 * 60 * 1000));
}

function isBot(pr) {
  return pr.user?.type === "Bot" || pr.user?.login?.endsWith("[bot]");
}

async function getPermission(github, owner, repo, username) {
  try {
    const response = await github.rest.repos.getCollaboratorPermissionLevel({
      owner,
      repo,
      username,
    });
    return response.data.permission || "none";
  } catch (error) {
    if (error.status === 404) {
      return "none";
    }
    throw error;
  }
}

function isExemptPermission(permission) {
  return ["admin", "write", "triage"].includes(permission);
}

async function getLinkedOpenIssues(github, owner, repo, prNumber) {
  const query = `
    query($owner: String!, $repo: String!, $prNumber: Int!) {
      repository(owner: $owner, name: $repo) {
        pullRequest(number: $prNumber) {
          closingIssuesReferences(first: 100) {
            nodes {
              number
              state
              assignees(first: 100) {
                nodes {
                  login
                }
              }
            }
          }
        }
      }
    }
  `;

  const result = await github.graphql(query, { owner, repo, prNumber });
  const issues = result.repository.pullRequest.closingIssuesReferences.nodes || [];
  return issues.filter((issue) => issue.state === "OPEN");
}

function authorAssigned(issues, authorLogin) {
  return issues.some((issue) =>
    (issue.assignees?.nodes || []).some((assignee) => assignee.login === authorLogin),
  );
}

async function commentOnce(github, owner, repo, prNumber, reason, body) {
  const marker = `${MARKER_PREFIX}${reason} -->`;
  const comments = await github.paginate(github.rest.issues.listComments, {
    owner,
    repo,
    issue_number: prNumber,
    per_page: 100,
  });

  if (comments.some((comment) => comment.body?.includes(marker))) {
    return;
  }

  if (dryRun) {
    console.log(`[enforcer] DRY RUN comment for PR #${prNumber}: ${reason}`);
    return;
  }

  await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: prNumber,
    body: `${marker}\n${body}`,
  });
}

async function closePr(github, owner, repo, prNumber) {
  if (dryRun) {
    console.log(`[enforcer] DRY RUN close PR #${prNumber}`);
    return;
  }

  await github.rest.pulls.update({
    owner,
    repo,
    pull_number: prNumber,
    state: "closed",
  });
}

module.exports = async ({ github, context }) => {
  try {
    const { owner, repo } = context.repo;
    const prs = await github.paginate(github.rest.pulls.list, {
      owner,
      repo,
      state: "open",
      per_page: 100,
    });

    for (const pr of prs) {
      if (isBot(pr)) {
        continue;
      }

      const permission = await getPermission(github, owner, repo, pr.user.login);
      if (isExemptPermission(permission)) {
        continue;
      }

      if (getHoursOpen(pr) < hoursBeforeClose) {
        continue;
      }

      const linkedIssues = await getLinkedOpenIssues(github, owner, repo, pr.number);
      if (!linkedIssues.length) {
        await commentOnce(
          github,
          owner,
          repo,
          pr.number,
          "no-linked-issue",
          `Hi @${pr.user.login}, this PR has no linked open issue. Please link it to a triaged issue, for example by adding \`Fixes #123\` to the PR description.`,
        );
        await closePr(github, owner, repo, pr.number);
        continue;
      }

      if (requireAuthorAssigned && !authorAssigned(linkedIssues, pr.user.login)) {
        const linkedIssueNumbers = linkedIssues.map((issue) => `#${issue.number}`).join(", ");
        await commentOnce(
          github,
          owner,
          repo,
          pr.number,
          "author-not-assigned",
          `Hi @${pr.user.login}, this PR links ${linkedIssueNumbers}, but you are not assigned to any of those issues. Please claim the linked issue first and then reopen the PR if it gets closed.`,
        );
        await closePr(github, owner, repo, pr.number);
      }
    }
  } catch (error) {
    console.error("[enforcer] Error:", error.message);
    throw error;
  }
};
