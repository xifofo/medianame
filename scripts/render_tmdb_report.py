#!/usr/bin/env python3
"""Render an existing TMDB comparison report without making network requests.

Usage: python3 scripts/render_tmdb_report.py reports/<run>/report.json
"""

import argparse
from collections import Counter
from datetime import datetime
from html import escape
import json
from pathlib import Path
import re


YEAR = re.compile(r"(?<![\w])(?:18|19|20)\d{2}(?![\w])")
STATUS = {
    "compatible": "无冲突候选",
    "review": "需要核对",
    "ambiguous": "多个候选待确认",
    "not_identified": "未找到候选",
    "request_error": "请求失败",
}
REASONS = {
    "input": "文件内部年份冲突",
    "year": "年份与 TMDB 不同",
    "title": "片名未精确匹配",
    "type": "影视类型不同",
    "id": "数据库 ID 不同",
    "release": "发行片名与候选不同",
    "multiple": "多个候选",
    "truncated": "候选未查完",
    "other": "其他问题",
}


def h(value):
    return escape(str(value if value is not None else ""), quote=True)


def unique(values):
    return list(dict.fromkeys(value for value in values if value))


def normalized(value):
    return "".join(char for char in value.lower() if char.isalnum())


def highlight_years(value):
    parts, offset = [], 0
    for match in YEAR.finditer(value):
        parts.extend([h(value[offset:match.start()]), "<mark>" + h(match[0]) + "</mark>"])
        offset = match.end()
    return "".join(parts) + h(value[offset:])


def source_clues(row):
    """Preserve source text; these excerpts are display clues, not new parsing results."""
    name = row["name"]
    prefix, separator, release = name.partition(" - ")
    if not separator:
        prefix, release = "", name
    stem = re.sub(r"\.[A-Za-z0-9]{2,5}$", "", release)
    first_year = YEAR.search(stem)
    excerpt = stem[:first_year.end()] if first_year else stem
    directory = row.get("path", "").replace("\\", "/").rsplit("/", 1)[0].rsplit("/", 1)[-1]
    return {
        "prefix": prefix,
        "prefix_years": unique(YEAR.findall(prefix)),
        "release": excerpt,
        "release_years": unique(YEAR.findall(stem)),
        "directory": directory,
        "directory_years": unique(YEAR.findall(directory)),
    }


def candidates_for(row):
    result = row["tmdb"]
    candidates = result.get("candidates") or []
    if result["status"] == "compatible" and result.get("media_info"):
        selected = result["media_info"]
        accepted = [c for c in candidates if c["media_info"]["tmdb_id"] == selected["tmdb_id"]
                    and c["media_info"]["type"] == selected["type"]]
        others = [c for c in candidates if c not in accepted]
        return accepted, others
    return candidates, []


def reason_keys(row):
    result = row["tmdb"]
    if result["status"] == "compatible":
        return []
    candidates = result.get("candidates") or []
    fields = {f["field"] for c in candidates for f in c.get("conflicts") or []}
    warnings = result.get("meta_info", {}).get("warnings") or []
    keys = []
    if "input" in fields and "conflicting_years" in warnings:
        keys.append("input")
    keys.extend(key for key in ("year", "title", "type") if key in fields)
    if "release_title" in fields:
        keys.append("release")
    if any(field.endswith("_id") for field in fields):
        keys.append("id")
    if len(candidates) > 1:
        keys.append("multiple")
    if result.get("truncated"):
        keys.append("truncated")
    if not keys or fields - {"input", "year", "title", "type", "release_title", "tmdb_id", "imdb_id", "tvdb_id", "douban_id", "bangumi_id", "anilist_id"}:
        keys.append("other")
    return keys


def input_year_description(clues):
    parts = []
    for label, key in (("整理名", "prefix_years"), ("发行名", "release_years")):
        if clues[key]:
            parts.append(label + " " + " / ".join(clues[key]))
    years = unique(clues["prefix_years"] + clues["release_years"])
    if len(years) < 2 and clues["directory_years"]:
        parts.insert(0, "目录名 " + " / ".join(clues["directory_years"]))
    return "；".join(parts) or "输入包含互相矛盾的年份，请查看下方原始文件名。"


