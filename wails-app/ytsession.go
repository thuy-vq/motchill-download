package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A YouTube request is judged by the session behind it, not by the link. The
// player page hands every browser a visitor id — and, when someone is signed in,
// an account sync id — and the PO token has to be minted for that exact session.
// A token bound to one session sent with the cookies of another is what makes
// YouTube answer "Sign in to confirm you're not a bot" no matter how fresh the
// token is, which is why a session is opened here before anything is minted.

const (
	youTubeHomeURL = "https://www.youtube.com/"
	// youTubeSessionTTL keeps one session for a whole queue. Pressing Phân tích
	// opens a new one regardless, which is the point of the refresh.
	youTubeSessionTTL = 30 * time.Minute
	// youTubeSessionLimit caps the home page read; the ids sit in the first
	// hundred kilobytes, well inside this.
	youTubeSessionLimit = 4 << 20
)

var (
	visitorDataPattern = regexp.MustCompile(`"(?:visitorData|VISITOR_DATA)"\s*:\s*"([^"]+)"`)
	dataSyncPattern    = regexp.MustCompile(`"(?:datasyncId|DATASYNC_ID)"\s*:\s*"([^"]*)"`)
)

// youTubeSession is one browsing identity: what the cookies, the visitor id and
// the PO token all have to agree on.
type youTubeSession struct {
	VisitorData string
	// DataSyncID is set only when the cookies carry a signed-in account. It is
	// then what the token is bound to, the way yt-dlp's own plugin does it.
	DataSyncID string
	OpenedAt   time.Time
}

// binding is the value the PO token is minted against.
func (s youTubeSession) binding() string {
	if s.DataSyncID != "" {
		return s.DataSyncID
	}
	return s.VisitorData
}

func (s youTubeSession) signedIn() bool {
	return s.DataSyncID != ""
}

func (s youTubeSession) usable() bool {
	return s.VisitorData != "" && time.Since(s.OpenedAt) < youTubeSessionTTL
}

var youTubeSessionCache struct {
	mu    sync.Mutex
	value youTubeSession
}

// RefreshYouTubeSession drops the stored session and PO token so the next
// YouTube request opens a brand new one. The interface calls this each time
// Phân tích is pressed: reusing one identity across every analysis of a long
// evening is exactly what gets an address flagged as a bot.
func (a *App) RefreshYouTubeSession() {
	youTubeSessionCache.mu.Lock()
	youTubeSessionCache.value = youTubeSession{}
	youTubeSessionCache.mu.Unlock()
	poTokenCache.mu.Lock()
	poTokenCache.value = mintedPOToken{}
	poTokenCache.mu.Unlock()
}

// youTubeSessionFor returns the session to use for this request, opening a new
// one when there is none. The cookies of the chosen source are sent along, so a
// signed-in store yields an account session instead of an anonymous one.
func (a *App) youTubeSessionFor(cookieSource string) (youTubeSession, error) {
	youTubeSessionCache.mu.Lock()
	cached := youTubeSessionCache.value
	youTubeSessionCache.mu.Unlock()
	if cached.usable() {
		return cached, nil
	}

	stored := youTubeCookieSource(cookieSource)
	session, fresh, err := openYouTubeSession(youTubeHomeURL, stored)
	if err != nil {
		return youTubeSession{}, err
	}
	// YouTube rotates its cookies on every visit. Writing the new ones back is
	// what stops a saved cookie file from dying after one use — the reason a new
	// file used to have to be exported before every single analysis.
	a.storeSessionCookies(cookieSource, fresh)

	youTubeSessionCache.mu.Lock()
	youTubeSessionCache.value = session
	youTubeSessionCache.mu.Unlock()
	if session.signedIn() {
		a.emit("download:log", "Phiên YouTube mới: đã đăng nhập, PO token sẽ gắn với tài khoản.")
	} else {
		a.emit("download:log", "Phiên YouTube mới: ẩn danh ("+shortVisitor(session.VisitorData)+").")
	}
	return session, nil
}

// youTubeCookieSource returns the cookies to open the session with. Only the
// app's own store can be read here; a browser source is left to yt-dlp, which
// reads it live and is therefore never stale.
func youTubeCookieSource(cookieSource string) []netscapeCookie {
	if !strings.HasPrefix(strings.TrimSpace(cookieSource), "file:") {
		return nil
	}
	return loadCookieStore()
}

