// medianame 将命令行中的媒体名称解析为 JSON。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"medianame"
	"medianame/category"
	"medianame/rename"
	"medianame/tmdb"
)

func main() {
	parsePath := flag.Bool("path", false, "将输入作为文件路径解析，并参考紧邻的媒体目录")
	queries := flag.Bool("queries", false, "输出解析结果及可用于媒体数据库的增强查询")
	online := flag.Bool("tmdb", false, "在线识别并返回 TMDB 影视详情；需要 TMDB_API_TOKEN 或 TMDB_API_KEY")
	language := flag.String("language", "zh-CN", "TMDB 返回信息的语言")
	tmdbURL := flag.String("tmdb-url", tmdb.DefaultBaseURL, "TMDB API 地址，包含 /3 前缀")
	includeAdult := flag.Bool("adult", false, "TMDB 名称搜索包含成人条目")
	renamePreview := flag.Bool("rename", false, "返回重命名预览，需结合 -tmdb；默认使用电影/剧集模板")
	format := flag.String("template", "", "自定义 Go text/template 重命名模板，隐含 -rename，需结合 -tmdb")
	categoryConfig := flag.String("category-config", "", "二级分类 JSON 文件；规则数组顺序即优先级，需结合 -tmdb")
	categoryFile := flag.String("category-yaml", "", "二级分类 YAML 文件；未指定时使用内置策略，需结合 -tmdb")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "用法: medianame [-path] [-queries] [-tmdb] '媒体名称或路径' [...]")
		os.Exit(2)
	}
	if (*renamePreview || *format != "" || *categoryConfig != "" || *categoryFile != "") && !*online {
		fmt.Fprintln(os.Stderr, "-rename、-template、-category-config、-category-yaml 需要结合 -tmdb 使用")
		os.Exit(2)
	}
	if *categoryConfig != "" && *categoryFile != "" {
		fmt.Fprintln(os.Stderr, "-category-config 和 -category-yaml 只能指定一个")
		os.Exit(2)
	}
	if *format != "" {
		*renamePreview = true
		if _, err := rename.Compile(*format); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	var policy *category.Policy
	policyPath := *categoryConfig
	if policyPath == "" {
		policyPath = *categoryFile
	}
	if policyPath != "" {
		data, err := os.ReadFile(policyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if *categoryConfig != "" {
			policy, err = category.ParseJSON(data)
		} else {
			policy, err = category.Parse(data)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var client *tmdb.Client
	if *online {
		var err error
		client, err = tmdb.NewClient(tmdb.Config{Token: os.Getenv("TMDB_API_TOKEN"), APIKey: os.Getenv("TMDB_API_KEY"),
			BaseURL: *tmdbURL, Language: *language, IncludeAdult: *includeAdult, CategoryPolicy: policy})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	failed := false
	for _, name := range flag.Args() {
		var info medianame.Info
		if *parsePath {
			info = medianame.ParsePath(name)
		} else {
			info = medianame.Parse(name)
		}
		var output any = info
		if *queries {
			output = struct {
				Info    medianame.Info          `json:"info"`
				Queries []medianame.SearchQuery `json:"queries"`
			}{info, info.SearchQueries()}
		}
		if client != nil {
			result, err := client.RecognizeInfo(ctx, info)
			response := struct {
				*tmdb.Result
				Queries     []medianame.SearchQuery `json:"queries,omitempty"`
				Error       string                  `json:"error,omitempty"`
				Rename      *rename.Result          `json:"rename,omitempty"`
				RenameError string                  `json:"rename_error,omitempty"`
			}{Result: result}
			if *queries {
				response.Queries = info.SearchQueries()
			}
			if err != nil {
				failed, response.Error = true, err.Error()
			}
			if *renamePreview && result != nil && result.MediaInfo != nil && err == nil {
				response.Rename, err = result.MediaInfo.Rename(info, *format)
				if err != nil {
					failed, response.RenameError = true, err.Error()
				}
			}
			output = response
		}
		if err := encoder.Encode(output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if failed {
		os.Exit(1)
	}
}
