#!/usr/bin/env python3
"""Capture one full-frame PNG from a VNC (RFB 3.8) server.

Built for the Omarchy QEMU guest that is reachable from this Mac only over
the tailnet (the VNC port is bound on the WSL loopback and forwarded with
`tailscale serve`). The picture is the only way a text session can see
whether the guest is on the installer, a Hyprland desktop, or a boot
console.

The password is read from VNC_PASSWORD, never from argv: `ps` lists
command lines, and a flag would land in shell history. Do not put the
value in this file, in logs, or in error text.

This tool is not a CI job. It talks to an operator-owned host that is not
on the runner, and a green/red result there would not mean anything about
the tree.

Requires: pycryptodome (Crypto.Cipher.DES) and Pillow. No pip install —
those two are already on the machine this was written on.
"""

from __future__ import annotations

import argparse
import os
import socket
import struct
import sys
import time

# Security type 2 (VNC Authentication) as used by QEMU -vnc password-secret=.
SEC_VNC = 2
SEC_NONE = 1

# Client → server
MSG_SET_PIXEL_FORMAT = 0
MSG_SET_ENCODINGS = 2
MSG_FB_UPDATE_REQUEST = 3
MSG_KEY_EVENT = 4
MSG_POINTER_EVENT = 5

# A short X11 keysym table for --do combo:/key: actions. Printable
# Latin-1 is sent as the code point itself (RFB uses X keysyms).
KEYSYMS = {
    "Escape": 0xFF1B,
    "Return": 0xFF0D,
    "Linefeed": 0xFF0A,
    "Tab": 0xFF09,
    "BackSpace": 0xFF08,
    "space": 0x0020,
    "Super_L": 0xFFE7,  # Meta_L; Super_L 0xFFEB is also sent (see send_combo)
    "Super_R": 0xFFEC,
    "Meta_L": 0xFFE7,
    "Alt_L": 0xFFE9,
    "Shift_L": 0xFFE1,
    "Control_L": 0xFFE3,
    "F1": 0xFFBE,
    "F2": 0xFFBF,
    "F3": 0xFFC0,
    "F4": 0xFFC1,
}

# Server → client
SMSG_FB_UPDATE = 0
SMSG_SET_COLORMAP = 1
SMSG_BELL = 2
SMSG_CUT_TEXT = 3

ENC_RAW = 0

# 32-bpp little-endian truecolour, red@16 green@8 blue@0 (BGRX in memory).
PIXEL_FORMAT = struct.pack(
    "!BBBBHHHBBBxxx",
    32,  # bits-per-pixel
    24,  # depth
    0,  # big-endian-flag
    1,  # true-colour-flag
    255,
    255,
    255,  # rgb max
    16,
    8,
    0,  # rgb shift
)


class VNCError(RuntimeError):
    pass


class Deadline:
    def __init__(self, seconds: float) -> None:
        self.end = time.monotonic() + seconds

    def remaining(self) -> float:
        left = self.end - time.monotonic()
        if left <= 0:
            raise VNCError("timed out waiting for the VNC server")
        return left


def recvn(sock: socket.socket, n: int, deadline: Deadline) -> bytes:
    buf = bytearray()
    while len(buf) < n:
        sock.settimeout(deadline.remaining())
        try:
            chunk = sock.recv(n - len(buf))
        except socket.timeout as e:
            raise VNCError("timed out waiting for the VNC server") from e
        if not chunk:
            raise VNCError(
                f"VNC connection closed after {len(buf)} bytes, wanted {n}"
            )
        buf.extend(chunk)
    return bytes(buf)


def sendall(sock: socket.socket, data: bytes, deadline: Deadline) -> None:
    view = memoryview(data)
    while view:
        sock.settimeout(deadline.remaining())
        try:
            n = sock.send(view)
        except socket.timeout as e:
            raise VNCError("timed out sending to the VNC server") from e
        if n == 0:
            raise VNCError("VNC connection closed while sending")
        view = view[n:]


def _reverse_bits(byte: int) -> int:
    # VNC DES keys are the password bytes with each byte's bits reversed
    # (LSB-first). Skipping this makes authentication fail with no extra hint.
    return int(f"{byte:08b}"[::-1], 2)


def vnc_des_response(password: str, challenge: bytes) -> bytes:
    if len(challenge) != 16:
        raise VNCError(f"VNC challenge was {len(challenge)} bytes, want 16")
    try:
        from Crypto.Cipher import DES
    except ImportError as e:
        raise VNCError(
            "Crypto.Cipher.DES is missing (pycryptodome). "
            "Do not pip-install on this machine; use the copy already present."
        ) from e
    raw = password.encode("utf-8")[:8].ljust(8, b"\x00")
    key = bytes(_reverse_bits(b) for b in raw)
    return DES.new(key, DES.MODE_ECB).encrypt(challenge)