// storeSessionCookies keeps the rotated cookies. A file store is updated in
// place; with no store at all the guest cookies YouTube just handed out are kept
// on their own, so the download runs as the same visitor that was minted for.
func (a *App) storeSessionCookies(cookieSource string, fresh []netscapeCookie) {
	if len(fresh) == 0 {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(cookieSource), "file:") {
		if err := writeCookieStore(mergeCookies(loadCookieStore(), fresh)); err != nil {
			a.emit("download:log", "Không lưu được cookie YouTube mới: "+err.Error())
		}
		return
	}
	if strings.TrimSpace(cookieSource) != "" && !strings.EqualFold(cookieSource, "none") {
		// A browser source stays untouched; yt-dlp reads it directly.
		return
	}
	if err := writeGuestCookies(fresh); err != nil {
		a.emit("download:log", "Không lưu được cookie khách YouTube: "+err.Error())
	}
}

// openYouTubeSession loads the YouTube home page the way a browser would and
// reads the identity out of the player configuration embedded in it.
func openYouTubeSession(target string, cookies []netscapeCookie) (youTubeSession, []netscapeCookie, error) {
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return youTubeSession{}, nil, err
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	// The cookies are chosen by what they are for, not by where the request goes,
	// so a test server on localhost still receives the YouTube ones.
	if header := cookieHeader(cookies, "www.youtube.com"); header != "" {
		request.Header.Set("Cookie", header)
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return youTubeSession{}, nil, fmt.Errorf("không mở được phiên YouTube: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return youTubeSession{}, nil, fmt.Errorf("YouTube trả về HTTP %d khi mở phiên", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, youTubeSessionLimit))
	if err != nil {
		return youTubeSession{}, nil, err
	}
	session := parseYouTubeSession(string(body))
	if session.VisitorData == "" {
		return youTubeSession{}, nil, fmt.Errorf("không đọc được visitor data của phiên YouTube")
	}
	session.OpenedAt = time.Now()
	return session, receivedCookies(response), nil
}

// parseYouTubeSession pulls the ids out of the page. Both are JSON strings, so
// the escapes they carry are decoded before use.
func parseYouTubeSession(page string) youTubeSession {
	session := youTubeSession{}
	if match := visitorDataPattern.FindStringSubmatch(page); len(match) > 1 {
		session.VisitorData = unquoteJSONString(match[1])
	}
	if match := dataSyncPattern.FindStringSubmatch(page); len(match) > 1 {
		// Signed out the value is empty or just the separator; signed in the
		// account id comes first, exactly as yt-dlp reads it.
		if id := strings.TrimSpace(strings.Split(unquoteJSONString(match[1]), "||")[0]); id != "" {
			session.DataSyncID = id
		}
	}
	return session
}

func unquoteJSONString(value string) string {
	if unquoted, err := strconv.Unquote(`"` + value + `"`); err == nil {
		return unquoted
	}
	return value
}

func shortVisitor(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12] + "…"
}

// cookieHeader builds the Cookie header for one host out of the store, leaving
// out other sites and anything that has expired.
func cookieHeader(cookies []netscapeCookie, host string) string {
	now := time.Now().Unix()
	parts := make([]string, 0, len(cookies))
	seen := map[string]bool{}
	for _, cookie := range cookies {
		if cookie.Name == "" || !cookieMatchesHost(cookie, host) {
			continue
		}
		if cookie.Expires > 0 && cookie.Expires < now {
			continue
		}
		if seen[cookie.Name] {
			continue
		}
		seen[cookie.Name] = true
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func cookieMatchesHost(cookie netscapeCookie, host string) bool {
	domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(cookie.Domain), "."))
	host = strings.ToLower(host)
	if domain == "" {
		return false
	}
	if domain == host {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// receivedCookies turns the Set-Cookie headers into store entries.
func receivedCookies(response *http.Response) []netscapeCookie {
	result := make([]netscapeCookie, 0, 8)
	for _, cookie := range response.Cookies() {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		domain := strings.TrimSpace(cookie.Domain)
		if domain == "" {
			domain = ".youtube.com"
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		expires := int64(0)
		switch {
		case !cookie.Expires.IsZero():
			expires = cookie.Expires.Unix()
		case cookie.MaxAge > 0:
			expires = time.Now().Add(time.Duration(cookie.MaxAge) * time.Second).Unix()
		}
		result = append(result, netscapeCookie{
			Domain:     domain,
			IncludeSub: strings.HasPrefix(domain, "."),
			Path:       path,
			Secure:     cookie.Secure,
			Expires:    expires,
			Name:       cookie.Name,
			Value:      cookie.Value,
		})
	}
	return result
}
