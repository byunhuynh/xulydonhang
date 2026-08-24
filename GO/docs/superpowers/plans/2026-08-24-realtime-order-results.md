# Realtime Order Results Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stream every vendor's completed or provisional PO result to the frontend immediately, safely finalize batched Excel writes, and replace the native JIT period selector with an app-styled menu.

**Architecture:** `RealProcessor` exposes an optional streaming entry point while retaining `Processor.Process`. Rows carry a stable `ResultKey`; the frontend upserts events so provisional and final emissions update in place. JIT and BigC batch Excel per PDF and finalize streamed rows after persistence, while other vendors emit final rows as their existing per-segment handlers return.

**Tech Stack:** Go 1.26, Wails v2, excelize v2, React 19, TypeScript 5.6, Zustand, Node 24 test runner, Tailwind CSS.

**Spec:** `docs/superpowers/specs/2026-08-24-realtime-order-results-design.md`

## Global Constraints

- Preserve the existing `Processor` interface for mocks and callers that do not stream.
- A repeated `resultKey` updates an existing frontend row and never increments STT or changes row order.
- JIT and BigC write Excel once per PDF; a failed combined write finalizes every affected provisional row as failed.
- Other vendors retain their established Excel-writing behavior and golden-fixture output.
- JIT never performs price reconciliation and keeps its period menu disabled during processing.
- Do not introduce a new frontend framework or external dependency.

---

### Task 1: Stable result identity and frontend upsert

**Files:**
- Modify: `internal/processing/types.go`
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/store/appStore.ts`
- Modify: `frontend/src/hooks/useWailsEvents.ts`
- Create: `frontend/src/lib/orderRowUpsert.ts`
- Create: `frontend/src/lib/orderRowUpsert.test.ts`
- Modify: `frontend/package.json`

**Interfaces:**
- Produces: `OrderRow.ResultKey string` / `OrderRow.resultKey: string`.
- Produces: `upsertOrderRow(rows: OrderRow[], incoming: OrderRow): OrderRow[]`.
- Produces: Zustand action `upsertRow(row: OrderRow): void`.

- [ ] **Step 1: Write the failing frontend test**

```ts
test('upsertOrderRow appends a new key and replaces an existing key in place', () => {
  const first = row('pdf|2/4|PO1', 'processing')
  const second = row('pdf|3/4|PO2', 'done')
  const final = row('pdf|2/4|PO1', 'done')
  assert.deepEqual(upsertOrderRow([first, second], final), [final, second])
})
```

- [ ] **Step 2: Run the test and verify RED**

Run: `cd frontend && npm.cmd test`

Expected: FAIL because `orderRowUpsert.ts` or `upsertOrderRow` does not exist.

- [ ] **Step 3: Implement identity and pure upsert**

```go
ResultKey string `json:"resultKey"`
```

```ts
export function upsertOrderRow(rows: OrderRow[], incoming: OrderRow): OrderRow[] {
  const index = rows.findIndex((row) => row.resultKey === incoming.resultKey)
  if (index < 0) return [...rows, incoming]
  const next = [...rows]
  next[index] = incoming
  return next
}
```

Change `process:row` handling to call `upsertRow`; keep `appendRow` only if another caller still needs unconditional append.

- [ ] **Step 4: Run frontend tests and build**

Run: `cd frontend && npm.cmd test && npm.cmd run build`

Expected: all Node tests pass and Vite production build succeeds.

- [ ] **Step 5: Commit the task**

```powershell
git add GO/internal/processing/types.go GO/frontend/src/types.ts GO/frontend/src/store/appStore.ts GO/frontend/src/hooks/useWailsEvents.ts GO/frontend/src/lib/orderRowUpsert.ts GO/frontend/src/lib/orderRowUpsert.test.ts GO/frontend/package.json
git commit -m "feat: upsert streamed order results"
```

### Task 2: Optional streaming processor contract and App event plumbing

**Files:**
- Modify: `internal/processing/processor.go`
- Modify: `internal/processing/coop_processor.go`
- Modify: `app.go`
- Modify: `app_test.go`
- Modify: `internal/processing/processor_test.go`

**Interfaces:**
- Consumes: `OrderRow.ResultKey` from Task 1.
- Produces: `type StreamingProcessor interface { ProcessStreaming(context.Context, string, int, func(OrderRow)) ([]OrderRow, error) }`.
- Produces: `RealProcessor.ProcessStreaming` and a shared private `process(..., emit func(OrderRow))`.

- [ ] **Step 1: Write failing App streaming tests**

Create a fake streaming processor that invokes `emit(processing.OrderRow{ResultKey: "a"})`, returns rows with keys `a` and `b`, then assert emitted `process:row` events contain `a` once and `b` once. Add a non-streaming fake assertion proving its returned rows still emit normally.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test . -run TestApp_RunBatchStreamsRowsWithoutDuplicates -count=1 -v`

Expected: FAIL because `StreamingProcessor` and streaming dispatch do not exist.

- [ ] **Step 3: Implement streaming dispatch**

```go
type StreamingProcessor interface {
    ProcessStreaming(ctx context.Context, filePath string, stt int, emit func(OrderRow)) ([]OrderRow, error)
}
```

