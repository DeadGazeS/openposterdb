package image

import (
	"openposterdb/internal/services"
)

// PosterPreviewArtwork passes the demo poster artwork through unchanged so the
// settings preview renders exactly what the real image endpoint renders: the
// fit selector feeds the original artwork into the standard fitPoster logic
// (native keeps the full poster at its natural ratio, cover centre-crops to
// 2:3, pad letterboxes, blur fills).
//
// Previously this function centre-cropped 2:3 artwork to a wide 3:2 box (and
// composed fit=native onto a 2:3 canvas over a dimmed backdrop) so the four
// fit modes looked artificially distinct. That cropped the poster — top and
// bottom cut off even under the default fit=native — diverging from the real
// render. A genuine 2:3 poster renders identically under native and cover in
// the real pipeline (that is honest, not a bug), and pad/blur still differ
// visibly. The web preview box derives its aspect-ratio from the rendered
// image's natural dimensions, so the portrait box needs no special-casing
// here.
func PosterPreviewArtwork(posterBytes []byte, _ services.PosterFit, _ uint32) ([]byte, error) {
	return posterBytes, nil
}
