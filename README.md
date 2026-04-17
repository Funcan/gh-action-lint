# gh-action-lint

A linter for GitHub Actions workflows that checks actions are pinned to a full commit SHA rather than a mutable tag or branch name.

Pinning to a tag like `@v4` or `@main` means your workflow can silently change if the tag is moved. This supply chain attack has been exploited multiple times in 2026 and iis likely to get worse in future. Pinning to a SHA gives you a fully reproducible, tamper-resistant build.

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

### Example output

```
.github/workflows/ci.yml:8: action not pinned to a SHA: actions/checkout@v4
.github/workflows/ci.yml:12: action not pinned to a SHA: actions/cache@main
```

Exits with code `1` if any warnings are found, making it suitable for use in CI.

### What is checked

- All files under `.github/workflows/` (`*.yml`, `*.yaml`)
- All `action.yml` / `action.yaml` files under `.github/actions/` (composite actions)

A `uses:` value is considered unsafe if the ref after `@` is not a full 40-character hex commit SHA. The following are **ignored** (not flagged):

| Pattern | Example | Reason |
|---|---|---|
| Local actions | `./my-action` | No remote ref |
| Docker images | `docker://alpine:3.18` | Not a GitHub Action |

### Fixing a warning

Find the SHA for the version you want to pin to. For example, for `actions/checkout@v4`:

```sh
git ls-remote https://github.com/actions/checkout refs/tags/v4
```

Then update your workflow:

```yaml
# Before
- uses: actions/checkout@v4

# After
- uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4
```

## License

GPLv2 — see [LICENSE](LICENSE).
