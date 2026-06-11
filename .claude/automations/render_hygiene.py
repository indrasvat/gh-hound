# /// script
# requires-python = ">=3.11"
# ///
"""Render-hygiene audit: the flicker contract, byte-exact.

Spawns the real binary on a pty (lossless — unlike `shux pane watch`,
whose data plane is intentionally sampled and can drop the very bytes
this audit greps for), drives a scroll burst through the runs list and
the log screen, and asserts the raw output stream:

  1. contains NO full-screen erase (ESC[2J) after the first frame —
     each one is the blank flash users see as flicker;
  2. wraps frames in synchronized-output guards (mode 2026);
  3. emits small steady-state updates (line-diffed scrolling must not
     rewrite whole frames).

Wired into `make vqa`. Negative-controlled 2026-06-11: the v0.5.0
renderer fails all three (26 erases / ~218KB for one scroll burst).
The flaky scenario drives a 6-row list so j/k moves real content
(QA round 18: the failing scenario's 1-row list made the size check
near-vacuous).
"""

import os
import pty
import select
import subprocess
import sys
import time

COLS, ROWS = 120, 40


def read_available(fd: int, duration: float) -> bytes:
    out = b""
    deadline = time.monotonic() + duration
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], 0.05)
        if not ready:
            continue
        try:
            chunk = os.read(fd, 65536)
        except OSError:
            break
        if not chunk:
            break
        out += chunk
    return out


def main() -> int:
    controller, follower = pty.openpty()
    os.set_blocking(controller, False)
    import fcntl
    import struct
    import termios

    fcntl.ioctl(follower, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    proc = subprocess.Popen(
        ["./bin/gh-hound", "__vqa-tui", "--scenario", "flaky"],
        stdin=follower,
        stdout=follower,
        stderr=subprocess.DEVNULL,
        close_fds=True,
    )
    os.close(follower)

    try:
        startup = read_available(controller, 3.0)
        os.write(controller, b"\r")  # welcome -> runs
        read_available(controller, 1.0)

        scroll = b""
        for _ in range(8):
            os.write(controller, b"j")
            scroll += read_available(controller, 0.1)
            os.write(controller, b"k")
            scroll += read_available(controller, 0.1)
        os.write(controller, b"l")  # the log screen
        log_phase = read_available(controller, 2.0)
        scroll += log_phase
        for _ in range(8):
            os.write(controller, b"j")
            scroll += read_available(controller, 0.1)
        os.write(controller, b"q")
        read_available(controller, 1.0)
        # q must exit the app cleanly: a stuck or crashed app means the
        # interaction never completed and the stream proves nothing.
        clean_exit = False
        for _ in range(40):
            if proc.poll() is not None:
                clean_exit = True
                break
            time.sleep(0.1)
    finally:
        if proc.poll() is None:
            proc.terminate()
        proc.wait(timeout=5)
        os.close(controller)

    failures = []
    if not startup:
        failures.append("no startup bytes captured (harness problem)")
    if not clean_exit:
        failures.append("app did not exit cleanly after q — the drive never completed")
    elif proc.returncode != 0:
        failures.append(f"app exited {proc.returncode} — the drive crashed, the stream proves nothing")
    if b"hound" not in startup:
        failures.append("startup chrome missing from the stream — the app never painted")
    # The log footer ('fold'/'scroll') only exists on the log screen:
    # this proves the l keypress actually navigated, independently of
    # the startup paint (codex: a conjunction here was vacuous).
    if b"fold" not in log_phase and b"scroll" not in log_phase:
        failures.append("log-screen content missing after l — the drive never reached the log screen")
    if b"\x1b[2J" in scroll:
        failures.append(
            f"full-screen erase during scroll: {scroll.count(b'\x1b[2J')} occurrences — flicker regression"
        )
    if b"\x1b[?2026h" not in scroll:
        failures.append("no synchronized-output guards in the scroll stream")
    # 16 j/k repaints of a line-diffed list must be small; whole-frame
    # rewrites at 120x40 would be hundreds of KB.
    if len(scroll) > 120_000:
        failures.append(
            f"scroll burst wrote {len(scroll)} bytes — steady-state updates are not line-diffed"
        )

    if failures:
        for f in failures:
            print(f"render hygiene: FAIL — {f}", file=sys.stderr)
        return 1
    print(f"render hygiene passed (scroll burst: {len(scroll)} bytes, 0 erases)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
