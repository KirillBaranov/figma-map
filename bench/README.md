# Benchmark: agent with Figma alone vs agent + figma-map

Measures how close an agent's implementation gets to a Figma design **with** and
**without** figma-map, using an *independent* metric so it doesn't just reward
figma-map's own oracle.

## Cases

| Case | Source | Independent pixel diff |
|---|---|---|
| Landing hero (this file) | frame `868:167`, 1440×1024 | ~48% closer, see [REPORT.md](REPORT.md) |
| [landing-hero-2](cases/landing-hero-2) | public Supa Resume community file, 1600×960 | ~27% closer, see [REPORT.md](cases/landing-hero-2/REPORT.md) |
| [admin-dashboard](cases/admin-dashboard) | public BankDash Dashboard UI Kit, 1440×1175, real dense admin UI | ~78% closer, see [REPORT.md](cases/admin-dashboard/REPORT.md) |

The method below applies to all three; each case's own README documents what's
specific to it (source file, shared assets, reproduce commands).

## Method

Same agent (Claude Code, Sonnet 5), same task, same shared image assets —
only the tool differs:

- **baseline** — the agent builds from the design screenshot by eye, stopping at
  "looks right" (no figma-map).
- **treatment** — the agent builds using figma-map's exact tokens and the
  `reconcile` loop, converging until the diff is within tolerance.

Both arms use the **same real background images** (assets are a given), so the
measured difference reflects layout, spacing, typography, and color — not photos.

## Metrics (exact definitions)

### Pixel diff vs the design — independent, headline

- Each arm is rendered headless at the **frame width** and cropped to the design
  PNG's dimensions (off-canvas area padded white).
- A pixel **counts as different** when the summed absolute per-channel
  difference exceeds **100** out of a max of 765 (i.e. `|ΔR|+|ΔG|+|ΔB| > 100`,
  ≈ 13% of full range). The threshold ignores anti-aliasing/sub-pixel noise.
- The metric is `differing_pixels / total_pixels`. It is **not** figma-map's, so
  it does not favor the treatment arm. (`bench/main.go`, `const tol = 100`.)

### figma-map `reconcile` remaining — ours, biased

Reported for context only. Per-property tolerances (`internal/service/reconcile.go`):

| Property kind | Tolerance |
|---|---|
| font-size, border-radius, border-width, line-height, letter-spacing | **±0.5px** |
| padding, gap | **±1px** |
| element width/height | **±2px** (advisory) |
| color (bg/text/border) | **exact** after canonicalizing to rgba ints |
| font-weight | **exact** |
| opacity | **±0.02** |

The treatment arm optimizes against exactly this, so it is **not** the headline.

### Side-by-side composite + per-arm heatmaps

For human judgment (`out/sidebyside.png`, `out/<arm>_diff.png`).

## Reproduce

Environment used: figma-map **v0.2.0**, headless Chrome, coding agent
**Claude Code (Sonnet 5)** for both arms, figma-map's own vision LLM
**gpt-4o-mini** for `setup bind` only, figma-bridge on `:1994` with the file
open, the catalog/binding from `scan`+`bind`.

Fixed parameters: **frame `868:167`** ("Drive your design to a new age" landing
hero), **scale 1** → design **1440×1024**, render **width 1440**.

```bash
go build -o figma-map .
go build -o bin-bench ./bench
mkdir -p bench/work/baseline bench/work/treatment

# 1. Design reference (scale 1 = frame px)
./figma-map screenshot 868:167 --out bench/work/design.png --scale 1

# 2. Shared assets for both arms (assets are a given)
./figma-map export-assets <leftBgNodeId>  --format PNG --out bench/work/baseline
./figma-map export-assets <rightBgNodeId> --format PNG --out bench/work/baseline
cp bench/work/baseline/*.png bench/work/treatment/

# 3. baseline/index.html  — built by eye from design.png (no figma-map)
#    treatment/index.html — built with `tokens`/`plan` + `reconcile` loop,
#    each element tagged data-figma-node="<id>", converged to match.

# 4. Serve both arms
( cd bench/work/baseline  && python3 -m http.server 8201 & )
( cd bench/work/treatment && python3 -m http.server 8202 & )

# 5. Independent pixel metric + composite
bin-bench -design bench/work/design.png -width 1440 \
  -arm baseline=http://localhost:8201/ \
  -arm treatment=http://localhost:8202/ -out bench/work/out

# 6. figma-map metric (context)
./figma-map reconcile 868:167 --url http://localhost:8201/ --json
./figma-map reconcile 868:167 --url http://localhost:8202/ --json
```

## Honest caveats

- The same (strong) agent builds both arms — this isolates the *tool's* effect,
  not "figma-map vs a weak agent". The relative gap is the signal.
- N is small (1 frame, manual head-to-head). Scale with an automated arm runner.
- baseline positions are estimated by eye; treatment converges via `reconcile`.
- The `reconcile` metric is biased toward treatment by construction; trust the
  pixel diff and the screenshots.
- Result is *spec-perfect*, not pixel-raster identity — font rendering differs
  between Figma and the browser, so a residual pixel diff is expected even at a
  perfect spec match.

See [REPORT.md](REPORT.md) for the latest run.