def handshake(sock: socket.socket, password: str, deadline: Deadline) -> tuple[int, int, str]:
    banner = recvn(sock, 12, deadline)
    if not banner.startswith(b"RFB "):
        raise VNCError(f"not an RFB server (banner={banner!r})")
    # Speak 3.8 even if the server offered something older; QEMU sends 003.008.
    sendall(sock, b"RFB 003.008\n", deadline)

    ntypes = recvn(sock, 1, deadline)[0]
    if ntypes == 0:
        (reason_len,) = struct.unpack("!I", recvn(sock, 4, deadline))
        reason = recvn(sock, reason_len, deadline).decode("utf-8", "replace")
        raise VNCError(f"server refused security negotiation: {reason}")
    types = list(recvn(sock, ntypes, deadline))
    if SEC_VNC not in types:
        raise VNCError(
            f"server did not offer VNC Authentication (type 2); offered {types}"
        )
    sendall(sock, bytes([SEC_VNC]), deadline)

    challenge = recvn(sock, 16, deadline)
    sendall(sock, vnc_des_response(password, challenge), deadline)

    (result,) = struct.unpack("!I", recvn(sock, 4, deadline))
    if result != 0:
        extra = ""
        # RFB 3.8 may follow a failed result with a reason string.
        try:
            (reason_len,) = struct.unpack("!I", recvn(sock, 4, deadline))
            if 0 < reason_len < 4096:
                extra = recvn(sock, reason_len, deadline).decode("utf-8", "replace")
        except VNCError:
            extra = ""
        if extra:
            raise VNCError(f"VNC authentication failed ({extra})")
        raise VNCError("VNC authentication failed")

    # ClientInit: shared desktop (do not kick an existing viewer).
    sendall(sock, b"\x01", deadline)

    init = recvn(sock, 24, deadline)
    width, height = struct.unpack("!HH", init[:4])
    (name_len,) = struct.unpack("!I", init[20:24])
    if name_len > 1_000_000:
        raise VNCError(f"implausible desktop name length {name_len}")
    name = recvn(sock, name_len, deadline).decode("utf-8", "replace")
    return width, height, name


def setup_raw(sock: socket.socket, deadline: Deadline) -> None:
    # SetPixelFormat: 3 pad bytes after the type.
    sendall(sock, bytes([MSG_SET_PIXEL_FORMAT, 0, 0, 0]) + PIXEL_FORMAT, deadline)
    # SetEncodings: Raw only, so the decoder stays a byte copy.
    sendall(
        sock,
        struct.pack("!BBH i", MSG_SET_ENCODINGS, 0, 1, ENC_RAW),
        deadline,
    )


def request_raw_frame(
    sock: socket.socket, width: int, height: int, deadline: Deadline
) -> None:
    sendall(
        sock,
        struct.pack("!BBHHHH", MSG_FB_UPDATE_REQUEST, 0, 0, 0, width, height),
        deadline,
    )


def key_event(sock: socket.socket, keysym: int, down: bool, deadline: Deadline) -> None:
    sendall(
        sock,
        struct.pack("!BBHI", MSG_KEY_EVENT, 1 if down else 0, 0, keysym & 0xFFFFFFFF),
        deadline,
    )


def tap_key(sock: socket.socket, keysym: int, deadline: Deadline) -> None:
    key_event(sock, keysym, True, deadline)
    key_event(sock, keysym, False, deadline)


def resolve_keysym(name: str) -> list[int]:
    """Return one or more keysyms to press for a named key.

    Super is sent as both Super_L (0xFFEB) and Meta_L (0xFFE7): QEMU/Hyprland
    bindings disagree on which one Super generates, and sending both down
    before the key is the cheap way to hit either mapping.
    """
    if name in ("Super", "Super_L", "Mod4"):
        return [0xFFEB, 0xFFE7]
    if name in KEYSYMS:
        return [KEYSYMS[name]]
    if len(name) == 1:
        return [ord(name)]
    raise VNCError(f"unknown key name {name!r}")


def send_combo(sock: socket.socket, spec: str, deadline: Deadline) -> None:
    parts = [p for p in spec.split("+") if p]
    if not parts:
        raise VNCError("empty combo")
    mods = []
    for p in parts[:-1]:
        mods.extend(resolve_keysym(p))
    keys = resolve_keysym(parts[-1])
    for m in mods:
        key_event(sock, m, True, deadline)
    for k in keys:
        tap_key(sock, k, deadline)
    for m in reversed(mods):
        key_event(sock, m, False, deadline)


