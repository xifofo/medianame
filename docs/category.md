# 分类使用说明

分类根据 TMDB 的影视类型、类型标签、语言、国家和其他详情字段计算，结果返回在 `media_info.category`。分类名称、匹配条件、规则顺序和兜底类别均可自定义。

- [可直接修改的完整 JSON 配置](../category/custom.example.json)
- [重命名模板与分类目录](renaming.md)
- [返回 README](../README.md#二级分类)

## 选择配置方式

| 方式 | 加载入口 | 适用场景 |
| --- | --- | --- |
| Go 对象 | `category.New(category.Config{...})` | 调用项目直接构造配置、从设置页面或数据库加载规则 |
| JSON 文件 | `category.ParseJSON(data)` | 保存和交换配置；命令行使用 `-category-config 文件.json` |
| YAML | `category.Parse(data)` | 兼容现有配置；命令行使用 `-category-yaml 文件.yaml` |
| 内置默认配置 | `category.Default()` 或不传 `CategoryPolicy` | 使用包提供的默认分类 |

Go 对象和 JSON 使用相同结构：电影规则在 `movie`，剧集规则在 `tv`，每个分支都是有序数组。JSON 对象内字段的排列不影响判断，规则数组的排列决定优先级。

## JSON 配置

```json
{
  "movie": [
    {"name": "动画电影", "conditions": {"genre_ids": [16]}},
    {"name": "华语电影", "conditions": {"original_language": ["zh", "cn", "bo", "za"]}},
    {"name": "其他电影"}
  ],
  "tv": [
    {"name": "成人动画", "conditions": {"adult": true, "genre_ids": [16]}},
    {"name": "动画番剧", "conditions": {"genre_ids": [16]}},
    {"name": "国产剧集", "conditions": {"origin_country": ["CN", "TW", "HK"]}},
    {"name": "其他剧集"}
  ]
}
```

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `movie` / `tv` | 规则数组 | 该影视类型的全部分类规则 |
| `name` | 非空字符串 | 命中后返回的分类名称；同一类型内不能重复 |
| `conditions` | 对象，可省略 | TMDB 字段名对应条件值；多个字段全部满足时才命中 |

规则从上到下执行，采用第一条命中规则。省略 `conditions` 或写成 `{}` 就是无条件兜底，应放在对应数组末尾。

上例中，`tv` 作品同时具有 `adult: true` 和动画标签 `16` 时归到“成人动画”；其他动画归到“动画番剧”。先匹配动画，再匹配出品地区，所以日本动画仍优先归到动画分类。

自定义配置完整定义自己的分类规则。只传 `tv` 就只分类剧集；电影分类为空。需要保留默认类别时，从 [完整示例](../category/custom.example.json) 复制全部规则后修改。

## 条件语法

条件值可用字符串、数字、布尔值或这些值的列表。字符串比较不区分大小写，字段名使用 TMDB / 本包的实际名称。多个字段是 AND；单个字段的多个允许值是 OR。

| 条件 | 示例 | 含义 |
| --- | --- | --- |
| 单值 | `"adult": true` | 成人标记为真 |
| 多值 | `"origin_country": ["CN", "TW", "HK"]` | 出品地区任一值在列表中 |
| 逗号字符串 | `"origin_country": "CN,TW,HK"` | 与上面的列表等价 |
| 排除值 | `"origin_country": "!US"` | 出品地区存在，且没有 `US` |
| 整数范围 | `"release_year": "2020-2026"` | 年份位于闭区间内 |
| 范围加排除 | `"release_year": ["2020-2026", "!2022"]` | 2020 至 2026 年，排除 2022 年 |

排除条件遇到命中值时优先排除；例如实际出品地区同时包含 `US` 和 `JP`，`"!US"` 不命中。缺失字段不满足任何条件，包括只有排除值的条件。整数范围支持整数值，不支持小数区间。

空条件对象是兜底；条件值 `null`、空列表、空字符串、嵌套对象或嵌套列表会作为无效配置返回错误。JSON 加载还会拒绝未知的配置结构字段和多份连续 JSON 文档。

### 常用字段

| 条件字段 | 来源 | 示例 |
| --- | --- | --- |
| `genre_ids` | TMDB 类型 ID 列表；也可从 `genres[].id` 提取 | `[16]` 为动画，`[10764, 10767]` 用于默认综艺规则 |
| `adult` | TMDB 成人标记 | `true` / `false` |
| `original_language` | TMDB 原始语言 | `["zh", "ja", "en"]` |
| `origin_country` | TMDB 剧集出品地区 | `["CN", "TW", "HK"]` |
| `production_countries` | TMDB 制片国家，提取 `iso_3166_1` | `["CN", "US"]` |
| `release_year` | 电影 `release_date` 或剧集 `first_air_date` 的年份 | `"2020-2026"` |
| `type` | TMDB 剧集形态，在线分类保留原始 `type` | `"Miniseries"`、`"Scripted"` |
| `status` | TMDB 影视状态 | `"Released"` |
| `vote_count` | TMDB 评分人数 | `"100-1000000"` |

还可匹配 TMDB 完整详情中的其他一级标量或标量列表字段。对象数组只有已定义的映射（如 `genres`、`production_countries`）才会提取对象成员；不会自动解析任意嵌套字段路径。

顶层 `movie` / `tv` 选择影视分支；剧集条件中的 `type: "Miniseries"` 等匹配 TMDB 的剧集形态。

## Go 对象配置

在调用项目中直接构造规则，然后把编译后的策略传给 TMDB Client：

```go
policy, err := category.New(category.Config{
    Movie: []category.Rule{
        {Name: "动画电影", Conditions: map[string]any{"genre_ids": []int{16}}},
        {Name: "其他电影"},
    },
    TV: []category.Rule{
        {Name: "成人动画", Conditions: map[string]any{"adult": true, "genre_ids": []int{16}}},
        {Name: "动画番剧", Conditions: map[string]any{"genre_ids": []int{16}}},
        {Name: "其他剧集"},
    },
})
if err != nil {
    return // 处理规则配置错误。
}
client, err := tmdb.NewClient(tmdb.Config{
    Token: os.Getenv("TMDB_API_TOKEN"),
    IncludeAdult: true,
    CategoryPolicy: policy,
})
```

分类规则编译后只读，可并发使用。修改原始 `Config` 的切片或映射不会改变已编译策略。设置页面保存新规则后，重新调用 `category.New` / `ParseJSON` 并创建使用新策略的 Client；已有影视详情可以按下文重新分类。

## 从 JSON 文件加载并识别

从项目根目录运行以下完整程序，凭证由终端环境提供：

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/xifofo/medianame/category"
    "github.com/xifofo/medianame/tmdb"
)