def reason_text(row, keys, clues):
    result = row["tmdb"]
    meta = result["meta_info"]
    candidates = result.get("candidates") or []
    facts = []
    if "input" in keys:
        facts.append("核对文件内部年份：" + input_year_description(clues))
    if "year" in keys:
        differing = [c for c in candidates if any(f["field"] == "year" for f in c.get("conflicts") or [])]
        years = unique(str(c["media_info"].get("year") or "未提供") for c in differing)
        displayed = " / ".join(years)
        numeric = sorted(int(year) for year in years if year.isdecimal())
        if len(years) > 3 and numeric:
            displayed = f"{numeric[0]}–{numeric[-1]}（{len(differing)} 个候选年份不符）"
        label = "TMDB" if len(candidates) == 1 else "有冲突的 TMDB 候选"
        difference = abs((meta.get('year') or 0) - (candidates[0]['media_info'].get('year') or 0)) if len(candidates) == 1 else 0
        gap = f"（相差 {difference} 年，先核对是否同一部作品）" if meta.get('year') and candidates[0]['media_info'].get('year') and difference >= 3 else ""
        facts.append(f"核对上映年份：解析年份 {meta.get('year') or '未提供'} ↔ {label} " + displayed + gap)
    if "title" in keys:
        facts.append("核对片名：解析名「" + meta.get("title", "") + "」未精确匹配部分候选的名称、原名或别名。")
    if "release" in keys:
        differences = unique("文件「" + f['expected'] + "」 ↔ 候选原名「" + f['actual'] + "」"
                             for c in candidates for f in c.get('conflicts') or [] if f['field'] == 'release_title')
        facts.append("先核对是否同一部作品：" + '；'.join(differences))
    if "type" in keys:
        facts.append("核对影视类型：文件与候选的电影 / 剧集类型不同。")
    if "id" in keys:
        facts.append("核对数据库 ID：文件标记与候选返回的 ID 不同。")
    if "multiple" in keys:
        facts.append(f"核对候选身份：共 {len(candidates)} 个候选；每个候选的原名、年份和冲突见下方。")
    if "truncated" in keys:
        facts.append(f"候选未查完：目前只返回 {len(candidates)} 条，不能据此确认作品。")
    if "other" in keys:
        if result.get("error"):
            facts.append("请求失败：" + result["error"])
        elif not candidates:
            facts.append("未找到影视候选：需要确认片名、年份或 TMDB ID。")
        else:
            facts.append("输入线索或候选信息需要确认，具体问题见候选对照。")
    return facts


def action_text(row, keys):
    if row["tmdb"]["status"] == "compatible":
        if row['tmdb'].get('selection_basis') == 'tmdb_order':
            return "同名同年的候选按 TMDB 返回顺序采用第一项；片名、原名和年份使用该作品的 TMDB 信息，其他候选与文件差异保留。"
        if selected_resolved(row):
            return "采用 TMDB 的片名、原名和年份。文件的原始名称及年份差异保留，命名预览使用 TMDB 信息。"
        return "当前选中候选没有发现字段冲突；这不是人工确认结果。"
    actions = []
    if "multiple" in keys or "truncated" in keys:
        actions.append("先确认正确作品（片名、原名、年份或 TMDB ID）")
    if "title" in keys:
        actions.append("对照发行名与 TMDB 原名 / 别名，确认是否同一部作品")
    if "release" in keys:
        actions.append("对照发行片名与候选原名，先确认文件实际属于哪部作品")
    if "input" in keys:
        actions.append("核对整理名与发行名的年份来源，确认应采用哪个年份")
    elif "year" in keys:
        candidates = row['tmdb'].get('candidates') or []
        year = row['tmdb']['meta_info'].get('year') or 0
        if len(candidates) == 1 and year and candidates[0]['media_info'].get('year') and abs(year - candidates[0]['media_info']['year']) >= 3:
            actions.append("对照发行片名、候选原名和剧情确认作品身份，别名命中也不能忽略年份差异")
        else:
            actions.append("核对作品版本及上映日期，确认年份来源")
    if "type" in keys or "id" in keys:
        actions.append("核对文件标记的类型与数据库 ID")
    return "；".join(unique(actions)) + "。" if actions else "确认片名、年份或明确的 TMDB ID 后重新查询。"


