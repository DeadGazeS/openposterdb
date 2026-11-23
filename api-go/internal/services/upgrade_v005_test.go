package services

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestHasRustOnlyCacheToken(t *testing.T) {
	rust := []string{
		"imdb/tt1234567_b_f@mil.ptr.sv.lt.dv.bm.shr.bgd.zm",
		"tt1234567@mil.sh.li.bxl.shr.bgk.zl",
		"tt1@mil.pbc.sd.lo.bs.zm",
	}
	goKeys := []string{
		"imdb/tt1234567@mil.sh.li.dd.ts110.bz120.shr.ba80.zm",
		"imdb/tt1234567_b_f@mil.sv.lt.dv.ly1a2b3c4d.shr.ba0.eh5.zl.nk",
		"tmdb/movie-1@m.sv.lo.dv.shp.ba80.blur.zm.re2",
		"imdb/tt1@mil.sv.lt.bh120.bw90.ls130.shr.ba80.zm",
	}
	for _, k := range rust {
		if !HasRustOnlyCacheToken(k) {
			t.Errorf("%q not detected as Rust-era", k)
		}
	}
	for _, k := range goKeys {
		if HasRustOnlyCacheToken(k) {
			t.Errorf("%q wrongly detected as Rust-era", k)
		}
	}
}

func TestCacheKeyRenderEpoch(t *testing.T) {
	cases := map[string]int{
		"imdb/tt1@mil.sv.lt.zm":         0,
		"imdb/tt1@mil.sv.lt.zm.re3":     3,
		"imdb/tt1@mil.sv.lt.zm.nk.re12": 12,
		"imdb/tt1@mil.sv.lt.zm.rex":     0,
	}
	for k, want := range cases {
		if got := cacheKeyRenderEpoch(k); got != want {
			t.Errorf("%q: got %d, want %d", k, got, want)
		}
	}
}

func TestRenderEpochCacheSuffix(t *testing.T) {
	s := DefaultRenderSettings()
	suffix := SettingsCacheSuffix(&s, "poster", nil)
	if got := cacheKeyRenderEpoch("imdb/tt1" + suffix); got != RenderEpoch {
		t.Errorf("suffix %q carries epoch %d, want %d", suffix, got, RenderEpoch)
	}
	if HasRustOnlyCacheToken("imdb/tt1" + suffix) {
		t.Errorf("current suffix %q looks Rust-era", suffix)
	}
}

// TestUpgradeV005_RemovesRustRenders: rendered files and image_meta rows with
// Rust-only tokens are deleted; current renders and the base-image cache stay.
func TestUpgradeV005_RemovesRustRenders(t *testing.T) {
	db := upgradeTestDB(t)
	cacheDir := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(cacheDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	rustKey := "imdb/tt1@mil.ptr.sv.lt.dv.bm.shr.bgd.zm"
	goKey := "imdb/tt1@mil.sh.li.dd.shr.ba80.zm" + RenderEpochCacheSuffix() // current epoch
	write("posters/" + rustKey + ".jpg")
	write("posters/" + goKey + ".jpg")
	write("backdrops/imdb/tt2_b_f@mil.ptr.sv.lt.dv.bl.shr.bgd.zm.jpg")
	write("base/posters/w780/abc.bm.jpg") // base cache is never swept
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES (?, 1, 1, 'p'), (?, 1, 1, 'p')`, rustKey, goKey); err != nil {
		t.Fatal(err)
	}

	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatal(err)
	}

	var keys []string
	rows, err := db.Query("SELECT cache_key FROM image_meta")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	if len(keys) != 1 || keys[0] != goKey {
		t.Errorf("image_meta after v005: %v, want [%s]", keys, goKey)
	}

	var files []string
	filepath.WalkDir(cacheDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(cacheDir, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	want := []string{"base/posters/w780/abc.bm.jpg", "posters/" + goKey + ".jpg"}
	if len(files) != len(want) || files[0] != want[0] || files[1] != want[1] {
		t.Errorf("files after v005: %v, want %v", files, want)
	}
}

func TestSweepRenderCache_EpochPredicate(t *testing.T) {
	db := upgradeTestDB(t)
	cacheDir := t.TempDir()
	old, cur := "imdb/tt1@mil.sv.zm", "imdb/tt1@mil.sv.zm.re2"
	for _, k := range []string{old, cur} {
		p := filepath.Join(cacheDir, "posters", filepath.FromSlash(k)+".jpg")
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at) VALUES (?, 1, 1)`, k); err != nil {
			t.Fatal(err)
		}
	}
	n, f, err := sweepRenderCacheCtx(context.Background(), db, cacheDir, false, func(k string) bool {
		return cacheKeyRenderEpoch(k) != 2
	})
	if err != nil || n != 1 || f != 1 {
		t.Fatalf("sweep: rows=%d files=%d err=%v, want 1/1/nil", n, f, err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "posters", filepath.FromSlash(cur)+".jpg")); err != nil {
		t.Errorf("current-epoch file removed: %v", err)
	}
}
