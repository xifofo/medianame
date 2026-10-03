package medianame

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type span struct{ start, end int }

var (
	yearPattern            = regexp.MustCompile(`(?:18|19|20)[0-9]{2}`)
	spacePattern           = regexp.MustCompile(`\s+`)
	bracketPattern         = regexp.MustCompile(`\[([^\[\]]+)\]|【([^【】]+)】`)
	partPattern            = regexp.MustCompile(`(?i)(?:part|cd|disc|disk|dvd)[ ._-]*([0-9]{1,2})`)
	groupPattern           = regexp.MustCompile(`-([\p{L}\p{N}][\p{L}\p{N}_@.]*)$`)
	organizedPrefixPattern = regexp.MustCompile(`^(.+?)\s+\(((?:18|19|20)[0-9]{2})\)\s+-\s+`)
	checksumPattern        = regexp.MustCompile(`(?i)^[a-f0-9]{8}$`)
	auxiliaryTagPattern    = regexp.MustCompile(`(?i)^(?:MKV|MP4|AVI|ASS|SRT|GB|BIG5|CHS|CHT|ENG|JPN|MULTI|DUAL|FIN|END|COMPLETE|[0-9.]+[MGTP]i?B)$`)
	promotionPattern       = regexp.MustCompile(`^(?:[★☆\s]*[0-9]{0,2}月?新番[★☆\s]*|[\[【](?:[0-9]{0,2}月?新番|[0-9]{2}年日剧|国漫|國漫|日漫|美漫)[\]】]\s*)`)
	nameReplacer           = strings.NewReplacer("（", "(", "）", ")", "［", "[", "］", "]", "｛", "{", "｝", "}",
		"－", "-", "–", "-", "—", "-", "～", "~", "＿", "_")
	titleReplacer = strings.NewReplacer("][", " / ", "] [", " / ", "】【", " / ", "】 【", " / ", ".", " ", "_", " ")
	extensions    = map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
		".ts": true, ".m2ts": true, ".mts": true, ".wmv": true, ".flv": true,
		".webm": true, ".mpg": true, ".mpeg": true, ".vob": true, ".iso": true,
		".strm": true, ".srt": true, ".ass": true, ".ssa": true, ".sub": true,
		".sup": true, ".vtt": true,
	}
)

var boundaryPatterns map[*regexp.Regexp]*regexp.Regexp

func init() {
	// 在正则内匹配左边界，避免前一段数字吞掉当前标记，例如 H265.50FPS。
	// 包初始化后只读，所有并发调用共享编译结果。
	boundaryPatterns = make(map[*regexp.Regexp]*regexp.Regexp)
	patterns := []*regexp.Regexp{yearPattern, partPattern, seasonEpisodePattern,
		xEpisodePattern, seasonPattern, episodePattern}
	for _, rule := range technicalRules {
		patterns = append(patterns, rule.pattern)
	}
	for _, pattern := range patterns {
		boundaryPatterns[pattern] = regexp.MustCompile(`(?:^|[^A-Za-z0-9])(` + pattern.String() + `)`)
	}
}

// Parse 解析一个媒体文件名或发布标题。剧集缺少季号时默认为第 1 季，其余未识别字段保持空值。
// 输入路径请使用 ParsePath，以免将双语标题中的斜杠误当成目录分隔符。
// 该函数可由多个 goroutine 并发调用。
func Parse(name string) Info {
	info := parseName(name, true)
	defaultSeason(&info)
	return info
}

