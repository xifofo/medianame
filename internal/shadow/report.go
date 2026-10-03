package shadow

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Report 保存可重放的原始数据和最终汇总；认证信息不属于报告数据。
type Report struct {
	SchemaVersion int      `json:"schema_version"`
	CreatedAt     string   `json:"created_at"`
	BaseURL       string   `json:"base_url"`
	InputFile     string   `json:"input_file"`
	InputEncoding string   `json:"input_encoding"`
	InputSHA256   string   `json:"input_sha256"`
	Scope         string   `json:"scope"`
	Complete      bool     `json:"complete"`
	Records       []Record `json:"records"`
	Summary       Summary  `json:"summary"`
}

// FieldStats 区分有值字段的一致率与两侧同时缺失。
type FieldStats struct {
	Field       string `json:"field"`
	Compared    int    `json:"compared"`
	Matched     int    `json:"matched"`
	Different   int    `json:"different"`
	BothMissing int    `json:"both_missing"`
}

// Summary 区分解析差异、在线识别失败和请求错误。
type Summary struct {
	Total             int          `json:"total"`
	UniqueNames       int          `json:"unique_names"`
	Matched           int          `json:"matched"`
	Different         int          `json:"different"`
	ParseErrors       int          `json:"parse_errors"`
	Recognized        int          `json:"recognized"`
	NotIdentified     int          `json:"not_identified"`
	RecognitionErrors int          `json:"recognition_errors"`
	PathRecovered     int          `json:"path_recovered"`
	PathNotIdentified int          `json:"path_not_identified"`
	PathErrors        int          `json:"path_errors"`
	LocalTitleEmpty   int          `json:"local_title_empty"`
	Fields            []FieldStats `json:"fields"`
}

// Summarize 基于已有记录重新计算汇总，不将接口异常计为名称识别失败。
func Summarize(records []Record) Summary {
	summary := Summary{Total: len(records)}
	names := map[string]bool{}
	fields := map[string]*FieldStats{}
	for _, field := range FieldOrder {
		fields[field] = &FieldStats{Field: field}
	}
	for _, record := range records {
		names[record.Name] = true
		switch record.Status {
		case "matched":
			summary.Matched++
		case "different":
			summary.Different++
		case "mp2_error":
			summary.ParseErrors++
		}
		if record.Local.Title == "" {
			summary.LocalTitleEmpty++
		}
		if recognition := record.Recognition; recognition != nil {
			switch recognition.Status {
			case "identified":
				summary.Recognized++
			case "not_identified":
				summary.NotIdentified++
			case "error":
				summary.RecognitionErrors++
			}
		}
		if recognition := record.PathRecognition; recognition != nil {
			switch recognition.Status {
			case "identified":
				summary.PathRecovered++
			case "not_identified":
				summary.PathNotIdentified++
			case "error":
				summary.PathErrors++
			}
		}
		if record.Status == "mp2_error" {
			continue
		}
		for _, field := range FieldOrder {
			stat := fields[field]
			left, right := record.LocalFields[field], record.MP2Fields[field]
			if left == nil && right == nil {
				stat.BothMissing++
				continue
			}
			stat.Compared++
			if reflect.DeepEqual(left, right) {
				stat.Matched++
			} else {
				stat.Different++
			}
		}
	}
	summary.UniqueNames = len(names)
	for _, field := range FieldOrder {
		summary.Fields = append(summary.Fields, *fields[field])
	}
	return summary
}

// FailureReasons 返回需要保留作为识别失败或接口异常的原因。
func FailureReasons(record Record) []string {
	var reasons []string
	if record.Local.Title == "" {
		reasons = append(reasons, "local_title_empty")
	}
	if record.Status == "mp2_error" {
		reasons = append(reasons, "mp2_parse_request_error")
	}
	if record.Recognition != nil {
		if record.Recognition.Status == "not_identified" {
			reasons = append(reasons, "mp2_name_not_identified")
		}
		if record.Recognition.Status == "error" {
			reasons = append(reasons, "mp2_name_request_error")
		}
	}
	if record.PathRecognition != nil {
		if record.PathRecognition.Status == "not_identified" {
			reasons = append(reasons, "mp2_path_not_identified")
		}
		if record.PathRecognition.Status == "error" {
			reasons = append(reasons, "mp2_path_request_error")
		}
	}
	return reasons
}

//go:embed report.html
var htmlTemplate string

