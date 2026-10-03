// Package shadow 提供目录树导入、MP2 名称解析对照与报告输出。
package shadow

import (
	"encoding/binary"
	"fmt"
	"path"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Input 表示目录树中的一个视频文件；Path 是树中的逻辑路径。
type Input struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".ts": true, ".m2ts": true, ".mts": true, ".wmv": true, ".flv": true,
	".webm": true, ".mpg": true, ".mpeg": true, ".vob": true, ".iso": true,
	".strm": true,
}

// ReadTree 解码 UTF-8、UTF-16 LE/BE 目录树，并提取带视频扩展名的叶子节点。
// 文件中的文字仅作为样本数据，不作为命令或配置执行。
func ReadTree(data []byte) ([]Input, string, error) {
	text, encoding, err := decodeText(data)
	if err != nil {
		return nil, "", err
	}
	type node struct {
		name        string
		depth, line int
	}
	var nodes []node
	root := ""
	for index, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "|——") {
			root = strings.TrimSpace(strings.TrimPrefix(line, "|——"))
			continue
		}
		depth := 0
		for strings.HasPrefix(line, "| ") {
			depth++
			line = line[2:]
		}
		if strings.HasPrefix(line, "|-") {
			nodes = append(nodes, node{strings.TrimSpace(line[2:]), depth, index + 1})
		}
	}
	stack := []string{root}
	var inputs []Input
	for index, n := range nodes {
		if n.depth > len(stack) {
			return nil, encoding, fmt.Errorf("第 %d 行目录层级跳跃，无法还原路径", n.line)
		}
		stack = append(stack[:n.depth], n.name)
		isDirectory := index+1 < len(nodes) && nodes[index+1].depth > n.depth
		if !isDirectory && videoExtensions[strings.ToLower(path.Ext(n.name))] {
			inputs = append(inputs, Input{n.name, strings.TrimPrefix(strings.Join(stack, "/"), "/"), n.line})
		}
	}
	if len(inputs) == 0 {
		return nil, encoding, fmt.Errorf("目录树没有可测试的视频文件")
	}
	return inputs, encoding, nil
}

func decodeText(data []byte) (string, string, error) {
	if len(data) >= 2 && (data[0] == 0xff && data[1] == 0xfe || data[0] == 0xfe && data[1] == 0xff) {
		if len(data)%2 != 0 {
			return "", "", fmt.Errorf("UTF-16 文件字节数无效")
		}
		var order binary.ByteOrder = binary.LittleEndian
		encoding := "UTF-16 LE"
		if data[0] == 0xfe {
			order, encoding = binary.BigEndian, "UTF-16 BE"
		}
		units := make([]uint16, (len(data)-2)/2)
		for index := range units {
			units[index] = order.Uint16(data[2+index*2:])
		}
		return string(utf16.Decode(units)), encoding, nil
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	if !utf8.ValidString(text) {
		return "", "", fmt.Errorf("目录树需要 UTF-8 或带 BOM 的 UTF-16 编码")
	}
	return text, "UTF-8", nil
}
