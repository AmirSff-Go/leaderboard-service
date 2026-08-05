# Changelog

All notable changes to this service are documented here. Versioning follows [Semantic Versioning](https://semver.org/): MAJOR for breaking API changes, MINOR for backwards-compatible additions, PATCH for fixes that don't change the API surface.

## [1.0.0] - 2026-08-06

Baseline. Everything merged before this changelog existed: multiple leaderboard types (record/additive/onetime), time-windowed periods including all-time, Redis-cached rankings with Postgres as source of truth, JWT game auth with revocation, graceful shutdown, health probes, and hexagonal architecture. See `docs/swagger.yaml` for the full API surface as of this version.

## [1.0.1] - 2026-08-06

### Fixed
- `GET /leaderboards/{name}/rankings` and the `user_entry` rank it returns now use the same ranking semantic. Previously the paginated list computed rank from list position while `user_entry` computed it as "count of strictly-greater scores + 1" (competition ranking) — the two disagreed whenever any two entries were tied. The list now uses competition ranking throughout, including across page boundaries.
- Postgres-backed `GetRanking` now breaks ties on `user_id ASC` (`ORDER BY score DESC, user_id ASC`). Without a secondary sort key, two separate paginated queries over a tied group had no guaranteed agreement on ordering, so a row could be duplicated across pages or skipped. Matches Redis's `ZREVRANGE`, which already breaks ties on member name.
- `page_size` on `GET /leaderboards/{name}/rankings` is now capped at 100 (previously unbounded). Computing rank now issues one lookup per distinct score in the page, so an unbounded page_size let a single request fan out into an unbounded number of sequential repository calls.
