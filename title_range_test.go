package medianame_test

import (
	"reflect"
	"testing"

	"github.com/xifofo/medianame"
)

func TestMixedChineseEnglishTitles(t *testing.T) {
	for _, tt := range []struct {
		name, chinese, english string
		year                   int
	}{
		{"凡人修仙传.The.Immortal.Ascension.2020.S01E05.2160p.WEB-DL.mkv", "凡人修仙传", "The Immortal Ascension", 2020},
		{"The_Immortal_Ascension_凡人修仙传_2020_S01E05.mkv", "凡人修仙传", "The Immortal Ascension", 2020},
		{"【幻月字幕组】【4月新番】【天国大魔境 Tengoku Daimakyou】【01】【1080P】【简日双语】.mp4", "天国大魔境", "Tengoku Daimakyou", 0},
		{"流浪地球2.The.Wandering.Earth.II.2023.1080p.mkv", "流浪地球2", "The Wandering Earth II", 2023},
		{"咒怨(美版).The.Grudge.2004.mkv", "咒怨(美版)", "The Grudge", 2004},
		{"[中文片名][English Title][2024][1080p].mkv", "中文片名", "English Title", 2024},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := medianame.Parse(tt.name)
			if info.Title != tt.chinese+" / "+tt.english || info.Original != tt.name || info.Year != tt.year {
				t.Fatal(asJSON(info))
			}
			queries := info.SearchQueries()
			if len(queries) != 2 || queries[0].Title != tt.chinese || queries[1].Title != tt.english {
				t.Fatalf("mixed names not searched separately: %+v", queries)
			}
			for _, query := range queries {
				if query.Type != info.Type || query.Year != info.Year || query.IDs != info.IDs {
					t.Fatalf("identity evidence lost: %+v", query)
				}
				if conflicts := info.CheckCandidate(medianame.MediaCandidate{Title: query.Title, Year: info.Year, Type: info.Type}); len(conflicts) != 0 {
					t.Fatalf("separated name not usable for candidate matching: %+v", conflicts)
				}
			}
		})
	}
}

func TestMixedTitleBoundaries(t *testing.T) {
	for _, title := range []string{
		"阿Q正传", "哆啦A梦", "X战警", "我的 AI 同学", "K 剧场版", "蜘蛛侠 Spider Man 英雄归来",
		"Film (导演剪辑版)", "片名 (English Edition)", "永久のユウグレ Dusk Beyond the End of the World",
		"오징어 게임 Squid Game", "Love and Peace", "你好 世界",
	} {
		if info := medianame.Parse(title + ".2024.mkv"); info.Title != title || len(info.SearchQueries()) != 1 {
			t.Errorf("split a single or unsupported title %q: %s", title, asJSON(info))
		}
	}
}

func TestMixedTitlesInPathsAndReleaseAliases(t *testing.T) {
	name := "/tv/凡人修仙传.The.Immortal.Ascension.2020.[tmdb=123]/Season 2/01.mkv"
	info := medianame.ParsePath(name)
	if info.Title != "凡人修仙传 / The Immortal Ascension" || info.Year != 2020 || info.IDs.TMDB != "123" ||
		!reflect.DeepEqual(info.Season, ptr(2)) || !reflect.DeepEqual(info.Episode, ptr(1)) || len(info.SearchQueries()) != 2 {
		t.Fatal(asJSON(info))
	}
	info = medianame.Parse("纵情女郎 (2006) - 工厂女孩.Factory.Girl.2006.1080p.mkv")
	if info.Title != "纵情女郎" || !reflect.DeepEqual(info.Aliases, []string{"工厂女孩", "Factory Girl"}) || len(info.SearchQueries()) != 3 {
		t.Fatal(asJSON(info))
	}
	info = medianame.Parse("预兆 (2023) - 哈尔滨.Harbin.2024.1080p.mkv")
	if info.Title != "预兆" || len(info.Aliases) != 0 || !reflect.DeepEqual(info.Warnings, []string{"conflicting_years"}) {
		t.Fatal("conflicting release identity became an alias: ", asJSON(info))
	}
}

func TestMixedTitleKeepsIdentityConflicts(t *testing.T) {
	info := medianame.Parse("中文片名.English.Title.2024.{[type=movie;imdb=tt123]}.mkv")
	candidate := medianame.MediaCandidate{Title: "English Title", Year: 2024, Type: medianame.TypeMovie, IDs: medianame.MediaIDs{IMDb: "tt123"}}
	for _, tt := range []struct {
		field string
		alter func(*medianame.MediaCandidate)
	}{
		{"year", func(c *medianame.MediaCandidate) { c.Year = 2023 }},
		{"type", func(c *medianame.MediaCandidate) { c.Type = medianame.TypeTV }},
		{"imdb_id", func(c *medianame.MediaCandidate) { c.IDs.IMDb = "tt456" }},
	} {
		changed := candidate
		tt.alter(&changed)
		conflicts := info.CheckCandidate(changed)
		found := false
		for _, conflict := range conflicts {
			found = found || conflict.Field == tt.field
		}
		if !found {
			t.Fatalf("matching a split name suppressed %s: %+v", tt.field, conflicts)
		}
	}
	for _, query := range info.SearchQueries() {
		if query.IDs.IMDb != "tt123" || query.Type != medianame.TypeMovie || query.Year != 2024 {
			t.Fatalf("query lost identity constraints: %+v", query)
		}
	}
}