func main() {
    data, err := os.ReadFile("category/custom.example.json")
    if err != nil {
        panic(err)
    }
    policy, err := category.ParseJSON(data)
    if err != nil {
        panic(err)
    }
    client, err := tmdb.NewClient(tmdb.Config{
        Token: os.Getenv("TMDB_API_TOKEN"),
        IncludeAdult: true,
        CategoryPolicy: policy,
    })
    if err != nil {
        panic(err)
    }
    result, err := client.Recognize(context.Background(),
        "Color.of.Sky.Color.of.Water.2006.S01E01.1080p.BluRay.REMUX.AVC.FLAC.2.0-Misaki.mkv",
    )
    if err != nil {
        panic(err)
    }
    if result.MediaInfo == nil {
        fmt.Println("需要处理识别状态：", result.Status)
        return
    }
    fmt.Println(result.MediaInfo.Category)
    named, err := result.MediaInfo.Rename(result.MetaInfo,
        `{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.en_title}}.{{.season_episode}}{{.fileExt}}`,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(named.Path)
}
```

对于 `adult: true`、类型标签 `16` 的上述作品，这份示例配置返回“成人动画”，并得到下面的命名预览：

```text
成人动画
成人动画/そらのいろ、みずのいろ (2006)/Color of Sky, Color of Water.S01E01.mkv
```

`client.Details` 和 `Recognize` / `RecognizePath` / `RecognizeInfo` 取得的完整候选都会自动分类，详情中的 `Category` 可直接传到重命名模板变量 `category`。

## 成人搜索与成人分类

搜索成人条目设置 `tmdb.Config.IncludeAdult: true`，命令行使用 `-adult`；它决定名称搜索是否包含这些条目。`conditions.adult` 决定取得详情后归到哪个分类。

默认搜索不包含成人条目。已经知道作品 TMDB ID 和类型时，可以直接用 `Details` 获取详情并分类。

## 已有详情重新分类

已有 `media` 时，无须再请求 TMDB：

```go
name, err := media.Classify(policy)
if err != nil {
    return
}
media.Category = name
named, err := media.Rename(info,
    `{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.original_name}}`,
)
```

`Classify` 返回分类名称；把结果赋给 `media.Category` 后，新的命名预览使用新分类。直接对 TMDB 原始详情 JSON 分类可以使用 `policy.MatchJSON(mediaType, rawDetails)`。

## 分类目录与命名模板

分类返回值写入 `media_info.category`。在模板中使用下面的前缀，让有分类的文件进入分类目录：

```text
{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.original_name}}
```

分类为空时前缀省略，可避免出现空目录段。模板中的 `/` 定义目录层级；分类名称内的 `/` 渲染时转换成全角，保持分类名称为单个目录段。

默认重命名模板使用片名目录；需要分类目录时传入包含上述前缀的自定义模板。模板变量和更多写法见 [重命名文档](renaming.md)。

## 命令行

从项目根目录运行，凭证使用 `TMDB_API_TOKEN` 或 `TMDB_API_KEY`：

```sh
# 自定义 JSON 分类，返回的 media_info.category 即匹配结果。
go run ./cmd/medianame -tmdb -adult \
  -category-config category/custom.example.json \
  'Color.of.Sky.Color.of.Water.2006.S01E01.1080p.BluRay.REMUX.AVC.FLAC.2.0-Misaki.mkv'

