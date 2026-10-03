package tmdb_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/tmdb"
)

const emptyFind = `{"movie_results":[],"tv_results":[],"person_results":[],"tv_season_results":[],"tv_episode_results":[]}`
const foundMovie = `{"movie_results":[{"id":9,"title":"Official Film"}],"tv_results":[]}`
const foundTV = `{"movie_results":[],"tv_results":[{"id":123,"name":"Official Show"}]}`
const externalMovieDetails = `{"id":9,"title":"Official Film","original_title":"Official Film","release_date":"2024-01-01",
	"overview":"Full movie details","external_ids":{"imdb_id":"tt0000123"}}`
const externalTVDetails = `{"id":123,"name":"Official Show","original_name":"Official Show","first_air_date":"2024-01-01",
	"number_of_episodes":12,"external_ids":{"imdb_id":"tt0000123","tvdb_id":456}}`

func TestFindByIDReturnsOnlyMovieAndSeriesSummaries(t *testing.T) {
	var requests atomic.Int32
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/3/find/tt0000123" ||
			r.URL.Query().Get("external_source") != "imdb_id" || r.URL.Query().Get("language") != "zh-CN" ||
			r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected find request: %s", r.URL)
		}
		respond(w, `{"movie_results":[{"id":9,"title":"Film"}],"tv_results":[{"id":123,"name":"Show"}],
			"person_results":[{"id":8,"name":"Actor"}],"tv_season_results":[{"id":10,"show_id":99}],
			"tv_episode_results":[{"id":11,"show_id":99,"season_number":1,"episode_number":2}]}`)
	})
	for i := 0; i < 2; i++ {
		result, err := client.FindByID(context.Background(), tmdb.SourceIMDb, "TT0000123")
		if err != nil || len(result.Results) != 2 || result.Results[0].Type != medianame.TypeMovie ||
			result.Results[0].TMDBID != "9" || result.Results[0].Title != "Film" || result.Results[0].Raw != nil ||
			result.Results[1].Type != medianame.TypeTV || result.Results[1].TMDBID != "123" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		result.Results[0].Title = "Changed"
	}
	if requests.Load() != 1 || client.CacheStats().Entries != 1 {
		t.Fatal("find did not reuse an independent cached response")
	}
}

func TestFindByIDValidatesInputs(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid ID caused a request") })
	for _, tc := range []struct {
		source tmdb.ExternalSource
		id     string
	}{
		{tmdb.SourceIMDb, ""}, {tmdb.SourceIMDb, "123"}, {tmdb.SourceIMDb, "tt0"},
		{tmdb.SourceIMDb, "tt000"}, {tmdb.SourceIMDb, "tt123abc"}, {tmdb.SourceIMDb, "tt123/4"},
		{tmdb.SourceIMDb, "tt123456789012345678901"}, {tmdb.SourceTVDB, "0"}, {tmdb.SourceTVDB, "tt123"},
		{tmdb.SourceTVDB, "../123"}, {tmdb.SourceTVDB, "123?api_key=other"}, {"other", "123"},
	} {
		if _, err := client.FindByID(context.Background(), tc.source, tc.id); err == nil {
			t.Errorf("invalid external ID accepted: %+v", tc)
		}
	}
}

func TestFindByIDRejectsMalformedResponsesWithoutCaching(t *testing.T) {
	for _, body := range []string{
		`invalid`, `null`, `{}`, `{"movie_results":[],"tv_results":null}`,
		`{"movie_results":[],"tv_results":{}}`, `{"movie_results":[],"tv_results":[{"id":0,"name":"Show"}]}`,
		`{"movie_results":[],"tv_results":[{"id":123}]}`, foundMovie,
	} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					respond(w, body)
				} else {
					respond(w, foundTV)
				}
			})
			if _, err := client.FindByID(context.Background(), tmdb.SourceTVDB, "456"); err == nil || client.CacheStats().Entries != 0 {
				t.Fatal("malformed find response accepted or cached")
			}
			if _, err := client.FindByID(context.Background(), tmdb.SourceTVDB, "456"); err != nil || requests.Load() != 2 {
				t.Fatal("malformed response persisted", err)
			}
		})
	}
}

