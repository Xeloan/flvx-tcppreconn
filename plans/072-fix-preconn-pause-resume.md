# 072 - Fix preconn service pause/resume ("service X not found")

## Problem

When TCP pre-connection (tcpPreconn) is enabled for a forward, the gost service
registry contains no entry for that forward (the `tcp_pool` process handles it
instead).  Toggling the rule switch calls `pauseServices` / `resumeServices`,
which looks up the service in the gost registry → `nil` → returns "service X
not found".

Two root causes:

1. `createServices` / `updateServices` do **not** persist preconn service
   configs into `config.Global()` — so there is no config to inspect later and
   no way to restart `tcp_pool` on resume.
2. `pauseServices` / `resumeServices` unconditionally fail when the service is
   absent from the gost registry, even for legitimate preconn-managed services.

## Fix

### `service.go` changes

- **`createServices`** — after `handlePreconnServices` identifies and launches
  preconn services, call `config.OnUpdate` to append those configs to the
  global config (same way gost services are stored).
- **`updateServices`** — same, but upsert (update existing or append new).
- **`pauseServices`**:
  - Move `serviceConfigs` map construction to before the validation loop.
  - In the validation loop: when `svc == nil`, check if the global config has
    a matching entry with `tcpPreconn=true`; if so, add to `servicesToPause`
    with `svc = nil` (preconn was already stopped by the leading loop) instead
    of returning an error.
  - In the phase-2 pause loop: only call `svc.Close()` when `svc != nil`.
- **`resumeServices`**:
  - In the validation loop: when `svc == nil`, check if the global config has
    a matching entry with `tcpPreconn=true`; if so, restart `tcp_pool` via
    `PreconnManager.StartPreconn` and track in `servicesToResume` with
    `svc = nil`.
  - In the phase-2 resume loop: when `svc == nil` and it is a preconn service,
    call `mgr.StartPreconn` instead of gost re-parse/register/serve.
- **`rollbackPausedServices`**: when `pss.service == nil` and
  `isPreconnService`, restart `tcp_pool` instead of trying to parse/register a
  gost service.
- **`rollbackResumedServices`**: when `rss.service == nil` and
  `isPreconnService`, stop `tcp_pool` and re-mark paused in config.

## Checklist

- [x] Create plan document
- [x] Fix `createServices` / `updateServices` config storage
- [x] Fix `pauseServices`
- [x] Fix `resumeServices`
- [x] Fix rollback helpers
- [x] Build and verify
