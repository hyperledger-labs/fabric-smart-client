function isValidContext(comment) {
  if (!comment?.body || !comment?.user?.login) return false;
  if (comment.user.type === "Bot") return false;
  return true;
}

function requestsWorking(body) {
  return typeof body === "string" && /(^|\s)\/working(\s|$)/i.test(body);
}

function isAuthorized(issue, username) {
  if (issue.pull_request && issue.user?.login === username) {
    return true;
  }
  return issue.assignees?.some((assignee) => assignee.login === username);
}

module.exports = async ({ github, context }) => {
  const dryRun = (process.env.DRY_RUN || "false").toLowerCase() === "true";
  try {
    const { issue, comment, repository } = context.payload;
    if (!issue || !isValidContext(comment) || !requestsWorking(comment.body)) {
      return;
    }

    const username = comment.user.login;
    if (!isAuthorized(issue, username)) {
      return;
    }

    if (dryRun) {
      console.log(`[working] Dry run for comment ${comment.id}`);
      return;
    }

    await github.rest.reactions.createForIssueComment({
      owner: repository.owner.login,
      repo: repository.name,
      comment_id: comment.id,
      content: "eyes",
    });
  } catch (error) {
    console.error("[working] Error:", error.message);
  }
};