func parseName(name string, collectAliases bool) Info {
	info := Info{Original: name, Type: TypeUnknown}
	text := strings.TrimSpace(name)
	if dot := strings.LastIndexByte(text, '.'); dot >= 0 {
		ext := strings.ToLower(text[dot:])
		if extensions[ext] {
			info.Extension = ext
			text = strings.TrimSpace(text[:dot])
		}
	}
	text = normalize(text)
	text = parseIDs(text, &info)
	text = removeLeadingGroup(text, &info)
	for {
		prefix := promotionPattern.FindStringIndex(text)
		if prefix == nil {
			break
		}
		text = strings.TrimSpace(text[prefix[1]:])
	}
	if text == "" {
		return info
	}

	var used []span
	used = append(used, parseEpisodes(text, &info)...)
	year := findYear(text, used)
	prefix := organizedPrefixPattern.FindStringSubmatchIndex(text)
	if prefix != nil {
		// 整理后的前缀是独立身份线索，不让后面的重复标题、数字片名覆盖它。
		year = &span{prefix[4], prefix[5]}
		if releaseYear := findYear(text[prefix[1]:], nil); releaseYear != nil {
			value, _ := strconv.Atoi(text[prefix[1]+releaseYear.start : prefix[1]+releaseYear.end])
			prefixYear, _ := strconv.Atoi(text[year.start:year.end])
			if value != prefixYear {
				info.Warnings = append(info.Warnings, "conflicting_years")
			}
		}
	}
	if year != nil {
		info.Year, _ = strconv.Atoi(text[year.start:year.end])
		used = append(used, *year)
	}

	boundary := firstStart(used, len(text))
	tech := findTechnical(text)
	// 结构标记之前的普通词可能属于片名，例如 Amazon Forever 或 The WEB。
	// 没有结构标记时，以分辨率、编码等明确的资源标记作为片名边界。
	if boundary == len(text) {
		for _, token := range tech {
			if token.strong && (hasTitlePrefix(text[:token.start]) || token.field == "resolution") {
				boundary = token.start
				break
			}
		}
	}
	for _, token := range tech {
		if token.start >= boundary {
			applyTechnical(&info, token)
			used = append(used, token.span)
		}
	}
	// Part 只有伴随年份、季集或资源标记时才作为介质分卷；否则保留在片名中。
	for _, match := range boundedMatches(partPattern, text) {
		if match[0] >= boundary {
			info.Part = number(text, match, 1)
			used = append(used, span{match[0], match[1]})
			break
		}
	}
	if info.ReleaseGroup == "" && boundary < len(text) {
		info.ReleaseGroup = trailingGroup(text, used)
	}
	info.Title = cleanTitle(text[:boundary])
	if collectAliases && prefix != nil {
		release := parseName(text[prefix[1]:], false)
		// 只将同年份的发行名称作为别名；冲突年份不强行合并为同一部作品。
		if release.Year == info.Year && release.Title != "" {
			titles := strings.Split(release.Title, " / ")
			if bracket := bracketPattern.FindStringIndex(release.Title); bracket != nil && bracket[0] == 0 {
				titles = []string{cleanTitle(release.Title[:bracket[1]]), cleanTitle(release.Title[bracket[1]:])}
			}
			for _, title := range titles {
				if title != "" && !strings.EqualFold(title, info.Title) {
					info.Aliases = append(info.Aliases, title)
				}
			}
		}
	}
	inferType(&info)
	return info
}

func normalize(text string) string {
	return nameReplacer.Replace(text)
}

func cleanTitle(text string) string {
	text = titleReplacer.Replace(text)
	text = strings.Trim(text, " \t\r\n.-~/")
	// 一次匹配括号，避免很深的嵌套标题在反复剥离时退化为平方耗时。
	runes := []rune(text)
	matched := make([]int, len(runes))
	for i := range matched {
		matched[i] = -1
	}
	closing := map[rune]rune{'[': ']', '(': ')', '{': '}', '【': '】'}
	var stack []int
	for i, r := range runes {
		if _, open := closing[r]; open {
			stack = append(stack, i)
			continue
		}
		if len(stack) > 0 && closing[runes[stack[len(stack)-1]]] == r {
			opening := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			matched[opening], matched[i] = i, opening
		}
	}
	start, end := 0, len(runes)
	for start < end {
		switch {
		case unicode.IsSpace(runes[start]):
			start++
		case unicode.IsSpace(runes[end-1]):
			end--
		case matched[start] == end-1:
			start++
			end--
		case matched[start] == -1 && strings.ContainsRune("[({【", runes[start]):
			start++
		case matched[end-1] == -1 && strings.ContainsRune("[](){}【】", runes[end-1]):
			end--
		default:
			return spacePattern.ReplaceAllString(string(runes[start:end]), " ")
		}
	}
	return ""
}

func hasTitlePrefix(text string) bool {
	return cleanTitle(text) != ""
}

func firstStart(spans []span, fallback int) int {
	for _, s := range spans {
		if s.start < fallback {
			fallback = s.start
		}
	}
	return fallback
}

func boundedMatches(pattern *regexp.Regexp, text string) [][]int {
	var result [][]int
	for _, wrapped := range boundaryPatterns[pattern].FindAllStringSubmatchIndex(text, -1) {
		match := wrapped[2:]
		if match[1] == len(text) || !asciiAlnum(text[match[1]]) {
			result = append(result, match)
		}
	}
	return result
}

func asciiAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func number(text string, match []int, group int) int {
	if len(match) <= group*2+1 || match[group*2] < 0 {
		return 0
	}
	n, _ := strconv.Atoi(text[match[group*2]:match[group*2+1]])
	return n
}

