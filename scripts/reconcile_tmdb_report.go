// Recalculate an existing TMDB report using stored details, without API requests.
// Usage: go run ./scripts/reconcile_tmdb_report.go reports/<run>/report.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"medianame"
	"medianame/tmdb"
)

func run(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(report["records"], &records); err != nil {
		return err
	}
	previousSummary := report["summary"]
	if len(report["reconciliation"]) > 0 {
		var previous map[string]json.RawMessage
		if err := json.Unmarshal(report["reconciliation"], &previous); err != nil {
			return err
		}
		if len(previous["previous_summary"]) > 0 {
			previousSummary = previous["previous_summary"]
		}
	}
	summary := map[string]int{}
	adopted := 0
	for _, record := range records {
		var result tmdb.Result
		if err := json.Unmarshal(record["tmdb"], &result); err != nil {
			return err
		}
		result.Reconcile()
		var original map[string]json.RawMessage
		if err := json.Unmarshal(record["tmdb"], &original); err != nil {
			return err
		}
		if result.Status != tmdb.StatusRequestError {
			// 只更新选择与差异；影视详情和原始响应直接沿用保存的 JSON，
			// 避免重复解析/编码详情时改写外部 ID 的 raw 等字段。
			var candidates []map[string]json.RawMessage
			if len(original["candidates"]) > 0 {
				if err := json.Unmarshal(original["candidates"], &candidates); err != nil {
					return err
				}
			}
			if len(candidates) != len(result.Candidates) {
				return fmt.Errorf("保存的候选数与解析结果不一致")
			}
			original["media_info"] = json.RawMessage("null")
			for i, candidate := range result.Candidates {
				if candidates[i] == nil {
					candidates[i] = map[string]json.RawMessage{"media_info": json.RawMessage("null")}
				}
				for key, conflicts := range map[string][]medianame.CandidateConflict{
					"conflicts": candidate.Conflicts, "resolved_conflicts": candidate.ResolvedConflicts,
				} {
					if len(conflicts) == 0 {
						delete(candidates[i], key)
						continue
					}
					encoded, err := json.Marshal(conflicts)
					if err != nil {
						return err
					}
					candidates[i][key] = encoded
				}
				if candidate.MediaInfo != nil && candidate.MediaInfo == result.MediaInfo {
					original["media_info"] = candidates[i]["media_info"]
				}
			}
			if len(original["candidates"]) > 0 {
				original["candidates"], err = json.Marshal(candidates)
				if err != nil {
					return err
				}
			}
			original["status"], _ = json.Marshal(result.Status)
			if result.SelectionBasis == "" {
				delete(original, "selection_basis")
			} else {
				original["selection_basis"], _ = json.Marshal(result.SelectionBasis)
			}
		}
		record["tmdb"], err = json.Marshal(original)
		if err != nil {
			return err
		}
		summary[string(result.Status)]++
		if result.Status == tmdb.StatusCompatible {
			for _, candidate := range result.Candidates {
				if candidate.MediaInfo == result.MediaInfo && (len(candidate.ResolvedConflicts) > 0 || result.SelectionBasis == "tmdb_order") {
					adopted++
				}
			}
		}
	}
	backup := filepath.Join(filepath.Dir(filename), "report-before-tmdb-precedence.json")
	file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else if !os.IsExist(err) {
		return err
	}
	reconciliation, _ := json.Marshal(map[string]any{
		"policy": "tmdb_metadata", "basis": "stored_tmdb_details", "updated_at": time.Now().Format(time.RFC3339),
		"candidate_selection": "tmdb_order_for_same_name_and_year",
		"fields":              []string{"title", "original_title", "release_title", "year"},
		"adopted_tmdb":        adopted, "previous_summary": json.RawMessage(previousSummary),
	})
	report["reconciliation"] = reconciliation
	report["records"], err = json.Marshal(records)
	if err != nil {
		return err
	}
	report["summary"], _ = json.Marshal(summary)
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".tmdb-reconcile-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), filename); err != nil {
		return err
	}
	fmt.Printf("已有 TMDB 数据重算完成：%d 条已按 TMDB 采用；状态 %v\n", adopted, summary)
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./scripts/reconcile_tmdb_report.go report.json")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
