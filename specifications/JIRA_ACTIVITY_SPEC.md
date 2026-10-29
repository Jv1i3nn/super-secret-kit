# Jira Activity Export Specification

Status: phase 1 implemented on `feat/jira-structured-output`.
Phases 2–4 remain proposed. The JSON contract is documented in `README.rst`.
The new repository uses `default` as the default workspace; named workspaces in
examples must be configured explicitly.

## Objective

Make SSK a reliable source of structured Jira evidence for reporting and
agent workflows. Consumers should receive complete issue selections,
current issue context, and dated changes without parsing presentation Markdown
or compensating for missing API pagination.

SSK collects and normalizes evidence. Consumers remain responsible for downstream
processing, presentation, and delivery. SSK must not call an LLM or infer
organizational intent.

## Current Gaps

Baseline inspected on 29 October 2026, before the phase 1 implementation:

- `SearchTickets` in `internal/services/jira/client.go` returns a single search
  response. `TicketSearchResponse` lacks the enhanced search pagination fields.
- `cmd/jira_query.go` emits human-readable text grouped by status; inspection
  emits Markdown from `internal/services/briefing.go`.
- Inspection exposes current state and dates, but no complete change history.
- Comments come from the issue's embedded comment field. Sorting this subset
  cannot guarantee that the newest comments have been retrieved.
- `GetBriefingTicketByKey` also fetches development information for every issue,
  even when the consumer only needs Jira activity.
- The Jira HTTP client has no explicit timeout or bounded rate-limit retry.

These limitations force consumers to compensate with repeated exclusion queries,
Markdown parsing, and external orchestration. A JQL test can confirm that a status
changed without exposing the dated transition itself.

## Delivery Order

1. Complete search pagination and JSON output for query/inspect.
2. Paginated changelogs and explicit comment retrieval with period filtering.
3. Compact output controls and reliable request cancellation/retries.
4. A reusable `jira activity` command built on those primitives.

Each phase must be independently usable. Preserve existing command defaults and
Markdown/text output unless a documented correctness fix requires otherwise.

## Implementation Progress

- Phase 1: complete enhanced-search pagination, normalized version-1 JSON for
  query/inspect, preserved text/Markdown defaults, and HTTP/CLI fixture tests.
  All search entry points share the pagination helper. Inspection JSON preserves
  bounded traversal and optional attachment-download results. Embedded comments
  are explicitly incomplete; JSON excludes development-panel data.
- Validation: Linux race-enabled tests, pinned-version lint, vet, module verification, and
  build pass.
  `go mod tidy -diff` cannot finish offline because two upstream test dependencies
  are not cached. macOS and live Jira validation have not been performed.
- Phases 2–4: pending; no changelog/comment-period flags, compact controls,
  cancellation/retry policy, or `jira activity` command are implemented yet.

## Search Pagination

`ssk jira query <jql>` must retrieve every page of matching visible issues before
reporting success. Use the enhanced Jira search endpoint's `nextPageToken` and
completion metadata, rather than offset pagination or growing JQL exclusions.
Do not use an approximate count as the number of retrieved issues.

- Extend the response model to represent pagination explicitly.
- Deduplicate by Jira issue ID within a workspace. Preserve deterministic order.
- Detect repeated tokens, non-progressing pages, and inconsistent termination.
- The operational cap is 100 pages, requesting up to 100 issues per page. If
  more pages remain at the cap, fail explicitly and advise narrowing the JQL;
  never silently truncate the result. A completed 100th page succeeds.
- A failure on a later page fails the command. Do not print an apparently
  successful partial JSON document or text listing.
- A successful empty search returns an empty collection and count zero.
- Count actual distinct retrieved issues after pagination.

The existing status-grouped text rendering remains the default. JSON preserves
search order rather than inheriting the presentation grouping.

## Structured Output

Add `--format json` to `jira query` and `jira inspect`; keep their current output
as the default. The proposed activity command also supports JSON and Markdown.

JSON must be one valid UTF-8 document on stdout. Diagnostics go to stderr.
Build the result before emitting it. JSON mode must not include Markdown wrappers,
agent instructions, progress messages, or raw unfiltered Jira responses.

Use a normalized, versioned contract with:

