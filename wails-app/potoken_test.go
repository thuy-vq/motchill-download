package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTuningArgsAreEmptyUntilConfigured(t *testing.T) {
	if args := ytDlpTuningArgs(YtDlpTuning{}); len(args) != 0 {
		t.Fatalf("an unconfigured app must add no arguments, got %v", args)
	}
}

func TestTuningArgsBuildPluginAndExtractorFlags(t *testing.T) {
	args := ytDlpTuningArgs(YtDlpTuning{
		PluginDir:    `D:\yt-dlp-plugins`,
		ProviderURL:  "http://127.0.0.1:4416/",
		Token:        "MnQ_ABC123",
		PlayerClient: "web_safari",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, `--plugin-dirs D:\yt-dlp-plugins`) {
		t.Fatalf("plugin dir missing: %q", joined)
	}
	// The trailing slash of the provider URL must not survive.
	if !strings.Contains(joined, "youtubepot-bgutilhttp:base_url=http://127.0.0.1:4416") ||
		strings.Contains(joined, "4416/") {
		t.Fatalf("provider argument looks wrong: %q", joined)
	}
	// Both YouTube settings belong to one --extractor-args value.
	youtube := ""
	for index, value := range args {
		if strings.HasPrefix(value, "youtube:") {
			youtube = value
			if args[index-1] != "--extractor-args" {
				t.Fatalf("youtube arguments must follow --extractor-args: %v", args)
			}
		}
	}
	if youtube != "youtube:player_client=web_safari;po_token=MnQ_ABC123" {
		t.Fatalf("unexpected youtube arguments: %q", youtube)
	}
}

func TestTuningArgsSkipTheFieldsLeftBlank(t *testing.T) {
	args := ytDlpTuningArgs(YtDlpTuning{PlayerClient: "tv"})
	if strings.Contains(strings.Join(args, " "), "po_token") {
		t.Fatalf("an empty token must not be sent: %v", args)
	}
	if strings.Join(args, " ") != "--extractor-args youtube:player_client=tv" {
		t.Fatalf("unexpected arguments: %v", args)
	}
}

// Sample of what yt-dlp --verbose prints when the provider plugin is loaded.
const verboseWithPlugin = `[debug] Command-line config: ['--verbose', '--simulate']
[debug] Plugin directories: ['D:\yt-dlp-plugins']
[debug] Extractor Plugins: BgUtilHTTPPOTokenProvider (YoutubeIE)
[debug] [youtube] [pot] Fetching GVS PO Token for web client
[info] jNQXAC9IVRw: Downloading 1 format(s): 137+251`

const verboseWithoutPlugin = `[debug] Command-line config: ['--verbose', '--simulate']
[debug] Loaded 1856 extractors
WARNING: [youtube] Some web client https formats have been skipped as they are missing a url. YouTube is forcing SABR streaming
[debug] [youtube] no PO token provider available`

func TestParsePluginCheckSeesTheProvider(t *testing.T) {
	check := parsePluginCheck(verboseWithPlugin)
	if !check.PluginsFound || len(check.Plugins) != 1 {
		t.Fatalf("plugin not detected: %+v", check)
	}
	if !strings.Contains(check.Plugins[0], "BgUtilHTTPPOTokenProvider") {
		t.Fatalf("unexpected plugin name: %v", check.Plugins)
	}
	if !check.TokenUsed {
		t.Fatal("a fetched PO token must be reported as used")
	}
}

func TestParsePluginCheckReportsAMissingProvider(t *testing.T) {
	check := parsePluginCheck(verboseWithoutPlugin)
	if check.PluginsFound {
		t.Fatalf("no plugin should be reported: %+v", check)
	}
	// "no PO token provider available" is the absence of a token, not its use.
	if check.TokenUsed {
		t.Fatal("a warning about a missing token must not count as a token")
	}
	if check.Plugins == nil {
		t.Fatal("plugins must serialise as [] rather than null")
	}
}

func TestPOTokenArgsLabelTheClient(t *testing.T) {
	token := mintedPOToken{Token: "TOK123", Binding: "VD456"}
	// With no session of its own, the token's own binding is the visitor data.
	args := poTokenArgs("", "", token)
	if len(args) != 2 || args[0] != "--extractor-args" {
		t.Fatalf("unexpected shape: %v", args)
	}
	// The default client is the one whose media URLs accept such a token.
	want := "youtube:player_client=tv_simply;visitor_data=VD456;po_token=tv_simply.gvs+TOK123"
	if args[1] != want {
		t.Fatalf("got %q, want %q", args[1], want)
	}
	// An account token is bound to the sync id, so the visitor data of the
	// session it belongs to has to win over the binding.
	account := poTokenArgs("", "VISITOR9", mintedPOToken{Token: "TOK123", Binding: "SYNC7"})[1]
	if !strings.Contains(account, "visitor_data=VISITOR9") {
		t.Fatalf("session visitor data not used: %q", account)
	}
	// A chosen client must appear both as the client and in the token label.
	chosen := poTokenArgs("web_safari", "", token)[1]
	if !strings.Contains(chosen, "player_client=web_safari") || !strings.Contains(chosen, "po_token=web_safari.gvs+TOK123") {
		t.Fatalf("client not carried through: %q", chosen)
	}
}

func TestMintedTokenExpiry(t *testing.T) {
	if (mintedPOToken{}).usable() {
		t.Fatal("an empty token is never usable")
	}
	fresh := mintedPOToken{Token: "t", Binding: "b", Expires: time.Now().Add(2 * time.Hour)}
	if !fresh.usable() {
		t.Fatal("a token with hours left must be reused")
	}
	// Inside the renew margin it must be treated as spent.
	nearly := mintedPOToken{Token: "t", Binding: "b", Expires: time.Now().Add(poTokenRenewMargin / 2)}
	if nearly.usable() {
		t.Fatal("a token about to expire must be re-minted")
	}
	if (mintedPOToken{Token: "t", Binding: "b", Expires: time.Now().Add(-time.Minute)}).usable() {
		t.Fatal("an expired token must not be reused")
	}
}

func TestFetchPOTokenReadsTheProviderReply(t *testing.T) {
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/get_pot" || request.Method != http.MethodPost {
			http.Error(writer, "wrong route", http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(request.Body)
		body = string(raw)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"poToken":"TOK","contentBinding":"VD","expiresAt":"2099-01-01T00:00:00.000Z"}`))
	}))
	defer server.Close()

	token, err := fetchPOToken(server.URL+"/", youTubeSession{})
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "TOK" || token.Binding != "VD" || !token.usable() {
		t.Fatalf("unexpected token: %+v", token)
	}
	if body != "{}" {
		t.Fatalf("with no session the provider must pick its own visitor: %s", body)
	}
}

// A token that is not tied to the session the download will use is what YouTube
// answers "Sign in to confirm you're not a bot" to, so the binding must reach
// the provider — as an account id when the cookies carry a login. It goes in
// content_binding alone: the provider rejects the older field names with 400.
func TestFetchPOTokenSendsTheSessionBinding(t *testing.T) {
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		body = string(raw)
		_, _ = writer.Write([]byte(`{"poToken":"TOK","contentBinding":"SYNC7","expiresAt":"2099-01-01T00:00:00.000Z"}`))
	}))
	defer server.Close()

	if _, err := fetchPOToken(server.URL, youTubeSession{VisitorData: "VISITOR9", DataSyncID: "SYNC7"}); err != nil {
		t.Fatal(err)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["content_binding"] != "SYNC7" {
		t.Fatalf("account binding not sent: %s", body)
	}
	// These two are answered with "deprecated, use content_binding instead".
	if _, found := sent["data_sync_id"]; found {
		t.Fatalf("the provider rejects data_sync_id: %s", body)
	}
	if _, found := sent["visitor_data"]; found {
		t.Fatalf("the provider rejects visitor_data: %s", body)
	}

	if _, err := fetchPOToken(server.URL, youTubeSession{VisitorData: "VISITOR9"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["content_binding"] != "VISITOR9" || len(sent) != 1 {
		t.Fatalf("visitor binding not sent alone: %s", body)
	}
}

func TestFetchPOTokenRejectsAnEmptyReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"poToken":"","contentBinding":""}`))
	}))
	defer server.Close()
	if _, err := fetchPOToken(server.URL, youTubeSession{}); err == nil {
		t.Fatal("a reply without a token must be an error")
	}
}

