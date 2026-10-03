package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/xifofo/medianame"
)

// ExternalSource 是 FindByID 支持的外部影视数据库。
type ExternalSource string

const (
	SourceIMDb ExternalSource = "imdb_id"
	SourceTVDB ExternalSource = "tvdb_id"
)

// FindResult 只包含电影和整部剧集摘要；完整信息通过 Details 取得。
// 人物、季和单集结果不转换为整部作品，也不作为影视候选返回。
type FindResult struct {
	Results []MediaInfo `json:"results"`
}

// FindByID 通过 IMDb 或 TVDB ID 查询对应的 TMDB 作品，不按片名搜索。
// IMDb ID 必须带 tt 前缀；TVDB ID 必须为正整数，且仅支持整部剧集。
// 没有作品映射时返回空 Results；请求或响应格式错误通过 error 返回。
func (c *Client) FindByID(ctx context.Context, source ExternalSource, id string) (*FindResult, error) {
	id, err := externalID(source, id)
	if err != nil {
		return nil, err
	}
	data, err := c.get(ctx, "/find/"+id, url.Values{"external_source": {string(source)}}, func(data []byte) error {
		_, err := decodeFindResult(data, source)
		return err
	})
	if err != nil {
		return nil, err
	}
	return decodeFindResult(data, source)
}

func externalID(source ExternalSource, id string) (string, error) {
	switch source {
	case SourceIMDb:
		id = strings.ToLower(id)
		if strings.HasPrefix(id, "tt") {
			if _, err := canonicalID(id[2:]); err == nil {
				// IMDb 的前导零是 ID 的一部分，例如 tt0073589。
				return id, nil
			}
		}
		return "", errors.New("IMDb ID 必须为 tt 加 1–20 位非全零数字")
	case SourceTVDB:
		if canonical, err := canonicalID(id); err == nil {
			return canonical, nil
		}
		return "", errors.New("TVDB ID 必须是 1–20 位正整数")
	default:
		return "", errors.New("外部 ID 查询仅支持 imdb_id 或 tvdb_id")
	}
}

func decodeFindResult(data []byte, source ExternalSource) (*FindResult, error) {
	var wire struct {
		Movies   []json.RawMessage `json:"movie_results"`
		TV       []json.RawMessage `json:"tv_results"`
		People   []json.RawMessage `json:"person_results"`
		Seasons  []json.RawMessage `json:"tv_season_results"`
		Episodes []json.RawMessage `json:"tv_episode_results"`
	}
	if err := json.Unmarshal(data, &wire); err != nil || wire.Movies == nil || wire.TV == nil {
		return nil, errInvalidJSON
	}
	if source == SourceTVDB && len(wire.Movies) != 0 {
		return nil, errors.New("TMDB 的 TVDB 反查响应包含不支持的电影映射")
	}
	result := &FindResult{Results: []MediaInfo{}}
	for _, group := range []struct {
		mediaType medianame.MediaType
		items     []json.RawMessage
	}{{medianame.TypeMovie, wire.Movies}, {medianame.TypeTV, wire.TV}} {
		for _, raw := range group.items {
			media, err := decodeMedia(raw, group.mediaType)
			if err != nil {
				return nil, err
			}
			media.Raw = nil
			result.Results = append(result.Results, *media)
		}
	}
	return result, nil
}
