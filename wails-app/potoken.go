package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// YouTube binds its media URLs to a proof-of-origin (PO) token produced by the
// player in a browser. yt-dlp gets one through a provider plugin, so the app
// only has to point yt-dlp at that plugin and pass the token arguments through.
// Everything here is optional: with no tuning set, nothing is added.

// bgutilProviderKey is the extractor key of the widely used HTTP provider.
const bgutilProviderKey = "youtubepot-bgutilhttp"

// poTokenUsed matches only the verbose lines that report a token in hand.
var poTokenUsed = regexp.MustCompile(`(?i)([pot]|po.?token).*(fetch|using|got|retriev|provided)`)

// YtDlpTuning holds the advanced YouTube switches shown in the interface.
type YtDlpTuning struct {
	// PluginDir is passed as --plugin-dirs so yt-dlp finds the provider plugin.
	PluginDir string `json:"pluginDir"`
	// ProviderURL is the base URL of a running PO token provider server.
	ProviderURL string `json:"providerUrl"`
	// Token is a PO token pasted by hand, for a one-off download.
	Token string `json:"token"`
	// PlayerClient overrides which YouTube client yt-dlp asks, e.g. "web_safari".
	PlayerClient string `json:"playerClient"`
}

func (a *App) GetYtDlpTuning() YtDlpTuning {
	settings := a.settings.snapshot()
	return YtDlpTuning{
		PluginDir:    settings.PluginDir,
		ProviderURL:  settings.POTokenProvider,
		Token:        settings.POToken,
		PlayerClient: settings.PlayerClient,
	}
}

func (a *App) RememberYtDlpTuning(tuning YtDlpTuning) error {
	return a.settings.setYtDlpTuning(
		strings.TrimSpace(tuning.PluginDir),
		strings.TrimSpace(tuning.ProviderURL),
		strings.TrimSpace(tuning.Token),
		strings.TrimSpace(tuning.PlayerClient),
	)
}

// SelectPluginDir points yt-dlp at the folder holding the provider plugin.
func (a *App) SelectPluginDir() (YtDlpTuning, error) {
	selected, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Chọn thư mục chứa plugin yt-dlp",
	})
	if err != nil || selected == "" {
		return a.GetYtDlpTuning(), err
	}
	tuning := a.GetYtDlpTuning()
	tuning.PluginDir = selected
	if err := a.RememberYtDlpTuning(tuning); err != nil {
		return YtDlpTuning{}, err
	}
	return a.GetYtDlpTuning(), nil
}

// ytDlpTuningArgs turns the settings into yt-dlp arguments. Extractor arguments
// are passed once per key, which is how yt-dlp expects them.
func ytDlpTuningArgs(tuning YtDlpTuning) []string {
	arguments := make([]string, 0, 8)
	if tuning.PluginDir != "" {
		arguments = append(arguments, "--plugin-dirs", tuning.PluginDir)
	}
	if tuning.ProviderURL != "" {
		arguments = append(arguments, "--extractor-args",
			fmt.Sprintf("%s:base_url=%s", bgutilProviderKey, strings.TrimRight(tuning.ProviderURL, "/")))
	}
	youtube := make([]string, 0, 2)
	if tuning.PlayerClient != "" {
		youtube = append(youtube, "player_client="+tuning.PlayerClient)
	}
	if tuning.Token != "" {
		youtube = append(youtube, "po_token="+tuning.Token)
	}
	if len(youtube) > 0 {
		arguments = append(arguments, "--extractor-args", "youtube:"+strings.Join(youtube, ";"))
	}
	return arguments
}

// PluginCheck reports what yt-dlp loaded, so a misplaced plugin is obvious
// before a whole queue fails.
type PluginCheck struct {
	PluginsFound bool     `json:"pluginsFound"`
	TokenUsed    bool     `json:"tokenUsed"`
	Resolved     bool     `json:"resolved"`
	Plugins      []string `json:"plugins"`
	Message      string   `json:"message"`
}

// parsePluginCheck reads the verbose output of a simulated download.
func parsePluginCheck(output string) PluginCheck {
	check := PluginCheck{Plugins: []string{}}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "extractor plugins:"), strings.Contains(lower, "postprocessor plugins:"):
			name := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if name != "" && !seen[name] {
				seen[name] = true
				check.Plugins = append(check.Plugins, name)
				check.PluginsFound = true
			}
		case poTokenUsed.MatchString(line):
			// Only a token actually fetched or supplied counts. Lines such as
			// "no PO token provider available" say the opposite.
			check.TokenUsed = true
		}
	}
	return check
}

