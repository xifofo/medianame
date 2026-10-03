package medianame

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type technicalToken struct {
	span
	field, value string
	strong       bool
}

type technicalRule struct {
	pattern *regexp.Regexp
	field   string
	strong  bool
}

var codecReplacer = strings.NewReplacer(" ", "", ".", "", "_", "", "-", "")

var technicalRules = []technicalRule{
	{regexp.MustCompile(`(?i)(?:[0-9]{3,4}x)(?:4320|2160|1440|1080|720|576|540|480|360)(?:p|i)?|(?:BD)?(?:4320|2160|1440|1080|720|576|540|480|360)(?:p|i)|\[(?:4320|2160|1440|1080)\]|[248]K`), "resolution", true},
	{regexp.MustCompile(`(?i)WEB[ ._-]?DL|WEB[ ._-]?Rip|Blu[ ._-]?Ray|BD[ ._-]?Rip|BR[ ._-]?Rip|DVD[ ._-]?Rip|DVD[ ._-]?Scr|HD[ ._-]?TV|UHD[ ._-]?TV|HD[ ._-]?Rip|HD[ ._-]?DVD|REMUX|CAM|TS|WEB|BD|DVD`), "source", false},
	{regexp.MustCompile(`(?i)[hx][ ._-]?26[45]|AVC|HEVC|AV1|VC[ ._-]?1|MPEG[ ._-]?[24]|XviD|DivX|AVS[23+]`), "video", true},
	{regexp.MustCompile(`(?i)(DTS[ ._-]?HD[ ._-]?MA|DTS[ ._-]?HD[ ._-]?HRA|DTS[ ._-]?HD|DTS[ ._-]?X|DTS|True[ ._-]?HD|E[ ._-]?AC[ ._-]?3|AC[ ._-]?3|DDP|DD\+|DD|AAC|FLAC|LPCM|PCM|Opus|Vorbis|MP3)(?:[ ._-]+(?:Dolby[ ._-]+)?Atmos)?([ ._-]*[1-8]\.[01])?`), "audio", true},
	{regexp.MustCompile(`(?i)HDR[ ._-]?Vivid|HDR10\+|HDR10Plus|HDR10P|HDR10|HDR|Dolby[ ._-]?Vision|DoVi|DV|HLG|SDR|Atmos|3D|IMAX|REPACK|PROPER`), "effect", false},
	{regexp.MustCompile(`(?i)(?:8|10|12|16)[ ._-]?bits?|Hi10P|yuv(?:420|422|444)p(?:8|10|12|16)(?:le|be)?`), "bit", true},
	{regexp.MustCompile(`(?i)[0-9]{2,3}(?:\.[0-9]{1,3})?[ ._-]?FPS`), "fps", true},
	{regexp.MustCompile(`(?i)AMZN|Amazon|Netflix|NF|DSNP|Disney\+|ATVP|AppleTV\+?|HMAX|HBO[ ._-]?Max|MAX|HULU|iTunes|iT|Bilibili|B[ ._-]?Global|CR|Crunchyroll`), "service", false},
}

func findTechnical(text string) []technicalToken {
	var tokens []technicalToken
	for _, rule := range technicalRules {
		for _, m := range boundedMatches(rule.pattern, text) {
			value := text[m[0]:m[1]]
			strong := rule.strong
			if rule.field == "source" {
				strong = strings.ContainsAny(value, "-._") || len(value) >= 5
			}
			tokens = append(tokens, technicalToken{span{m[0], m[1]}, rule.field, value, strong})
		}
	}
	sort.SliceStable(tokens, func(i, j int) bool { return tokens[i].start < tokens[j].start })
	return tokens
}