- `schema_version`: initially `1`; incompatible changes require a version change.
- `kind`: `jira.query`, `jira.inspect`, or `jira.activity`.
- Workspace name and site URL, collection start/end timestamps, and effective JQL.
- For activity, explicit reporting period and inclusion semantics.
- Retrieved issue count, selection completeness, and structured warnings.
- Issues identified by workspace, Jira issue ID, and key; never key alone across
  workspaces. Include a browser URL derived from the configured Jira site.
- Structured summary, project, status ID/name/category, resolution ID/name,
  creation/update/resolution dates, assignee, parent, subtasks, issue links,
  components, labels, and fix versions when available.
- Normalized plain-text descriptions/comments using existing ADF conversion.
- Dated changes and comments when requested, with stable source identifiers.

Preserve absent/unknown values as `null`; use empty arrays for known empty
collections. An unknown value must not become an invented default. Omit tokens,
credentials, unnecessary user email addresses, and other authentication data.

Distinguish upstream retrieval completeness from deliberate output limits. For
example, `retrieval_complete: true, output_truncated: true` means all relevant
source pages were read but the caller requested a compact excerpt. An empty
comment list with `retrieval_complete: false` must not imply no discussion.

## Dated Change History

Proposed inspection options:

```bash
ssk jira inspect PROJ-123 --depth 0 --max-related 0 \
  --changes-since 2026-10-21T10:00:00Z \
  --changes-until 2026-10-28T11:00:00Z --format json
```

- Request and paginate issue changelogs; an expanded issue response alone must
  not be assumed to contain complete history.
- Represent each change with changelog ID, timestamp, author when available,
  field ID/name, previous value ID/text, and new value ID/text.
- Include status and resolution changes at minimum. Preserve available changes
  to assignee, parent, priority, and other fields without inventing missing data.
- Normalize timestamps to RFC 3339 with explicit offset, preferably UTC.
- Filter events using `since <= timestamp < until`. Explicit bounds must include
  a timezone, and `since` must precede `until`.
- `--changes-until` requires `--changes-since`. If only the lower bound is given,
  capture the upper bound once at command start and include it in output.
- Retrieval errors or insufficient access fail a request for complete history.
- Bulk changelog fetching may reduce requests, provided access and output are
  equivalent; keep a per-issue implementation available where necessary.

An event can establish “entered review during the period.” Current status alone
cannot establish that date. A resolution event is not evidence of deployment.
Missing or deleted historical content must not be reconstructed or guessed.

## Explicit Comment Retrieval

Proposed inspection options:

```bash
ssk jira inspect PROJ-123 --depth 0 --max-related 0 \
  --comments-since 2026-10-21T10:00:00Z \
  --comments-until 2026-10-28T11:00:00Z \
  --comments-limit 5 --format json
```

Use Jira's paginated issue comments endpoint instead of relying on embedded issue
comments. Preserve ID, author, creation/update timestamps, plain-text body, and
browser link when established by Jira data or documented URL conventions.

- Include comments created or last edited in the requested period; distinguish
  a new comment from an edit. Do not claim to have historical body versions.
- Apply period filtering before limiting. Determine the newest qualifying
  comments by their activity timestamp, with stable ID tie-breaking.
- Retrieve enough pages to establish the result. Creation ordering alone cannot
  prove that an old comment was not edited recently.
- Record retrieval completeness, matching count when established, returned count,
  and whether output selection/body content was truncated.
- Apply the same bound validation/defaulting as changelog options.
- Define `--comments-limit 0` as no output count limit; reject negative values.

“Complete” means complete for the authenticated user's visible source data.
It does not assert access to restricted or deleted comments.

## Compact Context and Request Reliability

Add opt-in inspection controls:

- `--description-lines <n>`: retain the first n normalized text lines.
- `--comments-limit <n>`: retain the newest n qualifying comments.
- `--comment-chars <n>`: cap each retained comment body by Unicode characters.
- `--include-development=false`: skip development-panel requests entirely.

For excerpt controls, zero means no explicit caller limit; reject negatives.
Existing internal safety limits still apply and must be disclosed through
truncation metadata. Preserve current default behavior when flags are omitted.
Keep relationship metadata even when traversal is disabled.

