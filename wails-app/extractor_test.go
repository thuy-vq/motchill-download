package main

import (
	"os"
	"strings"
	"testing"
)

func TestExtractEmbeddedStreamsAndEpisodes(t *testing.T) {
	source := `<html><head><title>Xem phim Demo Tập 1 - Motchill</title>` +
		`<link rel="canonical" href="https://site.example/phim/demo/tap-1"></head>` +
		`<body><a href="/phim/demo/tap-2">Tập 2</a><a href="/phim/demo/tap-1">Tập 1</a>` +
		`<script>self.push("episode_data_count\":4,\"episodes\":[{\"name\":\"Dữ liệu mặc định\",\"server\":\"Sai\",\"link\":\"https:\/\/cdn.example/v/wrong.m3u8\"}],` +
		`\"episodeVariants\":[{\"name\":\"Tập 01\",` +
		`\"server\":\"Server A\",\"link\":\"https:\/\/cdn.example/v/master.m3u8\"}]")</script></body></html>`

	streams := extractCurrentStreams(source, "https://site.example/phim/demo/tap-1")
	if len(streams) != 1 || streams[0].URL != "https://cdn.example/v/master.m3u8" || streams[0].Server != "Server A" {
		t.Fatalf("unexpected streams: %#v", streams)
	}
	episodes := extractEpisodeLinks(source, "https://site.example/phim/demo/tap-1")
	if len(episodes) != 2 || episodes[0].Number != 1 || episodes[1].Number != 2 || !episodes[0].Current {
		t.Fatalf("unexpected episodes: %#v", episodes)
	}
	if title := extractTitle(source); title != "Demo" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestEpisodeOneAndTwoNeverShareStreamIdentity(t *testing.T) {
	first := streamIdentity("https://cdn.example/series/episode-1/master.m3u8?token=abc")
	second := streamIdentity("https://cdn.example/series/episode-2/master.m3u8?token=def")
	if first == second {
		t.Fatal("different episode paths must not share an identity")
	}
	sameWithNewToken := streamIdentity("https://cdn.example/series/episode-1/master.m3u8?token=new")
	if first != sameWithNewToken {
		t.Fatal("signed variants of the same stream path should be treated as the same stream")
	}
}

func TestExtractMovieIDFromSeriesLandingPage(t *testing.T) {
	source := `<script>self.__next_f.push([1,"{\"movieId\":\"82758\",\"episodes\":[{\"movie_id\":\"99999\"}]}"])</script>`
	if got := extractMovieID(source); got != "82758" {
		t.Fatalf("expected landing-page movie id 82758, got %q", got)
	}
}

func TestLiveSeriesLandingPageFindsEveryEpisode(t *testing.T) {
	page := os.Getenv("MOTCHILL_LIVE_SERIES")
	if page == "" {
		t.Skip("MOTCHILL_LIVE_SERIES is not set")
	}
	source, err := fetchHTML(page)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analyzeDocument(source, page, page)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Episodes) != 23 {
		t.Fatalf("expected 23 episodes, got %d", len(result.Episodes))
	}
	if result.Streams == nil {
		t.Fatal("landing-page streams must serialize as [] instead of null")
	}
	if result.Episodes[0].PageURL != page+"/tap-1" || result.Episodes[22].PageURL != page+"/tap-23" {
		t.Fatalf("unexpected episode URLs: first=%s last=%s", result.Episodes[0].PageURL, result.Episodes[22].PageURL)
	}
	if result.Episodes[0].StreamURL == result.Episodes[1].StreamURL {
		t.Fatal("episode 1 and 2 must not share a stream URL")
	}
}

func TestLiveEpisodeOneAndTwoResolveDifferently(t *testing.T) {
	value := os.Getenv("MOTCHILL_LIVE_EPISODES")
	if value == "" {
		t.Skip("MOTCHILL_LIVE_EPISODES is not set")
	}
	pages := strings.Split(value, "|")
	if len(pages) != 2 {
		t.Fatal("MOTCHILL_LIVE_EPISODES must contain two URLs separated by |")
	}
	identities := make([]string, 2)
	for index, page := range pages {
		source, err := fetchHTML(page)
		if err != nil {
			t.Fatal(err)
		}
		movieID := extractMovieID(source)
		episodes, err := fetchEpisodeIndex(page, movieID)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s -> movie %s -> %d episode buttons", page, movieID, len(episodes))
		if len(episodes) != 21 {
			t.Fatalf("expected 21 episode links for %s, got %d", page, len(episodes))
		}
		streams := extractCurrentStreams(source, page)
		if len(streams) == 0 {
			t.Fatalf("no stream for %s", page)
		}
		identities[index] = streamIdentity(streams[0].URL)
		t.Logf("%s -> %s", page, streams[0].URL)
	}
	if identities[0] == identities[1] {
		t.Fatal("episode 1 and episode 2 resolved to the same media stream")
	}
}

func TestProvidedHTMLSample(t *testing.T) {
	path := os.Getenv("MOTCHILL_SAMPLE_HTML")
	if path == "" {
		t.Skip("MOTCHILL_SAMPLE_HTML is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pageURL := extractPageURL(string(data))
	streams := extractCurrentStreams(string(data), pageURL)
	episodes := extractEpisodeLinks(string(data), pageURL)
	if len(streams) != 1 {
		t.Fatalf("expected 1 directly downloadable current-episode server, got %d", len(streams))
	}
	if len(episodes) != 21 {
		t.Fatalf("expected 21 episode links, got %d", len(episodes))
	}
	if episodes[0].Number != 1 || episodes[20].Number != 21 {
		t.Fatalf("episode order is wrong: first=%d last=%d", episodes[0].Number, episodes[20].Number)
	}
}
