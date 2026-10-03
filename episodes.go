package medianame

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	seasonEpisodePattern  = regexp.MustCompile(`(?i)S([0-9]{1,3})[ ._-]*EP?([0-9]{1,4})(?:[ ._]*[-~][ ._]*(?:S([0-9]{1,3})[ ._-]*)?EP?([0-9]{1,4})|[ ._]*[-~][ ._]*([0-9]{1,4}))?((?:[ ._]*EP?[0-9]{1,4})*)(?:v([0-9]{1,2}))?`)
	xEpisodePattern       = regexp.MustCompile(`(?i)([0-9]{1,2})x([0-9]{1,3})(?:-([0-9]{1,3}))?`)
	seasonPattern         = regexp.MustCompile(`(?i)(?:Season[ ._-]+|S)([0-9]{1,3})(?:[ ._]*[-~][ ._]*(?:Season[ ._-]+|S)([0-9]{1,3}))?`)
	episodePattern        = regexp.MustCompile(`(?i)(?:Episode[ ._-]+|TV[ ._-]+|#|EP?)([0-9]{1,4})(?:[ ._]*[-~][ ._]*(?:Episode[ ._-]+|TV[ ._-]+|EP?)?([0-9]{1,4}))?(?:v([0-9]{1,2}))?`)
	chineseSeasonPattern  = regexp.MustCompile(`第\s*([零〇一二两兩三四五六七八九十百千0-9]+)\s*季(?:\s*[-~]\s*第?\s*([零〇一二两兩三四五六七八九十百千0-9]+)\s*季)?`)
	chineseEpisodePattern = regexp.MustCompile(`第\s*([零〇一二两兩三四五六七八九十百千0-9]+)\s*(?:[集话話期])?(?:\s*[-~]\s*第?\s*([零〇一二两兩三四五六七八九十百千0-9]+)\s*)?[集话話期]`)
	animeBracketPattern   = regexp.MustCompile(`(?i)[\[【]([0-9]{1,4})(?:[-~]([0-9]{1,4}))?(?:v([0-9]{1,2}))?[\]】]`)
	animeDashPattern      = regexp.MustCompile(`(?i)\s-\s+([0-9]{1,4})(?:[-~]([0-9]{1,4}))?(?:v([0-9]{1,2}))?(?:\s*$|[ .]*[\[【(])`)
	extraEpisodePattern   = regexp.MustCompile(`(?i)EP?([0-9]{1,4})`)
)

