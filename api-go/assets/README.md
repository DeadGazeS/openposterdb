Binary assets required at runtime.
Copy these files from the original api/assets/ directory:

api-go/assets/
├── fonts/
│   └── Inter-Bold.ttf          (copied from api/assets/fonts/Inter-Bold.ttf)
└── icons/
    ├── imdb.png
    ├── tmdb.png
    ├── rt.png
    ├── rta.png
    ├── mc.png
    ├── trakt.png
    ├── lb.png
    ├── mal.png
    ├── mdblist.png
    ├── ebert.png
    └── official/
        ├── imdb.png
        ├── tmdb.png
        ├── metacritic.png
        ├── trakt.png
        ├── letterboxd.png
        ├── mal.webp
        ├── mdblist.png
        ├── ebert.png
        ├── Rotten_Tomatoes_critic_positive.png
        ├── Rotten_Tomatoes_critic_rotten.png
        ├── Rotten_Tomatoes_critic_certified_fresh.png
        ├── Rotten_Tomatoes_positive_audience.png
        ├── Rotten_Tomatoes_negative_audience.png
        └── Rotten_Tomatoes_verified_hot_audience.png

These are loaded at runtime by internal/image/icons.go via LoadIcons()
and internal/image/serve.go for font rendering.