// CheckYtDlpPlugins runs one simulated resolve with the current tuning and
// reports whether the plugin and a PO token were in play.
func (a *App) CheckYtDlpPlugins(target string) (PluginCheck, error) {
	path := a.findYtDlp()
	if path == "" {
		return PluginCheck{}, fmt.Errorf("chưa có yt-dlp")
	}
	if strings.TrimSpace(target) == "" {
		target = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	}
	tuning := a.GetYtDlpTuning()
	if tuning.PluginDir != "" {
		if info, err := os.Stat(tuning.PluginDir); err != nil || !info.IsDir() {
			return PluginCheck{}, fmt.Errorf("thư mục plugin không tồn tại: %s", tuning.PluginDir)
		}
	}
	arguments := []string{"--verbose", "--simulate", "--no-colors", "--no-playlist"}
	arguments = append(arguments, ytDlpTuningArgs(tuning)...)
	arguments = append(arguments, ytDlpAuthArgs(a.settings.snapshot().CookieSource)...)
	arguments = append(arguments, "--print", "%(height)sp %(format_id)s", target)

	command := exec.Command(path, arguments...)
	prepareBackgroundCommand(command)
	output, runErr := command.CombinedOutput()
	check := parsePluginCheck(string(output))
	check.Resolved = runErr == nil
	if runErr == nil {
		check.Message = "Lấy được thông tin video."
	} else {
		check.Message = lastLine(string(output))
	}
	return check, nil
}

// The yt-dlp .exe build does not load external plugins, so the app talks to the
// provider itself: it mints a PO token over HTTP and hands it to yt-dlp through
// documented extractor arguments. tv_simply is the client whose media URLs are
// accepted with such a token; web is served SABR-only and has no direct URLs.
const (
	poTokenDefaultClient = "tv_simply"
	// youTubeDefaultClient is the client the app asks for. web_embedded is the
	// only one whose adaptive media URLs still download: it offers the full
	// ladder up to 8K, works with or without cookies, and needs no PO token.
	youTubeDefaultClient = "web_embedded"
	// poTokenRenewMargin re-mints before the provider's own expiry.
	poTokenRenewMargin = 10 * time.Minute
)

type mintedPOToken struct {
	Token   string
	Binding string
	Expires time.Time
}

var poTokenCache struct {
	mu    sync.Mutex
	value mintedPOToken
}

func (t mintedPOToken) usable() bool {
	return t.Token != "" && t.Binding != "" && time.Now().Add(poTokenRenewMargin).Before(t.Expires)
}

// poTokenArgs formats what yt-dlp expects: the token is labelled with the client
// it was minted for, and the visitor data of the session it belongs to is sent
// along. For a signed-in session the token is bound to the account instead, so
// the visitor data has to be passed separately rather than read off the token.
func poTokenArgs(client, visitorData string, token mintedPOToken) []string {
	if client == "" {
		client = poTokenDefaultClient
	}
	if visitorData == "" {
		visitorData = token.Binding
	}
	return []string{"--extractor-args", fmt.Sprintf("youtube:player_client=%s;visitor_data=%s;po_token=%s.gvs+%s",
		client, visitorData, client, token.Token)}
}

