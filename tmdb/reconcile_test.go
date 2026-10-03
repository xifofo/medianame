package tmdb_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/tmdb"
)

func TestUltimateGiftUsesTMDBYearForRecognitionAndNaming(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			respond(w, `{"total_pages":1,"total_results":1,"results":[{"id":14624,"title":"超级礼物","release_date":"2007-03-09"}]}`)
		case "/3/movie/14624":
			respond(w, `{"id":14624,"title":"超级礼物","original_title":"The Ultimate Gift","release_date":"2007-03-09"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	name := "超级礼物 (2007) - [超级礼物].The.Ultimate.Gift.2006.USA.BluRay.1080p.x264.DDP.5.1-CMCT.mkv"
	result, err := client.Recognize(context.Background(), name)
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.MediaInfo.Year != 2007 || result.MediaInfo.Title != "超级礼物" || result.MetaInfo.Original != name ||
		!reflect.DeepEqual(result.MetaInfo.Warnings, []string{"conflicting_years"}) || len(result.Candidates[0].Conflicts) != 0 ||
		len(result.Candidates[0].ResolvedConflicts) != 1 || result.Candidates[0].ResolvedConflicts[0].Actual != "conflicting_years" {
		t.Fatalf("identity or difference lost: %+v", result)
	}
	preview, err := result.MediaInfo.Rename(result.MetaInfo, "")
	if err != nil || !strings.Contains(preview.Path, "超级礼物 (2007) {tmdb-14624}/The Ultimate Gift.2007.") || strings.Contains(preview.Path, "2006") {
		t.Fatalf("rename=%+v err=%v", preview, err)
	}
}

func TestReconcileAdoptsTMDBTitleAndOriginalTitle(t *testing.T) {
	for _, tt := range []struct {
		name  string
		media tmdb.MediaInfo
		field string
	}{
		{"小岛惊魂 (1973) - [小岛惊魂].The.Others.2001.1080p.mkv", tmdb.MediaInfo{Title: "小岛惊魂", OriginalTitle: "Voices", Year: 1973, Type: medianame.TypeMovie, TMDBID: "160910"}, "release_title"},
		{"预兆 (2023) - Harbin.2024.1080p.mkv", tmdb.MediaInfo{Title: "预兆", OriginalTitle: "The Harbinger", Year: 2023, Type: medianame.TypeMovie, TMDBID: "974191"}, "release_title"},
		{"预兆 （2023） — Harbin.2024.1080p.mkv", tmdb.MediaInfo{Title: "预兆", OriginalTitle: "The Harbinger", Year: 2023, Type: medianame.TypeMovie, TMDBID: "974191"}, "release_title"},
		{"甜蜜的小狐狸 (1983) - Sweet.Young.Foxes.1983.mkv", tmdb.MediaInfo{Title: "狐狸小姐", OriginalTitle: "Nathalie", Aliases: []string{"甜蜜的小狐狸"}, Year: 1957, Type: medianame.TypeMovie, TMDBID: "193282"}, "release_title"},
		{"Other.Film.2023.mkv", tmdb.MediaInfo{Title: "官方片名", OriginalTitle: "Official Title", Year: 2024, Type: medianame.TypeMovie, TMDBID: "9"}, "title"},
		{"[变形金刚3].Transformers.Dark.of.the.Moon.2011.1080p.mkv", tmdb.MediaInfo{Title: "变形金刚3：月黑之时", OriginalTitle: "Transformers: Dark of the Moon", Year: 2011, Type: medianame.TypeMovie, TMDBID: "38356"}, "title"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := medianame.Parse(tt.name)
			result := tmdb.Result{MetaInfo: info, Candidates: []tmdb.Candidate{{MediaInfo: &tt.media}}}
			result.Reconcile()
			if result.Status != tmdb.StatusCompatible || result.MediaInfo != &tt.media || len(result.Candidates[0].Conflicts) != 0 ||
				result.MediaInfo.Title != tt.media.Title || result.MediaInfo.OriginalTitle != tt.media.OriginalTitle || !reflect.DeepEqual(result.MetaInfo, info) {
				t.Fatalf("TMDB names not adopted: %+v", result)
			}
			found := false
			for _, conflict := range result.Candidates[0].ResolvedConflicts {
				found = found || conflict.Field == tt.field
			}
			if !found {
				t.Fatalf("missing %s: %+v", tt.field, result.Candidates[0])
			}
			preview, err := result.MediaInfo.Rename(result.MetaInfo, "")
			if err != nil || !strings.HasPrefix(preview.Path, tt.media.Title+" (") ||
				!strings.Contains(preview.Name, strings.ReplaceAll(tt.media.OriginalTitle, ":", "：")) {
				t.Fatalf("rename does not use TMDB names: %+v %v", preview, err)
			}
		})
	}
}

func TestReconcileKeepsTypeIDAndMissingMetadataConflicts(t *testing.T) {
	for _, tt := range []struct {
		name  string
		media tmdb.MediaInfo
		field string
	}{
		{"Film.2023.{[type=tv;tmdbid=9]}.mkv", tmdb.MediaInfo{Title: "Film", Year: 2024, Type: medianame.TypeMovie, TMDBID: "9"}, "type"},
		{"Film.2023.{[type=movie;tmdbid=8]}.mkv", tmdb.MediaInfo{Title: "Film", Year: 2024, Type: medianame.TypeMovie, TMDBID: "9"}, "tmdb_id"},
		{"Film.2023.mkv", tmdb.MediaInfo{Title: "Film", Type: medianame.TypeMovie, TMDBID: "9"}, "year"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := tmdb.Result{MetaInfo: medianame.Parse(tt.name), Candidates: []tmdb.Candidate{{MediaInfo: &tt.media}}}
			result.Reconcile()
			if result.Status != tmdb.StatusReview || result.MediaInfo != nil || len(result.Candidates[0].ResolvedConflicts) != 0 {
				t.Fatalf("unresolved identity adopted: %+v", result)
			}
			found := false
			for _, conflict := range result.Candidates[0].Conflicts {
				found = found || conflict.Field == tt.field
			}
			if !found {
				t.Fatalf("missing %s: %+v", tt.field, result.Candidates[0])
			}
		})
	}
}

func TestReconcileUsesTMDBOrderForSameNameAndYear(t *testing.T) {
	for _, name := range []string{
		"预兆 (2023) - Harbin.2024.1080p.BluRay.x265.10bit.DTS-WiKi.mkv",
		"预兆.2023.mkv",
	} {
		t.Run(name, func(t *testing.T) {
			first := &tmdb.MediaInfo{Title: "预兆", OriginalTitle: "The Harbinger", Year: 2023, Type: medianame.TypeMovie, TMDBID: "974191", Popularity: 1}
			second := &tmdb.MediaInfo{Title: "预兆", OriginalTitle: "Augure", Year: 2023, Type: medianame.TypeMovie, TMDBID: "889818", Popularity: 10}
			info := medianame.Parse(name)
			result := tmdb.Result{MetaInfo: info, Candidates: []tmdb.Candidate{{MediaInfo: first}, {MediaInfo: second}}}
			result.Reconcile()
			if result.Status != tmdb.StatusCompatible || result.MediaInfo != first || result.SelectionBasis != "tmdb_order" ||
				len(result.Candidates) != 2 || !reflect.DeepEqual(result.MetaInfo, info) {
				t.Fatalf("TMDB order or original evidence lost: %+v", result)
			}
			preview, err := result.MediaInfo.Rename(info, "")
			if err != nil || !strings.HasPrefix(preview.Path, "预兆 (2023) {tmdb-974191}/The Harbinger.2023.") {
				t.Fatalf("preview=%+v err=%v", preview, err)
			}
			before := result.Candidates[0].ResolvedConflicts
			result.Reconcile()
			if result.MediaInfo != first || result.SelectionBasis != "tmdb_order" || !reflect.DeepEqual(before, result.Candidates[0].ResolvedConflicts) {
				t.Fatal("repeated reconciliation changed selection")
			}
			result.Candidates[0], result.Candidates[1] = result.Candidates[1], result.Candidates[0]
			result.Reconcile()
			if result.MediaInfo != second || result.SelectionBasis != "tmdb_order" {
				t.Fatal("TMDB response order was replaced by another ranking")
			}
			result.Truncated = true
			result.Reconcile()
			if result.Status != tmdb.StatusReview || result.MediaInfo != nil || result.SelectionBasis != "" {
				t.Fatal("partial search adopted or stale selection basis retained")
			}
		})
	}
}

func TestOmenUsesFirstTMDBSearchResultForRecognitionAndNaming(t *testing.T) {
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			if r.URL.Query().Get("query") != "预兆" || r.URL.Query().Get("year") != "2023" {
				t.Errorf("unexpected name/year query: %v", r.URL.Query())
			}
			respond(w, `{"total_pages":1,"total_results":2,"results":[
				{"id":974191,"title":"预兆","release_date":"2023-11-16"},
				{"id":889818,"title":"预兆","release_date":"2023-11-15"}]}`)
		case "/3/movie/974191":
			respond(w, `{"id":974191,"title":"预兆","original_title":"The Harbinger","release_date":"2023-11-16",
				"external_ids":{"imdb_id":"tt13610158"},"alternative_titles":{"titles":[{"title":"預兆"}]}}`)
		case "/3/movie/889818":
			respond(w, `{"id":889818,"title":"预兆","original_title":"Augure","release_date":"2023-11-15",
				"alternative_titles":{"titles":[{"title":"Omen"}]}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	const path = "搬运整理/欧美电影搬运/预兆 (2023)/预兆 (2023) - Harbin.2024.1080p.BluRay.x265.10bit.DTS-WiKi.mkv"
	result, err := client.RecognizePath(context.Background(), path)
	if err != nil || result.Status != tmdb.StatusCompatible || result.MediaInfo == nil || result.SelectionBasis != "tmdb_order" ||
		result.MediaInfo.TMDBID != "974191" || result.MediaInfo.OriginalTitle != "The Harbinger" || result.MediaInfo.Year != 2023 ||
		len(result.Candidates) != 2 || result.Candidates[1].MediaInfo.OriginalTitle != "Augure" || len(result.Candidates[1].MediaInfo.Raw) == 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.MetaInfo.Original != path || len(result.Candidates[0].ResolvedConflicts) != 2 || len(result.Candidates[0].Conflicts) != 0 {
		t.Fatal("original release title/year evidence lost")
	}
	preview, err := result.MediaInfo.Rename(result.MetaInfo, "")
	if err != nil || !strings.Contains(preview.Name, "The Harbinger.2023") || strings.Contains(preview.Name, "Harbin.") ||
		!strings.Contains(preview.Directory, "{tmdb-974191}") || !strings.Contains(preview.Name, "1080p") {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
}

func TestTMDBOrderRequiresMatchingIdentityMetadata(t *testing.T) {
	for _, tt := range []struct {
		name   string
		info   medianame.Info
		first  tmdb.MediaInfo
		second tmdb.MediaInfo
	}{
		{"official aliases", medianame.Info{Title: "输入名", Year: 2023, Type: medianame.TypeMovie},
			tmdb.MediaInfo{Title: "甲", Aliases: []string{"输入名"}, Year: 2023, Type: medianame.TypeMovie, TMDBID: "1"},
			tmdb.MediaInfo{Title: "乙", Aliases: []string{"输入名"}, Year: 2023, Type: medianame.TypeMovie, TMDBID: "2"}},
		{"mixed movie and TV", medianame.Info{Title: "Film", Year: 2023, Type: medianame.TypeUnknown},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "1"},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeTV, TMDBID: "2"}},
		{"missing input year", medianame.Info{Title: "Film", Type: medianame.TypeMovie},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "1"},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "2"}},
		{"external ID ambiguity", medianame.Info{Title: "Film", Year: 2023, Type: medianame.TypeMovie, IDs: medianame.MediaIDs{IMDb: "tt123"}},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "1", IMDbID: "tt123"},
			tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "2", IMDbID: "tt123"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := tmdb.Result{MetaInfo: tt.info, SelectionBasis: "tmdb_order", Candidates: []tmdb.Candidate{{MediaInfo: &tt.first}, {MediaInfo: &tt.second}}}
			result.Reconcile()
			if tt.name == "official aliases" {
				if result.Status != tmdb.StatusCompatible || result.MediaInfo != &tt.first || result.SelectionBasis != "tmdb_order" {
					t.Fatalf("official alias match not used: %+v", result)
				}
			} else if result.Status != tmdb.StatusAmbiguous || result.MediaInfo != nil || result.SelectionBasis != "" {
				t.Fatalf("TMDB order replaced identity metadata: %+v", result)
			}
		})
	}
}

