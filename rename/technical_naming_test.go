package rename_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/rename"
)

func TestTechnicalNamingPreservesDVAndHDR(t *testing.T) {
	const input = "Kingdom.of.Heaven.2005.2160p.Directors.Cut.Roadshow.Version.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.Atmos.7.1-HDHIVE.mkv"
	info := medianame.Parse(input)
	media := rename.Media{Title: "天国王朝", EnglishTitle: "Kingdom of Heaven", Year: 2005,
		Type: medianame.TypeMovie, IDs: medianame.MediaIDs{TMDB: "1495"}}
	for _, tv := range []bool{false, true} {
		format := rename.DefaultMovieTemplate
		prefix := "天国王朝 (2005) {tmdb-1495}/Kingdom of Heaven.2005"
		if tv {
			info = medianame.Parse(strings.Replace(input, ".2005.", ".2005.S01E02.", 1))
			media.Type = medianame.TypeTV
			format = rename.DefaultTVTemplate
			prefix = "天国王朝 (2005) {tmdb-1495}/Season 01/Kingdom of Heaven.2005.S01E02"
		}
		ctx := rename.BuildContext(info, media)
		want := prefix + ".REMUX.DV.HDR.Atmos.2160p.H.265.TrueHD.7.1-HDHIVE.mkv"
		result, err := rename.Render(format, ctx)
		if err != nil || result == nil || result.Path != want {
			t.Fatalf("tv=%t: result=%+v err=%v; want %q", tv, result, err, want)
		}
		if !reflect.DeepEqual(info.Effects, []string{"Dolby Vision", "HDR", "Atmos"}) || info.AudioCodec != "TrueHD" || info.AudioChannels != "7.1" {
			t.Fatalf("naming changed parsed metadata: %+v", info)
		}
	}
}

func TestTechnicalNamingKeepsOnlyRecognizedTags(t *testing.T) {
	for _, tt := range []struct {
		input, effect, edition, audio, resource string
	}{
		{"Film.2024.2160p.REMUX.DV.HDR10+.TrueHD.7.1.Atmos.mkv", "DV.HDR10+.Atmos", "REMUX.DV.HDR10+.Atmos", "TrueHD.7.1", "REMUX.DV.HDR10+.Atmos.2160p"},
		{"Film.2024.2160p.DV.HDR10.mkv", "DV.HDR10", "DV.HDR10", "", "DV.HDR10.2160p"},
		{"Film.2024.DV.mkv", "DV", "DV", "", "DV"},
		{"Film.2024.HDR.mkv", "HDR", "HDR", "", "HDR"},
		{"Film.2024.Atmos.mkv", "Atmos", "Atmos", "", "Atmos"},
		{"Film.2024.WEB-DL.DDP2.0.mkv", "", "WEB-DL", "DDP.2.0", "WEB-DL"},
		{"Film.2024.DTS-HD.MA.5.1.mkv", "", "", "DTS-HD.MA.5.1", ""},
		{"Film.2024.HDRVivid.mkv", "HDR.Vivid", "HDR.Vivid", "", "HDR.Vivid"},
		{"Film.2024.1080p.mkv", "", "", "", "1080p"},
		{"Film.2024.mkv", "", "", "", ""},
	} {
		t.Run(tt.input, func(t *testing.T) {
			info := medianame.Parse(tt.input)
			effects := append([]string(nil), info.Effects...)
			ctx := rename.BuildContext(info, rename.Media{})
			for key, want := range map[string]string{"effect": tt.effect, "edition": tt.edition, "audioCodec": tt.audio, "resource_term": tt.resource} {
				if got := ctx[key]; got != want {
					t.Errorf("%s=%q; want %q", key, got, want)
				}
			}
			if !reflect.DeepEqual(info.Effects, effects) {
				t.Fatalf("naming changed parsed effects: %v -> %v", effects, info.Effects)
			}
		})
	}
}
