package medianame_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xifofo/medianame"
)

func TestRealShadowRegressions(t *testing.T) {
	tests := []struct {
		name, title, resolution, channels string
		year                              int
	}{
		{"金钱世界 (2017) - 金钱世界 (2017).2160p.WEB-DL.HDR.HEVC.10-bit.DTS-HD MA 5.1.mkv", "金钱世界", "2160p", "5.1", 2017},
		{"饥饿游戏3：嘲笑鸟（上） (2014) - [饥饿游戏3：嘲笑鸟(上)].The.Hunger.Games.Mockingjay.-.Part.1.2014.HKG.BluRay.1080p.x264.DDP.7.1.2Audios-CMCT.mkv", "饥饿游戏3：嘲笑鸟(上)", "1080p", "7.1", 2014},
		{"[咒怨(美版) 2004][美版原盘 加长版 DIY加长版简繁字幕].mkv", "咒怨(美版)", "", "", 2004},
		{"二零一二 (2009) - [2012].2009.USA.BluRay.1080p.x264.DDP.5.1.2Audios-CMCT.mkv", "二零一二", "1080p", "5.1", 2009},
		{"新万圣节.2007.1080p.Remux.AVC.TrueHD 5.1-404.mkv", "新万圣节", "1080p", "5.1", 2007},
		{"月光光心慌慌2.1981.2160p.Remux.DoVi.HDR10.HEVC.TrueHD Dolby Atmos 7.1-404.mkv", "月光光心慌慌2", "2160p", "7.1", 1981},
		{"飞机陷落.2023.2160p.REMUX.Dolby Vision, HDR10.h265.TrueHD Atmos.7.1-404.mkv", "飞机陷落", "2160p", "7.1", 2023},
		{"天衣无缝 (2007) - 完美无瑕.Flawless.2007.BD720P.中英双字.mkv", "天衣无缝", "720p", "", 2007},
		{"独孤里桥之役 (1954) - 独孤里桥之役.特效中英字幕.The.Bridge.at.Tokyo-Ri.1954.BD1080P.X264.DTS-HD.MA.2.0.English.CHS-ENG.FFans@星星.mkv", "独孤里桥之役", "1080p", "2.0", 1954},
		{"精灵旅社3：疯狂假期 (2018) - 精灵旅社3：疯狂假期 (2018).2160p.Ultra HD BluRay.Remux HDR 10-bit.H.265.TrueHD 7.1-SGNB.mkv", "精灵旅社3：疯狂假期", "2160p", "7.1", 2018},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := medianame.Parse(tt.name)
			if got.Title != tt.title || got.Year != tt.year || got.Resolution != tt.resolution || got.AudioChannels != tt.channels || got.Episode != nil || got.Season != nil || got.Type != medianame.TypeMovie {
				t.Fatalf("unexpected result: %s", asJSON(got))
			}
		})
	}
}

func TestTitleBracketsAndYearConflict(t *testing.T) {
	for _, tt := range []struct{ input, title string }{
		{"Film (Director's Cut).2024.mkv", "Film (Director's Cut)"},
		{"[Film (Part One)].2024.mkv", "Film (Part One)"},
		{"((Film)).2024.mkv", "Film"},
		{"[中文片名][English Title][2024][1080p].mkv", "中文片名 / English Title"},
		{"Film (One) and Film (Two).2024.mkv", "Film (One) and Film (Two)"},
		{strings.Repeat("(", 10000) + "Film" + strings.Repeat(")", 10000) + ".2024.mkv", "Film"},
	} {
		if got := medianame.Parse(tt.input); got.Title != tt.title {
			t.Errorf("title=%q want %q", got.Title, tt.title)
		}
	}
	got := medianame.Parse("预兆 (2023) - Harbin.2024.1080p.BluRay.mkv")
	if got.Title != "预兆" || got.Year != 2023 || !reflect.DeepEqual(got.Warnings, []string{"conflicting_years"}) {
		t.Fatal(asJSON(got))
	}
	if got := medianame.Parse("二零一二 (2009) - [2012].2009.1080p.mkv"); len(got.Warnings) != 0 {
		t.Fatal(asJSON(got))
	}
}

func TestBluRayPathInheritance(t *testing.T) {
	for _, filename := range []string{
		"/movies/哈利叔叔的不寻常的韵事 (1945) {tmdb-123}/BDMV/STREAM/00005.m2ts",
		`C:\movies\哈利叔叔的不寻常的韵事 (1945) {tmdb-123}\bdmv\stream\00000.M2TS`,
	} {
		got := medianame.ParsePath(filename)
		if got.Title != "哈利叔叔的不寻常的韵事" || got.Year != 1945 || got.IDs.TMDB != "123" || got.Source != "BluRay" || got.Episode != nil || got.Type != medianame.TypeMovie {
			t.Fatal(asJSON(got))
		}
	}
	// 缺少媒体根目录时宁可没有片名，也不继承技术目录；普通数字剧集、真实片名仍照旧。
	if got := medianame.ParsePath("BDMV/STREAM/00001.m2ts"); got.Title != "" || got.Episode != nil {
		t.Fatal(asJSON(got))
	}
	if got := medianame.ParsePath("/Film (2024)/STREAM/00001.m2ts"); got.Title != "00001" || got.Year != 0 {
		t.Fatal(asJSON(got))
	}
	if got := medianame.ParsePath("/movies/Stream (2024)/Stream.2024.mkv"); got.Title != "Stream" {
		t.Fatal(asJSON(got))
	}
	if got := medianame.ParsePath("/tv/Show/Season 1/01.mkv"); got.Episode == nil || *got.Episode != 1 || got.Type != medianame.TypeTV {
		t.Fatal(asJSON(got))
	}
	if got := medianame.ParsePath("/tv/Show.S02/BDMV/STREAM/00001.m2ts"); got.Season == nil || *got.Season != 2 || got.Episode != nil || got.Type != medianame.TypeTV {
		t.Fatal(asJSON(got))
	}
}

