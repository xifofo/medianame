package medianame_test

import (
	"fmt"
	"testing"

	"medianame"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		year   int
		season *int
		tmdb   string
	}{
		{"/tv/Example Show (2024) [tmdb=123]/Season 2/03.mkv", "Example Show", 2024, ptr(2), "123"},
		{`C:\tv\Example Show (2024) {tmdb-123}\Season 0\S00E01.mkv`, "Example Show", 2024, ptr(0), "123"},
		{"/tv/Example Show (2024) [tmdbid=123]/Season 1/Example.Show.S01E02.mkv", "Example Show", 2024, ptr(1), "123"},
		{"/tv/Example Show (2024) [tmdbid=123]/Season 1/Example.Show.S02E02.[tmdb=456].mkv", "Example Show", 2024, ptr(2), "456"},
		{"/movies/Movie Collection (2024) [tmdb=123]/Arrival.2016.1080p.mkv", "Arrival", 2016, nil, ""},
		{"/movies/Arrival (2016) [tmdb=123]/Arrival.2017.1080p.mkv", "Arrival", 2017, nil, "123"},
		{"Show.S01E01.mkv", "Show", 0, ptr(1), ""},
		{"/Show.S01E01.mkv", "Show", 0, ptr(1), ""},
		{"", "", 0, nil, ""},
		{"/tv/Example Show (2024)/Season 0/03.MKV", "Example Show", 2024, ptr(0), ""},
		{"/tv/Season 2/Other Show.mkv", "Other Show", 0, ptr(2), ""},
		{"Show.E03.mkv", "Show", 0, ptr(1), ""},
		{"/tv/Show/Show.E03.mkv", "Show", 0, ptr(1), ""},
		{"/tv/Show/Season 2/Show.E03.mkv", "Show", 0, ptr(2), ""},
		{"/tv/Show/Season 0/Show.E03.mkv", "Show", 0, ptr(0), ""},
		{"/tv/Show.S02/Show.E03.mkv", "Show", 0, ptr(2), ""},
		{"/tv/Show.S00/Show.E03.mkv", "Show", 0, ptr(0), ""},
		{"/tv/Show/Season 2/Show.S00E03.mkv", "Show", 0, ptr(0), ""},
		{"/tv/Show/Season 0/Show.S02E03.mkv", "Show", 0, ptr(2), ""},
		{"/tv/Show {[type=tv]}/Show.mkv", "Show", 0, ptr(1), ""},
		{"/tv/Show {[type=tv]}/BDMV/STREAM/00001.m2ts", "Show", 0, ptr(1), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianame.ParsePath(tt.name)
			if got.Original != tt.name || got.Title != tt.title || got.Year != tt.year || got.IDs.TMDB != tt.tmdb ||
				intValue(got.Season) != intValue(tt.season) {
				t.Errorf("unexpected result: %s; want title=%q year=%d season=%v tmdb=%q", asJSON(got), tt.title, tt.year, intValue(tt.season), tt.tmdb)
			}
		})
	}
}

func TestEmbyIDTagsInFileAndFolderNames(t *testing.T) {
	providers := []struct {
		name  string
		value string
		want  medianame.MediaIDs
	}{
		{"tmdb", "67018", medianame.MediaIDs{TMDB: "67018"}},
		{"tvdb", "12345", medianame.MediaIDs{TVDB: "12345"}},
		{"imdb", "tt0073589", medianame.MediaIDs{IMDb: "tt0073589"}},
	}
	for _, provider := range providers {
		parentValue := "999"
		if provider.name == "imdb" {
			parentValue = "tt999"
		}
		for _, alias := range []string{provider.name, provider.name + "id"} {
			for _, brackets := range [][2]string{{"[", "]"}, {"{", "}"}} {
				for _, separator := range []string{"=", "-"} {
					tag := brackets[0] + alias + separator + provider.value + brackets[1]
					t.Run(tag, func(t *testing.T) {
						folder := "Name (1975) " + tag
						for _, input := range []string{
							folder,
							"/movies/" + folder + "/Name.1975.mkv",
							"/movies/Name (1975)/Name (1975) " + tag + ".mkv",
							"/tv/" + folder + "/Season 1/01.mkv",
						} {
							info := medianame.ParsePath(input)
							if info.Title != "Name" || info.Year != 1975 || info.IDs != provider.want || info.Original != input {
								t.Errorf("ID tag in file/folder not preserved: %s", asJSON(info))
							}
						}
						// 文件中的明确 ID 优先；合集目录不会向另一部作品传递 ID。
						file := fmt.Sprintf("/movies/Name (1975) [%s=%s]/Name (1975) %s.mkv", provider.name, parentValue, tag)
						if info := medianame.ParsePath(file); info.IDs != provider.want {
							t.Errorf("folder ID overwrote file ID: %s", asJSON(info))
						}
						collection := "/movies/Collection " + tag + "/Name.1975.mkv"
						if info := medianame.ParsePath(collection); info.IDs != (medianame.MediaIDs{}) {
							t.Errorf("unrelated collection ID inherited: %s", asJSON(info))
						}
						if info := medianame.ParsePath("/movies/Name (1975) " + tag + "/Other.Name.1975.mkv"); info.IDs != (medianame.MediaIDs{}) {
							t.Errorf("another title inherited folder ID: %s", asJSON(info))
						}
						if info := medianame.ParsePath("/movies/Name (1975)/Name (1975) " + tag + ".mkv"); info.Type != medianame.TypeMovie {
							t.Errorf("ID tag changed media type: %s", asJSON(info))
						}
					})
				}
			}
		}
	}
}
