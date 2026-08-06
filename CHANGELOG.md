# Changelog

All notable changes to this service are documented here. Versioning follows [Semantic Versioning](https://semver.org/): MAJOR for breaking API changes, MINOR for backwards-compatible additions, PATCH for fixes that don't change the API surface.

## [1.0.0] - 2026-08-06

Baseline. Everything merged before this changelog existed: multiple leaderboard types (record/additive/onetime), time-windowed periods including all-time, Redis-cached rankings with Postgres as source of truth, JWT game auth with revocation, graceful shutdown, health probes, and hexagonal architecture. See `docs/swagger.yaml` for the full API surface as of this version.

## [1.0.1] - 2026-08-06

### Fixed
- `GET /leaderboards/{name}/rankings` and the `user_entry` rank it returns now use the same ranking semantic. Previously the paginated list computed rank from list position while `user_entry` computed it as "count of strictly-greater scores + 1" (competition ranking) — the two disagreed whenever any two entries were tied. The list now uses competition ranking throughout, including across page boundaries.
- Postgres-backed `GetRanking` now breaks ties on `user_id ASC` (`ORDER BY score DESC, user_id ASC`). Without a secondary sort key, two separate paginated queries over a tied group had no guaranteed agreement on ordering, so a row could be duplicated across pages or skipped. Matches Redis's `ZREVRANGE`, which already breaks ties on member name.
- `page_size` on `GET /leaderboards/{name}/rankings` is now capped at 100 (previously unbounded). Computing rank now issues one lookup per distinct score in the page, so an unbounded page_size let a single request fan out into an unbounded number of sequential repository calls.

## [1.0.2] - 2026-08-06

### Fixed
- Redis keys for period buckets (`lb:{id}:{n}` and its `:synced` marker) now get a TTL instead of living forever. Every `(leaderboard, period)` pair used to create two permanent Redis keys — for a leaderboard with tens of thousands of boards and daily/weekly periods, that's unbounded memory growth, since Postgres (the source of truth) is never re-consulted to clean anything up. The TTL is set to the period's end time plus a 24h grace window, computed and refreshed on every write, so an active period's bucket effectively never expires while it's still current, and a bucket nobody touches again after its period ends is reclaimed automatically without needing a background sweep. All-time leaderboards (`interval_seconds: 0`) are unaffected — their single bucket is permanently current and is never expired. No HTTP-facing behavior changes; this is purely a cache memory fix.

## [1.1.0] - 2026-08-06

### Added
- `GET /leaderboards` — lists every leaderboard belonging to the authenticated game.
- `PATCH /leaderboards/{name}` — renames and/or redescribes a leaderboard. `unique_name`/`description` are each optional; omit either to keep its current value. Type and `interval_seconds` are not editable here (changing either would reinterpret every score already recorded under the old semantics) — create a new leaderboard instead. Returns `409` if the new name is already taken by another leaderboard in the same game.
- `DELETE /leaderboards/{name}` — permanently deletes a leaderboard and all of its scores across every period. Postgres deletion cascades via foreign key; the Redis cache for every period bucket the leaderboard ever had is explicitly cleared via `SCAN`, since Redis has no equivalent cascade.
- `PATCH /leaderboards/{name}/scores/{user_id}` — directly overwrites a user's score for one period, bypassing the leaderboard type's normal record/additive/onetime processing. For organizer corrections ("I mistyped this score"), not game clients reporting an honest attempt. Accepts `duration_index` as an optional query param (defaults to the current period, matching `GET .../rankings`'s convention).
- `DELETE /leaderboards/{name}/scores/{user_id}` — removes a user's score for one period (default: current). Returns `404` if no score exists there. Use to correct accidental submissions or remove a participant.

These close the gap called out in the design review: organizers could create leaderboards and submit scores, but had no way to fix a typo, rename a board, remove a participant, or delete a leaderboard entirely without an endpoint. All four are B2C-blocking without this.

## [1.1.1] - 2026-08-07

### Fixed
- `cmd/migrate` read its migration file from a relative filesystem path (`internal/repository/migrations/001_init_schema.sql`), which only resolved when the source tree was present on disk — true under `go run` in a dev checkout, false in the distroless production image, which ships no source tree at all. Migrations are now compiled into the binary via `//go:embed` (`internal/repository/migrations.go`) and `cmd/migrate` reads from that embedded filesystem instead. The Dockerfile now also builds `/migrate` alongside `/server`, so a deployed image can actually run its own migrations (`docker run --rm <image> /migrate`) with no source checkout required. No API surface change.
