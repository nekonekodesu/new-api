# Public fork migration

## Baseline

- Upstream: https://github.com/QuantumNous/new-api
- Base release: `v1.0.0-rc.51`
- Base commit: `c9c6da2786b6a0912215127e78e28fbc6650c85f`
- The upstream baseline and tag have been pushed before feature migration.
- Local modifications are reviewed as individual feature increments. Each verified increment is committed and pushed separately.

## Data sanitization

Deployment credentials, private URLs, personal paths, local agent connection instructions, scratch deployment scripts, databases, and compiled binaries must not be copied from the source checkout into release commits. Runtime values belong in environment variables or administrator settings; examples use placeholders. Reports must record locations and remediation without reproducing secret values.

The fork ignores local environment variants, scratch files, agent session artifacts, private connection skills, and local Linux executables. Ignore rules prevent accidental additions of untracked files; they do not scan tracked content or remove secrets from history. Review staged changes before every push.

## Migration status

| Increment | Status |
| --- | --- |
| Upstream baseline | Pushed |
| Public repository exclusions and migration record | Implemented |
| Source feature inventory and sensitive-value review | In progress |
| Dashboard hierarchy and cost / price synchronization management | Pending |
| Scheduled task interaction and extension boundaries | Pending |
| Configurable currencies and payment method exchange rates | Pending |
| Taiwan payment integrations | Pending |
| HTTP preview and responsive verification | Pending |

## Findings requiring correction during migration

- The source currency implementation routes `NTD` through the USD-to-CNY rate. Currency selection must resolve its own configured rate; displaying a different symbol does not convert the currency correctly.
- The source replaces the backend fallback version with a deployment-specific release string. Release versions should come from build metadata.

## Release checks

Each feature increment records its validation and remaining limitations. Payment work requires amount and currency verification, authenticated callbacks, duplicate notification handling, and an immutable order conversion snapshot. Database changes require fresh-install and upgrade verification on SQLite, MySQL, and PostgreSQL. Frontend changes require relevant tests, type checking, lint, build, and desktop/mobile review before a feature release is described as complete.

No feature release has been published yet.

### Frontend management surfaces

Migrated the channel management API extensions and management surfaces for cost control, model price tables, upstream synchronization, scheduled tasks, channel classification, model health, network behavior, and configurable NTD display support. Existing plugin login and balance-task API contracts were restored when the feature batch initially replaced them.

Validation: `bun run typecheck` passed, `bun run build` passed, and targeted Oxlint over the changed `web/src` files passed. The repository-wide lint command still reports pre-existing errors in unrelated baseline files; those are kept separate from this increment. Locale JSON files and home-page branding changes were not included in this increment.

## Verified increments

### Channel balance response bounds

Migrated the source checkout's response size limit into the shared channel balance HTTP reader, reusing the existing 256 KiB balance response limit. Responses above that limit fail instead of being read without a bound. Response bodies now close on every return path, including non-200 responses and read failures.

Validation: `go test ./controller -run 'TestGetResponseBodySizeLimit' -count=1` passed. The regression cases cover acceptance at the limit and rejection above it. `git diff --check` passed. This increment does not change database schemas or balance conversion rules.
