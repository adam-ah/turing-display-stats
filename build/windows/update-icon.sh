#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SVG_PATH="$ROOT_DIR/internal/win/assets/tray.svg"
ICO_PATH="$ROOT_DIR/internal/win/assets/tray.ico"
WINRES_PATH="$ROOT_DIR/build/windows/winres.json"
WINRES_ICON_REL="../../internal/win/assets/tray.ico"

echo "Updating Windows icon assets..."
echo "  SVG source:  $SVG_PATH"
echo "  ICO target:   $ICO_PATH"
echo "  winres file:  $WINRES_PATH"

if [[ ! -f "$SVG_PATH" ]]; then
  echo "Missing source SVG: $SVG_PATH" >&2
  exit 1
fi

if command -v magick >/dev/null 2>&1; then
  IM_CMD=(magick)
elif command -v convert >/dev/null 2>&1; then
  IM_CMD=(convert)
else
  echo "ImageMagick is required (magick or convert)." >&2
  exit 1
fi

echo "Using ImageMagick command: ${IM_CMD[*]}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

# Render to a large PNG first so the ICO keeps a proper high-resolution layer.
echo "Rendering SVG to a 1024x1024 PNG..."
"${IM_CMD[@]}" "$SVG_PATH" -background none -resize 1024x1024 "$tmp_dir/tray.png"
echo "Building multi-size ICO..."
"${IM_CMD[@]}" "$tmp_dir/tray.png" -background none -define icon:auto-resize=256,128,64,48,32,16 "$ICO_PATH"
echo "Wrote ICO: $ICO_PATH"

mkdir -p "$(dirname "$WINRES_PATH")"
cat >"$tmp_dir/winres.json" <<EOF
{
  "RT_GROUP_ICON": {
    "APP": {
      "0000": "$WINRES_ICON_REL"
    }
  }
}
EOF

if [[ ! -f "$WINRES_PATH" ]] || ! cmp -s "$tmp_dir/winres.json" "$WINRES_PATH"; then
  echo "Updating winres.json..."
  cp "$tmp_dir/winres.json" "$WINRES_PATH"
else
  echo "winres.json already up to date."
fi

echo "Done."
