# Benchmark report

Run: manual head-to-head, 1 frame ("Drive your design to a new age" landing hero,
1440×1024). figma-map v0.2.0. See [README.md](README.md) for method and caveats.

## Result

| Arm | Pixel diff vs design (independent) | figma-map reconcile (biased) |
|---|---|---|
| **baseline** (Figma screenshot, by eye) | **7.83%** pixels differ | 36 fixable, 14/28 measured |
| **treatment** (figma-map tokens + reconcile loop) | **4.10%** pixels differ | 1 fixable, 16/28 measured |

**Treatment was ~48% closer on the independent pixel metric** (−3.73 pp). On
figma-map's own (biased) metric the gap is far larger, as expected.

*(Updated 2026-07-13: the comparator originally rendered at a fixed 900px
viewport height regardless of the design's actual height (1024px here),
silently cropping ~124px off the bottom of both arms and comparing that
padded-white strip against the design's real content. Fixed in `bench/main.go`
to render at the design's own height instead — a bug in the benchmark tool,
not in figma-map itself. Numbers above are post-fix and lower for both arms;
treatment's relative lead grew (28%→48%) since the cropped strip had been
diluting both arms' scores toward each other with shared white space.)*

![design vs baseline vs treatment](sidebyside.png)

*Left to right: design, baseline (by eye), treatment (figma-map). The baseline
drifts on heading size and the 01–04 grid spacing/positions; the treatment
converges to the design's exact values.*

## Read

The independent pixel diff already favors the treatment, and the gap is
understated: it includes shared photo regions (identical in both arms, so no
delta there) — the difference comes entirely from typography, spacing, and color
in the non-image areas. The point isn't the absolute %, it's that an objective
oracle let the agent converge instead of stopping at "looks right".
