Binary assets required at runtime, loaded relative to the working directory
(the Docker image copies them next to the server binary).

api-go/assets/
├── fonts/
│   └── Inter-Bold.ttf          badge text (internal/image LoadFont)
└── icons/
    ├── white/                  "white icons" label style (i)
    │   ├── imdb.png, tmdb.png, rt.png, rta.png, mc.png,
    │   └── trakt.png, lb.png, mal.png, mdblist.png, ebert.png
    ├── official/               "official logos" label style (o)
    │   ├── imdb.png, tmdb.png, metacritic.png, trakt.png, letterboxd.png,
    │   ├── mal.webp, mdblist.png, ebert.png
    │   └── Rotten_Tomatoes_{critic_positive, critic_rotten,
    │       critic_certified_fresh, positive_audience, negative_audience,
    │       verified_hot_audience}.png
    └── highRes/                "high-res logos" label style (h): source SVGs
                                named like the official PNGs, rasterized at
                                the badge size

Icons are loaded at startup by internal/image/icons.go (LoadIcons).
