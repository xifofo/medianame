package category_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/category"
)

func TestStructuredAndJSONConfigMatchOrderedRules(t *testing.T) {
	config := category.Config{
		Movie: []category.Rule{
			{Name: "成人电影", Conditions: map[string]any{"adult": true}},
			{Name: "华语电影", Conditions: map[string]any{"original_language": []string{"zh", "cn"}}},
			{Name: "其他电影"},
		},
		TV: []category.Rule{
			{Name: "成人动画", Conditions: map[string]any{"adult": true, "genre_ids": []int{16}}},
			{Name: "国漫", Conditions: map[string]any{"genre_ids": 16, "origin_country": " CN, TW,HK "}},
			{Name: "动画番剧", Conditions: map[string]any{"genre_ids": []int{16}}},
			{Name: "近期迷你剧", Conditions: map[string]any{"type": "Miniseries", "release_year": []string{"2020-2026", "!2022"}, "origin_country": "!US"}},
			{Name: "其他剧集"},
		},
	}
	structured, err := category.New(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	jsonPolicy, err := category.ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		mediaType    medianame.MediaType
		detail, want string
	}{
		{medianame.TypeTV, `{"adult":true,"genres":[{"id":16}],"origin_country":["CN"]}`, "成人动画"},
		{medianame.TypeTV, `{"adult":false,"genre_ids":[16],"origin_country":["TW","US"]}`, "国漫"},
		{medianame.TypeTV, `{"genre_ids":[16],"origin_country":["JP"]}`, "动画番剧"},
		{medianame.TypeTV, `{"adult":true,"genre_ids":[18]}`, "其他剧集"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2024-01-01","origin_country":["KR"]}`, "近期迷你剧"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2022-01-01","origin_country":["KR"]}`, "其他剧集"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2024-01-01","origin_country":["US"]}`, "其他剧集"},
		{medianame.TypeTV, `{"type":"Miniseries","first_air_date":"2024-01-01"}`, "其他剧集"},
		{medianame.TypeMovie, `{"adult":true,"original_language":"zh"}`, "成人电影"},
		{medianame.TypeMovie, `{"adult":false,"original_language":"CN"}`, "华语电影"},
		{medianame.TypeMovie, `{"original_language":"ja"}`, "其他电影"},
		{medianame.TypeUnknown, `{"adult":true,"genre_ids":[16]}`, ""},
	} {
		for _, policy := range []*category.Policy{structured, jsonPolicy} {
			got, err := policy.MatchJSON(tt.mediaType, []byte(tt.detail))
			if err != nil || got != tt.want {
				t.Fatalf("%s %s: got=%q want=%q err=%v", tt.mediaType, tt.detail, got, tt.want, err)
			}
		}
	}
}

func TestStructuredConfigOwnsCompiledValues(t *testing.T) {
	ids := []int{16}
	conditions := map[string]any{"genre_ids": ids, "adult": true}
	config := category.Config{TV: []category.Rule{{Name: "成人动画", Conditions: conditions}, {Name: "其他"}}}
	policy, err := category.New(config)
	if err != nil {
		t.Fatal(err)
	}
	config.TV[0].Name = "改名"
	ids[0] = 18
	conditions["adult"] = false
	if got := policy.Match(medianame.TypeTV, map[string]any{"genre_ids": []int{16}, "adult": true}); got != "成人动画" {
		t.Fatalf("caller mutation changed compiled policy: %q", got)
	}
	if got := category.Default().Match(medianame.TypeTV, map[string]any{"genre_ids": []int{16}, "adult": true}); got != "动画番剧" {
		t.Fatalf("custom rules changed default policy: %q", got)
	}
}

func TestJSONConfigKeepsLargeNumericConditions(t *testing.T) {
	policy, err := category.ParseJSON([]byte(`{"movie":[{"name":"指定影片","conditions":{"id":9007199254740993}},{"name":"其他"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ detail, want string }{
		{`{"id":9007199254740993}`, "指定影片"},
		{`{"id":9007199254740992}`, "其他"},
	} {
		got, err := policy.MatchJSON(medianame.TypeMovie, []byte(tt.detail))
		if err != nil || got != tt.want {
			t.Fatalf("numeric condition lost precision: got=%q err=%v", got, err)
		}
	}
}

func TestInvalidStructuredAndJSONConfig(t *testing.T) {
	for _, value := range []any{nil, "", "!", []int{}, []any{16, nil}, [][]int{{16}}, map[string]any{"id": 16}, math.NaN(), math.Inf(1)} {
		config := category.Config{TV: []category.Rule{{Name: "A", Conditions: map[string]any{"genre_ids": value}}}}
		if _, err := category.New(config); err == nil {
			t.Errorf("accepted invalid condition: %#v", value)
		}
	}
	for _, rules := range [][]category.Rule{
		{{Name: ""}},
		{{Name: " "}},
		{{Name: "A"}, {Name: "A"}},
		{{Name: "A", Conditions: map[string]any{" ": true}}},
	} {
		if _, err := category.New(category.Config{TV: rules}); err == nil {
			t.Errorf("accepted invalid rules: %+v", rules)
		}
	}
	for _, data := range []string{
		"", "null", "[]", "{", `{"anime":[]}`, `{"tv":{}}`,
		`{"tv":[{"name":"A","condition":{"adult":true}}]}`,
		`{"tv":[{"name":"A","conditions":{"adult":null}}]}`,
		`{"tv":[{"name":"A"},{"name":"A"}]}`,
		`{"tv":[{"name":"A","conditions":{"genre_ids":[[]]}}]}`,
		`{} {}`, `{} trailing`,
	} {
		if _, err := category.ParseJSON([]byte(data)); err == nil {
			t.Errorf("accepted invalid JSON config: %s", data)
		}
	}
	for _, data := range []string{`{}`, `{"movie":[],"tv":[]}`} {
		policy, err := category.ParseJSON([]byte(data))
		if err != nil || policy.Match(medianame.TypeTV, map[string]any{"genre_ids": []int{16}}) != "" {
			t.Fatalf("empty config did not disable classification: err=%v", err)
		}
	}
}

func TestCustomJSONExampleUsesAdultPriority(t *testing.T) {
	data, err := os.ReadFile("custom.example.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := category.ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, mediaType := range []medianame.MediaType{medianame.TypeMovie, medianame.TypeTV} {
		got, err := policy.MatchJSON(mediaType, []byte(`{"adult":true,"genres":[{"id":16}],"original_language":"ja","origin_country":["JP"]}`))
		if err != nil || got != "成人动画" {
			t.Fatalf("example did not prioritize adult animation: got=%q err=%v", got, err)
		}
	}
}
