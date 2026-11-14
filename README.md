<!-- Copyright (c) 2026 Alex So. -->

# gh-runner-monitor

A small terminal dashboard for GitHub Actions self-hosted runners and queued jobs across selected repositories. All GitHub operations are read-only and go through GitHub CLI. No service, database, or stored application credentials are needed.

```text
gh-runner-monitor
Observed running: 2 · Idle: Unknown · Queued: 3 · Updated: 12:34:56
╭ Runners ───────────────────╮ ╭ Job queue ─────────────────────────╮
│ › runner-1 · Running       │ │ › project-a · CI / test · ~2m10s    │
│   runner-2 · —             │ │   project-b · CI / build · ~40s  │
╰───────────────────────────╯ ╰────────────────────────────────────╯
Runner inventory access restricted
q quit · r refresh · Tab focus · ↑↓/jk move · Enter open · ? help
```

Wide terminals show both panels side by side. Medium terminals stack them; short terminals show the focused panel (switch with Tab). Lists scroll with selection.

## Prerequisites and installation

- Go 1.26.7 or newer for building.
- [GitHub CLI](https://cli.github.com/) installed and available as `gh` at runtime.
- A terminal; a default browser is needed only for opening jobs.

From this project directory:

```sh
gh auth login --hostname github.com
go build -o bin/gh-runner-monitor ./cmd/gh-runner-monitor
./bin/gh-runner-monitor init
# Edit the generated config, then:
./bin/gh-runner-monitor --check
./bin/gh-runner-monitor
```

To install into your Go binary directory, run `go install ./cmd/gh-runner-monitor` and ensure `$(go env GOPATH)/bin` is on PATH. Alternatively, copy the built executable into a directory on PATH. GitHub CLI must remain installed; the application is otherwise a single executable.

```sh
gh-runner-monitor --config ./config.toml
gh-runner-monitor init --config ./config.toml
gh-runner-monitor --help
gh-runner-monitor --version
```

`init` never overwrites an existing file. `--check` prints the API-confirmed account name and observed running job assignments (including group-filter exclusions), validates config, resolves authentication and fetches all monitored API data without opening the UI. Restricted inventory is reported as a warning and does not fail the check if repository monitoring works. Development builds report `dev`; set a version with `go build -ldflags '-X main.version=1.0.0' ./cmd/gh-runner-monitor`.

## Configuration

The default is `~/.config/gh-runner-monitor/config.toml` on Linux and macOS, honoring `XDG_CONFIG_HOME`; Windows uses `%AppData%\gh-runner-monitor\config.toml`. `--config` overrides it on all platforms.

```toml
[github]
# Required: GitHub organization login, for example "my-organization".
organization = ""

# Optional: existing gh account username. Empty uses the active gh account.
account = ""

# Required: repository names without owner, for example ["project-a", "project-b"].
# Wildcards are not supported.
repositories = []

# Optional: exact runner group name. Empty monitors all groups.
# Jobs with unknown groups remain visible.
runner_group = ""

# Optional: exact known runner names, for example ["runner-1", "runner-2"].
# These remain visible as Unknown when their status cannot be verified.
# Wildcards are not supported.
runners = []

[display]
# Automatic refresh: at least 1m. Units: s = seconds, m = minutes, h = hours.
# Examples: "120s", "2m", "1h". Default: "2m".
# "0s" fetches once at startup, then only when you press r (no automatic retries).
refresh_interval = "2m"

# Queue sorting: "oldest" is the only supported value (and the default).
# Uses job creation time, not GitHub's exact scheduling order.
queue_sort = "oldest"
```

The generated template intentionally has empty GitHub settings and must be edited before running or using `--check`. Organization and at least one repository name are required. Use bare repository names, not `owner/repository`. Account, runner group and known runner names are optional. Refresh defaults to `"2m"`; automatic intervals must be at least `"1m"`. Use `"0s"` for an initial fetch followed by manual refreshes only (`r`), with no automatic retries. Durations support seconds (`s`), minutes (`m`), and hours (`h`); for example `"120s"`, `"2m"`, or `"1h"`. Existing configurations using `"30s"` must be updated. The supported queue sort is `oldest`. Known runner names remain visible as Unknown until verified by inventory or a running assignment.

## Accounts and permissions

Log in to each desired account with `gh auth login`. An empty `github.account` uses the stored active GitHub.com account. A specified account uses `gh auth token --hostname github.com --user ACCOUNT`. The program never switches the global active account. Inherited `GH_TOKEN` and `GITHUB_TOKEN` are removed when resolving credentials; API subprocesses receive the resolved token only in their environment. Tokens are never written to config or logged.

Repository **Actions: read** permission is sufficient for workflow/job monitoring with a fine-grained token, subject to repository and organization policies. Classic tokens generally need `repo` for private repositories. Organization runner inventory additionally needs organization **Self-hosted runners: read** permission (or `admin:org` for classic tokens) and suitable organization access. Runner group listing also needs organization access. SSO organizations may require credential authorization. See GitHub's [workflow job API](https://docs.github.com/en/rest/actions/workflow-jobs) and [self-hosted runner API](https://docs.github.com/en/rest/actions/self-hosted-runners) permission documentation.

A denied organization inventory request (403/404) leaves repository monitoring working. Idle and Offline are shown only when inventory confirms them. Busy inventory runners are Running even if their job belongs to an unmonitored repository; their job details are unavailable. Without inventory, running self-hosted jobs are discovered using known runner names, a reported runner group, or the `self-hosted` label. Other runners remain Unknown internally and display `—` in the runners panel (“Status could not be verified”). The summary retains `Idle: Unknown` when the count cannot be verified. Counts are observations, not a complete organization total.

## Controls

| Key | Action |
| --- | --- |
| q / Ctrl+C | Quit |
| r | Refresh now |
| Tab | Switch panel focus |
| ↑ / ↓ or k / j | Move selection |
| Enter | Open selected job in default browser |
| ? | Toggle help |

## Behavior and limitations

- Active workflow runs are queried by status, then their jobs are inspected individually. A queued job inside an in-progress run is included. Completed workflows and completed jobs are not retained.
- Queued jobs are shown across all configured repositories, including jobs whose hosted/self-hosted target cannot be established yet. A queued job usually has no assigned runner or group. The group is explicitly Unknown when absent. `runner_group` excludes running assignments with a confirmed different group and filters inventory; missing group metadata does not hide an otherwise identified self-hosted runner assignment; it does not hide unassigned queued jobs or prove which group will execute them. Configured names outside the verified group remain Unknown.
- Queue order is creation time, oldest first, with deterministic ties. Age is approximate time since job creation, **not GitHub's exact scheduling order**. Missing timestamps show Unknown. Running duration uses job start time.
- Requests paginate with 100 entries per page, with up to four repositories fetched concurrently. Each request times out after 25 seconds; a refresh has a two-minute budget. Very large installations can exceed that budget. GitHub's filtered workflow run search has a 1,000-result limit; this initial version is intended for personal-scale monitoring.
- Rate-limit and API failures keep the previous snapshot visible as STALE and retry at the configured interval when automatic refresh is enabled. There is no aggressive retry loop or historical persistence. A refresh is published only after all repository fetches succeed. Normal polling can miss short-lived jobs.
- GitHub.com is supported; GitHub Enterprise hosts are not configurable in this version. Browser launching uses `open`, `xdg-open`, or Windows `rundll32` and accepts GitHub HTTPS URLs only.

## Development

```sh
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

Tests use mocked subprocess responses and render the dashboard at several terminal sizes; no live GitHub credentials are needed. GitHub integration is in `internal/github`, aggregation in `internal/monitor`, TOML handling in `internal/config`, and the Bubble Tea v2 dashboard in `internal/ui`. CLI flags and application wiring live in `cmd/gh-runner-monitor`.

## Binary downloads and Homebrew

Releases provide `.tar.gz` archives for macOS (`darwin`) and Linux, each for
Intel/AMD64 (`amd64`) and ARM64 (`arm64`). Download the matching archive from
the repository's GitHub Releases page and its checksums file. Verify the archive
with `shasum -a 256` on macOS or `sha256sum` on Linux, comparing against the
checksums file, extract it, and place `gh-runner-monitor` on your PATH. Go is not
required to run a downloaded binary; GitHub CLI is still required.

After the first stable release is published to the tap, macOS users can install:

```sh
brew install alexso/tap/gh-runner-monitor
gh auth login --hostname github.com
gh-runner-monitor init
# Edit the config, then run gh-runner-monitor --check
```

The formula installs `gh` as a dependency. These initial binaries are not signed or
notarized with an Apple Developer identity; macOS may require approval under
Privacy & Security for a downloaded executable.

## Publishing a release

Release automation is defined in `.github/workflows/release.yml` and
`.goreleaser.yml`, using GoReleaser v2.20.2. It runs formatting, vet, and tests before
cross-compiling all four targets, injecting the release version, creating
archives with license notices, uploading SHA-256 checksums to GitHub Releases,
and updating `Formula/gh-runner-monitor.rb` in `alexso/homebrew-tap` for stable tags.
Prereleases are uploaded to GitHub but do not update the stable Homebrew formula.
The release repository and download URLs are inferred from the Git remote.

Before the first release:

1. Create the intended GitHub repository and add its URL as this checkout's
   `origin`. If its name differs from `alexso/gh-runner-monitor`, also update the
   Homebrew homepage in `.goreleaser.yml`. The module path is
   `github.com/alexso/gh-runner-monitor`; update it and internal imports if using
   a different repository.
2. Ensure `alexso/homebrew-tap` exists. Add a repository Actions secret named
   `HOMEBREW_TAP_GITHUB_TOKEN` containing a fine-grained token with **Contents:
   read and write** for that tap repository. The workflow's built-in
   `GITHUB_TOKEN` handles releases in the application repository; it cannot
   write to a separate tap repository.
3. Ensure the tap has no existing formula with the same `gh-runner-monitor`
   name, or a cask with that name from an earlier release.

   GoReleaser’s built-in formula publisher (`brews`) is deprecated upstream but
   supported in the pinned v2.20.2 release. Validate formula generation before
   upgrading GoReleaser.

4. Install GoReleaser v2.20.2 locally if you want to validate packaging:

   ```sh
   goreleaser release --snapshot --clean
   ```

   Snapshot releases build local artifacts under `dist/` without publishing.
   This command requires a Git checkout with a commit and a configured remote.
   `goreleaser check` currently exits nonzero for the deprecated `brews` field;
   snapshot generation validates that the formula and archives still build.
5. When ready to publish, commit and push the code, then create and push a
   semantic-version tag, for example:

   ```sh
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

Pushing the tag starts publication. No repositories, tags, releases, or tap
updates are created by ordinary local builds.

## Copyright and license

Copyright (c) 2026 Alex So. Distributed under the [MIT License](LICENSE).
Third-party dependencies retain their respective copyrights and licenses.
