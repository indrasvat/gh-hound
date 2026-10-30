# /// script
# requires-python = ">=3.12"
# dependencies = ["pillow"]
# ///
"""Record main against a branch on the same fixture, mark what changed.

Records:
  - Two isolated shux panes, one per build, launched together on the
    same `__vqa-tui --scenario` fixture so fixture clocks line up.
  - A key timeline drives both (or one side); every frame keeps a PNG
    and the pane's plain text.

Marks:
  - Each rule's regex is found in a frame's text; the match becomes cell
    coordinates, so marks follow the content as rows move.
  - Style is part of the contract. Marks must be identifiable at a
    glance and still belong to the picture:
      * colours from the TUI theme only: fail red (what main gets
        wrong), ok green (what the branch shows), run amber (the same on
        both); never a colour the TUI does not use
      * thin rounded outlines at full strength with a faint tint inside;
        neighbouring rows of one point share one mark
      * a small numbered badge in blank cells beside the mark, its words
        in a legend on the pane's title bar: no glyph is ever covered
      * at most three or four marks per pane at once; say one thing each

Verification strategy:
  - Both panes must show the scenario's ready text before keys start.
  - Every run prints, per rule, how many frames it marked; a rule that
    marks no frame fails the run (its regex or window is wrong).
  - A missing JetBrainsMono Nerd Font fails the run: a fallback font
    silently changes badge and legend metrics.

Usage (spec: docs/tasks/<task>-demo.json):
  uv run .claude/skills/recording-pr-evidence/scripts/demo_compare.py docs/tasks/410-demo.json \\
      --main-bin /tmp/gh-hound-main --pr-bin ./bin/gh-hound \\
      --out .shux/out/demo-410 --mp4 .shux/out/demo-410/demo-410.mp4
  # re-mark recorded frames after editing rules: add --skip-record
"""

import argparse
import json
import os
import pathlib
import re
import shlex
import shutil
import subprocess
import sys
import time

from PIL import Image, ImageDraw, ImageFont

# shux's PNG raster cell (measured on shux 0.51), upscaled 2x so glyphs
# stay crisp under nearest-neighbour scaling.
CELL_W, CELL_H, SCALE = 9, 19, 2
CW, CH = CELL_W * SCALE, CELL_H * SCALE

# TUI theme colours (internal/theme): marks read as part of the frame.
BG = (14, 15, 12)
FG_SOFT = (207, 205, 187)
INK = (14, 15, 12)
COLOURS = {"bad": (226, 86, 75), "good": (79, 211, 122), "same": (224, 163, 62)}
RADIUS, STROKE, TINT = 7, 3, 34
CAP, BAR = 64, 52
KEYS = {"enter": "DQ==", "esc": "Gw==", "tab": "CQ==", "up": "G1tB", "down": "G1tC"}
# The keycap a pane just received, on its title bar: the screen's own
# surface, line and foreground colours, shown for KEYCAP_FRAMES frames.
KEYCAP_FILL, KEYCAP_LINE, KEYCAP_FG = (36, 39, 30), (61, 66, 51), (234, 232, 217)
KEYCAP_FRAMES = 10
KEY_GLYPHS = {"enter": "⏎", "esc": "⎋", "tab": "⇥", "up": "↑", "down": "↓", "space": "␣"}


def font(name, size):
    for root in (pathlib.Path.home() / "Library/Fonts", pathlib.Path("/Library/Fonts"), pathlib.Path("/usr/share/fonts")):
        for path in root.rglob(f"JetBrainsMonoNerdFont-{name}.ttf") if root.exists() else []:
            return ImageFont.truetype(str(path), size)
    sys.exit(f"JetBrainsMono Nerd Font {name} not found: install it (brew install --cask font-jetbrains-mono-nerd-font)")


def shux(*args, check=True):
    return subprocess.run(["shux", *args], check=check, capture_output=True, text=True)


