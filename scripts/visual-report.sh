#!/usr/bin/env bash
# Generate a visual HTML report of all image permutations.
# Expects the test container to already be running (see test.sh).
set -euo pipefail

cd "$(dirname "$0")/.."
source "$(dirname "$0")/test-constants.sh"

BASE_URL="${BASE_URL:-http://127.0.0.1:${TEST_PORT}}"
API_KEY="${API_KEY:-${TEST_FREE_API_KEY}}"
REPORT_DIR="test-report"
IMG_DIR="$REPORT_DIR/images"

rm -rf "$REPORT_DIR"
mkdir -p "$IMG_DIR"

# Test IDs
IMDB_ID="tt0111161"
EPISODE_ID="tt0529483"

PASS=0
FAIL=0

# Download an image and return the relative path, or empty string on failure
fetch_image() {
    local label="$1"
    local endpoint="$2"
    local query="$3"
    local ext="$4"
    # Sanitise label for filename
    local filename
    filename="$(echo "$label" | tr ' /=' '_' | tr -cd 'a-zA-Z0-9_-').${ext}"
    local id_type="${5:-imdb}"
    local id_value="${6:-$IMDB_ID}"
    local url="${BASE_URL}/${API_KEY}/${id_type}/${endpoint}/${id_value}.${ext}"
    if [ -n "$query" ]; then
        url="${url}?${query}"
    fi
    if curl -sf -o "${IMG_DIR}/${filename}" "$url"; then
        PASS=$((PASS + 1))
        echo "images/${filename}"
    else
        FAIL=$((FAIL + 1))
        echo ""
    fi
}

# Start building the HTML
REPORT_FILE="$REPORT_DIR/index.html"

# We'll collect all sections in a variable
SECTIONS=""

add_section() {
    local title="$1"
    SECTIONS="${SECTIONS}<h2>${title}</h2><div class=\"grid\">"
}

end_section() {
    SECTIONS="${SECTIONS}</div>"
}

add_image() {
    local label="$1"
    local path="$2"
    if [ -n "$path" ]; then
        SECTIONS="${SECTIONS}<figure><img src=\"${path}\" loading=\"lazy\"><figcaption>${label}</figcaption></figure>"
    else
        SECTIONS="${SECTIONS}<figure><div class=\"error\">Failed</div><figcaption>${label}</figcaption></figure>"
    fi
}

echo "Fetching permutations..."

# --- Helpers ---
# Each helper takes the common parameters and emits one section. The kind +
# endpoint + ext + (optional) extra query string + (optional) id_value are
# passed explicitly so the per-kind difference (e.g. episodes need
# ratings_order=imdb,tmdb to render badges; episode ids differ from poster
# ids) stays declarative at the call site.

# render_default: emits the single-image "Default" section.
render_default() {
    local kind="$1" ext="$2" endpoint="$3"
    local id_value="${4:-$IMDB_ID}" extra="${5:-}"
    add_section "$kind - Default"
    local path=$(fetch_image "${kind,,}_default" "$endpoint" "$extra" "$ext" "imdb" "$id_value")
    add_image "default" "$path"
    end_section
}

# render_perm: emits one parameter's permutations as a section (one image per value).
render_perm() {
    local kind="$1" param="$2" values="$3" ext="$4" endpoint="$5"
    local id_value="${6:-$IMDB_ID}" extra="${7:-}"
    add_section "$kind - $param"
    for val in $values; do
        local q="${param}=${val}"
        [ -n "$extra" ] && q="${q}&${extra}"
        local path=$(fetch_image "${param}=${val}" "$endpoint" "$q" "$ext" "imdb" "$id_value")
        add_image "${param}=${val}" "$path"
    done
    end_section
}

# render_combined: emits a param1 × param2 Cartesian-product section.
render_combined() {
    local kind="$1" p1="$2" v1="$3" p2="$4" v2="$5" ext="$6" endpoint="$7"
    local id_value="${8:-$IMDB_ID}" extra="${9:-}"
    add_section "$kind - ${p1} × ${p2}"
    for a in $v1; do
        for b in $v2; do
            local q="${p1}=${a}&${p2}=${b}"
            [ -n "$extra" ] && q="${q}&${extra}"
            local path=$(fetch_image "${p1}_${a}_${p2}_${b}" "$endpoint" "$q" "$ext" "imdb" "$id_value")
            add_image "${p1}=${a} ${p2}=${b}" "$path"
        done
    done
    end_section
}

# Episodes need ratings_order=imdb,tmdb so the badges are visible (the
# default order includes one source that resolves to the same rating and
# doesn't render a second badge).
EP_EXTRA="ratings_order=imdb,tmdb"

echo "Fetching poster permutations..."
render_default "Poster" "jpg" "poster-default"
render_perm "Poster" "badge_style" "h v d" "jpg" "poster-default"
render_perm "Poster" "label_style" "t i o" "jpg" "poster-default"
render_perm "Poster" "text_size" "50 75 100 150 200" "jpg" "poster-default"
render_perm "Poster" "badge_direction" "h v d" "jpg" "poster-default"
render_perm "Poster" "position" "bc tc l r tl tr bl br" "jpg" "poster-default"
render_perm "Poster" "textless" "true false" "jpg" "poster-default"
render_perm "Poster" "image_source" "t f" "jpg" "poster-default"
render_perm "Poster" "ratings_limit" "0 1 3 5 8" "jpg" "poster-default"
render_perm "Poster" "imageSize" "medium large" "jpg" "poster-default"
render_combined "Poster" "position" "bc tc l r tl tr bl br" "badge_direction" "h v" "jpg" "poster-default"
render_combined "Poster" "badge_style" "h v" "label_style" "t i o" "jpg" "poster-default"
render_combined "Poster" "position" "bc tc l r" "text_size" "50 75 100 150 200" "jpg" "poster-default"

