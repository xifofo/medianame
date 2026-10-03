package shadow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xifofo/medianame"
)

const naturalHighName = "Natural High.2023.S00E131.WEB-DL.2160p.H265.DDP 2.0-ADWeb"

func TestNaturalHighParseAndMP2Aliases(t *testing.T) {
	info := medianame.Parse(naturalHighName)
	if info.Title != "Natural High" || info.Year != 2023 || info.Type != medianame.TypeTV ||
		info.Season == nil || *info.Season != 0 || info.Episode == nil || *info.Episode != 131 ||
		info.Resolution != "2160p" || info.Source != "WEB-DL" || info.VideoCodec != "H.265" ||
		info.AudioCodec != "DDP" || info.AudioChannels != "2.0" || info.ReleaseGroup != "ADWeb" {
		t.Fatalf("unexpected parse: %+v", info)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"media_info":{"title":"现在就出发","original_title":"现在就出发","en_title":"Natural High","names":["Natural High","现在就出发","現在就出發"],"year":"2023","type":"电视剧","tmdb_id":231620}}`))
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "test-token", HTTP: server.Client()}
	recognition := client.Recognize(context.Background(), naturalHighName, false)
	if recognition.Status != "identified" || recognition.Media["en_title"] != "Natural High" || recognition.Media["names"] == nil {
		t.Fatalf("lost MP2 aliases: %+v", recognition)
	}
	if conflicts := info.CheckCandidate(Candidate(recognition)); len(conflicts) != 0 {
		t.Fatalf("translation falsely rejected: %+v", conflicts)
	}
	if result := Enhance(context.Background(), client, Record{Input: Input{Name: naturalHighName}, Recognition: recognition}); result.Status != "compatible" {
		t.Fatalf("unexpected enhancement: %+v", result)
	}
}

func TestMP2AliasSourcesAndConflictChecks(t *testing.T) {
	info := medianame.Parse(naturalHighName)
	for _, tt := range []struct {
		name, wantField string
		enTitle, names  any
		year, mediaType string
		legacy          bool
	}{
		{"English title", "", "Natural High", nil, "2023", "电视剧", false},
		{"names array", "", nil, []any{"Natural High", nil, 123}, "2023", "电视剧", false},
		{"string slice", "", nil, []string{"Natural High"}, "2023", "电视剧", false},
		{"saved raw response", "", "Natural High", []any{"Natural High"}, "2023", "电视剧", true},
		{"no alias evidence", "title", nil, nil, "2023", "电视剧", false},
		{"numeric alias rejected", "title", 123, []any{123}, "2023", "电视剧", false},
		{"year conflict", "year", "Natural High", nil, "2024", "电视剧", false},
		{"type conflict", "type", "Natural High", nil, "2023", "电影", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			media := map[string]any{"title": "现在就出发", "original_title": "现在就出发", "year": tt.year, "type": tt.mediaType, "tmdb_id": "231620"}
			aliases := map[string]any{"en_title": tt.enTitle, "names": tt.names}
			recognition := &Recognition{Status: "identified", Media: media}
			if tt.legacy {
				recognition.Raw = map[string]any{"media_info": aliases}
			} else {
				media["en_title"], media["names"] = tt.enTitle, tt.names
			}
			conflicts := info.CheckCandidate(Candidate(recognition))
			if tt.wantField == "" && len(conflicts) != 0 || tt.wantField != "" && (len(conflicts) != 1 || conflicts[0].Field != tt.wantField) {
				t.Fatalf("unexpected conflicts: %+v", conflicts)
			}
		})
	}
	// 别名命中不能覆盖显式 ID 冲突。
	info = medianame.Parse(naturalHighName + " {[tmdbid=999]}")
	candidate := Candidate(&Recognition{Media: map[string]any{"title": "现在就出发", "en_title": "Natural High", "year": "2023", "type": "电视剧", "tmdb_id": "231620"}})
	if conflicts := info.CheckCandidate(candidate); len(conflicts) != 1 || conflicts[0].Field != "tmdb_id" {
		t.Fatalf("alias hid ID conflict: %+v", conflicts)
	}
}
