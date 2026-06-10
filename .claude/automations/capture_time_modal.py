# /// script
# requires-python = ">=3.14"
# dependencies = [
#   "iterm2",
#   "pyobjc",
#   "pyobjc-framework-Quartz",
#   "pillow",
# ]
# ///
"""Capture the time-navigation modal for the landing page gallery.

Tests / captures:
  - Drives the deterministic live TUI (__vqa-tui --scenario failing)
    through welcome -> detail -> failure -> full log -> `t` modal.
  - Captures a retina screenshot via the documented landing pipeline:
    JetBrainsMonoNF-Regular 15 on #0E0F0C, grid converged to 124 cols,
    window placed on the built-in retina display (x=-1470), Quartz
    window-id screencapture, macOS chrome cropped by dark-row scan,
    one TUI border cell cropped so the page CSS supplies the frame.

Verification strategy:
  - Screen text must contain "Jump to time" and a gap entry before
    capture; abort loudly otherwise.

Usage:
  uv run .claude/automations/capture_time_modal.py <output.png>
"""

import asyncio
import pathlib
import subprocess
import sys

import iterm2
import Quartz
from PIL import Image

COLS = 124
ROWS = 24
RETINA_X = -1470
FONT = "JetBrainsMonoNF-Regular 15"
BG = (0x0E / 255, 0x0F / 255, 0x0C / 255)


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
    width, height = 1180, 660
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


def crop(output):
    image = Image.open(output).convert("RGB")
    pixels = image.load()
    width, height = image.size
    top = 0
    for y in range(height):
        samples = [pixels[x, y] for x in range(0, width, max(width // 48, 1))]
        if all(max(sample) < 30 for sample in samples):
            top = y
            break
    cell_w = width / COLS
    cell_h = (height - top) / (ROWS)
    box = (int(cell_w), int(top + cell_h), int(width - cell_w), int(height - cell_h))
    image.crop(box).save(output)


async def main(connection):
    output = sys.argv[1] if len(sys.argv) > 1 else "/tmp/time-modal.png"
    repo = pathlib.Path(__file__).resolve().parents[2]
    sessions = []
    try:
        window, session = await create_window(connection, "hound-shot-time")
        sessions.append(session)
        await style_session(session)
        if not await converge_grid(window, session, COLS, ROWS):
            raise RuntimeError(f"grid did not converge: {session.grid_size}")
        await session.async_send_text(
            f"clear; printf '\\033[?25l'; cd {repo} && HOUND_WELCOME=false TERM=xterm-256color "
            "./bin/gh-hound __vqa-tui --scenario failing\r"
        )
        await asyncio.sleep(2.0)
        for key, pause in [("\r", 0.8), ("\r", 0.8), ("l", 0.8), ("t", 0.8)]:
            await session.async_send_text(key)
            await asyncio.sleep(pause)
        text = await screen_text(session)
        if "Jump to time" not in text or "gap" not in text:
            raise RuntimeError("modal not on screen:\n" + text)
        frame = await window.async_get_frame()
        capture(frame, output)
        crop(output)
        subprocess.run(
            ["pngquant", "--quality", "75-95", "--force", "--output", output, output],
            check=False,
        )
        print(f"captured {output}")
    finally:
        for s in sessions:
            try:
                await s.async_send_text("q")
                await asyncio.sleep(0.3)
                await s.async_close()
            except Exception:
                pass


iterm2.run_until_complete(main)
