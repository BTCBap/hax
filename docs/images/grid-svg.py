#!/usr/bin/env python3
"""Snap freeze SVG text to the terminal cell grid.

freeze writes each terminal row as one <text> with run-based <tspan>s and lets
the font advance decide where glyphs land. Glyphs absent from the embedded
JetBrains Mono fall back to another font, so every following glyph on that row
shifts and box borders drift. The terminal itself places these glyphs on fixed
cells, so pin each character to an absolute x here, before Chrome screenshots
the SVG.
"""
import sys
import unicodedata
import xml.etree.ElementTree as ET

SVG_NS = "http://www.w3.org/2000/svg"
# JetBrains Mono advance width is 600/1000 em.
ADVANCE_EM = 0.6
DEFAULT_FONT_SIZE = 15.0


def local_name(tag):
    return tag.rsplit("}", 1)[-1]


def length(value):
    value = value.strip()
    return float(value[:-2]) if value.endswith("px") else float(value)


def char_width(ch):
    """Terminal cell width of one character."""
    if (
        ch in ("\ufe0e", "\ufe0f", "\u200d")
        or unicodedata.combining(ch)
        or unicodedata.category(ch) in ("Mn", "Me")
    ):
        return 0
    return 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1


def x_positions(text, start_x, start_col, cell_width):
    xs = []
    col = start_col
    for ch in text:
        xs.append(f"{start_x + col * cell_width:.2f}px")
        col += char_width(ch)
    return xs, col


def snap(path):
    tree = ET.parse(path)
    root = tree.getroot()

    font_size = DEFAULT_FONT_SIZE
    for el in root.iter():
        if el.get("font-size"):
            font_size = length(el.get("font-size"))
            break
    cell_width = font_size * ADVANCE_EM

    for el in root.iter():
        if local_name(el.tag) != "text":
            continue
        start_x = length(el.get("x", "0"))
        start_col = 0
        tspans = [child for child in el if local_name(child.tag) == "tspan"]
        for tspan in tspans:
            text = tspan.text or ""
            if not text:
                continue
            xs, start_col = x_positions(text, start_x, start_col, cell_width)
            tspan.set("x", " ".join(xs))
        if not tspans and el.text:
            xs, start_col = x_positions(el.text, start_x, start_col, cell_width)
            el.set("x", " ".join(xs))

    ET.register_namespace("", SVG_NS)
    with open(path, "w", encoding="utf-8") as f:
        f.write('<?xml version="1.0" encoding="UTF-8"?>\n')
        f.write(ET.tostring(root, encoding="unicode"))
        f.write("\n")


def main():
    if len(sys.argv) != 2:
        sys.stderr.write(f"usage: {sys.argv[0]} FILE.svg\n")
        return 2
    snap(sys.argv[1])
    return 0


if __name__ == "__main__":
    sys.exit(main())