func TestRecognizeExternalIDsUsesDetailsAndPreservesInput(t *testing.T) {
	for _, tc := range []struct {
		name, input, id, source, found, details, endpoint string
		path                                              bool
		mediaType                                         medianame.MediaType
	}{
		{"IMDb movie", "Wrong.Title.2023.[imdb=tt0000123].mkv", "tt0000123", "imdb_id", foundMovie, externalMovieDetails, "/3/movie/9", false, medianame.TypeMovie},
		{"IMDb series", "Official.Show.S01E02.[imdb=tt0000123].mkv", "tt0000123", "imdb_id", foundTV, externalTVDetails, "/3/tv/123", false, medianame.TypeTV},
		{"TVDB path", "/tv/Official Show (2024) [tvdb=456]/Season 0/01.mkv", "456", "tvdb_id", foundTV, externalTVDetails, "/3/tv/123", true, medianame.TypeTV},
		{"ID without title", "[imdb=tt0000123]", "tt0000123", "imdb_id", foundMovie, externalMovieDetails, "/3/movie/9", false, medianame.TypeMovie},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/3/find/" + tc.id:
					if r.URL.Query().Get("external_source") != tc.source {
						t.Errorf("wrong external source: %s", r.URL)
					}
					respond(w, tc.found)
				case tc.endpoint:
					respond(w, tc.details)
				default:
					t.Errorf("external ID fell back to search: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			parse, recognize := medianame.Parse, client.Recognize
			if tc.path {
				parse, recognize = medianame.ParsePath, client.RecognizePath
			}
			result, err := recognize(context.Background(), tc.input)
			if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil ||
				result.MediaInfo.Type != tc.mediaType || len(result.MediaInfo.Raw) == 0 || len(requests) != 2 ||
				!reflect.DeepEqual(result.MetaInfo, parse(tc.input)) {
				t.Fatalf("result=%+v requests=%v err=%v", result, requests, err)
			}
			if tc.path && (*result.MetaInfo.Season != 0 || *result.MetaInfo.Episode != 1 || result.MediaInfo.NumberOfEpisodes != 12) {
				t.Fatal("path episode or full series details lost")
			}
		})
	}
}

func TestTMDBIDTakesPrecedenceOverExternalIDs(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/movie/9" {
			t.Errorf("TMDB ID did not take precedence: %s", r.URL.Path)
		}
		respond(w, externalMovieDetails)
	})
	for _, id := range []string{"tt0000123", "tt999"} {
		result, err := client.Recognize(context.Background(), "Film.[tmdb=9].[imdb="+id+"]")
		want := tmdb.StatusCompatible
		if id == "tt999" {
			want = tmdb.StatusReview
		}
		if err != nil || result.Status != want {
			t.Fatalf("external ID cross-check lost: %+v err=%v", result, err)
		}
	}
}

