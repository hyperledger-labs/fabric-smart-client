const READY_LABEL = "ready";
const MAX_OPEN_ASSIGNMENTS = 2;
const ASSIGN_MARKER_PREFIX = "<!-- contribution-process-assigned:";

function requestsAssign(body) {
  return typeof body === "string" && /(^|\s)\/assign(\s|$)/i.test(body);
}

function isEligibleIssue(issue) {
  return issue && !issue.pull_request && issue.state === "open";
}

function hasReadyLabel(issue) {
  return Array.isArray(issue.labels) && issue.labels.some((label) => label.name === READY_LABEL);
}

function buildAssignmentMarker(username) {
  return `${ASSIGN_MARKER_PREFIX}${username} -->`;
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

async function countOpenAssignedIssues(github, owner, repo, username) {
  const issues = await github.paginate(github.rest.issues.listForRepo, {
    owner,
    repo,
    assignee: username,
    state: "open",
    per_page: 100,
  });

  return issues.filter((issue) => !issue.pull_request).length;
}

module.exports = async ({ github, context }) => {
  try {
    const { issue, comment, repository } = context.payload;
    if (!isEligibleIssue(issue) || !comment?.user?.login || comment.user.type === "Bot") {
      return;
    }
    if (!requestsAssign(comment.body)) {
      return;
    }

    const owner = repository.owner.login;
    const repo = repository.name;
    const username = comment.user.login;

    if (!hasReadyLabel(issue)) {
      await github.rest.issues.createComment({
        owner,
        repo,
        issue_number: issue.number,
        body: `Hi @${username}, this issue is not marked \`${READY_LABEL}\` yet. Please wait for maintainer triage before claiming it.`,
      });
      return;
    }

    if (issue.assignees?.length) {
      const currentAssignee = issue.assignees[0].login;
      await github.rest.issues.createComment({
        owner,
        repo,
        issue_number: issue.number,
        body: `Hi @${username}, this issue is already assigned to @${currentAssignee}.`,
      });
      return;
    }

    const permission = await getPermission(github, owner, repo, username);
    if (!isExemptPermission(permission)) {
      const openAssignments = await countOpenAssignedIssues(github, owner, repo, username);
      if (openAssignments >= MAX_OPEN_ASSIGNMENTS) {
        await github.rest.issues.createComment({
          owner,
          repo,
          issue_number: issue.number,
          body: `Hi @${username}, you already have ${openAssignments} open assigned issue(s). Please finish one before claiming another.`,
        });
        return;
      }
    }

    await github.rest.issues.addAssignees({
      owner,
      repo,
      issue_number: issue.number,
      assignees: [username],
    });

    await github.rest.issues.createComment({
      owner,
      repo,
      issue_number: issue.number,
      body: `${buildAssignmentMarker(username)}\nAssigned @${username} to this issue.`,
    });
  } catch (error) {
    console.error("[assign] Error:", error.message);
    throw error;
  }
};