# QEMU's VNC path maps keysyms through a US keycode table. Sending the
# shifted keysym alone (e.g. 0x2a for '*') arrives as the unshifted key
# ('8'). Hold Shift_L and send the unshifted US key instead.
_US_SHIFT = {
    "!": "1",
    "@": "2",
    "#": "3",
    "$": "4",
    "%": "5",
    "^": "6",
    "&": "7",
    "*": "8",
    "(": "9",
    ")": "0",
    "_": "-",
    "+": "=",
    "{": "[",
    "}": "]",
    "|": "\\",
    ":": ";",
    '"': "'",
    "<": ",",
    ">": ".",
    "?": "/",
    "~": "`",
}


def _key_gap(deadline: Deadline) -> None:
    # QEMU drops or coalesces RFB KeyEvents when they arrive back-to-back.
    end = time.monotonic() + 0.025
    while True:
        left = end - time.monotonic()
        if left <= 0:
            return
        deadline.remaining()
        time.sleep(min(left, 0.025))


def type_text(sock: socket.socket, text: str, deadline: Deadline) -> None:
    for ch in text:
        if ch == "\n":
            tap_key(sock, KEYSYMS["Return"], deadline)
            _key_gap(deadline)
            continue
        if ch == "\t":
            tap_key(sock, KEYSYMS["Tab"], deadline)
            _key_gap(deadline)
            continue
        if ord(ch) < 0x20 or ord(ch) > 0xFF:
            raise VNCError(f"cannot type character U+{ord(ch):04X}")
        if ch in _US_SHIFT or ch.isupper():
            base = _US_SHIFT.get(ch, ch.lower())
            key_event(sock, KEYSYMS["Shift_L"], True, deadline)
            tap_key(sock, ord(base), deadline)
            key_event(sock, KEYSYMS["Shift_L"], False, deadline)
        else:
            tap_key(sock, ord(ch), deadline)
        _key_gap(deadline)


def pointer(sock: socket.socket, x: int, y: int, mask: int, deadline: Deadline) -> None:
    sendall(
        sock,
        struct.pack("!BBHH", MSG_POINTER_EVENT, mask & 0xFF, x & 0xFFFF, y & 0xFFFF),
        deadline,
    )


def run_actions(
    sock: socket.socket, actions: list[str], deadline: Deadline
) -> None:
    """Run --do actions in order.

    Forms (no password, no host secrets — just key names and typed text):
      key:Escape
      combo:Super+Return
      type:uname -a
      enter
      sleep:0.4
      click:100,200
    """
    for raw in actions:
        if raw == "enter":
            tap_key(sock, KEYSYMS["Return"], deadline)
            continue
        if raw.startswith("sleep:"):
            sec = float(raw.split(":", 1)[1])
            if sec < 0 or sec > 30:
                raise VNCError(f"sleep out of range: {sec}")
            # Sleep is wall time; keep the RFB deadline from firing mid-wait
            # by requiring the caller to pass a timeout that covers it.
            end = time.monotonic() + sec
            while True:
                left = end - time.monotonic()
                if left <= 0:
                    break
                deadline.remaining()
                time.sleep(min(left, 0.05))
            continue
        if raw.startswith("key:"):
            for k in resolve_keysym(raw.split(":", 1)[1]):
                tap_key(sock, k, deadline)
            continue
        if raw.startswith("combo:"):
            send_combo(sock, raw.split(":", 1)[1], deadline)
            continue
        if raw.startswith("type:"):
            type_text(sock, raw.split(":", 1)[1], deadline)
            continue
        if raw.startswith("click:"):
            coords = raw.split(":", 1)[1]
            xs, ys = coords.split(",", 1)
            x, y = int(xs), int(ys)
            pointer(sock, x, y, 0, deadline)
            pointer(sock, x, y, 1, deadline)
            pointer(sock, x, y, 0, deadline)
            continue
        raise VNCError(f"unknown --do action {raw!r}")


def skip_colormap(sock: socket.socket, deadline: Deadline) -> None:
    recvn(sock, 1, deadline)  # padding
    _first, n = struct.unpack("!HH", recvn(sock, 4, deadline))
    recvn(sock, n * 6, deadline)


def skip_cut_text(sock: socket.socket, deadline: Deadline) -> None:
    recvn(sock, 3, deadline)  # padding
    (length,) = struct.unpack("!I", recvn(sock, 4, deadline))
    if length > 16_000_000:
        raise VNCError(f"ServerCutText length {length} is implausible")
    recvn(sock, length, deadline)


