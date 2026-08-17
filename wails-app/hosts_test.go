package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// The Motchill family shares one CMS but每 host spells the player URL its own
// way. These are the shapes reported from real links.
var hostVariantURLs = []struct {
	name          string
	url           string
	episodePage   bool
	episodeNumber int
	seriesPath    string
}{
	{"motphimchill landing", "https://motphimchill.cc/phim/xieu-long-giang-sinh", false, 0, "/phim/xieu-long-giang-sinh"},
	{"motchill.credit server suffix", "https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0", true, 1, "/phim/xac-song-thanh-pho-chet-phan-3"},
	{"phimmoichill landing", "https://phimmoichill.hair/phim/luc-luong-tinh-nhue", false, 0, "/phim/luc-luong-tinh-nhue"},
	{"motphimchilll id suffix", "https://motphimchilll.me/phim/bao-mau-bi-mat-cua-tieu-thu/tap-1-3097673", true, 1, "/phim/bao-mau-bi-mat-cua-tieu-thu"},
	{"motphimchill full with language tail", "https://motphimchill.cc/xem-phim/khu-rung-bi-tham/tap-full/vietsub", true, 0, "/xem-phim/khu-rung-bi-tham"},
}

func TestHostVariantURLsAreUnderstood(t *testing.T) {
	for _, testCase := range hostVariantURLs {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isEpisodePage(testCase.url); got != testCase.episodePage {
				t.Fatalf("isEpisodePage = %v, want %v", got, testCase.episodePage)
			}
			if _, number, _ := episodePathSegment(testCase.url); number != testCase.episodeNumber {
				t.Fatalf("episode number = %d, want %d", number, testCase.episodeNumber)
			}
			if got := seriesPathOf(testCase.url); got != testCase.seriesPath {
				t.Fatalf("seriesPathOf = %q, want %q", got, testCase.seriesPath)
			}
		})
	}
}

func TestMovieSlugWithDigitsIsNotAnEpisodePage(t *testing.T) {
	// "phan-3" and a slug containing "tap" must not look like a player page.
	for _, value := range []string{
		"https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3",
		"https://motphimchill.cc/phim/dao-hai-tap-hop-2023",
	} {
		if isEpisodePage(value) {
			t.Fatalf("%s must be treated as a landing page", value)
		}
	}
}

