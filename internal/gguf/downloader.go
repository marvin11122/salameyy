package gguf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	maxModelInfoBytes = 16 << 20
	maxDownloadName   = 512
)

type ProgressFunc func(downloaded, total int64, speed float64)

type HFFile struct {
	Filename string `json:"rfilename"`
	Size     int64  `json:"size"`
	Sha256   string `json:"sha256,omitempty"`
}

type HFModelInfo struct {
	ID           string   `json:"id"`
	Siblings     []HFFile `json:"siblings"`
	Private      bool     `json:"private"`
	Disabled     bool     `json:"disabled"`
	LastModified string   `json:"lastModified,omitempty"`
}

type DownloadOptions struct {
	ModelID        string
	Filename       string
	Revision       string
	OutputDir      string
	VerifyChecksum bool
	Resume         bool
	Token          string
	Timeout        time.Duration
}

type DownloadProgress struct {
	ModelID    string
	Filename   string
	TotalBytes int64
	Downloaded int64
	Speed      float64
	Status     DownloadStatus
	Error      error
	FilePath   string
}

type DownloadStatus string

const (
	StatusPending     DownloadStatus = "pending"
	StatusFetching    DownloadStatus = "fetching"
	StatusDownloading DownloadStatus = "downloading"
	StatusVerifying   DownloadStatus = "verifying"
	StatusCompleted   DownloadStatus = "completed"
	StatusFailed      DownloadStatus = "failed"
	StatusCancelled   DownloadStatus = "cancelled"
)

type DownloadResult struct {
	FilePath   string
	ModelID    string
	Filename   string
	TotalBytes int64
	Checksum   string
}

type Downloader struct {
	dataDir    string
	httpClient *http.Client
	mu         sync.Mutex
	active     map[string]*downloadSession
}

type downloadSession struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewDownloader(dataDir string) *Downloader {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Downloader{
		dataDir: dataDir,
		httpClient: &http.Client{
			Timeout:   30 * time.Minute,
			Transport: transport,
		},
		active: make(map[string]*downloadSession),
	}
}

func NewDownloaderWithClient(dataDir string, client *http.Client) *Downloader {
	d := NewDownloader(dataDir)
	if client != nil {
		d.httpClient = client
	}
	return d
}

func (d *Downloader) client() *http.Client {
	if d.httpClient == nil {
		return http.DefaultClient
	}
	return d.httpClient
}

func (d *Downloader) GetModelInfo(ctx context.Context, modelID string, opts ...RequestOption) (*HFModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint, err := huggingFaceModelURL(modelID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for _, option := range opts {
		if option != nil {
			option(req)
		}
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch model info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("model request returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelInfoBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxModelInfoBytes {
		return nil, fmt.Errorf("model response is too large")
	}
	var info HFModelInfo
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode response: multiple JSON values")
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &info, nil
}

func (d *Downloader) GetGGUFFiles(ctx context.Context, modelID string, opts ...RequestOption) ([]HFFile, error) {
	info, err := d.GetModelInfo(ctx, modelID, opts...)
	if err != nil {
		return nil, err
	}
	files := make([]HFFile, 0, len(info.Siblings))
	for _, file := range info.Siblings {
		if isGGUFName(file.Filename) {
			files = append(files, file)
		}
	}
	return files, nil
}

func (d *Downloader) AutoDiscoverGGUFFiles(ctx context.Context, modelID string, opts ...RequestOption) (map[string]HFFile, error) {
	files, err := d.GetGGUFFiles(ctx, modelID, opts...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]HFFile, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Filename) != "" {
			result[file.Filename] = file
		}
	}
	return result, nil
}

