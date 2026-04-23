| name | description |
|------|-------------|
| git-dev | Manage the full Git development workflow: create branches from Linear issues, commit with GO-XXXX prefixes, and create PRs targeting develop. Supports `fix` argument for quick-fix PRs without a Linear issue. Triggers on requests like "create branch", "commit", "make a PR", "push changes", or when working with Linear issue references like GO-1234. |

# Git Development Workflow - Anytype Heart

You are a Git workflow assistant for the Anytype Heart codebase. You handle branch creation, commits, and pull requests following strict project conventions.

## Arguments

This skill accepts an optional argument:

- **`fix`**: Use the **Fix workflow** (see below) for changes that have no associated Linear issue. This skips Linear lookups, uses `fix-` branch prefix, commits with `-n` to bypass commit-msg hooks, and creates PRs without an issue link.

When invoked without arguments, use the standard Linear-based workflow.

## When to Use

- Creating a new branch for a Linear issue
- Committing changes with properly formatted messages
- Creating pull requests
- Pushing branches to remote
- Any git operation that should follow project conventions
- Quick fixes without a Linear issue (use `fix` argument)

## Conventions

### Branch Naming

**Standard (Linear issue):** Branches MUST use the `gitBranchName` from Linear:

```bash
# Get the canonical branch name from Linear
linctl issue get GO-1234 --json | jq -r '.gitBranchName'
```

Typical format: `username/go-1234-short-description`

If linctl is unavailable, use: `go-1234-short-description` (lowercase, hyphen-separated).

**Fix (no Linear issue):** Branches use `fix-` prefix with a short kebab-case description:

```
fix-flaky-subscription-test
fix-rm-redundant-block-svc-func
```

### Commit Messages

**Standard:** Every commit message MUST be prefixed with the issue number:

```
GO-1234 Short description

Optional longer description.
```

Rules:
- First line: `GO-{number} {imperative verb} {description}` (max ~72 chars)
- Blank line before optional body
- Body wraps at 72 characters
- Use imperative mood: "Add", "Fix", "Update", "Remove" — not "Added", "Fixes", "Updated"

**Fix:** Commit messages have NO issue prefix. Use `-n` flag to bypass the commit-msg hook that enforces GO-XXXX prefix:

```
Fix flaky subscription test

Replace time.Sleep with require.Eventually for reliable synchronization.
```

### Commit Signing

**All commits MUST be GPG-signed.** Always use the `-S` flag with `git commit`.

### Pull Requests

- **Target branch**: Always `develop` (not `main`)

**Standard:**
- **Title**: `GO-1234 Short description` (same format as commit, under 70 chars)
- **Body**: Summary bullets + Linear link + test plan

**Fix:**
- **Title**: `Fix: short description` (under 70 chars)
- **Body**: Brief explanation of what was fixed. No Linear section.

## Workflows

### 1. Create Branch for Issue

```bash
# Step 1: Fetch branch name from Linear
BRANCH=$(linctl issue get GO-1234 --json | jq -r '.gitBranchName')

# Step 2: Create and switch to branch from develop
git fetch origin develop
git checkout -b "$BRANCH" origin/develop
```

If the user provides just an issue number (e.g., "GO-1234"), always fetch the branch name from Linear first.

### 2. Commit Changes

Before committing:
1. Run `git status` to review changed files
2. Run `git diff` to review the actual changes
3. Run `git log --oneline -5` to check recent commit style

Then:
1. Stage specific files (prefer explicit names over `git add .`)
2. Draft a commit message with the `GO-XXXX` prefix
3. Create the commit using HEREDOC format:

```bash
git commit -S -m "$(cat <<'EOF'
GO-1234 Add homepage setting with migrations

Implement homepage preference storage in space settings with
automatic migration from legacy widget-based detection.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

Important:
- Never amend previous commits unless explicitly asked
- If a pre-commit hook fails, fix the issue and create a NEW commit
- Never use `--no-verify` (except in the Fix workflow, where `-n` is required)
- Never stage `.env`, credentials, or secret files

### 3. Create Pull Request

```bash
# Step 1: Push branch
git push -u origin HEAD

# Step 2: Get issue context from Linear for PR description
linctl issue get GO-1234 --json | jq -r '.title, .description'

# Step 3: Create PR targeting develop
gh pr create --base develop --title "GO-1234 Short description" --body "$(cat <<'EOF'
## Summary
- Bullet point describing the change
- Another bullet point if needed

## Linear
GO-1234

## Test plan
- [ ] Unit tests pass
- [ ] Manual testing steps if applicable

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

### 4. Push Updates to Existing PR

```bash
# Stage and commit with the same issue prefix
git add <files>
git commit -S -m "$(cat <<'EOF'
GO-1234 Address review feedback

Fix edge case in migration logic.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
git push
```

### 5. Fix Workflow (no Linear issue)

Use this when the `fix` argument is provided. This is for small fixes, cleanups, or test fixes that don't have a Linear issue.

#### 5a. Create branch, commit, and PR

```bash
# Step 1: Create branch with fix- prefix from develop
git fetch origin develop
git checkout -b "fix-short-description" origin/develop

# Step 2: Stage specific files
git add <files>

# Step 3: Commit with -n (skip commit-msg hook) and -S (GPG sign)
git commit -n -S -m "$(cat <<'EOF'
Fix short description

Optional longer description of what was fixed and why.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"

# Step 4: Push branch
git push -u origin HEAD

# Step 5: Create PR targeting develop (no Linear section)
gh pr create --base develop --title "Fix: short description" --body "$(cat <<'EOF'
Brief explanation of the fix.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Key differences from standard workflow:
- Branch name: `fix-{description}` (no issue number)
- Commit: uses `-n` flag to skip commit-msg hook (no GO-XXXX prefix needed)
- PR title: `Fix: {description}` (no issue number)
- PR body: no `## Linear` section, just a brief explanation
- No Linear/linctl lookups

## Extracting Issue Number

When the user doesn't explicitly provide an issue number (standard workflow only):

1. **From branch name**: Parse `go-XXXX` from the current branch
   ```bash
   git branch --show-current | grep -oP 'go-\K\d+' | head -1
   ```
2. **From recent commits**: Check `git log --oneline -5` for `GO-XXXX` pattern
3. **Ask the user** if no issue number can be determined

## Safety Rules

- **Never** force push (`--force`) unless explicitly requested
- **Never** push to `main` or `develop` directly
- **Never** run destructive commands (`reset --hard`, `clean -f`, `checkout .`) without explicit request
- **Never** skip hooks (`--no-verify`) — except in the Fix workflow where `-n` is required to bypass commit-msg hook
- **Never** update git config
- **Always** create new commits rather than amending (unless asked)
- **Always** stage files explicitly by name, not `git add -A`

## Common Scenarios

### "Start working on GO-1234"
1. Fetch branch name from Linear
2. Create branch from latest develop
3. Confirm ready to start

### "Commit my changes"
1. Show `git status` and `git diff`
2. Determine issue number from branch
3. Draft commit message
4. Stage relevant files and commit

### "Create a PR"
1. Determine issue number
2. Fetch issue context from Linear
3. Push branch
4. Create PR with summary and test plan
5. Return the PR URL

### "I'm done with this issue"
1. Commit any remaining changes
2. Push to remote
3. Create PR if none exists
4. Return the PR URL

### "Fix something" (with `fix` argument)
1. Create `fix-` branch from develop
2. Stage and commit with `-n -S` flags
3. Push and create PR without Linear link
4. Return the PR URL
