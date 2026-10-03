package tmdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/tmdb"
)

func clientFor(t *testing.T, handler http.HandlerFunc, options ...func(*tmdb.Config)) *tmdb.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config := tmdb.Config{Token: "test-token", BaseURL: server.URL + "/3"}
	for _, option := range options {
		option(&config)
	}
	client, err := tmdb.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func respond(w http.ResponseWriter, value string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, value)
}

func TestDetailsReturnsFullMovieAndOriginalResponse(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/movie/1043905" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-token" ||
			r.URL.Query().Get("language") != "zh-CN" || r.URL.Query().Get("api_key") != "" ||
			r.URL.Query().Get("append_to_response") != "external_ids,alternative_titles,translations,credits" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		respond(w, `{"id":1043905,"title":"肮脏天使","original_title":"Dirty Angels","release_date":"2024-12-11",
			"overview":"行动电影简介","poster_path":"/poster.jpg","backdrop_path":"/backdrop.jpg","vote_average":6.1,"vote_count":55,
			"genres":[{"id":28,"name":"动作"}],"runtime":104,"original_language":"en","status":"Released",
			"production_countries":[{"iso_3166_1":"US","name":"美国"}],"belongs_to_collection":{"id":12,"name":"电影系列"},
			"external_ids":{"imdb_id":"tt15450946","tvdb_id":null},"alternative_titles":{"titles":[{"title":"骯髒天使"}]},
			"translations":{"translations":[{"data":{"title":"另一译名"}},{"data":{"title":"骯髒天使"}}]},
			"credits":{"cast":[{"id":1,"name":"Actor","character":"A"}],"crew":[{"id":2,"name":"Director","job":"Director"}]},
			"budget":35000000}`)
	})
	result, err := client.Recognize(context.Background(), "肮脏天使.2024.[tmdb=1043905].{[type=movie]}.mkv")
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	m := result.MediaInfo
	if m.TMDBID != "1043905" || m.IMDbID != "tt15450946" || m.Year != 2024 || m.Overview != "行动电影简介" ||
		m.PosterURL != "https://image.tmdb.org/t/p/original/poster.jpg" || m.BackdropURL == "" ||
		m.TMDBURL != "https://www.themoviedb.org/movie/1043905" || len(m.Aliases) != 2 ||
		m.GenreIDs[0] != 28 || m.Runtime != 104 || m.VoteAverage != 6.1 || len(m.Credits.Cast) != 1 || m.Collection.ID != 12 {
		t.Fatalf("missing media fields: %+v", m)
	}
	var raw map[string]any
	if err := json.Unmarshal(m.Raw, &raw); err != nil || raw["budget"] != float64(35000000) {
		t.Fatal("original response not preserved")
	}
}

