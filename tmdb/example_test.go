package tmdb_test

import (
	"context"
	"fmt"
	"os"

	"github.com/xifofo/medianame/tmdb"
)

func ExampleClient_Recognize() {
	client, err := tmdb.NewClient(tmdb.Config{Token: os.Getenv("TMDB_API_TOKEN")})
	if err != nil {
		fmt.Println(err)
		return
	}
	result, err := client.Recognize(context.Background(), "Dirty.Angels.2024.1080p.WEB-DL.mkv")
	if err != nil {
		fmt.Println(err)
		return
	}
	if result.MediaInfo != nil {
		fmt.Println(result.MediaInfo.Title, result.MediaInfo.Overview, result.MediaInfo.PosterURL)
	}
	for _, candidate := range result.Candidates {
		fmt.Println(candidate.MediaInfo.TMDBID, candidate.Conflicts)
	}
}