def match_basis(meta, media):
    ids = meta.get("ids") or {}
    if meta.get("type") == media.get("type") and meta.get("type") in ("movie", "tv"):
        for field in ("tmdb", "imdb", "tvdb", "douban", "bangumi", "anilist"):
            if ids.get(field) and str(ids[field]) == str(media.get(field + "_id", "")):
                return f"片名依据：按文件 {field.upper()} ID {ids[field]} 找到此候选。"
    inputs = unique(meta.get("title", "").split(" / ") + (meta.get("aliases") or []))
    names = [("名称", media.get("title", "")), ("原名", media.get("original_title", ""))]
    names.extend(("别名 / 译名", name) for name in media.get("aliases") or [])
    for value in inputs:
        for label, name in names:
            if normalized(value) and normalized(value) == normalized(name):
                return f"片名依据：输入「{value}」匹配 TMDB {label}「{name}」。"
    return "片名依据：本地名称与 TMDB 不同；选定候选后采用 TMDB 名称和原名，差异保留为记录。"


def conflict_label(conflict):
    field = conflict["field"]
    expected, actual = conflict.get("expected", ""), conflict.get("actual", "")
    if field == "input" and actual == "conflicting_years":
        return "文件内部年份冲突"
    if field == "title":
        return "片名未精确匹配"
    if field == "release_title":
        return f"发行片名 {expected} ↔ 候选原名 {actual}"
    if field == "type":
        kinds = {"movie": "电影", "tv": "剧集", "unknown": "未明确"}
        return f"类型 {kinds.get(expected, expected)} ↔ {kinds.get(actual, actual)}"
    if field == "year":
        return f"年份 {expected or '缺失'} ↔ {actual if actual and actual != '0' else '缺失'}"
    if field.endswith("_id"):
        return f"{field[:-3].upper()} ID {expected or '缺失'} ↔ {actual or '缺失'}"
    return f"输入问题：{actual}" if field == "input" else f"{field}：{expected} ↔ {actual}"


def media_details(media):
    aliases = media.get("aliases") or []
    preview = {key: value for key, value in media.items() if key not in ("raw", "credits")}
    return (f'<details class="media-details"><summary>查看全部别名、译名与影视详情（{len(aliases)} 个名称）</summary>'
            + ('<p class="alias-list">' + " / ".join(h(name) for name in aliases) + "</p>" if aliases else "")
            + '<pre>' + h(json.dumps(preview, ensure_ascii=False, indent=2)) + '</pre></details>')


