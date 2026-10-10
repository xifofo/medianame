# 重命名使用说明

`github.com/xifofo/medianame/rename` 根据变量和 Go 模板生成命名预览，返回相对路径、目录和文件名。实际创建目录、移动文件和修改文件名由调用项目执行。

- [全部 38 个重命名变量](../README.md#重命名变量)
- [分类配置与分类目录](category.md)
- [返回 README](../README.md)

## 选择入口

| 入口 | 适用场景 |
| --- | --- |
| `media.Rename(info, "")` | 已有 TMDB `MediaInfo`，按电影 / 剧集选择默认模板 |
| `media.Rename(info, format)` | 已有 TMDB `MediaInfo`，使用自己的模板 |
| `media.RenameContext(info)` | 取得完整变量表，然后覆盖或增加变量 |
| `rename.BuildContext(info, rename.Media{...})` | 离线使用，由调用方提供影视身份 |
| `rename.Render(format, context)` | 渲染一次 |
| `rename.Compile(format)` 后 `template.Render(context)` | 编译一次，批量复用，可并发渲染 |

`info` 使用 `medianame.Parse(name)` 或 `medianame.ParsePath(path)` 的结果。在线识别后使用 `result.MetaInfo` 和选定的 `result.MediaInfo`；`MediaInfo` 为空时先处理待核对、歧义或请求失败。

片名、英文名、原名、年份和主要数据库 ID 使用媒体身份优先值。季集、分辨率、编码、音轨和发布组来自文件。英文名缺失时依次使用媒体原名、最终片名。

## 可直接运行的离线示例

以下程序不需要 TMDB 凭证：

```go
package main

import (
    "fmt"

    "github.com/xifofo/medianame"
    "github.com/xifofo/medianame/rename"
)

func main() {
    info := medianame.Parse("Example.Show.2024.S01E02.1080p.WEB-DL.H264.AAC.mkv")
    variables := rename.BuildContext(info, rename.Media{
        Title: "示例剧", EnglishTitle: "Example Show", Year: 2024,
        Type: medianame.TypeTV,
        IDs: medianame.MediaIDs{TMDB: "123"}, Category: "动画番剧",
    })
    format := `{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.en_title}}.{{.year}}{{if .season_episode}}.{{.season_episode}}{{end}}{{if .videoFormat}}.{{.videoFormat}}{{end}}{{.fileExt}}`
    result, err := rename.Render(format, variables)
    if err != nil {
        panic(err)
    }
    fmt.Println(result.Path)
    fmt.Println(result.Directory)
    fmt.Println(result.Name)
}
```

输出：

```text
动画番剧/示例剧 (2024)/Example Show.2024.S01E02.1080p.mkv
动画番剧/示例剧 (2024)
Example Show.2024.S01E02.1080p.mkv
```

## 默认模板

空模板 `media.Rename(info, "")` 使用 `rename.DefaultMovieTemplate` 或 `rename.DefaultTVTemplate`。

| 类型 | 默认目录与文件内容 |
| --- | --- |
| 电影 | `片名 (年份) {tmdb-ID}/英文名.年份.来源与效果.分辨率.视频编码.音轨.平台-发布组.扩展名` |
| 剧集 | `片名 (年份) {tmdb-ID}/Season 01/英文名.年份.S01E02.来源与效果.分辨率.视频编码.音轨.平台.part2-发布组.扩展名` |

可选字段缺失时，其分隔点或发布组连字符也会省略。`fileExt` 自带点号；最后直接拼接 `{{.fileExt}}`。

示例：

```text
肮脏天使 (2024) {tmdb-1043905}/Dirty Angels.2024.WEB-DL.1080p.H.265.DDP.5.1.Amazon-404.mkv
示例剧 (2024) {tmdb-123}/Season 00/Example Show.2024.S00E01.mkv
```

默认模板要求片名、年份、TMDB ID。默认剧集模板还要求季号和集号 / 多集列表；解析剧集缺少季号时默认 `1`。缺少必要字段或遇到跨季文件时会返回错误，可提供适合该输入的自定义模板。

