#!/usr/bin/env python3
"""Erzeugt static/themes.css aus den Farbpaletten von Dashy.

Dashy beschreibt jedes Theme als Block von CSS-Variablen. Dieses Skript liest
die Quelldateien, löst die var()-Ketten gegen Dashys Standardwerte auf und
bildet das Ergebnis auf die Variablen dieser Anwendung ab.

Gehört nicht zum Build — das Ergebnis (static/themes.css) liegt im Repo. Nur
laufen lassen, wenn Themes nachgezogen werden sollen:

    python3 tools/gen-themes.py <verzeichnis-mit-dashy-scss> > static/themes.css

Erwartet color-palette.scss und themes/*.scss von
https://github.com/Lissy93/dashy (src/styles/).
"""
import re, sys, os, glob

# Nur Themes ohne eigenes Struktur-CSS: reine Farbpaletten lassen sich
# verlustfrei übertragen, alles andere wäre geraten.
PURE = [
    '_bee', '_callisto', '_catppuccin', '_crayola', '_cyberpunk', '_dracula',
    '_gruvbox', '_hacker-girl', '_high-contrast', '_midnight', '_nord',
    '_parchment', '_raspberry-jam', '_rose-pine', '_solarized', '_thebe',
    '_tiger', '_tokyo-night', '_zinc',
]

VAR = re.compile(r'--([a-z0-9-]+)\s*:\s*([^;}]+)')
BLOCK = re.compile(r"html\[data-theme=['\"]([a-z0-9-]+)['\"]\]\s*\{", re.I)
NTH = re.compile(r"nth-child\((\d)n\)\s*\{([^}]*)\}")


def root_defaults(palette_path):
    """Dashys :root-Block — die Grundwerte, von denen die Themes erben."""
    src = open(palette_path).read()
    start = src.index(':root')
    depth, i = 0, src.index('{', start)
    for j in range(i, len(src)):
        if src[j] == '{': depth += 1
        elif src[j] == '}':
            depth -= 1
            if depth == 0: break
    return dict(VAR.findall(src[i:j]))


def theme_blocks(path):
    """Alle html[data-theme='x'] { … }-Blöcke einer Datei, mit Klammerzählung."""
    src = open(path).read()
    for m in BLOCK.finditer(src):
        i = src.index('{', m.end() - 1)
        depth = 0
        for j in range(i, len(src)):
            if src[j] == '{': depth += 1
            elif src[j] == '}':
                depth -= 1
                if depth == 0: break
        yield m.group(1), src[i:j]


def resolve(value, table, seen=None):
    """Löst var(--x, fallback)-Ketten gegen die Variablentabelle auf."""
    seen = seen or set()
    for _ in range(12):
        m = re.search(r'var\(\s*--([a-z0-9-]+)\s*(?:,\s*([^)]*))?\)', value)
        if not m: break
        name, fallback = m.group(1), (m.group(2) or '').strip()
        if name in seen:
            repl = fallback
        else:
            seen = seen | {name}
            repl = table.get(name, fallback)
        value = value[:m.start()] + repl.strip() + value[m.end():]
    return value.strip()


def mix(color, other, pct):
    return f'color-mix(in srgb, {color} {pct}%, {other})'


def emit(name, v):
    """Bildet die aufgelösten Dashy-Variablen auf unsere ab."""
    bg        = v.get('background', '#0b1021')
    darker    = v.get('background-darker', bg)
    primary   = v.get('primary', '#5cabca')
    ink       = v.get('item-text-color') or v.get('foreground') or primary
    heading   = v.get('heading-text-color') or ink
    panel     = v.get('item-group-background', bg)
    surface   = v.get('item-background', darker)
    surfaceH  = v.get('item-background-hover', surface)
    danger    = v.get('danger', '#f80363')
    shadow    = v.get('item-shadow') or '0 1px 2px rgba(0,0,0,.35)'

    out = [f":root[data-theme='{name}'] {{"]
    out.append(f'  --bg: {bg}; --surface: {surface}; --surface-2: {surfaceH}; '
               f'--panel: {panel}; --panel-head: {darker};')
    out.append(f'  --ink: {ink}; --ink-soft: {mix(heading, "transparent", 82)}; '
               f'--ink-faint: {mix(heading, "transparent", 55)};')
    out.append(f'  --accent: {primary}; --accent-ink: {bg}; '
               f'--accent-wash: {mix(primary, bg, 18)};')
    out.append(f'  --border: {mix(ink, "transparent", 24)}; --danger: {danger};')
    out.append(f'  --shadow: {shadow};')
    # Dashy färbt Sektionen reihum aus einer kleinen Palette. Wir nehmen sie als
    # Rückfall für Kategorien ohne eigene Farbe.
    for i, c in enumerate(v.get('_cycle', [])[:5], start=1):
        out.append(f'  --cat-{i}: {c};')
    out.append('}')
    return '\n'.join(out)


def main(src_dir):
    root = root_defaults(os.path.join(src_dir, 'color-palette.scss'))
    print('/* Themes nach den Farbpaletten von Dashy (MIT, Lissy93/dashy).')
    print(' * Erzeugt von tools/gen-themes.py — nicht von Hand ändern.')
    print(' * Struktur (Panels, Karten) kommt aus app.css; hier stehen nur Farben. */')
    for stem in PURE:
        path = os.path.join(src_dir, 'themes', stem + '.scss')
        for name, body in theme_blocks(path):
            merged = dict(root)
            merged.update({k: v.strip() for k, v in VAR.findall(body)})
            resolved = {k: resolve(val, merged) for k, val in merged.items()}
            cycle = []
            for _, decl in NTH.findall(body):
                m = re.search(r'(?:background|--index-color)\s*:\s*([^;}]+)', decl)
                if m: cycle.append(resolve(m.group(1).strip(), merged))
            resolved['_cycle'] = cycle
            print()
            print(emit(name, resolved))


if __name__ == '__main__':
    main(sys.argv[1] if len(sys.argv) > 1 else '.')
