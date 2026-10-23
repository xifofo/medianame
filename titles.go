package medianame

import (
	"strings"
	"unicode"
)

// 按完整词区分相邻的中文、拉丁文字片名，复用已有双语标题的 " / " 格式。
// 不从阿Q正传等单个混合词内部拆分，也不拆其他文字、括号版本说明或交错语言。
func splitBilingualTitle(title string) string {
	if strings.Contains(title, " / ") {
		return title
	}
	var script, changes, split, han, latin, depth int
	start := 0
	consume := func(end int) bool {
		token := title[start:end]
		var current int
		for _, r := range token {
			var kind int
			switch {
			case unicode.Is(unicode.Han, r):
				kind = 1
				han++
			case unicode.Is(unicode.Latin, r):
				kind = 2
				latin++
			case unicode.IsLetter(r):
				return false
			}
			if kind != 0 {
				if current != 0 && current != kind {
					return false
				}
				current = kind
			}
		}
		if current != 0 {
			if script != 0 && script != current {
				changes++
				split = start
				if changes > 1 || strings.HasPrefix(token, "(") || strings.HasPrefix(token, "{") {
					return false
				}
			}
			script = current
		}
		return true
	}
	for i, r := range title {
		switch r {
		case '(', '[', '{', '【':
			depth++
		case ')', ']', '}', '】':
			if depth > 0 {
				depth--
			}
		}
		if unicode.IsSpace(r) && depth == 0 {
			if !consume(i) {
				return title
			}
			start = i + len(string(r))
		}
	}
	if !consume(len(title)) || changes != 1 || han < 2 || latin < 2 {
		return title
	}
	left, right := cleanTitle(title[:split]), cleanTitle(title[split:])
	if script == 1 {
		left, right = right, left
	}
	return left + " / " + right
}