默认模板的目录使用片名和年份。若要分类目录前缀，在自定义模板中加入 `{{if .category}}{{.category}}/{{end}}`。

音频中的 `DDP`、`DD+`、`DD` 保留发行别名，与 MP2 的对应字段一致。从 v0.1.2 起，`audioCodec` 用点号连接编码和声道，例如 `DDP2.0` 输出 `DDP.2.0`，`DD+7.1` 输出 `DD+.7.1`，`DTS-HD MA 5.1` 输出 `DTS-HD.MA.5.1`。原本写成 `E-AC-3` 或 `AC-3` 的输入继续使用对应名称。

`edition`、`effect` 和 `resource_term` 同样用点号连接技术标签，命名时将 `Dolby Vision` 缩写为 `DV`。DV 和 HDR、HDR10、HDR10+ 可以同时保留，不互相覆盖，也不会推断输入里没有的标签。Atmos 保留在 `effect` / `edition`，不再附加到 `audioCodec`。例如下面的原名：

```text
Kingdom.of.Heaven.2005.2160p.Directors.Cut.Roadshow.Version.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.Atmos.7.1-HDHIVE.mkv
```

配合英文名 `Kingdom of Heaven`，默认文件名输出：

```text
Kingdom of Heaven.2005.REMUX.DV.HDR.Atmos.2160p.H.265.TrueHD.7.1-HDHIVE.mkv
```

以上变化只作用于命名变量，原始解析仍保留 `Effects: ["Dolby Vision", "HDR", "Atmos"]`、`AudioCodec: "TrueHD"`、`AudioChannels: "7.1"`。现有模板直接使用这些变量就会得到新格式，无须增加 `replace`；只使用 `audioCodec` 的自定义模板若要保留效果标签，需要加入 `effect` 或 `edition`。片名中的空格保持原样。

## 常用模板

### 电影按分类归档

```text
{{if .category}}{{.category}}/{{end}}{{.title_year}} {tmdb-{{.tmdbid}}}/{{.en_title}}.{{.year}}{{if .videoFormat}}.{{.videoFormat}}{{end}}{{.fileExt}}
```

示例：

```text
欧美电影/肮脏天使 (2024) {tmdb-1043905}/Dirty Angels.2024.1080p.mkv
```

### 剧集按季归档

```text
{{if .category}}{{.category}}/{{end}}{{.title_year}}/Season {{printf "%02d" (int .season)}}/{{.en_title}}.{{.year}}.{{.season_episode}}{{if .videoFormat}}.{{.videoFormat}}{{end}}{{.fileExt}}
```

示例：

```text
动画番剧/名侦探柯南 (1996)/Season 01/Detective Conan.1996.S01E1214.1080p.mkv
```

### 使用原名

```text
{{.title_year}}/{{.original_title}}{{if .season_episode}}.{{.season_episode}}{{end}}{{.fileExt}}
```

### 保留原文件名，只调整目录

```text
{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.original_name}}
```

`original_name` 从原始输入取最后一个路径段，保留扩展名。渲染时仍会转换文件名非法字符。

## 模板语法

变量使用 `{{.title}}` 这种带点号的写法，名称大小写敏感；模板采用 Go `text/template`。

| 用途 | 写法 |
| --- | --- |
| 输出变量 | `{{.title}}` |
| 字段存在时追加 | `{{if .videoFormat}}.{{.videoFormat}}{{end}}` |
| 条件分支 | `{{if .category}}{{.category}}{{else}}未分类{{end}}` |
| 季号补零 | `{{printf "%02d" (int .season)}}` |
| 替换空格 | `{{.en_title \| replace " " "."}}` |
| 转小写 / 大写 | `{{.title \| lower}}` / `{{.title \| upper}}` |
| 清理两端空格 | `{{.title \| trim}}` |
| 访问调用方补充的列表 | `{{index .aliases 0}}`（先保证列表非空） |

支持 Go 模板内置的 `if/else/with/range`、`printf`、`index` 等能力。额外函数为 `replace`、`lower`、`upper`、`trim`、`int`；`replace` 的输入位于最后，直接调用为 `{{replace " " "." .en_title}}`。