func integer(value int) *int { return &value }

func findYear(text string, occupied []span) *span {
	var result *span
	parenthesized := false
	// 一次查出片名起点；每个年份都清理完整前缀会使重复年份的长输入退化。
	titleStart := strings.IndexFunc(text, func(r rune) bool {
		return !unicode.IsSpace(r) && !strings.ContainsRune(".-_~/[](){}【】", r)
	})
	for _, m := range boundedMatches(yearPattern, text) {
		s := span{m[0], m[1]}
		if titleStart < 0 || titleStart >= s.start || overlaps(s, occupied) {
			continue
		}
		// 年份紧邻中文时可能是片名的一部分，例如“新精武门1991”。
		if s.start > 0 && text[s.start-1] >= 0x80 {
			continue
		}
		if strings.HasPrefix(text[s.end:], "年") {
			continue
		}
		wrapped := s.start > 0 && s.end < len(text) &&
			(text[s.start-1] == '(' || text[s.start-1] == '[') &&
			(text[s.end] == ')' || text[s.end] == ']')
		if !parenthesized || wrapped {
			result, parenthesized = &s, wrapped
		}
	}
	return result
}

func overlaps(s span, occupied []span) bool {
	for _, other := range occupied {
		if s.start < other.end && other.start < s.end {
			return true
		}
	}
	return false
}

func removeLeadingGroup(text string, info *Info) string {
	m := bracketPattern.FindStringSubmatchIndex(text)
	if m == nil || m[0] != 0 {
		return text
	}
	start, end := m[2], m[3]
	if start < 0 {
		start, end = m[4], m[5]
	}
	content := text[start:end]
	rest := strings.TrimSpace(text[m[1]:])
	// 前置方括号也可以装片名；仅在后面有独立片名及动漫集号线索时视为发布组。
	if rest == "" || len(findTechnical(content)) > 0 ||
		checksumPattern.MatchString(content) || allDigits(content) {
		return text
	}
	if next := bracketPattern.FindStringSubmatchIndex(rest); next != nil && next[0] == 0 {
		start, end := next[2], next[3]
		if start < 0 {
			start, end = next[4], next[5]
		}
		if allDigits(rest[start:end]) || animeBracketPattern.MatchString(rest[:next[1]]) || len(findTechnical(rest[start:end])) > 0 {
			return text
		}
	}
	groupLike := strings.Contains(content, "字幕") || strings.Contains(content, "发布") ||
		strings.ContainsAny(content, "&_.-") || strings.HasSuffix(strings.ToLower(content), "group") ||
		strings.HasSuffix(strings.ToLower(content), "raws") || strings.HasSuffix(strings.ToLower(content), "subs") ||
		promotionPattern.MatchString(rest)
	if !strings.ContainsAny(content, " \t") && containsLatin(content) &&
		!strings.HasPrefix(rest, "[") && !strings.HasPrefix(rest, "【") &&
		(info.Extension != "" || len(findTechnical(rest)) > 0) && findYear(rest, nil) != nil {
		groupLike = true
	}
	if !groupLike && !hasAnimeEpisode(rest) {
		return text
	}
	if groupLike || (strings.HasPrefix(rest, "[") || strings.HasPrefix(rest, "【")) &&
		!strings.ContainsAny(content, " \t") && containsLatin(content) || animeDashPattern.MatchString(rest) {
		info.ReleaseGroup = content
		return rest
	}
	return text
}

func containsLatin(text string) bool {
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func trailingGroup(text string, occupied []span) string {
	if match := groupPattern.FindStringSubmatchIndex(text); match != nil {
		s := span{match[2], match[3]}
		// 发布组也可以是纯数字（如 -404）；已识别的集范围、编码等标记不能再作为发布组。
		if !overlaps(s, occupied) {
			return text[s.start:s.end]
		}
	}
	matches := bracketPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return ""
	}
	m := matches[len(matches)-1]
	if strings.TrimSpace(text[m[1]:]) != "" || overlaps(span{m[0], m[1]}, occupied) {
		return ""
	}
	start, end := m[2], m[3]
	if start < 0 {
		start, end = m[4], m[5]
	}
	value := text[start:end]
	if checksumPattern.MatchString(value) || auxiliaryTagPattern.MatchString(value) || allDigits(value) ||
		strings.ContainsAny(value, " \t") || strings.ContainsAny(value, "简繁字幕中文日语英語英语") {
		return ""
	}
	return value
}
