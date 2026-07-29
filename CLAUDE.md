# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A terminal UI (Bubble Tea / Elm architecture) for browsing and editing ServiceNow records via the Table API and Aggregate (Stats) API. Single Go binary, no backend of its own — it talks directly to a configured ServiceNow instance.

## Commands

```bash
go build -o snowtui .     # build
go vet ./...              # static checks
go mod tidy                # sync go.mod/go.sum after dependency changes
./snowtui [config.yaml]    # run; defaults to ./config.yaml if no path given
```

There is no test suite (`*_test.go` files do not exist yet); `go build` and `go vet` are the only current correctness checks. If you add tests, standard `go test ./...` applies.

Runtime requires a `config.yaml` (gitignored, copy from `config.yaml.example`) with at least one instance. This is a real ServiceNow connection — do not fabricate or guess instance URLs/credentials, and don't assume the contents of the local `config.yaml`.

Since this is a TUI, verifying a UI change means actually driving the program (Bubble Tea apps can't be meaningfully checked by reading code alone) — e.g. run it in a real terminal, or drive it headlessly via a PTY + keystroke script if one is available, rather than declaring a fix correct from a diff.

## Architecture

### Navigation: one root state machine, dumb sub-views

`ui/app.go`'s `AppModel` is the only `tea.Model` registered with Bubble Tea. It holds a `viewState` enum (`stateInstanceSelect` → `stateTableSelect` → `stateRecordList` ⇄ `stateGroupView` / `stateRecordDetail`) and one sub-model per state (`InstanceView`, `TableView`, `ListView`, `GroupView`, `DetailView`, in `ui/*_view.go`).

Sub-views never navigate directly or know about each other. Instead they emit intent as typed `tea.Msg` values (defined in `ui/messages.go`: `instanceSelectedMsg`, `tableSelectedMsg`, `recordSelectedMsg`, `newRecordRequestedMsg`, `groupViewRequestedMsg`, `groupSelectedMsg`, `goBackMsg`, ...). Only `AppModel.Update`'s top-level switch reacts to these, mutates `state`, and constructs/initializes the next sub-view. When adding a new screen or cross-view transition, follow this pattern rather than reaching into another view's fields.

`AppModel` also carries state that's inherently cross-view: `listFromGroup`, `groupFilterLabel`, and `savedListView` exist purely so "esc" from a group-filtered list returns to the GroupView (and restores the pre-group `ListView`) instead of falling through to table selection. This bookkeeping lives in `app.go`, not in `ListView`/`GroupView` themselves.

### Each sub-view is a small local state machine

Every `*_view.go` file repeats the same shape: a `Model` struct, a private `<name>Mode` enum for what that view is currently doing (e.g. `ListView`'s `listLoading`/`listBrowsing`/`listSearching`/`listConfirmDelete`/`listError`, `DetailView`'s `detailReady`/`detailEditingField`/`detailConfirm`/`detailSaving`/...), and an `Update`/`View` that switches on that mode. Follow this local-mode-enum idiom for new view behavior instead of adding ad hoc booleans.

All network I/O is dispatched as a `tea.Cmd` closure that captures the needed fields (client, table, query, ...) by value before returning, so the async call never touches the view struct directly — it comes back as a message (`recordsLoadedMsg`, `groupStatsLoadedMsg`, `errMsg`, `mutationErrMsg`, `recordSavedMsg`, `recordDeletedMsg`) that `Update` applies on the main loop. Mutations (create/update/delete) report failure via `mutationErrMsg` specifically so the current form/list stays on screen instead of being torn down like a fatal `errMsg` load failure.

### `api/client.go`

One `Client` per selected instance (constructed in `app.go` on `instanceSelectedMsg`), authenticated with either the `x-sn-apikey` header or HTTP Basic — never both (`APIKey` takes precedence when set). Every failed or non-2xx response is dumped verbatim (including full HTTP response) to `snowtui.log` via `dumpErrorToLog` — that log is the first place to check when an API call misbehaves, since the TUI itself only surfaces a short error string.

Two error-handling styles currently coexist: `GetRecords`/`GetRecord`/`GetGroupStats` switch on `resp.StatusCode` inline, while `CreateRecord`/`UpdateRecord`/`DeleteRecord` share `parseAPIError`, which parses ServiceNow's `{"error":{"message","detail"}}` JSON body. Match whichever style the function you're editing already uses.

ServiceNow-specific behavior baked into the client (see README's "API Reference" section for the full endpoint/param/header table):
- Reads use `sysparm_display_value=true`; `GetGroupStats` instead uses `sysparm_display_value=all`, which is required (not `true`) so `groupby_fields` includes both `value` (used to build the drill-down query) and `display_value` (used as the label) — `true` collapses them into just the display text.
- Writes additionally set `sysparm_input_display_value=true` so submitted field values are interpreted as display values, matching what the UI shows/edits.
- `GetGroupStats` (the `/api/now/stats` endpoint) needs the `stats_api` or `itil` role on some instances; missing it surfaces as a plain 403.
- Pagination totals come from the `X-Total-Count` response header, not the JSON body.

### `config/config.go`

Loads/validates `config.yaml`: a list of `Instance` (name + url + either `api_key` or `username`+`password`; `active: false` hides one without deleting it) and a list of `TableConfig` (list columns + detail-view "important fields"). Tables not present in config still work end-to-end — `ListView` falls back to the first 6 fields of the loaded records as columns, and `DetailView` always shows *every* field of a record regardless of `important_fields`, which only controls what's called out at the top.

### Rendering helpers (`ui/styles.go`)

`getStringValue(record, key)` is the one correct way to read a display string out of a ServiceNow record map anywhere in `ui/` — ServiceNow returns fields either as a plain string or as `{"display_value": ..., "value": ...}` depending on query params, and this function normalizes both (plus numbers/bools/nil). Use it instead of a direct map index when rendering a field.

`ui/list_view.go`'s shift-key column sorting relies on `sortKeyMap()` mapping the *unique* first letter of visible columns to shift-shortcuts, and marks that letter by appending a Unicode combining underline (`underlineCombining`) rather than an ANSI style — necessary because `bubbles/table` truncates/measures cell content without understanding ANSI escapes, but does treat combining marks as zero-width.

### Comment language

Most of the codebase is being migrated from German inline comments/error strings to English (e.g. `api/client.go`'s `GetRecords`/`GetGroupStats`/`newRequest`/`doRequest` still have German doc comments and/or German error text like `"Antwort dekodieren"`, while `CreateRecord`/`UpdateRecord`/`DeleteRecord`/`parseAPIError` are fully English). Write new comments and error strings in English; leave untouched German comments as-is unless you're already modifying that function.

## Known issues (from README, still open)

- Top header/UI layout can break if a confirmation dialog is answered with "no".
- List view column sort doesn't work for some fields.
- Layout assumes terminal width ~115 columns; other widths can look buggy (several views use fixed pixel widths, e.g. `artPaneWidth`/`minSplitWidth` in `instance_view.go`, `keyW` in `detail_view.go`).
