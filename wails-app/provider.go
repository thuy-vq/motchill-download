package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The PO token provider is a third-party project of the yt-dlp ecosystem. The
// app fetches it from that project's own GitHub release, builds it with the Node
// toolchain already on the machine, and runs it bound to localhost. Every step is
// logged, and nothing here runs unless the user asks for it.
const (
	providerRepo        = "Brainicism/bgutil-ytdlp-pot-provider"
	providerPluginAsset = "bgutil-ytdlp-pot-provider.zip"
	providerDefaultPort = 4416
	// providerStartTimeout is how long the server may take to answer /ping.
	providerStartTimeout = 40 * time.Second
)

// ProviderStatus is what the interface shows about the provider.
type ProviderStatus struct {
	PluginInstalled bool   `json:"pluginInstalled"`
	PluginDir       string `json:"pluginDir"`
	ServerInstalled bool   `json:"serverInstalled"`
	ServerDir       string `json:"serverDir"`
	Running         bool   `json:"running"`
	Port            int    `json:"port"`
	Version         string `json:"version"`
	NodeVersion     string `json:"nodeVersion"`
}

type providerRunner struct {
	mu      sync.Mutex
	process *os.Process
	port    int
}

var runningProvider = &providerRunner{}

func providerRoot() string      { return filepath.Join(toolInstallRoot(), "pot-provider") }
func providerServerDir() string { return filepath.Join(providerRoot(), "server") }
func providerPluginDir() string { return filepath.Join(toolInstallRoot(), "yt-dlp-plugins") }
func providerBuiltEntry() string {
	return filepath.Join(providerServerDir(), "build", "main.js")
}

func (a *App) GetProviderStatus() ProviderStatus {
	runningProvider.mu.Lock()
	running := runningProvider.process != nil
	port := runningProvider.port
	runningProvider.mu.Unlock()
	if port == 0 {
		port = providerDefaultPort
	}
	status := ProviderStatus{
		PluginDir:   providerPluginDir(),
		ServerDir:   providerServerDir(),
		Running:     running,
		Port:        port,
		Version:     readProviderVersion(),
		NodeVersion: toolVersion("node", "--version"),
	}
	if entries, err := os.ReadDir(providerPluginDir()); err == nil && len(entries) > 0 {
		status.PluginInstalled = true
	}
	if info, err := os.Stat(providerBuiltEntry()); err == nil && info.Size() > 0 {
		status.ServerInstalled = true
	}
	return status
}