echo "Fetching logo permutations..."
render_default "Logo" "png" "logo-default"
render_perm "Logo" "badge_style" "h v d" "png" "logo-default"
render_perm "Logo" "label_style" "t i o" "png" "logo-default"
render_perm "Logo" "text_size" "50 75 100 150 200" "png" "logo-default"
render_perm "Logo" "image_source" "t f" "png" "logo-default"
render_perm "Logo" "ratings_limit" "0 1 3 5 8" "png" "logo-default"
render_perm "Logo" "imageSize" "medium large" "png" "logo-default"
render_combined "Logo" "badge_style" "h v" "label_style" "t i o" "png" "logo-default"

echo "Fetching backdrop permutations..."
render_default "Backdrop" "jpg" "backdrop-default"
render_perm "Backdrop" "badge_style" "h v d" "jpg" "backdrop-default"
render_perm "Backdrop" "label_style" "t i o" "jpg" "backdrop-default"
render_perm "Backdrop" "text_size" "50 75 100 150 200" "jpg" "backdrop-default"
render_perm "Backdrop" "badge_direction" "h v d" "jpg" "backdrop-default"
render_perm "Backdrop" "position" "bc tc l r tl tr bl br" "jpg" "backdrop-default"
render_perm "Backdrop" "image_source" "t f" "jpg" "backdrop-default"
render_perm "Backdrop" "ratings_limit" "0 1 3 5 8" "jpg" "backdrop-default"
render_perm "Backdrop" "imageSize" "small medium large" "jpg" "backdrop-default"
render_combined "Backdrop" "position" "bc tc l r tl tr bl br" "badge_direction" "h v" "jpg" "backdrop-default"
render_combined "Backdrop" "badge_style" "h v" "label_style" "t i o" "jpg" "backdrop-default"

echo "Fetching episode permutations..."
render_default "Episode" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "badge_style" "h v d" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "label_style" "t i o" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "text_size" "50 75 100 150 200" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "badge_direction" "h v d" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "position" "bc tc l r tl tr bl br" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "blur" "true false" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "ratings_limit" "0 1 3 5 8" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_perm "Episode" "imageSize" "medium large" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
# Combined position × badge_direction: needs ratings_limit=2 to show direction.
render_combined "Episode" "position" "bc tc l r tl tr bl br" "badge_direction" "h v" "jpg" "episode-default" "$EPISODE_ID" "ratings_limit=2&${EP_EXTRA}"
render_combined "Episode" "badge_style" "h v" "label_style" "t i o" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"
render_combined "Episode" "blur" "true false" "badge_style" "h v d" "jpg" "episode-default" "$EPISODE_ID" "$EP_EXTRA"

echo "Generating HTML report..."

cat > "$REPORT_FILE" <<'HTMLHEAD'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>OpenPosterDB Visual Test Report</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: system-ui, sans-serif; background: #111; color: #eee; padding: 2rem; }
  h1 { margin-bottom: 1rem; }
  h2 { margin: 2rem 0 1rem; border-bottom: 1px solid #333; padding-bottom: 0.5rem; }
  .grid { display: flex; flex-wrap: wrap; gap: 1rem; }
  figure { background: #222; border-radius: 8px; padding: 0.5rem; max-width: 320px; }
  figure img { max-width: 100%; height: auto; border-radius: 4px; display: block; }
  figcaption { font-size: 0.85rem; color: #aaa; margin-top: 0.4rem; text-align: center; font-family: monospace; }
  .error { background: #400; color: #f88; padding: 2rem; text-align: center; border-radius: 4px; }
  .meta { color: #888; margin-bottom: 1rem; font-size: 0.9rem; }
  .summary { margin: 1rem 0; padding: 1rem; background: #222; border-radius: 8px; font-family: monospace; }
  .summary .pass { color: #4c4; }
  .summary .fail { color: #f44; }
</style>
</head>
<body>
<h1>OpenPosterDB Visual Test Report</h1>
<p class="meta">Generated at TIMESTAMP_PLACEHOLDER using free API key (t0-free-rpdb) against tt0111161 (The Shawshank Redemption) and tt0529483 (Bonanza S1E1)</p>
<div class="summary">PASS_PLACEHOLDER passed, FAIL_PLACEHOLDER failed out of TOTAL_PLACEHOLDER images</div>
HTMLHEAD

# Replace placeholders
TIMESTAMP=$(date -Iseconds)
TOTAL=$((PASS + FAIL))
sed -i "s/TIMESTAMP_PLACEHOLDER/$TIMESTAMP/" "$REPORT_FILE"
sed -i "s/PASS_PLACEHOLDER/<span class=\"pass\">$PASS<\/span>/" "$REPORT_FILE"
sed -i "s/FAIL_PLACEHOLDER/<span class=\"fail\">$FAIL<\/span>/" "$REPORT_FILE"
sed -i "s/TOTAL_PLACEHOLDER/$TOTAL/" "$REPORT_FILE"

# Append sections
echo "$SECTIONS" >> "$REPORT_FILE"

cat >> "$REPORT_FILE" <<'HTMLFOOT'
</body>
</html>
HTMLFOOT

echo ""
echo "Visual report: $REPORT_DIR/index.html ($PASS passed, $FAIL failed out of $TOTAL)"
