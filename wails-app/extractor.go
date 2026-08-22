package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

var (
	absoluteURLPattern = regexp.MustCompile(`(?i)https?://[^\s"'<>\\\[\]{}]+`)
	attributePattern   = regexp.MustCompile(`(?i)(?:src|file|source|url)\s*[:=]\s*["']([^"']+)["']`)
	unicodePattern     = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
	episodeNumberRE    = regexp.MustCompile(`(?i)(?:tap|tập|episode|ep)[-_.\s]*(\d+)`)
	// Motchill variants name the player segment "tap-1", "tap-1-sv-0",
	// "tap-1-3097673" or "tap-full"; only a whole segment counts so a movie slug
	// that happens to contain "tap-2" is not mistaken for an episode.
	episodeSegmentRE = regexp.MustCompile(`(?i)^(?:tap|tập|episode|ep)[-_.\s]*(\d+|full)\b`)
	// Language and quality tails that sit after the episode segment on some hosts.
	episodeTailSegments = map[string]bool{
		"vietsub": true, "thuyet-minh": true, "thuyet-minh-vietsub": true, "long-tieng": true,
		"sub": true, "tm": true, "lt": true, "full-hd": true, "hd": true,
	}
	titlePattern     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	canonicalPattern = regexp.MustCompile(`(?i)<link[^>]+rel\s*=\s*["']canonical["'][^>]+href\s*=\s*["'](https?://[^"']+)`)
	ogURLPattern     = regexp.MustCompile(`(?i)<meta[^>]+property\s*=\s*["']og:url["'][^>]+content\s*=\s*["'](https?://[^"']+)`)
	hrefPattern      = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
	movieIDPattern   = regexp.MustCompile(`(?i)["'](?:movieId|movie_id)["']\s*:\s*["']?(\d+)`)
	// Trailing site name on the page title, e.g. "… - Motchill",
	// "… | Motphimchill", "… - PhimmoiChill".
	titleSitePattern = regexp.MustCompile(`(?i)\s*[-|–]\s*[\p{L}\d .]*chill[\p{L}\d .]*$`)
	embedLinkPattern = regexp.MustCompile(`(?i)"link_embed"\s*:\s*"(https?:[^"]+)"`)
)

var mediaExtensions = map[string]string{
	".m3u8": "HLS",
	".mpd":  "DASH",
	".mp4":  "Video",
	".m4v":  "Video",
	".webm": "Video",
	".mov":  "Video",
	".mkv":  "Video",
}

