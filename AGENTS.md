# Contributor and coding-agent guidance

These instructions apply throughout this repository. Read any more specific
`AGENTS.md` before editing the files it governs. The README is the reference for
installation, configuration, and CLI usage.

## Project and layout

Super Secret Kit (`ssk`) is a Go CLI integrating with external services.
Use the Go version declared in `go.mod`.

- `cmd/`: Cobra commands, flags, configuration wiring, and CLI output.
- `internal/services/`: application logic and external-service integrations.
- `internal/config/`: configuration loading and persistence.
- `internal/models/`, `internal/repository/`, `internal/database/`: local data
  models, storage access, and database initialization.
- `internal/utils/`: shared utilities.
- `scripts/` and `Makefile`: installation, builds, and packaging.
- `.github/workflows/`: CI and release automation.

Keep service logic separate from command parsing. Follow existing conventions
and consult nearby tests before introducing a new pattern.

## Scope and compatibility

- Make focused changes that directly address the issue or requested behavior.
  Avoid unrelated refactors, formatting, and dependency upgrades.
- Explain material assumptions and tradeoffs. Ask when ambiguity changes the
  expected behavior or compatibility requirements.
- Preserve existing CLI flags, configuration, stored credentials, and encrypted
  data unless a breaking change is explicitly part of the task.
- For storage or encryption changes, test reading historical data and document
  migration behavior, rollback limits, and any compatibility break.
- Document changes to commands, configuration, installation, and supported
  platforms in `README.rst`.

## Secrets and external services

- Never commit or print real credentials, tokens, passphrases, personal
  documents, or local account configuration.
- Use fictitious identities such as John Doe and reserved example domains in
  documentation and tests. Keep organization-specific service URLs configurable.
- Do not read or modify a contributor's real credential store to test code.
  Use temporary directories, test configuration, and local HTTP servers.
- External reads must be relevant to the task and authorized. Obtain explicit
  authorization before posting comments, creating worklogs, declaring absences,
  downloading personal documents, or modifying credentials on real accounts.
  Use a dry-run first when available.
- Do not change global tool configuration, active accounts, or authentication
  settings as part of repository work.

## Validation

Use `.golangci.yml` and the golangci-lint version pinned in the CI workflow.
For code changes, run the relevant tests while developing, then the CI checks:

```sh
make lint
go mod verify
go mod tidy -diff
go vet ./...
CGO_ENABLED=1 go test -race -count=1 -coverprofile=coverage.out ./...
make VERSION=ci build
```

The race detector requires a supported Go toolchain and a C compiler. CI runs
tests and builds on Linux and macOS; verify both before declaring a PR ready.

For bug fixes, add a regression test that reproduces the actual failure before
applying the fix. Keep tests independent of the real hostname, credentials,
network services, and current date unless those are explicitly under test.
Use explicit fixtures or controlled inputs for platform-dependent behavior.

For documentation-only changes, review formatting, links, and command examples;
do not add tests that merely restate the documentation. Report which checks were
actually run and any checks that could not be completed. Never claim a check
passed without observing its result.

## Branches and pull requests

Work on a dedicated branch and submit a PR to `main`. Do not push directly to
`main`, bypass its protections, or merge a PR without authorization. Keep each
PR focused on one problem; reference its issue when one exists.

Use a clear title describing the final behavior. Update the title and body if
the scope changes. The PR description must use these three sections, also
provided by `.github/pull_request_template.md`:

```markdown
## Problème
Describe the concrete trigger and observed behavior, with an issue link or
before/after example when useful.

## Choix
Explain the decision and relevant tradeoffs, including alternatives only when
they help a reviewer understand the choice.

## Solution / tests
Describe the resulting implementation and affected components. Give the checks
actually run, their results, and any compatibility or migration limitations.
```

Keep each section concise and useful to a reviewer who has not read the
conversation. Avoid a chronological account of the work or a file-by-file tour.
Do not mark pending CI checks as successful. Required lint, Linux, and macOS
checks must pass, and the branch must be up to date before merging.
