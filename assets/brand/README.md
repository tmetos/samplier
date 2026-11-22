# Samplier brand kit

Night-market identity for an environmental data system: deep ink canvas, sampling
green, serving blue — and hot pink reserved *only* for regulatory exceedances.

## The mark

One shape: a **sample droplet with a meniscus fill line**. The curved line is the
surface of a measured volume — the single detail that turns a generic droplet
into a sample. Nothing else.

It reads at 16 px, works on light and dark backgrounds without a tile, and takes
the green→blue brand gradient across the body.

| File | Use |
|---|---|
| `logo-mark.svg` | Primary mark, transparent — README, docs, light or dark |
| `logo-mark-tile.svg` | Tiled variant — favicon, app icon, social avatars |
| `logo-lockup.svg` | Horizontal lockup for **dark** backgrounds |
| `logo-lockup-light.svg` | Horizontal lockup for **light** backgrounds |
| `social-preview.svg` / `.png` | GitHub social preview (1280×640) |
| `alternative-monogram.svg` | Not in use — an "S" with a droplet terminal, kept as a runner-up |

The mark is also inlined as a React component (`ui/src/components/BrandMark.tsx`,
`tiled` prop toggles the tile) so the UI renders it crisply with no extra request.

## Palette

| Token | Hex | Meaning |
|---|---|---|
| `--bg` | `#0a0f13` | Night-market ink (canvas) |
| `--panel` | `#111a20` | Surface |
| `--panel-2` | `#17232b` | Raised surface |
| `--border` | `#22303a` | Hairline |
| `--text` | `#e8f1f4` | Primary text |
| `--muted` | `#8598a6` | Secondary text |
| **`--green`** | `#35b37e` | Sampling — primary accent, locations, success |
| `--green-bright` | `#5ee7a8` | Hover, liquid fill |
| `--green-dim` | `#1f7a55` | Active nav, pressed states |
| **`--blue`** | `#6fa8dc` | Serving — links, samples, secondary data |
| `--blue-bright` | `#a8d4fb` | Hover, droplets |
| `--blue-dim` | `#2f6d9e` | Muted blue UI |
| **`--exceed`** | `#ff5c8a` | **Exceedances only** |
| `--exceed-wash` | `rgba(255,92,138,.14)` | Exceedance row background |
| `--danger` | `#e05252` | System errors (never exceedances) |
| `--warn` | `#e0a952` | Warnings, tasks |

**Rule:** pink means "this result breaches a regulatory limit." Never use it for
generic errors, deletions, or decoration — its meaning is the brand.

## Type

- UI: system sans stack (`ui-sans-serif, system-ui, -apple-system, "Segoe UI"`).
- Wordmark: same stack, weight 700, negative tracking (−1.2 at 52 px).
- The lockup is mark + wordmark only. It carries no tagline.

## Voice

Short, service-oriented, never hype. The product samples data and serves it —
but the identity stays literal: a droplet, measured. Keep the sommelier wink in
prose (a tasting note here, a "served" there), never in the mark.

## Usage

- Keep clear space around the mark equal to the droplet's height.
- Minimum sizes: 16 px (mark), 110 px wide (lockup).
- Don't recolour the mark, add outlines, rotate it, or place it on mid-tone
  photography. The bare mark works on light and dark; use the tile where the
  surface is busy or an app-icon shape is expected.
- Don't stretch or re-typeset the wordmark.

## Regenerating the social preview

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless --no-sandbox --disable-gpu --hide-scrollbars \
  --window-size=1280,640 \
  --screenshot=assets/brand/social-preview.png \
  "file://$PWD/assets/brand/social-preview.svg"
```
