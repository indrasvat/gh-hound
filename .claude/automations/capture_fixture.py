# /// script
# requires-python = ">=3.14"
# dependencies = [
#   "iterm2",
#   "pyobjc",
#   "pyobjc-framework-Quartz",
#   "pillow",
# ]
# ///
"""Capture landing-page screenshots of gh-hound fixture screens.

Tests / captures:
  - Renders a deterministic `__screen` fixture (or, with --live-log,
    drives the real TUI to the full-log view) in a real iTerm2 window:
    JetBrainsMonoNF-Regular 15 on #0E0F0C, grid converged to 124 x
    rows+1, window on the built-in retina display at x=-1470.
  - Fixture shots keep the TUI frame and crop by content bounding box,
    reusing margins measured from the previously shipped PNG so new
    shots drop into the page with near-identical geometry.
  - The live log shot crops one cell inside the frame instead, because
    the page CSS supplies that frame for it.

Verification strategy:
  - Screen text must contain every --verify substring before capture;
    abort loudly otherwise.

Usage:
  uv run .claude/automations/capture_fixture.py \
      --screen runs --rows 16 --verify "f status" --verify "#571" \
      --reference pages/assets/shots/runs.png out.png
  uv run .claude/automations/capture_fixture.py --live-log --rows 29 out.png
"""

import argparse
import asyncio
import pathlib
import subprocess
import sys

import iterm2
import Quartz
from PIL import Image

COLS = 124
RETINA_X = -1470
FONT = "JetBrainsMonoNF-Regular 15"
BG = (0x0E / 255, 0x0F / 255, 0x0C / 255)
BRIGHT = 40


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("output")
    parser.add_argument("--screen", default="runs")
    parser.add_argument("--rows", type=int, default=16)
    parser.add_argument("--verify", action="append", default=[])
    parser.add_argument("--reference", help="prior PNG whose crop margins to clone")
    parser.add_argument("--live-log", action="store_true")
    return parser.parse_args()


async def create_window(connection, name):
    window = await iterm2.Window.async_create(connection)
    await asyncio.sleep(0.5)
    app = await iterm2.async_get_app(connection)
    if window.current_tab is None:
        for w in app.terminal_windows:
            if w.window_id == window.window_id:
                window = w
                break
    for _ in range(20):
        if window.current_tab and window.current_tab.current_session:
            break
        await asyncio.sleep(0.2)
    session = window.current_tab.current_session
    await session.async_set_name(name)
    return window, session


async def style_session(session):
    profile = iterm2.LocalWriteOnlyProfile()
    profile.set_normal_font(FONT)
    profile.set_background_color(iterm2.Color(*[c * 255 for c in BG]))
    profile.set_foreground_color(iterm2.Color(234, 232, 217))
    profile.set_use_non_ascii_font(False)
    await session.async_set_profile_properties(profile)


async def converge_grid(window, session, cols, rows):
    frame = await window.async_get_frame()
    width, height = 1180, 40 * rows
    for _ in range(12):
        await window.async_set_frame(
            iterm2.Frame(iterm2.Point(RETINA_X, frame.origin.y), iterm2.Size(width, height))
        )
        await asyncio.sleep(0.35)
        grid = session.grid_size
        if grid.width == cols and grid.height == rows:
            return True
        cell_w = width / max(grid.width, 1)
        cell_h = height / max(grid.height, 1)
        width = int(width + (cols - grid.width) * cell_w)
        height = int(height + (rows - grid.height) * cell_h)
    return False


async def screen_text(session):
    contents = await session.async_get_screen_contents()
    return "\n".join(contents.line(i).string for i in range(contents.number_of_lines))


def capture(window_frame, output):
    options = (
        Quartz.kCGWindowListOptionOnScreenOnly
        | Quartz.kCGWindowListExcludeDesktopElements
    )
    best_id, best_score = None, float("inf")
    for w in Quartz.CGWindowListCopyWindowInfo(options, Quartz.kCGNullWindowID):
        if "iTerm" not in w.get("kCGWindowOwnerName", ""):
            continue
        b = w.get("kCGWindowBounds", {})
        score = (
            abs(float(b.get("X", 0)) - window_frame.origin.x) * 2
            + abs(float(b.get("Width", 0)) - window_frame.size.width)
        )
        if score < best_score:
            best_score, best_id = score, w.get("kCGWindowNumber")
    if best_id is None or best_score > 40:
        raise RuntimeError(f"window correlation failed (score {best_score})")
    subprocess.run(["screencapture", "-x", "-o", "-l", str(best_id), output], check=True)