# 同时预览分类目录和文件名。
go run ./cmd/medianame -tmdb -adult \
  -category-config category/custom.example.json \
  -template '{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.en_title}}.{{.season_episode}}{{.fileExt}}' \
  'Color.of.Sky.Color.of.Water.2006.S01E01.1080p.BluRay.REMUX.AVC.FLAC.2.0-Misaki.mkv'
```

`-category-config` 和 `-category-yaml` 只指定一个，均需结合 `-tmdb`。配置无效时命令以状态码 `2` 退出；身份歧义时仍返回识别状态与候选，并不因为分类规则而强行选定作品。

## 默认分类与关闭分类

内置配置位于 [category/default.yaml](../category/default.yaml)，同一分支内按下表顺序匹配。

| 影视类型 | 分类 | 条件 |
| --- | --- | --- |
| 电影 | 动画电影 | `genre_ids` 包含 `16` |
| 电影 | 华语电影 | `original_language` 为 `zh,cn,bo,za` |
| 电影 | 欧美电影 | `original_language` 为 `en,de,fr,nl,ru,es,el,da` |
| 电影 | 日韩电影 | `original_language` 为 `ja,ko` |
| 电影 | 外语电影 | 兜底 |
| 剧集 | 动画番剧 | `genre_ids` 包含 `16` |
| 剧集 | 综艺节目 | `genre_ids` 包含 `10764` 或 `10767` |
| 剧集 | 国产剧集 | `origin_country` 包含 `CN,TW,HK` 中任一项 |
| 剧集 | 欧美剧集 | `origin_country` 包含 `US,FR,GB,DE,ES,IT,NL,PT,RU,UK` 中任一项 |
| 剧集 | 日韩剧集 | `origin_country` 包含 `JP,KP,KR,TH,IN,SG` 中任一项 |
| 剧集 | 其他剧集 | 兜底 |

`CategoryPolicy` 为 `nil` 时使用默认配置。关闭分类需传入空策略：

```go
policy, err := category.New(category.Config{})
if err != nil {
    return
}
client, err := tmdb.NewClient(tmdb.Config{
    Token: os.Getenv("TMDB_API_TOKEN"), CategoryPolicy: policy,
})
```

JSON 文件内容 `{}` 同样表示关闭分类。未命中规则、没有对应类型规则或关闭分类时，分类值为 `""`，JSON 中的 `category` 字段会省略。

## YAML 兼容入口

```yaml
tv:
  成人动画:
    adult: true
    genre_ids: '16'
  动画番剧:
    genre_ids: '16'
  其他剧集:
```

通过 `category.Parse(data)` 或 `-category-yaml 文件.yaml` 加载，按 YAML 中分类的顺序判断；一级键还支持 `电影` / `电视剧` 中文别名。其条件语义与 Go / JSON 配置一致。