func readProviderVersion() string {
	data, err := os.ReadFile(filepath.Join(providerRoot(), "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func toolVersion(name string, arguments ...string) string {
	resolved, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	command := exec.Command(resolved, arguments...)
	prepareBackgroundCommand(command)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// latestProviderTag asks GitHub which release to install.
func latestProviderTag() (string, error) {
	request, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/"+providerRepo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return "", fmt.Errorf("không hỏi được GitHub: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub trả về HTTP %d", response.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("không đọc được tag của bản phát hành")
	}
	return release.TagName, nil
}

// InstallPOTokenProvider downloads the yt-dlp plugin and builds the provider
// server next to it, then points the tuning at both.
func (a *App) InstallPOTokenProvider() (ProviderStatus, error) {
	tag, err := latestProviderTag()
	if err != nil {
		return ProviderStatus{}, err
	}
	a.emit("download:log", fmt.Sprintf("PO token provider: cài bản %s từ github.com/%s", tag, providerRepo))
	// A running server keeps its files open, which would fail the reinstall.
	if _, err := a.StopPOTokenProvider(); err != nil {
		return ProviderStatus{}, err
	}

	if err := a.installProviderPlugin(tag); err != nil {
		return ProviderStatus{}, err
	}
	if err := a.buildProviderServer(tag); err != nil {
		return ProviderStatus{}, err
	}
	if err := os.WriteFile(filepath.Join(providerRoot(), "VERSION"), []byte(tag), 0o600); err != nil {
		return ProviderStatus{}, err
	}

	tuning := a.GetYtDlpTuning()
	tuning.PluginDir = providerPluginDir()
	if err := a.RememberYtDlpTuning(tuning); err != nil {
		return ProviderStatus{}, err
	}
	return a.GetProviderStatus(), nil
}

// installProviderPlugin unpacks the release zip into the yt-dlp plugin folder.
func (a *App) installProviderPlugin(tag string) error {
	target := providerPluginDir()
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	source := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", providerRepo, tag, providerPluginAsset)
	archivePath := filepath.Join(os.TempDir(), "bgutil-plugin-"+randomSuffix()+".zip")
	defer os.Remove(archivePath)
	if err := downloadFile(source, archivePath); err != nil {
		return fmt.Errorf("không tải được plugin: %w", err)
	}
	count, err := unzipInto(archivePath, target)
	if err != nil {
		return fmt.Errorf("không giải nén được plugin: %w", err)
	}
	a.emit("download:log", fmt.Sprintf("PO token provider: đã cài %d file plugin vào %s", count, target))
	return nil
}

// buildProviderServer fetches the source of the same tag and builds it with the
// Node toolchain. Dependency install scripts are disabled on purpose.
func (a *App) buildProviderServer(tag string) error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("cần Node.js để chạy provider; hãy cài Node rồi thử lại")
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("cần npm để dựng provider; hãy cài Node.js rồi thử lại")
	}
	root := providerRoot()
	if err := os.RemoveAll(providerServerDir()); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	tarball := filepath.Join(os.TempDir(), "bgutil-src-"+randomSuffix()+".tar.gz")
	defer os.Remove(tarball)
	source := fmt.Sprintf("https://codeload.github.com/%s/tar.gz/refs/tags/%s", providerRepo, tag)
	a.emit("download:log", "PO token provider: tải mã nguồn "+source)
	if err := downloadFile(source, tarball); err != nil {
		return fmt.Errorf("không tải được mã nguồn provider: %w", err)
	}
	extracted, err := extractTarSubdir(tarball, "server", providerServerDir())
	if err != nil {
		return fmt.Errorf("không giải nén được mã nguồn: %w", err)
	}
	a.emit("download:log", fmt.Sprintf("PO token provider: giải nén %d file vào %s", extracted, providerServerDir()))

	// --ignore-scripts keeps install hooks of the dependency tree from running.
	if err := a.runProviderCommand(npm, providerServerDir(), "install", "--ignore-scripts", "--no-audit", "--no-fund"); err != nil {
		return fmt.Errorf("npm install thất bại: %w", err)
	}
	// The project has no npm "build" script: its Dockerfile compiles with tsc.
	// The compiler installed in node_modules is invoked directly, which avoids
	// the shell shims npx would go through.
	if err := a.buildProviderTypeScript(); err != nil {
		return fmt.Errorf("build provider thất bại: %w", err)
	}
	if info, statErr := os.Stat(providerBuiltEntry()); statErr != nil || info.Size() == 0 {
		return fmt.Errorf("không thấy %s sau khi build", providerBuiltEntry())
	}
	return nil
}

// buildProviderTypeScript compiles the server with the TypeScript that npm just
// installed, falling back to npx when the direct entry point is missing.
func (a *App) buildProviderTypeScript() error {
	node, err := exec.LookPath("node")
	if err != nil {
		return err
	}
	compiler := filepath.Join(providerServerDir(), "node_modules", "typescript", "bin", "tsc")
	if info, statErr := os.Stat(compiler); statErr == nil && !info.IsDir() {
		return a.runProviderCommand(node, providerServerDir(), compiler)
	}
	npx, err := exec.LookPath("npx")
	if err != nil {
		return fmt.Errorf("không tìm thấy TypeScript trong node_modules và cũng không có npx")
	}
	return a.runProviderCommand(npx, providerServerDir(), "tsc")
}

func (a *App) runProviderCommand(name, directory string, arguments ...string) error {
	a.emit("download:log", fmt.Sprintf("PO token provider: chạy %s %s", filepath.Base(name), strings.Join(arguments, " ")))
	command := exec.Command(name, arguments...)
	command.Dir = directory
	prepareBackgroundCommand(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w — %s", err, lastLine(string(output)))
	}
	return nil
}

// StartPOTokenProvider runs the server on localhost and waits for it to answer.
func (a *App) StartPOTokenProvider(port int) (ProviderStatus, error) {
	if port <= 0 {
		port = providerDefaultPort
	}
	if info, err := os.Stat(providerBuiltEntry()); err != nil || info.Size() == 0 {
		return ProviderStatus{}, fmt.Errorf("provider chưa được cài; hãy bấm Cài provider trước")
	}
	if _, err := a.StopPOTokenProvider(); err != nil {
		return ProviderStatus{}, err
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return ProviderStatus{}, fmt.Errorf("không tìm thấy Node.js")
	}
	command := exec.Command(node, "build/main.js", "--port", strconv.Itoa(port))
	command.Dir = providerServerDir()
	prepareBackgroundCommand(command)
	if err := command.Start(); err != nil {
		return ProviderStatus{}, err
	}
	// The job object ties the server to this app, so it cannot linger after exit.
	if superviseErr := superviseProcess(command.Process); superviseErr != nil {
		a.emit("download:log", fmt.Sprintf("PO token provider: %v", superviseErr))
	}
	runningProvider.mu.Lock()
	runningProvider.process = command.Process
	runningProvider.port = port
	runningProvider.mu.Unlock()
	go func() { _ = command.Wait() }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForProvider(base, providerStartTimeout); err != nil {
		_, _ = a.StopPOTokenProvider()
		return ProviderStatus{}, err
	}
	tuning := a.GetYtDlpTuning()
	tuning.ProviderURL = base
	tuning.PluginDir = providerPluginDir()
	if err := a.RememberYtDlpTuning(tuning); err != nil {
		return ProviderStatus{}, err
	}
	a.emit("download:log", "PO token provider: đang chạy tại "+base)
	return a.GetProviderStatus(), nil
}

