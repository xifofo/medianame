package category_test

import (
	"strings"
	"sync"
	"testing"

	"medianame"
	"medianame/category"
)

func TestDefaultPolicyUsesTMDBDetailsAndPriority(t *testing.T) {
	for _, tt := range []struct {
		mediaType    medianame.MediaType
		detail, want string
	}{
		{medianame.TypeMovie, `{"genres":[{"id":16}],"original_language":"zh"}`, "动画电影"},
		{medianame.TypeMovie, `{"genre_ids":[28],"original_language":"zh"}`, "华语电影"},
		{medianame.TypeMovie, `{"original_language":"EN"}`, "欧美电影"},
		{medianame.TypeMovie, `{"original_language":"ko"}`, "日韩电影"},
		{medianame.TypeMovie, `{"original_language":"it"}`, "外语电影"},
		{medianame.TypeTV, `{"genres":[{"id":16}],"origin_country":["CN"]}`, "动画番剧"},
		{medianame.TypeTV, `{"genre_ids":[10767],"origin_country":["US"]}`, "综艺节目"},
		{medianame.TypeTV, `{"origin_country":["CN","US"]}`, "国产剧集"},
		{medianame.TypeTV, `{"origin_country":["GB"]}`, "欧美剧集"},
		{medianame.TypeTV, `{"origin_country":["KR"],"original_language":"en"}`, "日韩剧集"},
		{medianame.TypeTV, `{"origin_country":["BR"]}`, "其他剧集"},
		{medianame.TypeUnknown, `{"original_language":"en"}`, ""},
	} {
		got, err := category.Default().MatchJSON(tt.mediaType, []byte(tt.detail))
		if err != nil || got != tt.want {
			t.Fatalf("%s %s: got=%q want=%q err=%v", tt.mediaType, tt.detail, got, tt.want, err)
		}
	}
}

func TestCustomChineseKeysANDORCountriesRawFields(t *testing.T) {
	policy, err := category.Parse([]byte(`电影:
  国语动画:
    genre_ids: '16'
    production_countries: ' CN, TW,HK '
    original_language: [zh, cn]
  未上映:
    status: 'Planned'
    adult: false
  外语电影:
电视剧:
  新剧:
    type: 'Miniseries'
    release_year: '2020-2026,!2022'
    origin_country: '!US'
  其他剧集:
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		mediaType    medianame.MediaType
		detail, want string
	}{
		{medianame.TypeMovie, `{"genres":[{"id":16}],"production_countries":[{"iso_3166_1":"TW"}],"original_language":"zh"}`, "国语动画"},
		{medianame.TypeMovie, `{"genres":[{"id":16}],"production_countries":[{"iso_3166_1":"US"}],"original_language":"zh"}`, "外语电影"},
		{medianame.TypeMovie, `{"status":"Planned","adult":false}`, "未上映"},
		{medianame.TypeMovie, `{"status":"Released","adult":false}`, "外语电影"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2024-01-01","origin_country":["KR"]}`, "新剧"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2022-01-01","origin_country":["KR"]}`, "其他剧集"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2024-01-01"}`, "其他剧集"},
	} {
		got, err := policy.MatchJSON(tt.mediaType, []byte(tt.detail))
		if err != nil || got != tt.want {
			t.Fatalf("%s: got=%q want=%q err=%v", tt.detail, got, tt.want, err)
		}
	}
}

func TestInvalidYAMLAndMissingMatches(t *testing.T) {
	for _, text := range []string{"", "[]", "anime: {}", "movie: []", "movie: {}\n电影: {}", "movie:\n  A:\n  A:", "movie:\n  A:\n    genre_ids: 16\n    genre_ids: 99", "movie:\n  A:\n    genre_ids: ''", "movie:\n  A:\n    genre_ids: []", "movie: [", "movie: {}\n---\ntv: {}"} {
		if _, err := category.Parse([]byte(text)); err == nil {
			t.Errorf("accepted invalid YAML: %s", text)
		}
	}
	policy, err := category.Parse([]byte("movie:\n  A:\n    original_language: zh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Match(medianame.TypeMovie, map[string]any{"original_language": "en"}); got != "" {
		t.Fatal(got)
	}
	if got := category.Default().Match(medianame.TypeMovie, nil); got != "" {
		t.Fatal(got)
	}
	if _, err := policy.MatchJSON(medianame.TypeMovie, []byte("invalid")); err == nil {
		t.Fatal("accepted invalid JSON")
	}
	if _, err := policy.MatchJSON(medianame.TypeMovie, []byte(`{"original_language":"zh"} trailing`)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
	_, err = category.Parse([]byte("movie:\n  A:\n    original_language: null"))
	if err == nil || !strings.Contains(err.Error(), "第 3 行") {
		t.Fatal(err)
	}
}

func TestNumericPrecisionAndConcurrentPolicy(t *testing.T) {
	policy, err := category.Parse([]byte("movie:\n  精确 ID:\n    id: '9007199254740993'\n  其它:"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := policy.MatchJSON(medianame.TypeMovie, []byte(`{"id":9007199254740993}`))
			if err != nil || got != "精确 ID" {
				t.Errorf("got=%q err=%v", got, err)
			}
		}()
	}
	wg.Wait()
}
