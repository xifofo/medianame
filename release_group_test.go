package medianame_test

import (
	"testing"

	"github.com/xifofo/medianame"
)

func TestReleaseGroupSuffix(t *testing.T) {
	for _, tt := range []struct {
		name, group string
		episode     *int
		episodeEnd  *int
	}{
		{"新万圣节.2007.1080p.Remux.AVC.TrueHD.5.1-404.mkv", "404", nil, nil},
		{"Film.2024.2160p.WEB-DL.H265-1234.mkv", "1234", nil, nil},
		{"Film.2024.1080p.x264-CMCT.mkv", "CMCT", nil, nil},
		{"Film.2024.1080p.WEB-DL.H264-PTerWEB.mkv", "PTerWEB", nil, nil},
		{"Film.2024.1080p.x264-FFans@星星.mkv", "FFans@星星", nil, nil},
		{"Show.S01E01-03.mkv", "", ptr(1), ptr(3)},
		{"Show.S01E01-03.1080p.WEB-DL-HONE.mkv", "HONE", ptr(1), ptr(3)},
		{"Anime - 03 [1080p].mkv", "", ptr(3), nil},
		{"Film.2024.1080p.E-AC-3.mkv", "", nil, nil},
		{"Film.2024.1080p.10-bit.mkv", "", nil, nil},
		{"Spider-Man.mkv", "", nil, nil},
		{"Room-404.mkv", "", nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := medianame.Parse(tt.name)
			if got.ReleaseGroup != tt.group || intValue(got.Episode) != intValue(tt.episode) || intValue(got.EpisodeEnd) != intValue(tt.episodeEnd) {
				t.Fatalf("group=%q episode=%v end=%v; want %q / %v / %v: %s", got.ReleaseGroup, intValue(got.Episode), intValue(got.EpisodeEnd), tt.group, intValue(tt.episode), intValue(tt.episodeEnd), asJSON(got))
			}
		})
	}
}