Pass `context.Context` through requests, use explicit HTTP timeouts, and cancel
outstanding work when the command is interrupted or a required fetch fails.
For read-only retrieval, use bounded retries for transient network failures,
rate limits, and appropriate server errors; respect `Retry-After` within the
overall deadline. Do not retry authentication, authorization, or invalid queries.
Do not extend this retry policy to comment creation or other mutations.

Verbose logs may include issue keys, request counts and sanitized diagnostics;
they must not contain credentials or raw descriptions/comment bodies.

## Activity Command

After the primitives are available, add:

```bash
ssk jira activity \
  --since 2026-10-21T10:00:00Z --until 2026-10-28T11:00:00Z \
  --workspace somfy-connected --format json \
  --description-lines 10 --comments-limit 5 --comment-chars 2000 \
  --include-development=false --workers 6
```

This command selects visible issues potentially active in the period, enriches
them with current context, dated changes and comments, then emits one document.
Reuse existing workspace resolution and credentials. One invocation targets one
workspace initially; callers can aggregate multiple explicitly selected spaces.

- Require `--since`; default `--until` once at command start. Default format is
  deterministic Markdown. Reject invalid bounds before network requests.
- Use the same period for changes and comments. The activity command does not
  accept independently conflicting per-source period options.
- Fetch concurrently with bounded workers; default 6, allowed range 1–32.
- Keep deterministic output despite concurrent completion: issues by current
  update timestamp descending with key/ID ties; events chronologically with
  stable source ID ties. Include the effective ordering in the contract.
- Return current issue state separately from events occurring in the period.
- A candidate search must not exclude an issue merely because its latest update
  occurred after `until`: it may also contain events inside the requested period.
  Prefer a broad candidate selection followed by exact client-side event filtering.
- Document Jira's JQL timezone/precision semantics and translate bounds correctly;
  do not relabel timezone-less JQL timestamps as UTC without conversion.
- An updated candidate without an identifiable event may remain in the export,
  explicitly marked as having no detailed activity evidence available.
- Successful zero-result exports are valid. Required source failures fail the
  entire command, with no successful partial JSON on stdout.

This is not a historical issue snapshot or transactional Jira export. Current
fields reflect collection time. Completeness is relative to the documented
selection and visible APIs, not a guarantee of recovering deleted issues,
removed comments, or every historical organizational event.

Include separate summaries for current status counts and dated activity:
created issues, issues with status changes, transition-event counts, new comments,
and edited comments. Label units explicitly and deduplicate by source identity.
Expose native status categories and resolution values; consumer-specific
“review” and “cancelled” groupings remain caller policy, not guessed name mappings.

## Acceptance Criteria

Use HTTP fixtures and CLI tests; no live Jira account is required for CI.

1. Search spans several pages, including an empty result and repeated issue IDs;
   counts match the distinct returned collection. Repeated tokens and later-page
   failures do not yield successful partial output.
2. Existing query text and inspection Markdown behavior remains compatible.
   JSON is parseable, versioned, deterministic, and free of diagnostics.
3. Multiple transitions of one issue preserve dates, IDs and from/to values;
   an unchanged current status does not erase intermediate transitions.
4. Events at `since` are included and events at `until` excluded, including
   timezone-offset and daylight-saving boundary cases.
5. An issue updated after `until` can still contribute an earlier qualifying
   event; the export does not imply historical reconstruction of current fields.
6. Comment pagination finds a recent comment beyond the embedded subset and an
   old comment edited during the period. Output limits are applied after filtering,
   with truncation and completeness recorded separately.
7. Empty and unavailable data are distinguishable. Missing dates are not invented.
8. Description limits preserve valid Unicode; issue links and parents survive
   compact rendering. Development enrichment can be skipped without requests.
9. Bounded concurrency produces stable results. Cancellation, request timeout,
   rate-limit retry exhaustion, and access failures terminate predictably.
10. Statistics distinguish current states, distinct issues, and event counts.
    Test one issue with several transitions, including reopenings and cancellation.
11. Consumers can identify issues across workspaces without key collisions, and
    output/logs contain no credential values.

## References

- Current [Jira command documentation](../README.rst).
- [Atlassian enhanced JQL search and pagination](https://confluence.atlassian.com/jirakb/how-to-return-issues-from-a-jql-query-using-rest-api-1289424308.html).
- [Jira issue changelog API, including bulk retrieval](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/).
- [Jira issue comments API](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-comments/).
