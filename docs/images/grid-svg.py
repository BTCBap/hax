#!/usr/bin/env python3
"""Snap freeze SVG text to the terminal cell grid.

freeze writes each terminal row as one <text> with run-based <tspan>s and lets
the font advance decide where glyphs land. Glyphs absent from the embedded
JetBrains Mono fall back to another font, so every following glyph on that row
shifts and box borders drift. The terminal itself places these glyphs on fixed
cells, so pin each character to an absolute x here, before Chrome screenshots
the SVG. Background <rect>s written by freeze are snapped to the same grid and
to the exact cell height, so they stay under their glyphs rather than drifting
and overhanging the next row. Block-element glyphs are replaced by cell-sized
<rect>s as well, because the font's block glyphs are taller than freeze's row
pitch and otherwise notch and overlap neighbouring rows.
"""
import sys
import unicodedata
import xml.etree.ElementTree as ET

SVG_NS = "http://www.w3.org/2000/svg"
# JetBrains Mono advance width is 600/1000 em.
ADVANCE_EM = 0.6
# freeze positions background rects on its own narrower cell width (500/840 em).
FREEZE_ADVANCE_EM = 0.5952381
DEFAULT_FONT_SIZE = 15.0
DEFAULT_PITCH = 18.0
# freeze's background rect top to the row's text y.
FREEZE_RECT_TO_TEXT_Y = 14.76
# Lower blocks: k eighths of the cell, measured from the bottom.
LOWER_BLOCKS = {
    "\u2581": 1,
    "\u2582": 2,
    "\u2583": 3,
    "\u2584": 4,
    "\u2585": 5,
    "\u2586": 6,
    "\u2587": 7,
    "\u2588": 8,
}
UPPER_HALF = "\u2580"
UPPER_EIGHTH = "\u2594"
BLOCK_CHARS = set(LOWER_BLOCKS) | {UPPER_HALF, UPPER_EIGHTH}


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
    positions = []
    col = start_col
    for ch in text:
        positions.append((ch, f"{start_x + col * cell_width:.2f}px"))
        col += char_width(ch)
    return positions, col


def cell_pitch(rows):
    ys = sorted({length(el.get("y", "0")) for el in rows})
    diffs = [b - a for a, b in zip(ys, ys[1:]) if b > a]
    return min(diffs) if diffs else DEFAULT_PITCH


def inherited_fill(el, parents):
    while el is not None:
        fill = el.get("fill")
        if fill:
            return fill
        el = parents.get(el)
    return None


def block_geometry(ch, cell_top, pitch):
    """Top y and height of a block glyph's cell-exact rect."""
    k = LOWER_BLOCKS.get(ch)
    if k is not None:
        return cell_top + pitch * (8 - k) / 8.0, pitch * k / 8.0
    if ch == UPPER_HALF:
        return cell_top, pitch / 2.0
    return cell_top, pitch / 8.0


def block_rect(ch, x, cell_top, pitch, cell_width, fill):
    y, height = block_geometry(ch, cell_top, pitch)
    rect = ET.Element("rect")
    rect.set("x", x)
    rect.set("y", f"{y:.2f}px")
    rect.set("width", f"{cell_width:.2f}px")
    rect.set("height", f"{height:.2f}px")
    # Chrome antialiases cell-exact edges into hairline seams between cells.
    rect.set("shape-rendering", "crispEdges")
    if fill:
        rect.set("fill", fill)
    return rect


def replace_blocks(text, positions, cell_top, pitch, cell_width, fill):
    """Return the text with block glyphs spaced out, plus their rects."""
    chars = []
    rects = []
    for ch, x in positions:
        if ch in BLOCK_CHARS:
            rects.append(block_rect(ch, x, cell_top, pitch, cell_width, fill))
            chars.append(" ")
        else:
            chars.append(ch)
    return "".join(chars), rects


def snap(path):
    tree = ET.parse(path)
    root = tree.getroot()

    font_size = DEFAULT_FONT_SIZE
    for el in root.iter():
        if el.get("font-size"):
            font_size = length(el.get("font-size"))
            break
    cell_width = font_size * ADVANCE_EM

    texts = [el for el in root.iter() if local_name(el.tag) == "text"]
    pitch = cell_pitch(texts)

    text_start_x = None
    for el in texts:
        text_start_x = length(el.get("x", "0"))
        break

    if text_start_x is not None:
        freeze_cw = font_size * FREEZE_ADVANCE_EM
        row_ys = [length(el.get("y", "0")) for el in texts]
        for el in root.iter():
            if local_name(el.tag) != "rect":
                continue
            x = el.get("x")
            width = el.get("width")
            if not (el.get("fill") and x and width and width.endswith("px")):
                continue
            if el.get("height") is None or el.get("y") is None:
                continue
            xv = length(x)
            if xv < text_start_x - 1:
                continue
            col = round((xv - text_start_x) / freeze_cw)
            cells = max(1, round(length(width) / freeze_cw - 0.5))
            el.set("x", f"{text_start_x + col * cell_width:.2f}px")
            el.set("width", f"{cells * cell_width:.2f}px")
            el.set("shape-rendering", "crispEdges")
            if row_ys:
                rect_y = length(el.get("y"))
                row_y = min(
                    row_ys, key=lambda y: abs(y - (rect_y + FREEZE_RECT_TO_TEXT_Y))
                )
                el.set("y", f"{row_y - pitch:.2f}px")
                el.set("height", f"{pitch:.2f}px")

    parents = {child: parent for parent in root.iter() for child in parent}
    for el in texts:
        cell_top = length(el.get("y", "0")) - pitch
        start_x = length(el.get("x", "0"))
        start_col = 0
        rects = []
        tspans = [child for child in el if local_name(child.tag) == "tspan"]
        for tspan in tspans:
            text = tspan.text or ""
            if not text:
                continue
            positions, start_col = x_positions(text, start_x, start_col, cell_width)
            tspan.set("x", " ".join(x for _, x in positions))
            fill = inherited_fill(tspan, parents)
            tspan.text, found = replace_blocks(
                text, positions, cell_top, pitch, cell_width, fill
            )
            rects.extend(found)
        if not tspans and el.text:
            positions, start_col = x_positions(el.text, start_x, start_col, cell_width)
            el.set("x", " ".join(x for _, x in positions))
            fill = inherited_fill(el, parents)
            el.text, rects = replace_blocks(
                el.text, positions, cell_top, pitch, cell_width, fill
            )
        if rects:
            parent = parents[el]
            index = list(parent).index(el)
            for offset, rect in enumerate(rects):
                parent.insert(index + offset, rect)

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
