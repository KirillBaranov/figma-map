# Benchmark case: admin-dashboard

Same method as [bench/README.md](../../README.md). Source: node `78:351`
("Main Dashboard") in the public BankDash Dashboard UI Kit Figma community
file — a real component-dense admin screen (sidebar, top bar, cards,
transaction list, two charts, avatars), unlike the two landing-hero cases.

Result: see [REPORT.md](REPORT.md) — treatment ~78% closer to the design on
the independent pixel metric, the largest gap of the three cases so far.

```text
admin-dashboard/
  design.png    # design reference PNG (figma-map capture screenshot --scale 1)
  assets/       # exported shared assets (icons, avatars, chip, gradient card bg, chart PNGs)
  baseline/     # agent-by-eye arm: index.html + copies of assets/
  treatment/    # figma-map arm: index.html + the same assets, tagged data-figma-node
  out/          # sidebyside.png, baseline_diff.png, treatment_diff.png
```

To reproduce or extend:

```bash
figma-map figma inspect <nodeId> --depth <n> --tokens --json

go build -o bin-bench ./bench
./bin-bench -design bench/cases/admin-dashboard/design.png \
  -width 1440 \
  -out bench/cases/admin-dashboard/out \
  -arm baseline=http://localhost:8401 \
  -arm treatment=http://localhost:8402
```

(serve `baseline/` and `treatment/` with e.g. `python3 -m http.server` on
those ports before running the comparator.)

Note: the two chart widgets (bar/pie/line) and the avatar photos are shared
flattened image assets in both arms, not rebuilt from tokens — see REPORT.md
for why.
