package tmdb

import (
	"encoding/json"
	"strconv"
	"strings"

	"medianame"
)

// MediaInfo 是 TMDB 的影视详情。电影和剧集统一身份字段，数据库特有字段保留在 Raw。
// Type 使用 movie/tv，Year 为电影上映年或剧集首播年，IDs 保持字符串。
type MediaInfo struct {
	Source              string              `json:"source"`
	MediaID             string              `json:"media_id"`
	TMDBID              string              `json:"tmdb_id"`
	IMDbID              string              `json:"imdb_id,omitempty"`
	TVDBID              string              `json:"tvdb_id,omitempty"`
	Type                medianame.MediaType `json:"type"`
	Title               string              `json:"title"`
	EnglishTitle        string              `json:"en_title,omitempty"`
	OriginalTitle       string              `json:"original_title"`
	Category            string              `json:"category,omitempty"`
	Name                string              `json:"name,omitempty"`          // 剧集的 TMDB 原始 name 字段。
	OriginalName        string              `json:"original_name,omitempty"` // 剧集的 TMDB 原始 original_name 字段。
	Aliases             []string            `json:"aliases,omitempty"`
	AllTitles           []string            `json:"all_titles"`
	AlternativeTitles   []AlternativeTitle  `json:"alternative_titles,omitempty"`
	Translations        []Translation       `json:"translations,omitempty"`
	ExternalIDs         *ExternalIDs        `json:"external_ids,omitempty"`
	Year                int                 `json:"year,omitempty"`
	ReleaseDate         string              `json:"release_date,omitempty"`
	FirstAirDate        string              `json:"first_air_date,omitempty"`
	LastAirDate         string              `json:"last_air_date,omitempty"`
	Overview            string              `json:"overview"`
	Tagline             string              `json:"tagline,omitempty"`
	PosterPath          string              `json:"poster_path,omitempty"`
	PosterURL           string              `json:"poster_url,omitempty"`
	BackdropPath        string              `json:"backdrop_path,omitempty"`
	BackdropURL         string              `json:"backdrop_url,omitempty"`
	TMDBURL             string              `json:"tmdb_url"`
	Genres              []Genre             `json:"genres,omitempty"`
	GenreIDs            []int               `json:"genre_ids,omitempty"`
	OriginalLanguage    string              `json:"original_language,omitempty"`
	SpokenLanguages     []Language          `json:"spoken_languages,omitempty"`
	OriginCountry       []string            `json:"origin_country,omitempty"`
	ProductionCountries []Country           `json:"production_countries,omitempty"`
	ProductionCompanies []Company           `json:"production_companies,omitempty"`
	Networks            []Company           `json:"networks,omitempty"`
	VoteAverage         float64             `json:"vote_average"`
	VoteCount           int                 `json:"vote_count"`
	Popularity          float64             `json:"popularity"`
	Runtime             int                 `json:"runtime,omitempty"`
	Budget              int64               `json:"budget,omitempty"`
	Revenue             int64               `json:"revenue,omitempty"`
	Video               *bool               `json:"video,omitempty"`
	EpisodeRunTime      []int               `json:"episode_run_time,omitempty"`
	Status              string              `json:"status,omitempty"`
	Homepage            string              `json:"homepage,omitempty"`
	Adult               bool                `json:"adult"`
	NumberOfSeasons     int                 `json:"number_of_seasons,omitempty"`
	NumberOfEpisodes    int                 `json:"number_of_episodes,omitempty"`
	InProduction        *bool               `json:"in_production,omitempty"`
	Seasons             []Season            `json:"seasons,omitempty"`
	SeriesType          string              `json:"series_type,omitempty"` // TMDB 的 Scripted、Miniseries 等剧集类型。
	Languages           []string            `json:"languages,omitempty"`
	CreatedBy           []Creator           `json:"created_by,omitempty"`
	LastEpisodeToAir    *Episode            `json:"last_episode_to_air,omitempty"`
	NextEpisodeToAir    *Episode            `json:"next_episode_to_air,omitempty"`
	Credits             *Credits            `json:"credits,omitempty"`
	Collection          *Collection         `json:"belongs_to_collection,omitempty"`
	// Raw 是详情接口（含追加子响应）的完整 JSON，不包含请求凭证。
	Raw json.RawMessage `json:"raw,omitempty"`
}