func (d *Downloader) Download(ctx context.Context, opts DownloadOptions, progress ProgressFunc) (*DownloadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	modelID := strings.TrimSpace(opts.ModelID)
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	if _, err := huggingFaceModelURL(modelID); err != nil {
		return nil, err
	}
	if opts.Revision == "" {
		opts.Revision = "main"
	}
	if err := validateRevision(opts.Revision); err != nil {
		return nil, err
	}
	info, err := d.GetModelInfo(ctx, modelID, d.tokenHeader(opts.Token))
	if err != nil {
		return nil, fmt.Errorf("fetch model info: %w", err)
	}
	target, err := selectDownloadFile(info.Siblings, opts.Filename)
	if err != nil {
		return nil, fmt.Errorf("select model file: %w", err)
	}
	opts.Filename = target.Filename
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(d.dataDir, "models")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	fileName, err := safeDownloadName(target.Filename)
	if err != nil {
		return nil, err
	}
	outputPath := filepath.Join(outputDir, fileName)
	if err := ensureDownloadTarget(outputPath); err != nil {
		return nil, err
	}
	if existingSize, statErr := os.Stat(outputPath); statErr == nil {
		if !existingSize.Mode().IsRegular() {
			return nil, fmt.Errorf("download target is not a regular file: %s", outputPath)
		}
		if target.Size > 0 && existingSize.Size() == target.Size {
			if !opts.VerifyChecksum || target.Sha256 == "" {
				return &DownloadResult{FilePath: outputPath, ModelID: modelID, Filename: target.Filename, TotalBytes: target.Size}, nil
			}
			if d.verifyChecksum(outputPath, target.Sha256) == nil {
				return &DownloadResult{FilePath: outputPath, ModelID: modelID, Filename: target.Filename, TotalBytes: target.Size, Checksum: target.Sha256}, nil
			}
			if err := os.Remove(outputPath); err != nil {
				return nil, fmt.Errorf("remove checksum-invalid download: %w", err)
			}
		}
	}
	result, err := d.downloadFile(ctx, target, opts, outputPath, progress)
	if err != nil {
		return nil, err
	}
	if opts.VerifyChecksum && target.Sha256 != "" {
		if err := d.verifyChecksum(outputPath, target.Sha256); err != nil {
			_ = os.Remove(outputPath)
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
		result.Checksum = target.Sha256
	}
	return result, nil
}

func (d *Downloader) DownloadWithTeaCmd(opts DownloadOptions) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cancel := context.CancelFunc(func() {})
		if opts.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		} else {
			ctx, cancel = context.WithCancel(ctx)
		}
		defer cancel()
		sessionID := downloadSessionID(opts.ModelID, opts.Filename)
		d.storeSession(sessionID, ctx, cancel)
		defer d.deleteSession(sessionID)
		var lastUpdate time.Time
		progress := func(downloaded, total int64, speed float64) {
			now := time.Now()
			if downloaded != total && now.Sub(lastUpdate) < 100*time.Millisecond {
				return
			}
			lastUpdate = now
		}
		result, err := d.Download(ctx, opts, progress)
		if err != nil {
			return DownloadCompleteMsg{Error: err, ModelID: opts.ModelID, Filename: opts.Filename}
		}
		return DownloadCompleteMsg{Result: result, ModelID: opts.ModelID, Filename: opts.Filename}
	}
}

func (d *Downloader) storeSession(id string, ctx context.Context, cancel context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active == nil {
		d.active = make(map[string]*downloadSession)
	}
	d.active[id] = &downloadSession{ctx: ctx, cancel: cancel}
}

func (d *Downloader) deleteSession(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.active, id)
}

