package category

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/xifofo/medianame"
)

// Config 用数组保存各类型的分类规则，数组顺序即匹配优先级。
// 空配置关闭分类；每种类型的规则分别生效。
type Config struct {
	Movie []Rule `json:"movie,omitempty"`
	TV    []Rule `json:"tv,omitempty"`
}

// Rule 定义分类名称及条件。条件之间为 AND，单个条件的列表值为 OR。
// 值支持字符串、数字、布尔值及这些值的列表；字符串也支持逗号值、! 排除和数字范围。
// Conditions 为空时作为兜底，通常放在所属数组末尾。
type Rule struct {
	Name       string         `json:"name"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

// New 编译调用方提供的结构化配置。编译结果独立于 Config 中的切片和映射。
func New(config Config) (*Policy, error) {
	p := &Policy{rules: make(map[medianame.MediaType][]rule)}
	for _, group := range []struct {
		mediaType medianame.MediaType
		rules     []Rule
	}{{medianame.TypeMovie, config.Movie}, {medianame.TypeTV, config.TV}} {
		seen := map[string]bool{}
		for i, input := range group.rules {
			if strings.TrimSpace(input.Name) == "" {
				return nil, fmt.Errorf("分类 %s 第 %d 项: 分类名称不能为空", group.mediaType, i+1)
			}
			if seen[input.Name] {
				return nil, fmt.Errorf("分类 %s 第 %d 项: 分类名称重复", group.mediaType, i+1)
			}
			seen[input.Name] = true
			r := rule{name: input.Name}
			fields := make([]string, 0, len(input.Conditions))
			for field := range input.Conditions {
				fields = append(fields, field)
			}
			sort.Strings(fields)
			for _, field := range fields {
				if strings.TrimSpace(field) == "" {
					return nil, fmt.Errorf("分类 %s 第 %d 项: 条件字段不能为空", group.mediaType, i+1)
				}
				values, err := configValues(input.Conditions[field])
				if err != nil {
					return nil, fmt.Errorf("分类 %s 第 %d 项条件 %s: %w", group.mediaType, i+1, field, err)
				}
				r.conditions = append(r.conditions, condition{field: field, values: values})
			}
			p.rules[group.mediaType] = append(p.rules[group.mediaType], r)
		}
	}
	return p, nil
}

func configValues(value any) ([]string, error) {
	var values []string
	appendScalar := func(value any) error {
		if value == nil {
			return fmt.Errorf("条件值不能为 null")
		}
		v := reflect.ValueOf(value)
		if (v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64) && (math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0)) {
			return fmt.Errorf("条件数字必须是有限值")
		}
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			return fmt.Errorf("条件值必须是字符串、数字、布尔值或这些值的列表")
		}
		scalar := flatten(value, "")
		if len(scalar) != 1 {
			return fmt.Errorf("条件值必须是字符串、数字、布尔值或这些值的列表")
		}
		for _, part := range strings.Split(scalar[0], ",") {
			part = strings.TrimSpace(part)
			if part == "" || part == "!" {
				return fmt.Errorf("条件值不能为空")
			}
			values = append(values, part)
		}
		return nil
	}
	if value != nil && (reflect.TypeOf(value).Kind() == reflect.Slice || reflect.TypeOf(value).Kind() == reflect.Array) {
		v := reflect.ValueOf(value)
		for i := 0; i < v.Len(); i++ {
			if err := appendScalar(v.Index(i).Interface()); err != nil {
				return nil, err
			}
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("条件值不能为空")
		}
	} else if err := appendScalar(value); err != nil {
		return nil, err
	}
	return values, nil
}

// ParseJSON 加载 Config 格式的 JSON，保留规则数组顺序和数字精度。
// 未知配置字段、无效条件和多份 JSON 文档会返回错误。
func ParseJSON(data []byte) (*Policy, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return nil, fmt.Errorf("分类 JSON 顶层必须是对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("分类 JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("分类 JSON 必须是单个对象")
	}
	return New(config)
}
