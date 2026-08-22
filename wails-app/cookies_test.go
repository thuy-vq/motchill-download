package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The shape produced by the common browser extensions: an object with a
// "cookies" array, expiry as a float, and hostOnly / session flags.
const sampleJSONExport = `{"url":"https://lg.example.com","cookies":[
 {"domain":"lg.example.com","name":"access_token","value":"tok-1","path":"/","secure":true,
  "httpOnly":true,"hostOnly":true,"session":false,"expirationDate":1789570265.916867},
 {"domain":".example.com","name":"shared","value":"tok-2","path":"/","secure":false,
  "httpOnly":false,"hostOnly":false,"session":false,"expirationDate":1818521368.930691},
 {"domain":"lg.example.com","name":"temp","value":"tok-3","path":"/x","secure":false,
  "httpOnly":false,"hostOnly":true,"session":true},
 {"domain":"","name":"broken","value":"x"}
]}`

func TestParseJSONCookieExport(t *testing.T) {
	cookies, err := parseCookieExport([]byte(sampleJSONExport))
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 3 {
		t.Fatalf("want 3 usable cookies, got %d", len(cookies))
	}
	first := cookies[0]
	if first.Domain != "lg.example.com" || first.Name != "access_token" || !first.Secure {
		t.Fatalf("unexpected first cookie: %+v", first)
	}
	// hostOnly means the cookie is not valid for subdomains.
	if first.IncludeSub {
		t.Fatal("a hostOnly cookie must not include subdomains")
	}
	if first.Expires != 1789570265 {
		t.Fatalf("expiry = %d, want 1789570265", first.Expires)
	}
	if !cookies[1].IncludeSub {
		t.Fatal("a leading-dot domain must include subdomains")
	}
	// A session cookie has no expiry, which Netscape writes as 0.
	if cookies[2].Expires != 0 {
		t.Fatalf("session cookie expiry = %d, want 0", cookies[2].Expires)
	}
}

func TestParseJSONCookieArrayForm(t *testing.T) {
	cookies, err := parseCookieExport([]byte(`[{"domain":"a.example","name":"n","value":"v","path":"/"}]`))
	if err != nil || len(cookies) != 1 {
		t.Fatalf("array form failed: %v / %+v", err, cookies)
	}
	if cookies[0].Path != "/" {
		t.Fatalf("unexpected path: %q", cookies[0].Path)
	}
}