// fetchPOToken asks the local provider for a token. A binding ties the token to
// the session the download will actually use — the account id when the cookies
// carry a login, the visitor id otherwise. With no binding the provider picks a
// fresh anonymous visitor of its own, which is the older behaviour.
func fetchPOToken(base string, session youTubeSession) (mintedPOToken, error) {
	body := "{}"
	if binding := session.binding(); binding != "" {
		// content_binding is the only field the provider accepts: it answers 400
		// to the older visitor_data and data_sync_id, which it calls deprecated.
		// The account id and the visitor id both go in here.
		payload, err := json.Marshal(map[string]string{"content_binding": binding})
		if err != nil {
			return mintedPOToken{}, err
		}
		body = string(payload)
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/get_pot",
		strings.NewReader(body))
	if err != nil {
		return mintedPOToken{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 120 * time.Second}).Do(request)
	if err != nil {
		return mintedPOToken{}, fmt.Errorf("không gọi được provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return mintedPOToken{}, fmt.Errorf("provider trả về HTTP %d", response.StatusCode)
	}
	var payload struct {
		PoToken        string `json:"poToken"`
		ContentBinding string `json:"contentBinding"`
		ExpiresAt      string `json:"expiresAt"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return mintedPOToken{}, err
	}
	if payload.PoToken == "" || payload.ContentBinding == "" {
		return mintedPOToken{}, fmt.Errorf("provider không trả về token")
	}
	expires, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil {
		expires = time.Now().Add(time.Hour)
	}
	return mintedPOToken{Token: payload.PoToken, Binding: payload.ContentBinding, Expires: expires}, nil
}

// ytDlpRequestArgs assembles the arguments for one yt-dlp run against a target:
// the browser cookies for sites that need a login, and the YouTube tuning or a
// token minted from the provider.
//
// For YouTube the deciding factor turned out to be the player client, not the
// token. Measured on a 4K video: every adaptive format of mweb, tv_simply and
// android_vr answers 403 on the media URL — with a PO token, without one, and
// with the token bound to the account, to the visitor or to the video id alike;
// web and web_safari are served SABR and expose no adaptive format at all. Only
// web_embedded hands over media URLs that download, and it needs no token. The
// PO token machinery is therefore kept for the Player client field: it is what
// to reach for when this stops working too.
func (a *App) ytDlpRequestArgs(target string) []string {
	tuning := a.GetYtDlpTuning()
	cookieSource := a.settings.snapshot().CookieSource
	arguments := jsRuntimeArgs()

	// A hand-entered token is the user asking for one exact setup; nothing is
	// second-guessed there.
	if tuning.Token != "" {
		arguments = append(arguments, ytDlpAuthArgs(cookieSource)...)
		return append(arguments, ytDlpTuningArgs(tuning)...)
	}
	if !isYouTubeURL(target) {
		arguments = append(arguments, ytDlpAuthArgs(cookieSource)...)
		return append(arguments, ytDlpTuningArgs(tuning)...)
	}

	// Opening a session is what refreshes the stored cookies; its ids are only
	// needed when a token has to be minted below.
	session, sessionErr := a.youTubeSessionFor(cookieSource)
	if sessionErr != nil {
		a.emit("download:log", "Phiên YouTube: "+sessionErr.Error())
	}
	arguments = append(arguments, a.youTubeCookieArgs(cookieSource)...)

	if tuning.PlayerClient == "" {
		return append(arguments, "--extractor-args", "youtube:player_client="+youTubeDefaultClient)
	}
	// A client chosen by hand comes with a token bound to this session, which is
	// the most that can be done for the clients that ask for one.
	token, err := a.providerToken(tuning, session)
	if err != nil {
		a.emit("download:log", "PO token: "+err.Error())
		return append(arguments, ytDlpTuningArgs(tuning)...)
	}
	if tuning.PluginDir != "" {
		arguments = append(arguments, "--plugin-dirs", tuning.PluginDir)
	}
	return append(arguments, poTokenArgs(tuning.PlayerClient, session.VisitorData, token)...)
}

// youTubeCookieArgs is ytDlpAuthArgs plus the guest cookies: with no source of
// their own, the ones YouTube handed this session are still better than none,
// because they belong to the visitor the token was minted for.
func (a *App) youTubeCookieArgs(cookieSource string) []string {
	if arguments := ytDlpAuthArgs(cookieSource); len(arguments) > 0 {
		return arguments
	}
	if guestCookiesExist() {
		return []string{"--cookies", guestCookiePath()}
	}
	return nil
}

// providerToken reuses a cached token until it is close to expiring, starting the
// provider when it is installed but not running. A token minted for a different
// session is of no use, so a changed binding forces a new one.
func (a *App) providerToken(tuning YtDlpTuning, session youTubeSession) (mintedPOToken, error) {
	poTokenCache.mu.Lock()
	cached := poTokenCache.value
	poTokenCache.mu.Unlock()
	if cached.usable() && (session.binding() == "" || cached.Binding == session.binding()) {
		return cached, nil
	}

	base := strings.TrimSpace(tuning.ProviderURL)
	status := a.GetProviderStatus()
	if !status.Running {
		if !status.ServerInstalled {
			return mintedPOToken{}, fmt.Errorf("chưa cài provider")
		}
		started, err := a.StartPOTokenProvider(status.Port)
		if err != nil {
			return mintedPOToken{}, err
		}
		base = fmt.Sprintf("http://127.0.0.1:%d", started.Port)
	}
	if base == "" {
		base = fmt.Sprintf("http://127.0.0.1:%d", status.Port)
	}
	token, err := fetchPOToken(base, session)
	if err != nil {
		return mintedPOToken{}, err
	}
	poTokenCache.mu.Lock()
	poTokenCache.value = token
	poTokenCache.mu.Unlock()
	boundTo := "phiên ẩn danh của provider"
	switch {
	case session.signedIn():
		boundTo = "tài khoản trong cookie, cookie được gửi kèm"
	case session.VisitorData != "":
		boundTo = "phiên vừa mở, cookie được gửi kèm"
	}
	a.emit("download:log", fmt.Sprintf("PO token: đã lấy token mới, hạn đến %s (gắn với %s)",
		token.Expires.Local().Format("15:04:05"), boundTo))
	return token, nil
}

// jsRuntimeArgs points yt-dlp at Node, which recent builds need for YouTube and
// only enable for deno by default.
func jsRuntimeArgs() []string {
	if _, err := exec.LookPath("node"); err != nil {
		return nil
	}
	return []string{"--js-runtimes", "node"}
}