func TestEpisodeLinksAcrossHostVariants(t *testing.T) {
	// A player page whose episode segment carries a server suffix, listing the
	// same episode twice for two servers.
	source := `<html><head><title>Xem phim Xác Sống Thành Phố Chết Phần 3 Tập 1 - Motchill</title>` +
		`<link rel="canonical" href="https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0"></head><body>` +
		`<a href="/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0">Tập 1</a>` +
		`<a href="/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-1">Tập 1 SV2</a>` +
		`<a href="/phim/xac-song-thanh-pho-chet-phan-3/tap-2-sv-0">Tập 2</a>` +
		`<a href="/phim/phim-khac/tap-9-sv-0">Phim khác</a></body></html>`
	episodes := extractEpisodeLinks(source, "https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0")
	if len(episodes) != 2 {
		t.Fatalf("want 2 episodes after collapsing servers, got %#v", episodes)
	}
	if episodes[0].Number != 1 || !episodes[0].Current || episodes[1].Number != 2 {
		t.Fatalf("unexpected episodes: %#v", episodes)
	}
	if !strings.HasSuffix(episodes[0].PageURL, "/tap-1-sv-0") {
		t.Fatalf("the current episode must keep the link that is open: %s", episodes[0].PageURL)
	}
	if title := extractTitle(source); title != "Xác Sống Thành Phố Chết Phần 3" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestSinglePartFilmKeepsItsStream(t *testing.T) {
	// /tap-full/vietsub used to read as a landing page, which threw the stream
	// away and left "không tìm thấy luồng".
	pageURL := "https://motphimchill.cc/xem-phim/khu-rung-bi-tham/tap-full/vietsub"
	source := `<html><head><title>Xem phim Khu Rừng Bí Thảm Vietsub - Motphimchill</title>` +
		`<link rel="canonical" href="` + pageURL + `"></head><body>` +
		`<a href="/xem-phim/khu-rung-bi-tham/tap-full/vietsub">Full</a>` +
		`<script>var data = "\"episodeVariants\":[{\"name\":\"Full\",\"server\":\"Vietsub #1\",` +
		`\"link\":\"https:\/\/cdn.example\/khu-rung\/index.m3u8\"}]"</script></body></html>`

	result, err := analyzeDocument(source, pageURL, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Streams) != 1 || result.Streams[0].URL != "https://cdn.example/khu-rung/index.m3u8" {
		t.Fatalf("single-part film must keep its stream, got %#v", result.Streams)
	}
	if len(result.Episodes) != 1 || result.Episodes[0].Number != 0 || result.Episodes[0].ID != "episode-full" {
		t.Fatalf("unexpected episodes: %#v", result.Episodes)
	}
	if title := extractTitle(source); title != "Khu Rừng Bí Thảm Vietsub" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestLandingPageDropsRecommendationStreams(t *testing.T) {
	// A landing page with a real episode index must offer the index, not the
	// media URL of whatever card the page happened to render.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/baseapi/episodes") {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "success", "servers": []any{map[string]any{
			"name": "Vietsub", "items": []any{
				map[string]any{"slug": "tap-1-sv-0", "name": "Tập 1", "type": "m3u8", "link": "https://cdn.example/e1/index.m3u8"},
				map[string]any{"slug": "tap-2-sv-0", "name": "Tập 2", "type": "m3u8", "link": "https://cdn.example/e2/index.m3u8"},
			}}}})
	}))
	defer server.Close()

	pageURL := server.URL + "/phim/luc-luong-tinh-nhue"
	source := `<html><head><title>Lực Lượng Tinh Nhuệ - PhimmoiChill</title>` +
		`<link rel="canonical" href="` + pageURL + `"></head><body>` +
		`<div data-props='{"movieId":"555"}'></div>` +
		`<script>var data = "\"episodeVariants\":[{\"name\":\"Tập 01\",\"server\":\"SV1\",` +
		`\"link\":\"https:\/\/cdn.example\/goi-y\/index.m3u8\"}]"</script></body></html>`

	result, err := analyzeDocument(source, pageURL, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Streams) != 0 {
		t.Fatalf("landing pages must not keep media from cards, got %#v", result.Streams)
	}
	if len(result.Episodes) != 2 || result.Episodes[0].Number != 1 {
		t.Fatalf("episodes should come from the index: %#v", result.Episodes)
	}
	if title := extractTitle(source); title != "Lực Lượng Tinh Nhuệ" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestPlayerPageWithoutEpisodeSegmentKeepsItsStream(t *testing.T) {
	// Some hosts play a single-part film straight from /xem-phim/<slug>. With no
	// episode index to replace it, the stream on the page must be kept.
	pageURL := "https://motphimchill.cc/xem-phim/khu-rung-bi-tham"
	source := `<html><head><title>Khu Rừng Bí Thảm - Motphimchill</title>` +
		`<link rel="canonical" href="` + pageURL + `"></head><body>` +
		`<script>var data = "\"episodeVariants\":[{\"name\":\"Full\",\"server\":\"SV1\",` +
		`\"link\":\"https:\/\/cdn.example\/khu-rung\/index.m3u8\"}]"</script></body></html>`
	result, err := analyzeDocument(source, pageURL, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Streams) != 1 {
		t.Fatalf("want the page stream kept, got %#v", result.Streams)
	}
	// Without an episode marker in the URL there is no episode to name, so the
	// single entry stays the generic one.
	if len(result.Episodes) != 1 || result.Episodes[0].ID != "current" || result.Episodes[0].Name != "Video" {
		t.Fatalf("unexpected fallback episode: %#v", result.Episodes)
	}
}

