package tmdb_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/tmdb"
)

func TestMixedTitleRecognizesOriginalNameWithoutCorrection(t *testing.T) {
	// This previously needed a title correction because the two names were joined.
	// Separating the original name now gives the unchanged matcher an exact hit.
	info := medianame.Parse("[变形金刚3].Transformers.Dark.of.the.Moon.2011.1080p.mkv")
	media := &tmdb.MediaInfo{Title: "变形金刚3：月黑之时", OriginalTitle: "Transformers: Dark of the Moon", Year: 2011, Type: medianame.TypeMovie, TMDBID: "38356"}
	result := tmdb.Result{MetaInfo: info, Candidates: []tmdb.Candidate{{MediaInfo: media}}}
	result.Reconcile()
	if result.Status != tmdb.StatusCompatible || result.MediaInfo != media || !reflect.DeepEqual(result.MetaInfo, info) ||
		len(result.Candidates[0].Conflicts) != 0 || len(result.Candidates[0].ResolvedConflicts) != 0 {
		t.Fatalf("original name not matched directly: %+v", result)
	}
}

func TestMixedTitleAndCompletedRangeRecognition(t *testing.T) {
	for _, tt := range []struct {
		name, results string
		pages         int
		status        tmdb.Status
	}{
		{"exact English title", `[{"id":7,"name":"Some Anime","first_air_date":"2024-01-01"}]`, 1, tmdb.StatusCompatible},
		{"different names remain ambiguous", `[{"id":7,"name":"Other A","first_air_date":"2024-01-01"},{"id":8,"name":"Other B","first_air_date":"2024-01-01"}]`, 1, tmdb.StatusAmbiguous},
		{"partial search still needs review", `[{"id":7,"name":"Some Anime","first_air_date":"2024-01-01"}]`, 2, tmdb.StatusReview},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var queries []string
			var mu sync.Mutex
			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/3/search/tv":
					query := r.URL.Query()
					mu.Lock()
					queries = append(queries, query.Get("query"))
					mu.Unlock()
					if query.Get("first_air_date_year") != "2024" {
						t.Errorf("year lost: %v", query)
					}
					respond(w, fmt.Sprintf(`{"total_pages":%d,"results":%s}`, tt.pages, tt.results))
				case "/3/tv/7":
					name := "Some Anime"
					if tt.status == tmdb.StatusAmbiguous {
						name = "Other A"
					}
					respond(w, fmt.Sprintf(`{"id":7,"name":%q,"first_air_date":"2024-01-01"}`, name))
				case "/3/tv/8":
					respond(w, `{"id":8,"name":"Other B","first_air_date":"2024-01-01"}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			const input = "[01-26Fin] 某动画.Some.Anime.2024.1080p.mkv"
			result, err := client.Recognize(context.Background(), input)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || result.Status != tt.status || !reflect.DeepEqual(queries, []string{"某动画", "Some Anime"}) {
				t.Fatalf("result=%+v queries=%v err=%v", result, queries, err)
			}
			if !reflect.DeepEqual(result.MetaInfo, medianame.Parse(input)) || result.MetaInfo.Episode == nil || *result.MetaInfo.Episode != 1 ||
				result.MetaInfo.EpisodeEnd == nil || *result.MetaInfo.EpisodeEnd != 26 {
				t.Fatal("episode range or input evidence lost")
			}
			if (result.MediaInfo != nil) != (tt.status == tmdb.StatusCompatible) {
				t.Fatal("ambiguous or incomplete candidates adopted")
			}
		})
	}
}
