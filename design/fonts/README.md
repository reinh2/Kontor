# Fonts

The design system uses Geist (sans) and Geist Mono. Variable woff2 builds are
not committed to this repository due to their binary size.

Download from https://github.com/vercel/geist-font and place these files here:

- geist-variable-latin.woff2
- geist-variable-latin-ext.woff2
- geist-mono-variable-latin.woff2
- geist-mono-variable-latin-ext.woff2

Until the files are present, the CSS falls back to the system font stack
(ui-sans-serif / ui-monospace). Nothing breaks.
