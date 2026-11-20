package services

import (
	"regexp"

	apperr "openposterdb/internal/errors"
)

var (
	imdbFormRe       = regexp.MustCompile(`^(tt\d+|episode-tt\d+-[Ss]\d+[Ee]\d+)$`)
	tmdbFormRe       = regexp.MustCompile(`^(movie|series)-\d+$`)
	numEpisodeFormRe = regexp.MustCompile(`^episode-\d+-[Ss]\d+[Ee]\d+$`) // TMDB or TVDB show + S/E
	numberFormRe     = regexp.MustCompile(`^\d+$`)
	kitsuSlugRe      = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)
)

var idFormHelp = map[string]string{
	"imdb":  "tt0137523 or episode-tt0903747-S1E1",
	"tmdb":  "movie-550, series-1396 or episode-1396-S1E1",
	"tvdb":  "81189 or episode-81189-S3E5",
	"kitsu": "13658 or a slug such as fairy-tail-2018",
	"mal":   "a number such as 35972",
}

// EffectiveIDType is the id type a request is resolved as. The form of the
// id decides where it is unambiguous: tt… is IMDb and movie-/series- is TMDB
// whatever type the URL names (clients send ids under the wrong type; this
// replaces the blind imdb → tmdb → tvdb retry chain). A plain number and
// episode-N-S1E1 keep the URL's type where that type takes the form. An id
// no type takes is a 400. Unknown types pass through for ParseIDType.
func EffectiveIDType(idType, idValue string) (string, error) {
	help, known := idFormHelp[idType]
	if !known {
		return idType, nil
	}
	switch {
	case imdbFormRe.MatchString(idValue):
		return "imdb", nil
	case tmdbFormRe.MatchString(idValue):
		return "tmdb", nil
	case numEpisodeFormRe.MatchString(idValue):
		if idType == "tvdb" {
			return "tvdb", nil
		}
		return "tmdb", nil
	case numberFormRe.MatchString(idValue):
		if idType == "tvdb" || idType == "kitsu" || idType == "mal" {
			return idType, nil
		}
	case idType == "kitsu" && kitsuSlugRe.MatchString(idValue):
		return "kitsu", nil
	}
	return "", apperr.NewInvalidIDType("invalid " + idType + " id " + idValue + ": expected " + help)
}

// ValidateIDFormat rejects an id value no id type takes (400).
func ValidateIDFormat(idType, idValue string) error {
	_, err := EffectiveIDType(idType, idValue)
	return err
}
