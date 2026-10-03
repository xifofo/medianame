package medianame

// MediaType 表示根据显式标签及名称、路径线索判断的媒体类型。
type MediaType string

const (
	// TypeUnknown 表示既没有片名，也没有季集或显式类型线索。
	TypeUnknown MediaType = "unknown"
	// TypeMovie 表示显式标记为电影，或有片名且没有剧集线索的名称。
	TypeMovie MediaType = "movie"
	// TypeTV 表示具有季集线索或显式剧集标记的名称。
	TypeTV MediaType = "tv"
)

// Info 保存名称中的解析结果。空字符串、0 和 nil 通常表示未识别到对应字段。
// 剧集未提供季号时 Season 默认为 1；季集使用指针，显式第 0 季、第 0 集仍会保留。
type Info struct {
	Original         string    `json:"original"`
	Title            string    `json:"title"`
	Aliases          []string  `json:"aliases,omitempty"`
	Type             MediaType `json:"type"`
	Year             int       `json:"year,omitempty"`
	Season           *int      `json:"season,omitempty"`
	SeasonEnd        *int      `json:"season_end,omitempty"`
	Episode          *int      `json:"episode,omitempty"`
	EpisodeEnd       *int      `json:"episode_end,omitempty"`
	Episodes         []int     `json:"episodes,omitempty"`
	EpisodeVersion   int       `json:"episode_version,omitempty"`
	EpisodeGroup     string    `json:"episode_group,omitempty"`
	Resolution       string    `json:"resolution,omitempty"`
	Source           string    `json:"source,omitempty"`
	VideoCodec       string    `json:"video_codec,omitempty"`
	AudioCodec       string    `json:"audio_codec,omitempty"`
	AudioChannels    string    `json:"audio_channels,omitempty"`
	Effects          []string  `json:"effects,omitempty"`
	BitDepth         int       `json:"bit_depth,omitempty"`
	FPS              float64   `json:"fps,omitempty"`
	ReleaseGroup     string    `json:"release_group,omitempty"`
	StreamingService string    `json:"streaming_service,omitempty"`
	Part             int       `json:"part,omitempty"`
	Extension        string    `json:"extension,omitempty"`
	IDs              MediaIDs  `json:"ids"`
	// Warnings 保留互相矛盾的输入线索，调用方应复核对应媒体候选。
	Warnings []string `json:"warnings,omitempty"`

	// 显式类型不受后续路径中的季集推断覆盖；不属于输出字段。
	typeExplicit bool
}

// defaultSeason 在文件与目录线索全部合并后补默认值，避免覆盖目录中的实际季号。
func defaultSeason(info *Info) {
	if info.Type == TypeTV && info.Season == nil {
		info.Season = integer(1)
	}
}

// MediaIDs 保存显式标记的媒体数据库 ID，不通过片名进行在线查询。
// ID 使用字符串，避免长数字在下游 JSON 环境中丢失精度。
type MediaIDs struct {
	TMDB    string `json:"tmdb,omitempty"`
	TVDB    string `json:"tvdb,omitempty"`
	IMDb    string `json:"imdb,omitempty"`
	Douban  string `json:"douban,omitempty"`
	Bangumi string `json:"bangumi,omitempty"`
	AniList string `json:"anilist,omitempty"`
}