func TestEpisodeIndexUsesSlugsFromTheHost(t *testing.T) {
	// The API is the same across the family; only the slugs differ. The built
	// URLs must reuse those slugs instead of assuming "tap-N".
	payload := map[string]any{"status": "success", "servers": []any{map[string]any{
		"name": "Vietsub", "items": []any{
			map[string]any{"slug": "tap-1-3097673", "name": "Tập 1", "type": "m3u8", "link": "https://cdn.example/e1/index.m3u8"},
			map[string]any{"slug": "tap-2-3097674", "name": "Tập 2", "type": "m3u8", "link": "https://cdn.example/e2/index.m3u8"},
		},
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/baseapi/episodes") {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("movie_id") != "3097673" {
			http.Error(writer, "wrong movie", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(payload)
	}))
	defer server.Close()

	pageURL := server.URL + "/phim/bao-mau-bi-mat-cua-tieu-thu/tap-1-3097673"
	episodes, err := fetchEpisodeIndex(pageURL, "3097673")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("want 2 episodes, got %#v", episodes)
	}
	base, _ := url.Parse(server.URL)
	want := []string{
		base.String() + "/phim/bao-mau-bi-mat-cua-tieu-thu/tap-1-3097673",
		base.String() + "/phim/bao-mau-bi-mat-cua-tieu-thu/tap-2-3097674",
	}
	for index, episode := range episodes {
		if episode.PageURL != want[index] {
			t.Fatalf("episode %d URL = %s, want %s", index+1, episode.PageURL, want[index])
		}
		if episode.StreamURL == "" {
			t.Fatalf("episode %d lost its stream", index+1)
		}
	}
	if !episodes[0].Current {
		t.Fatal("the open episode must be marked as current")
	}
}

// TestLiveHostVariants checks real pages of any host list, e.g.
//
//	MOTCHILL_LIVE_HOSTS="https://motphimchill.cc/phim/xieu-long-giang-sinh|https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0" go test -run TestLiveHostVariants -v
//
// It needs network access, so it stays skipped by default.
func TestLiveHostVariants(t *testing.T) {
	value := os.Getenv("MOTCHILL_LIVE_HOSTS")
	if value == "" {
		t.Skip("MOTCHILL_LIVE_HOSTS is not set")
	}
	for _, page := range strings.Split(value, "|") {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		t.Run(page, func(t *testing.T) {
			source, err := fetchHTML(page)
			if err != nil {
				t.Fatalf("không tải được trang: %v", err)
			}
			result, err := analyzeDocument(source, page, page)
			if err != nil {
				t.Fatalf("phân tích thất bại: %v", err)
			}
			t.Logf("title=%q episodes=%d streams=%d movieID=%q episodePage=%v",
				result.Title, len(result.Episodes), len(result.Streams), extractMovieID(source), isEpisodePage(page))
			for index, episode := range result.Episodes {
				if index >= 3 {
					break
				}
				t.Logf("  #%d %s → %s (stream %q)", episode.Number, episode.Name, episode.PageURL, episode.StreamURL)
			}
			if len(result.Episodes) == 0 {
				t.Fatal("không tìm thấy tập nào")
			}
			if isEpisodePage(page) && len(result.Streams) == 0 {
				t.Fatal("trang tập phải có ít nhất một luồng")
			}
		})
	}
}