def candidate_html(row, candidate, clues, index):
    meta, media = row["tmdb"]["meta_info"], candidate["media_info"]
    chosen = row['tmdb'].get('media_info') or {}
    selected = (row['tmdb']['status'] == 'compatible' and media['tmdb_id'] == chosen.get('tmdb_id')
                and media['type'] == chosen.get('type'))
    conflicts = candidate.get("conflicts") or []
    resolved = candidate.get("resolved_conflicts") or []
    fields = {f["field"] for f in conflicts}
    title_class = "mismatch" if "title" in fields else ""
    year_class = "mismatch" if fields & {"year", "input"} else ""
    kind = "剧集" if media["type"] == "tv" else "电影"
    movie_url = "https://www.themoviedb.org/" + media["type"] + "/" + str(media["tmdb_id"])
    tags = ''.join('<span class="tag warn">' + h(conflict_label(f)) + '</span>' for f in conflicts)
    for difference in resolved:
        label = "文件内部年份差异" if difference['field'] == 'input' else conflict_label(difference)
        disposition = '已按 TMDB 处理' if selected else '可按 TMDB 处理'
        tags += '<span class="tag good">' + h(label + ' · ' + disposition) + '</span>'
    if not tags:
        tags = '<span class="tag good">该候选无字段冲突</span>'
    file_title = '<strong class="' + title_class + '">' + h(meta.get("title") or "未解析出片名") + '</strong>'
    if clues["release"]:
        file_title += '<div class="sub">发行名：' + highlight_years(clues["release"]) + '</div>'
    if clues["prefix"]:
        file_title += '<div class="sub">整理名：' + highlight_years(clues["prefix"]) + '</div>'
    if clues["directory"] and clues["directory"] != clues["prefix"]:
        file_title += '<div class="sub">目录名：' + highlight_years(clues["directory"]) + '</div>'
    tmdb_title = '<strong class="' + title_class + '">' + h(media["title"]) + '</strong>'
    tmdb_title += '<div>原名：<bdi>' + h(media.get("original_title") or "未提供") + '</bdi></div>'
    if media.get("en_title") and media["en_title"] != media.get("original_title"):
        tmdb_title += '<div class="sub">英文名：' + h(media["en_title"]) + '</div>'
    if resolved:
        tmdb_title += '<div><span class="tag good">' + ('采用 TMDB 名称 / 原名' if selected else '选定后采用 TMDB 名称 / 原名') + '</span></div>'
    file_year = '<strong class="' + year_class + '">' + h(meta.get("year") or "未提供") + '</strong><span class="sub inline">（解析采用）</span>'
    for label, key in (("整理名", "prefix_years"), ("发行名", "release_years")):
        if clues[key]:
            file_year += '<div>' + label + '：<span class="' + year_class + '">' + h(" / ".join(clues[key])) + '</span></div>'
    if not clues["prefix_years"] and clues["directory_years"]:
        file_year += '<div>目录名：' + h(" / ".join(clues["directory_years"])) + '</div>'
    if resolved:
        file_year += '<div class="sub">' + ('文件线索保留；命名采用 TMDB 影视信息' if selected else '文件线索保留；此候选尚未采用') + '</div>'
    date = media.get("first_air_date") if media["type"] == "tv" else media.get("release_date")
    tmdb_year = '<strong class="' + ('mismatch' if 'year' in fields else '') + '">' + h(media.get("year") or "未提供") + '</strong>'
    if date:
        tmdb_year += '<div class="sub">' + ("首播日期" if media["type"] == "tv" else "上映日期") + '：' + h(date) + '</div>'
    if resolved and media.get('year'):
        disposition = '最终采用 ' if selected else '候选年份 '
        tmdb_year += '<div><span class="tag good">' + disposition + h(media.get('year')) + '</span></div>'
    extra = ''
    for field in sorted(fields - {"input", "title", "year"}):
        conflict = next(f for f in conflicts if f["field"] == field)
        label = "发行片名 / 原名" if field == "release_title" else "影视类型" if field == "type" else field[:-3].upper() + " ID" if field.endswith("_id") else field
        extra += '<tr class="field-difference"><th scope="row">' + h(label) + '</th><td class="mismatch">' + h(conflict.get("expected") or "未提供") + '</td><td class="mismatch">' + h(conflict.get("actual") or "未提供") + '</td></tr>'
    for difference in resolved:
        if difference['field'] not in ('title', 'release_title'):
            continue
        label = '发行片名 / 原名' if difference['field'] == 'release_title' else '片名差异'
        disposition = '已按 TMDB 采用' if selected else '选定后按 TMDB 采用'
        extra += '<tr class="field-resolved"><th scope="row">' + h(label) + '</th><td>' + h(difference.get('expected') or '未提供') + '</td><td><strong>' + h(difference.get('actual') or '未提供') + '</strong><div><span class="tag good">' + disposition + '</span></div></td></tr>'
    disposition = '已采用候选' if selected else '未采用候选' if row['tmdb']['status'] == 'compatible' else '候选'
    return (f'<section class="candidate" data-tmdb-id="{h(media["tmdb_id"])}" data-selected="{str(selected).lower()}">'
            f'<div class="candidate-head"><span>{disposition} {index} · {kind} · TMDB <a href="{h(movie_url)}" target="_blank" rel="noreferrer">#{h(media["tmdb_id"])}</a></span><div class="tags">{tags}</div></div>'
            '<table class="comparison"><thead><tr><th>核对项</th><th>文件 / 目录线索</th><th>TMDB 候选信息</th></tr></thead><tbody>'
            f'<tr><th scope="row">片名 / 原名</th><td>{file_title}</td><td>{tmdb_title}</td></tr>'
            f'<tr><th scope="row">年份</th><td>{file_year}</td><td>{tmdb_year}</td></tr>{extra}</tbody></table>'
            '<p class="basis">' + h(match_basis(meta, media)) + '</p>' + media_details(media) + '</section>')


