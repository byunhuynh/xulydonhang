# Realtime Order Results Design

## Goal

Stream each recognized order or store-page result to the frontend without waiting for its entire source file to finish, while preserving safe and efficient Excel writes. Replace the native JIT period selector with an app-styled custom menu.

## Result identity and lifecycle

Each `OrderRow` gains a stable `resultKey` derived from source file, logical page/segment, and PO. A row also has one of four typed states: `processing`, `done`, `warning`, or `failed`.

The backend may emit the same `resultKey` more than once:

1. A provisional `processing` row as soon as parsing and business calculations for that order finish.
2. A final row after its Excel write succeeds, or a `failed` row if persistence fails.

The frontend handles `process:row` as an upsert. A new key appends a row; an existing key replaces that row in place. This prevents duplicates and keeps STT stable while status and Excel metadata change.

## Backend streaming boundary

Add an optional streaming processor interface alongside the existing `Processor` interface. `App.runBatch` uses it when available and passes a row callback that immediately emits logs and `process:row`. It records streamed keys, then emits only returned rows that were never streamed. Existing test processors and non-streaming implementations remain compatible.

`RealProcessor` implements the streaming interface. Vendor loops invoke the callback at the earliest safe point described below. The normal `Process` method remains available and delegates to the same implementation with no callback.

## Vendor behavior

### JIT Top Value

Parse pages sequentially. Emit a provisional row after a page has a tracking number, PO, mapped products, quantities, weights, and prices. Accumulate all Excel rows for the PDF in memory and write them in one `WriteOrderRows` call after all pages have been processed. Convert relative row references to absolute Excel rows, then emit final updates. If the single Excel write fails, update every provisional successful JIT row to `failed`. Parse, mapping, or price failures are emitted immediately as final failures and are excluded from the Excel batch.

JIT does not perform price reconciliation. Its UI reconciliation cell reads `Không đối soát`.

### BigC

Keep the existing one-write-per-file strategy. Emit each successfully calculated store page provisionally while continuing through the document. Parse failures are emitted immediately as final failures. After the combined Excel write succeeds, add absolute Excel row references and emit final updates. If the write fails, update all provisional successful store rows to `failed`.

### Other vendors

Keep their current Excel strategy. Their segment handler already returns only after that segment's Excel write finishes, so emit each returned row immediately as a final row from the existing dispatch loop. File-level failures are emitted immediately. No broad rewrite of their stable Excel builders is required.

## Ordering and counters

Rows stay in first-seen order. An update never moves a row or increments STT. `process:done` keeps the existing final STT contract; only the first appearance of a `resultKey` counts as a new result.

## JIT period menu

Render one control per JIT PDF using the streamed/upserted rows already present in the store. Replace the native `<select>` with a custom button and popover matching the app's panel, border, accent, hover, and focus styles. The menu contains `Sáng`, `Chiều`, and `Tối`, marks the active value, closes on outside click or Escape, and is disabled while processing or while its update request is running.

Changing a period calls `UpdateJITPeriod` once with every absolute Excel row associated with that PDF. The backend validates the period and updates columns B and L in one workbook open/save cycle. It never affects another PDF.

## Error handling

- A callback must never cause duplicate rows; `resultKey` upsert is the final guard.
- A failed combined Excel write changes every affected provisional row to `failed`.
- Rows that failed before persistence remain failed and are not included in Excel row ranges.
- UI period-update errors retain the prior selection and append an error to the system log.
- Period updates and price confirmations share the workbook mutex and are rejected while a processing batch is active.

## Testing

- Processor streaming tests prove rows arrive before file completion and are not duplicated in the final result.
- App tests prove streamed keys count once and returned-but-unstreamed rows still appear.
- JIT integration tests cover multiple provisional pages, one combined Excel write, absolute row assignment, and write-failure updates.
- BigC tests cover provisional store rows followed by successful and failed final updates.
- Frontend pure-state tests cover append-versus-update semantics, stable ordering/STT, and JIT grouping as rows arrive.
- Custom-menu behavior is verified through extracted state helpers plus TypeScript production build.
- Existing vendor golden tests and Wails production build remain release gates.
