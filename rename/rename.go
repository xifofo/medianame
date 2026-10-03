// Package rename 将 Go text/template 模板渲染为相对路径、目录和文件名。
// 它不访问网络、不读取模板文件、不移动或重命名实际文件。
package rename

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/xifofo/medianame"
)

// DefaultMovieTemplate 和 DefaultTVTemplate 将用户提供的 MP2 模板等价转换为 Go 语法。
const DefaultMovieTemplate = `{{.title}} ({{.year}}) {tmdb-{{.tmdbid}}}/{{.en_title}}.{{.year}}{{if .edition}}.{{.edition}}{{end}}{{if .videoFormat}}.{{.videoFormat}}{{end}}{{if .videoCodec}}.{{.videoCodec}}{{end}}{{if .audioCodec}}.{{.audioCodec}}{{end}}{{if .webSource}}.{{.webSource}}{{end}}{{if .releaseGroup}}-{{.releaseGroup}}{{end}}{{.fileExt}}`
const DefaultTVTemplate = `{{.title}} ({{.year}}) {tmdb-{{.tmdbid}}}/Season {{printf "%02d" (int .season)}}/{{.en_title}}.{{.year}}.{{.season_episode | replace " " ""}}{{if .edition}}.{{.edition}}{{end}}{{if .videoFormat}}.{{.videoFormat}}{{end}}{{if .videoCodec}}.{{.videoCodec}}{{end}}{{if .audioCodec}}.{{.audioCodec}}{{end}}{{if .webSource}}.{{.webSource}}{{end}}{{if .part}}.{{.part}}{{end}}{{if .releaseGroup}}-{{.releaseGroup}}{{end}}{{.fileExt}}`

// Context 使用 MP2 的变量名称，可由调用方添加或覆盖字段。
type Context map[string]any

// Media 是在线识别身份信息；资源参数与季集来自 medianame.Info。
type Media struct {
	Title, EnglishTitle, OriginalTitle string
	Year                               int
	Type                               medianame.MediaType
	IDs                                medianame.MediaIDs
	Category                           string
}

// Result.Path 使用 /，适合作为云盘或媒体库根目录下的相对路径。
type Result struct {
	Path      string `json:"path"`
	Directory string `json:"directory"`
	Name      string `json:"name"`
}

