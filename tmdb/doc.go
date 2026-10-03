// Package tmdb 为 medianame 提供可选的 TMDB 在线搜索、识别和影视详情查询。
//
// Client.Recognize 和 Client.RecognizePath 返回本地 meta_info、在线 media_info
// 及完整候选和冲突证据。选定候选后，片名、发行片名、原名及年份以 TMDB 为准，
// 已处理差异保存在 resolved_conflicts；compatible 表示已选定可采用候选。
// 显式 TMDB ID 直接查详情；IMDb/TVDB ID 通过 FindByID 反查电影或整部剧集，
// 再查询详情并校验全部输入 ID。无映射时回退片名搜索，请求失败不回退；
// 季和单集 ID 暂不作为整剧识别，回退候选仍保留原始 ID 校验。
// 完整详情附带英文标题和二级分类；MediaInfo.Rename 使用 Go 模板返回命名预览。
// Client 默认缓存成功的搜索、外部 ID 反查和详情 JSON，预算 16 MiB、512 条、有效期 1 小时；
// 相同并发请求合并，每次调用仍独立解码和校验文件身份。可通过 Config.Cache 调整或禁用。
// medianame.Parse 和 medianame.ParsePath 仍完全离线，不需要凭证。
package tmdb
