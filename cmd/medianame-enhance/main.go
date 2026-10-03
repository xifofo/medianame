// medianame-enhance 在保留旧 MP2 基线的前提下，验证本地增强查询的实际返回值。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xifofo/medianame"
	"github.com/xifofo/medianame/internal/shadow"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	baselinePath := flag.String("baseline", "", "已有影子测试 report.json")
	output := flag.String("output", "", "增强验证输出目录，必须与原报告不同")
	workers := flag.Int("workers", 2, "并发 1–8")
	limit := flag.Int("limit", 0, "最多测试多少条，0 为全部")
	flag.Parse()
	if *baselinePath == "" || *output == "" || *workers < 1 || *workers > 8 || *limit < 0 {
		return fmt.Errorf("需要 -baseline、-output；workers 为 1–8，limit 非负")
	}
	baselineAbs, err := filepath.Abs(*baselinePath)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if outAbs == filepath.Dir(baselineAbs) {
		return fmt.Errorf("增强报告需要独立目录，保留原始基线")
	}
	token := os.Getenv("MP2_API_TOKEN")
	if token == "" {
		return fmt.Errorf("MP2_API_TOKEN 未设置")
	}
	data, err := os.ReadFile(baselineAbs)
	if err != nil {
		return err
	}
	var baseline shadow.Report
	if err := shadow.DecodeReport(data, &baseline); err != nil {
		return err
	}
	if baseline.SchemaVersion != 1 || len(baseline.Records) == 0 {
		return fmt.Errorf("报告版本不支持或没有记录")
	}
	var selected []shadow.Record
	for _, record := range baseline.Records {
		if record.Recognition == nil {
			continue
		}
		info := medianame.ParsePath(record.Path)
		if record.Path == "" {
			info = medianame.Parse(record.Name)
		}
		if record.Recognition.Status != "identified" || len(info.CheckCandidate(shadow.Candidate(record.Recognition))) > 0 {
			selected = append(selected, record)
		}
	}
	if *limit > 0 && len(selected) > *limit {
		selected = selected[:*limit]
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := &shadow.Client{BaseURL: baseline.BaseURL, Token: token, RuleGroup: fmt.Sprintf("__medianame_enhance_%d", time.Now().UnixNano()),
		HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	if len(selected) > 0 {
		if _, err := client.ParseName(ctx, selected[0].Name); err != nil {
			return fmt.Errorf("MP2 预检查失败: %w", err)
		}
	}
	if err := os.MkdirAll(outAbs, 0700); err != nil {
		return err
	}
	result := struct {
		Baseline  string               `json:"baseline"`
		CreatedAt string               `json:"created_at"`
		Scope     string               `json:"scope"`
		Selected  int                  `json:"selected"`
		Complete  bool                 `json:"complete"`
		Summary   map[string]int       `json:"summary"`
		Records   []shadow.Enhancement `json:"records"`
	}{Baseline: baselineAbs, CreatedAt: time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339),
		Scope: "MP2 原始名称未识别或存在身份冲突的样本；本地增强查询仍使用同一个 MP2 作为数据库服务。compatible 仅表示未发现冲突，候选未经人工确认。", Selected: len(selected)}
	write := func() error {
		sort.Slice(result.Records, func(i, j int) bool { return result.Records[i].Line < result.Records[j].Line })
		result.Summary = map[string]int{}
		var unresolved strings.Builder
		for _, record := range result.Records {
			result.Summary[record.Status]++
			if record.BaselineStatus == "not_identified" && record.Status == "compatible" {
				result.Summary["baseline_failed_now_compatible"]++
			}
			if record.Status != "compatible" {
				entry, _ := json.Marshal(record)
				unresolved.Write(entry)
				unresolved.WriteByte('\n')
			}
		}
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if strings.Contains(string(encoded), token) {
			return fmt.Errorf("响应含认证信息，拒绝保存")
		}
		if err := os.WriteFile(filepath.Join(outAbs, "results.json.tmp"), encoded, 0600); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(outAbs, "results.json.tmp"), filepath.Join(outAbs, "results.json")); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outAbs, "unresolved.jsonl"), []byte(unresolved.String()), 0600); err != nil {
			return err
		}
		return shadow.WriteEnhancementHTML(outAbs, result.Records, result.Summary, result.Complete)
	}
	jobs, results := make(chan shadow.Record), make(chan shadow.Enhancement, *workers)
	var group sync.WaitGroup
	for i := 0; i < *workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for record := range jobs {
				results <- shadow.Enhance(ctx, client, record)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, record := range selected {
			select {
			case jobs <- record:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(results) }()
	fmt.Printf("测试 %d 条失败或身份有冲突的样本，并发 %d\n", len(selected), *workers)
	lastSave := time.Now()
	for record := range results {
		result.Records = append(result.Records, record)
		if len(result.Records)%25 == 0 || time.Since(lastSave) > 20*time.Second {
			if err := write(); err != nil {
				cancel()
				for range results {
				}
				return err
			}
			fmt.Printf("完成 %d/%d，无冲突候选 %d，需复核 %d，未返回 %d，请求错误 %d\n", len(result.Records), len(selected), result.Summary["compatible"], result.Summary["review"], result.Summary["no_candidate"], result.Summary["request_error"])
			lastSave = time.Now()
		}
	}
	result.Complete = len(result.Records) == len(selected) && ctx.Err() == nil
	if err := write(); err != nil {
		return err
	}
	fmt.Printf("报告：%s\n结果：%v\n", outAbs, result.Summary)
	if !result.Complete {
		return fmt.Errorf("测试中断，已保留结果")
	}
	return nil
}
