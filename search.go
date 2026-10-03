package medianame

import (
	"strconv"
	"strings"
	"unicode"
)

// SearchQuery 是从本地解析结果生成的数据库查询条件。
// Text 可传给接受发布名称的识别服务；其余字段用于直接查询媒体数据库及结果校验。
type SearchQuery struct {
	Title string    `json:"title"`
	Year  int       `json:"year,omitempty"`
	Type  MediaType `json:"type"`
	IDs   MediaIDs  `json:"ids"`
	Text  string    `json:"text"`
}

// SearchQueries 生成不包含发布组、编码、蓝光技术目录的名称查询。
// 双语名称分别查询；不删除年份、版本括号或显式 ID 来强行获得候选。
// 本方法不进行网络访问，实际数据库查询由调用方执行。
func (info Info) SearchQueries() []SearchQuery {
	var result []SearchQuery
	seen := map[string]bool{}
	titles := append(strings.Split(info.Title, " / "), info.Aliases...)
	for _, title := range titles {
		title = strings.TrimSpace(title)
		if title == "" || seen[strings.ToLower(title)] {
			continue
		}
		seen[strings.ToLower(title)] = true
		query := SearchQuery{Title: title, Year: info.Year, Type: info.Type, IDs: info.IDs, Text: title}
		if info.Year != 0 {
			query.Text += "." + strconv.Itoa(info.Year)
		}
		// 查询携带已判断的类型，避免电影和剧集之间的同名候选混淆。
		var tags []string
		if info.Type == TypeTV || info.Type == TypeMovie {
			tags = append(tags, "type="+string(info.Type))
		}
		for _, id := range identityFields(info.IDs) {
			if id.value != "" {
				tags = append(tags, id.name+"id="+id.value)
			}
		}
		if len(tags) > 0 {
			query.Text += " {[" + strings.Join(tags, ";") + "]}"
		}
		result = append(result, query)
	}
	return result
}

// MediaCandidate 是在线服务返回的身份字段；返回候选不等于确认媒体身份。
type MediaCandidate struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	// Aliases 可由在线数据库的别名或译名接口提供；调用方负责这些名称的来源。
	Aliases []string  `json:"aliases,omitempty"`
	Year    int       `json:"year,omitempty"`
	Type    MediaType `json:"type"`
	IDs     MediaIDs  `json:"ids"`
}

// CandidateConflict 保留候选与输入的冲突或缺失证据。
type CandidateConflict struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// CheckCandidate 检查输入与候选身份的一致性。空结果表示没有发现冲突，不能代替人工确认。
// 别名无法在离线状态下确认时也要求复核；不会自动接受年份或类型冲突的候选。
func (info Info) CheckCandidate(candidate MediaCandidate) []CandidateConflict {
	var conflicts []CandidateConflict
	add := func(field, expected, actual string) {
		conflicts = append(conflicts, CandidateConflict{field, expected, actual})
	}
	for _, warning := range info.Warnings {
		add("input", "consistent_identity", warning)
	}
	if info.Year != 0 && info.Year != candidate.Year {
		add("year", strconv.Itoa(info.Year), strconv.Itoa(candidate.Year))
	}
	if info.Type != "" && info.Type != TypeUnknown && info.Type != candidate.Type {
		add("type", string(info.Type), string(candidate.Type))
	}
	wantIDs, gotIDs := identityFields(info.IDs), identityFields(candidate.IDs)
	idMatched := false
	for i, id := range wantIDs {
		if id.value == "" {
			continue
		}
		if id.value != gotIDs[i].value {
			add(id.name+"_id", id.value, gotIDs[i].value)
		} else {
			idMatched = true
		}
	}
	// TMDB ID 在电影和剧集间不唯一；只有类型也明确时才可凭 ID 接受译名差异。
	if !(idMatched && info.Type != "" && info.Type != TypeUnknown && info.Type == candidate.Type) {
		matched := false
		for _, query := range info.SearchQueries() {
			for _, title := range append([]string{candidate.Title, candidate.OriginalTitle}, candidate.Aliases...) {
				if identityTitle(title) != "" && identityTitle(query.Title) == identityTitle(title) {
					matched = true
				}
			}
		}
		if !matched {
			add("title", info.Title, candidate.Title)
		}
	}
	return conflicts
}

func identityTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, title)
}

type identityField struct{ name, value string }

func identityFields(ids MediaIDs) []identityField {
	return []identityField{{"tmdb", ids.TMDB}, {"tvdb", ids.TVDB}, {"imdb", ids.IMDb}, {"douban", ids.Douban}, {"bangumi", ids.Bangumi}, {"anilist", ids.AniList}}
}