func (d *Downloader) CancelDownload(modelID, filename string) {
	ids := []string{downloadSessionID(modelID, filename)}
	if strings.TrimSpace(filename) == "" {
		ids = append(ids, modelID)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, id := range ids {
		if session, ok := d.active[id]; ok && session.cancel != nil {
			session.cancel()
		}
	}
}

type DownloadProgressMsg struct {
	Progress DownloadProgress
}

type DownloadCompleteMsg struct {
	Result   *DownloadResult
	Error    error
	ModelID  string
	Filename string
}

type RequestOption func(*http.Request)

func WithToken(token string) RequestOption {
	return func(req *http.Request) {
		if req != nil && token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

func (d *Downloader) tokenHeader(token string) RequestOption {
	return WithToken(token)
}

func (d *Downloader) downloadFile(ctx context.Context, file HFFile, opts DownloadOptions, outputPath string, progress ProgressFunc) (*DownloadResult, error) {
	endpoint, err := huggingFaceResolveURL(opts.ModelID, opts.Revision, file.Filename)
	if err != nil {
		return nil, err
	}
	currentSize := int64(0)
	if opts.Resume {
		if info, statErr := os.Stat(outputPath); statErr == nil && info.Mode().IsRegular() {
			currentSize = info.Size()
			if file.Size > 0 && currentSize > file.Size {
				if err := os.Remove(outputPath); err != nil {
					return nil, fmt.Errorf("remove oversized partial download: %w", err)
				}
				currentSize = 0
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if opts.Token != "" {
		WithToken(opts.Token)(req)
	}
	if opts.Resume && currentSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", currentSize))
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()
	resuming := false
	switch resp.StatusCode {
	case http.StatusOK:
		currentSize = 0
	case http.StatusPartialContent:
		resuming = opts.Resume && currentSize > 0
		if !resuming {
			return nil, fmt.Errorf("download returned partial content without a valid resume request")
		}
		if err := validateContentRange(resp.Header.Get("Content-Range"), currentSize, file.Size); err != nil {
			return nil, err
		}
	case http.StatusNotFound:
		return nil, fmt.Errorf("file not found: %s", file.Filename)
	default:
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	totalSize, err := responseSize(resp, currentSize, file.Size, resuming)
	if err != nil {
		return nil, err
	}
	if file.Size > 0 && totalSize > 0 && totalSize != file.Size {
		return nil, fmt.Errorf("download size mismatch: got %d, expected %d", totalSize, file.Size)
	}
	if err := ensureDownloadTarget(outputPath); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if resuming {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	out, err := os.OpenFile(outputPath, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	downloaded := currentSize
	start := time.Now()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			_ = out.Close()
			return nil, ctx.Err()
		default:
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if file.Size > 0 && downloaded+int64(n) > file.Size {
				_ = out.Close()
				return nil, fmt.Errorf("download exceeded expected size %d", file.Size)
			}
			if _, err := out.Write(buf[:n]); err != nil {
				_ = out.Close()
				return nil, fmt.Errorf("write file: %w", err)
			}
			downloaded += int64(n)
			if progress != nil {
				elapsed := time.Since(start).Seconds()
				speed := float64(0)
				if elapsed > 0 {
					speed = float64(downloaded-currentSize) / elapsed
				}
				progress(downloaded, totalSize, speed)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = out.Close()
			return nil, fmt.Errorf("read response: %w", readErr)
		}
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return nil, fmt.Errorf("sync file: %w", err)
	}
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close file: %w", err)
	}
	if file.Size > 0 && downloaded != file.Size {
		return nil, fmt.Errorf("download size mismatch: got %d, expected %d", downloaded, file.Size)
	}
	if totalSize == 0 {
		totalSize = downloaded
	}
	return &DownloadResult{FilePath: outputPath, ModelID: opts.ModelID, Filename: file.Filename, TotalBytes: totalSize}, nil
}

func (d *Downloader) checkExistingFile(path string, expectedSize int64) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("not a regular file: %s", path)
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return info.Size(), fmt.Errorf("size mismatch: got %d, expected %d", info.Size(), expectedSize)
	}
	return info.Size(), nil
}

func (d *Downloader) verifyChecksum(path, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file for checksum: %w", err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("calculate checksum: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actual, expected)
	}
	return nil
}

func sanitizeModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	var b strings.Builder
	for _, r := range modelID {
		switch r {
		case '/', '\\', ':', '\x00':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func GetDefaultGGUFFiles() []string {
	return []string{
		"bartowski/Phi-3.5-mini-instruct-GGUF",
		"bartowski/Qwen2.5-Coder-7B-Instruct-GGUF",
		"mradermacher/DeepSeek-R1-Distill-Qwen-7B-GGUF",
		"bartowski/Llama-3.2-3B-Instruct-GGUF",
		"bartowski/Mistral-7B-Instruct-v0.3-GGUF",
	}
}

type ProgressRenderer struct {
	width int
	style lipgloss.Style
}

func NewProgressRenderer(width int) *ProgressRenderer {
	if width < 1 {
		width = 1
	}
	return &ProgressRenderer{width: width, style: lipgloss.NewStyle()}
}

func (r *ProgressRenderer) Render(progress DownloadProgress) string {
	if r == nil || progress.TotalBytes <= 0 {
		return "Downloading..."
	}
	pct := float64(progress.Downloaded) / float64(progress.TotalBytes)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	barWidth := r.width - 40
	if barWidth < 20 {
		barWidth = 20
	}
	filled := int(pct * float64(barWidth))
	empty := barWidth - filled
	line := fmt.Sprintf("%s %s/%s %s", strings.Repeat("█", filled)+strings.Repeat("░", empty), formatBytes(progress.Downloaded), formatBytes(progress.TotalBytes), formatSpeed(progress.Speed))
	switch progress.Status {
	case StatusCompleted:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(line)
	case StatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line)
	case StatusCancelled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(line)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(line)
	}
}

func formatBytes(b int64) string {
	if b < 0 {
		return "unknown"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return ""
	}
	return fmt.Sprintf("(%s/s)", formatBytes(int64(bytesPerSecond)))
}

func (d *Downloader) ModelDiscovery(ctx context.Context, repos []string, token string) (map[string][]HFFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make(map[string][]HFFile)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		wg.Add(1)
		go func(repoID string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			files, err := d.GetGGUFFiles(ctx, repoID, WithToken(token))
			if err != nil {
				return
			}
			mu.Lock()
			results[repoID] = files
			mu.Unlock()
		}(repo)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func (d *Downloader) ListDownloadedModels() ([]string, error) {
	modelsDir := filepath.Join(d.dataDir, "models")
	if _, err := os.Stat(modelsDir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var models []string
	err := filepath.WalkDir(modelsDir, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry != nil && !entry.IsDir() && isGGUFName(entry.Name()) {
			relative, err := filepath.Rel(modelsDir, filePath)
			if err == nil {
				models = append(models, relative)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk models directory: %w", err)
	}
	sort.Strings(models)
	return models, nil
}

func (d *Downloader) GetModelPath(modelID, filename string) string {
	name, err := safeDownloadName(filename)
	if err != nil {
		name = ""
	}
	return filepath.Join(d.dataDir, "models", sanitizeModelID(modelID), name)
}

func ParseModelID(modelID string) (owner, repo, revision string) {
	modelID = strings.TrimSpace(modelID)
	if at := strings.IndexByte(modelID, '@'); at >= 0 {
		revision = strings.TrimSpace(modelID[at+1:])
		modelID = modelID[:at]
	}
	parts := strings.Split(modelID, "/")
	if len(parts) == 2 {
		owner, repo = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	} else if len(parts) == 1 {
		repo = strings.TrimSpace(parts[0])
	}
	if revision == "" {
		revision = "main"
	}
	return owner, repo, revision
}

var _ interface {
	GetModelInfo(context.Context, string, ...RequestOption) (*HFModelInfo, error)
	GetGGUFFiles(context.Context, string, ...RequestOption) ([]HFFile, error)
	Download(context.Context, DownloadOptions, ProgressFunc) (*DownloadResult, error)
} = (*Downloader)(nil)

func isGGUFName(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".gguf")
}

func selectDownloadFile(files []HFFile, requested string) (HFFile, error) {
	requested = strings.TrimSpace(requested)
	for _, file := range files {
		if requested != "" && file.Filename == requested {
			if !isGGUFName(file.Filename) {
				return HFFile{}, fmt.Errorf("file %q is not a GGUF file", file.Filename)
			}
			if file.Size < 0 {
				return HFFile{}, fmt.Errorf("file %q has a negative size", file.Filename)
			}
			return file, nil
		}
	}
	if requested == "" {
		for _, file := range files {
			if isGGUFName(file.Filename) {
				if file.Size < 0 {
					return HFFile{}, fmt.Errorf("file %q has a negative size", file.Filename)
				}
				return file, nil
			}
		}
		return HFFile{}, fmt.Errorf("no GGUF files found")
	}
	return HFFile{}, fmt.Errorf("file %q not found", requested)
}

func ensureDownloadTarget(target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect download target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to download through a symlink: %s", target)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("download target is not a regular file: %s", target)
	}
	return nil
}

func safeDownloadName(name string) (string, error) {
	name, err := validateRemoteFilename(name)
	if err != nil {
		return "", err
	}
	name = path.Base(name)
	if _, err := validateRemoteFilename(name); err != nil {
		return "", err
	}
	return name, nil
}

func validateRemoteFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxDownloadName || strings.HasPrefix(name, "/") || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid download filename %q", name)
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid download filename %q", name)
		}
		for _, r := range part {
			if r < 0x20 || r == 0x7f {
				return "", fmt.Errorf("invalid download filename %q", name)
			}
		}
	}
	return strings.Join(parts, "/"), nil
}

func huggingFaceModelURL(modelID string) (string, error) {
	parts, err := modelPathParts(modelID)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "https", Host: "huggingface.co", Path: "/api/models/" + strings.Join(parts, "/")}
	return u.String(), nil
}

func huggingFaceResolveURL(modelID, revision, filename string) (string, error) {
	modelParts, err := modelPathParts(modelID)
	if err != nil {
		return "", err
	}
	if err := validateRevision(revision); err != nil {
		return "", err
	}
	name, err := validateRemoteFilename(filename)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "https", Host: "huggingface.co", Path: "/" + strings.Join(append(modelParts, "resolve", revision, name), "/")}
	return u.String(), nil
}

func modelPathParts(modelID string) ([]string, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || len(modelID) > 512 {
		return nil, fmt.Errorf("invalid model ID %q", modelID)
	}
	parts := strings.Split(modelID, "/")
	if len(parts) > 2 {
		return nil, fmt.Errorf("model ID must be owner/repo: %q", modelID)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\?#\x00") {
			return nil, fmt.Errorf("invalid model ID %q", modelID)
		}
		for _, r := range part {
			if r < 0x20 || r == 0x7f {
				return nil, fmt.Errorf("invalid model ID %q", modelID)
			}
		}
	}
	return parts, nil
}

