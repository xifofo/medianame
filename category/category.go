// Package category 根据有序规则和 TMDB 详情返回二级分类名称。
// 规则可通过 Go Config、JSON 或 YAML 配置。
// 多条件为 AND，单条件的逗号值为 OR；只返回第一个匹配，不创建目录。
package category

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"medianame"
)

// DefaultYAML 为用户提供的电影、电视剧分类策略。
//
//go:embed default.yaml
var DefaultYAML string

type condition struct {
	field  string
	values []string
}

type rule struct {
	name       string
	conditions []condition
}

// Policy 编译后只读，保留规则顺序，可并发使用。
type Policy struct {
	rules map[medianame.MediaType][]rule
}

var defaultPolicy = func() *Policy {
	p, err := Parse([]byte(DefaultYAML))
	if err != nil {
		panic(err)
	}
	return p
}()

// Default 返回内置只读策略。
func Default() *Policy { return defaultPolicy }

// Parse 支持 movie/tv，也兼容中文一级键电影/电视剧。
// 同一类型只能出现一次，重复分类、重复条件、无效结构返回带行号的错误。
func Parse(data []byte) (*Policy, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("分类 YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("分类 YAML 必须只有一个文档")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("分类 YAML 顶层必须是映射")
	}
	p := &Policy{rules: make(map[medianame.MediaType][]rule)}
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		var mediaType medianame.MediaType
		switch key.Value {
		case "movie", "电影":
			mediaType = medianame.TypeMovie
		case "tv", "电视剧":
			mediaType = medianame.TypeTV
		default:
			return nil, invalid(key, "一级分类仅支持 movie/tv 或 电影/电视剧")
		}
		if _, exists := p.rules[mediaType]; exists {
			return nil, invalid(key, "一级分类重复")
		}
		p.rules[mediaType] = nil
		if value.Tag == "!!null" {
			continue
		}
		if value.Kind != yaml.MappingNode {
			return nil, invalid(value, "二级分类必须是有序映射")
		}
		seen := map[string]bool{}
		for j := 0; j < len(value.Content); j += 2 {
			name, fields := value.Content[j], value.Content[j+1]
			if name.Kind != yaml.ScalarNode || strings.TrimSpace(name.Value) == "" {
				return nil, invalid(name, "分类名称不能为空")
			}
			if seen[name.Value] {
				return nil, invalid(name, "二级分类重复")
			}
			seen[name.Value] = true
			r := rule{name: name.Value}
			if fields.Tag != "!!null" {
				if fields.Kind != yaml.MappingNode {
					return nil, invalid(fields, "分类条件必须是映射或空值")
				}
				seenFields := map[string]bool{}
				for k := 0; k < len(fields.Content); k += 2 {
					field, values := fields.Content[k], fields.Content[k+1]
					if field.Kind != yaml.ScalarNode || field.Value == "" || seenFields[field.Value] {
						return nil, invalid(field, "条件字段为空或重复")
					}
					seenFields[field.Value] = true
					c := condition{field: field.Value}
					nodes := []*yaml.Node{values}
					if values.Kind == yaml.SequenceNode {
						nodes = values.Content
					}
					for _, node := range nodes {
						if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
							return nil, invalid(node, "条件值必须是标量或标量列表")
						}
						for _, v := range strings.Split(node.Value, ",") {
							v = strings.TrimSpace(v)
							if v == "" || v == "!" {
								return nil, invalid(node, "条件值不能为空")
							}
							c.values = append(c.values, strings.ToUpper(v))
						}
					}
					if len(c.values) == 0 {
						return nil, invalid(values, "条件值不能为空")
					}
					r.conditions = append(r.conditions, c)
				}
			}
			p.rules[mediaType] = append(p.rules[mediaType], r)
		}
	}
	return p, nil
}

func invalid(node *yaml.Node, message string) error {
	return fmt.Errorf("分类 YAML 第 %d 行: %s", node.Line, message)
}

// Match 返回首个命中的二级分类，无规则命中时返回空字符串。
// detail 使用 TMDB 详情的一级字段；genres 自动映射为 genre_ids。
// 支持 ! 排除值、数字范围及 release_year。缺失字段不会满足排除条件。
func (p *Policy) Match(mediaType medianame.MediaType, detail map[string]any) string {
	if p == nil || len(detail) == 0 {
		return ""
	}
	for _, r := range p.rules[mediaType] {
		matched := true
		for _, c := range r.conditions {
			actual := fieldValues(mediaType, c.field, detail)
			if !matches(c.values, actual) {
				matched = false
				break
			}
		}
		if matched {
			return r.name
		}
	}
	return ""
}

func fieldValues(mediaType medianame.MediaType, field string, detail map[string]any) []string {
	value := detail[field]
	if field == "release_year" {
		dateField := "release_date"
		if mediaType == medianame.TypeTV {
			dateField = "first_air_date"
		}
		date, _ := detail[dateField].(string)
		if len(date) < 4 {
			return nil
		}
		return []string{date[:4]}
	}
	if field == "genre_ids" && len(flatten(value, "")) == 0 {
		return flatten(detail["genres"], "id")
	}
	if field == "production_countries" {
		return flatten(value, "iso_3166_1")
	}
	return flatten(value, "")
}

func flatten(value any, objectKey string) []string {
	if value == nil {
		return nil
	}
	if object, ok := value.(map[string]any); ok {
		if objectKey == "" {
			return nil
		}
		return flatten(object[objectKey], "")
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		var result []string
		for i := 0; i < v.Len(); i++ {
			result = append(result, flatten(v.Index(i).Interface(), objectKey)...)
		}
		return result
	}
	var s string
	switch v.Kind() {
	case reflect.String:
		s = v.String()
	case reflect.Bool:
		s = strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s = strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		s = strconv.FormatFloat(v.Float(), 'f', -1, 64)
	default:
		return nil
	}
	if s == "" {
		return nil
	}
	return []string{strings.ToUpper(s)}
}

func matches(wanted, actual []string) bool {
	if len(actual) == 0 {
		return false
	}
	positive, hit := false, false
	for _, value := range wanted {
		exclude := strings.HasPrefix(value, "!")
		value = strings.TrimPrefix(value, "!")
		if !exclude {
			positive = true
		}
		for _, a := range actual {
			if valueMatches(value, a) {
				if exclude {
					return false
				}
				hit = true
			}
		}
	}
	return !positive || hit
}

func valueMatches(wanted, actual string) bool {
	if wanted == actual {
		return true
	}
	start, end, rangeValue := strings.Cut(wanted, "-")
	if !rangeValue {
		return false
	}
	min, err1 := strconv.ParseInt(start, 10, 64)
	max, err2 := strconv.ParseInt(end, 10, 64)
	value, err3 := strconv.ParseInt(actual, 10, 64)
	return err1 == nil && err2 == nil && err3 == nil && min <= value && value <= max
}

// MatchJSON 用原始 TMDB JSON 分类，保留大整数的精度。
func (p *Policy) MatchJSON(mediaType medianame.MediaType, data []byte) (string, error) {
	var detail map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&detail); err != nil {
		return "", fmt.Errorf("TMDB 分类详情 JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("TMDB 分类详情必须是单个 JSON 对象")
	}
	return p.Match(mediaType, detail), nil
}
