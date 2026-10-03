package medianame_test

import (
	"fmt"
	"testing"

	"medianame"
)

func ExampleParse() {
	info := medianame.Parse("Breaking.Bad.S01E02.1080p.WEB-DL.H264.mkv")
	fmt.Println(info.Title, *info.Season, *info.Episode, info.Resolution, info.VideoCodec)
	// Output: Breaking Bad 1 2 1080p H.264
}

func ExampleParsePath() {
	info := medianame.ParsePath("/tv/Example Show (2024) [tmdb=123]/Season 0/01.mkv")
	fmt.Println(info.Title, info.Year, *info.Season, *info.Episode, info.IDs.TMDB)
	// Output: Example Show 2024 0 1 123
}

func ExampleInfo_SearchQueries() {
	info := medianame.Parse("纵情女郎 (2006) - [工厂女孩].Factory.Girl.2006.1080p.mkv")
	for _, query := range info.SearchQueries() {
		fmt.Println(query.Text)
	}
	// Output:
	// 纵情女郎.2006 {[type=movie]}
	// 工厂女孩.2006 {[type=movie]}
	// Factory Girl.2006 {[type=movie]}
}

func ExampleInfo_CheckCandidate() {
	info := medianame.Parse("[咒怨(美版) 2004][原盘].mkv")
	conflicts := info.CheckCandidate(medianame.MediaCandidate{Title: "咒怨", Year: 2002, Type: medianame.TypeMovie})
	for _, conflict := range conflicts {
		fmt.Println(conflict.Field, conflict.Expected, conflict.Actual)
	}
	// Output:
	// year 2004 2002
	// title 咒怨(美版) 咒怨
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"", "Show.S00E00.mkv", "【字幕组】[动漫][01][1080p].mp4", "Film {[tmdbid=123;s=0;e=1-3]}", "\xff\x00", "[1080p]"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		info := medianame.Parse(name)
		if info.Original != name || info.Year < 0 {
			t.Fatalf("invalid result: %#v", info)
		}
		for _, value := range []*int{info.Season, info.SeasonEnd, info.Episode, info.EpisodeEnd} {
			if value != nil && *value < 0 {
				t.Fatalf("negative season/episode: %#v", info)
			}
		}
		// 路径入口也必须能处理非法 UTF-8 和任意路径分隔符，且保留原始输入。
		if pathInfo := medianame.ParsePath(name); pathInfo.Original != name {
			t.Fatalf("path original lost: %#v", pathInfo)
		}
	})
}