// The written file must be exactly what yt-dlp expects, including the magic
// header, tabs, and plain domains for HttpOnly cookies.
func TestNetscapeOutputRoundTrips(t *testing.T) {
	cookies, err := parseCookieExport([]byte(sampleJSONExport))
	if err != nil {
		t.Fatal(err)
	}
	text := formatNetscapeCookies(cookies)
	if !strings.HasPrefix(text, "# Netscape HTTP Cookie File") {
		t.Fatalf("missing magic header: %q", text[:40])
	}
	if strings.Contains(text, "#HttpOnly_") {
		t.Fatal("an HttpOnly prefix would make yt-dlp skip the session token")
	}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if fields := strings.Split(line, "\t"); len(fields) != 7 {
			t.Fatalf("line must have 7 tab-separated fields: %q", line)
		}
	}
	if !strings.Contains(text, "lg.example.com\tFALSE\t/\tTRUE\t1789570265\taccess_token\ttok-1") {
		t.Fatalf("unexpected serialisation:\n%s", text)
	}
	// Reading it back gives the same cookies.
	again, err := parseCookieExport([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(cookies) || again[0].Name != cookies[0].Name || again[0].Value != cookies[0].Value {
		t.Fatalf("round trip changed the cookies: %+v", again)
	}
}

func TestNetscapeParserKeepsHttpOnlyLines(t *testing.T) {
	text := "# Netscape HTTP Cookie File\n" +
		"#HttpOnly_lg.example.com\tFALSE\t/\tTRUE\t1789570265\taccess_token\ttok-1\n" +
		"# just a comment\n" +
		".example.com\tTRUE\t/\tFALSE\t0\tshared\ttok-2\n"
	cookies, err := parseCookieExport([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 2 {
		t.Fatalf("want 2 cookies, got %d: %+v", len(cookies), cookies)
	}
	if cookies[0].Name != "access_token" {
		t.Fatal("the HttpOnly session token must survive parsing")
	}
}

func TestMergeCookiesKeepsBothSitesAndPrefersTheNewer(t *testing.T) {
	existing, err := parseCookieExport([]byte(sampleJSONExport))
	if err != nil {
		t.Fatal(err)
	}
	incoming := []netscapeCookie{
		{Domain: ".youtube.com", IncludeSub: true, Path: "/", Secure: true, Expires: 111, Name: "SID", Value: "yt"},
		{Domain: "lg.example.com", IncludeSub: false, Path: "/", Secure: true, Expires: 222, Name: "access_token", Value: "tok-new"},
	}
	merged := mergeCookies(existing, incoming)
	if len(merged) != 4 {
		t.Fatalf("want 4 cookies after the merge, got %d", len(merged))
	}
	domains := cookieDomains(merged)
	if strings.Join(domains, ",") != "example.com,lg.example.com,youtube.com" {
		t.Fatalf("unexpected domains: %v", domains)
	}
	for _, cookie := range merged {
		if cookie.Name == "access_token" && cookie.Value != "tok-new" {
			t.Fatal("the newer file must replace an existing cookie")
		}
	}
}

func TestCookieStoreIsWrittenPrivately(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("APPDATA", directory)
	t.Setenv("XDG_CONFIG_HOME", directory)
	cookies, err := parseCookieExport([]byte(sampleJSONExport))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCookieStore(cookies); err != nil {
		t.Fatal(err)
	}
	path := cookieStorePath()
	if !strings.HasPrefix(path, directory) {
		t.Skipf("config dir is not redirectable here: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows ignores Unix mode bits — there the protection is the ACL of the
	// per-user config folder the file sits in.
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(filepath.Dir(path), directory) {
			t.Fatalf("cookie store must stay in the user config folder, got %s", path)
		}
	} else if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("cookie store must not be group or world readable, got %v", mode)
	}
	if loaded := loadCookieStore(); len(loaded) != len(cookies) {
		t.Fatalf("store reload gave %d cookies, want %d", len(loaded), len(cookies))
	}
}

// TestLiveCookieFile converts a real export and, when given a URL, checks that a
// site accepts it. It never prints a cookie value — only domains and counts:
//
//	MOTCHILL_LIVE_COOKIE_FILE=<path> MOTCHILL_LIVE_COOKIE_URL=<url> go test -run TestLiveCookieFile -v
func TestLiveCookieFile(t *testing.T) {
	source := os.Getenv("MOTCHILL_LIVE_COOKIE_FILE")
	if source == "" {
		t.Skip("MOTCHILL_LIVE_COOKIE_FILE is not set")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	cookies, err := parseCookieExport(data)
	if err != nil {
		t.Fatalf("không đọc được file cookie: %v", err)
	}
	t.Logf("đọc được %d cookie cho: %s", len(cookies), strings.Join(cookieDomains(cookies), ", "))

	converted := os.Getenv("MOTCHILL_LIVE_COOKIE_OUT")
	if converted == "" {
		converted = filepath.Join(t.TempDir(), "cookies.txt")
	}
	if err := os.WriteFile(converted, []byte(formatNetscapeCookies(cookies)), 0o600); err != nil {
		t.Fatal(err)
	}

	target := os.Getenv("MOTCHILL_LIVE_COOKIE_URL")
	if target == "" {
		return
	}
	app := NewApp()
	ytDlp := app.findYtDlp()
	if ytDlp == "" {
		t.Skip("yt-dlp chưa được cài")
	}
	command := exec.Command(ytDlp, "--simulate", "--no-warnings", "--no-colors",
		"--cookies", converted, "--print", "%(playlist_count)s|%(title)s", target)
	prepareBackgroundCommand(command)
	output, runErr := command.CombinedOutput()
	// Trim to keep any long provider message readable in the test log.
	message := strings.TrimSpace(string(output))
	if len(message) > 1500 {
		message = message[len(message)-1500:]
	}
	t.Logf("yt-dlp: %v\n%s", runErr, message)
	if runErr != nil {
		t.Fatalf("site chưa nhận cookie này")
	}
}

func TestParseCookieExportRejectsRubbish(t *testing.T) {
	for _, data := range []string{"", "   ", "hello world", `{"cookies":[]}`, `{`} {
		if _, err := parseCookieExport([]byte(data)); err == nil {
			t.Fatalf("must reject %q", data)
		}
	}
}
