package shadow

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestReadTreeEncodingsAndPaths(t *testing.T) {
	text := "|——测试\n| |-电影\n| | |-片名 (2024)\n| | | |-File.2024.mkv\n| | | |-poster.jpg\n| | |-folder.mkv\n| | | |-02.MP4\n| |-电视剧\n| | |-Other.S01E02.mkv\n"
	variants := map[string][]byte{"utf8": []byte(text)}
	for _, little := range []bool{true, false} {
		var order binary.ByteOrder = binary.BigEndian
		data := []byte{0xfe, 0xff}
		name := "utf16be"
		if little {
			order, data, name = binary.LittleEndian, []byte{0xff, 0xfe}, "utf16le"
		}
		for _, unit := range utf16.Encode([]rune(text)) {
			var pair [2]byte
			order.PutUint16(pair[:], unit)
			data = append(data, pair[:]...)
		}
		variants[name] = data
	}
	for name, data := range variants {
		t.Run(name, func(t *testing.T) {
			inputs, _, err := ReadTree(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(inputs) != 3 || inputs[0].Path != "测试/电影/片名 (2024)/File.2024.mkv" || inputs[0].Line != 4 || inputs[1].Name != "02.MP4" || inputs[2].Path != "测试/电视剧/Other.S01E02.mkv" {
				t.Fatalf("incorrect inputs: %#v", inputs)
			}
		})
	}
	for _, data := range [][]byte{[]byte("no files"), {0xff, 0xfe, 1}, {0xff, 0x81}, []byte("| | | |-jump.mkv")} {
		if _, _, err := ReadTree(data); err == nil {
			t.Fatalf("invalid tree accepted: %q", data)
		}
	}
}

func TestCompareCanonicalFieldsAndZero(t *testing.T) {
	input := Input{Name: "Show.S00E01.2024.4K.AMZN.WEB-DL.x265.10bit.DDP5.1.Atmos-GROUP.mkv"}
	remote := map[string]any{"name": "SHOW", "type": "电视剧", "year": "2024", "begin_season": json.Number("0"), "begin_episode": json.Number("1"),
		"episode_list": []any{json.Number("1")}, "resource_pix": "4k", "resource_type": "WEB-DL", "video_encode": "HEVC 10bit", "video_bit": "10bit",
		"audio_encode": "DDP 5.1 Atmos", "resource_team": "GROUP", "web_source": "AMZN"}
	record := Compare(input, remote, nil)
	if record.Status != "matched" {
		t.Fatalf("format-only differences: %#v", record.Differences)
	}
	remote["begin_season"] = nil
	record = Compare(input, remote, nil)
	if len(record.Differences) != 1 || record.Differences[0].Field != "season" || record.Differences[0].Local != 0 || record.Differences[0].MP2 != nil {
		t.Fatalf("zero/missing collapsed: %#v", record.Differences)
	}
}

func TestCompareRetainsRealDifferencesAndIDPrecision(t *testing.T) {
	input := Input{Name: "Film.2024.[tmdb=12345678901234567890].1080p.mkv"}
	remote := map[string]any{"name": "Film", "type": "电影", "year": "2024", "resource_pix": "1080p", "tmdbid": json.Number("12345678901234567890")}
	if record := Compare(input, remote, nil); record.Status != "matched" {
		t.Fatalf("long ID precision lost: %#v", record.Differences)
	}
	remote["year"] = "2023"
	remote["name"] = "Other Film"
	record := Compare(input, remote, nil)
	if len(record.Differences) != 2 {
		t.Fatalf("real differences hidden: %#v", record.Differences)
	}
	if record := Compare(input, nil, errors.New("network failed")); record.Status != "mp2_error" || record.Error != "network failed" || len(record.Differences) != 0 {
		t.Fatalf("error became ordinary difference: %#v", record)
	}
}

func TestClientExpectedParseBranchAndRecognitionStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "private-token" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/system/ruletest":
			if r.URL.Query().Get("title") == "bad" {
				w.Write([]byte(`{"success":false,"data":{}}`))
				return
			}
			if r.URL.Query().Get("title") == "active-rule" {
				w.Write([]byte(`{"success":true,"data":{"rulegroup":{},"meta_info":{"name":"Film"}}}`))
				return
			}
			w.Write([]byte(`{"success":false,"data":{"rulegroup":null,"media_info":null,"meta_info":{"name":"Film","tmdbid":12345678901234567890}}}`))
		case "/api/v1/media/recognize":
			if r.URL.Query().Get("title") == "failed" {
				w.Write([]byte(`{"meta_info":null,"media_info":null}`))
				return
			}
			w.Write([]byte(`{"meta_info":{},"media_info":{"title":"Film","year":"2024","source":"themoviedb","tmdb_id":123}}`))
		case "/api/v1/media/recognize_file":
			w.Write([]byte(`{"meta_info":{},"media_info":{"title":"Film","tmdb_id":123}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "private-token", HTTP: server.Client(), RuleGroup: "absent"}
	meta, err := client.ParseName(context.Background(), "Film & (2024)")
	if err != nil || text(meta["tmdbid"]) != "12345678901234567890" {
		t.Fatalf("parse response: %v, %#v", err, meta)
	}
	for _, name := range []string{"bad", "active-rule"} {
		if _, err := client.ParseName(context.Background(), name); err == nil {
			t.Fatalf("invalid branch accepted: %s", name)
		}
	}
	if got := client.Recognize(context.Background(), "failed", false); got.Status != "not_identified" {
		t.Fatalf("failure lost: %#v", got)
	}
	if got := client.Recognize(context.Background(), "Film", false); got.Status != "identified" || text(got.Media["tmdb_id"]) != "123" {
		t.Fatalf("identity lost: %#v", got)
	}
	if got := client.Recognize(context.Background(), "root/Film.mkv", true); got.Status != "identified" {
		t.Fatalf("path response: %#v", got)
	}
	client.Token = "wrong"
	if _, err := client.ParseName(context.Background(), "Film"); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("HTTP error not safely handled: %v", err)
	}
	client.Token = "private-token"
	server.Close()
	if _, err := client.ParseName(context.Background(), "Film"); err == nil || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("network error leaked URL: %v", err)
	}
}

func TestReportsPreserveFailuresAndEscapeHTML(t *testing.T) {
	record := Compare(Input{Name: "Film.2024.mkv", Path: "root/Film.2024.mkv", Line: 3}, map[string]any{"name": "Film", "type": "未知", "year": "2024"}, nil)
	record.Recognition = &Recognition{Status: "not_identified", Raw: map[string]any{"media_info": nil}}
	record.PathRecognition = &Recognition{Status: "identified", Media: map[string]any{"title": "Film", "tmdb_id": json.Number("123")}}
	record.Name += "</script><script>alert('bad')</script>"
	report := &Report{SchemaVersion: 1, Complete: true, Records: []Record{record}}
	directory := t.TempDir()
	if err := WriteReport(directory, report, "secret"); err != nil {
		t.Fatal(err)
	}
	if report.Summary.NotIdentified != 1 || report.Summary.PathRecovered != 1 || report.Summary.Recognized != 0 {
		t.Fatalf("recovery hid filename failure: %#v", report.Summary)
	}
	failures, err := os.ReadFile(filepath.Join(directory, "failures.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(failures), "mp2_name_not_identified") {
		t.Fatalf("missing failure reasons: %s", failures)
	}
	html, err := os.ReadFile(filepath.Join(directory, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>alert('bad')") || strings.Contains(string(html), "@@REPORT_JSON@@") {
		t.Fatal("unsafe or missing HTML data")
	}
	data, err := os.ReadFile(filepath.Join(directory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := DecodeReport(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Records[0].Recognition.Raw == nil || text(decoded.Records[0].PathRecognition.Media["tmdb_id"]) != "123" {
		t.Fatal("replay data lost")
	}
	report.Records[0].MP2["accidental_echo"] = "secret"
	if err := WriteReport(t.TempDir(), report, "secret"); err == nil {
		t.Fatal("credential echo persisted")
	}
}

func TestEnhancementUsesCleanQueryAndRetainsConflicts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("title") {
		case "Film.2024 {[type=movie]}":
			w.Write([]byte(`{"media_info":{"title":"Film","year":"2024","type":"电影","tmdb_id":123}}`))
		case "咒怨(美版).2004 {[type=movie]}":
			w.Write([]byte(`{"media_info":{"title":"咒怨","year":"2002","type":"电影"}}`))
		case "Error.2024 {[type=movie]}":
			http.Error(w, "unavailable", 503)
		case "Factory Girl.2006 {[type=movie]}":
			w.Write([]byte(`{"media_info":{"title":"工厂女孩","original_title":"Factory Girl","year":"2006","type":"电影"}}`))
		default:
			w.Write([]byte(`{"media_info":null}`))
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "private", HTTP: server.Client()}
	for _, tt := range []struct{ name, status string }{
		{"Film (2024) - Film.2024.1080p-404.mkv", "compatible"},
		{"[咒怨(美版) 2004][原盘].mkv", "review"},
		{"Missing.2024.mkv", "no_candidate"},
		{"Error.2024.mkv", "request_error"},
		{"1080p.mkv", "no_query"},
		{"纵情女郎 (2006) - Factory.Girl.2006.1080p.mkv", "compatible"},
	} {
		baseline := Record{Input: Input{Name: tt.name}, Recognition: &Recognition{Status: "not_identified"}}
		got := Enhance(context.Background(), client, baseline)
		if got.Status != tt.status || got.Baseline.Status != "not_identified" {
			t.Fatalf("unexpected enhancement: %#v", got)
		}
		if tt.status == "review" && len(got.Attempts[0].Conflicts) != 2 {
			t.Fatal("conflicts lost", got)
		}
	}
}
