#!/usr/bin/env python3
import json
import math
import struct
import sys
import time
import zlib
from pathlib import Path


DEFAULT_FORBIDDEN = ["\ufffd", "\u25a1", "\u25af"]
MIN_UNIQUE_COLORS = 16
MIN_COLORED_SAMPLES = 60
MIN_NON_BG_SAMPLES = 100
FRESH_SECONDS = 180


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)


def visible_len(value):
    return len(value.replace("\t", "    "))


def verify_text(assertion_path, capture_path, cols, rows):
    assertion = json.loads(Path(assertion_path).read_text())
    capture = Path(capture_path).read_text(errors="replace")
    missing = [needle for needle in assertion.get("contains", []) if needle not in capture]
    if missing:
        print(f"{capture_path} missing assertions:", file=sys.stderr)
        for needle in missing:
            print(f"  - {needle}", file=sys.stderr)
        raise SystemExit(1)

    forbidden = DEFAULT_FORBIDDEN + assertion.get("forbid", [])
    present = [needle for needle in forbidden if needle and needle in capture]
    if present:
        print(f"{capture_path} contains forbidden glyph/text:", file=sys.stderr)
        for needle in present:
            print(f"  - {needle!r}", file=sys.stderr)
        raise SystemExit(1)

    if "\x1b[" in capture:
        fail(f"{capture_path} contains raw ANSI escapes despite plain capture")

    if rows:
        lines = capture.splitlines()
        if len(lines) > rows:
            fail(f"{capture_path} has {len(lines)} lines, expected at most {rows}")
    if cols:
        too_wide = [(i + 1, visible_len(line), line) for i, line in enumerate(capture.splitlines()) if visible_len(line) > cols]
        if too_wide:
            line, width, content = too_wide[0]
            fail(f"{capture_path}:{line} width {width} exceeds {cols}: {content}")

    return assertion, capture


