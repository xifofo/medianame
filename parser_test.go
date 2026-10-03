package medianame_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/xifofo/medianame"
)

func ptr(n int) *int { return &n }

func TestParseNames(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		year       int
		season     *int
		episode    *int
		seasonEnd  *int
		episodeEnd *int
	}{
		{name: "Breaking.Bad.S01E02.1080p.WEB-DL.H264.mkv", title: "Breaking Bad", season: ptr(1), episode: ptr(2)},
		{name: "Arrival.2016.2160p.BluRay.x265-GROUP.mkv", title: "Arrival", year: 2016},
		{name: "流浪地球2.2023.2160p.WEB-DL.H265.mkv", title: "流浪地球2", year: 2023},
		{name: "三体 第十二集 1080p.mp4", title: "三体", season: ptr(1), episode: ptr(12)},
		{name: "示例剧 第二季 第三十二话.mkv", title: "示例剧", season: ptr(2), episode: ptr(32)},
		{name: "示例剧 第两季 第零集.mkv", title: "示例剧", season: ptr(2), episode: ptr(0)},
		{name: "Show.S00E00.720p.mkv", title: "Show", season: ptr(0), episode: ptr(0)},
		{name: "Show.S00.x265.AAC.mkv", title: "Show", season: ptr(0)},
		{name: "Show.S01E02-E05.mkv", title: "Show", season: ptr(1), episode: ptr(2), episodeEnd: ptr(5)},
		{name: "Show.S01E02-05.mkv", title: "Show", season: ptr(1), episode: ptr(2), episodeEnd: ptr(5)},
		{name: "Show.S01E02-S01E05.mkv", title: "Show", season: ptr(1), episode: ptr(2), episodeEnd: ptr(5)},
		{name: "Show.S01E10-S02E03.mkv", title: "Show", season: ptr(1), episode: ptr(10), seasonEnd: ptr(2), episodeEnd: ptr(3)},
		{name: "Show.S01E05-E02.mkv", title: "Show", season: ptr(1), episode: ptr(2), episodeEnd: ptr(5)},
		{name: "Show.S01-S03.1080p", title: "Show", season: ptr(1), seasonEnd: ptr(3)},
		{name: "Show.S03-S00.1080p", title: "Show", season: ptr(0), seasonEnd: ptr(3)},
		{name: "Show.Season.2.Episode.4.mkv", title: "Show", season: ptr(2), episode: ptr(4)},
		{name: "Show.2x04-06.mkv", title: "Show", season: ptr(2), episode: ptr(4), episodeEnd: ptr(6)},
		{name: "Show.EP14.2026.1080p.mp4", title: "Show", year: 2026, season: ptr(1), episode: ptr(14)},
		{name: "示例剧 第十二-十四集.mkv", title: "示例剧", season: ptr(1), episode: ptr(12), episodeEnd: ptr(14)},
		{name: "示例剧 第12集-第14集.mkv", title: "示例剧", season: ptr(1), episode: ptr(12), episodeEnd: ptr(14)},
		{name: "示例剧 第一季-第三季.mkv", title: "示例剧", season: ptr(1), seasonEnd: ptr(3)},
		{name: "[MoonGroup] Sample Anime - 07v2 [1080p][HEVC][AAC].mkv", title: "Sample Anime", season: ptr(1), episode: ptr(7)},
		{name: "【月光字幕组】[示例动漫 第二季][11][1080p][x264].mp4", title: "示例动漫", season: ptr(2), episode: ptr(11)},
		{name: "[MoonGroup][Sample Anime][01-12][720p].mkv", title: "Sample Anime", season: ptr(1), episode: ptr(1), episodeEnd: ptr(12)},
		{name: "[Sample Anime][01][1080p].mkv", title: "Sample Anime", season: ptr(1), episode: ptr(1)},
		{name: "Sample Anime [01][1080p][AAC][DEADBEEF].mkv", title: "Sample Anime", season: ptr(1), episode: ptr(1)},
		{name: "2001.A.Space.Odyssey.1968.1080p.mkv", title: "2001 A Space Odyssey", year: 1968},
		{name: "1917.2019.1080p.mkv", title: "1917", year: 2019},
		{name: "1917.mkv", title: "1917"},
		{name: "24.S01.1080p.mkv", title: "24", season: ptr(1)},
		{name: "新精武门1991 (1991).mkv", title: "新精武门1991", year: 1991},
		{name: "Example 2021 (2022) - 1080p.mp4", title: "Example 2021", year: 2022},
		{name: "Amazon.Forever.2004.1080p.WEB-DL.mkv", title: "Amazon Forever", year: 2004},
		{name: "The.WEB.2024.1080p.mkv", title: "The WEB", year: 2024},
		{name: "AV1.mkv", title: "AV1"},
		{name: "The.S01E02Story.2024.mkv", title: "The S01E02Story", year: 2024},
		{name: "Name.1080p.BluRay.mkv", title: "Name"},
		{name: "电影（2024）［1080p］.mkv", title: "电影", year: 2024},
		{name: "Season 0", season: ptr(0)},
		{name: "E00.mkv", season: ptr(1), episode: ptr(0)},
		{name: "", title: ""},
		{name: " \n\t ", title: ""},
		{name: "Untitled Movie", title: "Untitled Movie"},
		{name: "Wonder Woman 1984 2020 BluRay 1080p.mkv", title: "Wonder Woman 1984", year: 2020},
		{name: "[MoonSubs] Anime S2 - 03v2 [WebRip 1080p HEVC AAC].mkv", title: "Anime", season: ptr(2), episode: ptr(3)},
		{name: "【月光字幕组】【22年日剧】【示例电视剧】【04】【1080P】【中日双语】", title: "示例电视剧", season: ptr(1), episode: ptr(4)},
		{name: "【MoonSubs】★04月新番★[Example Anime][15][720p]", title: "Example Anime", season: ptr(1), episode: ptr(15)},
		{name: "[MoonGroup][国漫][示例动漫 第1季][01][1080P]", title: "示例动漫", season: ptr(1), episode: ptr(1)},
		{name: "[MoonRaws] アニメ #13 (BD HEVC 1920x1080 FLAC).mkv", title: "アニメ", season: ptr(1), episode: ptr(13)},
		{name: "[TEAM] Film.2024.1080p.BluRay.mkv", title: "Film", year: 2024},
		{name: "[中文片名][English Title][2024][1080p].mkv", title: "中文片名 / English Title", year: 2024},
		{name: "Sample Anime [09] [1080p] [2022年7月番]", title: "Sample Anime", season: ptr(1), episode: ptr(9)},
		{name: "Sample Anime [TV 01-26 Fin][1080p]", title: "Sample Anime", season: ptr(1), episode: ptr(1), episodeEnd: ptr(26)},
		{name: "1080p.mkv", title: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianame.Parse(tt.name)
			if got.Original != tt.name || got.Title != tt.title || got.Year != tt.year ||
				!reflect.DeepEqual(got.Season, tt.season) || !reflect.DeepEqual(got.Episode, tt.episode) ||
				!reflect.DeepEqual(got.SeasonEnd, tt.seasonEnd) || !reflect.DeepEqual(got.EpisodeEnd, tt.episodeEnd) {
				t.Errorf("Parse(%q) = %s; want title=%q year=%d season=%v episode=%v seasonEnd=%v episodeEnd=%v",
					tt.name, asJSON(got), tt.title, tt.year, intValue(tt.season), intValue(tt.episode), intValue(tt.seasonEnd), intValue(tt.episodeEnd))
			}
			wantType := medianame.TypeUnknown
			if tt.title != "" {
				wantType = medianame.TypeMovie
			}
			if tt.season != nil || tt.episode != nil {
				wantType = medianame.TypeTV
			}
			if got.Type != wantType {
				t.Errorf("type=%q; want %q", got.Type, wantType)
			}
		})
	}
}

