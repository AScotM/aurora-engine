# Aurora Engine — Bioluminescent Worlds

Aurora Engine is a living, terminal-rendered ecosystem of fireflies, drifting light, and mathematically generated night landscapes. This edition is deliberately closer to Firefly Observatory than a conventional aurora or plasma renderer.

Written in a single Go source file using the standard library. No external packages, GPU, GUI, or privileged access required.

## Build

```sh
go build -o aurora-engine main.go
./aurora-engine
```

Press Ctrl+C to leave the alternate terminal screen and restore your shell. The renderer uses cursor repositioning rather than clearing the terminal every frame. Truecolor and Unicode half-block support are recommended. The default canvas is 100 columns wide by 24 terminal rows, plus one status line.

## Scenes

- `grove`: hovering lights above a dark tree-line silhouette
- `river`: gentle current, light following a sinuous river channel
- `canopy`: lights gathering near the upper part of the frame
- `meadow`: open drifting field

## Palettes

- `forest`: lime, green, honey and mint
- `amber`: warm candlelight and gold
- `moon`: cool blue, violet and cyan
- `mixed`: multicolored luminous insects

## Examples

```sh
./aurora-engine -scene grove -palette forest -fireflies 180 -fps 30
./aurora-engine -scene river -palette moon -wind 1.2 -trails 0.9
./aurora-engine -scene canopy -palette amber -fireflies 240 -speed 0.6
./aurora-engine -scene meadow -palette mixed -seed 42
```

To render a reproducible 15-second simulation without terminal animation, and export the final frame and full particle state:

```sh
./aurora-engine -seed 42 -duration 15 -fps 30 -snapshot exports/night.ppm -metrics exports/night.json
```

PPM is a simple lossless image format supported by ImageMagick, GIMP, and many image tools. For PNG conversion, if ImageMagick is installed, run `magick exports/night.ppm exports/night.png`.

## Simulation

Each light has position, velocity, phase, oscillation rate, energy, age, species and group. The flow field combines spatially varying trigonometric waves with slow time evolution. Motion uses inertia, gentle wind and scene-specific attraction. Flash intensity follows a sharply peaked oscillator. Nearby particles exert a weak coupling effect on the flash phase, yielding loose emergent synchronization rather than identical blinking.

A persistent RGB buffer stores phosphorescent trails with exponential decay. Each luminous particle contributes a narrow bright core and a wider Gaussian halo. The renderer uses the Unicode upper-half-block glyph with separate foreground and background truecolor values to double vertical resolution. Color values are tone-mapped before conversion to 8-bit RGB.

The exported JSON contains configuration, latest simulation metrics and particle state. Metrics include active lights, average glow and circular phase coherence.

## Configuration

All command-line parameters are represented in the JSON configuration structure. Pass `-config settings.json` to load a complete or partial configuration. JSON settings are loaded after command-line parsing and therefore override command-line flags. The supported options are displayed with `-help`.

## Limitations

The simulation is a generative artwork, not a scientifically validated insect model. The current implementation uses a fixed-size viewport; resize your terminal and restart with adjusted `-width` and `-height`. At high firefly counts and resolutions, rendering and phase coupling may increase CPU usage. A finite-duration run computes frames as fast as possible rather than waiting for wall-clock pacing.

## License

GNU General Public License version 3. See `LICENSE`.

## Important startup fix and verification

The previous edition had a critical loop-bound overflow: continuous mode converted an effectively infinite duration into an `int`, which overflowed and caused the rendering loop to execute zero times. The corrected loop uses an explicit unbounded condition when `-duration 0`, and a bounded frame count for finite-duration rendering.

This revision contains over 1,200 formatted lines of Go source and adds spatial-grid neighborhood queries, multiscale noise and curl-flow utilities, colony aggregation, light profiles, color-processing functions, image-buffer operations, observatory statistics, entropy and synchronization measurements, state-archive support, and CSV/JSON timeline utilities. The original scenes and command-line interface remain compatible.

Build and verify without an interactive terminal:

```sh
go build -o aurora-engine main.go
./aurora-engine -width 60 -height 30 -duration 2 -fps 20 -snapshot sample.ppm -metrics sample.json
```

To watch the continuous simulation:

```sh
./aurora-engine -width 80 -height 36 -scene grove -palette forest -fireflies 180 -fps 24
```

The `-height` option specifies pixel rows; the display consumes roughly half that many terminal text rows, plus a status line. Ctrl+C exits continuous mode. The renderer requires a terminal supporting ANSI escape sequences for the intended visual experience.

The supplementary analysis and archive primitives are implemented in the single source file as an extensible foundation. They are not all exposed as command-line options in this release.
