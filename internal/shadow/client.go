package shadow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client 调用 MP2 的规则测试接口，使用不存在的规则组取得原始 MetaInfo。
// 该分支在规则组查询后立即返回，不进行媒体数据库识别或文件整理。
type Client struct {
	BaseURL   string
	Token     string
	HTTP      *http.Client
	RuleGroup string
}

// ParseName 从真实 MP2 服务取得名称解析结果；认证信息不进入返回值或错误文本。
func (c *Client) ParseName(ctx context.Context, name string) (map[string]any, error) {
	payload, err := c.get(ctx, "/api/v1/system/ruletest", url.Values{"title": {name}, "rulegroup_name": {c.RuleGroup}})
	if err != nil {
		return nil, err
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, errors.New("MP2 响应缺少 data")
	}
	if data["rulegroup"] != nil || data["media_info"] != nil || payload["success"] == true {
		return nil, errors.New("MP2 未返回预期的纯名称解析分支，停止对该响应进行比较")
	}
	meta, ok := data["meta_info"].(map[string]any)
	if !ok || len(meta) == 0 {
		return nil, errors.New("MP2 响应缺少 data.meta_info")
	}
	return meta, nil
}

// Recognition 保存 MP2 在线媒体识别的状态、身份字段及原始响应。
type Recognition struct {
	Status string         `json:"status"`
	Media  map[string]any `json:"media,omitempty"`
	Raw    map[string]any `json:"raw,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// Recognize 通过识别接口测试标题或路径，不调用整理、重命名或刮削写入接口。
func (c *Client) Recognize(ctx context.Context, value string, asPath bool) *Recognition {
	endpoint, key := "/api/v1/media/recognize", "title"
	if asPath {
		endpoint, key = "/api/v1/media/recognize_file", "path"
	}
	payload, err := c.get(ctx, endpoint, url.Values{key: {value}})
	if err != nil {
		return &Recognition{Status: "error", Error: err.Error()}
	}
	if _, ok := payload["media_info"]; !ok {
		return &Recognition{Status: "error", Raw: payload, Error: "MP2 未返回识别 Context 结构"}
	}
	media, _ := payload["media_info"].(map[string]any)
	if len(media) == 0 {
		return &Recognition{Status: "not_identified", Raw: payload}
	}
	identity := map[string]any{}
	for _, field := range []string{"title", "original_title", "en_title", "names", "year", "type", "source", "media_id", "tmdb_id", "imdb_id", "tvdb_id", "douban_id", "bangumi_id", "anilist_id", "category"} {
		if media[field] != nil {
			identity[field] = media[field]
		}
	}
	return &Recognition{Status: "identified", Media: identity, Raw: payload}
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values) (map[string]any, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("MP2 地址必须是无认证信息、无查询参数的 HTTP(S) 服务地址")
	}
	if c.Token == "" {
		return nil, errors.New("缺少 MP2 API Token")
	}
	base.Path = strings.TrimRight(base.Path, "/") + endpoint
	query.Set("token", c.Token)
	base.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, errors.New("无法创建 MP2 请求")
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		// url.Error 会携带完整查询串；仅保留内部网络错误，避免认证信息进入报告。
		var urlError *url.Error
		if errors.As(err, &urlError) {
			err = urlError.Err
		}
		return nil, fmt.Errorf("MP2 网络请求失败: %s", strings.ReplaceAll(err.Error(), c.Token, "[redacted]"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MP2 HTTP %d", response.StatusCode)
	}
	var payload map[string]any
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, errors.New("MP2 返回无效 JSON")
	}
	return payload, nil
}
