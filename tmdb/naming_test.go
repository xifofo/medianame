package tmdb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/category"
	"github.com/xifofo/medianame/tmdb"
)

func TestDetailsAddsEnglishTitleCategoryAndMovieRename(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, `{"id":123,"title":"中文片名","original_title":"原语种片名","release_date":"2024-01-01","original_language":"ja",
			"genres":[{"id":16}],"translations":{"translations":[
			{"iso_639_1":"en","iso_3166_1":"GB","data":{"title":"UK Name"}},
			{"iso_639_1":"en","iso_3166_1":"US","data":{"title":"English Name"}},
			{"iso_639_1":"en","iso_3166_1":"AU","data":{"title":"Other Name"}}]}}`)
	})
	m, err := client.Details(context.Background(), medianame.TypeMovie, "123")
	if err != nil {
		t.Fatal(err)
	}
	if m.EnglishTitle != "English Name" || m.OriginalTitle != "原语种片名" || m.Category != "动画电影" || len(m.Translations) != 3 {
		t.Fatalf("media=%+v", m)
	}
	result, err := m.Rename(medianame.Parse("Release.2024.mkv"), "")
	if err != nil || result.Path != "中文片名 (2024) {tmdb-123}/English Name.2024.mkv" {
		t.Fatalf("rename=%+v err=%v", result, err)
	}
}

func TestDefaultMovieRenamePreservesDolbyAliases(t *testing.T) {
	m := tmdb.MediaInfo{Title: "仙逆剧场版：弑仙之战", EnglishTitle: "Renegade Immortal: Battle of the Immortal Slayer",
		Type: medianame.TypeMovie, Year: 2026, TMDBID: "1599191"}
	for _, tt := range []struct{ audio, codec string }{
		{"DDP2.0", "DDP"},
		{"ddp.2.0", "DDP"},
		{"DD+2.0", "DD+"},
		{"DD2.0", "DD"},
		{"E-AC-3.2.0", "E-AC-3"},
		{"AC-3.2.0", "AC-3"},
	} {
		t.Run(tt.audio, func(t *testing.T) {
			info := medianame.Parse("Renegade.Immortal.Battle.of.the.Immortal.Slayer.2026.2160p.WEB-DL.H.265." + tt.audio + "-HHWEB.mkv")
			if info.AudioCodec != tt.codec || info.AudioChannels != "2.0" {
				t.Fatalf("audio alias/channels lost: %+v", info)
			}
			if got := m.RenameContext(info)["audioCodec"]; got != tt.codec+".2.0" {
				t.Fatalf("audioCodec=%q; want %q", got, tt.codec+".2.0")
			}
			result, err := m.Rename(info, "")
			wantName := "Renegade Immortal： Battle of the Immortal Slayer.2026.WEB-DL.2160p.H.265." + tt.codec + ".2.0-HHWEB.mkv"
			wantDirectory := "仙逆剧场版：弑仙之战 (2026) {tmdb-1599191}"
			if err != nil || result == nil || result.Name != wantName || result.Directory != wantDirectory ||
				result.Path != wantDirectory+"/"+wantName {
				t.Fatalf("rename=%+v err=%v; want %q", result, err, wantDirectory+"/"+wantName)
			}
		})
	}
}

func TestTVEnglishTranslationAndCustomCategoryPolicy(t *testing.T) {
	policy, err := category.Parse([]byte("tv:\n  韩国迷你剧:\n    type: Miniseries\n    origin_country: KR\n  其它:"))
	if err != nil {
		t.Fatal(err)
	}
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, `{"id":123,"name":"示例剧","original_name":"원제","first_air_date":"2024-01-01","type":"Miniseries","origin_country":["KR"],
			"translations":{"translations":[{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Example Show"}}]},
			"seasons":[{"season_number":0,"episode_count":4,"air_date":"2025-01-01"}]}`)
	}, func(c *tmdb.Config) { c.CategoryPolicy = policy })
	m, err := client.Details(context.Background(), medianame.TypeTV, "123")
	if err != nil {
		t.Fatal(err)
	}
	if m.Category != "韩国迷你剧" || m.EnglishTitle != "Example Show" || m.SeriesType != "Miniseries" {
		t.Fatalf("media=%+v", m)
	}
	info := medianame.Parse("Show.S00E01.mkv")
	result, err := m.Rename(info, "")
	if err != nil || result.Path != "示例剧 (2024) {tmdb-123}/Season 00/Example Show.2024.S00E01.mkv" {
		t.Fatalf("rename=%+v err=%v", result, err)
	}
	context := m.RenameContext(info)
	if context["season_year"] != "2025" || context["total_episodes"] != 4 {
		t.Fatal(context)
	}
}

