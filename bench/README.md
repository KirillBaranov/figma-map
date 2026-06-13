# Benchmark: agent with Figma alone vs agent + figma-map

Measures how close an agent's implementation gets to a Figma design **with** and
**without** figma-map, using an *independent* metric so it doesn't just reward
figma-map's own oracle.

## Method

Same agent, same model, same task, same shared image assets — only the tool
differs:

- **baseline** — the agent builds from the design screenshot by eye, stopping at
  "looks right" (no figma-map).
- **treatment** — the agent builds using figma-map's exact tokens and the
  `reconcile` loop, converging until the diff is within tolerance.

Both arms use the **same real background images** (assets are a given), so the
measured difference reflects layout, spacing, typography, and color — not photos.

## Metrics

- **Pixel diff vs the design (independent, headline).** `bench` renders each arm
  at the frame width, crops to the design size, and reports the share of pixels
  that differ beyond a tolerance. This metric is *not* figma-map's, so it does
  not favor the treatment arm.
- **figma-map `reconcile` remaining (ours — biased).** Reported for context, but
  the treatment arm optimizes against exactly this, so it is not the headline.
- **Side-by-side composite + per-arm heatmaps** for human judgment.

## Run

```bash
go build -o bin-bench ./bench
# serve each arm (baseline/, treatment/) and screenshot the design, then:
bin-bench -design work/design.png -width 1440 \
  -arm baseline=http://localhost:8201/ \
  -arm treatment=http://localhost:8202/ -out work/out
```

## Honest caveats

- The same (strong) agent builds both arms — this isolates the *tool's* effect,
  not "figma-map vs a weak agent". The relative gap is the signal.
- N is small (manual head-to-head). Scale with an automated arm runner later.
- The `reconcile` metric is biased toward treatment by construction; trust the
  pixel diff and the screenshots.

See [REPORT.md](REPORT.md) for the latest run.
