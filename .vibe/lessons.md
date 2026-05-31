# Lessons Learned

Review this file at the start of every session. Update it after ANY correction from the user.
Write rules for yourself that prevent the same mistake. Ruthlessly iterate until mistake rate drops.

## Format

**Pattern**: [what happened]
**Rule**: [what to do / not do going forward]
**Why**: [root cause or user reasoning]

---

## Active Lessons

**Pattern**: `celery_worker.py` and `lean_runner.py` both define `_UUID_RE` independently. In Wave 6, `celery_worker` had `re.IGNORECASE` while `lean_runner` did not — the celery layer accepted uppercase UUIDs that the lean layer then rejected with `ValueError`, leaving jobs stuck in `running` state.
**Rule**: When modifying UUID validation in either `python/celery_worker.py` or `python/lean_runner.py`, verify both modules define `_UUID_RE` identically. Canonical form: `re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")` (lowercase, no IGNORECASE).
**Why**: PostgreSQL `gen_random_uuid()` always emits lowercase, so IGNORECASE adds no value and risks a cross-module mismatch that silently corrupts job state.

---

**Pattern**: `go` is not on PATH in this dev environment. `go test ./go-app/handlers/` from the repo root fails with "cannot find main module."
**Rule**: For any Go command in this repo, use `/usr/local/go/bin/go` and run from the `go-app/` directory: `cd go-app && /usr/local/go/bin/go test ./handlers/ -v`.
**Why**: Go binary lives at `/usr/local/go/bin/go` and is not on PATH; the module root is `go-app/`, so package paths like `./handlers/` only resolve from inside that directory.

---

<!-- Add new lessons above this line. Remove lessons that have been internalized into config files. -->