func applyTechnical(info *Info, token technicalToken) {
	value := strings.ToUpper(token.value)
	key := codecReplacer.Replace(value)
	switch token.field {
	case "resolution":
		if info.Resolution == "" {
			value = strings.Trim(value, "[]")
			if strings.HasPrefix(value, "BD") {
				value = strings.TrimPrefix(value, "BD")
				if info.Source == "" {
					info.Source = "BluRay"
				}
			}
			if x := strings.IndexByte(value, 'X'); x >= 0 {
				value = value[x+1:]
			}
			if allDigits(value) {
				value += "p"
			}
			info.Resolution = strings.ToLower(value)
			switch info.Resolution {
			case "4k":
				info.Resolution = "2160p"
			case "8k":
				info.Resolution = "4320p"
			case "2k":
				info.Resolution = "1080p"
			}
		}
	case "source":
		if info.Source == "" || key == "REMUX" {
			sources := map[string]string{"WEBDL": "WEB-DL", "WEBRIP": "WEBRip", "BLURAY": "BluRay",
				"BDRIP": "BDRip", "BRRIP": "BRRip", "DVDRIP": "DVDRip", "DVDSCR": "DVDScr",
				"HDRIP": "HDRip", "HDDVD": "HD-DVD"}
			info.Source = sources[key]
			if info.Source == "" {
				info.Source = key
			}
		}
	case "video":
		if info.VideoCodec == "" {
			switch key {
			case "H264", "X264", "AVC":
				info.VideoCodec = "H.264"
			case "H265", "X265", "HEVC":
				info.VideoCodec = "H.265"
			case "VC1":
				info.VideoCodec = "VC-1"
			default:
				info.VideoCodec = key
			}
		}
	case "audio":
		if info.AudioCodec == "" {
			m := technicalRules[3].pattern.FindStringSubmatch(token.value)
			codec := strings.ToUpper(codecReplacer.Replace(m[1]))
			// DDP、DD+、DD 保留发行别名，与 MP2 一致；声道独立提取，供重命名时用空格连接。
			canonical := map[string]string{"DTSHDMA": "DTS-HD MA", "DTSHDHRA": "DTS-HD HRA",
				"DTSHD": "DTS-HD", "DTSX": "DTS:X", "TRUEHD": "TrueHD",
				"EAC3": "E-AC-3", "AC3": "AC-3",
				"OPUS": "Opus", "VORBIS": "Vorbis"}
			info.AudioCodec = canonical[codec]
			if info.AudioCodec == "" {
				info.AudioCodec = codec
			}
			info.AudioChannels = strings.TrimLeft(m[2], " ._-")
		}
	case "effect":
		canonical := map[string]string{"HDR10PLUS": "HDR10+", "HDR10P": "HDR10+", "DOLBYVISION": "Dolby Vision",
			"DOVI": "Dolby Vision", "DV": "Dolby Vision", "HDRVIVID": "HDR Vivid", "ATMOS": "Atmos"}
		effect := canonical[key]
		if effect == "" {
			effect = key
		}
		for _, existing := range info.Effects {
			if existing == effect {
				return
			}
		}
		info.Effects = append(info.Effects, effect)
	case "bit":
		if info.BitDepth == 0 {
			if key == "HI10P" {
				info.BitDepth = 10
			} else if strings.HasPrefix(key, "YUV") {
				value := strings.TrimRight(strings.SplitN(key, "P", 2)[1], "LEB")
				info.BitDepth, _ = strconv.Atoi(value)
			} else {
				info.BitDepth, _ = strconv.Atoi(strings.TrimRight(key, "BITS"))
			}
		}
	case "fps":
		if info.FPS == 0 {
			info.FPS, _ = strconv.ParseFloat(strings.TrimRight(value, " FPS._-"), 64)
		}
	case "service":
		if info.StreamingService == "" {
			services := map[string]string{"AMZN": "Amazon", "AMAZON": "Amazon", "NF": "Netflix", "NETFLIX": "Netflix",
				"DSNP": "Disney+", "DISNEY+": "Disney+", "ATVP": "Apple TV+", "APPLETV": "Apple TV+", "APPLETV+": "Apple TV+",
				"HMAX": "Max", "HBOMAX": "Max", "MAX": "Max", "HULU": "Hulu", "IT": "iTunes", "ITUNES": "iTunes",
				"BILIBILI": "Bilibili", "BGLOBAL": "B-Global", "CR": "Crunchyroll", "CRUNCHYROLL": "Crunchyroll"}
			info.StreamingService = services[key]
		}
	}
}
