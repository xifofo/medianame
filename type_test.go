package medianame_test

import (
	"testing"

	"github.com/xifofo/medianame"
)

func TestMediaTypeInference(t *testing.T) {
	for _, tt := range []struct {
		name string
		want medianame.MediaType
	}{
		{"新万圣节.2007.1080p.Remux.AVC.TrueHD.5.1-404.mkv", medianame.TypeMovie},
		{"1917.mkv", medianame.TypeMovie},
		{"Untitled Movie", medianame.TypeMovie},
		{"Stream.2024.1080p.mkv", medianame.TypeMovie},
		{"Show.S00E00.2024.mkv", medianame.TypeTV},
		{"Show.2024.S02.BluRay.mkv", medianame.TypeTV},
		{"示例剧 第二季 第十二集.mkv", medianame.TypeTV},
		{"[MoonSubs] Anime - 03v2 [1080p].mkv", medianame.TypeTV},
		{"Show.2024 {[type=tv]}.mkv", medianame.TypeTV},
		{"Film.S01E01 {[type=movie]}.mkv", medianame.TypeMovie},
		{"{[type=movies]}", medianame.TypeMovie},
		{"", medianame.TypeUnknown},
		{"1080p.mkv", medianame.TypeUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := medianame.Parse(tt.name)
			if info.Type != tt.want {
				t.Fatalf("type=%q want %q: %s", info.Type, tt.want, asJSON(info))
			}
			for _, query := range info.SearchQueries() {
				if query.Type != tt.want || medianame.Parse(query.Text).Type != tt.want {
					t.Fatalf("query lost type: %#v", query)
				}
			}
		})
	}
}

func TestPathTypeOverridesMovieDefault(t *testing.T) {
	for _, tt := range []struct {
		path string
		want medianame.MediaType
	}{
		{"/tv/Show (2024)/Season 2/Show.mkv", medianame.TypeTV},
		{"/tv/Season 0/Other Show.mkv", medianame.TypeTV},
		{"/tv/Show (2024) {[type=tv]}/Show.mkv", medianame.TypeTV},
		{"/tv/Show (2024) {[type=tv]}/Show {[type=movie]}.mkv", medianame.TypeMovie},
		{"/tv/Show (2024)/Season 2/Show {[type=movie]}.mkv", medianame.TypeMovie},
		{"/movies/Film {[type=movie]}/Film.S01E02.mkv", medianame.TypeTV},
		{"/movies/Film (2024)/Film.mkv", medianame.TypeMovie},
		{"/tv/Show (2024) {[type=tv]}/BDMV/STREAM/00001.m2ts", medianame.TypeTV},
		{"/movies/Film (2024)/BDMV/STREAM/00001.m2ts", medianame.TypeMovie},
		{"BDMV/STREAM/00001.m2ts", medianame.TypeUnknown},
		{"/", medianame.TypeUnknown},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if info := medianame.ParsePath(tt.path); info.Type != tt.want {
				t.Fatalf("type=%q want %q: %s", info.Type, tt.want, asJSON(info))
			}
		})
	}
}

func TestInferredTypeChecksCandidate(t *testing.T) {
	info := medianame.Parse("Film.2024.mkv")
	conflicts := info.CheckCandidate(medianame.MediaCandidate{Title: "Film", Year: 2024, Type: medianame.TypeTV})
	if len(conflicts) != 1 || conflicts[0].Field != "type" || conflicts[0].Expected != "movie" || conflicts[0].Actual != "tv" {
		t.Fatalf("type conflict lost: %#v", conflicts)
	}
}