def compact_candidates(row, candidates, clues, start):
    rows = []
    for index, candidate in enumerate(candidates, start):
        media = candidate["media_info"]
        conflicts = '；'.join(conflict_label(f) for f in candidate.get("conflicts") or [])
        if not conflicts:
            conflicts = '名称与年份可按 TMDB 采用，待确认候选' if candidate.get('resolved_conflicts') else '无字段冲突'
        url = 'https://www.themoviedb.org/' + media['type'] + '/' + str(media['tmdb_id'])
        year_class = 'mismatch' if any(f['field'] == 'year' for f in candidate.get('conflicts') or []) else ''
        rows.append(f'<tr><td>{index}</td><td><a href="{h(url)}" target="_blank" rel="noreferrer">{h(media["title"])} · #{h(media["tmdb_id"])}</a><div class="sub">原名：{h(media.get("original_title", ""))}</div></td><td class="{year_class}">{h(media.get("year") or "未提供")}</td><td>{h(conflicts)}{media_details(media)}</td></tr>')
    return (f'<details class="remaining"><summary>查看其余 {len(candidates)} 个候选及各自冲突</summary>'
            '<div class="table-scroll"><table class="candidate-list"><thead><tr><th>候选</th><th>片名 / 原名 / ID</th><th>年份</th><th>需要核对</th></tr></thead><tbody>'
            + ''.join(rows) + '</tbody></table></div></details>')


def selected_resolved(row):
    result = row['tmdb']
    if result['status'] != 'compatible' or not result.get('media_info'):
        return []
    media = result['media_info']
    return next((c.get('resolved_conflicts') or [] for c in result.get('candidates', [])
                 if c['media_info']['tmdb_id'] == media['tmdb_id'] and c['media_info']['type'] == media['type']), [])


def selected_adopted(row):
    result = row['tmdb']
    return (result['status'] == 'compatible' and bool(result.get('media_info'))
            and (bool(selected_resolved(row)) or result.get('selection_basis') == 'tmdb_order'))


