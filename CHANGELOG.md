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
