# Commit Signing Guidelines (DCO + GPG)

**Both DCO sign-off and GPG signature signed commits** are required for all pull requests to be merged successfully.

This guide makes it as easy as possible for you to set up your GPG key and DCO and GPG sign your commits.

| Signature         | Flag | Purpose                                                                           | GitHub Check   |
|:------------------|:-----|:----------------------------------------------------------------------------------|:---------------|
| **DCO Sign-off**  | `-s` | Confirms legal right to contribute code (required by CI bot).                     | DCO Check      |
| **GPG Signature** | `-S` | Proves you are the author of the commit (required by CI bot, requires GPG setup). | Verified Badge |

> [!IMPORTANT]
> To pass the DCO check and achieve the "Verified" status, **all commits** must be signed using **both** the `-S` and `-s` flags together.

## Step-by-Step Guide

### 1. Generate a GPG key

If you don't already have a GPG key:

```bash
gpg --full-generate-key
```

Choose:
- Kind: ECC (sign and encrypt) *default*
- Elliptic curve: Curve 25519 *default*
- Expiration: 0 *default* (does not expire)
- Name, Email: Must match your GitHub email
- Passphrase: Set a strong passphrase that you'll need to remember

Learn more [GPG key set-up documentation on Github](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key)

Once created, list your keys:

```bash
gpg --list-secret-keys --keyid-format LONG
```

Copy the key ID (looks like `34AA6DBC`).

### 2. Add your GPG key to GitHub

Export your GPG public key:

```bash
gpg --armor --export YOUR_KEY_ID
```

Paste the output into GitHub:
* [Add GPG key on Github](https://github.com/settings/gpg/new)

### 3. Configure Git to use your GPG key

```bash
git config --global user.signingkey YOUR_KEY_ID
git config --global commit.gpgsign true
```

### 4. Make signed commits

**All commits must be signed using both DCO and GPG.**
Each time you create a commit, use -S and -s flags like this:

```bash
git commit -S -s -m "chore: your commit message"
```

* `-S` = GPG sign
* `-s` = DCO sign-off

> [!IMPORTANT]
> Ensure **every commit** in your branch follows this rule.

### 5. Verify signed status of commits

To check that your commits are signed correctly:

```bash
git log --show-signature
```

* Ensure each commit shows both **GPG verified** and **DCO signed-off**.

For a quick check of recent n commits:
Note how many commits you have added, and make n equal to that.

```bash
git log -n --pretty=format:'%h %an %G? %s'
```
Legend:

- G = Good (valid signature - you want to see `G`)
- B = Bad (invalid signature)
- U = Unknown (not signed)
- E = Signed (but not verifiable locally)

## Fixing Unsigned Commits

If you accidentally forgot to sign commits, there are **two ways to fix them**:

### 1. Soft reverting commits (recommended for new contributors)

Soft revert the impacted commits while keeping changes locally:

```bash
git reset --soft HEAD~n
```

* `HEAD~n` = number of commits to go back
* Example: To fix the last 3 commits: `git reset --soft HEAD~3`

Then, recommit each commit with proper signing:

```bash
git commit -S -s -m "chore: your commit message"
```

### 2. Retroactively signing commits

Alternatively, you can **amend commits retroactively**:

```bash
git commit --amend -S -s
git rebase -i HEAD~n            # For multiple commits
git push --force-with-lease
```
This is difficult and you may run into problems, for example, if you have merged from main.

## Rebasing and Signing

Rebase operations will be required when your branch is behind the upstream main. We do not recommend merging from main, rebasing is strongly suggested.

When rebasing, you must use this command to ensure your commits remain DCO and GPG signed:

```bash
git rebase main -S
```

> [!NOTE]
> `git push --force-with-lease` safely updates the remote branch without overwriting others' changes.