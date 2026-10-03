package tmdb_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/tmdb"
)

const cachedTVDetails = `{"id":123,"name":"Example Show","original_name":"Example Show","first_air_date":"2024-01-01",
	"external_ids":{"imdb_id":"tt123","tvdb_id":456},"alternative_titles":{"results":[{"title":"示例剧"}]},
	"translations":{"translations":[{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Example Show","overview":"Translation"}}]},
	"seasons":[{"season_number":1,"episode_count":30}],"credits":{"cast":[{"id":1,"name":"Actor"}],"crew":[]}}`

const cachedTVSearch = `{"results":[{"id":123,"name":"Example Show","original_name":"Example Show","first_air_date":"2024-01-01"}],"total_pages":1,"total_results":1}`

func TestCacheReusesRequestsAcrossEpisodes(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		for _, explicitID := range []bool{false, true} {
			t.Run(fmt.Sprintf("concurrent=%v/id=%v", concurrent, explicitID), func(t *testing.T) {
				var searches, details atomic.Int32
				client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/3/search/tv":
						searches.Add(1)
						respond(w, cachedTVSearch)
					case "/3/tv/123":
						details.Add(1)
						respond(w, cachedTVDetails)
					default:
						t.Errorf("unexpected endpoint: %s", r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				})
				results := make([]*tmdb.Result, 30)
				recognize := func(i int) {
					name := fmt.Sprintf("Example.Show.2024.S01E%02d.1080p.mkv", i+1)
					if explicitID {
						name += ".[tmdb=123]"
					}
					result, err := client.Recognize(context.Background(), name)
					if err != nil || result.Status != tmdb.StatusCompatible || result.MetaInfo.Episode == nil || *result.MetaInfo.Episode != i+1 {
						t.Errorf("episode %d: result=%+v err=%v", i+1, result, err)
						return
					}
					results[i] = result
				}
				var wg sync.WaitGroup
				for i := range results {
					if concurrent {
						wg.Add(1)
						go func(i int) { defer wg.Done(); recognize(i) }(i)
					} else {
						recognize(i)
					}
				}
				wg.Wait()
				wantSearches := int32(1)
				if explicitID {
					wantSearches = 0
				}
				if searches.Load() != wantSearches || details.Load() != 1 {
					t.Fatalf("searches=%d details=%d", searches.Load(), details.Load())
				}
				if !t.Failed() {
					results[0].MediaInfo.Credits.Cast[0].Name = "Changed"
					*results[0].MetaInfo.Episode = 999
					if results[1].MediaInfo.Credits.Cast[0].Name != "Actor" || *results[1].MetaInfo.Episode != 2 {
						t.Fatal("episode results share mutable data")
					}
				}
			})
		}
	}
}

func TestCacheReturnsIndependentDetailsAndSearchResults(t *testing.T) {
	var requests atomic.Int32
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.Contains(r.URL.Path, "/search/") {
			respond(w, cachedTVSearch)
		} else {
			respond(w, cachedTVDetails)
		}
	})
	first, err := client.Details(context.Background(), medianame.TypeTV, "00123")
	if err != nil {
		t.Fatal(err)
	}
	first.Title, first.Aliases[0], first.Credits.Cast[0].Name = "Changed", "Changed", "Changed"
	first.Translations[0].Data.Overview, first.ExternalIDs.IMDbID = "Changed", "Changed"
	first.Seasons[0].EpisodeCount, first.Raw[0], first.ExternalIDs.Raw[0] = 999, '!', '!'
	second, err := client.Details(context.Background(), medianame.TypeTV, "123")
	if err != nil || second.Title != "Example Show" || second.Aliases[0] != "示例剧" || second.Credits.Cast[0].Name != "Actor" ||
		second.Translations[0].Data.Overview != "Translation" || second.ExternalIDs.IMDbID != "tt123" ||
		second.Seasons[0].EpisodeCount != 30 || second.Raw[0] != '{' || second.ExternalIDs.Raw[0] != '{' {
		t.Fatalf("cache was modified by caller: %+v err=%v", second, err)
	}
	query := medianame.SearchQuery{Title: "Example Show", Year: 2024, Type: medianame.TypeTV}
	search, err := client.Search(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	search.Results[0].Title, search.TotalResults = "Changed", 99
	search, err = client.Search(context.Background(), query)
	if err != nil || search.Results[0].Title != "Example Show" || search.TotalResults != 1 || requests.Load() != 2 {
		t.Fatalf("search=%+v requests=%d err=%v", search, requests.Load(), err)
	}
	if stats := client.CacheStats(); stats.Entries != 2 || stats.Bytes != int64(len(cachedTVDetails)+len(cachedTVSearch)) {
		t.Fatalf("wrong JSON byte accounting: %+v", stats)
	}
}