// WriteReport 原子写入报告、失败样本与差异样本，并拒绝保存传入的认证信息。
func WriteReport(directory string, report *Report, credential string) error {
	report.Summary = Summarize(report.Records)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if credential != "" && bytes.Contains(data, []byte(credential)) {
		return fmt.Errorf("响应中包含认证信息，拒绝保存")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "report.json"), data); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "summary.md"), []byte(markdown(report))); err != nil {
		return err
	}
	preview := *report
	preview.Records = append([]Record(nil), report.Records...)
	for index := range preview.Records {
		r := &preview.Records[index]
		if r.Recognition != nil {
			copy := *r.Recognition
			copy.Raw = nil
			r.Recognition = &copy
		}
		if r.PathRecognition != nil {
			copy := *r.PathRecognition
			copy.Raw = nil
			r.PathRecognition = &copy
		}
	}
	jsonPreview, err := json.Marshal(preview)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "report.html"), []byte(strings.Replace(htmlTemplate, "@@REPORT_JSON@@", string(jsonPreview), 1))); err != nil {
		return err
	}
	var failures, differences bytes.Buffer
	for _, record := range report.Records {
		if reasons := FailureReasons(record); len(reasons) > 0 {
			value := struct {
				Reasons []string `json:"reasons"`
				Record  Record   `json:"record"`
			}{reasons, record}
			if err := json.NewEncoder(&failures).Encode(value); err != nil {
				return err
			}
		}
		if record.Status == "different" {
			if err := json.NewEncoder(&differences).Encode(record); err != nil {
				return err
			}
		}
	}
	if err := atomicWrite(filepath.Join(directory, "failures.jsonl"), failures.Bytes()); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(directory, "differences.jsonl"), differences.Bytes())
}

func atomicWrite(filename string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(filename), ".shadow-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, filename)
}

func markdown(report *Report) string {
	s := report.Summary
	var out strings.Builder
	fmt.Fprintf(&out, "# MP2 / medianame 影子测试\n\n时间：%s\n\n数据：`%s`（%s）\n\n后端：`%s`\n\n", report.CreatedAt, report.InputFile, report.InputEncoding, report.BaseURL)
	if !report.Complete {
		out.WriteString("**本批次尚未完成，以下仅为已完成样本。**\n\n")
	}
	fmt.Fprintf(&out, "共 %d 个文件，%d 个不同文件名。名称解析完全一致 %d，存在差异 %d，解析接口错误 %d。\n\n", s.Total, s.UniqueNames, s.Matched, s.Different, s.ParseErrors)
	fmt.Fprintf(&out, "MP2 文件名识别成功 %d，未识别 %d，请求错误 %d。未识别样本追加目录路径后识别成功 %d，仍未识别 %d，路径请求错误 %d。本地未提取片名 %d。\n\n", s.Recognized, s.NotIdentified, s.RecognitionErrors, s.PathRecovered, s.PathNotIdentified, s.PathErrors, s.LocalTitleEmpty)
	out.WriteString("名称解析采用相同文件名，比较 MP2 原始 MetaInfo。在线识别结果另存；文件名未识别时可追加目录树路径测试。后端返回媒体结果只代表识别到候选，未人工确认候选正确性。目录树中的路径不代表 MP2 能读取到真实文件。\n\n")
	out.WriteString("规则测试接口返回 `success=false` 是不存在规则组分支的预期行为；只有确认返回原始 MetaInfo、没有规则组和 MediaInfo 时才用于比对，不计为媒体识别失败。\n\n")
	out.WriteString("## 字段差异\n\n| 字段 | 有值样本 | 一致 | 不同 | 两边缺失 |\n| --- | ---: | ---: | ---: | ---: |\n")
	fields := append([]FieldStats(nil), s.Fields...)
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Different > fields[j].Different })
	for _, field := range fields {
		fmt.Fprintf(&out, "| %s | %d | %d | %d | %d |\n", field.Field, field.Compared, field.Matched, field.Different, field.BothMissing)
	}
	out.WriteString("\n## 失败样本\n\n完整样本位于 `failures.jsonl`，保留失败原因、目录树行号、本地结果、MP2 解析字段及在线识别原始响应。识别失败经目录路径恢复的样本也保留在其中。\n\n")
	shown := 0
	for _, record := range report.Records {
		if reasons := FailureReasons(record); len(reasons) > 0 {
			fmt.Fprintf(&out, "- 第 %d 行：%s — %s\n", record.Line, strings.ReplaceAll(record.Name, "\n", " "), strings.Join(reasons, ", "))
			shown++
			if shown >= 15 {
				break
			}
		}
	}
	out.WriteString("\n`report.html` 支持按失败状态、差异字段和文件名筛选；`report.json` 保存完整原始数据，可离线重放；`differences.jsonl` 保存全部解析差异。\n")
	return out.String()
}
