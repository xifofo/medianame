package medianame

// inferType 在完整身份线索合并后判断类型。没有剧集线索的片名默认按电影处理。
func inferType(info *Info) {
	if info.typeExplicit {
		return
	}
	switch {
	case info.Type == TypeTV || info.Season != nil || info.Episode != nil:
		info.Type = TypeTV
	case info.Title != "":
		info.Type = TypeMovie
	default:
		info.Type = TypeUnknown
	}
}
