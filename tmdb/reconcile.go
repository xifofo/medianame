package tmdb

import (
	"path"
	"regexp"
	"strings"
	"unicode"

	"medianame"
)

var leadingReleaseTitle = regexp.MustCompile(`^\[([^\[\]]+)\]\s*(.+)$|^【([^【】]+)】\s*(.+)$`)

// Reconcile 根据已有完整候选重新校验身份，不请求网络，也不改写 MetaInfo 或影视详情。
// 选定候选后，片名、发行片名、原名及年份差异均按 TMDB 处理，
// 差异保存在 ResolvedConflicts。同名同年的候选按 TMDB 返回顺序采用首项；
// 影视类型、ID、其他歧义与截断仍需要核对。
// 可以用它将历史联调报告按当前规则重算；请求失败的部分结果不会被当成成功。
func (result *Result) Reconcile() {
	if result == nil || result.Status == StatusRequestError {
		return
	}
	result.MediaInfo = nil
	result.SelectionBasis = ""
	var compatible []*MediaInfo
	for i := range result.Candidates {
		candidate := &result.Candidates[i]
		candidate.ResolvedConflicts = nil
		if candidate.MediaInfo == nil {
			candidate.Conflicts = []medianame.CandidateConflict{{Field: "media_info", Expected: "complete_details", Actual: "missing"}}
			continue
		}
		candidate.Conflicts = result.MetaInfo.CheckCandidate(candidate.MediaInfo.Candidate())
		if len(candidate.Conflicts) == 0 {
			compatible = append(compatible, candidate.MediaInfo)
		}
	}
	// 已有无冲突候选时先确定作品，不让其他年份的同名搜索结果抢占它。
	if len(compatible) > 0 {
		result.selectCompatible(compatible)
		return
	}
	var eligible, titleMatched []*Candidate
	for i := range result.Candidates {
		candidate := &result.Candidates[i]
		if candidate.MediaInfo == nil {
			continue
		}
		if metadataOnly(candidate.Conflicts) && metadataAvailable(candidate) {
			eligible = append(eligible, candidate)
			matched := true
			for _, conflict := range candidate.Conflicts {
				if conflict.Field == "title" {
					matched = false
				}
			}
			if matched {
				titleMatched = append(titleMatched, candidate)
			}
		}
	}
	// 优先采用名称、原名或别名已命中的候选；不能因放宽名称差异而让
	// 其他模糊搜索结果抢占它。没有名称命中时，单一完整候选也可按 TMDB 采用。
	if len(titleMatched) > 0 {
		eligible = titleMatched
	}
	for _, candidate := range eligible {
		candidate.ResolvedConflicts, candidate.Conflicts = candidate.Conflicts, nil
		if difference := releaseDifference(result.MetaInfo, candidate.MediaInfo); difference != nil {
			candidate.ResolvedConflicts = append(candidate.ResolvedConflicts, *difference)
		}
		compatible = append(compatible, candidate.MediaInfo)
	}
	result.selectCompatible(compatible)
}

func (result *Result) selectCompatible(compatible []*MediaInfo) {
	switch {
	case len(result.Candidates) == 0:
		result.Status = StatusNotIdentified
	case result.Truncated:
		result.Status = StatusReview
	case len(compatible) == 1:
		result.Status, result.MediaInfo = StatusCompatible, compatible[0]
	case len(compatible) > 1 && result.sameNameAndYear(compatible):
		result.Status, result.MediaInfo = StatusCompatible, compatible[0]
		result.SelectionBasis = "tmdb_order"
	case len(compatible) > 1:
		result.Status = StatusAmbiguous
	default:
		result.Status = StatusReview
	}
}

// 名称、年份和类型相同的搜索候选，保留 TMDB 返回的先后顺序。
// 显式 ID 的类型歧义、多种年份或未命中名称时，仍不能由搜索顺序代替身份线索。
func (result *Result) sameNameAndYear(candidates []*MediaInfo) bool {
	info := result.MetaInfo
	if info.Year == 0 || info.IDs != (medianame.MediaIDs{}) {
		return false
	}
	mediaType := candidates[0].Type
	if !validType(mediaType) {
		return false
	}
	for _, media := range candidates {
		if media.Year != info.Year || media.Type != mediaType {
			return false
		}
		for _, conflict := range info.CheckCandidate(media.Candidate()) {
			if conflict.Field == "title" {
				return false
			}
		}
	}
	return true
}

func metadataOnly(conflicts []medianame.CandidateConflict) bool {
	if len(conflicts) == 0 {
		return false
	}
	for _, conflict := range conflicts {
		if conflict.Field != "year" && conflict.Field != "title" && conflict.Field != "release_title" &&
			!(conflict.Field == "input" && conflict.Actual == "conflicting_years") {
			return false
		}
	}
	return true
}

func metadataAvailable(candidate *Candidate) bool {
	if candidate.MediaInfo.Title == "" {
		return false
	}
	for _, conflict := range candidate.Conflicts {
		if (conflict.Field == "year" || conflict.Field == "input") && candidate.MediaInfo.Year == 0 {
			return false
		}
	}
	return true
}

// 发行名称只保留为差异证据，不阻止采用 TMDB 的片名及原名。
func releaseDifference(info medianame.Info, media *MediaInfo) *medianame.CandidateConflict {
	if info.Original == "" {
		return nil
	}
	name := path.Base(strings.ReplaceAll(info.Original, `\`, "/"))
	name = strings.NewReplacer("－", "-", "–", "-", "—", "-").Replace(name)
	if _, release, ok := strings.Cut(name, " - "); ok {
		name = release
	}
	release := medianame.Parse(name)
	if release.Title == "" {
		return nil
	}
	titles := strings.Split(release.Title, " / ")
	if parts := leadingReleaseTitle.FindStringSubmatch(release.Title); parts != nil {
		titles = parts[1:3]
		if parts[3] != "" {
			titles = parts[3:5]
		}
	}
	names := append([]string{media.Title, media.OriginalTitle}, media.Aliases...)
	for _, title := range titles {
		title = medianame.Parse(title).Title
		if identityName(title) == "" {
			continue
		}
		matched := false
		for _, candidate := range names {
			if identityName(title) == identityName(candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return &medianame.CandidateConflict{Field: "release_title", Expected: title, Actual: media.OriginalTitle}
		}
	}
	return nil
}

func identityName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}
