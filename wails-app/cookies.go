package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// yt-dlp only reads Netscape cookie files, while browser extensions usually
// export JSON. Both are accepted here and stored as one Netscape file, so
// cookies for several sites — Udemy and YouTube, say — can live side by side.
const netscapeHeader = "# Netscape HTTP Cookie File\n" +
	"# Được tạo bởi Video HTML Downloader. Đây là dữ liệu đăng nhập, đừng chia sẻ.\n"

type netscapeCookie struct {
	Domain     string
	IncludeSub bool
	Path       string
	Secure     bool
	Expires    int64
	Name       string
	Value      string
}

// exportedCookie covers the field names used by the common cookie extensions.
type exportedCookie struct {
	Domain         string   `json:"domain"`
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Path           string   `json:"path"`
	Secure         bool     `json:"secure"`
	HostOnly       bool     `json:"hostOnly"`
	Session        bool     `json:"session"`
	ExpirationDate *float64 `json:"expirationDate"`
	Expires        *float64 `json:"expires"`
	ExpiresRaw     *float64 `json:"expiry"`
}

type cookieExportFile struct {
	Cookies []exportedCookie `json:"cookies"`
}

// CookieStatus reports what is stored, without ever exposing a cookie value.
type CookieStatus struct {
	Path    string   `json:"path"`
	Count   int      `json:"count"`
	Domains []string `json:"domains"`
}

func (c exportedCookie) expiry() int64 {
	for _, candidate := range []*float64{c.ExpirationDate, c.Expires, c.ExpiresRaw} {
		if candidate != nil && *candidate > 0 {
			return int64(*candidate)
		}
	}
	return 0
}

// parseCookieExport reads a JSON export in either shape, or a Netscape file.
func parseCookieExport(data []byte) ([]netscapeCookie, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("file cookie trống")
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return parseJSONCookies(trimmed)
	}
	return parseNetscapeCookies(trimmed)
}

func parseJSONCookies(data []byte) ([]netscapeCookie, error) {
	var exported []exportedCookie
	if data[0] == '{' {
		var wrapper cookieExportFile
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("không đọc được JSON cookie: %w", err)
		}
		exported = wrapper.Cookies
	} else if err := json.Unmarshal(data, &exported); err != nil {
		return nil, fmt.Errorf("không đọc được JSON cookie: %w", err)
	}
	result := make([]netscapeCookie, 0, len(exported))
	for _, cookie := range exported {
		domain := strings.TrimSpace(cookie.Domain)
		name := strings.TrimSpace(cookie.Name)
		if domain == "" || name == "" {
			continue
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		expires := cookie.expiry()
		if cookie.Session {
			expires = 0
		}
		result = append(result, netscapeCookie{
			Domain: domain,
			// A leading dot, or an explicit hostOnly=false, means subdomains too.
			IncludeSub: strings.HasPrefix(domain, ".") || !cookie.HostOnly,
			Path:       path,
			Secure:     cookie.Secure,
			Expires:    expires,
			Name:       name,
			Value:      cookie.Value,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("JSON không chứa cookie nào dùng được")
	}
	return result, nil
}

func parseNetscapeCookies(data []byte) ([]netscapeCookie, error) {
	result := make([]netscapeCookie, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// "#HttpOnly_" is the only comment that carries a cookie; keeping it is
		// essential because session tokens are usually HttpOnly.
		if strings.HasPrefix(line, "#") {
			if !strings.HasPrefix(line, "#HttpOnly_") {
				continue
			}
			line = strings.TrimPrefix(line, "#HttpOnly_")
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		expires, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		result = append(result, netscapeCookie{
			Domain:     fields[0],
			IncludeSub: strings.EqualFold(fields[1], "TRUE"),
			Path:       fields[2],
			Secure:     strings.EqualFold(fields[3], "TRUE"),
			Expires:    expires,
			Name:       fields[5],
			Value:      fields[6],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("file không chứa cookie theo định dạng Netscape")
	}
	return result, nil
}

// mergeCookies keeps one entry per domain, path and name, with the newer file
// winning, so adding YouTube cookies does not drop the Udemy ones.
func mergeCookies(existing, incoming []netscapeCookie) []netscapeCookie {
	index := map[string]int{}
	result := make([]netscapeCookie, 0, len(existing)+len(incoming))
	for _, cookie := range append(append([]netscapeCookie{}, existing...), incoming...) {
		key := strings.ToLower(cookie.Domain) + "\x00" + cookie.Path + "\x00" + cookie.Name
		if position, found := index[key]; found {
			result[position] = cookie
			continue
		}
		index[key] = len(result)
		result = append(result, cookie)
	}
	return result
}

func formatNetscapeCookies(cookies []netscapeCookie) string {
	var builder strings.Builder
	builder.WriteString(netscapeHeader)
	for _, cookie := range cookies {
		// yt-dlp reads plain domains only, so no HttpOnly prefix is written.
		builder.WriteString(strings.Join([]string{
			cookie.Domain,
			strings.ToUpper(strconv.FormatBool(cookie.IncludeSub)),
			cookie.Path,
			strings.ToUpper(strconv.FormatBool(cookie.Secure)),
			strconv.FormatInt(cookie.Expires, 10),
			cookie.Name,
			cookie.Value,
		}, "\t"))
		builder.WriteString("\n")
	}
	return builder.String()
}

func cookieDomains(cookies []netscapeCookie) []string {
	seen := map[string]bool{}
	domains := make([]string, 0)
	for _, cookie := range cookies {
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// cookieStorePath is where the converted cookies live: the app config folder,
// readable by this user only.
func cookieStorePath() string {
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = os.TempDir()
	}
	return filepath.Join(configRoot, "MotchillDownloader", "cookies.txt")
}

func loadCookieStore() []netscapeCookie {
	data, err := os.ReadFile(cookieStorePath())
	if err != nil {
		return nil
	}
	cookies, err := parseCookieExport(data)
	if err != nil {
		return nil
	}
	return cookies
}

func writeCookieStore(cookies []netscapeCookie) error {
	path := cookieStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 0600: this file is as sensitive as a password.
	return os.WriteFile(path, []byte(formatNetscapeCookies(cookies)), 0o600)
}

// guestCookiePath holds the cookies YouTube hands out to a visitor who is not
// signed in. They are kept apart from the user's own store: nothing here comes
// from a login, and the file is rewritten on every new session.
func guestCookiePath() string {
	return filepath.Join(filepath.Dir(cookieStorePath()), "youtube-guest.txt")
}

func writeGuestCookies(cookies []netscapeCookie) error {
	path := guestCookiePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(formatNetscapeCookies(cookies)), 0o600)
}

func guestCookiesExist() bool {
	info, err := os.Stat(guestCookiePath())
	return err == nil && info.Size() > 0
}