func validateRevision(revision string) error {
	if revision == "" || len(revision) > 256 || strings.HasPrefix(revision, "/") || strings.ContainsAny(revision, "\\?#\x00") || strings.IndexFunc(revision, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("invalid model revision %q", revision)
	}
	for _, part := range strings.Split(revision, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid model revision %q", revision)
		}
	}
	return nil
}

func validateContentRange(value string, currentSize int64, expected int64) error {
	if value == "" {
		return fmt.Errorf("missing Content-Range on resumed download")
	}
	parts := strings.Split(value, " ")
	if len(parts) != 2 || parts[0] != "bytes" {
		return fmt.Errorf("invalid Content-Range %q", value)
	}
	rangeParts := strings.Split(parts[1], "/")
	if len(rangeParts) != 2 || rangeParts[0] == "" || rangeParts[1] == "" {
		return fmt.Errorf("invalid Content-Range %q", value)
	}
	startEnd := strings.Split(rangeParts[0], "-")
	if len(startEnd) != 2 {
		return fmt.Errorf("invalid Content-Range %q", value)
	}
	start, err := strconv.ParseInt(startEnd[0], 10, 64)
	if err != nil || start != currentSize {
		return fmt.Errorf("invalid resume start in Content-Range %q", value)
	}
	end, err := strconv.ParseInt(startEnd[1], 10, 64)
	if err != nil || end < start {
		return fmt.Errorf("invalid resume end in Content-Range %q", value)
	}
	if rangeParts[1] != "*" {
		total, err := strconv.ParseInt(rangeParts[1], 10, 64)
		if err != nil || total < 0 || (total > 0 && end >= total) {
			return fmt.Errorf("invalid resume total in Content-Range %q", value)
		}
		if expected > 0 && total != expected {
			return fmt.Errorf("resume size mismatch: got %d, expected %d", total, expected)
		}
	}
	return nil
}

func responseSize(resp *http.Response, currentSize, expected int64, resuming bool) (int64, error) {
	total := expected
	if total < 0 {
		total = 0
	}
	if resp.ContentLength > 0 {
		contentLength := resp.ContentLength
		if resuming {
			if currentSize > math.MaxInt64-contentLength {
				return 0, fmt.Errorf("download size overflows int64")
			}
			contentLength += currentSize
		}
		if total == 0 {
			total = contentLength
		} else if contentLength != total {
			return 0, fmt.Errorf("download size mismatch: got %d, expected %d", contentLength, total)
		}
	}
	return total, nil
}

func downloadSessionID(modelID, filename string) string {
	return strings.TrimSpace(modelID) + "/" + strings.TrimSpace(filename)
}