def card_html(row):
    result, meta = row["tmdb"], row["tmdb"]["meta_info"]
    keys, clues = reason_keys(row), source_clues(row)
    facts = reason_text(row, keys, clues)
    candidates, discarded = candidates_for(row)
    is_review = result["status"] != "compatible"
    adopted = selected_adopted(row)
    active = is_review or adopted
    heading = result['media_info'] if adopted else meta
    tags = ''.join('<span class="tag warn">' + h(REASONS[key]) + '</span>' for key in keys)
    main = '<div class="review-focus"><strong>需要你核对什么</strong><ul>' + ''.join('<li>' + h(fact) + '</li>' for fact in facts) + '</ul></div>' if facts else ''
    if adopted:
        media = result['media_info']
        year = ' (' + h(media['year']) + ')' if media.get('year') else ''
        main = '<div class="resolution-focus"><strong>已按 TMDB 采用：' + h(media['title']) + year + '</strong>'
        main += '<p>原名：<strong><bdi>' + h(media.get('original_title') or '未提供') + '</bdi></strong>'
        if media.get('year'):
            main += ' · 年份：<strong>' + h(media['year']) + '</strong>'
        main += '。片名、发行片名与年份的不同写法已按 TMDB 处理，不再单独列入待核对。</p>'
        if result.get('selection_basis') == 'tmdb_order':
            main += '<p><strong>采用依据：</strong>同名同年候选按 TMDB 返回顺序选择第一项（#' + h(media['tmdb_id']) + '），其他候选保留在下方。</p>'
        main += '</div>'
    correction = row.get('recognition_correction') or {}
    if correction.get('description'):
        main += '<p class="basis"><strong>识别修正：</strong>' + h(correction['description']) + '</p>'
    main += '<p class="action"><strong>' + ('确认事项：' if is_review else '当前结果：') + '</strong>' + h(action_text(row, keys)) + '</p>'
    main += ''.join(candidate_html(row, c, clues, index) for index, c in enumerate(candidates[:3], 1))
    if len(candidates) > 3:
        main += compact_candidates(row, candidates[3:], clues, 4)
    if discarded:
        main += '<details class="remaining"><summary>其他搜索结果（' + str(len(discarded)) + ' 个，未选中）</summary>' + ''.join(candidate_html(row, c, clues, index) for index, c in enumerate(discarded, 1)) + '</details>'
    mp2 = unique((c.get('media') or {}).get('title', '') + ' · ' + str((c.get('media') or {}).get('year', '')) + ' · TMDB #' + str((c.get('media') or {}).get('tmdb_id', '')) for c in row.get('mp2_enhanced', []) if c.get('media'))
    source_detail = '<p><strong>完整文件名：</strong>' + highlight_years(row['name']) + '</p><p><strong>完整路径：</strong>' + h(row.get('path', '')) + '</p>'
    source_detail += '<p><strong>MP2 增强候选：</strong>' + h('；'.join(mp2) or '未返回候选') + '</p>'
    source_detail += '<pre>' + h(json.dumps(meta, ensure_ascii=False, indent=2)) + '</pre>'
    main += '<details class="source"><summary>完整文件名、路径、MP2 结果与解析详情</summary>' + source_detail + '</details>'
    searchable = [row['name'], row.get('path', ''), json.dumps(meta, ensure_ascii=False), *facts, *mp2]
    for candidate in result.get('candidates') or []:
        media = candidate['media_info']
        searchable.append(json.dumps({key: media.get(key) for key in (
            'title', 'original_title', 'en_title', 'aliases', 'all_titles',
            'year', 'type', 'tmdb_id', 'imdb_id', 'tvdb_id',
        )}, ensure_ascii=False))
    search_text = ' '.join(searchable).lower()
    return (f'<article id="line-{h(row["line"])}" class="record {"needs-review" if is_review else "compatible"}" data-status="{h(result["status"])}" data-adopted="{str(adopted).lower()}" data-active="{str(active).lower()}" data-reasons="{h(" ".join(keys))}" data-search="{h(search_text)}"{("" if active else " hidden")}>'
            '<div class="record-head"><div><div class="eyebrow">目录树第 ' + h(row['line']) + ' 行</div><h2><bdi>' + h(heading.get('title') or row['name']) + '</bdi>' + (' <span class="heading-year">(' + h(heading['year']) + ')</span>' if heading.get('year') else '') + '</h2></div>'
            '<span class="status ' + ('warn' if is_review else 'good') + '">' + h('已按 TMDB 采用' if adopted else STATUS.get(result['status'], result['status'])) + '</span></div><div class="tags row-tags">' + tags + '</div>' + main + '</article>')


def render(report, json_name):
    records = report['records']
    review_count = sum(row['tmdb']['status'] != 'compatible' for row in records)
    adopted_count = sum(selected_adopted(row) for row in records)
    counts = Counter(key for row in records for key in reason_keys(row))
    created = datetime.fromisoformat(report['created_at']).strftime('%Y-%m-%d %H:%M')
    template = Path(__file__).with_name('tmdb_review.html').read_text(encoding='utf-8')
    values = {
        'CREATED': h(created), 'TOTAL': str(len(records)), 'REVIEW': str(review_count),
        'COMPATIBLE': str(len(records) - review_count), 'JSON_FILE': h(json_name),
        'ADOPTED': str(adopted_count), 'ACTIVE': str(review_count + adopted_count),
        'POLICY_NOTE': ' · 已根据保存的 TMDB 详情按当前规则重算' if report.get('reconciliation') else '',
        'REASON_OPTIONS': ''.join(f'<option value="{h(key)}">{h(label)}（{counts[key]} 条）</option>' for key, label in REASONS.items() if counts[key]),
        'CARDS': ''.join(card_html(row) for row in records),
    }
    for key, value in values.items():
        template = template.replace('@@' + key + '@@', value)
    return template


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('report', type=Path)
    args = parser.parse_args()
    report = json.loads(args.report.read_text(encoding='utf-8'))
    output = args.report.with_suffix('.html')
    temporary = output.with_suffix('.html.tmp')
    temporary.write_text(render(report, args.report.name), encoding='utf-8')
    temporary.replace(output)
    print(f'已生成 {output}（{len(report["records"])} 条，默认展示本次处理与待核对的记录）')


if __name__ == '__main__':
    main()