func TestCompletedEpisodeRanges(t *testing.T) {
	for _, tt := range []struct {
		name, title, group  string
		season, first, last int
	}{
		{"[01-26Fin] 某动画 1080p", "某动画", "", 1, 1, 26},
		{"[01-26eNd] Some Anime 1080p.mkv", "Some Anime", "", 1, 1, 26},
		{"01-26完结 某动画 1080p.mkv", "某动画", "", 1, 1, 26},
		{"【01～26 完結】某动画.1080p.mkv", "某动画", "", 1, 1, 26},
		{"[MoonGroup][01-26Fin][Some Anime][1080p].mkv", "Some Anime", "MoonGroup", 1, 1, 26},
		{"[01-26Fin][MoonGroup][Some Anime][1080p].mkv", "Some Anime", "MoonGroup", 1, 1, 26},
		{"[Some Anime][01-26Fin][1080p].mkv", "Some Anime", "", 1, 1, 26},
		{"Some Anime 01-26End 1080p.mkv", "Some Anime", "", 1, 1, 26},
		{"Some.Anime.S02.[01-26完结].1080p.mkv", "Some Anime", "", 2, 1, 26},
		{"Some Anime [26-01 Fin][1080p].mkv", "Some Anime", "", 1, 1, 26},
		{"Some Anime S00 [00-12End][1080p].mkv", "Some Anime", "", 0, 0, 12},
		{"[1214-1216Fin] CONAN [1080p].mkv", "CONAN", "", 1, 1214, 1216},
		{"[01-26Fin]", "", "", 1, 1, 26},
		{"[01-26Fin] 某动画.Some.Anime.2024.1080p.mkv", "某动画 / Some Anime", "", 1, 1, 26},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := medianame.Parse(tt.name)
			if info.Title != tt.title || info.ReleaseGroup != tt.group || info.Type != medianame.TypeTV ||
				!reflect.DeepEqual(info.Season, ptr(tt.season)) || !reflect.DeepEqual(info.Episode, ptr(tt.first)) ||
				!reflect.DeepEqual(info.EpisodeEnd, ptr(tt.last)) || info.Original != tt.name {
				t.Fatal(asJSON(info))
			}
		})
	}
}

func TestCompletedRangeDoesNotOverrideEpisodes(t *testing.T) {
	for _, name := range []string{
		"[01-26Fin] Show.S02E03.1080p.mkv",
		"Show.S02E03.[01-26Fin].1080p.mkv",
		"[01-26End] Show.1080p.{[type=tv;s=2;e=3]}.mkv",
	} {
		info := medianame.Parse(name)
		if info.Title != "Show" || !reflect.DeepEqual(info.Season, ptr(2)) || !reflect.DeepEqual(info.Episode, ptr(3)) || info.EpisodeEnd != nil {
			t.Fatalf("overrode explicit episodes: %s", asJSON(info))
		}
	}
	info := medianame.Parse("[01-26Fin] Show.S01E01E03E05.mkv")
	if !reflect.DeepEqual(info.Episodes, []int{1, 3, 5}) || info.EpisodeEnd != nil {
		t.Fatal("filled gaps in explicit episode list: ", asJSON(info))
	}
	info = medianame.Parse("[01-26Fin] Show.S01E10-S02E03.mkv")
	if !reflect.DeepEqual(info.SeasonEnd, ptr(2)) || !reflect.DeepEqual(info.Episode, ptr(10)) || !reflect.DeepEqual(info.EpisodeEnd, ptr(3)) {
		t.Fatal("overrode cross-season range: ", asJSON(info))
	}
	info = medianame.Parse("[01-26Fin] Film {[type=movie]}.mkv")
	if info.Type != medianame.TypeMovie {
		t.Fatal("overrode explicit media type: ", asJSON(info))
	}
}

func TestCompletedRangeFalsePositives(t *testing.T) {
	for _, name := range []string{
		"Some Anime [01-26Final] 1080p.mkv", "Some Anime [01-26Endless] 1080p.mkv",
		"Some Anime [01-26完结篇] 1080p.mkv", "Some Anime [01-26End2] 1080p.mkv",
		"Some Anime [10001-10026Fin] 1080p.mkv", "Some Anime [2020-2024End] 1080p.mkv",
		"Some Anime [1990-2000完结] 1080p.mkv", "Film01-26Fin.1080p.mkv",
		"The.End.2024.mkv", "1917.mkv", "Show.01-26.1080p.mkv",
	} {
		info := medianame.Parse(name)
		if info.Episode != nil || info.EpisodeEnd != nil || info.Type != medianame.TypeMovie {
			t.Fatalf("invented episode range for %q: %s", name, asJSON(info))
		}
	}
}