func TestReconcileKeepsAmbiguityTruncationAndRequestFailures(t *testing.T) {
	first := &tmdb.MediaInfo{Title: "Film", Year: 2023, Type: medianame.TypeMovie, TMDBID: "1"}
	second := &tmdb.MediaInfo{Title: "Film", Year: 2025, Type: medianame.TypeMovie, TMDBID: "2"}
	result := tmdb.Result{MetaInfo: medianame.Parse("Film.2024.mkv"), Candidates: []tmdb.Candidate{{MediaInfo: first}, {MediaInfo: second}}}
	result.Reconcile()
	if result.Status != tmdb.StatusAmbiguous || result.MediaInfo != nil {
		t.Fatalf("ambiguous works selected: %+v", result)
	}
	result.Candidates = result.Candidates[:1]
	result.Truncated = true
	result.Reconcile()
	if result.Status != tmdb.StatusReview || result.MediaInfo != nil {
		t.Fatalf("truncated search selected: %+v", result)
	}
	result.Truncated = false
	result.Status = tmdb.StatusRequestError
	result.Reconcile()
	if result.Status != tmdb.StatusRequestError || result.MediaInfo != nil {
		t.Fatal("partial failure changed to success")
	}
	result.Status = tmdb.StatusReview
	result.Reconcile()
	before := result.Candidates[0].ResolvedConflicts
	result.Reconcile()
	if result.Status != tmdb.StatusCompatible || result.MediaInfo != first || !reflect.DeepEqual(before, result.Candidates[0].ResolvedConflicts) {
		t.Fatalf("repeated reconciliation changed result: %+v", result)
	}
	var empty *tmdb.Result
	empty.Reconcile()
	missing := tmdb.Result{Candidates: []tmdb.Candidate{{}}}
	missing.Reconcile()
	if missing.Status != tmdb.StatusReview || missing.MediaInfo != nil {
		t.Fatal("missing details adopted")
	}
	// 已有匹配年份的候选时，其他年份的同名结果不影响身份选择。
	matched := &tmdb.MediaInfo{Title: "Film", Year: 2024, Type: medianame.TypeMovie, TMDBID: "3"}
	result = tmdb.Result{MetaInfo: medianame.Parse("Film.2024.mkv"), Candidates: []tmdb.Candidate{{MediaInfo: first}, {MediaInfo: matched}}}
	result.Reconcile()
	if result.Status != tmdb.StatusCompatible || result.MediaInfo != matched || len(result.Candidates[0].ResolvedConflicts) != 0 {
		t.Fatalf("existing match displaced by other year: %+v", result)
	}
	// 名称已命中的候选优先，放宽片名差异不让其他搜索结果取代它。
	other := &tmdb.MediaInfo{Title: "Other Film", Year: 2024, Type: medianame.TypeMovie, TMDBID: "4"}
	result = tmdb.Result{MetaInfo: medianame.Parse("Film.2024.mkv"), Candidates: []tmdb.Candidate{{MediaInfo: other}, {MediaInfo: first}}}
	result.Reconcile()
	if result.Status != tmdb.StatusCompatible || result.MediaInfo != first || len(result.Candidates[0].ResolvedConflicts) != 0 {
		t.Fatalf("name match displaced: %+v", result)
	}
	// 多个都未命中输入名称的候选，不直接选搜索排序第一条。
	result = tmdb.Result{MetaInfo: medianame.Parse("Unknown.Title.2024.mkv"), Candidates: []tmdb.Candidate{{MediaInfo: first}, {MediaInfo: other}}}
	result.Reconcile()
	if result.Status != tmdb.StatusAmbiguous || result.MediaInfo != nil {
		t.Fatalf("multiple name corrections silently selected: %+v", result)
	}
}