// The default YouTube run is the one that was measured to work: the cookies, and
// web_embedded. No PO token is attached — every client that needs one answered
// 403 on the media URL no matter how the token was bound.
func TestYouTubeRequestArgsUseTheEmbeddedClient(t *testing.T) {
	app := NewApp()
	// A primed session keeps the test off the network; opening one is what
	// refreshes the cookies in production.
	youTubeSessionCache.mu.Lock()
	youTubeSessionCache.value = youTubeSession{VisitorData: "VISITOR9", OpenedAt: time.Now()}
	youTubeSessionCache.mu.Unlock()
	defer app.RefreshYouTubeSession()

	joined := strings.Join(app.ytDlpRequestArgs("https://www.youtube.com/watch?v=jNQXAC9IVRw"), " ")
	if !strings.Contains(joined, "youtube:player_client=web_embedded") {
		t.Fatalf("the embedded client must be asked for: %q", joined)
	}
	if strings.Contains(joined, "po_token") || strings.Contains(joined, "visitor_data") {
		t.Fatalf("no token is needed on this path: %q", joined)
	}
}

// Another site must not be handed YouTube's client.
func TestOtherSitesKeepTheirOwnArguments(t *testing.T) {
	app := NewApp()
	joined := strings.Join(app.ytDlpRequestArgs("https://www.udemy.com/course/abc/"), " ")
	if strings.Contains(joined, "player_client") {
		t.Fatalf("a YouTube client leaked to another site: %q", joined)
	}
}
