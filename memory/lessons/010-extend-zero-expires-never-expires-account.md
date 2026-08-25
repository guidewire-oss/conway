# Extend(0) on a never-expires account silently expires it

## Lesson

The admin extend endpoint originally read `hours` with a default of 24. Treating
an explicit `{"hours":0}` as "keep as-is" is wrong in a dangerous way:
`Extend(name, 0)` computes `base + 0`. For an active account `base` is the
current expiry (no-op), but for a **never-expires** account (`ExpiresAt == 0`)
the server treats `base` as *now* — so the account that was supposed to live
forever suddenly expires this instant. The live probe that tested explicit zero
against the real `admin` account expired it, and every subsequent request
returned 401 "account revoked or expired".

Two compounding traps while fixing it:

1. **Boot does not restore expiry.** `CONWAY_ADMIN_PASSWORD` resets the admin
   *password* on every boot, which creates the false expectation that a broken
   admin heals on restart. It does not: expiry persists in the store, and the
   server caches users in memory, so the fix required updating the row
   (`UPDATE accounts SET expires_at = 0 WHERE username = 'admin'`) *and*
   restarting the server to reload it.
2. **Values that fit int64-hours still fit int64-seconds.** 999999999999999
   hours is ~3.6e18 seconds ≈ 114 billion years — absurd, but representable in
   the int64 column. The real overflow boundary is
   `(MaxInt64 - base) / 3600` **hours** (the largest hour count whose
   seconds still fit); values beyond int64 itself are rejected earlier by
   JSON unmarshal into `int`.

Guard shape that works: refuse non-positive hours outright (explicit zero is a
request, not a missing field — hence `*int` in the request body), and range-check
the horizon against the remaining int64 seconds before adding.

## Provenance

- Observed 2026-08-24 via live probe on PR #30: `POST
  /api/admin/users/admin/extend` with `{"hours":0}` returned 200, after which
  all authenticated requests returned 401 "account revoked or expired";
  `psql` showed `expires_at = 1787619931` (the probe's timestamp) for admin.
- Fix + spec: `server/server_suite_test.go` ("refuses an explicit zero rather
  than expiring a never-expires account"), server/main.go extend handler
  (commit 55d04cc on `feat/ui-polish-expiry`).