func TestTechnicalInfo(t *testing.T) {
	tests := []struct {
		name string
		want map[string]any
	}{
		{"Film.2024.2160p.AMZN.WEB-DL.DDP5.1.H.265.10bit-GROUP.mkv", map[string]any{
			"resolution": "2160p", "source": "WEB-DL", "video_codec": "H.265", "audio_codec": "DDP",
			"audio_channels": "5.1", "bit_depth": float64(10), "release_group": "GROUP", "streaming_service": "Amazon", "extension": ".mkv"}},
		{"Film.2024.1080p.Blu-ray.DTS-HD.MA5.1.x264.mkv", map[string]any{
			"source": "BluRay", "video_codec": "H.264", "audio_codec": "DTS-HD MA", "audio_channels": "5.1"}},
		{"Film.2024.4K.BluRay.REMUX.TrueHD.7.1.Atmos.DV.HDR10+.HEVC.mkv", map[string]any{
			"resolution": "2160p", "source": "REMUX", "audio_codec": "TrueHD", "audio_channels": "7.1",
			"effects": []any{"Atmos", "Dolby Vision", "HDR10+"}, "video_codec": "H.265"}},
		{"Film.2024.1080i.HDTV.AAC2.0.H264-TEST.ts", map[string]any{
			"resolution": "1080i", "source": "HDTV", "audio_codec": "AAC", "audio_channels": "2.0", "extension": ".ts"}},
		{"Film.2024.1920x1080p.WEBRip.AV1.FLAC.23.976fps.mkv", map[string]any{
			"resolution": "1080p", "source": "WEBRip", "video_codec": "AV1", "audio_codec": "FLAC", "fps": 23.976}},
		{"Film.2024.2160p.HDRVivid.H265.50Fps.10bit.mkv", map[string]any{
			"effects": []any{"HDR Vivid"}, "bit_depth": float64(10), "fps": float64(50)}},
		{"[MoonGroup] Sample Anime - 07v2 [1080p][Hi10P][AAC][DEADBEEF].mkv", map[string]any{
			"release_group": "MoonGroup", "episode_version": float64(2), "bit_depth": float64(10), "audio_codec": "AAC"}},
		{"Film.2024.720p.DSNP.WEB-DL.E-AC-3.5.1.H264.mkv", map[string]any{
			"streaming_service": "Disney+", "audio_codec": "E-AC-3", "audio_channels": "5.1"}},
		{"Film.2024.1080p.NF.WEB-DL.H264[TEAM].MKV", map[string]any{
			"streaming_service": "Netflix", "release_group": "TEAM", "extension": ".mkv"}},
		{"Film.2024.Part2.1080p.mkv", map[string]any{"part": float64(2)}},
		{"Show.S01E01E03E05.mkv", map[string]any{"episodes": []any{float64(1), float64(3), float64(5)}}},
		{"[MoonRaws] アニメ #13 (BD HEVC 1920x1080 yuv444p10le FLAC).mkv", map[string]any{
			"resolution": "1080p", "bit_depth": float64(10), "release_group": "MoonRaws"}},
		{"Film.2024.[2160].x265.mkv", map[string]any{"resolution": "2160p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianame.Parse(tt.name)
			data := map[string]any{}
			if err := json.Unmarshal([]byte(asJSON(got)), &data); err != nil {
				t.Fatal(err)
			}
			for key, want := range tt.want {
				if !reflect.DeepEqual(data[key], want) {
					t.Errorf("%s=%v; want %v; result=%s", key, data[key], want, asJSON(got))
				}
			}
		})
	}
}

