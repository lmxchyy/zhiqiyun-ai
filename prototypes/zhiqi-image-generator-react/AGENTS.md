# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

## Zhiqi Cloud AI component decisions

- Use React 18 and Tailwind CSS.
- Use `#423499` for brand/selection states and `#FF771B` for the primary action.
- Use Plus Jakarta Sans for headings and Be Vietnam Pro for body copy, with Chinese/system fallbacks.
- Keep cards at 16px radius, meet WCAG AA contrast, and implement hover, focus, selected, disabled, loading, success, and error states.
- Default prompt: `例如：生成一张水果店开业促销海报，橙色系，高级感`; default aspect ratio: `auto`.