def record(spec, main_bin, pr_bin, out):
    cols, rows = (int(v) for v in spec["size"].split("x"))
    fps, seconds = spec.get("fps", 5), spec["seconds"]
    # One shux daemon per run: a shared dir would let one run's
    # `daemon stop` kill another's panes. Kept short (socket path limit).
    runtime = pathlib.Path(spec.get("runtime_dir", f"/tmp/hdemo{os.getpid()}"))
    runtime.mkdir(parents=True, exist_ok=True)
    env = {**os.environ, "XDG_RUNTIME_DIR": str(runtime)}
    if out.exists():
        shutil.rmtree(out)
    sessions = {}
    try:
        for side, binary in (("main", main_bin), ("pr", pr_bin)):
            (out / side).mkdir(parents=True)
            runner = runtime / f"run-{side}.sh"
            # The terminal the spec asks for: its env (an empty value
            # unsets, e.g. COLORTERM for a 256-colour terminal) and the
            # colours a light terminal sets, OSC 10 and 11, for a light
            # theme: without the foreground, plain text stays pale.
            term_env = {"TERM": "xterm-256color", "COLORTERM": "truecolor", **spec.get("env", {})}
            for key in term_env:
                if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
                    sys.exit(f"spec env: {key!r} is not a variable name")
            exports = "".join(f"export {k}={shlex.quote(v)}\n" if v else f"unset {k}\n" for k, v in term_env.items())
            background = "".join(
                f"printf '\\033]{code};%s\\007' {shlex.quote(spec[field])}\n"
                for code, field in (("10", "foreground"), ("11", "background"))
                if spec.get(field)
            )
            runner.write_text(
                "#!/usr/bin/env bash\n"
                f"{exports}export CLICOLOR_FORCE=1 HOUND_WELCOME=false\n"
                # The TUI reads its size once at start: the pane is
                # sized while the runner sleeps, before the first paint.
                f"unset NO_COLOR\n{background}sleep 1\n"
                f"exec {pathlib.Path(binary).resolve()} __vqa-tui --scenario {spec['scenario']}\n"
            )
            runner.chmod(0o755)
            name = f"demo-{side}-{os.getpid()}"
            subprocess.run(["shux", "session", "create", name, "-d", "--", str(runner)], env=env, check=True, capture_output=True)
            subprocess.run(["shux", "pane", "set-size", "-s", name, "--cols", str(cols), "--rows", str(rows)], env=env, check=True, capture_output=True)
            sessions[side] = name
        for name in sessions.values():
            subprocess.run(["shux", "pane", "wait-for", "-s", name, "--text", spec["ready"], "--timeout-ms", "20000"], env=env, check=True, capture_output=True)

        steps = {s["frame"]: s for s in spec["timeline"]}
        captions = out / "captions.tsv"
        captions.write_text("")
        start = time.time()
        for i in range(seconds * fps):
            step = steps.get(i)
            if step:
                for side, key in step.get("keys", {}).items():
                    for target in (("main", "pr") if side == "both" else (side,)):
                        data = KEYS.get(key)
                        args = ["--data", data] if data else ["--text", key]
                        subprocess.run(["shux", "pane", "send-keys", "-s", sessions[target], *args], env=env, check=True, capture_output=True)
                if step.get("caption"):
                    with captions.open("a") as f:
                        f.write(f"{i}\t{step['caption']}\n")
            for side, name in sessions.items():
                subprocess.run(["shux", "pane", "snapshot", "-s", name, "-o", str(out / side / f"{i:04d}.png")], env=env, check=True, capture_output=True)
                text = subprocess.run(["shux", "pane", "capture", "-s", name, "--lines", str(rows), "--format", "plain"], env=env, check=True, capture_output=True, text=True).stdout
                (out / side / f"{i:04d}.txt").write_text(text)
            time.sleep(max(start + (i + 1) / fps - time.time(), 0))
    finally:
        for name in sessions.values():
            subprocess.run(["shux", "session", "kill", name], env=env, capture_output=True)
        subprocess.run(["shux", "daemon", "stop"], env=env, capture_output=True)