In `runBatch`, maintain `streamed := map[string]bool{}`. The callback emits `SkuLog`, then `process:row`, and records the key. After processing returns, emit only rows whose non-empty key was not streamed. Assign a deterministic fallback key before emission if a legacy row omitted it.

- [ ] **Step 4: Implement `RealProcessor.ProcessStreaming` without changing behavior yet**

Make `Process` call the shared implementation with `nil`; make `ProcessStreaming` pass the callback. Initially the shared implementation may emit rows only at existing return boundaries so the contract is testable before vendor loops are changed.

- [ ] **Step 5: Run App and processor tests**

Run: `go test . ./internal/processing -run 'TestApp_RunBatchStreams|TestProcessor' -count=1`

Expected: PASS with no duplicate row events.

- [ ] **Step 6: Commit the task**

```powershell
git add GO/app.go GO/app_test.go GO/internal/processing/processor.go GO/internal/processing/processor_test.go GO/internal/processing/coop_processor.go
git commit -m "feat: add realtime processor event contract"
```

### Task 3: Stream existing per-segment vendors

**Files:**
- Modify: `internal/processing/coop_processor.go`
- Modify: `internal/processing/coop_processor_test.go`

**Interfaces:**
- Consumes: shared `process(..., emit func(OrderRow))` from Task 2.
- Produces: helper `emitOrderRow(emit func(OrderRow), row OrderRow) OrderRow` that assigns `ResultKey` and invokes the callback nil-safely.

- [ ] **Step 1: Write a failing multi-segment streaming test**

Use an existing multi-page fixture and a callback collecting rows. Assert callback length increases after each segment and matches returned key order. Include one failed segment and assert it is streamed immediately as `failed`.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/processing -run TestRealProcessorStreamsEachCompletedSegment -count=1 -v`

Expected: callback remains empty until final return.

- [ ] **Step 3: Emit rows at every append site in the common vendor loop**

Assign keys with:

```go
func orderResultKey(fileName, page, po string) string {
    return fileName + "|" + page + "|" + po
}
```

Invoke the callback immediately after each vendor handler returns or each failure row is constructed. Do not change vendor Excel builders.

- [ ] **Step 4: Run focused and golden vendor tests**

Run: `go test ./internal/processing -run 'TestRealProcessorStreamsEachCompletedSegment|GoldenFixtures' -count=1`

Expected: streaming test and existing golden fixtures pass, except any already documented unrelated baseline failures must be reported exactly.

- [ ] **Step 5: Commit the task**

```powershell
git add GO/internal/processing/coop_processor.go GO/internal/processing/coop_processor_test.go
git commit -m "feat: stream completed vendor segments"
```

### Task 4: Batch JIT Excel writes and stream provisional/final rows

**Files:**
- Modify: `internal/processing/jit_airway_processor.go`
- Modify: `internal/processing/jit_airway_processor_test.go`
- Modify: `internal/processing/jit_airway_all_samples_test.go`

**Interfaces:**
- Consumes: streaming callback and `ResultKey` from Tasks 1–2.
- Produces: JIT rows emitted first with `StatusKindProcessing`, then with final status and absolute `ExcelRows`.

- [ ] **Step 1: Write failing JIT streaming tests**

Use a synthetic two-page JIT fixture or extracted page-text seam. Capture callback events and assert keys appear in this order: `page1 processing`, `page2 processing`, `page1 done`, `page2 done`. Assert the workbook receives all product rows from one combined write and final `ExcelRows` are absolute.

- [ ] **Step 2: Add a failing write-error test**

Use an invalid/locked output path after provisional parsing. Assert both provisional keys receive final `failed` updates and no row remains `processing`.

- [ ] **Step 3: Run and verify RED**

Run: `go test ./internal/processing -run 'TestJITStreaming|TestJITCombinedWriteFailure' -count=1 -v`

Expected: no provisional events and/or multiple per-page workbook writes.

- [ ] **Step 4: Refactor JIT to one combined write**

Accumulate per-page structures containing `OrderRow`, `[]excelwriter.Row`, and totals. Emit provisional rows after validation. Flatten successful Excel rows, write once, translate each page's relative offsets using the returned `startRow`, then emit final rows. Immediate parse/mapping/price failures emit only one final failed event.

- [ ] **Step 5: Run all JIT tests and real-sample audit**

Run: `go test ./internal/processing -run 'TestJIT|TestParseJIT|TestRealProcessor.*JIT' -count=1 -v`

Expected: 525 pages and 536 product lines audited, streaming tests pass.

- [ ] **Step 6: Commit the task**

```powershell
git add GO/internal/processing/jit_airway_processor.go GO/internal/processing/jit_airway_processor_test.go GO/internal/processing/jit_airway_all_samples_test.go
git commit -m "feat: stream JIT orders with one Excel write"
```

### Task 5: Stream BigC store pages around its combined Excel write

**Files:**
- Modify: `internal/processing/bigc_processor.go`
- Modify: `internal/processing/bigc_processor_test.go`

**Interfaces:**
- Consumes: streaming callback and `ResultKey`.
- Produces: provisional BigC store rows followed by final updates after the existing combined write.

- [ ] **Step 1: Write failing success and failure tests**

For a multi-store BigC fixture, assert each successful store callback first has `processing`, and after the combined write the same keys have `done` or `warning`. Force `WriteOrderRows` failure with an invalid target and assert all provisional successful stores receive `failed` updates; parse-failed pages remain one final failure.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/processing -run TestBigCStreaming -count=1 -v`