func (a *App) StopPOTokenProvider() (ProviderStatus, error) {
	runningProvider.mu.Lock()
	process := runningProvider.process
	runningProvider.process = nil
	runningProvider.mu.Unlock()
	if process != nil {
		_ = killProcessTree(process)
		a.emit("download:log", "PO token provider: đã dừng")
	}
	return a.GetProviderStatus(), nil
}

// waitForProvider polls until the server answers, so a failed start becomes a
// clear error instead of a silent misconfiguration.
func waitForProvider(base string, timeout time.Duration) error {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/ping")
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
			response.Body.Close()
			if response.StatusCode < 500 {
				return nil
			}
			lastErr = fmt.Errorf("provider trả về HTTP %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("provider không phản hồi sau %s: %v", timeout, lastErr)
}

func downloadFile(source, target string) error {
	response, err := http.Get(source)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, 200<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// unzipInto extracts an archive, refusing entries that would escape the target.
func unzipInto(archivePath, target string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	count := 0
	for _, file := range reader.File {
		destination, err := safeJoin(target, file.Name)
		if err != nil {
			return count, err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return count, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return count, err
		}
		input, err := file.Open()
		if err != nil {
			return count, err
		}
		output, err := os.Create(destination)
		if err != nil {
			input.Close()
			return count, err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, 100<<20))
		input.Close()
		output.Close()
		if copyErr != nil {
			return count, copyErr
		}
		count++
	}
	return count, nil
}

// extractTarSubdir pulls one directory out of a GitHub source tarball, whose
// entries all sit under a single generated root folder.
func extractTarSubdir(archivePath, subdir, target string) (int, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, err
	}
	defer gzipReader.Close()
	if err := os.MkdirAll(target, 0o755); err != nil {
		return 0, err
	}
	reader := tar.NewReader(gzipReader)
	count := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		parts := strings.SplitN(header.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		inner := parts[1]
		if inner != subdir && !strings.HasPrefix(inner, subdir+"/") {
			continue
		}
		inner = strings.TrimPrefix(strings.TrimPrefix(inner, subdir), "/")
		if inner == "" {
			continue
		}
		destination, err := safeJoin(target, inner)
		if err != nil {
			return count, err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return count, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return count, err
			}
			output, err := os.Create(destination)
			if err != nil {
				return count, err
			}
			_, copyErr := io.Copy(output, io.LimitReader(reader, 100<<20))
			output.Close()
			if copyErr != nil {
				return count, copyErr
			}
			count++
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("không tìm thấy thư mục %q trong gói", subdir)
	}
	return count, nil
}

// hasDriveLetter spots "C:/..." style entries, which are absolute on Windows.
func hasDriveLetter(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	letter := value[0]
	return (letter >= 'a' && letter <= 'z') || (letter >= 'A' && letter <= 'Z')
}

// safeJoin blocks archive entries that try to write outside the target folder.
func safeJoin(target, name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	// filepath.IsAbs is platform specific — on Windows "/etc/passwd" is not
	// absolute — so leading separators and drive letters are checked directly.
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(normalized) || hasDriveLetter(normalized) {
		return "", fmt.Errorf("mục trong gói có đường dẫn tuyệt đối: %s", name)
	}
	cleaned := filepath.Clean(normalized)
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("mục trong gói có đường dẫn không hợp lệ: %s", name)
	}
	destination := filepath.Join(target, cleaned)
	if !strings.HasPrefix(destination, filepath.Clean(target)+string(os.PathSeparator)) &&
		destination != filepath.Clean(target) {
		return "", fmt.Errorf("mục trong gói thoát khỏi thư mục đích: %s", name)
	}
	return destination, nil
}
