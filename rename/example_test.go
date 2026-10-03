package rename_test

import (
	"fmt"

	"medianame"
	"medianame/rename"
)

func ExampleRender() {
	info := medianame.Parse("Show.S00E01.mkv")
	context := rename.BuildContext(info, rename.Media{Title: "示例剧", EnglishTitle: "Example Show", Year: 2024,
		Type: medianame.TypeTV, IDs: medianame.MediaIDs{TMDB: "123"}, Category: "日韩剧集"})
	result, err := rename.Render(`{{.category}}/Season {{printf "%02d" (int .season)}}/{{.en_title}}.{{.season_episode}}{{.fileExt}}`, context)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.Path)
	// Output: 日韩剧集/Season 00/Example Show.S00E01.mkv
}
