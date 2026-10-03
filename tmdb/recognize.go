package tmdb

import (
	"context"
	"errors"

	"medianame"
)

type Status string

const (
	StatusCompatible    Status = "compatible"     // 已选定可采用候选；已处理的元数据差异按 TMDB 为准。
	StatusReview        Status = "review"         // 身份有冲突，或搜索结果未完全查完。
	StatusAmbiguous     Status = "ambiguous"      // 多个可采用候选，无法唯一选择。
	StatusNotIdentified Status = "not_identified" // 未取得候选。
	StatusRequestError  Status = "request_error"  // 请求失败；返回部分结果并同时返回 error。
)

// Candidate 保存完整详情、尚待核对的冲突，以及可按 TMDB 处理的名称/年份差异。
type Candidate struct {
	MediaInfo         *MediaInfo                    `json:"media_info"`
	Conflicts         []medianame.CandidateConflict `json:"conflicts,omitempty"`
	ResolvedConflicts []medianame.CandidateConflict `json:"resolved_conflicts,omitempty"`
}

// Result 与 MP2 的 meta_info/media_info 结构对应，但使用本包自己的类型定义。
// 选定候选后填 MediaInfo，片名、原名、年份以 TMDB 为准。
// MetaInfo 保留解析线索，剧集缺失季号默认为 1；待核对和多候选的详情保存在 Candidates。
type Result struct {
	Status         Status         `json:"status"`
	MetaInfo       medianame.Info `json:"meta_info"`
	MediaInfo      *MediaInfo     `json:"media_info"`
	Candidates     []Candidate    `json:"candidates,omitempty"`
	Truncated      bool           `json:"truncated,omitempty"`
	SelectionBasis string         `json:"selection_basis,omitempty"` // tmdb_order：同名同年候选按 TMDB 顺序采用首项。
}

func (c *Client) Recognize(ctx context.Context, name string) (*Result, error) {
	return c.RecognizeInfo(ctx, medianame.Parse(name))
}

func (c *Client) RecognizePath(ctx context.Context, path string) (*Result, error) {
	return c.RecognizeInfo(ctx, medianame.ParsePath(path))
}

// RecognizeInfo 优先按 TMDB ID 查详情，其次用 IMDb/TVDB ID 反查作品，再取得完整详情。
// 外部 ID 同时存在时分别查询、合并候选，并校验全部输入 ID。
// 无上述 ID 或外部 ID 无电影/整剧映射时搜索名称及同年份别名；请求失败不回退。
// 年份搜索无结果时扩大搜索范围；作品身份一致时，年份差异以 TMDB 为准。
// 多页或详情数超限时标记 Truncated，不将部分候选静默作为唯一匹配。
func (c *Client) RecognizeInfo(ctx context.Context, info medianame.Info) (*Result, error) {
	// 也覆盖调用方直接构造 Info 的情况；显式季号（包括 0）保持原值。
	if info.Type == medianame.TypeTV && info.Season == nil {
		season := 1
		info.Season = &season
	}
	result := &Result{Status: StatusNotIdentified, MetaInfo: info}
	add := func(media *MediaInfo) {
		result.Candidates = append(result.Candidates, Candidate{MediaInfo: media, Conflicts: info.CheckCandidate(media.Candidate())})
	}
	fail := func(err error) (*Result, error) {
		result.Status = StatusRequestError
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if info.Type != "" && info.Type != medianame.TypeUnknown && !validType(info.Type) {
		return fail(errors.New("不支持的媒体类型"))
	}
	seen := map[string]bool{}
	addSummary := func(summary MediaInfo) error {
		key := string(summary.Type) + ":" + summary.TMDBID
		if seen[key] {
			return nil
		}
		seen[key] = true
		if len(result.Candidates) >= c.maxCandidates {
			result.Truncated = true
			return nil
		}
		media, err := c.Details(ctx, summary.Type, summary.TMDBID)
		if err != nil {
			return err
		}
		add(media)
		return nil
	}
	if info.IDs.TMDB != "" {
		for _, mediaType := range searchTypes(info.Type) {
			media, err := c.Details(ctx, mediaType, info.IDs.TMDB)
			if err != nil {
				var apiError *APIError
				if errors.As(err, &apiError) && apiError.StatusCode == 404 {
					continue
				}
				return fail(err)
			}
			add(media)
		}
	} else if info.IDs.IMDb != "" || info.IDs.TVDB != "" {
		for _, external := range []struct {
			source ExternalSource
			id     string
		}{{SourceIMDb, info.IDs.IMDb}, {SourceTVDB, info.IDs.TVDB}} {
			if external.id == "" {
				continue
			}
			found, err := c.FindByID(ctx, external.source, external.id)
			if err != nil {
				return fail(err)
			}
			for _, summary := range found.Results {
				if err := addSummary(summary); err != nil {
					return fail(err)
				}
			}
		}
	}
	if info.IDs.TMDB == "" && len(result.Candidates) == 0 {
		for _, query := range info.SearchQueries() {
			found, err := c.Search(ctx, query)
			if err != nil {
				return fail(err)
			}
			if len(found.Results) == 0 && query.Year != 0 {
				query.Year = 0
				found, err = c.Search(ctx, query)
				if err != nil {
					return fail(err)
				}
			}
			result.Truncated = result.Truncated || found.Truncated
			for _, summary := range found.Results {
				if err := addSummary(summary); err != nil {
					return fail(err)
				}
			}
		}
	}
	result.Reconcile()
	return result, nil
}
