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
	episodeNumberRE    = regexp.MustCompile(`(?i)(?:tap|tập|episode|ep)[-_]?(\d+)`)
	titlePattern       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	canonicalPattern   = regexp.MustCompile(`(?i)<link[^>]+rel\s*=\s*["']canonical["'][^>]+href\s*=\s*["'](https?://[^"']+)`)
	ogURLPattern       = regexp.MustCompile(`(?i)<meta[^>]+property\s*=\s*["']og:url["'][^>]+content\s*=\s*["'](https?://[^"']+)`)
	hrefPattern        = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
	movieIDPattern     = regexp.MustCompile(`(?i)["']movieId["']\s*:\s*["']?(\d+)`)
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

func extractTitle(source string) string {
	match := titlePattern.FindStringSubmatch(normalizeHTML(source))
	if len(match) < 2 {
		return "Video"
	}
	title := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(html.UnescapeString(match[1]), " "))
	title = regexp.MustCompile(`(?i)^xem\s+phim\s+`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`(?i)\s+(tập|episode)\s+\d+.*$`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`(?i)\s*[-|]\s*motchill.*$`).ReplaceAllString(title, "")
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
	currentNumber := episodeNumber(base.Path)
	segments := strings.Split(strings.Trim(base.Path, "/"), "/")
	if len(segments) < 2 {
		return nil
	}
	if episodeNumber(segments[len(segments)-1]) > 0 {
		segments = segments[:len(segments)-1]
	}
	seriesPath := "/" + strings.Join(segments, "/")
	seen := map[string]bool{}
	result := make([]Episode, 0)
	for _, match := range hrefPattern.FindAllStringSubmatch(normalizeHTML(source), -1) {
		if len(match) < 2 {
			continue
		}
		reference, err := url.Parse(html.UnescapeString(match[1]))
		if err != nil {
			continue
		}
		absolute := base.ResolveReference(reference)
		if !strings.EqualFold(absolute.Host, base.Host) || !strings.HasPrefix(absolute.Path, seriesPath+"/") {
			continue
		}
		number := episodeNumber(absolute.Path)
		if number <= 0 || seen[absolute.String()] {
			continue
		}
		seen[absolute.String()] = true
		result = append(result, Episode{
			ID:      fmt.Sprintf("episode-%d", number),
			Name:    fmt.Sprintf("Tập %02d", number),
			Number:  number,
			PageURL: absolute.String(),
			Current: number == currentNumber,
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Number < result[j].Number })
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

	currentSlug := path.Base(strings.TrimRight(page.Path, "/"))
	currentNumber := episodeNumber(currentSlug)
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
	items := payload.Servers[selectedServer].Items
	seriesPath := strings.TrimRight(page.Path, "/")
	if currentNumber > 0 {
		seriesPath = strings.TrimSuffix(seriesPath, "/"+currentSlug)
	}
	type candidate struct {
		item   episodeRecord
		stream string
	}
	byNumber := map[int]candidate{}
	for _, item := range items {
		number := episodeNumber(item.Slug)
		if number <= 0 {
			number = episodeNumber(item.Name)
		}
		if number <= 0 {
			continue
		}
		streams := extractMedia(item.Link, pageURL)
		streamURL := ""
		if len(streams) > 0 {
			streamURL = streams[0].URL
		}
		previous, exists := byNumber[number]
		isDirect := strings.EqualFold(item.Type, "m3u8") || strings.EqualFold(item.Type, "mpd")
		previousDirect := strings.EqualFold(previous.item.Type, "m3u8") || strings.EqualFold(previous.item.Type, "mpd")
		if !exists || (isDirect && !previousDirect) || (previous.stream == "" && streamURL != "") {
			byNumber[number] = candidate{item: item, stream: streamURL}
		}
	}
	result := make([]Episode, 0, len(byNumber))
	for number, value := range byNumber {
		name := strings.TrimSpace(value.item.Name)
		if name == "" {
			name = fmt.Sprintf("Tập %02d", number)
		}
		pageCopy := *page
		pageCopy.Path = strings.TrimRight(seriesPath, "/") + "/tap-" + strconv.Itoa(number)
		pageCopy.RawQuery = ""
		pageCopy.Fragment = ""
		result = append(result, Episode{
			ID:        fmt.Sprintf("episode-%d", number),
			Name:      name,
			Number:    number,
			PageURL:   pageCopy.String(),
			StreamURL: value.stream,
			Current:   number == currentNumber,
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
