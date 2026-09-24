package services

// Version is the app version of this build, set at link time from the same
// APP_VERSION the web build uses (Makefile / CI → Dockerfile):
//
//	go build -ldflags "-X openposterdb/internal/services.Version=1.3.0-dev-abc1234"
//
// Plain `go run` / `go test` builds report "dev".
var Version = "dev"

// UserAgent is the User-Agent openposterdb sends to the rating and artwork
// APIs (TMDB, MDBList, Trakt, service-key validation). Kitsu requests use
// kitsuUserAgent instead (a browser UA to pass Cloudflare's bot check).
func UserAgent() string {
	return "openposterdb/" + Version
}