func TestStructuredCategoryPolicyReturnsCustomNameAndTemplateCategory(t *testing.T) {
	policy, err := category.New(category.Config{TV: []category.Rule{
		{Name: "成人动画", Conditions: map[string]any{"adult": true, "genre_ids": []int{16}}},
		{Name: "动画番剧", Conditions: map[string]any{"genre_ids": []int{16}}},
		{Name: "其他"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, `{"id":90698,"name":"そらのいろ、みずのいろ","original_name":"そらのいろ、みずのいろ",
			"first_air_date":"2006-07-28","adult":true,"genres":[{"id":16}],
			"translations":{"translations":[{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Color of Sky, Color of Water"}}]}}`)
	}, func(c *tmdb.Config) { c.CategoryPolicy = policy })
	const name = "Color.of.Sky.Color.of.Water.2006.S01E01.1080p.BluRay.REMUX.AVC.FLAC.2.0-Misaki.[tmdb=90698].mkv"
	result, err := client.Recognize(context.Background(), name)
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil || result.MediaInfo.Category != "成人动画" {
		t.Fatalf("custom classification not returned: result=%+v err=%v", result, err)
	}
	preview, err := result.MediaInfo.Rename(result.MetaInfo, `{{.category}}/{{.title_year}}/{{.season_episode}}{{.fileExt}}`)
	if err != nil || preview.Path != "成人动画/そらのいろ、みずのいろ (2006)/S01E01.mkv" {
		t.Fatalf("custom category missing from naming template: result=%+v err=%v", preview, err)
	}
}

func TestClassificationUsesNormalizedGenresAndUnmappedRawFields(t *testing.T) {
	policy, err := category.Parse([]byte("movie:\n  自定义:\n    custom_rank: 7\n    genre_ids: 16\n    production_countries: CN\n  其它:"))
	if err != nil {
		t.Fatal(err)
	}
	m := tmdb.MediaInfo{Type: medianame.TypeMovie, Genres: []tmdb.Genre{{ID: 16}}, ProductionCountries: []tmdb.Country{{Code: "CN"}},
		Raw: json.RawMessage(`{"custom_rank":7}`)}
	got, err := m.Classify(policy)
	if err != nil || got != "自定义" {
		t.Fatalf("category=%q err=%v", got, err)
	}
	seriesPolicy, err := category.Parse([]byte("tv:\n  迷你剧:\n    type: Miniseries"))
	if err != nil {
		t.Fatal(err)
	}
	got, err = (tmdb.MediaInfo{Type: medianame.TypeTV, SeriesType: "Miniseries"}).Classify(seriesPolicy)
	if err != nil || got != "迷你剧" {
		t.Fatalf("typed series category=%q err=%v", got, err)
	}
	m.Raw = json.RawMessage(`bad`)
	if _, err := m.Classify(policy); err == nil {
		t.Fatal("accepted invalid Raw")
	}
}

func TestDefaultNamingUsesDefaultSeasonAndRequiresIdentityAndEpisode(t *testing.T) {
	m := tmdb.MediaInfo{Title: "示例剧", OriginalTitle: "Example Show", Type: medianame.TypeTV, Year: 2024, TMDBID: "123"}
	result, err := m.Rename(medianame.Parse("Show.E01.mkv"), "")
	if err != nil || result.Path != "示例剧 (2024) {tmdb-123}/Season 01/Example Show.2024.S01E01.mkv" {
		t.Fatalf("default season lost: rename=%+v err=%v", result, err)
	}
	for _, name := range []string{"Show.S01.mkv", "Show.S01E02-S02E03.mkv"} {
		if result, err := m.Rename(medianame.Parse(name), ""); err == nil {
			t.Fatalf("accepted incomplete/cross-season input: %+v", result)
		}
	}
	m.TMDBID = ""
	if _, err := m.Rename(medianame.Parse("Show.S01E01.mkv"), ""); err == nil {
		t.Fatal("missing TMDB ID accepted")
	}
}