// BuildContext 每次生成独立变量表。数据库片名、年份和 ID 优先，资源参数使用本地解析值。
// 英文标题缺失时依次使用原名、标题，不猜测未知的英文译名。
func BuildContext(info medianame.Info, media Media) Context {
	title := first(media.Title, info.Title)
	enTitle := first(media.EnglishTitle, media.OriginalTitle, title)
	year := media.Year
	if year == 0 {
		year = info.Year
	}
	mediaType := media.Type
	if mediaType == "" || mediaType == medianame.TypeUnknown {
		mediaType = info.Type
	}
	titleYear := title
	var yearValue any = ""
	if year != 0 {
		yearValue = year
		titleYear = fmt.Sprintf("%s (%d)", title, year)
	}
	seasonFmt, episodeFmt := "", ""
	var season, episode any = "", ""
	if info.Season != nil {
		season = *info.Season
		seasonFmt = fmt.Sprintf("S%02d", *info.Season)
		if info.SeasonEnd != nil && *info.SeasonEnd != *info.Season {
			seasonFmt += fmt.Sprintf("-S%02d", *info.SeasonEnd)
		}
	}
	if len(info.Episodes) > 0 {
		var numbers []string
		for _, n := range info.Episodes {
			numbers = append(numbers, strconv.Itoa(n))
			episodeFmt += fmt.Sprintf("E%02d", n)
		}
		episode = strings.Join(numbers, ",")
	} else if info.Episode != nil {
		episode = *info.Episode
		episodeFmt = fmt.Sprintf("E%02d", *info.Episode)
		if info.EpisodeEnd != nil && (*info.EpisodeEnd != *info.Episode ||
			(info.SeasonEnd != nil && info.Season != nil && *info.SeasonEnd != *info.Season)) {
			episode = fmt.Sprintf("%d-%d", *info.Episode, *info.EpisodeEnd)
			if info.SeasonEnd != nil && info.Season != nil && *info.SeasonEnd != *info.Season {
				// 跨季范围保留两端的实际季集，不展开成同季范围。
				seasonFmt = fmt.Sprintf("S%02d", *info.Season)
				episodeFmt += fmt.Sprintf("-S%02dE%02d", *info.SeasonEnd, *info.EpisodeEnd)
			} else {
				episodeFmt += fmt.Sprintf("-E%02d", *info.EpisodeEnd)
			}
		}
	}
	seasonEpisode := seasonFmt + episodeFmt
	if mediaType != medianame.TypeTV {
		seasonEpisode = ""
	}
	var part, bit, fps any = "", "", ""
	if info.Part > 0 {
		part = fmt.Sprintf("part%d", info.Part)
	}
	if info.BitDepth > 0 {
		bit = fmt.Sprintf("%dbit", info.BitDepth)
	}
	if info.FPS > 0 {
		fps = info.FPS
	}
	var effects = strings.Join(info.Effects, " ")
	edition := strings.TrimSpace(strings.Join([]string{info.Source, effects}, " "))
	audio := info.AudioCodec
	if audio != "" && info.AudioChannels != "" {
		audio += " " + info.AudioChannels
	}
	for _, effect := range info.Effects {
		if effect == "Atmos" && audio != "" {
			audio += " Atmos"
			break
		}
	}
	originalName := path.Base(strings.ReplaceAll(info.Original, `\`, "/"))
	if info.Original == "" {
		originalName = ""
	}
	return Context{
		"title": title, "en_title": enTitle, "original_title": first(media.OriginalTitle, title),
		"name": info.Title, "en_name": "", "original_name": originalName,
		"year": yearValue, "title_year": titleYear, "type": string(mediaType), "category": media.Category,
		"tmdbid": first(media.IDs.TMDB, info.IDs.TMDB), "imdbid": first(media.IDs.IMDb, info.IDs.IMDb),
		"tvdbid": first(media.IDs.TVDB, info.IDs.TVDB), "doubanid": info.IDs.Douban,
		"bangumiid": info.IDs.Bangumi, "anilistid": info.IDs.AniList,
		"season": season, "season_fmt": seasonFmt, "episode": episode, "season_episode": seasonEpisode,
		"part": part, "edition": edition, "resourceType": info.Source, "effect": effects,
		"videoFormat": info.Resolution, "videoCodec": info.VideoCodec, "videoBit": bit,
		"audioCodec": audio, "webSource": info.StreamingService, "releaseGroup": info.ReleaseGroup,
		"resource_term": strings.TrimSpace(edition + " " + info.Resolution), "fps": fps,
		"fileExt": info.Extension, "customization": "", "episode_title": "", "episode_date": "",
		"total_episodes": 0, "season_year": "",
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Template 编译后可并发渲染，不修改调用方的 Context。
type Template struct{ compiled *template.Template }

// Compile 检查 Go 模板语法。支持内置 printf/index 等函数，另提供
// replace（管道输入在最后）、lower、upper、trim 和 int。
// 未定义的变量在渲染时返回错误，便于发现拼写或上下文错误。
func Compile(format string) (*Template, error) {
	if strings.TrimSpace(format) == "" {
		return nil, fmt.Errorf("重命名模板不能为空")
	}
	if strings.Contains(format, "{%") || strings.Contains(format, "{#") {
		return nil, fmt.Errorf("重命名模板使用 Go text/template 语法，请将 Jinja 条件改为 {{if .字段}}…{{end}}")
	}
	functions := template.FuncMap{
		"replace": func(old, replacement, value string) string { return strings.ReplaceAll(value, old, replacement) },
		"lower":   strings.ToLower, "upper": strings.ToUpper, "trim": strings.TrimSpace,
		"int": func(value any) (int, error) { return strconv.Atoi(fmt.Sprint(value)) },
	}
	compiled, err := template.New("rename").Option("missingkey=error").Funcs(functions).Parse(format)
	if err != nil {
		return nil, fmt.Errorf("重命名模板语法: %w", err)
	}
	return &Template{compiled: compiled}, nil
}

// Render 编译并渲染一次。批量场景可 Compile 后复用 Template.Render。
func Render(format string, context Context) (*Result, error) {
	template, err := Compile(format)
	if err != nil {
		return nil, err
	}
	return template.Render(context)
}

func (t *Template) Render(context Context) (*Result, error) {
	if t == nil || t.compiled == nil {
		return nil, fmt.Errorf("重命名模板未初始化")
	}
	data := make(map[string]any, len(context))
	for key, value := range context {
		if s, ok := value.(string); ok {
			value = safeSegment(s)
		}
		data[key] = value
	}
	var output bytes.Buffer
	if err := t.compiled.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("重命名模板渲染: %w", err)
	}
	rendered := strings.TrimSpace(output.String())
	if rendered == "" || strings.HasPrefix(rendered, "/") || strings.HasPrefix(rendered, `\`) ||
		(len(rendered) >= 2 && rendered[1] == ':' && unicode.IsLetter(rune(rendered[0]))) {
		return nil, fmt.Errorf("重命名结果必须是非空相对路径")
	}
	segments := strings.Split(rendered, "/")
	for i, segment := range segments {
		if strings.TrimSpace(segment) == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("重命名结果包含空目录或非法路径段")
		}
		for _, r := range segment {
			if unicode.IsControl(r) {
				return nil, fmt.Errorf("重命名结果包含控制字符")
			}
		}
		segments[i] = safeSegment(segment)
	}
	result := &Result{Path: strings.Join(segments, "/"), Name: segments[len(segments)-1]}
	if len(segments) > 1 {
		result.Directory = strings.Join(segments[:len(segments)-1], "/")
	}
	return result, nil
}

// safeSegment 与 MP2 一样，将文件名不支持的字符转换为全角。
func safeSegment(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return r + 0xFEE0
		}
		return r
	}, s)
}