func TestExternalIDsMergeCandidatesAndCheckAllIDs(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			var finds, details atomic.Int32
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/3/find/tt0000123", "/3/find/456":
					finds.Add(1)
					if conflict && r.URL.Path == "/3/find/456" {
						respond(w, `{"movie_results":[],"tv_results":[{"id":124,"name":"Other Show"}]}`)
					} else {
						respond(w, foundTV)
					}
				case "/3/tv/123":
					details.Add(1)
					body := externalTVDetails
					if conflict {
						body = strings.Replace(body, `"tvdb_id":456`, `"tvdb_id":999`, 1)
					}
					respond(w, body)
				case "/3/tv/124":
					details.Add(1)
					respond(w, `{"id":124,"name":"Other Show","external_ids":{"imdb_id":"tt999","tvdb_id":456}}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			result, err := client.Recognize(context.Background(), "Official.Show.S01E01.[imdb=tt0000123].[tvdb=456]")
			wantStatus, wantCount := tmdb.StatusCompatible, 1
			if conflict {
				wantStatus, wantCount = tmdb.StatusReview, 2
			}
			if err != nil || result.Status != wantStatus || finds.Load() != 2 || int(details.Load()) != wantCount || len(result.Candidates) != wantCount {
				t.Fatalf("result=%+v finds=%d details=%d err=%v", result, finds.Load(), details.Load(), err)
			}
			if conflict && (result.MediaInfo != nil || result.Candidates[0].Conflicts[0].Field != "tvdb_id" || result.Candidates[1].Conflicts[0].Field != "imdb_id") {
				t.Fatal("conflicting external IDs were not retained")
			}
		})
	}
}

func TestExternalIDNoMappingFallsBackToNameWithIdentityChecks(t *testing.T) {
	for _, tc := range []struct {
		name, findBody, details string
		status                  tmdb.Status
		conflict                string
	}{
		{"matching ID", emptyFind, externalTVDetails, tmdb.StatusCompatible, ""},
		{"different ID", emptyFind, strings.Replace(externalTVDetails, `"tvdb_id":456`, `"tvdb_id":999`, 1), tmdb.StatusReview, "tvdb_id"},
		{"missing ID", emptyFind, `{"id":123,"name":"Official Show","first_air_date":"2024-01-01"}`, tmdb.StatusReview, "tvdb_id"},
		{"episode ID", `{"movie_results":[],"tv_results":[],"tv_episode_results":[{"id":999,"show_id":888,"season_number":1,"episode_number":2}]}`,
			strings.Replace(externalTVDetails, `"tvdb_id":456`, `"tvdb_id":789`, 1), tmdb.StatusReview, "tvdb_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/3/find/456":
					respond(w, tc.findBody)
				case "/3/search/tv":
					query := r.URL.Query()
					if query.Get("query") != "Official Show" {
						t.Errorf("fallback lost file title: %s", r.URL)
					}
					if query.Get("first_air_date_year") == "2023" {
						respond(w, `{"results":[],"total_pages":0,"total_results":0}`)
					} else {
						respond(w, `{"results":[{"id":123,"name":"Official Show"}],"total_pages":1,"total_results":1}`)
					}
				case "/3/tv/123":
					respond(w, tc.details)
				default:
					t.Errorf("unexpected request or episode promoted to series: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			result, err := client.Recognize(context.Background(), "Official.Show.2023.S01E02.[tvdb=456].mkv")
			if err != nil || result.Status != tc.status || len(result.Candidates) != 1 || len(requests) != 4 ||
				result.MetaInfo.IDs.TVDB != "456" || result.MetaInfo.Year != 2023 {
				t.Fatalf("result=%+v requests=%v err=%v", result, requests, err)
			}
			if tc.conflict != "" {
				found := false
				for _, conflict := range result.Candidates[0].Conflicts {
					found = found || conflict.Field == tc.conflict
				}
				if result.MediaInfo != nil || !found {
					t.Fatal("fallback hid explicit ID conflict")
				}
			}
		})
	}
}

func TestExternalIDNoMappingWithoutTitleRemainsNotIdentified(t *testing.T) {
	for _, body := range []string{emptyFind,
		`{"movie_results":[],"tv_results":[],"tv_season_results":[{"id":999,"show_id":888,"season_number":1}]}`,
		`{"movie_results":[],"tv_results":[],"tv_episode_results":[{"id":999,"show_id":888,"season_number":1,"episode_number":2}]}`,
	} {
		var requests atomic.Int32
		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Path != "/3/find/456" {
				t.Errorf("unsupported ID promoted to series: %s", r.URL.Path)
			}
			respond(w, body)
		})
		for i := 0; i < 2; i++ {
			result, err := client.Recognize(context.Background(), "[tvdb=456]")
			if err != nil || result.Status != tmdb.StatusNotIdentified || result.MediaInfo != nil || len(result.Candidates) != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		}
		if requests.Load() != 1 {
			t.Fatal("empty find response was not cached")
		}
	}
}

func TestExternalIDRequestFailuresDoNotFallBack(t *testing.T) {
	for _, status := range []int{401, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/3/find/tt0000123" {
					t.Errorf("request error fell back to search: %s", r.URL.Path)
				}
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(status)
				respond(w, `{"success":false,"status_code":34,"status_message":"test-token"}`)
			})
			for i := 0; i < 2; i++ {
				result, err := client.Recognize(context.Background(), "Film.[imdb=tt0000123]")
				var apiError *tmdb.APIError
				if !errors.As(err, &apiError) || result.Status != tmdb.StatusRequestError || result.MediaInfo != nil ||
					apiError.StatusCode != status || apiError.RetryAfter != "5" || strings.Contains(err.Error(), "test-token") {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if requests.Load() != 2 || client.CacheStats().Entries != 0 {
				t.Fatal("request error was cached or triggered extra requests")
			}
		})
	}
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("canceled context sent a request") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := client.Recognize(ctx, "Film.[imdb=tt0000123]")
	if !errors.Is(err, context.Canceled) || result.Status != tmdb.StatusRequestError {
		t.Fatalf("cancellation lost: %+v %v", result, err)
	}
}

func TestExternalIDLookupPreservesCandidatesOnLaterFailure(t *testing.T) {
	for _, failedEndpoint := range []string{"/3/find/456", "/3/tv/124"} {
		t.Run(failedEndpoint, func(t *testing.T) {
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == failedEndpoint {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				switch r.URL.Path {
				case "/3/find/tt0000123":
					respond(w, foundTV)
				case "/3/tv/123":
					respond(w, externalTVDetails)
				case "/3/find/456":
					respond(w, `{"movie_results":[],"tv_results":[{"id":124,"name":"Other"}]}`)
				default:
					t.Errorf("failure fell back to search: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			result, err := client.Recognize(context.Background(), "Show.S01E01.[imdb=tt0000123].[tvdb=456]")
			if err == nil || result.Status != tmdb.StatusRequestError || result.MediaInfo != nil || len(result.Candidates) != 1 || result.Candidates[0].MediaInfo.TMDBID != "123" {
				t.Fatalf("partial candidates lost or adopted: %+v err=%v", result, err)
			}
			result.Reconcile()
			if result.Status != tmdb.StatusRequestError || result.MediaInfo != nil {
				t.Fatal("partial request failure became compatible")
			}
		})
	}
}

func TestExternalIDAmbiguityLimitsAndTypeConflicts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mediaType medianame.MediaType
		limit     int
		status    tmdb.Status
		count     int
		truncated bool
	}{
		{"unknown type", medianame.TypeUnknown, 20, tmdb.StatusAmbiguous, 2, false},
		{"limited candidates", medianame.TypeUnknown, 1, tmdb.StatusReview, 1, true},
		{"known movie", medianame.TypeMovie, 20, tmdb.StatusCompatible, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/3/find/tt0000123":
					respond(w, `{"movie_results":[{"id":123,"title":"Same"},{"id":123,"title":"Same"}],"tv_results":[{"id":123,"name":"Same"}]}`)
				case "/3/movie/123":
					respond(w, `{"id":123,"title":"Same","release_date":"2024-01-01","imdb_id":"tt0000123"}`)
				case "/3/tv/123":
					respond(w, `{"id":123,"name":"Same","first_air_date":"2024-01-01","external_ids":{"imdb_id":"tt0000123"}}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}, func(c *tmdb.Config) { c.MaxCandidates = tc.limit })
			result, err := client.RecognizeInfo(context.Background(), medianame.Info{Title: "Same", Year: 2024, Type: tc.mediaType, IDs: medianame.MediaIDs{IMDb: "tt0000123"}})
			if err != nil || result.Status != tc.status || len(result.Candidates) != tc.count || result.Truncated != tc.truncated || result.SelectionBasis != "" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if tc.mediaType == medianame.TypeMovie && (result.MediaInfo.Type != medianame.TypeMovie || result.Candidates[1].Conflicts[0].Field != "type") {
				t.Fatal("type conflict was hidden")
			}
		})
	}
}

