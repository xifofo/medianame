package medianame

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	idEnvelopePattern   = regexp.MustCompile(`\{\[([^\[\]{}]+)\]\}|\[([^\[\]{}]+)\]|\{([^{}]+)\}|\{([^{}\[\]()]+)\)`)
	idTagPattern        = regexp.MustCompile(`(?i)^(tmdb(?:id)?|tvdb(?:id)?|douban(?:id)?|bangumi(?:id)?|anilist(?:id)?|imdb(?:id)?)[=\-](tt[0-9]{1,20}|[0-9]{1,20})$`)
	numberRangePattern  = regexp.MustCompile(`^([0-9]{1,4})(?:-([0-9]{1,4}))?$`)
	episodeGroupPattern = regexp.MustCompile(`^[0-9a-fA-F]{1,64}$`)
)

func parseIDs(text string, info *Info) string {
	var result strings.Builder
	last := 0
	for _, m := range idEnvelopePattern.FindAllStringSubmatchIndex(text, -1) {
		content := ""
		for group := 1; group < len(m)/2; group++ {
			if m[group*2] >= 0 {
				content = text[m[group*2]:m[group*2+1]]
				break
			}
		}
		recognized := false
		composite := strings.HasPrefix(text[m[0]:m[1]], "{[")
		fields := strings.Split(content, ";")
		if m[8] >= 0 {
			// 容忍单个 ID 标签把右花括号写成圆括号，例如 ｛tmdbid-67018）。
			// 内容仍须完整匹配合法 ID，不从普通括号文字或复合标签中猜测。
			fields = []string{content}
		}
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if tag := idTagPattern.FindStringSubmatch(field); tag != nil {
				key, value := strings.TrimSuffix(strings.ToLower(tag[1]), "id"), strings.ToLower(tag[2])
				if key != "imdb" && strings.HasPrefix(value, "tt") || strings.TrimLeft(strings.TrimPrefix(value, "tt"), "0") == "" {
					continue
				}
				if key == "imdb" && !strings.HasPrefix(value, "tt") {
					continue
				}
				setID(&info.IDs, key, value)
				recognized = true
				continue
			}
			if !composite {
				continue
			}
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			switch key {
			case "type":
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "movie", "movies":
					info.Type, recognized = TypeMovie, true
					info.typeExplicit = true
				case "tv":
					info.Type, recognized = TypeTV, true
					info.typeExplicit = true
				}
			case "s", "e":
				if values := numberRangePattern.FindStringSubmatch(value); values != nil {
					start, _ := strconv.Atoi(values[1])
					var end *int
					if values[2] != "" {
						n, _ := strconv.Atoi(values[2])
						if start > n {
							start, n = n, start
						}
						end = integer(n)
					}
					if key == "s" {
						info.Season, info.SeasonEnd = integer(start), end
					} else {
						info.Episode, info.EpisodeEnd = integer(start), end
					}
					recognized = true
				}
			case "g":
				if episodeGroupPattern.MatchString(value) {
					info.EpisodeGroup, recognized = value, true
				}
			}
		}
		if recognized {
			result.WriteString(text[last:m[0]])
			result.WriteByte(' ')
			last = m[1]
		}
	}
	result.WriteString(text[last:])
	return strings.TrimSpace(result.String())
}

func setID(ids *MediaIDs, key, value string) {
	switch key {
	case "tmdb":
		ids.TMDB = value
	case "tvdb":
		ids.TVDB = value
	case "imdb":
		ids.IMDb = value
	case "douban":
		ids.Douban = value
	case "bangumi":
		ids.Bangumi = value
	case "anilist":
		ids.AniList = value
	}
}
