# fetch-prs

## Description

`fetch-prs` is a Go CLI that fetches GitHub work contributions across multiple repositories within a custom date range.

It reports:

- pull requests authored by `GITHUB_USERNAME`
- closed issues that are backed by a valid timeline-linked commit attributable to `GITHUB_USERNAME`

## Features

- Search across multiple repositories configured in `REPOS`
- Filter work items by inclusive start and end dates
- Mark work items with `status` (`wip` or `done`)
- Output invoice-friendly plain text with `--plain`
- Output structured data with `--json`
- Install the CLI with a single macOS/Linux command via `install.sh`
- Publish tagged GitHub releases automatically with semantic-release

## Prerequisites

- A GitHub personal access token with permission to read the repositories you want to scan
- Go if you want to build locally or if the installer falls back to source builds

## Configuration

Create a `.env` file in the repository root:

```env
GITHUB_USERNAME=your-github-username
GITHUB_TOKEN=your-github-token
REPOS=hashgraph/solo-weaver,swirlds/swirlds-docker
```

### Environment variables

- `GITHUB_USERNAME`: GitHub username whose work should be included
- `GITHUB_TOKEN`: personal access token used for GitHub API requests
- `REPOS`: comma-separated list of repositories in `owner/repo` format

## Installation

### Quick install for macOS/Linux

Install the latest released version:

```bash
curl -fsSL https://raw.githubusercontent.com/mehedygroup/fetch-prs/main/install.sh | bash
```

Install a specific release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/mehedygroup/fetch-prs/main/install.sh | bash -s -- v0.1.1
```

Pin the version with an environment variable:

```bash
curl -fsSL https://raw.githubusercontent.com/mehedygroup/fetch-prs/main/install.sh | FETCH_PRS_VERSION=v0.1.1 bash
```

Optional installer environment variables:

- `FETCH_PRS_VERSION`: release tag to install, such as `v0.1.1`
- `VERSION`: generic alternative to `FETCH_PRS_VERSION`
- `INSTALL_DIR`: destination directory for the binary, default: `/usr/local/bin`
- `GITHUB_TOKEN`: optional GitHub token for release API requests if you need higher rate limits

The installer looks for a matching prebuilt GitHub release asset first. If no matching asset is available, it falls back to downloading the GitHub release source tarball and building locally.

### Build locally

```bash
go build -o fetch-prs .
```

## Usage

JSON output:

```bash
./fetch-prs fetch 2025-12-01 2025-12-15 --json
```

Plain output is the default and is the most convenient for invoice lines:

```bash
./fetch-prs fetch 2025-12-01 2025-12-15 --plain
```

### Output formats

- `--plain`
- `--json`

### Plain output example

```text
[done] hashgraph/solo-weaver: feat: allow RSL to resolve effective value from multiple sources deterministically, #446
[wip] hashgraph/solo-weaver: feat: daemon core wiring, #688
[done] hashgraph/solo-weaver: fix cache invalidation on startup, #589 (commit abc1234)
```

### Notes

- The end date is inclusive
- PR status is `done` when merged; otherwise `wip`
- Closed issues are included only when GitHub issue timeline events point to a commit authored by `GITHUB_USERNAME`

## Development

Run tests:

```bash
go test ./...
```

Build release assets locally:

```bash
task release:artifacts VERSION=v0.1.1
```

## Releases

This repository uses semantic-release for GitHub releases.

- Pushes to `main` are evaluated for a new release
- Release tags use the `vX.Y.Z` format
- Release assets are built for:
  - `darwin/amd64`
  - `darwin/arm64`
  - `linux/amd64`
  - `linux/arm64`

Use conventional commits so semantic-release can determine version bumps automatically.

## Contributing

Contributions are welcome:

1. Fork the repository.
2. Create a branch for your feature or bug fix.
3. Use conventional commits when practical.
4. Open a pull request.

## License

This project is licensed under the Apache 2.0 License. See the `LICENSE` file for details.
