package image

import "testing"

// Direct artwork URLs come from Kitsu/AniList API responses; only https on
// their CDNs may be fetched (hosts checked live 2026-11-03).
func TestAllowedDirectArtworkURL(t *testing.T) {
	for url, want := range map[string]bool{
		"https://media.kitsu.app/anime/poster_images/1/original.jpg":            true,
		"https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx1.png": true,
		"http://media.kitsu.app/x.jpg":                                          false, // not https
		"https://169.254.169.254/latest/meta-data":                              false,
		"https://evil.example/kitsu.app.jpg":                                    false,
		"https://kitsu.app.evil.example/x.jpg":                                  false,
		"https://user:pw@media.kitsu.app/x.jpg":                                 false,
	} {
		if got := allowedDirectArtworkURL(url); got != want {
			t.Errorf("%s: got %v want %v", url, got, want)
		}
	}
}
