function isValidContext(issue, comment) {
  if (!issue?.number || issue.pull_request) return false;
  if (!comment?.body || !comment?.user?.login) return false;
  if (comment.user.type === "Bot") return false;
  if (issue.state !== "open") return false;
  return true;
}

function requestsUnassign(body) {
  return typeof body === "string" && /(^|\s)\/unassign(\s|$)/i.test(body);
}

function marker(username) {
  return `<!-- unassign-requested:${username} -->`;
}

module.exports = async ({ github, context }) => {
  try {
    const { issue, comment, repository } = context.payload;
    if (!isValidContext(issue, comment) || !requestsUnassign(comment.body)) {
      return;
    }

    const owner = repository.owner.login;
    const repo = repository.name;
    const username = comment.user.login;

    if (!issue.assignees?.some((assignee) => assignee.login === username)) {
      return;
    }

    const comments = await github.paginate(github.rest.issues.listComments, {
      owner,
      repo,
      issue_number: issue.number,
      per_page: 100,
    });

    if (comments.some((entry) => entry.body?.includes(marker(username)))) {
      return;
    }

    await github.rest.issues.removeAssignees({
      owner,
      repo,
      issue_number: issue.number,
      assignees: [username],
    });

    await github.rest.issues.createComment({
      owner,
      repo,
      issue_number: issue.number,
      body: `${marker(username)}\n\n@${username} has been unassigned from this issue.`,
    });
  } catch (error) {
    console.error("[unassign] Error:", error.message);
    throw error;
  }
};