def paint_raw(
    canvas: bytearray, width: int, x: int, y: int, w: int, h: int, pixels: bytes
) -> None:
    # canvas is packed RGB, 3 bytes/pixel. pixels is BGRX, 4 bytes/pixel.
    if w <= 0 or h <= 0:
        return
    if len(pixels) != w * h * 4:
        raise VNCError(
            f"raw rectangle {w}x{h} needed {w * h * 4} bytes, got {len(pixels)}"
        )
    for row in range(h):
        src = row * w * 4
        dst = ((y + row) * width + x) * 3
        for col in range(w):
            b = pixels[src + col * 4]
            g = pixels[src + col * 4 + 1]
            r = pixels[src + col * 4 + 2]
            canvas[dst + col * 3] = r
            canvas[dst + col * 3 + 1] = g
            canvas[dst + col * 3 + 2] = b


def read_framebuffer(
    sock: socket.socket, width: int, height: int, deadline: Deadline
) -> bytes:
    canvas = bytearray(width * height * 3)
    painted = False
    while not painted:
        kind = recvn(sock, 1, deadline)[0]
        if kind == SMSG_SET_COLORMAP:
            skip_colormap(sock, deadline)
            continue
        if kind == SMSG_BELL:
            continue
        if kind == SMSG_CUT_TEXT:
            skip_cut_text(sock, deadline)
            continue
        if kind != SMSG_FB_UPDATE:
            raise VNCError(f"unexpected server message type {kind}")
        recvn(sock, 1, deadline)  # padding
        (nrects,) = struct.unpack("!H", recvn(sock, 2, deadline))
        if nrects == 0:
            continue
        for _ in range(nrects):
            x, y, w, h, enc = struct.unpack("!HHHHi", recvn(sock, 12, deadline))
            if enc != ENC_RAW:
                raise VNCError(
                    f"server sent encoding {enc} at {w}x{h}+{x}+{y}; only Raw (0) is supported"
                )
            if x < 0 or y < 0 or x + w > width or y + h > height:
                raise VNCError(
                    f"rectangle {w}x{h}+{x}+{y} is outside {width}x{height}"
                )
            pixels = recvn(sock, w * h * 4, deadline)
            paint_raw(canvas, width, x, y, w, h, pixels)
        painted = True
    return bytes(canvas)


def save_png(path: str, width: int, height: int, rgb: bytes) -> None:
    try:
        from PIL import Image
    except ImportError as e:
        raise VNCError(
            "PIL is missing (Pillow). "
            "Do not pip-install on this machine; use the copy already present."
        ) from e
    parent = os.path.dirname(path)
    if parent:
        os.makedirs(parent, exist_ok=True)
    Image.frombytes("RGB", (width, height), rgb).save(path, "PNG")


def snap(
    host: str,
    port: int,
    out: str,
    timeout: float,
    actions: list[str] | None = None,
) -> dict:
    password = os.environ.get("VNC_PASSWORD")
    if password is None or password == "":
        raise VNCError("VNC_PASSWORD is not set")
    deadline = Deadline(timeout)
    try:
        sock = socket.create_connection((host, port), timeout=deadline.remaining())
    except OSError as e:
        raise VNCError(f"could not connect to {host}:{port}: {e}") from e
    try:
        width, height, name = handshake(sock, password, deadline)
        setup_raw(sock, deadline)
        if actions:
            run_actions(sock, actions, deadline)
        request_raw_frame(sock, width, height, deadline)
        rgb = read_framebuffer(sock, width, height, deadline)
    finally:
        try:
            sock.close()
        except OSError:
            pass
    save_png(out, width, height, rgb)
    size = os.path.getsize(out)
    return {
        "width": width,
        "height": height,
        "bytes": size,
        "out": out,
        "name": name,
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Capture one PNG from an RFB 3.8 VNC server. "
        "Password comes from VNC_PASSWORD, not from a flag."
    )
    p.add_argument("--host", required=True, help="VNC host (name or address)")
    p.add_argument("--port", required=True, type=int, help="VNC TCP port")
    p.add_argument("--out", required=True, help="PNG path to write")
    p.add_argument(
        "--timeout",
        type=float,
        default=20.0,
        help="overall deadline in seconds (default 20)",
    )
    p.add_argument(
        "--do",
        action="append",
        default=[],
        metavar="ACTION",
        help="optional input before the capture: key:Escape, combo:Super+Return, "
        "type:TEXT, enter, sleep:SEC, click:X,Y (repeatable, in order)",
    )
    args = p.parse_args(argv)
    if args.port <= 0 or args.port > 65535:
        print("vnc-snap: --port must be 1..65535", file=sys.stderr)
        return 2
    if args.timeout <= 0:
        print("vnc-snap: --timeout must be positive", file=sys.stderr)
        return 2
    try:
        info = snap(args.host, args.port, args.out, args.timeout, actions=args.do)
    except VNCError as e:
        print(f"vnc-snap: {e}", file=sys.stderr)
        return 1
    print(
        f"ok {info['width']}x{info['height']} {info['bytes']}B {info['out']} "
        f"name={info['name']}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
