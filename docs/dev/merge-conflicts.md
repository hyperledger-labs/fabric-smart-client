# Resolving Merge Conflicts

Merge conflicts are caused by working on outdated versions of the codebase, or another developer merging a change involving similar parts of the codebase to you.

> [!IMPORTANT]
> **Avoid Merge Conflicts** by syncing your branch regularly [Sync Guide](rebasing.md).

## Step-by-Step Guide

### 1. See which files are conflicted
```bash
git status
```

### 2. Understand the conflict markers
You will see sections like:

```text
<<<<<<< HEAD
code from main
=======
your branch’s code
>>>>>>> mybranch
```

### 3. Decide what the final code should be

Have a vision of what you'd like the final code to look like, given what is currently on main and what you'd like to propose.

> [!WARNING]
> - Do not blindly accept both changes
> - Do not blindly accept incoming
> - Do not blindly accept existing

Merge conflicts require: ✅ Human interpretation

Sometimes you'll be:
- Accepting both incoming and current
- Accepting only incoming
- Accepting only current
- Accepting **parts** of both incoming and current

Generally, you want to keep all changes that were merged to main, but additionally, layer on your changes.

### 4. Resolve conflicts in VS Code (recommended):
Once you understand you have a merge conflict and have a vision of the final document, we recommend using VS Code.


VS Code makes solving merge-conflicts easier with a 3-pane interface for resolving conflicts:

- Incoming Change → code from main (left/top)
- Current Change → code from your branch (right/top)
- Result → the lower/third pane, where you create the final merged file.

You want to accept or reject content from the top left and top right panes, and edit the final pane in the bottom so the final code submission reasonably resolves the issue while respecting the work of others.

#### Steps to resolve in VS Code

1. Open the conflicted file in VS Code.

2. Look at both the Incoming Change (left) and Current Change (right) panels.

3. In the Result (lower pane), edit the file so it contains the correct final version.
   - Sometimes keep Incoming (main)
   - Sometimes keep Current (your branch)
   - Often, combine both parts and edit the code manually.

   If the conflict is resolved correctly, VS Code will mark it as fixed.

4. Save the lower pane file once there are no more merge conflicts to resolve in this file.

5. Click the add button next to the file to resolve conflicts or:
   ```bash
   git add <file>
   ```

6. Continue the rebase:
   ```bash
   git rebase --continue
   ```

7. If there are more conflicts in other files, VS Code will automatically move you to the next one. Repeat until no conflicts remain.

   > [!WARNING]
   > Do not just click “Accept All Incoming” or “Accept All Current” — that usually **deletes** or **corrupts** important code.

   Once the rebase operation completes, your commits will be layered on top of main. It means your commit history will look “different” and you may even see changes to commits from other authors — this is expected, since rebase rewrites history.

8. Push changes. If you already have an open Pull Request, you will need to update it with a **force push**. Before pushing, double-check that your commits are both DCO signed and GPG verified:
   ```bash
   git log -n 10 --pretty=format:'%h %an %G? %s'
   ```
   Ensure you see: `G` = Good (valid signature)

   Then:
   ```bash
   git push --force-with-lease
   ```

> [!TIP]
> To be safe, create a backup branch before force pushing.

## Common Issues

#### 1. Message: *“No changes – did you forget to use git add?”*
- **What it means:** You resolved the conflicts but forgot to stage the files.
- **Solution:** Run `git add <file>` and try again.

#### 2. Message: *“Are you sure you want to continue with conflicts?”*
- **What it means:** Some conflicts are still unresolved or the files were not saved properly.
- **Solution:** Double-check your files in VS Code, make sure they are saved, and resolve any remaining conflict markers:

### If you need to stop
```bash
git rebase --abort
```

> [!TIP]
> At each conflict: resolve → save → stage → continue. Repeat until all conflicts are gone.

### What NOT to do
1. ❌ Do not run git merge main
   → This creates messy merge commits. Always rebase instead.

2. ❌ Do not merge into your local main
   → Keep main as a clean mirror of upstream/main.

3. ❌ Do not open PRs from your fork’s main
   → Always create a feature branch for your changes.

At each conflict instance, you'll have to repeat: fix the conflict, stage the files and continue rebasing.

## Recovery Tips

Undo the last rebase commit, but keep changes staged (while still in rebase):
  If you are in the middle of a rebase and realize the last step went wrong, you can undo it while keeping changes staged:
```bash
git reset --soft HEAD~i
```

> [!NOTE]
> The number after HEAD~ refers to how many commits you want to go back.

For example:
- HEAD~1 → go back 1 commit
- HEAD~3 → go back 3 commits
- HEAD~5 → go back 5 commits

### If you are completely stuck
Sometimes a rebase can get too messy to fix conflict by conflict. In that case, it’s often easier to start fresh:

1. Abort the rebase to stop where you are:
```bash
git rebase --abort
```

2. Reset your fork's main to the upstream main and layer your commits on top of that:
``` bash
git checkout main
git reset --hard upstream/main
git push origin main --force-with-lease
git checkout mybranch
git rebase upstream/main -S
```

> [!WARNING]
> Use git stash only if you really want to save some local changes that aren’t yet committed. In most cases, if the rebase is failing, it’s safer to abort or reset rather than reapplying a stash of broken work.