func TestNoInventedTechnicalInfo(t *testing.T) {
	for _, name := range []string{"Amazon Forever 2004 1080p WEB-DL", "The WEB 2024", "DV 2024", "AV1", "Part 2", "Title.2024.x265", "Title.2024.AAC", "Anime [01][1080p][MP4]", "Anime [01][1080p][GB]", "Film.2024.1080p[3.4GB]"} {
		info := medianame.Parse(name)
		if info.ReleaseGroup != "" {
			t.Errorf("%q: invented release group %q", name, info.ReleaseGroup)
		}
		if strings.HasPrefix(name, "Amazon") && info.StreamingService != "" {
			t.Errorf("title word became streaming service: %s", asJSON(info))
		}
		if strings.HasPrefix(name, "The WEB") && info.Source != "" {
			t.Errorf("title word became source: %s", asJSON(info))
		}
		if info.Extension != "" {
			t.Errorf("metadata suffix became extension: %s", asJSON(info))
		}
	}
}

func TestFourDigitAnimeEpisodesAndChecksum(t *testing.T) {
	const name = `[SBSUB][CONAN][1214][WEBRIP][1080P][HEVC_AAC][CHS_CHT_JP][PGS]\(C9B12A40).mkv`
	for _, input := range []string{name, strings.ReplaceAll(name, "C9B12A40", "E5B7FCE8")} {
		info := medianame.Parse(input)
		if info.Title != "CONAN" || info.Type != medianame.TypeTV || info.Episode == nil || *info.Episode != 1214 ||
			info.Year != 0 || info.Season == nil || *info.Season != 1 || info.ReleaseGroup != "SBSUB" || info.Source != "WEBRip" ||
			info.Resolution != "1080p" || info.VideoCodec != "H.265" || info.AudioCodec != "AAC" || info.Extension != ".mkv" || info.Original != input {
			t.Fatalf("long episode or release metadata lost: %s", asJSON(info))
		}
		queries := info.SearchQueries()
		if len(queries) != 1 || queries[0].Title != "CONAN" || queries[0].Type != medianame.TypeTV {
			t.Fatalf("group or episode leaked into name searches: %+v", queries)
		}
	}
	for _, input := range []string{
		"[SBSUB][CONAN][1214-1216v2][1080p].mkv",
		"[SBSUB][CONAN][1216-1214v2][1080p].mkv",
		"CONAN - 1214-1216v2 [1080p].mkv",
	} {
		info := medianame.Parse(input)
		if info.Title != "CONAN" || info.Episode == nil || *info.Episode != 1214 || info.EpisodeEnd == nil ||
			*info.EpisodeEnd != 1216 || info.EpisodeVersion != 2 || info.Type != medianame.TypeTV {
			t.Errorf("four-digit range/version lost: %s", asJSON(info))
		}
	}
	for _, input := range []string{"Film [2024][1080p].mkv", "[Film][2024][1080p].mkv", "Film - 2024 [1080p].mkv"} {
		info := medianame.Parse(input)
		if info.Title != "Film" || info.Year != 2024 || info.Episode != nil || info.Type != medianame.TypeMovie {
			t.Errorf("year became an episode: %s", asJSON(info))
		}
	}
	info := medianame.Parse("[SBSUB][CONAN][2024][1214][1080p].mkv")
	if info.Title != "CONAN" || info.Year != 2024 || info.Episode == nil || *info.Episode != 1214 || info.ReleaseGroup != "SBSUB" {
		t.Errorf("year hid the later episode: %s", asJSON(info))
	}
	for _, input := range []string{"CONAN.1080p.(E5B7FCE8).mkv", "CONAN [12145][1080p].mkv"} {
		if info := medianame.Parse(input); info.Episode != nil {
			t.Errorf("checksum or oversized number became an episode: %s", asJSON(info))
		}
	}
}