def row_end(row):
    body = row.rstrip()
    if body.endswith("│"):
        body = body[:-1].rstrip()
    return len(body)


def find(rows, rule):
    """(first row, last row, first col, end col) of a rule's match."""
    first, last = rule["match"], rule.get("through")
    for r, row in enumerate(rows):
        m = re.search(first, row)
        if not m:
            continue
        c1 = row_end(row) if rule.get("to_row_end") else m.end()
        if not last:
            return r, r, m.start(), c1
        for r2 in range(r, len(rows)):
            m2 = re.search(last, rows[r2])
            if m2:
                ends = [m2.end()] + [row_end(rows[k]) for k in range(r, r2)]
                starts = [m.start()] + [len(rows[k]) - len(rows[k].lstrip("│ ")) for k in range(r + 1, r2 + 1)]
                return r, r2, min(starts), max(ends)
        return None
    return None


def mark(img, rows, side, i, rules, fonts, hits):
    overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    legend = []
    for index, rule in enumerate(rules):
        lo, hi = rule["frames"]
        if rule["side"] not in (side, "both") or not lo <= i <= hi:
            continue
        hit = find(rows, rule)
        if not hit:
            continue
        hits[index] += 1
        colour = COLOURS[rule["tone"]]
        r0, r1, c0, c1 = hit
        x0, y0 = max(c0 * CW - CW // 2, STROKE), r0 * CH - 2
        x1, y1 = min(c1 * CW + CW // 2, img.width - STROKE - 1), (r1 + 1) * CH + 2
        draw.rounded_rectangle((x0, y0, x1, y1), radius=RADIUS, fill=colour + (TINT,), outline=colour + (255,), width=STROKE)
        number = str(len(legend) + 1)
        row = rows[r0] if r0 < len(rows) else ""
        if row[c1 + 1 : c1 + 4].strip() == "" and x1 + 34 < img.width:
            cx, cy = x1 + 18, y0 + CH // 2 + 1
        else:
            cx, cy = max(x0, 13), max(y0, 13)
        draw.ellipse((cx - 12, cy - 12, cx + 12, cy + 12), fill=colour + (255,))
        draw.text((cx, cy + 1), number, font=fonts["badge"], fill=INK, anchor="mm")
        legend.append((number, rule["label"], colour))
    return Image.alpha_composite(img.convert("RGBA"), overlay).convert("RGB"), legend


def keycap(draw, right, y, key, fonts):
    """Draw the key a pane just received, right-aligned in its title bar."""
    text = KEY_GLYPHS.get(key, key)
    label = "pressed"
    label_w = draw.textlength(label, font=fonts["title"])
    cap_w = max(64, draw.textlength(text, font=fonts["keycap"]) + 36)
    x1 = right - 20
    x0 = x1 - cap_w
    draw.rounded_rectangle((x0, y + 3, x1, y + 49), radius=9, fill=KEYCAP_FILL, outline=KEYCAP_FG, width=3)
    draw.line((x0 + 8, y + 44, x1 - 8, y + 44), fill=KEYCAP_LINE, width=3)
    draw.text(((x0 + x1) / 2, y + 24), text, font=fonts["keycap"], fill=KEYCAP_FG, anchor="mm")
    draw.text((x0 - 14 - label_w, y + 9), label, font=fonts["title"], fill=FG_SOFT)


def compose(spec, out, marked):
    fonts = {
        "badge": font("Bold", 19),
        "legend": font("Regular", 23),
        "caption": font("Regular", 28),
        "title": font("Bold", 26),
        "keycap": font("Bold", 36),
    }
    pressed = {"main": [], "pr": []}
    for step in spec["timeline"]:
        for target, key in step.get("keys", {}).items():
            for side in ("main", "pr") if target == "both" else (target,):
                pressed[side].append((step["frame"], key))
    rules = spec["rules"]
    hits = [0] * len(rules)
    captions = [(int(f), t) for f, t in (line.split("\t", 1) for line in (out / "captions.tsv").read_text().splitlines())]
    if marked.exists():
        shutil.rmtree(marked)
    marked.mkdir(parents=True)
    panes_spec = [(spec["panes"]["main"], COLOURS["bad"], "main"), (spec["panes"]["pr"], COLOURS["good"], "pr")]
    for i, path in enumerate(sorted((out / "main").glob("*.png"))):
        panes = []
        for _, _, side in panes_spec:
            img = Image.open(out / side / path.name).convert("RGB")
            img = img.resize((img.width * SCALE, img.height * SCALE), Image.NEAREST)
            rows = (out / side / f"{path.stem}.txt").read_text().split("\n")
            panes.append(mark(img, rows, side, i, rules, fonts, hits))
        width = max(p.width for p, _ in panes)
        canvas = Image.new("RGB", (width, CAP + sum(BAR + p.height for p, _ in panes)), BG)
        draw = ImageDraw.Draw(canvas)
        caption = next((t for f, t in reversed(captions) if f <= i), "")
        draw.text((20, 18), caption, font=fonts["caption"], fill=FG_SOFT)
        y = CAP
        for (pane, legend), (label, colour, side) in zip(panes, panes_spec):
            draw.ellipse((20, y + 17, 34, y + 31), fill=colour)
            draw.text((44, y + 9), label, font=fonts["title"], fill=colour)
            x = 44 + draw.textlength(label, font=fonts["title"]) + 36
            for number, text, tone in legend:
                draw.ellipse((x, y + 12, x + 24, y + 36), fill=tone)
                draw.text((x + 12, y + 25), number, font=fonts["badge"], fill=INK, anchor="mm")
                draw.text((x + 33, y + 11), text, font=fonts["legend"], fill=FG_SOFT)
                x += 33 + draw.textlength(text, font=fonts["legend"]) + 30
            recent = [key for frame, key in pressed[side] if frame <= i < frame + KEYCAP_FRAMES]
            if recent:
                keycap(draw, width, y, recent[-1], fonts)
            canvas.paste(pane, (0, y + BAR))
            y += BAR + pane.height
        canvas.save(marked / path.name, compress_level=1)
    return hits


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("spec", type=pathlib.Path)
    parser.add_argument("--main-bin")
    parser.add_argument("--pr-bin")
    parser.add_argument("--out", type=pathlib.Path, required=True)
    parser.add_argument("--mp4", type=pathlib.Path)
    parser.add_argument("--skip-record", action="store_true", help="re-mark frames already recorded in --out")
    args = parser.parse_args()
    spec = json.loads(args.spec.read_text())
    raw = args.out / "raw"
    if not args.skip_record:
        if not (args.main_bin and args.pr_bin):
            parser.error("--main-bin and --pr-bin are required to record")
        record(spec, args.main_bin, args.pr_bin, raw)
    hits = compose(spec, raw, args.out / "marked")
    idle = [rule["label"] for rule, n in zip(spec["rules"], hits) if n == 0]
    for rule, n in zip(spec["rules"], hits):
        print(f"{n:4d} frames · {rule['side']:4s} · {rule['label']}")
    if args.mp4:
        playback = spec.get("playback_fps", spec.get("fps", 5) * 2)
        subprocess.run(
            ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-framerate", str(playback),
             "-i", str(args.out / "marked" / "%04d.png"), "-c:v", "libx264", "-preset", "slow",
             "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(args.mp4)],
            check=True,
        )
        print(f"wrote {args.mp4}")
    if idle:
        sys.exit("rules that marked no frame: " + "; ".join(idle))


if __name__ == "__main__":
    main()
