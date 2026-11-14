---
name: update-demo-gif
description: Re-record the README demo GIF (docs/demo.gif) with VHS from the built-in demo data. Use after a visible UI, resource, column or key-binding change, or when asked to refresh or change the demo.
---

The GIF is generated, never hand-recorded: `docs/demo.tape` (a VHS script) drives `cloudssh --demo` inside a Docker container, so it needs no cloud account, no network and nothing installed on the host except Docker.

1. Edit `docs/demo.tape` only if the flow should change (keys are `Type`, `Enter`, `Tab`, `Sleep`, ...; see the VHS docs). Keep it under about 35 seconds; `make demo-gif` shrinks the GIF with gifsicle (about 0.7 MB), keep it under about 1 MB. Demo data must look realistic (see `demo-data`).
2. Run `make demo-gif`. It builds a Linux binary into `bin/linux/` (the recorder container is Linux; this is separate from `make build`), runs `ghcr.io/charmbracelet/vhs`, then compresses the GIF with gifsicle.
3. Inspect the result: `Screenshot docs/x.png` lines in the tape give single frames; for a whole-GIF contact sheet run `docker run --rm --entrypoint ffmpeg -v "$PWD":/vhs -w /vhs ghcr.io/charmbracelet/vhs -i docs/demo.gif -vf "fps=1/2,scale=480:-1,tile=4x4" -frames:v 1 /vhs/sheet.png`, then delete the helper files.
4. Commit `docs/demo.gif` (and `docs/demo.tape` if changed) as `docs: refresh demo gif`.
