// medianame-shadow 对目录树样本执行 MP2 影子测试，保留原始响应与失败记录。
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"medianame/internal/shadow"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	tree := flag.String("tree", "", "目录树文件（UTF-8 或 UTF-16）")
	base := flag.String("base-url", os.Getenv("MP2_BASE_URL"), "MP2 服务地址")
	tokenEnv := flag.String("token-env", "MP2_API_TOKEN", "保存 API Token 的环境变量名")
	output := flag.String("output", "", "报告输出目录")
	replay := flag.String("replay", "", "离线重放已有 report.json，重新运行本地解析")
	limit := flag.Int("limit", 0, "样本上限，0 为全部")
	workers := flag.Int("workers", 2, "并发请求工作线程数（1–8）")
	timeout := flag.Duration("timeout", 30*time.Second, "每个 HTTP 请求的超时")
	recognize := flag.Bool("recognize", false, "追加 MP2 在线媒体识别，记录未识别样本")
	pathFallback := flag.Bool("path-fallback", false, "文件名未识别时追加目录树路径识别测试")
	flag.Parse()
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600))
	if *output == "" {
		*output = filepath.Join("reports", "mp2-shadow-"+now.Format("20060102-150405"))
	}
	if *replay != "" {
		return replayReport(*replay, *output)
	}
	if *tree == "" || *base == "" {
		return errors.New("需要 -tree 和 -base-url；可通过 MP2_BASE_URL 设置服务地址")
	}
	if *workers < 1 || *workers > 8 || *limit < 0 || *timeout <= 0 {
		return errors.New("workers 需要 1–8，limit 需要非负，timeout 需要为正")
	}
	if *pathFallback && !*recognize {
		return errors.New("-path-fallback 需要同时启用 -recognize")
	}
	token := os.Getenv(*tokenEnv)
	if token == "" {
		return fmt.Errorf("环境变量 %s 未设置", *tokenEnv)
	}
	data, err := os.ReadFile(*tree)
	if err != nil {
		return err
	}
	inputs, encoding, err := shadow.ReadTree(data)
	if err != nil {
		return err
	}
	if *limit > 0 && *limit < len(inputs) {
		inputs = inputs[:*limit]
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	client := &shadow.Client{BaseURL: *base, Token: token, RuleGroup: "__medianame_shadow_" + hex.EncodeToString(nonce[:]),
		HTTP: &http.Client{Timeout: *timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// 先确认认证和纯解析分支，避免对整批样本重复发送无法完成的请求。
	first, err := client.ParseName(ctx, inputs[0].Name)
	if err != nil {
		return fmt.Errorf("MP2 预检查失败: %w", err)
	}
	sum := sha256.Sum256(data)
	inputPath, err := filepath.Abs(*tree)
	if err != nil {
		return err
	}
	report := &shadow.Report{SchemaVersion: 1, CreatedAt: now.Format(time.RFC3339), BaseURL: *base, InputFile: inputPath,
		InputEncoding: encoding, InputSHA256: hex.EncodeToString(sum[:]), Scope: "filename_parse", Records: []shadow.Record{}}
	if *recognize {
		report.Scope = "filename_parse_and_mp2_identification"
	}
	fmt.Printf("导入 %d 个视频样本，%s；并发 %d，在线识别 %v，路径补充 %v\n", len(inputs), encoding, *workers, *recognize, *pathFallback)
	type completed struct {
		index  int
		record shadow.Record
	}
	jobs, results := make(chan int), make(chan completed, *workers)
	var group sync.WaitGroup
	for worker := 0; worker < *workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				input := inputs[index]
				remote, parseErr := first, error(nil)
				if index != 0 {
					remote, parseErr = client.ParseName(ctx, input.Name)
				}
				record := shadow.Compare(input, remote, parseErr)
				if *recognize && ctx.Err() == nil {
					record.Recognition = client.Recognize(ctx, input.Name, false)
					if *pathFallback && record.Recognition.Status == "not_identified" && ctx.Err() == nil {
						record.PathRecognition = client.Recognize(ctx, input.Path, true)
					}
				}
				results <- completed{index, record}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range inputs {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(results) }()
	done := map[int]shadow.Record{}
	lastSave := time.Now()
	for result := range results {
		done[result.index] = result.record
		if len(done)%25 == 0 || time.Since(lastSave) >= 20*time.Second {
			report.Records = ordered(done)
			if err := shadow.WriteReport(*output, report, token); err != nil {
				stop()
				for range results {
				}
				return err
			}
			lastSave = time.Now()
			s := report.Summary
			fmt.Printf("完成 %d/%d：解析一致 %d，差异 %d，文件名未识别 %d，目录恢复 %d，仍未识别 %d，识别请求错误 %d\n",
				len(done), len(inputs), s.Matched, s.Different, s.NotIdentified, s.PathRecovered, s.PathNotIdentified, s.RecognitionErrors+s.PathErrors)
		}
	}
	report.Records, report.Complete = ordered(done), len(done) == len(inputs) && ctx.Err() == nil
	if err := shadow.WriteReport(*output, report, token); err != nil {
		return err
	}
	absOutput, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	s := report.Summary
	fmt.Printf("报告：%s\n样本 %d，一致 %d，差异 %d，解析错误 %d；名称识别成功 %d，未识别 %d，识别错误 %d；目录恢复 %d，仍失败 %d，路径错误 %d\n",
		absOutput, s.Total, s.Matched, s.Different, s.ParseErrors, s.Recognized, s.NotIdentified, s.RecognitionErrors, s.PathRecovered, s.PathNotIdentified, s.PathErrors)
	if !report.Complete {
		return errors.New("批次中断，已完成样本已保存，报告标记为未完成")
	}
	return nil
}

func ordered(records map[int]shadow.Record) []shadow.Record {
	keys := make([]int, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	result := make([]shadow.Record, 0, len(keys))
	for _, key := range keys {
		result = append(result, records[key])
	}
	return result
}

func replayReport(filename, output string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var report shadow.Report
	if err := shadow.DecodeReport(data, &report); err != nil {
		return err
	}
	if report.SchemaVersion != 1 || len(report.Records) == 0 {
		return errors.New("报告版本不支持或没有样本")
	}
	for index, old := range report.Records {
		var oldErr error
		if old.Status == "mp2_error" {
			oldErr = errors.New(old.Error)
		}
		record := shadow.Compare(old.Input, old.MP2, oldErr)
		record.Recognition, record.PathRecognition = old.Recognition, old.PathRecognition
		report.Records[index] = record
	}
	if err := shadow.WriteReport(output, &report, ""); err != nil {
		return err
	}
	fmt.Printf("离线重放完成：%d 个样本，一致 %d，差异 %d；在线结果沿用原批次，未发送请求\n", report.Summary.Total, report.Summary.Matched, report.Summary.Different)
	return nil
}