func TestSearchQueriesAndCandidateValidation(t *testing.T) {
	info := medianame.Parse("肮脏天使 (2024) - Dirty.Angels.2024.1080p.x264-404.mkv")
	queries := info.SearchQueries()
	if len(queries) != 2 || queries[0].Text != "肮脏天使.2024 {[type=movie]}" || queries[1].Text != "Dirty Angels.2024 {[type=movie]}" {
		t.Fatal(queries)
	}
	good := medianame.MediaCandidate{Title: "肮脏天使", OriginalTitle: "Dirty Angels", Year: 2024, Type: medianame.TypeMovie}
	if conflicts := info.CheckCandidate(good); len(conflicts) != 0 {
		t.Fatal(conflicts)
	}
	for _, bad := range []medianame.MediaCandidate{
		{Title: "肮脏天使", Year: 2023, Type: medianame.TypeMovie},
		{Title: "Another Film", Year: 2024, Type: medianame.TypeMovie},
		{Title: "肮脏天使", Type: medianame.TypeMovie},
	} {
		if len(info.CheckCandidate(bad)) == 0 {
			t.Fatal("accepted conflict", bad)
		}
	}
	info = medianame.Parse("[咒怨(美版) 2004][美版原盘].mkv")
	if len(info.CheckCandidate(medianame.MediaCandidate{Title: "咒怨", Year: 2002, Type: medianame.TypeMovie})) != 2 {
		t.Fatal("version/year conflict lost")
	}
	info = medianame.Parse("Film.2024 {[tmdbid=123;type=movie]}")
	queries = info.SearchQueries()
	parsed := medianame.Parse(queries[0].Text)
	if parsed.IDs.TMDB != "123" || parsed.Type != medianame.TypeMovie || parsed.Year != 2024 {
		t.Fatal(queries)
	}
	for _, bad := range []medianame.MediaCandidate{
		{Title: "Film", Year: 2024, Type: medianame.TypeTV, IDs: medianame.MediaIDs{TMDB: "123"}},
		{Title: "Film", Year: 2024, Type: medianame.TypeMovie, IDs: medianame.MediaIDs{TMDB: "456"}},
		{Title: "Film", Year: 2024, Type: medianame.TypeMovie},
	} {
		if len(info.CheckCandidate(bad)) == 0 {
			t.Fatal("accepted ID/type conflict", bad)
		}
	}
	info = medianame.Parse("[中文片名][English Title][2024][1080p].mkv")
	if len(info.SearchQueries()) != 2 || len(info.CheckCandidate(medianame.MediaCandidate{Title: "English Title", Year: 2024, Type: medianame.TypeMovie})) != 0 {
		t.Fatal(info.SearchQueries())
	}
	info = medianame.Parse("预兆 (2023) - Harbin.2024.1080p.mkv")
	if len(info.CheckCandidate(medianame.MediaCandidate{Title: "预兆", Year: 2023, Type: medianame.TypeMovie})) == 0 {
		t.Fatal("input conflict lost")
	}
	if len(medianame.Parse("").SearchQueries()) != 0 {
		t.Fatal("empty title query")
	}
}

func TestReleaseAliasesKeepIdentityEvidence(t *testing.T) {
	info := medianame.Parse("纵情女郎 (2006) - [工厂女孩].Factory.Girl.2006.AUS.BluRay.1080p.x264-CMCT.mkv")
	if !reflect.DeepEqual(info.Aliases, []string{"工厂女孩", "Factory Girl"}) {
		t.Fatal(asJSON(info))
	}
	if len(info.CheckCandidate(medianame.MediaCandidate{Title: "工厂女孩", OriginalTitle: "Factory Girl", Year: 2006, Type: medianame.TypeMovie})) != 0 {
		t.Fatal("same-year alias rejected")
	}
	info = medianame.Parse("预兆 (2023) - Harbin.2024.1080p.mkv")
	if len(info.Aliases) != 0 || len(info.SearchQueries()) != 1 {
		t.Fatal("mixed conflicting identity", asJSON(info))
	}
	info = medianame.Parse("金钱世界 (2017) - 金钱世界 (2017).1080p.mkv")
	if len(info.SearchQueries()) != 1 {
		t.Fatal("duplicate query", info.SearchQueries())
	}
}