func TestEpisodeIndexKeepsEveryServerAsFallback(t *testing.T) {
	// One 404 server used to fail the episode outright because only the server
	// of the open page was read.
	payload := map[string]any{"status": "success", "servers": []any{
		map[string]any{"name": "Vietsub #1", "items": []any{
			map[string]any{"slug": "tap-1-sv-0", "name": "Tập 1", "type": "m3u8", "link": "https://cdn1.example/e1/index.m3u8"},
			map[string]any{"slug": "tap-2-sv-0", "name": "Tập 2", "type": "m3u8", "link": "https://cdn1.example/e2/index.m3u8"},
		}},
		map[string]any{"name": "Vietsub #2", "items": []any{
			map[string]any{"slug": "tap-1-sv-1", "name": "Tập 1", "type": "m3u8", "link": "https://cdn2.example/e1/index.m3u8"},
			map[string]any{"slug": "tap-2-sv-1", "name": "Tập 2", "type": "m3u8", "link": "https://cdn2.example/e2/index.m3u8"},
		}},
		map[string]any{"name": "Thuyết minh", "items": []any{
			map[string]any{"slug": "tap-1-sv-2", "name": "Tập 1", "type": "m3u8", "link": "https://cdn3.example/e1/index.m3u8"},
		}},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(payload)
	}))
	defer server.Close()

	// The open page belongs to the second server, which must be tried first.
	pageURL := server.URL + "/phim/xac-song/tap-1-sv-1"
	episodes, err := fetchEpisodeIndex(pageURL, "123")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("want 2 episodes, got %#v", episodes)
	}
	first := episodes[0]
	if len(first.Streams) != 3 {
		t.Fatalf("episode 1 must carry all three servers, got %#v", first.Streams)
	}
	if first.Streams[0].URL != "https://cdn2.example/e1/index.m3u8" {
		t.Fatalf("the server of the open page must come first, got %s", first.Streams[0].URL)
	}
	if first.StreamURL != first.Streams[0].URL {
		t.Fatalf("StreamURL must mirror the first candidate, got %s", first.StreamURL)
	}
	if first.Streams[0].Server != "Vietsub #2" || first.Streams[2].Server != "Thuyết minh" {
		t.Fatalf("server names were lost: %#v", first.Streams)
	}
	// Episode 2 exists on two servers only.
	if len(episodes[1].Streams) != 2 {
		t.Fatalf("episode 2 should have two servers, got %#v", episodes[1].Streams)
	}
	// The page URL keeps the slug of the server that page came from.
	if !strings.HasSuffix(first.PageURL, "/tap-1-sv-1") {
		t.Fatalf("unexpected episode URL: %s", first.PageURL)
	}
}

func TestResolveStreamsUsesKnownServersAndHonoursPreference(t *testing.T) {
	app := &App{}
	item := DownloadItem{
		ID: "movie::episode-1", Name: "Tập 01", Number: 1,
		PageURL:   "https://motchill.credit/phim/xac-song/tap-1-sv-0",
		StreamURL: "https://cdn1.example/e1/index.m3u8",
		Streams: []MediaStream{
			{URL: "https://cdn1.example/e1/index.m3u8", Kind: "HLS", Server: "Vietsub #1"},
			{URL: "https://cdn2.example/e1/index.m3u8", Kind: "HLS", Server: "Vietsub #2"},
			// A duplicate of the primary must not be tried twice.
			{URL: "https://cdn1.example/e1/index.m3u8", Kind: "HLS", Server: "Vietsub #1"},
		},
	}
	streams, err := app.resolveStreams(item, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 2 {
		t.Fatalf("want two unique servers, got %#v", streams)
	}
	preferred, err := app.resolveStreams(item, "Vietsub #2")
	if err != nil {
		t.Fatal(err)
	}
	if preferred[0].Server != "Vietsub #2" {
		t.Fatalf("preferred server must lead, got %#v", preferred)
	}
}

func TestNewStreamsSkipsWhatWasAlreadyTried(t *testing.T) {
	tried := []MediaStream{{URL: "https://cdn1.example/a.m3u8"}, {URL: "https://cdn2.example/b.m3u8"}}
	fresh := []MediaStream{
		{URL: "https://cdn1.example/a.m3u8"},
		{URL: "https://cdn9.example/new.m3u8?token=1"},
		{URL: "https://cdn9.example/new.m3u8?token=1"},
	}
	unseen := newStreams(fresh, tried)
	if len(unseen) != 1 || unseen[0].URL != "https://cdn9.example/new.m3u8?token=1" {
		t.Fatalf("unexpected fresh streams: %#v", unseen)
	}
}

func TestEpisodeIndexFindsMovieIDInSnakeCase(t *testing.T) {
	source := `<div data-props='{"movie_id":"98765","title":"Phim"}'></div>`
	if got := extractMovieID(source); got != "98765" {
		t.Fatalf("extractMovieID = %q, want 98765", got)
	}
}