func TestExternalIDCacheReusesFindAndDetailsAcrossEpisodes(t *testing.T) {
	var finds, details atomic.Int32
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/find/456":
			finds.Add(1)
			respond(w, foundTV)
		case "/3/tv/123":
			details.Add(1)
			respond(w, externalTVDetails)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	results := make([]*tmdb.Result, 12)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := client.Recognize(context.Background(), fmt.Sprintf("Show.S01E%02d.[tvdb=456].mkv", i+1))
			if err != nil || result.Status != tmdb.StatusCompatible || result.MetaInfo.Episode == nil || *result.MetaInfo.Episode != i+1 {
				t.Errorf("episode=%d result=%+v err=%v", i+1, result, err)
				return
			}
			results[i] = result
		}(i)
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	if finds.Load() != 1 || details.Load() != 1 || client.CacheStats().Entries != 2 {
		t.Fatalf("finds=%d details=%d stats=%+v", finds.Load(), details.Load(), client.CacheStats())
	}
	results[0].MediaInfo.ExternalIDs.TVDBID = "Changed"
	*results[0].MetaInfo.Episode = 999
	if results[1].MediaInfo.ExternalIDs.TVDBID != "456" || *results[1].MetaInfo.Episode != 2 {
		t.Fatal("external ID cache shared mutable episode results")
	}
}
