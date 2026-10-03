package shadow

import (
	"context"
	_ "embed"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xifofo/medianame"
)

type EnhancementAttempt struct {
	Query     medianame.SearchQuery         `json:"query"`
	Result    *Recognition                  `json:"result"`
	Conflicts []medianame.CandidateConflict `json:"conflicts,omitempty"`
}

//go:embed enhancement.html
var enhancementHTML string

// WriteEnhancementHTML 生成去掉大型在线原始响应的本地可筛选预览。
func WriteEnhancementHTML(directory string, records []Enhancement, summary map[string]int, complete bool) error {
	preview := append([]Enhancement(nil), records...)
	for index := range preview {
		row := &preview[index]
		if row.Baseline != nil {
			baseline := *row.Baseline
			baseline.Raw = nil
			row.Baseline = &baseline
		}
		row.Attempts = append([]EnhancementAttempt(nil), row.Attempts...)
		for i := range row.Attempts {
			if row.Attempts[i].Result != nil {
				response := *row.Attempts[i].Result
				response.Raw = nil
				row.Attempts[i].Result = &response
			}
		}
	}
	data, err := json.Marshal(struct {
		Records  []Enhancement  `json:"records"`
		Summary  map[string]int `json:"summary"`
		Complete bool           `json:"complete"`
	}{preview, summary, complete})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(directory, "report.html"), []byte(strings.Replace(enhancementHTML, "@@ENHANCEMENT_JSON@@", string(data), 1)))
}

type Enhancement struct {
	Input
	BaselineStatus string               `json:"baseline_status"`
	Baseline       *Recognition         `json:"baseline"`
	Local          medianame.Info       `json:"local"`
	Status         string               `json:"status"`
	Attempts       []EnhancementAttempt `json:"attempts,omitempty"`
}

// Candidate 从服务身份字段构造公开包的校验输入。
func Candidate(recognition *Recognition) medianame.MediaCandidate {
	media := recognition.Media
	year, _ := strconv.Atoi(text(media["year"]))
	mediaType := medianame.TypeUnknown
	switch text(media["type"]) {
	case "电影", "movie":
		mediaType = medianame.TypeMovie
	case "电视剧", "tv":
		mediaType = medianame.TypeTV
	}
	return medianame.MediaCandidate{Title: text(media["title"]), OriginalTitle: text(media["original_title"]), Aliases: candidateAliases(recognition), Year: year, Type: mediaType,
		IDs: medianame.MediaIDs{TMDB: text(media["tmdb_id"]), TVDB: text(media["tvdb_id"]), IMDb: text(media["imdb_id"]), Douban: text(media["douban_id"]), Bangumi: text(media["bangumi_id"]), AniList: text(media["anilist_id"])}}
}

// 只使用 MP2 返回的英文名与别名；旧报告可以从已保存的原始响应补充这些字段。
func candidateAliases(recognition *Recognition) []string {
	var aliases []string
	seen := map[string]bool{}
	add := func(value any) {
		name, ok := value.(string)
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if ok && name != "" && !seen[key] {
			aliases = append(aliases, name)
			seen[key] = true
		}
	}
	raw, _ := recognition.Raw["media_info"].(map[string]any)
	for _, media := range []map[string]any{recognition.Media, raw} {
		add(media["en_title"])
		switch names := media["names"].(type) {
		case []any:
			for _, name := range names {
				add(name)
			}
		case []string:
			for _, name := range names {
				add(name)
			}
		}
	}
	return aliases
}

// Enhance 用增强后的本地解析生成查询，并拒绝将存在身份冲突的响应计为无冲突候选。
// 原始 MP2 结果独立保留，不覆盖影子测试基线。
func Enhance(ctx context.Context, client *Client, baseline Record) Enhancement {
	info := medianame.ParsePath(baseline.Path)
	if baseline.Path == "" {
		info = medianame.Parse(baseline.Name)
	}
	result := Enhancement{Input: baseline.Input, Baseline: baseline.Recognition, Local: info, Status: "no_candidate"}
	if baseline.Recognition != nil {
		result.BaselineStatus = baseline.Recognition.Status
	}
	queries := info.SearchQueries()
	if len(queries) == 0 {
		result.Status = "no_query"
		return result
	}
	for _, query := range queries {
		if ctx.Err() != nil {
			if result.Status == "no_candidate" {
				result.Status = "request_error"
			}
			break
		}
		recognition := client.Recognize(ctx, query.Text, false)
		attempt := EnhancementAttempt{Query: query, Result: recognition}
		if recognition.Status == "identified" {
			attempt.Conflicts = info.CheckCandidate(Candidate(recognition))
			if len(attempt.Conflicts) == 0 {
				result.Status = "compatible"
			} else if result.Status != "compatible" {
				result.Status = "review"
			}
		} else if recognition.Status == "error" && result.Status == "no_candidate" {
			result.Status = "request_error"
		}
		result.Attempts = append(result.Attempts, attempt)
		if result.Status == "compatible" {
			break
		}
	}
	return result
}