Expected: callbacks are absent before the combined write.

- [ ] **Step 3: Emit provisional and final BigC rows**

Pass the callback into `processBigcDocument`. Emit provisional rows after each store calculation, retain current `allRows` accumulation, and finalize after `WriteOrderRows`. On write error, clone each provisional successful row with `StatusKindFailed` and an Excel-write error message before returning.

- [ ] **Step 4: Run BigC tests and the latest real PDF smoke test**

Run the BigC focused suite, then process `806_SOUTHDC_Q06_3005382_2634058273095.pdf` against a temporary workbook. Expected: 23 store rows, no duplicates, no failed rows under the frozen pricing fixture.

- [ ] **Step 5: Commit the task**

```powershell
git add GO/internal/processing/bigc_processor.go GO/internal/processing/bigc_processor_test.go
git commit -m "feat: stream BigC store results"
```

### Task 6: Replace the native JIT select with a custom menu

**Files:**
- Create: `frontend/src/components/JITPeriodMenu.tsx`
- Create: `frontend/src/lib/jitPeriodMenu.ts`
- Create: `frontend/src/lib/jitPeriodMenu.test.ts`
- Modify: `frontend/src/components/ResultTable.tsx`
- Modify: `frontend/package.json`

**Interfaces:**
- Consumes: existing `UpdateJITPeriod(rows, date, warehouse, period)` binding and `JITFileGroup`.
- Produces: `<JITPeriodMenu value disabled onChange />`.

- [ ] **Step 1: Write failing pure state tests**

Test exact choices `sáng`, `chiều`, `tối`, active-choice recognition, and Escape/outside-close transitions in `jitPeriodMenu.ts`.

- [ ] **Step 2: Run and verify RED**

Run: `cd frontend && npm.cmd test`

Expected: FAIL because the menu state helper does not exist.

- [ ] **Step 3: Implement the custom component**

Render a styled trigger button with chevron and a positioned panel of three buttons. Use `useRef` plus a document `mousedown` listener for outside close and `keydown` for Escape. Mark the selected choice with `FaCheck`; use existing `border-border`, `bg-panel`, `text-accent`, hover, disabled, and focus classes.

- [ ] **Step 4: Replace `<select>` in `ResultTable`**

Keep existing per-file grouping, request state, logs, and disabled conditions. Only presentation and menu interaction change.

- [ ] **Step 5: Run frontend tests and build**

Run: `cd frontend && npm.cmd test && npm.cmd run build`

Expected: all pure-state tests and TypeScript/Vite build pass.

- [ ] **Step 6: Commit the task**

```powershell
git add GO/frontend/src/components/JITPeriodMenu.tsx GO/frontend/src/components/ResultTable.tsx GO/frontend/src/lib/jitPeriodMenu.ts GO/frontend/src/lib/jitPeriodMenu.test.ts GO/frontend/package.json
git commit -m "feat: add custom JIT period menu"
```

### Task 7: Full verification and production build

**Files:**
- Verify only: all files changed in Tasks 1–6.
- Output: `build/bin/order-processor.exe`

**Interfaces:**
- Consumes: completed streaming backend, frontend upsert, and custom JIT menu.
- Produces: verified Windows production executable and SHA-256.

- [ ] **Step 1: Run frontend verification**

Run: `cd frontend && npm.cmd test && npm.cmd run build`

Expected: all tests and production bundle pass.

- [ ] **Step 2: Run focused backend verification**

Run: `go test . ./internal/processing/excelwriter ./internal/zalosend/... -count=1`

Run: `go test ./internal/processing -run 'TestJIT|TestBigC|TestRealProcessorStreams|TestParseJIT' -count=1`

Expected: all focused suites pass.

- [ ] **Step 3: Run repository build and diff check**

Run: `go build ./...`

Run: `git diff --check`

Expected: build exit 0 and no whitespace errors. Line-ending conversion warnings are informational.

- [ ] **Step 4: Build Wails executable**

Run: `wails build`

Expected: `build/bin/order-processor.exe` is produced successfully.

- [ ] **Step 5: Record artifact metadata**

```powershell
$exe = Get-Item -LiteralPath 'build\bin\order-processor.exe'
$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $exe.FullName
[PSCustomObject]@{ FullName=$exe.FullName; Length=$exe.Length; LastWriteTime=$exe.LastWriteTime; SHA256=$hash.Hash }
```

- [ ] **Step 6: Commit verification-compatible generated bindings if changed**

```powershell
git add GO/frontend/wailsjs/go/main/App.d.ts GO/frontend/wailsjs/go/main/App.js GO/frontend/wailsjs/go/models.ts
git commit -m "build: refresh Wails bindings for realtime results"
```
