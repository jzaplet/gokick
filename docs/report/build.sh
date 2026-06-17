#!/usr/bin/env bash
#
# Rebuild the gokick assessment report PDF.
#
#   ./build.sh
#
# What it does:
#   1) embeds mascot_card.png (the baked cover image) as a data URI into report.html
#   2) renders the result to ../gokick-hodnoceni.pdf via headless Google Chrome
#
# Requirements: Google Chrome + python3 (only the stdlib).
#   Override the Chrome path with:  CHROME=/path/to/chrome ./build.sh
#
# To edit the report, change report.html and re-run this script.
# To regenerate the cover image (only if the mascot art changes), run make-cover.py first.
#
set -euo pipefail
cd "$(dirname "$0")"

CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
OUT="../gokick-hodnoceni.pdf"

if [ ! -x "$CHROME" ]; then
  echo "Chrome not found at: $CHROME" >&2
  echo "Set CHROME=/path/to/your/chrome and retry." >&2
  exit 1
fi

# 1) embed the cover image into the template
python3 - <<'PY'
import base64, pathlib
b64 = base64.b64encode(pathlib.Path("mascot_card.png").read_bytes()).decode()
html = pathlib.Path("report.html").read_text()
if "__MASCOT_SRC__" not in html:
    raise SystemExit("report.html is missing the __MASCOT_SRC__ placeholder")
html = html.replace("__MASCOT_SRC__", f"data:image/png;base64,{b64}")
pathlib.Path("report_final.html").write_text(html)
print("embedded cover image into report_final.html")
PY

# 2) render to PDF (Chrome --print-to-pdf is synchronous)
"$CHROME" --headless=new --disable-gpu --no-pdf-header-footer \
  --print-to-pdf="$OUT" \
  "file://$PWD/report_final.html" >/dev/null 2>&1

# 3) drop the large intermediate file
rm -f report_final.html

echo "→ wrote $OUT"
