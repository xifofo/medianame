package tmdb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"medianame"
	"medianame/category"
)

const DefaultBaseURL = "https://api.themoviedb.org/3"

var (
	errInvalidJSON  = errors.New("TMDB 返回无效 JSON")
	errInvalidMedia = errors.New("TMDB 响应缺少有效的影视 ID 或标题")
)

// Config 的凭证由调用方提供；包不读取环境变量、不写入磁盘。
type Config struct {
	Token          string           // API Read Access Token，优先于 APIKey。
	APIKey         string           // v3 API Key；与 Token 至少提供一个。
	BaseURL        string           // 默认 DefaultBaseURL，代理地址需包含 /3 等 API 前缀。
	Language       string           // 默认 zh-CN。
	IncludeAdult   bool             // 控制名称搜索是否包括成人条目。
	HTTPClient     *http.Client     // 默认超时 20 秒；请求仍受传入的 context 控制。
	MaxCandidates  int              // 识别时最多查询的详情数，默认 20，范围 1–100。
	CategoryPolicy *category.Policy // 详情二级分类，nil 使用 category.Default()。
	Cache          CacheConfig      // 零值默认启用 16 MiB、512 条、1 小时的内存缓存。
}

// Client 创建后配置只读，可以并发使用。HTTPClient 的 Transport 也应支持并发。
type Client struct {
	base           string
	token          string
	apiKey         string
	language       string
	includeAdult   bool
	http           *http.Client
	maxCandidates  int
	categoryPolicy *category.Policy
	cache          *responseCache
}

func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("TMDB 地址必须是无认证信息、无查询参数的 HTTP(S) API 地址")
	}
	config.Token, config.APIKey = strings.TrimSpace(config.Token), strings.TrimSpace(config.APIKey)
	if config.Token == "" && config.APIKey == "" {
		return nil, errors.New("需要 TMDB API Read Access Token 或 API Key")
	}
	if strings.ContainsAny(config.Token+config.APIKey, "\r\n") {
		return nil, errors.New("TMDB 凭证不能包含换行")
	}
	if config.Language == "" {
		config.Language = "zh-CN"
	}
	if config.MaxCandidates == 0 {
		config.MaxCandidates = 20
	}
	if config.MaxCandidates < 1 || config.MaxCandidates > 100 {
		return nil, errors.New("MaxCandidates 必须为 1–100")
	}
	httpClient := http.Client{Timeout: 20 * time.Second}
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
	}
	// 不跟随重定向，避免凭证被转发到其他服务，且不修改调用方的 Client。
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if config.CategoryPolicy == nil {
		config.CategoryPolicy = category.Default()
	}
	cache, err := newResponseCache(config.Cache)
	if err != nil {
		return nil, err
	}
	return &Client{base: strings.TrimRight(base.String(), "/"), token: config.Token, apiKey: config.APIKey,
		language: config.Language, includeAdult: config.IncludeAdult, http: &httpClient, maxCandidates: config.MaxCandidates,
		categoryPolicy: config.CategoryPolicy, cache: cache}, nil
}

// APIError 保留 HTTP 状态、TMDB 错误码及 Retry-After，调用方可以区分无结果和请求失败。
type APIError struct {
	StatusCode int
	Code       int
	Message    string
	RetryAfter string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("TMDB HTTP %d (code %d): %s", e.StatusCode, e.Code, e.Message)
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values, validate func([]byte) error) ([]byte, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("language", c.language)
	key := responseKey(sha256.Sum256([]byte(endpoint + "?" + query.Encode())))
	if c.token == "" {
		query.Set("api_key", c.apiKey)
	}
	requestURL := c.base + endpoint + "?" + query.Encode()
	return c.cache.get(ctx, key, func(requestContext context.Context) ([]byte, error) {
		data, err := c.request(requestContext, requestURL)
		if err != nil {
			return nil, err
		}
		// HTTP 200 和合法 JSON 还不足以确认响应有效；身份、分页和字段校验
		// 通过后才缓存，防止错误 ID 或残缺数据让后续调用持续失败。
		if err := validate(data); err != nil {
			return nil, err
		}
		return data, nil
	})
}

func (c *Client) request(ctx context.Context, requestURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, errors.New("无法创建 TMDB 请求")
	}
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("TMDB 请求取消: %w", context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("TMDB 请求超时: %w", context.DeadlineExceeded)
		}
		var urlError *url.Error
		if errors.As(err, &urlError) {
			err = urlError.Err
		}
		return nil, fmt.Errorf("TMDB 网络请求失败: %s", c.redact(err.Error()))
	}
	defer response.Body.Close()
	const maxBody = 4 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, fmt.Errorf("TMDB 请求取消: %w", context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("TMDB 请求超时: %w", context.DeadlineExceeded)
		}
		return nil, errors.New("读取 TMDB 响应失败")
	}
	if len(data) > maxBody {
		return nil, errors.New("TMDB 响应超过 4 MiB")
	}
	var status struct {
		Success *bool  `json:"success"`
		Code    int    `json:"status_code"`
		Message string `json:"status_message"`
	}
	jsonErr := json.Unmarshal(data, &status)
	if response.StatusCode != http.StatusOK || (status.Success != nil && !*status.Success) {
		return nil, &APIError{StatusCode: response.StatusCode, Code: status.Code,
			Message: c.redact(status.Message), RetryAfter: c.redact(response.Header.Get("Retry-After"))}
	}
	if jsonErr != nil {
		return nil, errInvalidJSON
	}
	return data, nil
}