func TestRecoveredTMDBIDUsesDetailsWithoutNameSearch(t *testing.T) {
	var requests []string
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path != "/3/movie/67018" {
			t.Errorf("explicit ID fell back to name search: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		respond(w, `{"id":67018,"title":"映像","original_title":"The Image","release_date":"1975-05-20",
			"external_ids":{"imdb_id":"tt0073589"},"alternative_titles":{"titles":[{"title":"The Punishment of Anne"}]},
			"translations":{"translations":[{"iso_639_1":"zh","data":{"title":"安妮的惩罚"}}]},
			"credits":{"cast":[{"id":1,"name":"Actor"}]}}`)
	})
	const path = "搬运整理/欧美电影搬运/映像 (1975)/映像 (1975) - 映像 (1974)｛tmdbid-67018）.mkv"
	result, err := client.RecognizePath(context.Background(), path)
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil || result.Truncated ||
		len(result.Candidates) != 1 || len(requests) != 1 {
		t.Fatalf("result=%+v requests=%v err=%v", result, requests, err)
	}
	media := result.MediaInfo
	if media.TMDBID != "67018" || media.Title != "映像" || media.OriginalTitle != "The Image" || media.Year != 1975 ||
		media.IMDbID != "tt0073589" || len(media.Aliases) != 2 || len(media.Raw) == 0 || len(media.Credits.Cast) != 1 ||
		result.MetaInfo.Original != path || result.MetaInfo.IDs.TMDB != "67018" || len(result.MetaInfo.Warnings) != 1 ||
		result.MetaInfo.Warnings[0] != "conflicting_years" || len(result.Candidates[0].Conflicts) != 0 {
		t.Fatalf("TMDB details or original evidence lost: %+v", result)
	}
	resolvedYear := false
	for _, conflict := range result.Candidates[0].ResolvedConflicts {
		resolvedYear = resolvedYear || conflict.Field == "input" && conflict.Actual == "conflicting_years"
	}
	if !resolvedYear {
		t.Fatal("file year conflict was not retained as a resolved difference")
	}
	preview, err := media.Rename(result.MetaInfo, "")
	if err != nil || preview.Directory != "映像 (1975) {tmdb-67018}" || preview.Name != "The Image.1975.mkv" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
}

func TestTVPathUsesSeriesDetailsAndPreservesSpecialSeason(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/tv/123" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		respond(w, `{"id":123,"name":"示例剧","original_name":"Example Show","first_air_date":"2024-01-03","last_air_date":"2025-02-03",
			"number_of_episodes":28,"number_of_seasons":2,"in_production":false,"episode_run_time":[45],
			"seasons":[{"id":1,"name":"特别篇","season_number":0,"episode_count":1},{"id":2,"season_number":1,"episode_count":27}],
			"external_ids":{"imdb_id":"tt1234567","tvdb_id":456},"alternative_titles":{"results":[{"title":"Example Series"}]},
			"translations":{"translations":[{"data":{"name":"示例节目"}}]},"networks":[{"id":7,"name":"Network"}]}`)
	})
	result, err := client.RecognizePath(context.Background(), "/tv/示例剧 (2024) [tmdb=123]/Season 0/01.mkv")
	if err != nil || result.MediaInfo == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	m := result.MediaInfo
	if m.Type != medianame.TypeTV || m.Title != "示例剧" || m.OriginalTitle != "Example Show" || m.Year != 2024 ||
		m.NumberOfEpisodes != 28 || m.NumberOfSeasons != 2 || m.TVDBID != "456" || m.Seasons[0].SeasonNumber != 0 ||
		m.InProduction == nil || *m.InProduction || len(m.Aliases) != 2 || m.Networks[0].ID != 7 ||
		result.MetaInfo.Season == nil || *result.MetaInfo.Season != 0 || *result.MetaInfo.Episode != 1 {
		t.Fatalf("missing TV fields: %+v", result)
	}
}

func TestLongAnimeEpisodeUsesTVAndPreservesExplicitIdentityHint(t *testing.T) {
	const name = `[SBSUB][CONAN][1214][WEBRIP][1080P][HEVC_AAC][CHS_CHT_JP][PGS]\(C9B12A40).mkv`
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/tv/30983" {
			t.Errorf("anime episode queried as movie or by short name: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		respond(w, `{"id":30983,"name":"名侦探柯南","original_name":"名探偵コナン","first_air_date":"1996-01-08",
			"genres":[{"id":16,"name":"动画"}],"number_of_episodes":1216,"number_of_seasons":1,
			"seasons":[{"season_number":1,"episode_count":1216}],
			"alternative_titles":{"results":[{"title":"Detective Conan"}]}}`)
	})
	parsed := medianame.Parse(name)
	info := parsed
	// 名称和动画类型已由调用方确认；提供身份线索不改写原始文件名与集号。
	info.IDs.TMDB = "30983"
	result, err := client.RecognizeInfo(context.Background(), info)
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil || result.MediaInfo.TMDBID != "30983" ||
		result.MediaInfo.Title != "名侦探柯南" || result.MediaInfo.Category != "动画番剧" ||
		result.MetaInfo.Original != name || result.MetaInfo.Episode == nil || *result.MetaInfo.Episode != 1214 ||
		result.MetaInfo.Season == nil || *result.MetaInfo.Season != 1 ||
		parsed.IDs.TMDB != "" || result.MetaInfo.ReleaseGroup != "SBSUB" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	preview, err := result.MediaInfo.Rename(parsed, `{{.title_year}} {tmdb-{{.tmdbid}}}/{{.en_title}}.E{{.episode}}{{.fileExt}}`)
	if err != nil || !strings.Contains(preview.Name, ".E1214.mkv") || !strings.Contains(preview.Directory, "{tmdb-30983}") {
		t.Fatalf("absolute episode changed: preview=%+v err=%v", preview, err)
	}
	preview, err = result.MediaInfo.Rename(result.MetaInfo, "")
	if err != nil || !strings.HasSuffix(preview.Directory, "/Season 01") || !strings.Contains(preview.Name, ".S01E1214.") {
		t.Fatalf("default season lost in naming: preview=%+v err=%v", preview, err)
	}
}

func TestRecognizeInfoDefaultsSeasonAndPreservesZero(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/tv/123" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		respond(w, `{"id":123,"name":"Show","first_air_date":"2024-01-01"}`)
	})
	zero, two := 0, 2
	for _, tt := range []struct {
		name   string
		season *int
		want   int
	}{
		{"missing", nil, 1},
		{"special", &zero, 0},
		{"second", &two, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := medianame.Info{Title: "Show", Type: medianame.TypeTV, Season: tt.season, IDs: medianame.MediaIDs{TMDB: "123"}}
			result, err := client.RecognizeInfo(context.Background(), info)
			if err != nil || result.Status != tmdb.StatusCompatible || result.MetaInfo.Season == nil || *result.MetaInfo.Season != tt.want {
				t.Fatalf("season lost: result=%+v err=%v", result, err)
			}
			data, err := json.Marshal(result)
			var output struct {
				MetaInfo struct {
					Season *int `json:"season"`
				} `json:"meta_info"`
			}
			if err != nil || json.Unmarshal(data, &output) != nil || output.MetaInfo.Season == nil || *output.MetaInfo.Season != tt.want {
				t.Fatalf("season omitted in recognition JSON: %s err=%v", data, err)
			}
			if info.Season != tt.season || (info.Season != nil && *info.Season != tt.want) {
				t.Fatal("caller season changed")
			}
		})
	}
}

func TestShortAnimeNameDoesNotAdoptPartialSearchResult(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/tv":
			if r.URL.Query().Get("query") != "CONAN" {
				t.Errorf("group or episode entered the query: %v", r.URL.Query())
			}
			respond(w, `{"total_pages":2,"total_results":30,"results":[{"id":32415,"name":"柯南秀"},{"id":30983,"name":"名侦探柯南"}]}`)
		case "/3/tv/32415":
			respond(w, `{"id":32415,"name":"柯南秀","original_name":"Conan","first_air_date":"2010-11-08"}`)
		case "/3/tv/30983":
			respond(w, `{"id":30983,"name":"名侦探柯南","original_name":"名探偵コナン","first_air_date":"1996-01-08",
				"genres":[{"id":16,"name":"动画"}],"alternative_titles":{"results":[{"title":"Detective Conan"}]}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	result, err := client.Recognize(context.Background(), "[SBSUB][CONAN][1214][WEBRIP][1080P].mkv")
	if err != nil || result.Status != tmdb.StatusReview || !result.Truncated || result.MediaInfo != nil ||
		result.MetaInfo.Episode == nil || *result.MetaInfo.Episode != 1214 || len(result.Candidates) != 2 {
		t.Fatalf("partial short-name search adopted: %+v err=%v", result, err)
	}
}

func TestSearchReturnsDetailsAndDeduplicatesAliases(t *testing.T) {
	var mu sync.Mutex
	details := 0
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			query := r.URL.Query()
			if query.Get("year") != "2006" || query.Get("include_adult") != "true" || query.Get("query") == "" ||
				strings.Contains(query.Get("query"), "2006") {
				t.Errorf("wrong search parameters: %v", query)
			}
			respond(w, `{"total_pages":1,"total_results":1,"results":[{"id":9,"title":"工厂女孩","release_date":"2006-12-29"}]}`)
		case "/3/movie/9":
			mu.Lock()
			details++
			mu.Unlock()
			respond(w, `{"id":9,"title":"工厂女孩","original_title":"Factory Girl","release_date":"2006-12-29","overview":"完整简介","runtime":90}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, func(c *tmdb.Config) { c.IncludeAdult = true })
	info := medianame.Parse("纵情女郎 (2006) - [工厂女孩].Factory.Girl.2006.1080p.mkv")
	result, err := client.RecognizeInfo(context.Background(), info)
	if err != nil || result.Status != tmdb.StatusCompatible || len(result.Candidates) != 1 || details != 1 ||
		result.MediaInfo.Overview != "完整简介" || result.MediaInfo.Runtime != 90 {
		t.Fatalf("result=%+v calls=%d err=%v", result, details, err)
	}
}

func TestOfficialAliasesUseTMDBMetadataAndPreserveDifferences(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, `{"id":9,"title":"官方片名","original_title":"Official Title","release_date":"2024-01-01",
			"alternative_titles":{"titles":[{"title":"发行译名"}]}}`)
	})
	for _, tt := range []struct {
		year     int
		warnings []string
		status   tmdb.Status
		field    string
	}{
		{2024, nil, tmdb.StatusCompatible, ""},
		{2023, nil, tmdb.StatusCompatible, "year"},
		{2024, []string{"conflicting_years"}, tmdb.StatusCompatible, "input"},
	} {
		info := medianame.Info{Title: "发行译名", Type: medianame.TypeMovie, Year: tt.year, Warnings: tt.warnings,
			IDs: medianame.MediaIDs{TMDB: "9"}}
		result, err := client.RecognizeInfo(context.Background(), info)
		if err != nil || result.Status != tt.status || len(result.Candidates) != 1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if result.MediaInfo == nil || result.MediaInfo.Year != 2024 || result.MetaInfo.Year != tt.year || len(result.Candidates[0].Conflicts) != 0 {
			t.Fatalf("TMDB metadata not adopted: %+v", result)
		}
		if tt.field != "" && (len(result.Candidates[0].ResolvedConflicts) != 1 || result.Candidates[0].ResolvedConflicts[0].Field != tt.field) {
			t.Fatal(result.Candidates[0].ResolvedConflicts)
		}
	}
	// 无显式 ID 时官方别名也能校验，但不能屏蔽其他身份线索。
	candidate := medianame.MediaCandidate{Title: "官方片名", Aliases: []string{"发行译名"}, Year: 2024, Type: medianame.TypeMovie}
	if got := (medianame.Info{Title: "发行译名", Year: 2024, Type: medianame.TypeMovie}).CheckCandidate(candidate); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestUnknownTypeSameNumericIDIsAmbiguous(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/movie/123":
			respond(w, `{"id":123,"title":"Film","release_date":"2024-01-01"}`)
		case "/3/tv/123":
			respond(w, `{"id":123,"name":"Film","first_air_date":"2024-01-01"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	})
	result, err := client.RecognizeInfo(context.Background(), medianame.Info{Title: "Film", Year: 2024,
		Type: medianame.TypeUnknown, IDs: medianame.MediaIDs{TMDB: "123"}})
	if err != nil || result.Status != tmdb.StatusAmbiguous || result.MediaInfo != nil || len(result.Candidates) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestUnknownTypeSearchAndYearFallbackRetainEvidence(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie", "/3/search/tv":
			q := r.URL.Query()
			if r.URL.Path == "/3/search/tv" && q.Get("year") != "" {
				t.Error("TV must use first_air_date_year")
			}
			if q.Get("year") != "" || q.Get("first_air_date_year") != "" || r.URL.Path == "/3/search/tv" {
				respond(w, `{"total_pages":0,"total_results":0,"results":[]}`)
			} else {
				respond(w, `{"total_pages":1,"total_results":1,"results":[{"id":9,"title":"Film","release_date":"2023-01-01"}]}`)
			}
		case "/3/movie/9":
			respond(w, `{"id":9,"title":"Film","release_date":"2023-01-01","overview":"保留详情"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	})
	result, err := client.RecognizeInfo(context.Background(), medianame.Info{Title: "Film", Type: medianame.TypeUnknown, Year: 2024})
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil || result.MediaInfo.Year != 2023 || result.MetaInfo.Year != 2024 ||
		len(result.Candidates[0].ResolvedConflicts) != 1 || result.Candidates[0].ResolvedConflicts[0].Field != "year" || result.Candidates[0].MediaInfo.Overview != "保留详情" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestTruncatedSearchCannotChooseUniqueCandidate(t *testing.T) {
	for _, pagination := range []bool{false, true} {
		t.Run(fmt.Sprint(pagination), func(t *testing.T) {
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/3/search/movie" {
					pages := 1
					if pagination {
						pages = 2
					}
					respond(w, fmt.Sprintf(`{"total_pages":%d,"total_results":2,"results":[{"id":1,"title":"Film"},{"id":2,"title":"Film"}]}`, pages))
				} else {
					respond(w, `{"id":1,"title":"Film","release_date":"2024-01-01"}`)
				}
			}, func(c *tmdb.Config) { c.MaxCandidates = 1 })
			result, err := client.RecognizeInfo(context.Background(), medianame.Info{Title: "Film", Type: medianame.TypeMovie})
			if err != nil || result.Status != tmdb.StatusReview || !result.Truncated || result.MediaInfo != nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestOnly404IsNoCandidateAndRateLimitIsError(t *testing.T) {
	for _, status := range []int{404, 401, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(status)
				respond(w, `{"success":false,"status_code":34,"status_message":"test-token rejected"}`)
			})
			result, err := client.RecognizeInfo(context.Background(), medianame.Info{Type: medianame.TypeMovie, IDs: medianame.MediaIDs{TMDB: "123"}})
			if status == 404 {
				if err != nil || result.Status != tmdb.StatusNotIdentified {
					t.Fatal(result, err)
				}
				return
			}
			var apiError *tmdb.APIError
			if !errors.As(err, &apiError) || result.Status != tmdb.StatusRequestError || apiError.StatusCode != status ||
				apiError.RetryAfter != "2" || strings.Contains(err.Error(), "test-token") {
				t.Fatal(result, err)
			}
		})
	}
}

func TestInvalidResponseCannotMasqueradeAsNoResult(t *testing.T) {
	for _, body := range []string{"not json", "null", `{}`, `{"id":0,"title":"Film"}`, `{"id":123}`, `{"id":456,"title":"Film"}`} {
		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { respond(w, body) })
		if _, err := client.Details(context.Background(), medianame.TypeMovie, "123"); err == nil {
			t.Errorf("accepted malformed/mismatched response: %s", body)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestKeyAuthenticationRedactionAndContextCancellation(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.URL.Query().Get("api_key") != "test-key" {
			t.Error("wrong API key authentication")
		}
		respond(w, `{"id":1,"title":"Film"}`)
	}, func(c *tmdb.Config) { c.Token, c.APIKey = "", "test-key" })
	if _, err := client.Details(context.Background(), medianame.TypeMovie, "1"); err != nil {
		t.Fatal(err)
	}
	client, err := tmdb.NewClient(tmdb.Config{APIKey: "key+secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("bad request %s key+secret", r.URL)
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Details(context.Background(), medianame.TypeMovie, "1")
	if err == nil || strings.Contains(err.Error(), "key+secret") || strings.Contains(err.Error(), "key%2Bsecret") {
		t.Fatal("credential leaked", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := client.Recognize(ctx, "Film.2024")
	if !errors.Is(err, context.Canceled) || result.Status != tmdb.StatusRequestError {
		t.Fatal(result, err)
	}
}

func TestRedirectCannotForwardCredentials(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) })
	if _, err := client.Details(context.Background(), medianame.TypeMovie, "1"); err == nil || forwarded {
		t.Fatal("redirect followed", err)
	}
}

func TestEmptyInputMakesNoRequests(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	result, err := client.Recognize(context.Background(), "")
	if err != nil || result.Status != tmdb.StatusNotIdentified {
		t.Fatal(result, err)
	}
}

func TestPartialResultIsRetainedOnRequestError(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			respond(w, `{"total_pages":1,"total_results":2,"results":[{"id":1,"title":"Film"},{"id":2,"title":"Film"}]}`)
		case "/3/movie/1":
			respond(w, `{"id":1,"title":"Film","overview":"已取得详情"}`)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	result, err := client.RecognizeInfo(context.Background(), medianame.Info{Title: "Film", Type: medianame.TypeMovie})
	if err == nil || result.Status != tmdb.StatusRequestError || result.MediaInfo != nil ||
		len(result.Candidates) != 1 || result.Candidates[0].MediaInfo.Overview != "已取得详情" {
		t.Fatal(result, err)
	}
}

func TestClientConcurrentRequestsAndCustomHTTPConfiguration(t *testing.T) {
	custom := &http.Client{}
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, `{"id":1,"title":"Film","overview":"并发详情"}`)
	}, func(c *tmdb.Config) { c.HTTPClient = custom })
	if custom.CheckRedirect != nil {
		t.Fatal("modified caller's HTTP client")
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			media, err := client.Details(context.Background(), medianame.TypeMovie, "1")
			if err != nil || media.Overview != "并发详情" {
				t.Error(media, err)
			}
		}()
	}
	workers.Wait()
}

func TestConfigurationAndIDValidation(t *testing.T) {
	for _, config := range []tmdb.Config{
		{},
		{Token: "token", BaseURL: "https://name:password@api.example/3"},
		{Token: "token", BaseURL: "https://api.example/3?api_key=key"},
		{Token: "to\nken"},
		{Token: "token", MaxCandidates: -1},
	} {
		if _, err := tmdb.NewClient(config); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	for _, id := range []string{"", "0", "../../1", "123?api_key=other"} {
		if _, err := client.Details(context.Background(), medianame.TypeMovie, id); err == nil {
			t.Fatal("accepted invalid id", id)
		}
	}
	if _, err := client.Details(context.Background(), medianame.TypeUnknown, "1"); err == nil {
		t.Fatal("accepted unknown type")
	}
}

func TestIdentityMetadataRetainsRegionsLanguagesAndAllNames(t *testing.T) {
	for _, mediaType := range []medianame.MediaType{medianame.TypeMovie, medianame.TypeTV} {
		t.Run(string(mediaType), func(t *testing.T) {
			identity := `"title":"本地片名","original_title":"Original Title","release_date":"2024-01-01"`
			aliasField, translatedField := "titles", "title"
			if mediaType == medianame.TypeTV {
				identity = `"name":"本地片名","original_name":"Original Title","first_air_date":"2024-01-01","type":"Scripted"`
				aliasField, translatedField = "results", "name"
			}
			payload := fmt.Sprintf(`{"id":1,%s,
				"alternative_titles":{"%s":[
					{"iso_3166_1":"CN","title":"另一个名字","type":"Alternative Title"},
					{"iso_3166_1":"TW","title":"另一个名字","type":"Theatrical"},
					{"iso_3166_1":"US","title":"Original Title","type":"Original Title"}]},
				"translations":{"translations":[
					{"iso_3166_1":"CN","iso_639_1":"zh","name":"中文","english_name":"Chinese","data":{"%s":"另一个名字","overview":"中文简介","tagline":"中文标语","homepage":"https://example.com/cn","runtime":0}},
					{"iso_3166_1":"JP","iso_639_1":"ja","name":"日本語","english_name":"Japanese","data":{"%s":"日本語の名前","overview":"日本語の紹介","homepage":""}},
					{"iso_3166_1":"FR","iso_639_1":"fr","name":"Français","english_name":"French","data":{"%s":"","overview":"仅有简介的记录也保留","homepage":""}}]},
				"external_ids":{"imdb_id":"tt1234567","tvdb_id":12345678901234567890,"tvrage_id":321,"wikidata_id":"Q123",
					"facebook_id":"facebook-page","instagram_id":"instagram-name","twitter_id":"twitter-name","tiktok_id":"tiktok-name","youtube_id":"youtube-channel","future_source":"future-id"},
				"future_metadata":{"flag":true}}`, identity, aliasField, translatedField, translatedField, translatedField)
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { respond(w, payload) })
			media, err := client.Details(context.Background(), mediaType, "1")
			if err != nil {
				t.Fatal(err)
			}
			if media.OriginalTitle != "Original Title" || len(media.AlternativeTitles) != 3 ||
				media.AlternativeTitles[0].Country != "CN" || media.AlternativeTitles[1].Country != "TW" ||
				media.AlternativeTitles[1].Type != "Theatrical" || len(media.Translations) != 3 ||
				media.Translations[0].Language != "zh" || media.Translations[0].Name != "中文" ||
				media.Translations[0].Data.Overview != "中文简介" || media.Translations[0].Data.Tagline != "中文标语" ||
				media.Translations[0].Data.Runtime == nil || *media.Translations[0].Data.Runtime != 0 ||
				media.Translations[2].Data.Overview != "仅有简介的记录也保留" {
				t.Fatalf("lost identity metadata: %+v", media)
			}
			if mediaType == medianame.TypeTV && (media.Name != "本地片名" || media.OriginalName != "Original Title" ||
				media.Type != medianame.TypeTV || media.SeriesType != "Scripted" || media.Translations[1].Data.Name != "日本語の名前") {
				t.Fatal("TV names or series type lost", media)
			}
			if len(media.Aliases) != 2 || len(media.AllTitles) != 4 || media.AllTitles[1] != "Original Title" {
				t.Fatal("convenient name lists incomplete or not deduplicated", media.AllTitles, media.Aliases)
			}
			ids := media.ExternalIDs
			if ids == nil || ids.TVDBID != "12345678901234567890" || media.TVDBID != ids.TVDBID || ids.TVRageID != "321" ||
				ids.WikidataID != "Q123" || ids.TikTokID != "tiktok-name" || ids.YouTubeID != "youtube-channel" ||
				!strings.Contains(string(ids.Raw), `"future_source":"future-id"`) || string(media.Raw) != payload {
				t.Fatal("external IDs or raw fields lost", ids)
			}
			encoded, err := json.Marshal(media)
			if err != nil {
				t.Fatal(err)
			}
			var output struct {
				AlternativeTitles []tmdb.AlternativeTitle    `json:"alternative_titles"`
				Translations      []tmdb.Translation         `json:"translations"`
				ExternalIDs       map[string]json.RawMessage `json:"external_ids"`
			}
			if err := json.Unmarshal(encoded, &output); err != nil || len(output.AlternativeTitles) != 3 || len(output.Translations) != 3 ||
				string(output.ExternalIDs["tvdb_id"]) != `"12345678901234567890"` {
				t.Fatal("public JSON lacks full metadata", err)
			}
			// 各语言译名参与核对，仍不消除年份矛盾。
			info := medianame.Info{Title: "日本語の名前", Type: mediaType, Year: 2024}
			if conflicts := info.CheckCandidate(media.Candidate()); len(conflicts) != 0 {
				t.Fatal(conflicts)
			}
			info.Year = 2023
			if conflicts := info.CheckCandidate(media.Candidate()); len(conflicts) != 1 || conflicts[0].Field != "year" {
				t.Fatal(conflicts)
			}
		})
	}
}

func TestAdditionalMovieAndTVDetails(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/movie/") {
			respond(w, `{"id":1,"title":"Film","original_title":"Original Film","budget":35000000,"revenue":4200000000,"video":false,
				"credits":{"cast":[{"id":2,"name":"Actor","original_name":"Original Actor","gender":2,"popularity":4.5,"cast_id":0,"credit_id":"credit"}]}}`)
		} else {
			respond(w, `{"id":1,"name":"Show","original_name":"Original Show","type":"Miniseries","languages":["en","zh"],
				"created_by":[{"id":2,"name":"Creator","original_name":"Original Creator","gender":2,"credit_id":"creator-credit"}],
				"last_episode_to_air":{"id":10,"name":"Special","season_number":0,"episode_number":1,"episode_type":"standard","runtime":0,"overview":"上一集"},
				"next_episode_to_air":{"id":11,"name":"Next","season_number":1,"episode_number":1,"runtime":null,"overview":"下一集"}}`)
		}
	})
	movie, err := client.Details(context.Background(), medianame.TypeMovie, "1")
	if err != nil || movie.Budget != 35000000 || movie.Revenue != 4200000000 || movie.Video == nil || *movie.Video ||
		movie.Credits.Cast[0].CreditID != "credit" || movie.Credits.Cast[0].CastID == nil || *movie.Credits.Cast[0].CastID != 0 {
		t.Fatal(movie, err)
	}
	tv, err := client.Details(context.Background(), medianame.TypeTV, "1")
	if err != nil || tv.Type != medianame.TypeTV || tv.SeriesType != "Miniseries" || len(tv.Languages) != 2 ||
		tv.CreatedBy[0].CreditID != "creator-credit" || tv.LastEpisodeToAir.SeasonNumber != 0 ||
		tv.LastEpisodeToAir.Runtime == nil || *tv.LastEpisodeToAir.Runtime != 0 || tv.NextEpisodeToAir.Runtime != nil {
		t.Fatal(tv, err)
	}
}