func TestIDs(t *testing.T) {
	for _, provider := range []string{"tmdb", "tvdb", "douban", "bangumi", "anilist"} {
		for _, alias := range []string{provider, provider + "id"} {
			for _, brackets := range [][2]string{{"[", "]"}, {"{", "}"}, {"｛", "｝"}} {
				for _, separator := range []string{"=", "-"} {
					name := "Film.2024 " + brackets[0] + alias + separator + "12345678901234567890" + brackets[1] + ".mkv"
					t.Run(name, func(t *testing.T) {
						info := medianame.Parse(name)
						data := map[string]string{}
						encoded, _ := json.Marshal(info.IDs)
						if err := json.Unmarshal(encoded, &data); err != nil {
							t.Fatal(err)
						}
						if data[provider] != "12345678901234567890" || info.Title != "Film" {
							t.Errorf("unexpected result: %s", asJSON(info))
						}
					})
				}
			}
		}
	}
	for _, tag := range []string{"[tmdb=0]", "[tmdb=000]", "[tmdb=abc]", "[tmdb=tt123]", "[tmdb=123456789012345678901]", "[imdb=123]"} {
		if info := medianame.Parse("Film " + tag); info.IDs != (medianame.MediaIDs{}) {
			t.Errorf("invalid ID accepted: %s", asJSON(info))
		}
	}
	info := medianame.Parse("Film {[tmdbid=123;type=movies;s=0;e=2-4;g=abc123]} [tmdbid=456] [imdb-tt1234567]")
	if info.Title != "Film" || info.Type != medianame.TypeMovie || info.IDs.TMDB != "456" || info.IDs.IMDb != "tt1234567" ||
		info.Season == nil || *info.Season != 0 || info.Episode == nil || *info.Episode != 2 || info.EpisodeEnd == nil || *info.EpisodeEnd != 4 || info.EpisodeGroup != "abc123" {
		t.Errorf("composite tags: %s", asJSON(info))
	}
	info = medianame.Parse("Show {[TMDBID=123;TYPE=TV;S=0;E=4]}")
	if info.Season == nil || *info.Season != 0 || info.Episode == nil || *info.Episode != 4 || info.IDs.TMDB != "123" {
		t.Errorf("uppercase composite tags: %s", asJSON(info))
	}
}