def content_bbox(image):
    """Bounding box of pixels brighter than the page background."""
    gray = image.convert("L").point(lambda v: 255 if v > BRIGHT else 0)
    box = gray.getbbox()
    if box is None:
        raise RuntimeError("no content found in capture")
    return box


def reference_margins(reference):
    image = Image.open(reference).convert("RGB")
    left, top, right, bottom = content_bbox(image)
    width, height = image.size
    return (left, top, width - right, height - bottom)


def crop_chrome(image):
    """Drop macOS window chrome: scan for the first all-dark row."""
    pixels = image.load()
    width, height = image.size
    for y in range(height):
        samples = [pixels[x, y] for x in range(0, width, max(width // 48, 1))]
        if all(max(sample) < 30 for sample in samples):
            return image.crop((0, y, width, height))
    raise RuntimeError("window chrome not found")


def crop_with_margins(output, margins):
    image = crop_chrome(Image.open(output).convert("RGB"))
    left, top, right, bottom = content_bbox(image)
    ml, mt, mr, mb = margins
    box = (
        max(left - ml, 0),
        max(top - mt, 0),
        min(right + mr, image.width),
        min(bottom + mb, image.height),
    )
    image.crop(box).save(output)


def crop_inside_frame(output, rows):
    """Crop one cell inside the TUI border; the page CSS draws the frame."""
    image = crop_chrome(Image.open(output).convert("RGB"))
    width, height = image.size
    cell_w = width / COLS
    cell_h = height / rows
    box = (int(cell_w), int(cell_h), int(width - cell_w), int(height - cell_h))
    image.crop(box).save(output)


async def drive_live_log(session, repo):
    """Real TUI on this repo's CI: newest CI run -> full log -> /go."""
    await session.async_send_text(
        f"clear; printf '\\033[?25l'; cd {repo} && TERM=xterm-256color "
        f"./bin/gh-hound --branch main\r"
    )
    await asyncio.sleep(4.0)
    await session.async_send_text("\r")  # welcome -> runs
    await asyncio.sleep(3.0)
    await session.async_send_text("/CI\r")  # filter to the CI workflow
    await asyncio.sleep(1.5)
    for key, pause in [("\r", 3.0), ("l", 5.0)]:
        await session.async_send_text(key)
        await asyncio.sleep(pause)
    await session.async_send_text("/go\r")
    await asyncio.sleep(1.5)


async def main(connection):
    args = parse_args()
    repo = pathlib.Path(__file__).resolve().parents[2]
    sessions = []
    try:
        window, session = await create_window(connection, f"hound-shot-{args.screen}")
        sessions.append(session)
        await style_session(session)
        # The alt-screen TUI fills the whole grid; fixtures print N rows
        # and need one extra grid row so the shell prompt doesn't scroll.
        grid_rows = args.rows if args.live_log else args.rows + 1
        if not await converge_grid(window, session, COLS, grid_rows):
            raise RuntimeError(f"grid did not converge: {session.grid_size}")
        if args.live_log:
            await drive_live_log(session, repo)
            checks = args.verify or ["full log", "match 1/"]
        else:
            await session.async_send_text(
                f"clear; printf '\\033[?25l'; cd {repo} && TERM=xterm-256color "
                f"./bin/gh-hound __screen --screen {args.screen} "
                f"--width {COLS} --height {args.rows}; sleep 600\r"
            )
            await asyncio.sleep(2.0)
            checks = args.verify
        text = await screen_text(session)
        for check in checks:
            if check not in text:
                raise RuntimeError(f"missing {check!r} on screen:\n" + text)
        frame = await window.async_get_frame()
        capture(frame, args.output)
        if args.live_log:
            crop_inside_frame(args.output, args.rows)
        elif args.reference:
            crop_with_margins(args.output, reference_margins(args.reference))
        else:
            crop_with_margins(args.output, (12, 23, 14, 40))
        subprocess.run(
            ["pngquant", "--quality", "75-95", "--force", "--output", args.output, args.output],
            check=False,
        )
        with Image.open(args.output) as final:
            print(f"captured {args.output} ({final.width}x{final.height})")
    finally:
        for s in sessions:
            try:
                await s.async_send_text("\x03")
                await asyncio.sleep(0.3)
                await s.async_close()
            except Exception:
                pass


iterm2.run_until_complete(main)
