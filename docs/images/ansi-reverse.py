#!/usr/bin/env python3
"""Resolve SGR reverse video to explicit colors in a .ansi render.

Some screens paint partial cells by enabling reverse video (SGR 7), so the cell
shows the real terminal background. Real terminals swap the current foreground
and background when they draw it, but freeze ignores SGR 7 and renders the
foreground as-is. Rewrite each SGR sequence in place with the swapped colors
already applied, keeping the text bytes untouched, so freeze sees explicit
colors and the screenshot matches a terminal.
"""
import argparse
import re
import sys

DEFAULT_BG = "#1e1e2e"
DEFAULT_FG = "#cdd6f4"
SGR_RE = re.compile(rb"\x1b\[([0-9;]*)m")

# A color is ("rgb", r, g, b) or ("idx", n); None means the terminal default.
Color = tuple


def parse_hex(value):
    value = value.lstrip("#")
    if len(value) != 6:
        raise argparse.ArgumentTypeError(f"expected RRGGBB, got {value!r}")
    try:
        return ("rgb", int(value[0:2], 16), int(value[2:4], 16), int(value[4:6], 16))
    except ValueError:
        raise argparse.ArgumentTypeError(f"expected RRGGBB, got {value!r}")


def color_code(channel, color):
    if color[0] == "rgb":
        return f"{channel};2;{color[1]};{color[2]};{color[3]}"
    return f"{channel};5;{color[1]}"


def rewrite_sgr(match, state, theme_bg, theme_fg):
    params = [int(p) for p in match.group(1).split(b";") if p] or [0]
    others = []
    reset = False
    fg, bg, reverse = state
    i = 0
    while i < len(params):
        p = params[i]
        if p == 0:
            reset = True
            fg, bg, reverse = None, None, False
            others = []
            i += 1
        elif p == 7:
            reverse = True
            i += 1
        elif p == 27:
            reverse = False
            i += 1
        elif p == 39:
            fg = None
            i += 1
        elif p == 49:
            bg = None
            i += 1
        elif p in (38, 48) and i + 1 < len(params):
            kind = params[i + 1]
            if kind == 2 and i + 4 < len(params):
                color = ("rgb", params[i + 2], params[i + 3], params[i + 4])
                i += 5
            elif kind == 5 and i + 2 < len(params):
                color = ("idx", params[i + 2])
                i += 3
            else:
                others.extend(params[i:])
                break
            if p == 38:
                fg = color
            else:
                bg = color
        else:
            others.append(p)
            i += 1

    out = ["0"] if reset else []
    out.extend(str(p) for p in others)
    if reverse:
        out.append(color_code(38, bg or theme_bg))
        out.append(color_code(48, fg or theme_fg))
    else:
        out.append(color_code(38, fg) if fg else "39")
        out.append(color_code(48, bg) if bg else "49")
    return ("\x1b[" + ";".join(out) + "m").encode(), (fg, bg, reverse)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("path", help=".ansi file to rewrite in place")
    parser.add_argument("--bg", type=parse_hex, default=parse_hex(DEFAULT_BG),
                        help=f"theme background (default {DEFAULT_BG})")
    parser.add_argument("--fg", type=parse_hex, default=parse_hex(DEFAULT_FG),
                        help=f"theme foreground (default {DEFAULT_FG})")
    args = parser.parse_args()

    with open(args.path, "rb") as f:
        data = f.read()

    state = (None, None, False)

    def replace(match):
        nonlocal state
        replacement, state = rewrite_sgr(match, state, args.bg, args.fg)
        return replacement

    with open(args.path, "wb") as f:
        f.write(SGR_RE.sub(replace, data))


if __name__ == "__main__":
    sys.exit(main())
