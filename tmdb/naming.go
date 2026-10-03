package tmdb

import (
	"bytes"
	"encoding/json"
	"fmt"

	"medianame"
	"medianame/category"
	"medianame/rename"
)

// Classify 使用完整 TMDB 原始详情及归一化字段计算二级分类，不发起网络请求。
// nil 策略使用内置分类；Raw 中其它一级字段也可作为条件。
func (m MediaInfo) Classify(policy *category.Policy) (string, error) {
	if policy == nil {
		policy = category.Default()
	}
	fields := map[string]any{}
	decode := func(data []byte, target *map[string]any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		return decoder.Decode(target)
	}
	if len(m.Raw) > 0 {
		if err := decode(m.Raw, &fields); err != nil {
			return "", fmt.Errorf("TMDB 分类原始详情: %w", err)
		}
		if fields == nil {
			fields = map[string]any{}
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	var normalized map[string]any
	if err := decode(data, &normalized); err != nil {
		return "", err
	}
	for key, value := range normalized {
		if key != "raw" && (key != "type" || fields[key] == nil) {
			fields[key] = value
		}
	}
	if m.Type == medianame.TypeTV && m.SeriesType != "" && (len(m.Raw) == 0 || fields["type"] == string(m.Type)) {
		fields["type"] = m.SeriesType
	}
	return policy.Match(m.Type, fields), nil
}

// RenameContext 合并在线身份和文件名资源信息，调用方可覆盖字段或添加自定义变量。
func (m MediaInfo) RenameContext(info medianame.Info) rename.Context {
	context := rename.BuildContext(info, rename.Media{Title: m.Title, EnglishTitle: m.EnglishTitle,
		OriginalTitle: m.OriginalTitle, Year: m.Year, Type: m.Type,
		IDs: medianame.MediaIDs{TMDB: m.TMDBID, IMDb: m.IMDbID, TVDB: m.TVDBID}, Category: m.Category})
	if m.Type == medianame.TypeTV && info.Season != nil {
		for _, season := range m.Seasons {
			if season.SeasonNumber == *info.Season {
				context["total_episodes"] = season.EpisodeCount
				if len(season.AirDate) >= 4 {
					context["season_year"] = season.AirDate[:4]
				}
				break
			}
		}
	}
	return context
}

// Rename 返回命名预览。format 为空时使用用户提供的默认电影/电视剧模板。
// 默认模板需要年份、TMDB ID；默认剧集模板还需要确定的季集，不将未知季号变成 S00。
func (m MediaInfo) Rename(info medianame.Info, format string) (*rename.Result, error) {
	if format == "" {
		if m.TMDBID == "" || m.Year == 0 || m.Title == "" {
			return nil, fmt.Errorf("默认重命名需要片名、年份和 TMDB ID")
		}
		switch m.Type {
		case medianame.TypeMovie:
			format = rename.DefaultMovieTemplate
		case medianame.TypeTV:
			if info.Season == nil || (info.Episode == nil && len(info.Episodes) == 0) {
				return nil, fmt.Errorf("默认剧集重命名需要确定的季号和集号")
			}
			if info.SeasonEnd != nil && *info.SeasonEnd != *info.Season {
				return nil, fmt.Errorf("默认剧集重命名不支持跨季文件，请提供自定义模板")
			}
			format = rename.DefaultTVTemplate
		default:
			return nil, fmt.Errorf("默认重命名需要 movie 或 tv 类型")
		}
	}
	return rename.Render(format, m.RenameContext(info))
}