func TestRecoverMismatchedIDBracket(t *testing.T) {
	const name = "映像 (1975) - 映像 (1974)｛tmdbid-67018）.mkv"
	const path = "搬运整理/欧美电影搬运/映像 (1975)/" + name
	for _, info := range []medianame.Info{medianame.Parse(name), medianame.ParsePath(path)} {
		if info.Title != "映像" || info.Year != 1975 || info.Type != medianame.TypeMovie || info.IDs.TMDB != "67018" ||
			info.Extension != ".mkv" || !reflect.DeepEqual(info.Warnings, []string{"conflicting_years"}) ||
			!strings.Contains(info.Original, "｛tmdbid-67018）") {
			t.Fatalf("explicit ID or original evidence lost: %s", asJSON(info))
		}
	}
	for _, tag := range []string{"{tmdbid-67018)", "｛TMDBID=67018）", "｛ tmdb-67018 )"} {
		if info := medianame.Parse("Film " + tag); info.Title != "Film" || info.IDs.TMDB != "67018" {
			t.Errorf("explicit ID not recovered: %s", asJSON(info))
		}
	}
	info := medianame.Parse("Film ｛imdbid-tt0073589） [tmdb=67018] ｛tmdbid=123）")
	if info.Title != "Film" || info.IDs.IMDb != "tt0073589" || info.IDs.TMDB != "123" {
		t.Errorf("ID validation or tag order changed: %s", asJSON(info))
	}
	for _, tag := range []string{
		"｛tmdbid-0）", "｛tmdbid-000）", "｛tmdbid-tt67018）", "｛tmdbid-abc）",
		"｛tmdbid-67018abc）", "｛tmdbid-123456789012345678901）", "｛imdbid-67018）",
		"｛tmdbid-67018;type=movie）", "｛a tmdbid-67018）", "｛tmdbid-67018 sequel）",
		"｛tmdbid-67018", "{tmdbid-67018) sequel}", "(67018)", "(The tmdbid-67018 Story)",
	} {
		if info := medianame.Parse("Film " + tag); info.IDs != (medianame.MediaIDs{}) {
			t.Errorf("non-ID or invalid tag accepted: %s", asJSON(info))
		}
	}
}

func TestJSONKeepsSeasonZero(t *testing.T) {
	got := asJSON(medianame.Parse("Show.S00E00.mkv"))
	if !strings.Contains(got, `"season":0`) || !strings.Contains(got, `"episode":0`) {
		t.Fatalf("zero values lost: %s", got)
	}
	for _, name := range []string{"Show.E01.mkv", "Show {[type=tv]}.mkv", "{[type=tv]}"} {
		got = asJSON(medianame.Parse(name))
		if !strings.Contains(got, `"season":1`) {
			t.Errorf("default season missing from JSON: %s", got)
		}
	}
	got = asJSON(medianame.Parse("Film.2024.mkv"))
	if strings.Contains(got, `"season"`) || strings.Contains(got, `"episode"`) {
		t.Fatalf("missing values invented: %s", got)
	}
}

func TestParseConcurrentAndIndependent(t *testing.T) {
	const name = "Show.S01E01E03.1080p.HDR.x265.mkv"
	want := medianame.Parse(name)
	for n := 0; n < 16; n++ {
		t.Run(string(rune('a'+n)), func(t *testing.T) {
			t.Parallel()
			got := medianame.Parse(name)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("nondeterministic result: %s", asJSON(got))
			}
			*got.Season = 99
			got.Episodes[0] = 99
			got.Effects[0] = "changed"
			if next := medianame.Parse(name); !reflect.DeepEqual(next, want) {
				t.Fatalf("result shares mutable state: %s", asJSON(next))
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	for n := 0; n < b.N; n++ {
		medianame.Parse("Example.Show.2024.S01E02.2160p.AMZN.WEB-DL.DDP5.1.H265.10bit-GROUP.mkv")
	}
}

func asJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func intValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