func parseEpisodes(text string, info *Info) []span {
	var used []span
	if matches := boundedMatches(seasonEpisodePattern, text); len(matches) > 0 {
		m := matches[0]
		season, episode := number(text, m, 1), number(text, m, 2)
		if info.Season == nil {
			info.Season = integer(season)
		}
		if info.Episode == nil {
			info.Episode = integer(episode)
			if m[8] >= 0 || m[10] >= 0 {
				end := number(text, m, 4)
				if m[10] >= 0 {
					end = number(text, m, 5)
				}
				// 跨季集范围不能用单季的 EpisodeEnd 表达，保留两端季号。
				if m[6] >= 0 && number(text, m, 3) != season {
					info.SeasonEnd = integer(number(text, m, 3))
					info.EpisodeEnd = integer(end)
				} else {
					setEpisodeRange(info, episode, end)
				}
			} else if m[12] >= 0 && m[12] != m[13] {
				info.Episodes = []int{episode}
				for _, extra := range extraEpisodePattern.FindAllStringSubmatch(text[m[12]:m[13]], -1) {
					value, _ := strconv.Atoi(extra[1])
					if !containsInt(info.Episodes, value) {
						info.Episodes = append(info.Episodes, value)
					}
				}
			}
			info.EpisodeVersion = number(text, m, 7)
		}
		used = append(used, span{m[0], m[1]})
	}
	if len(used) == 0 {
		if matches := boundedMatches(xEpisodePattern, text); len(matches) > 0 {
			m := matches[0]
			if info.Season == nil {
				info.Season = integer(number(text, m, 1))
			}
			if info.Episode == nil {
				info.Episode = integer(number(text, m, 2))
				if m[6] >= 0 {
					setEpisodeRange(info, *info.Episode, number(text, m, 3))
				}
			}
			used = append(used, span{m[0], m[1]})
		}
	}
	for _, m := range boundedMatches(seasonPattern, text) {
		s := span{m[0], m[1]}
		if overlaps(s, used) {
			continue
		}
		if info.Season == nil {
			info.Season = integer(number(text, m, 1))
			if m[4] >= 0 {
				start, end := *info.Season, number(text, m, 2)
				if start > end {
					start, end = end, start
				}
				info.Season, info.SeasonEnd = integer(start), integer(end)
			}
		}
		used = append(used, s)
		break
	}
	for _, m := range boundedMatches(episodePattern, text) {
		s := span{m[0], m[1]}
		if overlaps(s, used) {
			continue
		}
		if info.Episode == nil {
			info.Episode = integer(number(text, m, 1))
			if m[4] >= 0 {
				setEpisodeRange(info, *info.Episode, number(text, m, 2))
			}
			info.EpisodeVersion = number(text, m, 3)
		}
		used = append(used, s)
		break
	}
	for _, m := range chineseSeasonPattern.FindAllStringSubmatchIndex(text, -1) {
		if value, ok := chineseNumber(text[m[2]:m[3]]); ok && value <= 999 {
			if info.Season == nil {
				info.Season = integer(value)
				if m[4] >= 0 {
					if end, ok := chineseNumber(text[m[4]:m[5]]); ok && end <= 999 {
						if value > end {
							value, end = end, value
						}
						info.Season, info.SeasonEnd = integer(value), integer(end)
					}
				}
			}
			used = append(used, span{m[0], m[1]})
			break
		}
	}
	for _, m := range chineseEpisodePattern.FindAllStringSubmatchIndex(text, -1) {
		if value, ok := chineseNumber(text[m[2]:m[3]]); ok && value <= 9999 {
			if info.Episode == nil {
				info.Episode = integer(value)
				if m[4] >= 0 {
					if end, ok := chineseNumber(text[m[4]:m[5]]); ok && end <= 9999 {
						setEpisodeRange(info, value, end)
					}
				}
			}
			used = append(used, span{m[0], m[1]})
			break
		}
	}
	if info.Episode == nil {
		var m []int
		if match := findAnimeEpisode(animeBracketPattern, text); match != nil && hasTitlePrefix(text[:match[0]]) {
			m = match
		} else if match := findAnimeEpisode(animeDashPattern, text); match != nil && len(findTechnical(text)) > 0 {
			m = match
		}
		if m != nil {
			info.Episode = integer(number(text, m, 1))
			if m[4] >= 0 {
				setEpisodeRange(info, *info.Episode, number(text, m, 2))
			}
			info.EpisodeVersion = number(text, m, 3)
			used = append(used, span{m[0], m[1]})
		}
	}
	if info.Season != nil || info.Episode != nil {
		if info.Type == TypeUnknown {
			info.Type = TypeTV
		}
	}
	return used
}

func setEpisodeRange(info *Info, start, end int) {
	if start > end {
		start, end = end, start
	}
	info.Episode, info.EpisodeEnd = integer(start), integer(end)
}

func containsInt(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func hasAnimeEpisode(text string) bool {
	return findAnimeEpisode(animeBracketPattern, text) != nil || findAnimeEpisode(animeDashPattern, text) != nil
}

func findAnimeEpisode(pattern *regexp.Regexp, text string) []int {
	for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
		// 无 E/S 前缀的四位数也可能是发行年份，保留给年份解析。
		// 例如 [2024] 不能仅因扩展了长篇动漫集号范围就变成第 2024 集。
		if yearPattern.MatchString(text[match[2]:match[3]]) {
			continue
		}
		return match
	}
	return nil
}

func chineseNumber(text string) (int, bool) {
	if len(text) > 32 || text == "" {
		return 0, false
	}
	if value, err := strconv.Atoi(text); err == nil {
		return value, value >= 0
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '兩': 2,
		'三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	units := map[rune]int{'十': 10, '百': 100, '千': 1000}
	result, current := 0, 0
	unitNotation := strings.ContainsAny(text, "十百千")
	for _, r := range text {
		if digit, ok := digits[r]; ok {
			if unitNotation {
				current = digit
			} else {
				result = result*10 + digit
			}
		} else if unit, ok := units[r]; ok {
			if current == 0 {
				current = 1
			}
			result += current * unit
			current = 0
		} else {
			return 0, false
		}
		if result > 9999 {
			return 0, false
		}
	}
	return result + current, true
}
