# PPT Generation V2 Phase 1 Implementation Report

- Date: 2026-08-15
- Worktree: `E:\code\work\ppt-v2-phase0-contract`
- Branch: `codex/ppt-v2-phase1-vertical-slice`
- Baseline HEAD: `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- Status: READY

## 1. Changed files

- `backend-go/internal/app/ppt/service.go`: add the three V2 artifact relation fields and owner-scoped relation validation.
- `backend-go/internal/app/ppt/postgres.go`: persist the relation through the existing task JSON storage path.
- `package.json` and `package-lock.json`: expose the Phase 1 test/render commands and lock the existing Phase 0 dependencies/workspace package.
- Phase 0 ADR and exit-gate documents are marked as superseded where Phase 1 contract `2.1` changes their decisions.

## 2. New files

- Strict `2.1` schemas under `contracts/ppt-v2/`.
- Canonical Golden 1 legacy input, SlideIR, and LayoutResult fixtures under `contracts/ppt-v2/fixtures/`.
- Formal Node package under `packages/ppt-v2/`.
- Internal application service and existing Work Center adapter under `backend-go/internal/httpserver/`.
- Node and Go automated tests for contract, migration adapter, layout, renderer, Golden 1, private artifact, owner scope, and task relation.
- ADR-011 and this implementation report.

## 3. Architecture boundaries

The implemented path is:

`Existing PPT Task/Request -> migration-only Legacy Adapter -> fixed two-page DeckRevision/SlideIR -> Layout Compiler -> LayoutResult -> PptxRenderer Port -> PptxGenJS Adapter -> existing private storage -> existing Work Center asset -> existing PPT task relation`.

- SlideIR is the only V2 content authority.
- LayoutResult is the only geometry and resolved-style authority.
- Renderer joins the two inputs only by `elementId`, respects `zIndex`, and does not change content, bounds, layout, or font size.
- PptxGenJS types are isolated inside the renderer adapter.
- There is no public V2 API, durable generation state machine, billing change, provider call, Connector change, or Phase 2 capability.

## 4. Contract changes

Phase 0 `2.0` could not express the required split authority because geometry and resolved style lived on renderer-facing elements. ADR-011 records this Contract Gap and publishes strict exact version `2.1`:

- `DeckRevision/SlideIR`: content, element semantics/order, slot, style role, and speaker notes; no geometry.
- `LayoutResult`: `elementId`, point geometry, `zIndex`, resolved style, and diagnostics; no content.
- `RenderInput`: deck revision metadata, SlideIR array, LayoutResult array, DesignSystem, Asset Manifest, and render options.
- All schema objects reject unknown fields. There is no `2.0` compatibility renderer, fallback, alias, migration layer, or dual write.
- Canonical geometry remains `960 x 540 pt`; only the PptxGenJS adapter converts points to inches.

## 5. Legacy Adapter behavior

The migration-only adapter performs one-way deterministic conversion:

- Legacy GenerateRequest -> DeckSpec.
- Legacy Outline -> fixed two-beat Minimal Storyline -> OutlinePlan.
- OutlineSlide -> SlideObjective.
- Legacy Task Context -> GenerationContext.

It creates stable readable IDs from the existing task identity, reports consumed/ignored/unmapped fields, rejects missing required source data, and has no reverse conversion. The legacy task retains its original slide JSON; only `v2DeckId`, `v2Revision`, and `pptxAssetId` are attached.

## 6. Layout Compiler behavior

Only two layout definitions exist:

- Cover: `title`, `subtitle`, optional `footer`.
- Standard Content: `title`, `body`; speaker notes are non-visual.

Compilation is deterministic and emits `elementId`, `x`, `y`, `width`, `height`, `zIndex`, `resolvedStyle`, and diagnostics. It fails closed for missing slots, negative size, invalid z-index, out-of-safe-area bounds, and error diagnostics. A simple text character threshold produces an explicit warning rather than silently changing layout.

## 7. Renderer behavior

The `PptxRenderer` port accepts only validated RenderInput. The PptxGenJS 4.0.1 adapter renders a 16:9 deck with native editable text, native editable shapes, bullets, speaker notes, theme fonts, and explicit z-order. It does not mutate the input or run layout logic. OOXML package normalization fixes notes-master ordering and timestamps so identical RenderInput produces byte-for-byte identical output.

## 8. Artifact integration

The internal application service reuses existing `storage.Service.StoreObject` and the existing Work Center asset store:

- PPTX visibility is `PRIVATE`.
- owner, tenant, and organization scope come from the authenticated existing task context.
- the asset is related to the existing task, then the task stores only `v2DeckId`, `v2Revision`, and `pptxAssetId`.
- owner-scope failure is rejected before render or storage.
- relation failure performs best-effort cleanup of the new private artifact and asset.
- no new storage subsystem, signed URL policy, billing rule, or task type was introduced.

## 9. Tests

Fresh verification:

- `npm.cmd run test:ppt-v2`: PASS, 13 tests, 0 failures.
- `go test ./internal/app/ppt`: PASS.
- `go test ./internal/httpserver -run 'Test(PPT|BuildPPTX|ConnectorPPT)'`: PASS.
- `npm.cmd run render:ppt-v2:golden`: PASS, two slides, deterministic 17,741-byte PPTX.
- `git diff --check`: PASS.

The broader `go test ./internal/app/ppt ./internal/httpserver` invocation also passed `internal/app/ppt`; eight unrelated existing Postgres integration tests in `internal/httpserver` could not connect to the absent local database at `127.0.0.1:55441`. This does not affect or mask any Phase 1 test; the focused PPT/Connector regression set and all new Phase 1 tests pass.

## 10. Golden Deck result

Golden 1 — Professional Business Deck is fixed to two pages and passes:

- Semantic parity: 100%.
- Seven stable, traceable `elementId` values.
- Exact SlideIR and LayoutResult fixture parity.
- Byte-for-byte repeatable renderer output.
- OOXML structure, two slides, two notes pages, bullets, native text/shapes, and no media part.
- OfficeCLI `validate`: 0 errors.
- OfficeCLI `view issues`: 0 issues.
- OfficeCLI text and per-slide visual inspection: no placeholder, clipping, overlap, off-slide content, or low contrast.

Golden 2-5 are not implemented. The strict element union leaves them for a future explicit contract version instead of silently accepting unsupported content.

## 11. Protected surfaces check

Checked against `.ai/CodexPrompt.md` and `docs/regression/protected-surfaces.md`:

- [x] W1-W4: no web points, landing/image/video, login, guest entry, or free-editing implementation changed.
- [x] M1-M8: no mini-program home, video inspiration, pricing/model selection, login, guest entry, or request-client implementation changed.
- [x] P1: no public image generation API, model, ratio, style, billing, or task behavior changed.
- [x] P2: existing PPT/Connector focused Go regression set remains green; no Connector adapter, billing chain, or public PPT API changed.
- [x] No merge, rebase, cherry-pick, main write, or other worktree write occurred.

The main worktree remains a distinct `main` worktree at the same baseline HEAD. Its four observed untracked governance documents are external existing/concurrent state and were recorded without reading, changing, moving, staging, or committing them.

## 12. Known limitations

- Exactly two pages: Cover and Standard Content.
- TextElement, ShapeElement, bullets, and SpeakerNotes only; no ImageElement.
- Fixed layouts and simple character-threshold diagnostics; no text measurement or reflow.
- Internal application service only; no new HTTP endpoint.
- LayoutResult JSON is the preview/debug boundary; no visual PreviewRenderer.
- No durable GenerationJob, retries, recovery, or progress model.
- No AI/provider usage and therefore no new billing event.
- No research, citation, chart, table, diagram, image, template, editor, import, regenerate, undo, PDF, Google Slides, or Connector work.

## 13. Phase 2 prerequisites

The minimum Phase 2 — Durable Generation scope is limited to persisting a GenerationJob around the same validated RenderInput and renderer/artifact boundary: explicit states, idempotency key, attempt record, retry/recovery policy, terminal error, and restart-safe linkage to the existing task and private artifact. It must reuse the current contract, layout compiler, renderer port, private storage, Work Center asset, owner scope, and existing billing behavior; research, richer elements, new APIs, UI, and Connector changes remain outside that slice.

## Decision

**Phase 1 Status: READY**
