# MoviePilot 参考范围

实现时参考了 MoviePilot 以下模块的输入格式、输出字段和边界场景。Go 实现使用独立的结构标记与资源标记解析流程，没有引入 Python、Rust、anitopy 或 MoviePilot 的运行时。

| MoviePilot 文件 | 本包对应范围 |
| --- | --- |
| `app/core/metainfo.py` | 名称解析入口、显式数据库 ID、`{[...;s=...;e=...;g=...]}` 复合标签 |
| `app/core/meta/metabase.py` | 季集、范围、中文数字、特别季、资源信息字段 |
| `app/core/meta/metavideo.py` | 常见发布名、分辨率、来源、音视频编码、位深、帧率、扩展名边界 |
| `app/core/meta/metaanime.py` | 字幕组、括号集号、`- 03v2`、动漫技术标签 |
| `app/core/meta/infopath.py`、`MetaInfoPath` | 文件名与季目录、媒体目录的分工；本包采用较保守的继承范围 |
| `app/helper/message.py`、`app/modules/filemanager/transhandler.py` | 重命名变量映射与路径拆分；本包使用 Go `text/template`，提供的 MP2 默认模板已等价转换 |
| `app/modules/themoviedb/category.py` | YAML 分类顺序、AND/OR 条件、国家/类型字段、年份范围与排除值 |
| `tests/cases/meta.py`、`tests/test_metainfo.py` | 对照常见标题与回归场景，包括数字片名、S00、HDR Vivid、片名内的平台词 |
| `tests/test_anime_source_metainfo.py` | Bangumi、AniList 显式 ID 格式 |

本包的测试采用对应场景建立独立样例。对照 MoviePilot 的测试标题用于发现遗漏，不代表运行了 MoviePilot，也不代表它的全部测试已通过。

## 字段差异

- `Title` 保留整理后的名称，不套用 MoviePilot 的中文、英文、拼音优先级。双语名称可以同时保留。
- 年份不能证明媒体是电影；季集可推断 `tv`，复合标签可显式指定 `movie` 或 `tv`。
- 季集用可空整数表达，区分未识别与第 0 季。连续范围与非连续集列表分别保存。
- 视频编码使用统一值，例如 `x265` / `HEVC` 归一为 `H.265`。音频中的 `DDP`、`DD+`、`DD` 对齐 MP2 保留发行别名；本包仍将编码和声道分开存储，从 v0.1.2 起重命名时用点号连接，例如 `DDP2.0` 输出 `DDP.2.0`。命名变量中的 Dolby Vision 缩写为 DV，独立 HDR 标签全部保留，Atmos 统一由 `effect` / `edition` 输出。
- 来源与效果分别保存；遇到 `BluRay REMUX` 时，`Source` 取 `REMUX`。
- 从文件的相邻媒体目录继承；文件有片名时要求与媒体目录同名。确定的 `BDMV/STREAM/五位数字.m2ts` 结构会跳过技术目录，继承媒体根目录，不把 `STREAM` 或分片号视为作品身份。

## 实际影子样本驱动的增强

2026-10-02 的 MP2 基线暴露了整理前缀年份混入标题、`-404` 被识别为集号、蓝光 `STREAM` 被识别为作品等情况。本包对发布名与目录作独立解析；新增 `SearchQueries()` 生成清理后的查询，并用 `CheckCandidate()` 保留年份、类型、ID 和标题冲突。电影或电视剧数据库查询仍由调用方执行，不把 MP2 的行为差异直接当作本地错误。

相应回归位于 `enhancement_test.go`，覆盖真实失败格式与普通目录、真实数字片名、版本括号、剧集等反例。技术字段还补充了 Atmos 前后声道、BD 紧连分辨率。

## 后续可扩展范围

自定义重命名和二级分类现已分别提供 `github.com/xifofo/medianame/rename`、`github.com/xifofo/medianame/category`。模板采用用户选择的 Go 原生语法，不能直接执行 MP2 Jinja 模板；字段使用本包的归一化资源信息。YAML 一级键兼容 `movie/tv` 与 `电影/电视剧`，分类使用直接从 TMDB 获取的完整详情，不依赖 MP2 服务。使用说明与差异见 [README](../README.md)。参考：[MP2 进阶文档](https://wiki.movie-pilot.org/zh/advanced)、[Go 模板文档](https://pkg.go.dev/text/template)。

自定义识别词的替换、定位、集偏移表达式，发布组和平台配置，字幕语言细分，副标题中的“全 X 集”与复杂宣传描述，中英文候选名称分离，以及更完整的 MoviePilot 行为对照，都尚未实现。它们可以作为后续功能加入，当前 API 不对这些行为作出保证。