func fetchHTML(pageURL string) (string, error) {
	client := &http.Client{Timeout: 35 * time.Second}
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("không thể tải trang: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("máy chủ trả về HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeHTML(input string) string {
	result := html.UnescapeString(input)
	result = strings.ReplaceAll(result, `\/`, "/")
	result = unicodePattern.ReplaceAllStringFunc(result, func(value string) string {
		code, err := strconv.ParseInt(value[2:], 16, 32)
		if err != nil {
			return value
		}
		return string(rune(code))
	})
	return result
}

func extractMedia(source, baseURL string) []MediaStream {
	normalized := normalizeHTML(source)
	candidates := make([]string, 0)
	for _, match := range absoluteURLPattern.FindAllString(normalized, -1) {
		for offset := 0; offset < len(match); {
			relative := strings.Index(strings.ToLower(match[offset:]), "http")
			if relative < 0 {
				break
			}
			start := offset + relative
			candidates = append(candidates, match[start:])
			offset = start + 4
		}
		if decoded, err := url.QueryUnescape(match); err == nil && decoded != match {
			for offset := 0; offset < len(decoded); {
				relative := strings.Index(strings.ToLower(decoded[offset:]), "http")
				if relative < 0 {
					break
				}
				start := offset + relative
				candidates = append(candidates, decoded[start:])
				offset = start + 4
			}
		}
	}
	for _, match := range attributePattern.FindAllStringSubmatch(normalized, -1) {
		if len(match) > 1 {
			candidates = append(candidates, match[1])
		}
	}

	seen := map[string]bool{}
	result := make([]MediaStream, 0)
	base, _ := url.Parse(baseURL)
	for _, candidate := range candidates {
		candidate = strings.TrimRight(strings.TrimSpace(html.UnescapeString(candidate)), ".,;)")
		parsed, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		if !parsed.IsAbs() && base != nil {
			parsed = base.ResolveReference(parsed)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		kind, ok := mediaExtensions[strings.ToLower(path.Ext(parsed.Path))]
		if !ok || seen[parsed.String()] {
			continue
		}
		seen[parsed.String()] = true
		result = append(result, MediaStream{URL: parsed.String(), Kind: kind})
	}
	return result
}

func extractCurrentStreams(source, baseURL string) []MediaStream {
	decoded := strings.ReplaceAll(normalizeHTML(source), `\"`, `"`)
	records, ok := extractEpisodeRecords(decoded, "episodeVariants", 0)
	if !ok {
		start := strings.Index(decoded, `"episode_data_count"`)
		if start < 0 {
			start = 0
		}
		records, ok = extractEpisodeRecords(decoded, "episodes", start)
	}
	if !ok {
		return extractMedia(source, baseURL)
	}
	return streamsFromEpisodeRecords(records, baseURL, source)
}

type episodeRecord struct {
	ID      string `json:"id"`
	MovieID string `json:"movie_id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Server  string `json:"server"`
	Type    string `json:"type"`
	Link    string `json:"link"`
}

func extractEpisodeRecords(decoded, key string, start int) ([]episodeRecord, bool) {
	if start < 0 || start >= len(decoded) {
		start = 0
	}
	keyOffset := strings.Index(decoded[start:], `"`+key+`"`)
	if keyOffset < 0 {
		return nil, false
	}
	keyOffset += start
	arrayOffset := strings.Index(decoded[keyOffset:], "[")
	if arrayOffset < 0 {
		return nil, false
	}
	arrayStart := keyOffset + arrayOffset
	arrayEnd := matchingJSONBracket(decoded, arrayStart)
	if arrayEnd < 0 {
		return nil, false
	}
	var records []episodeRecord
	if err := json.Unmarshal([]byte(decoded[arrayStart:arrayEnd+1]), &records); err != nil {
		return nil, false
	}
	return records, true
}

func streamsFromEpisodeRecords(records []episodeRecord, baseURL, fallbackSource string) []MediaStream {
	result := make([]MediaStream, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		links := extractMedia(record.Link, baseURL)
		for _, stream := range links {
			if seen[stream.URL] {
				continue
			}
			seen[stream.URL] = true
			stream.Server = strings.TrimSpace(record.Server)
			if stream.Server == "" {
				stream.Server = strings.TrimSpace(record.Name)
			}
			result = append(result, stream)
		}
	}
	if len(result) == 0 {
		return extractMedia(fallbackSource, baseURL)
	}
	return result
}

func matchingJSONBracket(value string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(value); i++ {
		char := value[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		if char == '"' {
			inString = true
		} else if char == '[' {
			depth++
		} else if char == ']' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// extractEmbedLinks finds players that host the video themselves instead of
// exposing a playlist, e.g. "link_embed":"https://embed1.streamc.xyz/embed.php…".
func extractEmbedLinks(source string) []string {
	decoded := strings.ReplaceAll(normalizeHTML(source), `\"`, `"`)
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, match := range embedLinkPattern.FindAllStringSubmatch(decoded, -1) {
		value := strings.TrimSpace(match[1])
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// embedHosts names the embed providers of a page for the error shown to the user.
func embedHosts(embeds []string) string {
	seen := map[string]bool{}
	hosts := make([]string, 0, len(embeds))
	for _, embed := range embeds {
		parsed, err := url.Parse(embed)
		if err != nil || parsed.Host == "" || seen[parsed.Host] {
			continue
		}
		seen[parsed.Host] = true
		hosts = append(hosts, parsed.Host)
	}
	return strings.Join(hosts, ", ")
}

// embedOnlyError explains why a page with a player still has nothing to download.
func embedOnlyError(embeds []string) error {
	return fmt.Errorf("tập này chỉ phát qua trình nhúng %s và playlist của nó được mã hóa riêng, "+
		"FFmpeg không đọc được; hãy thử server khác hoặc nguồn khác", embedHosts(embeds))
}

func extractTitle(source string) string {
	match := titlePattern.FindStringSubmatch(normalizeHTML(source))
	if len(match) < 2 {
		return "Video"
	}
	title := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(html.UnescapeString(match[1]), " "))
	title = regexp.MustCompile(`(?i)^xem\s+phim\s+`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`(?i)\s+(tập|episode)\s+\d+.*$`).ReplaceAllString(title, "")
	// Every host in the family ends its title with its own name.
	title = titleSitePattern.ReplaceAllString(title, "")
	title = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(title), "-|–"))
	if title == "" {
		return "Video"
	}
	return title
}

func extractPageURL(source string) string {
	normalized := normalizeHTML(source)
	if match := canonicalPattern.FindStringSubmatch(normalized); len(match) > 1 {
		return html.UnescapeString(match[1])
	}
	if match := ogURLPattern.FindStringSubmatch(normalized); len(match) > 1 {
		return html.UnescapeString(match[1])
	}
	return ""
}

func extractEpisodeLinks(source, pageURL string) []Episode {
	base, err := url.Parse(pageURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil
	}
	currentSegment, _, _ := episodePathSegment(base.Path)
	seriesPath := seriesPathOf(base.Path)
	if strings.Count(seriesPath, "/") < 2 {
		return nil
	}
	// Matching on the movie slug rather than the whole prefix, because hosts list
	// a movie under /phim/<slug> but play it under /xem-phim/<slug>/tap-N/….
	slug := strings.ToLower(path.Base(seriesPath))
	seen := map[string]bool{}
	byNumber := map[int]Episode{}
	for _, match := range hrefPattern.FindAllStringSubmatch(normalizeHTML(source), -1) {
		if len(match) < 2 {
			continue
		}
		reference, err := url.Parse(html.UnescapeString(match[1]))
		if err != nil {
			continue
		}
		absolute := base.ResolveReference(reference)
		if !strings.EqualFold(absolute.Host, base.Host) || !pathHasSegment(absolute.Path, slug) {
			continue
		}
		segment, number, found := episodePathSegment(absolute.Path)
		if !found || seen[absolute.String()] {
			continue
		}
		seen[absolute.String()] = true
		episode := Episode{
			ID:      episodeIdentity(number),
			Name:    episodeDisplayName(number),
			Number:  number,
			PageURL: absolute.String(),
			Current: segment == currentSegment,
		}
		// Hosts link one episode once per server ("…/tap-1-sv-0", "…/tap-1-sv-1").
		// Keep the shortest link and let the download step walk the servers.
		if existing, exists := byNumber[number]; exists {
			if !episode.Current && (existing.Current || len(existing.PageURL) <= len(episode.PageURL)) {
				continue
			}
		}
		byNumber[number] = episode
	}
	result := make([]Episode, 0, len(byNumber))
	for _, episode := range byNumber {
		result = append(result, episode)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Number < result[j].Number })
	// A single-part film is usually linked twice, once as "tap-full" and once as
	// "tap-1". Both play the same video, so the list keeps the "full" entry only
	// instead of offering a duplicate that later fails the sameness check.
	if len(result) == 2 && result[0].Number == 0 && result[1].Number == 1 {
		result = result[:1]
	}
	return result
}

func episodeNumber(value string) int {
	match := episodeNumberRE.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0
	}
	number, _ := strconv.Atoi(match[1])
	return number
}

// episodePathSegment finds the segment of a URL that marks a player page and its
// number, where a single-part film ("tap-full") reports number 0. Motchill
// variants differ only in what follows that segment, so every host shape —
// /phim/slug/tap-1, /phim/slug/tap-1-sv-0, /xem-phim/slug/tap-full/vietsub —
// resolves through this one helper.
func episodePathSegment(value string) (string, int, bool) {
	target := value
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		target = parsed.Path
	}
	for _, segment := range strings.Split(strings.Trim(target, "/"), "/") {
		match := episodeSegmentRE.FindStringSubmatch(segment)
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			number = 0
		}
		return segment, number, true
	}
	return "", 0, false
}

// isEpisodePage separates a player page from a movie landing page. Landing pages
// must not keep the media URLs found in recommendation cards.
func isEpisodePage(value string) bool {
	_, _, found := episodePathSegment(value)
	return found
}

// seriesPathOf strips the episode segment and any language or quality tail, so
// all episodes of one movie share a prefix regardless of host.
func seriesPathOf(value string) string {
	target := value
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		target = parsed.Path
	}
	segments := strings.Split(strings.Trim(target, "/"), "/")
	for index, segment := range segments {
		if episodeSegmentRE.MatchString(segment) {
			segments = segments[:index]
			return "/" + strings.Join(segments, "/")
		}
	}
	for len(segments) > 0 && episodeTailSegments[strings.ToLower(segments[len(segments)-1])] {
		segments = segments[:len(segments)-1]
	}
	return "/" + strings.Join(segments, "/")
}

// episodeSlugNumber reads the episode number from a slug or a display name.
// "tap-full" is a real episode, so it reports (0, true).
func episodeSlugNumber(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if match := episodeSegmentRE.FindStringSubmatch(value); match != nil {
		number, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, true
		}
		return number, true
	}
	if number := episodeNumber(value); number > 0 {
		return number, true
	}
	return 0, false
}

// pathHasSegment reports whether a slug appears as a whole segment, so
// "/xem-phim/gantz/tap-1" belongs to "gantz" but "/phim/gantz-2/tap-1" does not.
func pathHasSegment(value, slug string) bool {
	if slug == "" || slug == "." || slug == "/" {
		return false
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if strings.EqualFold(segment, slug) {
			return true
		}
	}
	return false
}

func episodeIdentity(number int) string {
	if number > 0 {
		return fmt.Sprintf("episode-%d", number)
	}
	return "episode-full"
}

func episodeDisplayName(number int) string {
	if number > 0 {
		return fmt.Sprintf("Tập %02d", number)
	}
	return "Tập Full"
}

func extractMovieID(source string) string {
	decoded := strings.ReplaceAll(normalizeHTML(source), `\"`, `"`)
	// Landing pages expose the target id as a movieId component prop. Check it
	// before snake_case episode records, which may be recommendation cards.
	if match := movieIDPattern.FindStringSubmatch(decoded); len(match) > 1 {
		return match[1]
	}
	if records, ok := extractEpisodeRecords(decoded, "episodeVariants", 0); ok {
		for _, record := range records {
			if record.MovieID != "" {
				return record.MovieID
			}
		}
	}
	return ""
}

// otherServerIndexes lists every server except the one already handled first.
func otherServerIndexes(count, exclude int) []int {
	indexes := make([]int, 0, count)
	for index := 0; index < count; index++ {
		if index != exclude {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func isDirectStreamType(value string) bool {
	return strings.EqualFold(value, "m3u8") || strings.EqualFold(value, "mpd")
}

func containsStreamURL(streams []MediaStream, target string) bool {
	for _, stream := range streams {
		if stream.URL == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// episodePageURL trusts the slug the site itself reports, because the suffix
// differs per host ("tap-1", "tap-1-sv-0", "tap-1-3097673", "tap-full"). Only
// when a record carries no slug is a plain "tap-N" built as a last resort.
func episodePageURL(page *url.URL, seriesPath, slug string, number int) string {
	slug = strings.TrimSpace(slug)
	if strings.HasPrefix(strings.ToLower(slug), "http") {
		if parsed, err := url.Parse(slug); err == nil && parsed.Host != "" {
			return parsed.String()
		}
	}
	target := *page
	target.RawQuery = ""
	target.Fragment = ""
	switch {
	case strings.HasPrefix(slug, "/"):
		target.Path = slug
	case slug != "":
		target.Path = strings.TrimRight(seriesPath, "/") + "/" + strings.Trim(slug, "/")
	case number > 0:
		target.Path = strings.TrimRight(seriesPath, "/") + "/tap-" + strconv.Itoa(number)
	default:
		target.Path = strings.TrimRight(seriesPath, "/") + "/tap-full"
	}
	return target.String()
}

type episodeAPIResponse struct {
	Status  string `json:"status"`
	Servers []struct {
		Name  string          `json:"name"`
		Items []episodeRecord `json:"items"`
	} `json:"servers"`
}

func fetchEpisodeIndex(pageURL, movieID string) ([]Episode, error) {
	page, err := url.Parse(pageURL)
	if err != nil || page.Scheme == "" || page.Host == "" || movieID == "" {
		return nil, fmt.Errorf("không đủ thông tin để tải danh sách tập")
	}
	endpoint := page.Scheme + "://" + page.Host + "/baseapi/episodes?movie_id=" + url.QueryEscape(movieID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", pageURL)
	resp, err := (&http.Client{Timeout: 35 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("không thể tải khung danh sách tập: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API danh sách tập trả về HTTP %d", resp.StatusCode)
	}
	var payload episodeAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if !strings.EqualFold(payload.Status, "success") || len(payload.Servers) == 0 {
		return nil, fmt.Errorf("API danh sách tập không có dữ liệu")
	}

	currentSlug, currentNumber, isPlayerPage := episodePathSegment(page.Path)
	if !isPlayerPage {
		currentSlug = path.Base(strings.TrimRight(page.Path, "/"))
	}
	selectedServer := -1
	for serverIndex, server := range payload.Servers {
		for _, item := range server.Items {
			if strings.EqualFold(item.Slug, currentSlug) {
				selectedServer = serverIndex
				break
			}
		}
		if selectedServer >= 0 {
			break
		}
	}
	if selectedServer < 0 {
		selectedServer = 0
	}
	seriesPath := seriesPathOf(page.Path)
	type candidate struct {
		item    episodeRecord
		streams []MediaStream
	}
	byNumber := map[int]*candidate{}
	// Every server is collected, starting with the one the open page belongs to.
	// A single episode often lives on several servers, so one dead CDN link no
	// longer means the episode fails.
	for _, serverIndex := range append([]int{selectedServer}, otherServerIndexes(len(payload.Servers), selectedServer)...) {
		server := payload.Servers[serverIndex]
		serverName := strings.TrimSpace(server.Name)
		for _, item := range server.Items {
			number, found := episodeSlugNumber(item.Slug)
			if !found {
				number, found = episodeSlugNumber(item.Name)
			}
			if !found {
				continue
			}
			entry := byNumber[number]
			if entry == nil {
				entry = &candidate{}
				byNumber[number] = entry
			}
			// The record that names the episode is the first one seen, unless a
			// later one is a direct stream and the current pick is not.
			if entry.item.Slug == "" || (isDirectStreamType(item.Type) && !isDirectStreamType(entry.item.Type)) {
				entry.item = item
			}
			for _, stream := range extractMedia(item.Link, pageURL) {
				if stream.Server == "" {
					stream.Server = firstNonEmpty(serverName, strings.TrimSpace(item.Server), strings.TrimSpace(item.Name))
				}
				if !containsStreamURL(entry.streams, stream.URL) {
					entry.streams = append(entry.streams, stream)
				}
			}
		}
	}
	result := make([]Episode, 0, len(byNumber))
	for number, value := range byNumber {
		name := strings.TrimSpace(value.item.Name)
		if name == "" {
			name = episodeDisplayName(number)
		}
		primary := ""
		if len(value.streams) > 0 {
			primary = value.streams[0].URL
		}
		result = append(result, Episode{
			ID:        episodeIdentity(number),
			Name:      name,
			Number:    number,
			PageURL:   episodePageURL(page, seriesPath, value.item.Slug, number),
			StreamURL: primary,
			Streams:   value.streams,
			Current:   number == currentNumber && isPlayerPage,
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Number < result[j].Number })
	if len(result) == 0 {
		return nil, fmt.Errorf("khung danh sách tập không có tập tải được")
	}
	if currentNumber == 0 {
		result[0].Current = true
	}
	return result, nil
}