MP2 Jinja 模板迁移时，将 `{{title}}` 改为 `{{.title}}`，将 `{% if edition %}...{% endif %}` 改为 `{{if .edition}}...{{end}}`。未定义变量、未知函数、错误模板语法和失败的 `int` 转换都会返回 `error`。

## 季集与空值

| 文件线索 | `season` | `episode` | `season_episode` |
| --- | --- | --- | --- |
| `Show.E01.mkv` | `1` | `1` | `S01E01` |
| `Show.S00E00.mkv` | `0` | `0` | `S00E00` |
| `Show.S01E1214.mkv` | `1` | `1214` | `S01E1214` |
| `Show.S01E02-E05.mkv` | `1` | `"2-5"` | `S01E02-E05` |
| `Show.S01E01E03E05.mkv` | `1` | `"1,3,5"` | `S01E01E03E05` |
| `Show.S01E10-S02E03.mkv` | `1` | `"10-3"` | `S01E10-S02E03` |
| `Film.2024.mkv` | `""` | `""` | `""` |

季号和单集号 `0` 是有效数据。Go 模板的 `{{if .season}}`、`{{if .episode}}` 会把整数 `0` 当作假；判断是否存在季号使用 `{{if .season_fmt}}`，输出季集使用 `{{.season_episode}}`。需要单独判断集号时，可由调用方补充 `has_episode` 布尔变量。

文件名里的明确季号优先于目录；`ParsePath` 会在目录季号继承完成后补默认第 1 季。手动构造剧集 `Info` 时，明确 `TypeTV` 并先自行补齐季集，或使用 `client.RecognizeInfo` 应用默认季号。

`media.RenameContext(info)` 按文件季号匹配 TMDB 的 `seasons`，填充当前季的 `total_episodes` 和 `season_year`，特别季 `0` 同样可匹配。无法找到对应季时分别为 `0`、`""`；不额外请求季详情。整部作品总集数可以从 `media.NumberOfEpisodes` 加入新变量。

## 覆盖变量与增加变量

已有 `media` 和 `info` 时：

```go
variables := media.RenameContext(info)
variables["episode_title"] = "单集名称"
variables["customization"] = "收藏版"
variables["aliases"] = media.Aliases
variables["series_total_episodes"] = media.NumberOfEpisodes
variables["has_episode"] = info.Episode != nil || len(info.Episodes) > 0

result, err := rename.Render(
    `{{.title_year}}/{{.season_episode}}{{if .episode_title}} - {{.episode_title}}{{end}}{{.fileExt}}`,
    variables,
)
```

`en_name`、`customization`、`episode_title`、`episode_date` 初始为空，按调用方的数据来源填充。`RenameContext` 每次生成独立变量表，修改该表不会改写原始 `Info` 或 `MediaInfo`。

## 命令行

先在终端配置 `TMDB_API_TOKEN` 或 `TMDB_API_KEY`，从项目根目录运行：

```sh
# 默认命名预览。
go run ./cmd/medianame -tmdb -rename 'Show.2024.S01E02.[tmdb=123].mkv'

# JSON 自定义分类，并在命名模板中使用分类目录。
go run ./cmd/medianame -tmdb -adult \
  -category-config category/custom.example.json \
  -template '{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.en_title}}.{{.season_episode}}{{.fileExt}}' \
  'Color.of.Sky.Color.of.Water.2006.S01E01.1080p.BluRay.REMUX.AVC.FLAC.2.0-Misaki.mkv'
```

第一条的 `123` 为写法示例，实际联调应替换成作品的真实 TMDB ID。`-template` 隐含 `-rename`；成功时输出 `rename.path`、`rename.directory` 和 `rename.name`，命名失败输出 `rename_error`。

## 路径规则

生成的路径相对于调用方的媒体库根目录，目录分隔符使用 `/`。模板中的 `/` 创建目录层级，顶层字符串变量内的 `\ / : * ? " < > |` 转换为全角字符。调用方增加列表或对象变量并输出其中的文本时，应自行处理文本中的文件名字符。

每个目录段都必须非空，绝对路径、`.`、`..` 和控制字符会返回错误。把模板写成单行可避免换行符进入文件名。