// AlternativeTitle 保留 TMDB 的每条别名及其国家和名称类型；不删除同名但地区不同的记录。
type AlternativeTitle struct {
	Country string `json:"iso_3166_1"`
	Title   string `json:"title"`
	Type    string `json:"type"`
}

// Translation 保留每个语言、国家组合的译名和译文，不只提取用于名称匹配的 title/name。
type Translation struct {
	Country     string          `json:"iso_3166_1"`
	Language    string          `json:"iso_639_1"`
	Name        string          `json:"name"`
	EnglishName string          `json:"english_name"`
	Data        TranslationData `json:"data"`
}

type TranslationData struct {
	Title    string `json:"title,omitempty"` // 电影译名。
	Name     string `json:"name,omitempty"`  // 剧集译名。
	Overview string `json:"overview"`
	Tagline  string `json:"tagline,omitempty"`
	Homepage string `json:"homepage"`
	Runtime  *int   `json:"runtime,omitempty"`
}

// ExternalIDs 提供 TMDB 的外部影视、知识库和社交账号 ID。数字 ID 也以字符串返回。
// Raw 保留原接口的数字/空值表达以及未来可能新增的来源。
type ExternalIDs struct {
	IMDbID      string          `json:"imdb_id,omitempty"`
	TVDBID      string          `json:"tvdb_id,omitempty"`
	TVRageID    string          `json:"tvrage_id,omitempty"`
	WikidataID  string          `json:"wikidata_id,omitempty"`
	FreebaseID  string          `json:"freebase_id,omitempty"`
	FreebaseMID string          `json:"freebase_mid,omitempty"`
	FacebookID  string          `json:"facebook_id,omitempty"`
	InstagramID string          `json:"instagram_id,omitempty"`
	TwitterID   string          `json:"twitter_id,omitempty"`
	TikTokID    string          `json:"tiktok_id,omitempty"`
	YouTubeID   string          `json:"youtube_id,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

func (ids *ExternalIDs) UnmarshalJSON(data []byte) error {
	type fields ExternalIDs
	var wire struct {
		fields
		TVDBID   json.Number `json:"tvdb_id"`
		TVRageID json.Number `json:"tvrage_id"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*ids = ExternalIDs(wire.fields)
	ids.TVDBID, ids.TVRageID = wire.TVDBID.String(), wire.TVRageID.String()
	ids.Raw = append(json.RawMessage(nil), data...)
	return nil
}

type Creator struct {
	ID           int    `json:"id"`
	CreditID     string `json:"credit_id,omitempty"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name,omitempty"`
	Gender       int    `json:"gender"`
	ProfilePath  string `json:"profile_path,omitempty"`
}

// Episode 是剧集详情接口附带的上一集或下一集摘要；不会主动抓取所有集的详情。
type Episode struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Overview       string  `json:"overview"`
	AirDate        string  `json:"air_date,omitempty"`
	EpisodeNumber  int     `json:"episode_number"`
	SeasonNumber   int     `json:"season_number"`
	EpisodeType    string  `json:"episode_type,omitempty"`
	ShowID         int     `json:"show_id"`
	ProductionCode string  `json:"production_code,omitempty"`
	Runtime        *int    `json:"runtime,omitempty"`
	StillPath      string  `json:"still_path,omitempty"`
	VoteAverage    float64 `json:"vote_average"`
	VoteCount      int     `json:"vote_count"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Language struct {
	Code        string `json:"iso_639_1"`
	Name        string `json:"name"`
	EnglishName string `json:"english_name"`
}

type Country struct {
	Code string `json:"iso_3166_1"`
	Name string `json:"name"`
}

type Company struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path,omitempty"`
	OriginCountry string `json:"origin_country,omitempty"`
}

type Season struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	SeasonNumber int     `json:"season_number"`
	EpisodeCount int     `json:"episode_count"`
	AirDate      string  `json:"air_date,omitempty"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
}

type Collection struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path,omitempty"`
	BackdropPath string `json:"backdrop_path,omitempty"`
}

type Credits struct {
	Cast []Person `json:"cast"`
	Crew []Person `json:"crew"`
}

type Person struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	OriginalName       string  `json:"original_name,omitempty"`
	Character          string  `json:"character,omitempty"`
	Job                string  `json:"job,omitempty"`
	Department         string  `json:"department,omitempty"`
	KnownForDepartment string  `json:"known_for_department,omitempty"`
	ProfilePath        string  `json:"profile_path,omitempty"`
	Gender             int     `json:"gender"`
	Adult              bool    `json:"adult"`
	Popularity         float64 `json:"popularity"`
	CreditID           string  `json:"credit_id,omitempty"`
	CastID             *int    `json:"cast_id,omitempty"`
	Order              int     `json:"order"`
}

