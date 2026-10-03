package shadow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"medianame"
)

// Difference 保存统一格式之后仍然存在的字段差异。
type Difference struct {
	Field string `json:"field"`
	Local any    `json:"local"`
	MP2   any    `json:"mp2"`
}

// Record 保留每个样本的两侧原始结果、规范化字段及差异。
type Record struct {
	Input
	Status          string         `json:"status"`
	Local           medianame.Info `json:"local"`
	MP2             map[string]any `json:"mp2,omitempty"`
	LocalFields     map[string]any `json:"local_fields"`
	MP2Fields       map[string]any `json:"mp2_fields,omitempty"`
	Differences     []Difference   `json:"differences,omitempty"`
	Error           string         `json:"error,omitempty"`
	Recognition     *Recognition   `json:"recognition,omitempty"`
	PathRecognition *Recognition   `json:"path_recognition,omitempty"`
	LocalPath       medianame.Info `json:"local_path"`
}

// FieldOrder 固定报告中的字段顺序。
var FieldOrder = []string{"title", "type", "year", "season", "season_end", "episode", "episode_end", "episodes",
	"resolution", "source", "video_codec", "audio_codec", "audio_channels", "effects", "bit_depth", "fps", "release_group",
	"streaming_service", "part", "episode_group", "tmdb_id", "douban_id", "bangumi_id", "anilist_id"}

var (
	spaces          = regexp.MustCompile(`\s+`)
	channels        = regexp.MustCompile(`[1-8]\.[01]`)
	depthPattern    = regexp.MustCompile(`(?i)(8|10|12|16)\s*bit`)
	audioPattern    = regexp.MustCompile(`(?i)DTS[ ._-]?HD[ ._-]?MA|DTS[ ._-]?HD[ ._-]?HRA|DTS[ ._-]?HD|DTS[: ._-]?X|DTS|TRUE[ ._-]?HD|E[ ._-]?AC[ ._-]?3|AC[ ._-]?3|DDP|DD\+|DD|AAC|FLAC|LPCM|PCM|OPUS|VORBIS|MP3`)
	effectPattern   = regexp.MustCompile(`(?i)HDR[ ._-]?Vivid|HDR10\+|HDR10Plus|HDR10P|HDR10|HDR|Dolby[ ._-]?Vision|DoVi|\bDV\b|HLG|SDR|Atmos|3D|IMAX|REPACK|PROPER|EDR|HQ`)
	partNumber      = regexp.MustCompile(`[0-9]+`)
	compactReplacer = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "")
)

// Compare 对相同文件名进行对比；不以 MP2 的在线媒体识别标题覆盖名称解析字段。
func Compare(input Input, remote map[string]any, remoteErr error) Record {
	local := medianame.Parse(input.Name)
	record := Record{Input: input, Local: local, LocalPath: medianame.ParsePath(input.Path), LocalFields: localFields(local), MP2: remote}
	if remoteErr != nil {
		record.Status, record.Error = "mp2_error", remoteErr.Error()
		return record
	}
	record.MP2Fields = remoteFields(remote)
	record.Status = "matched"
	for _, field := range FieldOrder {
		left, right := record.LocalFields[field], record.MP2Fields[field]
		if !reflect.DeepEqual(left, right) {
			record.Status = "different"
			record.Differences = append(record.Differences, Difference{field, left, right})
		}
	}
	return record
}

func localFields(info medianame.Info) map[string]any {
	return map[string]any{
		"title": title(info.Title), "type": string(info.Type), "year": nonzero(info.Year),
		"season": pointer(info.Season), "season_end": pointer(info.SeasonEnd),
		"episode": pointer(info.Episode), "episode_end": pointer(info.EpisodeEnd),
		"episodes":   episodeRanges(info.Episodes, info.Episode, info.EpisodeEnd, info.Season, info.SeasonEnd),
		"resolution": resolution(info.Resolution), "source": source(info.Source), "video_codec": video(info.VideoCodec),
		"audio_codec": audio(info.AudioCodec), "audio_channels": optional(info.AudioChannels), "effects": effects(strings.Join(info.Effects, " ")),
		"bit_depth": nonzero(info.BitDepth), "fps": nonzeroFloat(info.FPS), "release_group": lowerOptional(info.ReleaseGroup),
		"streaming_service": service(info.StreamingService), "part": nonzero(info.Part), "episode_group": lowerOptional(info.EpisodeGroup),
		"tmdb_id": optional(info.IDs.TMDB), "douban_id": optional(info.IDs.Douban),
		"bangumi_id": optional(info.IDs.Bangumi), "anilist_id": optional(info.IDs.AniList),
	}
}

