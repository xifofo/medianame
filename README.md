# medianame

从媒体文件名、发布标题或文件路径提取视频信息的 Go 包。参考 MoviePilot 的名称解析场景，以 Go 标准库独立实现，适用于电影、电视剧和动漫名称。

需要 Go 1.22 或更高版本。名称解析和重命名使用 Go 标准库；YAML 分类策略使用 `gopkg.in/yaml.v3`。

文档入口：

- [重命名变量表](#重命名变量)：全部变量的类型、来源、示例和空值。
- [重命名使用文档](docs/renaming.md)：默认模板、自定义模板、季集处理、变量覆盖和命令行。
- [分类使用文档](docs/category.md)：JSON / Go 配置、规则优先级、条件、默认分类和分类目录。

## 使用

在使用本包的 Go 项目中安装：

```bash
go get github.com/xifofo/medianame@latest
```

```go
import "github.com/xifofo/medianame"

info := medianame.Parse("Breaking.Bad.S01E02.1080p.WEB-DL.H264.mkv")
// info.Title      == "Breaking Bad"
// *info.Season    == 1
// *info.Episode   == 2
// info.Resolution == "1080p"
// info.Source     == "WEB-DL"
// info.VideoCodec == "H.264"
```

`Parse` 接受名称，`ParsePath` 接受路径。两者都不读取文件、不访问网络，可以并发调用。

```go
info := medianame.ParsePath(
    "/tv/Example Show (2024) [tmdb=123]/Season 0/01.mkv",
)
// info.Title   == "Example Show"
// info.Year    == 2024
// *info.Season == 0
// *info.Episode == 1
// info.IDs.TMDB == "123"
```

季集字段使用 `*int`：剧集缺少季号时 `Season` 默认为 `1`；明确标注的第 0 季或第 0 集仍返回 `0`。`ParsePath` 会先合并文件名与目录中的季号，再补默认值，文件名中的明确季号优先。电影及未知类型不补季号；其余缺失字段为 `nil`，JSON 中会省略。

模块路径为 `github.com/xifofo/medianame`，包名仍为 `medianame`。在线识别、重命名和分类分别导入该路径下的 `tmdb`、`rename`、`category` 子包。

## 支持的信息

| 字段 | 说明 |
| --- | --- |
| `Original`、`Title`、`Aliases` | 原始输入、清理后的片名和同年份发行别名；保留大小写、版本括号、片名内部连字符 |
| `Type`、`Year` | `movie`（电影）或 `tv`（剧集）与年份；无片名、季集及类型标签的输入才返回 `unknown` |
| `Season`、`SeasonEnd` | 季号、季范围，包括特别季 `S00`；剧集缺失季号默认 `1` |
| `Episode`、`EpisodeEnd` | 集号、集范围；同一季中的倒序范围会交换两端 |
| `Episodes` | `S01E01E03E05` 这种多集列表，不将未出现的集号补成连续范围 |
| `EpisodeVersion` | 动漫重发版本，如 `03v2` 中的 `2` |
| `EpisodeGroup` | MoviePilot 复合标签中的 `g` |
| `Resolution` | `1080p`、`1080i`、`1920x1080`、`4K` 等；`4K` 统一为 `2160p` |
| `Source` | `WEB-DL`、`WEBRip`、`BluRay`、`HDTV`、`REMUX` 等 |
| `VideoCodec` | `H264`、`x264`、`AVC` 统一为 `H.264`；`H265`、`x265`、`HEVC` 统一为 `H.265` |
| `AudioCodec`、`AudioChannels` | 如 `DTS-HD MA` / `5.1`、`TrueHD` / `7.1`；`DDP`、`DD+`、`DD` 对齐 MP2 保留发行别名，声道独立存储 |
| `Effects` | HDR、HDR10+、Dolby Vision、HDR Vivid、HLG、Atmos、IMAX、3D 等标记 |
| `BitDepth`、`FPS` | 位深与帧率，支持 `10bit`、`Hi10P`、`yuv420p10`、`23.976fps` 等 |
| `ReleaseGroup` | 常见前置字幕组或资源信息后的 `-发布组` 尾缀，支持纯数字 `-404`；避免将集范围、编码尾缀、CRC 校验码当成发布组 |
| `StreamingService` | Amazon、Netflix、Disney+、Apple TV+ 等名称标记 |
| `Part`、`Extension` | 数字介质分卷与已知媒体/字幕扩展名；不会把 `.x265` 当扩展名 |
| `IDs` | TMDB、TVDB、IMDb、豆瓣、Bangumi、AniList 的显式 ID；使用字符串保留长数字 |
| `Warnings` | 矛盾的输入线索，例如整理前缀与发行名称年份不同 |

支持的季集写法包括：

```text
Show.S01E02
Show.S01E02-E05
Show.S01E02-05
Show.S01E02-S02E03
Show.S01-S03
Show.Season.2.Episode.4
Show.2x04
Show.EP14
示例剧 第二季 第十二集
示例剧 第12集-第14集
[MoonGroup] Sample Anime - 07v2 [1080p][HEVC]
【月光字幕组】[示例动漫 第二季][11][1080p]
[SBSUB][CONAN][1214][WEBRIP][1080P][HEVC_AAC]
```

动漫方括号及横杠集号支持四位数、范围和重发版本，例如 `[1214]`、`[1214-1216v2]`；无季集前缀的发行年份（如 `[2024]`）保留为年份。示例解析为 `CONAN`、`tv`、默认第 1 季、第 1214 集、发布组 `SBSUB`，不凭短名离线断定影视身份。

支持 `[tmdb=123]`、`[tmdbid-123]`、`{tmdb-123}` 等标签及对应数据库别名，也支持 MoviePilot 复合格式：

```text
Show {[tmdbid=123;type=tv;s=0;e=1-3;g=abc123]}
Film [imdb-tt1234567]
```

同一名称内重复出现的 ID 使用最后一个有效值。复合标签中的季集优先于普通名称中的季集，显式 `type` 优先于季集推断。

TMDB、TVDB 和 IMDb 的文件名及媒体目录名均支持 Emby 的 8 种组合：`[]` 或 `{}`、数据库名带或不带 `id` 后缀、`=` 或 `-` 分隔。IMDb 使用 `tt` 前缀，例如 `[imdbid=tt0073589]` 或 `{imdb-tt0073589}`。路径解析时，文件里的 ID 优先；只从同名媒体目录或季目录所属的媒体目录补充缺失 ID。

支持全角花括号 `｛tmdbid-123｝`。单个有效 ID 标签若把右花括号误写成圆括号，例如 `｛tmdbid-67018）`，也会恢复该 ID；普通括号文字、无效 ID 和括号错误的复合标签不会据此推测 ID。输入原文始终保留在 `Original`。

类型规则：显式 `type=movie` / `type=tv` 优先；名称或路径有季集线索时返回 `tv`，其余有片名的输入默认返回 `movie`。例如 `Film.2024.mkv` 和 `1917.mkv` 是电影，`Show.S01E02.mkv`、`Show/Season 2/Show.mkv` 是剧集。默认电影是名称解析策略；缺少季集及类型标签的剧名也会按电影处理，可用 `type=tv` 指定。空名称或只有 `1080p` 等技术信息时仍返回 `unknown`。

`ParsePath` 支持 POSIX 和 Windows 分隔符。文件字段优先，可跳过一个纯季目录继承媒体目录信息。有自身片名的文件只从同名目录补充信息，以减少合集目录的错误继承。只有路径入口会将 `01.mkv` 这样的纯数字文件名解释为集号。

对于 `电影目录/BDMV/STREAM/00005.m2ts`，会识别蓝光结构并继承电影目录的片名、年份和 ID，分片编号不会变成片名或集号。普通 `STREAM` 目录或名为《Stream》的电影不会触发该规则。蓝光介质本身不决定电影/剧集类型。

## 增强名称与查询候选

已整理名称 `片名 (年份) - 原发行名` 优先采用前缀身份，后方的重复标题和数字片名不会覆盖它；两侧年份不同会在 `Warnings` 中保留 `conflicting_years`，用于提示复核。片名中的版本括号如 `咒怨(美版)`、`嘲笑鸟(上)` 会保留，技术字段支持 `TrueHD Dolby Atmos 7.1` 和 `BD720P/BD1080P`。

```go
info := medianame.Parse("肮脏天使 (2024) - Dirty.Angels.2024.1080p.x264-404.mkv")
queries := info.SearchQueries()
// queries[0].Text == "肮脏天使.2024 {[type=movie]}"
// info.Type == medianame.TypeMovie
// info.ReleaseGroup == "404"
// info.Episode == nil

disc := medianame.ParsePath("/movies/哈利叔叔的不寻常的韵事 (1945)/BDMV/STREAM/00005.m2ts")
// disc.Title == "哈利叔叔的不寻常的韵事"
// disc.Type == medianame.TypeMovie
// disc.SearchQueries()[0].Text == "哈利叔叔的不寻常的韵事.1945 {[type=movie]}"
```

`SearchQueries()` 返回纯本地查询条件：名称、年份、类型、显式数据库 ID，以及可提交给 MP2 等服务的清理后文本。双语名称及同年份发行别名分别生成查询，例如 `纵情女郎 (2006) - [工厂女孩].Factory.Girl.2006...` 可产生 `纵情女郎.2006 {[type=movie]}`、`工厂女孩.2006 {[type=movie]}`、`Factory Girl.2006 {[type=movie]}`。不同年份的发行名称不会被合并为别名。根包不访问媒体数据库；可选的 `github.com/xifofo/medianame/tmdb` 子包提供在线识别。调用方取得在线候选后，可用 `CheckCandidate()` 检查身份：

```go
conflicts := medianame.Parse("[咒怨(美版) 2004][原盘].mkv").CheckCandidate(
    medianame.MediaCandidate{Title: "咒怨", Year: 2002, Type: medianame.TypeMovie},
)
// 包含年份、标题冲突，调用方应转入待核对，不能直接采纳。
```

检查会保留输入年份矛盾，并对候选缺失或不同的年份、解析类型、ID、标题给出冲突证据。TMDB ID 必须结合类型理解。空冲突列表只表示没有发现矛盾，不等于媒体内容已核实；无法离线确认的译名/别名也可能需要复核。

## TMDB 在线识别与影视详情

`github.com/xifofo/medianame/tmdb` 直接访问 TMDB，不依赖 MP2。它先用本包解析名称或路径，再搜索影视候选、取得完整详情并检查身份。`Parse`、`ParsePath` 的离线行为保持不变，无凭证时也可使用。

```go
import (
    "context"
    "os"

    "github.com/xifofo/medianame/tmdb"
)

client, err := tmdb.NewClient(tmdb.Config{
    Token: os.Getenv("TMDB_API_TOKEN"), // TMDB API Read Access Token
    // 也可设置 APIKey；Token 与 APIKey 至少提供一个。
})
if err != nil {
    return // 处理配置错误。
}
result, err := client.Recognize(context.Background(),
    "肮脏天使 (2024) - Dirty.Angels.2024.1080p.x264-404.mkv",
)
if err != nil {
    return // 请求失败；result 中仍保留已取得的候选。
}
if result.MediaInfo != nil {
    media := result.MediaInfo
    _ = media.Title
    _ = media.Overview
    _ = media.PosterURL
}
// 待核对或同名歧义时，完整详情位于 result.Candidates[i].MediaInfo，
// 原因位于 result.Candidates[i].Conflicts，不应直接自动采纳。
// 选定候选后，片名、发行片名、原名和年份均采用 TMDB 信息；
// 已处理差异位于 result.Candidates[i].ResolvedConflicts，原始线索仍在 MetaInfo。
```

其他入口：

- `client.RecognizePath(ctx, path)`：参考目录中的片名、年份、ID 和季集线索。
- `client.RecognizeInfo(ctx, info)`：复用已有 `medianame.Info`。
- `client.Details(ctx, medianame.TypeMovie, "1043905")`：已知类型和 TMDB ID 时直接取得详情；剧集使用 `TypeTV`。
- `client.FindByID(ctx, tmdb.SourceIMDb, "tt15450946")`：通过外部 ID 取得电影／整部剧集摘要；TVDB 使用 `tmdb.SourceTVDB` 和数字 ID。完整详情另用 `Details` 查询，`Recognize*` 已自动串联这两步。
- `client.Search(ctx, query)`：取得名称搜索第一页的摘要及分页信息，完整详情另用 `Details` 查询。
- `result.Reconcile()`：用已有完整候选按当前规则重新校验，不发起网络请求，适合缓存或历史报告。

识别结果包含 `status`、本地 `meta_info`、在线 `media_info` 和 `candidates`：

| 状态 | 含义 | `media_info` |
| --- | --- | --- |
| `compatible` | 已选定可采用候选；元数据以 TMDB 为准 | 完整详情 |
| `review` | 类型、ID 或其他输入冲突，必要元数据缺失；或搜索被截断 | `null`，完整详情与冲突在 `candidates` |
| `ambiguous` | 多个候选未能按身份线索及 TMDB 顺序规则选定 | `null`，所有候选详情在 `candidates` |
| `not_identified` | 未取得候选 | `null` |
| `request_error` | API、网络、取消或响应格式错误，同时返回 Go `error` | `null`，已取得的候选仍保留 |

默认以 TMDB 为影视元数据准：选定候选后，片名、发行片名、原名和年份统一采用 TMDB 信息；本地名称不同、年份不同或文件内部 `conflicting_years` 不再单独阻止识别。原始文件线索保留在 `meta_info`，已处理差异保存在候选的 `resolved_conflicts`。例如 `超级礼物 (2007) - [超级礼物].The.Ultimate.Gift.2006...` 返回 `compatible`，采用 TMDB 的《超级礼物》及上映年 2007，同时保留文件中的年份差异。

先选择候选，再统一使用 TMDB 元数据。优先选择已有无冲突候选，其次选择名称、原名或别名已命中的候选，其他模糊搜索结果不会抢占它；都未命中名称时，完整的单一候选也可采用，多候选继续返回 `ambiguous`。发行片名与 TMDB 原名不同只记录为已处理的 `release_title` 差异。影视类型、数据库 ID 不同，以及其他候选歧义、搜索截断仍需要核对。根包的 `CheckCandidate()` 继续返回严格的原始差异。

同一类型中，多个候选都命中输入名称、原名或官方别名，且年份均与输入一致时，采用 TMDB 返回顺序中的第一项，并返回 `selection_basis: "tmdb_order"`。全部候选及原始详情保留。例如 `预兆 (2023) - Harbin.2024...` 的同名同年候选按 TMDB 顺序选定，最终发行名和年份使用选中作品的信息。跨类型、年份不明或不同、显式 ID 存在歧义、多个名称都未命中，以及搜索截断不会用此规则强行选定。

`MediaInfo` 返回片名、原名、TMDB 官方别名/译名、年份、影视类型、外部 ID、简介、海报及背景图片、评分与票数、类型标签、语言、制片国家和公司、演职员、上映/首播日期、时长和状态。电影可包含所属系列、预算和票房；剧集可包含播出平台、创作者、剧集类型、语言列表、季列表、总季数、TMDB 总集数及上一集/下一集摘要。`meta_info` 中的本地季集与 TMDB 的季列表、总集数分别保留，剧集缺失季号时 `meta_info.season` 默认为 `1`，显式 `0` 会保留；此入口不会逐季抓取每一集的详情。

名称及身份信息既提供方便比对的名称列表，也保留完整的结构化记录：

| 字段 | 内容 |
| --- | --- |
| `title` / `original_title` | 当前返回语言的片名及原始片名；电影和剧集统一使用这两个字段 |
| `en_title` | 英文译名，优先英语美国地区的记录；无英文翻译时回退原名 |
| `category` | 按完整 TMDB 详情匹配出的二级分类，默认使用本文的内置策略 |
| `name` / `original_name` | 剧集同时保留 TMDB 原始字段名对应的名称 |
| `aliases` | 全部官方别名及各语言译名的去重列表，不重复当前片名和原名 |
| `all_titles` | 当前片名、原名及全部别名/译名，供调用方直接做名称比对 |
| `alternative_titles` | 全部别名记录，包含 `title`、`iso_3166_1` 国家和 `type` 名称类型；不同地区的同名记录分别保留 |
| `translations` | 全部语言/国家翻译记录，保留 `iso_639_1`、`iso_3166_1`、语言名称以及 `data` 内的译名、简介、标语、主页和时长（如接口提供）；只有简介而没有译名的记录也保留 |
| `external_ids` | IMDb、TVDB、TVRage、Wikidata、Freebase 及社交账号 ID（如接口提供）；数字 ID 转为字符串，原始值及新增来源仍完整保存在其 `raw` 内 |

`alternative_titles` 和 `translations` 不按当前 `language` 配置裁剪，也不将不同地区/语言的记录合并。名称比对使用 `aliases`，完整记录供详情展示或进一步核对。TMDB 上没有的字段不编造。

`poster_path`、`backdrop_path` 保留 TMDB 图片路径，`poster_url`、`backdrop_url` 提供默认图片服务的 `original` 尺寸链接。影视详情接口及追加的别名、翻译、外部 ID、演职员响应完整保存在 `raw`，不裁剪空值、地区记录或尚未归一化的字段，调用方可以继续读取。该返回范围是影视详情和上述四个子接口。

查询规则按以下顺序执行：

1. 有 TMDB ID 时结合类型直接查详情；类型未知时分别查电影和剧集，数字 ID 相同也作为两个候选。
2. 没有 TMDB ID、但有 IMDb／TVDB ID 时，通过 `/find/{external_id}?external_source=imdb_id` 或 `tvdb_id` 反查，再按返回的 TMDB ID 和类型取得详情。IMDb 与 TVDB 同时存在时分别查询，按“类型 + TMDB ID”合并重复候选，并用完整详情校验全部输入 ID。
3. 外部 ID 没有电影／整剧映射，或没有上述三种 ID 时，回退片名及同年份发行别名搜索。电影使用 `year`，剧集使用 `first_air_date_year`；指定年份未返回结果时再搜索其他年份，原年份仍保留为输入线索。

例如 `Film.[imdb=tt15450946].mkv` 可通过 IMDb 定位电影，`Show.S01E02.[tvdb=123456].mkv` 可通过 TVDB 定位整部剧集；ID 也可以从 `RecognizePath` 的父目录继承。`meta_info` 始终保留文件的原始 ID、片名和季集，转换得到的 TMDB ID 位于 `media_info` 或候选详情中。

当前外部 ID 反查只接收电影和整部剧集：TVDB 不支持电影映射；人物、季和单集结果不会被当作整部作品。只返回这些结果时也可回退文件名搜索；没有可用片名且没有映射时返回 `not_identified`。回退搜索仍校验原始 ID，ID 不同或详情未提供对应 ID 时返回 `review` 并保留候选。超时、限流、HTTP 错误或无效响应返回 `request_error`，不会作为“无映射”触发回退。

官方别名和译名参与候选选择；选定后的名称、原名及年份差异按 TMDB 处理，类型、ID、歧义和截断仍需核对。外部 ID 返回的类型与输入类型不同时也保留冲突，不改写本地类型线索。

默认语言 `zh-CN`，请求超时 20 秒，名称搜索不含成人条目。`Config` 可配置语言、`IncludeAdult`、API 代理地址（含 `/3` 前缀）、HTTP Client 和详情候选上限（默认 20，最大 100）。名称搜索仅取第一页；多页或候选超限时返回 `truncated: true` 和 `review`，不把未查完的结果当成唯一匹配。Client 配置只读，可并发使用。API 错误可通过 `errors.As` 取得 `*tmdb.APIError`，其中保留 HTTP 状态、TMDB 错误码和 `Retry-After`；429 或超时不会被计为无候选。包不自动重试，也不保存凭证。

接口依据：[TMDB 官方 OpenAPI](https://developer.themoviedb.org/openapi/tmdb-api.json)、[认证](https://developer.themoviedb.org/docs/authentication-application)、[外部 ID 反查](https://developer.themoviedb.org/reference/find-by-id)、[电影详情](https://developer.themoviedb.org/reference/movie-details)、[剧集详情](https://developer.themoviedb.org/reference/tv-series-details)。所有请求为 `GET`，无请求体；详情追加 `external_ids,alternative_titles,translations,credits`。

### 批量识别与内存缓存

同一部电视剧的不同集应复用一个 `tmdb.Client`。搜索、外部 ID 反查与详情默认缓存 TMDB 响应 JSON，相同请求并发到达时只发起一次网络请求；每次调用重新解码独立的返回对象，并按当前文件重新校验候选，季号、集号、路径和技术信息不会混用。调用方修改返回的嵌套字段或 `Raw` 不会污染缓存或其他识别结果。

例如 30 集的解析片名、年份、类型相同，且搜索只返回一个候选时，缓存有效期间总共只需 1 次搜索和 1 次详情请求；已知 TMDB ID 和类型时，只需 1 次详情请求。使用同一个 IMDb 或 TVDB ID 且反查只返回一个作品时，只需 1 次反查和 1 次详情请求。不同名称、年份、类型仍独立搜索；多个候选的详情、候选顺序、分页截断和冲突处理保持原有规则。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `Cache.MaxBytes` | `16 << 20`（16 MiB） | 搜索、外部 ID 反查和详情共用的 JSON 正文字节预算 |
| `Cache.MaxEntries` | `512` | 总条目上限；达到字节或条目上限时按 LRU 淘汰 |
| `Cache.TTL` | `time.Hour` | 成功响应入缓存后的有效期，命中不续期 |
| `Cache.Disabled` | `false` | 为 `true` 时关闭缓存及并发请求合并 |

数值配置为 `0` 表示使用默认值，负数返回配置错误。内存按实际数据分配，不会在创建 Client 时预分配 16 MiB。缓存仅保留一份 JSON，字节预算不包括键索引、在途请求、临时解码和调用方保留的结果，并非整个进程的内存上限。单条响应超过缓存预算时仍正常返回，但不缓存；已有的单响应 4 MiB 限制继续生效。

```go
// 使用自定义有效期时需要 import "time"。
client, err := tmdb.NewClient(tmdb.Config{
    Token: os.Getenv("TMDB_API_TOKEN"),
    Cache: tmdb.CacheConfig{
        MaxBytes:   16 << 20,
        MaxEntries: 512,
        TTL:        time.Hour,
    },
})
if err != nil {
    return // 处理配置错误。
}

// 多次 Recognize、RecognizePath、RecognizeInfo、FindByID、Search、Details 共用缓存。
stats := client.CacheStats()
_ = stats.Entries // 当前有效条目数。
_ = stats.Bytes   // 当前缓存 JSON 正文的总字节数。

client.ClearCache() // 下一次查询重新请求 TMDB。
// 完全关闭：Cache: tmdb.CacheConfig{Disabled: true}
```

缓存按 Client 实例隔离，搜索键区分实际片名、年份、类型、语言、成人内容开关等请求参数，外部 ID 反查键区分来源、ID 和语言，详情键区分类型和 ID 等请求参数；不同 Client 不共享数据。进程退出后缓存消失，包不写磁盘，也不需要 Redis。命令行一次传入多个文件时会复用同一个 Client；分别启动命令不会共享缓存。

只有 HTTP 和内容校验均成功的响应会缓存，包括合法的空搜索和空反查结果。网络错误、取消、超时、404、429、其他 API 错误、无效 JSON、残缺详情或 ID 不匹配均不缓存。每个并发调用的 context 取消和截止时间独立生效；还有其他等待者时共享请求继续，全部等待者离开时取消网络请求，`HTTPClient.Timeout` 仍生效。

过期条目在访问、写入或读取统计时清理，不启用常驻清理 goroutine。`ClearCache()` 可并发调用，清除前已经在等待的请求仍可完成，但其响应不会重新填入缓存，也不会影响清除后的新请求；用于主动刷新连载剧信息时，清除后再发起查询即可。

## 自定义重命名

`github.com/xifofo/medianame/rename` 使用 Go 自带的 [`text/template`](https://pkg.go.dev/text/template)。电影、剧集默认模板由提供的 MP2 格式等价转换，保留目录、季集补零、空技术字段省略和文件后缀。

完整操作步骤与可运行示例见 [重命名使用文档](docs/renaming.md)。模板变量大小写敏感，使用 `{{.变量名}}`；例如分辨率为 `{{.videoFormat}}`，扩展名为 `{{.fileExt}}`。

```go
// 已取得 TMDB 详情时，不会再发起网络请求。
named, err := media.Rename(info, "") // 空模板使用默认电影/电视剧格式。
if err != nil {
    return // 处理缺失身份、季集或模板错误。
}
_ = named.Path      // 片名 (2024) {tmdb-123}/Season 01/English Title.2024.S01E02.mkv
_ = named.Directory // 片名 (2024) {tmdb-123}/Season 01
_ = named.Name      // English Title.2024.S01E02.mkv

// 自定义格式；category 作为可选的二级目录前缀。
named, err = media.Rename(info,
    `{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.en_title}}{{if .season_episode}}.{{.season_episode}}{{end}}{{.fileExt}}`,
)
```

也可直接传变量表，或编译一次后批量复用：

```go
import "github.com/xifofo/medianame/rename"

template, err := rename.Compile(`{{.title}}.{{printf "%02d" (int .season)}}{{.fileExt}}`)
if err != nil {
    return
}
context := media.RenameContext(info) // 可添加或覆盖变量。
named, err := template.Render(context)

// 完全离线；调用方提供媒体身份字段。
context = rename.BuildContext(info, rename.Media{
    Title: "示例剧", EnglishTitle: "Example Show", Year: 2024,
    Type: medianame.TypeTV, IDs: medianame.MediaIDs{TMDB: "123"},
})
named, err = rename.Render(rename.DefaultTVTemplate, context)
```

Go 模板语法与 MP2 的 Jinja 语法不同，不直接接受 Jinja 模板：

| MP2 Jinja 写法 | 本包 Go 写法 |
| --- | --- |
| `{{title}}` | `{{.title}}` |
| `{% if edition %}.{{edition}}{% endif %}` | `{{if .edition}}.{{.edition}}{{end}}` |
| `{{ "%02d"\|format(season\|int) }}` | `{{printf "%02d" (int .season)}}` |
| `{{season_episode\|replace(' ', '')}}` | `{{.season_episode \| replace " " ""}}` |

支持内置 `if/else/with/range`、`printf`、`index` 等能力，并提供 `replace`、`lower`、`upper`、`trim`、`int` 函数。`replace` 按 Go 管道惯例将输入放在最后：`{{.title | replace " " "."}}`。

### 重命名变量

`rename.BuildContext(info, media)` 生成以下 38 个变量；`media.RenameContext(info)` 还会按文件季号填充 `total_episodes`、`season_year`。下表的“媒体”指传入的 `rename.Media` 或已选定的 TMDB `MediaInfo`，“文件”指 `medianame.Info`。变量值在渲染前会进行文件名字符处理。

#### 片名、年份与分类

| 变量 | 类型 | 来源与含义 | 示例 / 缺失值 |
| --- | --- | --- | --- |
| `title` | 字符串 | 媒体片名优先，缺失时使用文件解析片名 | `示例剧` / `""` |
| `en_title` | 字符串 | 媒体英文名，依次回退媒体原名、最终片名 | `Example Show` / `""` |
| `original_title` | 字符串 | 媒体原名，缺失时使用最终片名 | `原语种片名` / `""` |
| `title_year` | 字符串 | 最终片名加年份；无年份时只有片名 | `示例剧 (2024)` |
| `year` | 整数或空字符串 | 媒体年份优先，缺失时使用文件年份 | `2024` / `""` |
| `type` | 字符串 | 媒体影视类型优先，缺失时使用文件类型 | `movie`、`tv`、`unknown` |
| `category` | 字符串 | 媒体的二级分类；在线识别由分类策略计算 | `动画番剧` / `""` |
| `name` | 字符串 | 文件名解析出的片名 | `Color of Sky Color of Water` / `""` |
| `original_name` | 字符串 | 原始输入的最后一个路径段，包含扩展名 | `Show.S01E02.mkv` / `""` |

#### 数据库 ID

所有 ID 都是字符串，缺失时为 `""`；IMDb 保留 `tt` 前缀。

| 变量 | 来源 | 示例 |
| --- | --- | --- |
| `tmdbid` | 媒体 TMDB ID 优先，回退文件中的显式 ID | `30983` |
| `imdbid` | 媒体 IMDb ID 优先，回退文件中的显式 ID | `tt0131179` |
| `tvdbid` | 媒体 TVDB ID 优先，回退文件中的显式 ID | `72454` |
| `doubanid` | 文件中的显式豆瓣 ID | `123456` |
| `bangumiid` | 文件中的显式 Bangumi ID | `123456` |
| `anilistid` | 文件中的显式 AniList ID | `123456` |

#### 季集

| 变量 | 类型 | 来源与含义 | 示例 / 缺失值 |
| --- | --- | --- | --- |
| `season` | 整数或空字符串 | 文件季号；解析剧集时缺失季号默认 `1`，明确 `0` 保留 | `1`、`0` / `""` |
| `season_fmt` | 字符串 | 格式化季号，至少两位；支持季范围 | `S01`、`S00`、`S01-S03` / `""` |
| `episode` | 整数、字符串或空字符串 | 单集为整数；范围和非连续集为字符串 | `2`、`"2-5"`、`"1,3,5"` / `""` |
| `season_episode` | 字符串 | 剧集季集组合，适合直接写入文件名；电影为空 | `S01E02`、`S00E00`、`S01E1214`、`S01E02-E05`、`S01E01E03E05`、`S01E10-S02E03` / `""` |
| `total_episodes` | 整数 | `media.RenameContext` 按文件季号找到的 TMDB 对应季集数，包含特别季；离线 `BuildContext` 默认为 `0` | `1216` / `0` |
| `season_year` | 字符串 | `media.RenameContext` 中对应季 `air_date` 的前四位；离线 `BuildContext` 为空 | `1996` / `""` |

`total_episodes` 表示文件所属季的总集数；整部作品的总集数在 `media.NumberOfEpisodes`，可由调用方加入自定义变量。季范围和跨季文件使用起始季匹配季详情。直接构造 `Info` 时应自行提供季号，或明确 `TypeTV` 并先调用 `RecognizeInfo` 应用剧集默认季号。

Go 模板中整数 `0` 在 `if` 判断里为假。保留特别季时，用 `{{if .season_fmt}}` 判断季号是否存在，并优先输出 `{{.season_episode}}`，这样 `S00/E00` 也会保留。

#### 资源参数

资源参数来自文件名解析，字符串缺失时为 `""`。

| 变量 | 类型 | 含义 | 示例 |
| --- | --- | --- | --- |
| `edition` | 字符串 | 来源和效果，用空格连接 | `WEB-DL HDR10`、`REMUX` |
| `resourceType` | 字符串 | `Info.Source` | `WEB-DL`、`BluRay`、`REMUX` |
| `effect` | 字符串 | 全部效果，用空格连接 | `Dolby Vision HDR10` |
| `videoFormat` | 字符串 | 分辨率 | `1080p`、`2160p` |
| `videoCodec` | 字符串 | 归一化视频编码 | `H.264`、`H.265`、`AV1` |
| `videoBit` | 字符串 | 位深加 `bit` 后缀 | `10bit` |
| `audioCodec` | 字符串 | 音频编码保留 `DDP` / `DD+` / `DD` 别名，用空格附加已识别声道和 Atmos | `FLAC 2.0`、`DDP 5.1`、`TrueHD 7.1 Atmos` |
| `webSource` | 字符串 | 流媒体平台标记 | `Amazon`、`Netflix` |
| `releaseGroup` | 字符串 | 发布组 / 字幕组 | `Misaki`、`SBSUB` |
| `part` | 字符串 | 分卷号加 `part` 前缀 | `part2` |
| `fps` | 浮点数或空字符串 | 帧率 | `23.976` / `""` |
| `resource_term` | 字符串 | `edition` 与分辨率组合 | `WEB-DL HDR10 2160p` |
| `fileExt` | 字符串 | 已识别扩展名，包含点号并转为小写 | `.mkv`、`.mp4`、`.ass` |

视频编码使用本包的归一化值，例如 `H.265`。音频中的 `DDP`、`DD+`、`DD` 对齐 MP2 保留发行别名，不替换成 `E-AC-3` / `AC-3`；例如 `DDP2.0` 解析为 `AudioCodec: "DDP"`、`AudioChannels: "2.0"`，重命名变量 `audioCodec` 为 `DDP 2.0`。原本使用 `EAC3` / `E-AC-3` 或 `AC3` / `AC-3` 的输入仍分别归一为 `E-AC-3`、`AC-3`。`edition`、`effect`、`audioCodec` 可能同时包含 Atmos，按所需信息选择模板变量。

#### 调用方补充的变量

| 变量 | 初始值 | 用途 |
| --- | --- | --- |
| `en_name` | `""` | 调用方补充本地英文名称 |
| `customization` | `""` | 调用方自定义标记 |
| `episode_title` | `""` | 调用方补充单集片名 |
| `episode_date` | `""` | 调用方补充单集播出日期 |

通过 `context := media.RenameContext(info)` 后设置 `context["episode_title"] = "单集名称"`，即可覆盖预留变量或增加新变量。需要别名、简介、整部作品总集数等额外字段时，也通过此方式加入。模板只读取生成的变量表，见 [变量覆盖示例](docs/renaming.md#覆盖变量与增加变量)。

未定义变量或错误模板返回 `error`。生成的路径只使用 `/`，不允许绝对路径、空路径段和 `..`；变量内的文件名非法字符会转换为全角，模板本身的 `/` 用于分隔目录。结果是命名预览，包不创建目录或修改文件。默认模板需要片名、年份和 TMDB ID；默认剧集模板还需要季集，解析结果缺少季号时使用默认第 1 季，明确的 S00 保留，跨季文件需要自定义模板。

## 二级分类

完整的配置、加载和命名示例见 [分类使用文档](docs/category.md)。可直接修改 [JSON 分类示例](category/custom.example.json)，再通过 `-category-config` 或 `category.ParseJSON` 加载。

内置策略位于 [category/default.yaml](category/default.yaml)，按提供的顺序保留动画电影、华语电影、欧美电影、日韩电影、外语电影，以及动画番剧、综艺节目、国产剧集、欧美剧集、日韩剧集、其他剧集。

`client.Details` 和在线识别的完整候选自动返回 `media_info.category`。自定义分类推荐使用结构化 `category.Config`：包调用方可直接传 Go 对象，保存成文件时使用 JSON。电影和剧集分别用规则数组表示，数组顺序就是优先级，适合配置页面增删规则和调整顺序。分类名称、条件和兜底名称都可以自定义。

自定义策略通过 `tmdb.Config.CategoryPolicy` 传入，后续详情查询复用同一只读策略：

```go
import "github.com/xifofo/medianame/category"

policy, err := category.New(category.Config{
    Movie: []category.Rule{
        {Name: "华语动画", Conditions: map[string]any{
            "genre_ids": []int{16}, "production_countries": []string{"CN", "TW", "HK"},
        }},
        {Name: "其它电影"},
    },
    TV: []category.Rule{
        {Name: "成人动画", Conditions: map[string]any{"adult": true, "genre_ids": []int{16}}},
        {Name: "动画番剧", Conditions: map[string]any{"genre_ids": []int{16}}},
        {Name: "其它剧集"},
    },
})
if err != nil {
    return
}
client, err := tmdb.NewClient(tmdb.Config{
    Token: os.Getenv("TMDB_API_TOKEN"), CategoryPolicy: policy,
    IncludeAdult: true, // 需要搜索成人条目时开启。
})

// 已有详情时重新分类，不发起网络请求。
name, err := media.Classify(policy)
if err == nil {
    media.Category = name // 命名模板使用 media.Category。
}

// 也可直接对 TMDB 原始详情 JSON 分类。
name, err = policy.MatchJSON(medianame.TypeMovie, rawDetails)
```

JSON 使用相同的结构，完整可修改示例见 [category/custom.example.json](category/custom.example.json)。加载时调用 `category.ParseJSON(data)`，然后传入 `tmdb.Config.CategoryPolicy`：

```json
{
  "tv": [
    {"name": "成人动画", "conditions": {"adult": true, "genre_ids": [16]}},
    {"name": "动画番剧", "conditions": {"genre_ids": [16]}},
    {"name": "其他剧集"}
  ]
}
```

多字段为 AND，单字段列表或逗号值为 OR，空分类条件为兜底。值支持字符串、数字、布尔值和这些值的列表。条件大小写不敏感，支持 `!` 排除值、数字范围和 `release_year`，也可匹配 `status`、`adult` 等 TMDB 一级标量或标量列表字段。`genres[].id` 自动映射为 `genre_ids`，`production_countries` 使用 `iso_3166_1`，剧集使用 `origin_country`。`release_year` 取电影上映年或剧集首播年。缺少条件字段不会匹配，包括只有排除值的条件；没有命中或没有对应类型策略时返回空字符串。自定义配置完整定义自己的规则，没有配置的类型不分类。规则编译后不受调用方修改原配置影响。

原有 YAML 配置继续通过 `category.Parse(data)` 加载，支持 `movie` / `tv` 和中文别名 `电影` / `电视剧`，按 YAML 顺序匹配。JSON 和 Go 对象使用 `movie` / `tv` 分支和规则数组。

原始 TMDB 的剧集 `type`（如 `Miniseries`）在分类中保持其原值，影视 `movie/tv` 类型单独决定策略分支。策略只计算分类名称；是否在目标路径增加分类目录由模板控制，如 `{{if .category}}{{.category}}/{{end}}`。传入空策略 `category.New(category.Config{})` 可关闭分类。不传 `CategoryPolicy` 时使用内置默认规则。搜索成人条目仍需单独设置 `IncludeAdult: true` 或命令行 `-adult`。

## 命令行预览

安装命令行工具后，可直接运行 `medianame`：

```bash
go install github.com/xifofo/medianame/cmd/medianame@latest
medianame 'Breaking.Bad.S01E02.1080p.WEB-DL.H264.mkv'
```

在源码目录中运行或构建：

```sh
go run ./cmd/medianame 'Breaking.Bad.S01E02.1080p.WEB-DL.H264.mkv'
go run ./cmd/medianame -path '/tv/Example Show (2024)/Season 0/01.mkv'
go run ./cmd/medianame -queries '肮脏天使 (2024) - Dirty.Angels.2024.1080p-404.mkv'
go run ./cmd/medianame -tmdb '肮脏天使 (2024) - Dirty.Angels.2024.1080p-404.mkv'
go run ./cmd/medianame -tmdb -path '/tv/示例剧 (2024) [tmdb=123]/Season 0/01.mkv'
go run ./cmd/medianame -tmdb -rename 'Dirty.Angels.2024.[tmdb=1043905].1080p.mkv'
go run ./cmd/medianame -tmdb -adult -category-config category/custom.example.json 'Color.of.Sky.Color.of.Water.2006.S01E01.1080p.mkv'
go run ./cmd/medianame -tmdb -template '{{if .category}}{{.category}}/{{end}}{{.title_year}}/{{.original_name}}' -category-yaml category/default.yaml 'Dirty.Angels.2024.[tmdb=1043905].1080p.mkv'
go build ./cmd/medianame
```

每个输入参数输出一个 JSON 对象。

`-tmdb` 需要在终端环境设置 `TMDB_API_TOKEN` 或 `TMDB_API_KEY`（二者都有时 Token 优先）。可结合 `-path`、`-queries` 使用，`-language` 控制返回语言，`-tmdb-url` 设置 API 代理，`-adult` 包括成人搜索结果。请求失败会在 JSON 中返回 `request_error` 和错误信息，并以非零退出码结束；身份冲突仍正常输出待核对候选。凭证不作为命令行参数，也不进入错误文本。

`-rename` 输出 `rename.path`、`rename.directory`、`rename.name`；`-template` 隐含 `-rename`，使用 Go 模板。`-category-config` 加载 JSON 分类配置，`-category-yaml` 加载原有 YAML 分类配置，二者只指定一个。这些选项需要结合 `-tmdb`。已选定的可采用候选生成命名预览，片名和年份使用 TMDB 信息；同名同年多候选按上述 TMDB 顺序规则选择，其他待核对、歧义或请求错误保留识别证据。命名失败另返回 `rename_error` 并以非零退出码结束。

## 验证

```sh
go test ./...
go test -race -cover ./...
go vet ./...
go test -run='^$' -fuzz=FuzzParse -fuzztime=20s -parallel=2
go test -run='^$' -bench=BenchmarkParse -benchmem
```

测试覆盖中文数字、动漫版本、特殊季、多集及跨季范围、显式 ID、扩展名边界、数字片名、路径继承与并发隔离。示例代码也由 `go test` 验证。

GitHub Actions 配置在 Go 1.22 和当前稳定版本上检查格式、依赖、`go vet`、竞态测试与构建。测试使用本地 HTTP 模拟服务，不需要 TMDB 或 MP2 凭证。

## 发布版本

推送代码到 `main` 后，CI 自动执行检查。准备发版时，打开 [Release 工作流](https://github.com/xifofo/medianame/actions/workflows/release.yml)，点击 **Run workflow**，选择 `main` 并填写版本号，例如 `v0.1.0`。

Release 会先校验版本号及模块路径，再复用完整 CI；全部通过后，给本次运行的提交创建标签，生成发布说明和 GitHub Release，最后从 Go 公共代理下载指定版本，并在独立项目中执行 `go get` 和构建验证。发布任务使用仓库自带的 `GITHUB_TOKEN`，不需要额外配置个人访问令牌。此流程发布 Go 模块和源码，不上传各平台命令行二进制。

当前模块路径支持 `v0.x.x` 和 `v1.x.x`；发布 `v2` 及以上版本前需要先迁移到对应的 `/vN` 模块路径。`v0.2.0-rc.1` 这样的版本自动标记为预发布。同一仓库的发布流程串行执行，已有标签不会被改写；只有标签仍指向本次提交时才允许重跑，已发布的 Release 会复用。如果版本已发布但 Go 代理下载暂时失败，可在该次运行中选择 **Re-run failed jobs**，无需重新发版。

也可在已登录的 GitHub CLI 中触发：

```sh
gh workflow run release.yml --ref main -f version=v0.1.0
```

## 与本地 MP2 做影子测试

`medianame-shadow` 可导入 UTF-8 或带 BOM 的 UTF-16 目录树，把视频文件名交给本地包与真实 MP2 对比。API Token 从环境变量读取，不写入报告。

先在终端设置 `MP2_API_TOKEN`，然后运行：

```sh
go run ./cmd/medianame-shadow \
  -tree '/path/to/media-tree.txt' \
  -base-url 'http://localhost:3001' \
  -output reports/mp2-shadow \
  -workers 2 \
  -recognize \
  -path-fallback
```

可加 `-limit 10` 先测试少量样本。默认只做名称解析比对；`-recognize` 追加 MP2 在线媒体识别，`-path-fallback` 在文件名未识别时追加目录树路径识别测试。目录树路径是逻辑路径，无须在 MP2 上存在同名文件；这项测试无法证明实际 NFO 或本地文件读取行为。

名称解析使用 `GET /api/v1/system/ruletest` 的不存在规则组分支，取得 `data.meta_info` 后立即返回。这个分支的 `success=false` 是预期行为，工具会验证它确实没有规则组和在线 MediaInfo。在线识别使用 `GET /api/v1/media/recognize`，目录路径识别使用 `GET /api/v1/media/recognize_file`。

报告包含：

- `report.html`：可筛选文件、差异字段、未识别样本和目录恢复样本。
- `report.json`：两侧结果、字段归一化值、差异、完整 MP2 在线原始响应。
- `failures.jsonl`：识别失败、接口错误、本地无片名的完整样本，目录恢复成功的失败原例仍然保留。
- `differences.jsonl`：全部名称解析差异，便于建立兼容性回归样例。
- `summary.md`：统计和字段差异分布。

名称解析与在线媒体识别分别统计。返回媒体候选不等于已人工确认识别正确；请求超时也不等于片名无法识别。比较时统一编码名称、分辨率别名、类型语言、大小写和集列表表达，保留原始值供核对；两侧同时缺失的字段不计入有值字段的一致率。

工具每 25 个样本或约 20 秒保存进度。中断后，已有报告标记为未完成。修改本地解析器后，可以沿用已记录的 MP2 基线离线重放，无须重新请求 MP2：

```sh
go run ./cmd/medianame-shadow \
  -replay reports/mp2-shadow/report.json \
  -output reports/mp2-shadow-replay
```

`reports/` 默认排除在 Git 跟踪之外。

增强查询的真实验证使用独立命令，选取基线中未识别或身份有冲突的记录。它调用 MP2 获取候选，再使用包的 `CheckCandidate()` 检查，不会覆盖原始基线：

```sh
# 需要同一个 MP2_API_TOKEN 环境变量，会发送真实 API 请求。
go run ./cmd/medianame-enhance \
  -baseline reports/mp2-shadow/report.json \
  -output reports/mp2-enhanced \
  -workers 2
```

`report.html` 可筛选并展开每条增强查询；`results.json` 保存清理后的查询、原始基线、在线响应和冲突原因；`unresolved.jsonl` 保存未返回、请求错误或待复核的记录。状态 `compatible` 表示无冲突候选，`review` 表示有候选但存在冲突，`no_candidate` 表示未返回候选；请求错误单独记为 `request_error`。这与只重新解析已有数据的 `medianame-shadow -replay` 不同。

## 与 MoviePilot 的关系

参考的模块与当前覆盖范围见 [docs/moviepilot-reference.md](docs/moviepilot-reference.md)。本包专注名称解析，结果是名称线索，不能保证名称描述与实际音视频流一致。

可选 `tmdb` 子包提供直接连接 TMDB 的在线识别和影视详情，返回 `meta_info`/`media_info` 结构及冲突候选，但字段类型以本包为准，不承诺与 MoviePilot 完全兼容。MoviePilot 的识别词表达式、完整发布组配置、拼音与中英文名称优先级规则仍未实现。这里的统一字段格式与保守目录继承策略也有明确差异，因此不承诺与 MoviePilot 的全部测试逐项一致。