func (c *Client) redact(value string) string {
	for _, secret := range []string{c.token, c.apiKey} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
			value = strings.ReplaceAll(value, url.QueryEscape(secret), "[redacted]")
		}
	}
	return value
}

// Details 根据类型和 TMDB ID 返回完整影视详情，包含别名、译名、外部 ID 和演职员。
func (c *Client) Details(ctx context.Context, mediaType medianame.MediaType, id string) (*MediaInfo, error) {
	if !validType(mediaType) {
		return nil, errors.New("TMDB 详情查询需要 movie 或 tv 类型")
	}
	id, err := canonicalID(id)
	if err != nil {
		return nil, err
	}
	data, err := c.get(ctx, "/"+string(mediaType)+"/"+id, url.Values{
		"append_to_response": {"external_ids,alternative_titles,translations,credits"},
	}, func(data []byte) error {
		_, err := c.decodeDetails(data, mediaType, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return c.decodeDetails(data, mediaType, id)
}

func (c *Client) decodeDetails(data []byte, mediaType medianame.MediaType, id string) (*MediaInfo, error) {
	media, err := decodeMedia(data, mediaType)
	if err != nil {
		return nil, err
	}
	if media.TMDBID != id {
		return nil, errors.New("TMDB 详情响应 ID 与请求 ID 不一致")
	}
	media.Category, err = media.Classify(c.categoryPolicy)
	if err != nil {
		return nil, err
	}
	return media, nil
}

// SearchResult 保留搜索分页信息。搜索结果是摘要；完整信息请通过 Details 取得。
type SearchResult struct {
	Results      []MediaInfo `json:"results"`
	TotalPages   int         `json:"total_pages"`
	TotalResults int         `json:"total_results"`
	Truncated    bool        `json:"truncated"`
}

// Search 按片名、年份和类型查询第一页。未知类型分别搜索电影和剧集，不将其默认为电影。
// query.Text 是给发布名称识别服务的文本；TMDB 使用 query.Title 和独立年份参数。
func (c *Client) Search(ctx context.Context, query medianame.SearchQuery) (*SearchResult, error) {
	if strings.TrimSpace(query.Title) == "" {
		return nil, errors.New("TMDB 名称搜索需要片名")
	}
	if query.Type != "" && query.Type != medianame.TypeUnknown && !validType(query.Type) {
		return nil, errors.New("不支持的媒体类型")
	}
	result := &SearchResult{Results: []MediaInfo{}}
	for _, mediaType := range searchTypes(query.Type) {
		params := url.Values{"query": {query.Title}, "include_adult": {strconv.FormatBool(c.includeAdult)}, "page": {"1"}}
		if query.Year != 0 {
			field := "year"
			if mediaType == medianame.TypeTV {
				field = "first_air_date_year"
			}
			params.Set(field, strconv.Itoa(query.Year))
		}
		data, err := c.get(ctx, "/search/"+string(mediaType), params, func(data []byte) error {
			_, err := decodeSearchPage(data, mediaType)
			return err
		})
		if err != nil {
			return nil, err
		}
		page, err := decodeSearchPage(data, mediaType)
		if err != nil {
			return nil, err
		}
		result.TotalPages += page.TotalPages
		result.TotalResults += page.TotalResults
		result.Truncated = result.Truncated || page.Truncated
		result.Results = append(result.Results, page.Results...)
	}
	return result, nil
}

func decodeSearchPage(data []byte, mediaType medianame.MediaType) (*SearchResult, error) {
	var page struct {
		Results      []json.RawMessage `json:"results"`
		TotalPages   int               `json:"total_pages"`
		TotalResults int               `json:"total_results"`
	}
	if err := json.Unmarshal(data, &page); err != nil || page.Results == nil {
		return nil, errInvalidJSON
	}
	result := &SearchResult{Results: []MediaInfo{}, TotalPages: page.TotalPages,
		TotalResults: page.TotalResults, Truncated: page.TotalPages > 1}
	for _, raw := range page.Results {
		media, err := decodeMedia(raw, mediaType)
		if err != nil {
			return nil, err
		}
		media.Raw = nil
		result.Results = append(result.Results, *media)
	}
	return result, nil
}

func validType(mediaType medianame.MediaType) bool {
	return mediaType == medianame.TypeMovie || mediaType == medianame.TypeTV
}

func searchTypes(mediaType medianame.MediaType) []medianame.MediaType {
	if validType(mediaType) {
		return []medianame.MediaType{mediaType}
	}
	return []medianame.MediaType{medianame.TypeMovie, medianame.TypeTV}
}

func canonicalID(id string) (string, error) {
	if len(id) == 0 || len(id) > 20 {
		return "", errors.New("TMDB ID 必须是 1–20 位正整数")
	}
	for _, digit := range id {
		if digit < '0' || digit > '9' {
			return "", errors.New("TMDB ID 必须是 1–20 位正整数")
		}
	}
	id = strings.TrimLeft(id, "0")
	if id == "" {
		return "", errors.New("TMDB ID 必须为正整数")
	}
	return id, nil
}
