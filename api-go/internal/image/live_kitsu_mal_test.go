package image

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"openposterdb/internal/services"
)

// TestLive_KitsuMAL exercises the real Kitsu / AniList / TheBeastLT
// endpoints through openposterdb's own clients (no API keys needed). It is
// skipped unless OPENPOSTERDB_LIVE=1, so normal and CI runs stay offline:
//
//	OPENPOSTERDB_LIVE=1 go test -run TestLive_KitsuMAL -v ./internal/image/
//
// Reference title: Fairy Tail Final Series = Kitsu 13658 = MAL 35972 =
// IMDb tt1528406 (verified by hand 2026-09-24).
func TestLive_KitsuMAL(t *testing.T) {
	if os.Getenv("OPENPOSTERDB_LIVE") != "1" {
		t.Skip("set OPENPOSTERDB_LIVE=1 to run against the real Kitsu / AniList APIs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpClient := &http.Client{Timeout: 20 * time.Second}
	kitsu := services.NewKitsuClient(httpClient)
	anilist := services.NewAniListClient(httpClient)

	t.Run("kitsu numeric id", func(t *testing.T) {
		anime, mappings, err := kitsu.GetAnimeCtx(ctx, "13658")
		if err != nil {
			t.Fatal(err)
		}
		if anime.PosterImageOriginal == nil || anime.StartDate == nil || anime.CanonicalTitle == "" {
			t.Errorf("missing poster/start date/title: %+v", anime)
		}
		if m := services.MALIDForMapping(mappings); m == nil || *m != 35972 {
			t.Errorf("MAL mapping = %v, want 35972", m)
		}
		t.Logf("kitsu 13658: %q start=%s poster=%s", anime.CanonicalTitle, *anime.StartDate, *anime.PosterImageOriginal)
	})

	t.Run("kitsu slug", func(t *testing.T) {
		anime, _, err := kitsu.GetAnimeCtx(ctx, "fairy-tail-2018")
		if err != nil {
			t.Fatal(err)
		}
		if anime.ID != 13658 {
			t.Errorf("slug resolved to %d, want 13658", anime.ID)
		}
	})

	t.Run("mal to kitsu mapping", func(t *testing.T) {
		if id := kitsu.AnimeIDByMALCtx(ctx, 35972); id == nil || *id != 13658 {
			t.Errorf("AnimeIDByMALCtx(35972) = %v, want 13658", id)
		}
	})

	t.Run("anilist by mal id", func(t *testing.T) {
		media, err := anilist.MediaByMALCtx(ctx, 35972)
		if err != nil {
			t.Fatal(err)
		}
		if media.CoverImageExtra == nil || media.StartDate == nil || (media.TitleEnglish == "" && media.TitleRomaji == "") {
			t.Errorf("missing cover/start date/title: %+v", media)
		}
		t.Logf("anilist 35972: en=%q romaji=%q start=%s cover=%s", media.TitleEnglish, media.TitleRomaji, *media.StartDate, *media.CoverImageExtra)
	})

	t.Run("kitsu to imdb table", func(t *testing.T) {
		mapper := services.NewKitsuIMDbMapper(httpClient)
		if err := mapper.Load(ctx); err != nil {
			t.Fatal(err)
		}
		if imdb := mapper.LookupIMDB(13658); imdb == nil || *imdb != "tt1528406" {
			t.Errorf("LookupIMDB(13658) = %v, want tt1528406", imdb)
		}
		t.Logf("mapper entries: %d", mapper.Size())
	})

	// tt19861160 (Sailor Moon Cosmos, 2023) is one of the anime movies whose
	// IMDb id TMDB didn't resolve (user report 2026-09-24); the reverse table
	// lookup finds its Kitsu entry, which Kitsu types as a (lowercase) movie.
	t.Run("imdb to kitsu for an anime movie", func(t *testing.T) {
		mapper := services.NewKitsuIMDbMapper(httpClient)
		if err := mapper.Load(ctx); err != nil {
			t.Fatal(err)
		}
		id := mapper.LookupKitsuByIMDB("tt19861160")
		if id == nil || *id != 46085 {
			t.Fatalf("LookupKitsuByIMDB(tt19861160) = %v, want 46085", id)
		}
		r, err := services.ResolveIDCtx(ctx, services.IDTypeKitsu, "46085", services.IDClients{Kitsu: kitsu})
		if err != nil {
			t.Fatal(err)
		}
		if r.MediaType != services.MediaTypeMovie || r.DirectPosterURL == nil {
			t.Errorf("kitsu 46085: MediaType=%v poster=%v, want a movie with a poster", r.MediaType, r.DirectPosterURL)
		}
		t.Logf("kitsu 46085: %v, movie=%v, poster=%s", derefStr(r.Title), r.MediaType == services.MediaTypeMovie, *r.DirectPosterURL)
	})
}