func remoteFields(meta map[string]any) map[string]any {
	name := text(meta["name"])
	if name == "" {
		name = text(meta["cn_name"])
		if name == "" {
			name = text(meta["en_name"])
		}
	}
	season, seasonEnd := numericPointer(meta["begin_season"]), numericPointer(meta["end_season"])
	episode, episodeEnd := numericPointer(meta["begin_episode"]), numericPointer(meta["end_episode"])
	var episodeList []int
	if values, ok := meta["episode_list"].([]any); ok {
		for _, value := range values {
			if n := numericPointer(value); n != nil {
				episodeList = append(episodeList, *n)
			}
		}
	}
	audioValue := text(meta["audio_encode"])
	bit := 0
	if m := depthPattern.FindStringSubmatch(text(meta["video_bit"]) + " " + text(meta["video_encode"])); m != nil {
		bit, _ = strconv.Atoi(m[1])
	}
	mediaType := "unknown"
	switch strings.ToLower(text(meta["type"])) {
	case "电影", "movie", "movies":
		mediaType = "movie"
	case "电视剧", "tv", "series", "tvshow", "show":
		mediaType = "tv"
	}
	part := nonzero(numeric(text(meta["part"])))
	if value := text(meta["part"]); value != "" {
		if digits := partNumber.FindString(value); digits != "" {
			part = nonzero(numeric(digits))
		} else {
			part = strings.ToLower(value)
		}
	}
	return map[string]any{
		"title": title(name), "type": mediaType, "year": nonzero(numeric(meta["year"])),
		"season": pointer(season), "season_end": pointer(seasonEnd), "episode": pointer(episode), "episode_end": pointer(episodeEnd),
		"episodes":   episodeRanges(episodeList, episode, episodeEnd, season, seasonEnd),
		"resolution": resolution(text(meta["resource_pix"])), "source": source(text(meta["resource_type"])),
		"video_codec": video(text(meta["video_encode"])), "audio_codec": audio(audioValue),
		"audio_channels": optional(channels.FindString(audioValue)),
		"effects":        effects(text(meta["resource_effect"]) + " " + audioValue),
		"bit_depth":      nonzero(bit), "fps": nonzeroFloat(decimal(meta["fps"])), "release_group": lowerOptional(text(meta["resource_team"])),
		"streaming_service": service(text(meta["web_source"])), "part": part, "episode_group": lowerOptional(text(meta["episode_group"])),
		"tmdb_id": optional(text(meta["tmdbid"])), "douban_id": optional(text(meta["doubanid"])),
		"bangumi_id": optional(text(meta["bangumiid"])), "anilist_id": optional(text(meta["anilistid"])),
	}
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func optional(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func lowerOptional(value string) any { return optional(strings.ToLower(value)) }
func nonzero(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
func nonzeroFloat(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}
func pointer(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
func numeric(value any) int     { n, _ := strconv.Atoi(text(value)); return n }
func decimal(value any) float64 { n, _ := strconv.ParseFloat(text(value), 64); return n }
func numericPointer(value any) *int {
	if value == nil || text(value) == "" {
		return nil
	}
	n, err := strconv.Atoi(text(value))
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

func title(value string) any {
	value = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(value)
	return lowerOptional(spaces.ReplaceAllString(strings.TrimSpace(value), " "))
}
func resolution(value string) any {
	value = strings.ToLower(value)
	switch value {
	case "4k":
		value = "2160p"
	case "8k":
		value = "4320p"
	case "2k":
		value = "1080p"
	}
	return optional(value)
}
func source(value string) any { return optional(strings.ToUpper(compactReplacer.Replace(value))) }
func video(value string) any {
	value = compactReplacer.Replace(strings.ToUpper(depthPattern.ReplaceAllString(value, "")))
	switch value {
	case "X264", "H264", "AVC":
		value = "H.264"
	case "X265", "H265", "HEVC":
		value = "H.265"
	}
	return optional(value)
}
func audio(value string) any {
	value = compactReplacer.Replace(strings.ToUpper(audioPattern.FindString(value)))
	switch value {
	case "DDP", "DD+", "EAC3":
		value = "E-AC-3"
	case "DD", "AC3":
		value = "AC-3"
	case "DTSHDMA":
		value = "DTS-HD MA"
	case "DTSHDHRA":
		value = "DTS-HD HRA"
	case "DTSHD":
		value = "DTS-HD"
	case "DTSX", "DTS:X":
		value = "DTS:X"
	case "TRUEHD":
		value = "TrueHD"
	}
	return optional(value)
}
func service(value string) any {
	value = compactReplacer.Replace(strings.ToUpper(value))
	switch value {
	case "AMZN", "AMAZON", "AMAZONPRIME":
		value = "Amazon"
	case "NF", "NETFLIX":
		value = "Netflix"
	case "DSNP", "DISNEY+", "DISNEYPLUS":
		value = "Disney+"
	case "ATVP", "APPLETV", "APPLETV+":
		value = "Apple TV+"
	case "HMAX", "HBOMAX", "MAX":
		value = "Max"
	case "IT", "ITUNES":
		value = "iTunes"
	case "CR", "CRUNCHYROLL":
		value = "Crunchyroll"
	}
	return lowerOptional(value)
}
func effects(value string) any {
	var result []string
	seen := map[string]bool{}
	for _, effect := range effectPattern.FindAllString(value, -1) {
		effect = compactReplacer.Replace(strings.ToUpper(effect))
		switch effect {
		case "DV", "DOVI", "DOLBYVISION":
			effect = "Dolby Vision"
		case "HDR10PLUS", "HDR10P":
			effect = "HDR10+"
		case "HDRVIVID":
			effect = "HDR Vivid"
		case "ATMOS":
			effect = "Atmos"
		}
		if !seen[effect] {
			result = append(result, effect)
			seen[effect] = true
		}
	}
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

func episodeRanges(list []int, start, end, season, seasonEnd *int) any {
	if season != nil && seasonEnd != nil && *season != *seasonEnd {
		return nil
	}
	if len(list) == 0 {
		if start == nil {
			return nil
		}
		last := *start
		if end != nil {
			last = *end
		}
		return [][2]int{{*start, last}}
	}
	list = append([]int(nil), list...)
	sort.Ints(list)
	result := [][2]int{{list[0], list[0]}}
	for _, value := range list[1:] {
		last := &result[len(result)-1]
		if value <= last[1]+1 {
			if value > last[1] {
				last[1] = value
			}
		} else {
			result = append(result, [2]int{value, value})
		}
	}
	return result
}

// DecodeReport 保留原始 JSON 数字精度，供离线重放使用。
func DecodeReport(data []byte, report *Report) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	return decoder.Decode(report)
}