func TestCacheSeparatesRequestParametersAndClients(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.Contains(r.URL.Path, "/search/") {
			respond(w, `{"results":[],"total_pages":0,"total_results":0}`)
		} else if strings.Contains(r.URL.Path, "/movie/") {
			respond(w, `{"id":123,"title":"Movie"}`)
		} else {
			respond(w, cachedTVDetails)
		}
	}))
	defer server.Close()
	newClient := func(language string, adult bool) *tmdb.Client {
		client, err := tmdb.NewClient(tmdb.Config{Token: "test", BaseURL: server.URL + "/3", Language: language, IncludeAdult: adult})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	client := newClient("zh-CN", false)
	queries := []medianame.SearchQuery{
		{Title: "Show", Year: 2024, Type: medianame.TypeTV},
		{Title: "Other", Year: 2024, Type: medianame.TypeTV},
		{Title: "Show", Year: 2023, Type: medianame.TypeTV},
		{Title: "Show", Type: medianame.TypeTV},
		{Title: "Show", Year: 2024, Type: medianame.TypeMovie},
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, query := range queries {
			if _, err := client.Search(context.Background(), query); err != nil {
				t.Fatal(err)
			}
		}
		for _, mediaType := range []medianame.MediaType{medianame.TypeTV, medianame.TypeMovie} {
			if _, err := client.Details(context.Background(), mediaType, "123"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if requests.Load() != 7 {
		t.Fatalf("different searches/types collided or empty results not cached: %d", requests.Load())
	}
	for _, other := range []*tmdb.Client{newClient("en-US", false), newClient("zh-CN", true), newClient("zh-CN", false)} {
		if _, err := other.Search(context.Background(), queries[0]); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 10 {
		t.Fatalf("different clients shared responses: %d", requests.Load())
	}
}

func TestCacheRejectsErrorAndInvalidDetailsResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"not-found", 404, `{}`}, {"unauthorized", 401, `{}`}, {"rate-limit", 429, `{}`}, {"server-error", 500, `{}`},
		{"invalid-json", 200, `invalid`}, {"null", 200, `null`}, {"missing-fields", 200, `{}`},
		{"invalid-id", 200, `{"id":0,"name":"Show"}`}, {"wrong-id", 200, `{"id":456,"name":"Show"}`},
		{"api-error", 200, `{"success":false,"status_code":34}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					w.WriteHeader(tc.status)
					respond(w, tc.body)
				} else {
					respond(w, cachedTVDetails)
				}
			})
			if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err == nil {
				t.Fatal("invalid response succeeded")
			}
			if stats := client.CacheStats(); stats != (tmdb.CacheStats{}) {
				t.Fatalf("invalid response cached: %+v", stats)
			}
			if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err != nil || requests.Load() != 2 {
				t.Fatalf("fresh request failed: requests=%d err=%v", requests.Load(), err)
			}
		})
	}
}

func TestCacheRejectsMalformedSearchResults(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":null}`, `{"results":{}}`, `{"results":[{"id":0,"name":"Show"}]}`} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					respond(w, body)
				} else {
					respond(w, cachedTVSearch)
				}
			})
			query := medianame.SearchQuery{Title: "Show", Type: medianame.TypeTV}
			if _, err := client.Search(context.Background(), query); err == nil || client.CacheStats().Entries != 0 {
				t.Fatal("malformed search response accepted/cached")
			}
			if _, err := client.Search(context.Background(), query); err != nil || requests.Load() != 2 {
				t.Fatal("malformed search response persisted", err)
			}
		})
	}
}

func TestCacheClearDisableAndCanceledHit(t *testing.T) {
	var requests atomic.Int32
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		respond(w, cachedTVDetails)
	})
	if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Details(ctx, medianame.TypeTV, "123"); !errors.Is(err, context.Canceled) || requests.Load() != 1 {
		t.Fatal("cache hit ignored cancellation", err)
	}
	client.ClearCache()
	if stats := client.CacheStats(); stats != (tmdb.CacheStats{}) {
		t.Fatalf("clear retained cache data: %+v", stats)
	}
	if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err != nil || requests.Load() != 2 {
		t.Fatal("clear did not force fresh request", err)
	}
	client = clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		respond(w, cachedTVDetails)
	}, func(c *tmdb.Config) { c.Cache.Disabled = true })
	for i := 0; i < 2; i++ {
		if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err != nil {
			t.Fatal(err)
		}
	}
	client.ClearCache()
	if requests.Load() != 4 || client.CacheStats() != (tmdb.CacheStats{}) {
		t.Fatal("disabled cache retained a response")
	}
}

func TestCachePreservesHTTPTimeoutAndRetriesNetworkErrors(t *testing.T) {
	for _, responseBody := range []bool{false, true} {
		for _, cacheDisabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("body=%v/cacheDisabled=%v", responseBody, cacheDisabled), func(t *testing.T) {
				var requests atomic.Int32
				client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
					if requests.Add(1) == 1 {
						if responseBody {
							_, _ = w.Write([]byte(`{"id":`))
							w.(http.Flusher).Flush()
						}
						<-r.Context().Done()
						return
					}
					respond(w, cachedTVDetails)
				}, func(c *tmdb.Config) {
					c.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}
					c.Cache.Disabled = cacheDisabled
				})
				if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("HTTP timeout lost: %v", err)
				}
				if stats := client.CacheStats(); stats != (tmdb.CacheStats{}) {
					t.Fatalf("timeout response cached: %+v", stats)
				}
				if _, err := client.Details(context.Background(), medianame.TypeTV, "123"); err != nil || requests.Load() != 2 {
					t.Fatal("could not retry after network error", err)
				}
			})
		}
	}
}

func TestCacheConfigurationValidation(t *testing.T) {
	for _, config := range []tmdb.CacheConfig{{MaxBytes: -1}, {MaxEntries: -1}, {TTL: -time.Second}} {
		if _, err := tmdb.NewClient(tmdb.Config{Token: "test", Cache: config}); err == nil {
			t.Fatalf("accepted invalid cache config: %+v", config)
		}
	}
}
