package rename_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"medianame"
	"medianame/rename"
)

func TestUserMovieTemplate(t *testing.T) {
	info := medianame.Parse("Dirty.Angels.2024.1080p.WEB-DL.H265.DDP5.1.AMZN-404.mkv")
	context := rename.BuildContext(info, rename.Media{Title: "肮脏天使", EnglishTitle: "Dirty Angels", Year: 2024,
		Type: medianame.TypeMovie, IDs: medianame.MediaIDs{TMDB: "1043905"}})
	result, err := rename.Render(rename.DefaultMovieTemplate, context)
	want := "肮脏天使 (2024) {tmdb-1043905}/Dirty Angels.2024.WEB-DL.1080p.H.265.DDP 5.1.Amazon-404.mkv"
	if err != nil || result == nil || result.Path != want || result.Name != strings.Split(want, "/")[1] ||
		result.Directory != "肮脏天使 (2024) {tmdb-1043905}" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestUserTVTemplateAndEpisodeBoundaries(t *testing.T) {
	for _, tt := range []struct{ name, season, episode, part string }{
		{"Show.S00E00.mkv", "00", "S00E00", ""},
		{"Show.S01E02-E05.mkv", "01", "S01E02-E05", ""},
		{"Show.S01E01E03E05.mkv", "01", "S01E01E03E05", ""},
		{"Show.S02E03.part2.mkv", "02", "S02E03", ".part2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			context := rename.BuildContext(medianame.Parse(tt.name), rename.Media{Title: "示例剧", EnglishTitle: "Example Show",
				Year: 2024, Type: medianame.TypeTV, IDs: medianame.MediaIDs{TMDB: "123"}})
			result, err := rename.Render(rename.DefaultTVTemplate, context)
			want := "示例剧 (2024) {tmdb-123}/Season " + tt.season + "/Example Show.2024." + tt.episode + tt.part + ".mkv"
			if err != nil || result == nil || result.Path != want {
				t.Fatalf("want=%q result=%+v err=%v", want, result, err)
			}
		})
	}
}

func TestCustomGoTemplateAndSafeNames(t *testing.T) {
	context := rename.BuildContext(medianame.Parse("Film.2024.mkv"), rename.Media{Title: `A/B: C?`, OriginalTitle: "原名", Category: "华语电影"})
	context["season_episode"] = "S01 E08"
	context["missing"] = ""
	result, err := rename.Render(`{{.category}}/{{if .missing}}bad{{else if .year}}{{.title}}{{else}}unknown{{end}}.{{.season_episode | replace " " ""}}.{{.original_title | lower}}{{.fileExt}}`, context)
	if err != nil || result.Path != "华语电影/A／B： C？.S01E08.原名.mkv" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if context["title"] != `A/B: C?` {
		t.Fatal("caller context was modified")
	}
	result, err = rename.Render(`{{.en_title}}{{.fileExt}}`, context)
	if err != nil || result.Name != "原名.mkv" {
		t.Fatalf("fallback=%+v err=%v", result, err)
	}
}

func TestTemplateAndPathErrors(t *testing.T) {
	for _, format := range []string{"", "{{if .title}}", "{{ .title | no_such_function }}", "{{.missing}}", "{{int .missing}}", "{{int .title}}", "/outside.mkv", "../outside.mkv", "a//b.mkv", "a/./b.mkv", "C:/outside.mkv", "a\x00.mkv", `{{template "external" .}}`, `{{title}}`, `{% if edition %}x{% endif %}`} {
		t.Run(fmt.Sprintf("%q", format), func(t *testing.T) {
			if result, err := rename.Render(format, rename.Context{"title": "Film"}); err == nil {
				t.Fatalf("accepted invalid template/path: %+v", result)
			}
		})
	}
}

func TestCompiledTemplateConcurrentIsolation(t *testing.T) {
	template, err := rename.Compile(`{{$label := .title | upper}}{{$label}}{{.fileExt}}`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			context := rename.Context{"title": fmt.Sprintf("film%d", i), "fileExt": ".mkv"}
			result, err := template.Render(context)
			if err != nil || result.Name != fmt.Sprintf("FILM%d.mkv", i) {
				t.Errorf("result=%+v err=%v", result, err)
			}
			if _, exists := context["label"]; exists {
				t.Error("template changed input")
			}
		}(i)
	}
	wg.Wait()
}

func TestSourceExtensionAndCrossSeason(t *testing.T) {
	for _, name := range []string{"Show.S01E02-S02E03.10bit.x265.mkv", "Show.S01E02-S02E02.10bit.x265.mkv"} {
		context := rename.BuildContext(medianame.Parse(name), rename.Media{})
		result, err := rename.Render(`{{.season_episode}}{{.fileExt}}`, context)
		want := "S01E02-S02E03.mkv"
		if strings.Contains(name, "S02E02") {
			want = "S01E02-S02E02.mkv"
		}
		if err != nil || result.Name != want {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	context := rename.BuildContext(medianame.Parse("Film.2024.10bit.x265"), rename.Media{})
	if context["fileExt"] != "" {
		t.Fatal("codec mistaken for extension")
	}
}