// Candidate 将在线详情转换为 medianame 的身份校验输入，包含 TMDB 官方别名和译名。
func (m MediaInfo) Candidate() medianame.MediaCandidate {
	return medianame.MediaCandidate{Title: m.Title, OriginalTitle: m.OriginalTitle, Aliases: m.Aliases,
		Year: m.Year, Type: m.Type, IDs: medianame.MediaIDs{TMDB: m.TMDBID, IMDb: m.IMDbID, TVDB: m.TVDBID}}
}

// ImageURL 用 TMDB 默认图片服务生成 original 尺寸 URL；没有图片时返回空字符串。
// 如需其他图片服务或尺寸，调用方可使用保留的 PosterPath、BackdropPath。
func ImageURL(path string) string {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return ""
	}
	return "https://image.tmdb.org/t/p/original" + path
}

// wireMedia 兼容电影 title/original_title 和剧集 name/original_name。
type wireMedia struct {
	MediaInfo
	ID                json.Number `json:"id"`
	SeriesType        string      `json:"type"`
	AlternativeTitles struct {
		Titles  []AlternativeTitle `json:"titles"`
		Results []AlternativeTitle `json:"results"`
	} `json:"alternative_titles"`
	Translations struct {
		Translations []Translation `json:"translations"`
	} `json:"translations"`
}

func decodeMedia(data []byte, mediaType medianame.MediaType) (*MediaInfo, error) {
	var wire wireMedia
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, errInvalidJSON
	}
	id, err := canonicalID(wire.ID.String())
	if err != nil {
		return nil, errInvalidMedia
	}
	m := wire.MediaInfo
	m.Type, m.Source, m.MediaID, m.TMDBID = mediaType, "themoviedb", id, id
	date := m.ReleaseDate
	if mediaType == medianame.TypeTV {
		m.Title, m.OriginalTitle, date = m.Name, m.OriginalName, m.FirstAirDate
		m.SeriesType = wire.SeriesType
	}
	if m.Title == "" && m.OriginalTitle == "" {
		return nil, errInvalidMedia
	}
	if len(date) >= 4 {
		m.Year, _ = strconv.Atoi(date[:4])
	}
	if m.ExternalIDs != nil {
		if m.ExternalIDs.IMDbID != "" {
			m.IMDbID = m.ExternalIDs.IMDbID
		}
		m.TVDBID = m.ExternalIDs.TVDBID
	}
	m.AlternativeTitles = append(wire.AlternativeTitles.Titles, wire.AlternativeTitles.Results...)
	m.Translations = wire.Translations.Translations
	seen := map[string]bool{m.Title: true, m.OriginalTitle: true}
	addAlias := func(title string) {
		title = strings.TrimSpace(title)
		if title != "" && !seen[title] {
			seen[title] = true
			m.Aliases = append(m.Aliases, title)
		}
	}
	for _, title := range m.AlternativeTitles {
		addAlias(title.Title)
	}
	for _, translation := range m.Translations {
		addAlias(translation.Data.Title)
		addAlias(translation.Data.Name)
		if translation.Language == "en" {
			title := translation.Data.Title
			if mediaType == medianame.TypeTV {
				title = translation.Data.Name
			}
			if title != "" && (m.EnglishTitle == "" || translation.Country == "US") {
				m.EnglishTitle = title
			}
		}
	}
	if m.EnglishTitle == "" {
		m.EnglishTitle = m.OriginalTitle
	}
	allSeen := map[string]bool{}
	for _, title := range append([]string{m.Title, m.OriginalTitle}, m.Aliases...) {
		if title != "" && !allSeen[title] {
			allSeen[title] = true
			m.AllTitles = append(m.AllTitles, title)
		}
	}
	if len(m.Genres) > 0 {
		m.GenreIDs = nil
		for _, genre := range m.Genres {
			m.GenreIDs = append(m.GenreIDs, genre.ID)
		}
	}
	m.PosterURL, m.BackdropURL = ImageURL(m.PosterPath), ImageURL(m.BackdropPath)
	m.TMDBURL = "https://www.themoviedb.org/" + string(mediaType) + "/" + id
	m.Raw = append(json.RawMessage(nil), data...)
	return &m, nil
}