def read_png(path):
    data = Path(path).read_bytes()
    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
        fail(f"{path} is not a PNG")
    pos = 8
    width = height = color_type = bit_depth = None
    idat = bytearray()
    while pos < len(data):
        if pos + 8 > len(data):
            fail(f"{path} has a truncated PNG chunk")
        length = struct.unpack(">I", data[pos : pos + 4])[0]
        kind = data[pos + 4 : pos + 8]
        chunk = data[pos + 8 : pos + 8 + length]
        pos += 12 + length
        if kind == b"IHDR":
            width, height, bit_depth, color_type, _, _, interlace = struct.unpack(">IIBBBBB", chunk)
            if bit_depth != 8 or color_type not in (2, 6) or interlace != 0:
                fail(f"{path} uses unsupported PNG format bit_depth={bit_depth} color_type={color_type} interlace={interlace}")
        elif kind == b"IDAT":
            idat.extend(chunk)
        elif kind == b"IEND":
            break
    if width is None or height is None:
        fail(f"{path} is missing IHDR")
    channels = 4 if color_type == 6 else 3
    row_bytes = width * channels
    raw = zlib.decompress(bytes(idat))
    rows = []
    prev = bytearray(row_bytes)
    cursor = 0
    for _ in range(height):
        filter_type = raw[cursor]
        cursor += 1
        row = bytearray(raw[cursor : cursor + row_bytes])
        cursor += row_bytes
        recon = bytearray(row_bytes)
        for i, value in enumerate(row):
            left = recon[i - channels] if i >= channels else 0
            up = prev[i]
            upper_left = prev[i - channels] if i >= channels else 0
            if filter_type == 0:
                recon[i] = value
            elif filter_type == 1:
                recon[i] = (value + left) & 0xFF
            elif filter_type == 2:
                recon[i] = (value + up) & 0xFF
            elif filter_type == 3:
                recon[i] = (value + ((left + up) // 2)) & 0xFF
            elif filter_type == 4:
                recon[i] = (value + paeth(left, up, upper_left)) & 0xFF
            else:
                fail(f"{path} has unsupported PNG filter {filter_type}")
        rows.append(recon)
        prev = recon
    return width, height, channels, rows


def paeth(a, b, c):
    p = a + b - c
    pa = abs(p - a)
    pb = abs(p - b)
    pc = abs(p - c)
    if pa <= pb and pa <= pc:
        return a
    if pb <= pc:
        return b
    return c


def pixel_at(rows, channels, x, y):
    row = rows[y]
    i = x * channels
    return tuple(row[i : i + 3])


def verify_png(assertion, png_path, capture_path, cols, rows_count):
    png = Path(png_path)
    if not png.exists():
        fail(f"{png_path} does not exist")
    if png.stat().st_size < 1024:
        fail(f"{png_path} is suspiciously small ({png.stat().st_size} bytes)")
    age = time.time() - png.stat().st_mtime
    if age > assertion.get("fresh_seconds", FRESH_SECONDS):
        fail(f"{png_path} is stale ({age:.0f}s old)")
    if png.stat().st_mtime + 1 < Path(capture_path).stat().st_mtime:
        fail(f"{png_path} is older than its text capture")

    width, height, channels, rows = read_png(png_path)
    if cols and width % cols != 0:
        fail(f"{png_path} width {width} is not divisible by terminal cols {cols}")
    if rows_count and height % rows_count != 0:
        fail(f"{png_path} height {height} is not divisible by terminal rows {rows_count}")
    if cols and rows_count:
        cell_w = width // cols
        cell_h = height // rows_count
        if cell_w < 6 or cell_h < 10:
            fail(f"{png_path} cell size {cell_w}x{cell_h} is too small for visual QA")

    step = max(1, int(math.sqrt((width * height) / 90000)))
    bg = pixel_at(rows, channels, 0, 0)
    colors = set()
    colored = 0
    non_bg = 0
    for y in range(0, height, step):
        for x in range(0, width, step):
            rgb = pixel_at(rows, channels, x, y)
            colors.add(rgb)
            if color_distance(rgb, bg) > 18:
                non_bg += 1
            if max(rgb) - min(rgb) > 28 and max(rgb) > 60:
                colored += 1

    min_colors = assertion.get("min_unique_colors", MIN_UNIQUE_COLORS)
    if len(colors) < min_colors:
        fail(f"{png_path} has only {len(colors)} sampled colors, expected at least {min_colors}")
    min_colored = assertion.get("min_colored_samples", MIN_COLORED_SAMPLES)
    if colored < min_colored:
        fail(f"{png_path} has only {colored} colored samples, expected at least {min_colored}")
    if non_bg < assertion.get("min_non_bg_samples", MIN_NON_BG_SAMPLES):
        fail(f"{png_path} has only {non_bg} non-background samples; image may be blank")

    if cols and rows_count and not assertion.get("allow_bottom_right_cursor", False):
        assert_no_bottom_right_cursor(png_path, rows, channels, width, height, cols, rows_count)


def color_distance(a, b):
    return sum(abs(x - y) for x, y in zip(a, b))


def assert_no_bottom_right_cursor(path, rows, channels, width, height, cols, rows_count):
    cell_w = width // cols
    cell_h = height // rows_count
    x0 = max(0, width - cell_w)
    y0 = max(0, height - cell_h)
    total = bright = 0
    for y in range(y0, height):
        for x in range(x0, width):
            r, g, b = pixel_at(rows, channels, x, y)
            total += 1
            if r > 220 and g > 220 and b > 220:
                bright += 1
    if total and bright / total > 0.18:
        fail(f"{path} has a bright block in the bottom-right cell; possible leaked terminal cursor")


def main():
    if len(sys.argv) not in (3, 6):
        fail("usage: verify_capture.py ASSERTION.json CAPTURE.txt [CAPTURE.png COLS ROWS]")
    assertion_path, capture_path = sys.argv[1], sys.argv[2]
    cols = rows = 0
    if len(sys.argv) == 6:
        png_path = sys.argv[3]
        cols = int(sys.argv[4])
        rows = int(sys.argv[5])
    assertion, _ = verify_text(assertion_path, capture_path, cols, rows)
    if len(sys.argv) == 6:
        verify_png(assertion, png_path, capture_path, cols, rows)


if __name__ == "__main__":
    main()
