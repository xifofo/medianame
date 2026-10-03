package medianame

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

var numericFilenamePattern = regexp.MustCompile(`^[0-9]{1,3}$`)
var blurayStreamPattern = regexp.MustCompile(`^[0-9]{5}$`)

// ParsePath 解析 POSIX 或 Windows 媒体路径，并从紧邻的媒体目录补充缺失字段。
// 支持“媒体目录/Season 01/文件”及 BDMV/STREAM/五位数字.m2ts 蓝光布局。
// 已有的文件字段优先，纯季目录只补季号；合并后剧集仍缺季号则默认为第 1 季。
// 函数只处理字符串，不检查文件是否存在，也不继续向媒体目录以上继承。
func ParsePath(filename string) (info Info) {
	cleaned := path.Clean(strings.ReplaceAll(filename, `\`, "/"))
	leaf := path.Base(cleaned)
	info = parseName(leaf, true)
	defer func() {
		inferType(&info)
		defaultSeason(&info)
	}()
	info.Original = filename
	if cleaned == "." || leaf == "/" {
		return Info{Original: filename, Type: TypeUnknown}
	}
	stem := leaf[:len(leaf)-len(info.Extension)]
	directory := path.Dir(cleaned)
	// 只有确切的蓝光目录结构和分片名才跨过技术目录；普通 STREAM 片名不受影响。
	if info.Extension == ".m2ts" && blurayStreamPattern.MatchString(stem) &&
		strings.EqualFold(path.Base(directory), "STREAM") && strings.EqualFold(path.Base(path.Dir(directory)), "BDMV") {
		mediaDirectory := path.Dir(path.Dir(directory))
		info.Title, info.Type = "", TypeUnknown
		if mediaDirectory != "." && mediaDirectory != "/" {
			parent := parseName(path.Base(mediaDirectory), true)
			info.Title, info.Year, info.Type = parent.Title, parent.Year, parent.Type
			info.typeExplicit = parent.typeExplicit
			info.Season, info.SeasonEnd = parent.Season, parent.SeasonEnd
			info.IDs, info.EpisodeGroup, info.Warnings = parent.IDs, parent.EpisodeGroup, parent.Warnings
			info.Aliases = parent.Aliases
		}
		info.Source = "BluRay"
		return info
	}
	if numericFilenamePattern.MatchString(stem) {
		value, _ := strconv.Atoi(stem)
		info.Title, info.Episode, info.Type = "", integer(value), TypeTV
	}
	if directory == "." || directory == "/" {
		return info
	}
	parent := parseName(path.Base(directory), true)
	if parent.Title == "" && parent.Season != nil {
		if info.Season == nil {
			info.Season, info.SeasonEnd = parent.Season, parent.SeasonEnd
		}
		directory = path.Dir(directory)
		if directory == "." || directory == "/" {
			return info
		}
		parent = parseName(path.Base(directory), true)
	}
	if parent.Title == "" {
		return info
	}
	// 有自己片名的文件仅从同名媒体目录继承，避免混入合集目录的年份或 ID。
	if info.Title != "" && !strings.EqualFold(info.Title, parent.Title) {
		return info
	}
	if info.Title == "" {
		info.Title = parent.Title
		info.Aliases = parent.Aliases
	}
	if info.Year == 0 {
		info.Year = parent.Year
	}
	if info.Season == nil {
		info.Season, info.SeasonEnd = parent.Season, parent.SeasonEnd
	}
	inheritIDs(&info.IDs, parent.IDs)
	if info.EpisodeGroup == "" {
		info.EpisodeGroup = parent.EpisodeGroup
	}
	if !info.typeExplicit && info.Season == nil && info.Episode == nil && parent.typeExplicit {
		info.Type = parent.Type
		info.typeExplicit = true
	}
	return info
}

func inheritIDs(ids *MediaIDs, parent MediaIDs) {
	if ids.TMDB == "" {
		ids.TMDB = parent.TMDB
	}
	if ids.TVDB == "" {
		ids.TVDB = parent.TVDB
	}
	if ids.IMDb == "" {
		ids.IMDb = parent.IMDb
	}
	if ids.Douban == "" {
		ids.Douban = parent.Douban
	}
	if ids.Bangumi == "" {
		ids.Bangumi = parent.Bangumi
	}
	if ids.AniList == "" {
		ids.AniList = parent.AniList
	}
}
