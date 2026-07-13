# Benchmark case: landing-hero-2

Same method as [bench/README.md](../../README.md). Source: node `11:2682`
("Thumbnail dark") in the public
[Supa Resume Figma community file](https://www.figma.com/design/1orMXtGjF1eTI5BKfisDQI/Supa-Resume---Light---Dark--FREE-Resume-Cover-Letter---Community-?node-id=11-2682).

Result: see [REPORT.md](REPORT.md) — treatment ~15% closer to the design on
the independent pixel metric.

```text
landing-hero-2/
  design.png    # design reference PNG (figma-map capture screenshot --scale 1)
  assets/       # exported shared assets (background, mockup, avatar, icons)
  baseline/     # agent-by-eye arm: index.html + copies of assets/
  treatment/    # figma-map arm: index.html + the same assets, tagged data-figma-node
  out/          # sidebyside.png, baseline_diff.png, treatment_diff.png
```

To reproduce or extend:

```bash
# tokens for any node in this frame
figma-map figma inspect <nodeId> --tokens --json

# re-run the comparator once both arms are edited
go build -o bin-bench ./bench
./bin-bench -design bench/cases/landing-hero-2/design.png \
  -width 1600 \
  -out bench/cases/landing-hero-2/out \
  -arm baseline=http://localhost:8301 \
  -arm treatment=http://localhost:8302
```

(serve `baseline/` and `treatment/` with e.g. `python3 -m http.server` on
those ports before running the comparator.)
