// Package medianame 从媒体文件名或发布标题中提取片名、季集和资源信息。
//
// 解析完全在本地完成，不读取文件、不访问网络。结果只描述名称中的线索，
// 不代表媒体数据库识别结果，也不能代替对实际音视频流的探测。
// Info.SearchQueries 生成查询条件，Info.CheckCandidate 检查调用方取得的媒体候选。
// 可选的 medianame/tmdb 子包提供在线识别及 TMDB 影视详情，不改变本包的离线行为。
// medianame/rename 使用 Go 模板生成命名预览，medianame/category 根据 YAML 与详情计算二级分类。
package medianame
