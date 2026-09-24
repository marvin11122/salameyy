package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wiesnermarven-hue/salameyy/internal/gguf"
)

const maxRegistryBytes = 32 << 20

type ModelStatus string

const (
	StatusUnknown    ModelStatus = "unknown"
	StatusDiscovered ModelStatus = "discovered"
	StatusValidated  ModelStatus = "validated"
	StatusLoading    ModelStatus = "loading"
	StatusLoaded     ModelStatus = "loaded"
	StatusError      ModelStatus = "error"
	StatusMissing    ModelStatus = "missing"
)

type ModelEntry struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Path       string              `json:"path"`
	Status     ModelStatus         `json:"status"`
	Metadata   *gguf.ModelMetadata `json:"-"`
	AddedAt    time.Time           `json:"added_at"`
	LastUsed   time.Time           `json:"last_used_at,omitempty"`
	UseCount   int64               `json:"use_count"`
	Tags       []string            `json:"tags,omitempty"`
	Notes      string              `json:"notes,omitempty"`
	Error      string              `json:"error,omitempty"`
	CustomName string              `json:"custom_name,omitempty"`
}

func (m *ModelEntry) GetDisplayName() string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.CustomName) != "" {
		return m.CustomName
	}
	if m.Metadata != nil && strings.TrimSpace(m.Metadata.Name) != "" {
		return m.Metadata.Name
	}
	return m.Name
}

func (m *ModelEntry) GetParameterCount() string {
	if m == nil || m.Metadata == nil {
		return "unknown"
	}
	return m.Metadata.ParameterCountString()
}

func (m *ModelEntry) GetArchitecture() string {
	if m == nil || m.Metadata == nil || m.Metadata.Architecture == "" {
		return "unknown"
	}
	return m.Metadata.Architecture
}

func (m *ModelEntry) GetContextLength() uint64 {
	if m == nil || m.Metadata == nil {
		return 0
	}
	return m.Metadata.ContextLength
}

func (m *ModelEntry) GetFileSizeGB() float64 {
	if m == nil || m.Metadata == nil || m.Metadata.FileSize <= 0 {
		return 0
	}
	return float64(m.Metadata.FileSize) / (1024 * 1024 * 1024)
}

func (m *ModelEntry) GetEstimatedMemoryGB() float64 {
	if m == nil || m.Metadata == nil {
		return 0
	}
	return m.Metadata.EstimatedMemoryGB()
}

func (m *ModelEntry) GetQuantType() string {
	if m == nil || m.Metadata == nil {
		return "unknown"
	}
	return m.Metadata.QuantType()
}

type Registry struct {
	mu      sync.RWMutex
	models  map[string]*ModelEntry
	dataDir string
	path    string
}

func NewRegistry(dataDir string) *Registry {
	return &Registry{models: make(map[string]*ModelEntry), dataDir: dataDir, path: filepath.Join(dataDir, "model_registry.json")}
}

func (r *Registry) Load() error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.models == nil {
		r.models = make(map[string]*ModelEntry)
	}
	data, err := readRegistryFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.models = make(map[string]*ModelEntry)
			return nil
		}
		return err
	}
	var entries []ModelEntry
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("decode registry: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode registry: multiple JSON values")
		}
		return fmt.Errorf("decode registry: %w", err)
	}
	loaded := make(map[string]*ModelEntry, len(entries))
	for i := range entries {
		entry := entries[i]
		if strings.TrimSpace(entry.Path) == "" {
			return fmt.Errorf("registry entry %d has an empty path", i)
		}
		absPath, err := filepath.Abs(entry.Path)
		if err != nil {
			return fmt.Errorf("resolve registry entry %d path: %w", i, err)
		}
		entry.Path = filepath.Clean(absPath)
		if strings.TrimSpace(entry.ID) == "" {
			entry.ID = generateID(entry.Path)
		}
		if entry.Name == "" {
			entry.Name = filepath.Base(entry.Path)
		}
		if entry.AddedAt.IsZero() {
			entry.AddedAt = time.Now().UTC()
		}
		if entry.UseCount < 0 {
			entry.UseCount = 0
		}
		if _, exists := loaded[entry.ID]; exists {
			return fmt.Errorf("registry contains duplicate model ID %q", entry.ID)
		}
		entry.Tags = cloneStrings(entry.Tags)
		entry.Metadata = nil
		refreshEntry(&entry)
		loaded[entry.ID] = &entry
	}
	r.models = loaded
	return nil
}

func (r *Registry) Save() error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := make([]ModelEntry, 0, len(r.models))
	for _, model := range r.models {
		if model == nil {
			continue
		}
		entry := cloneEntry(model)
		entry.Metadata = nil
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i].GetDisplayName(), entries[j].GetDisplayName()
		if left == right {
			return entries[i].ID < entries[j].ID
		}
		return left < right
	})
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(r.path), ".model_registry-*.tmp")
	if err != nil {
		return fmt.Errorf("create registry temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set registry permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write registry: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync registry: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close registry: %w", err)
	}
	if err := os.Rename(temporaryName, r.path); err != nil {
		return fmt.Errorf("replace registry: %w", err)
	}
	return nil
}

func (r *Registry) Discover(dirs ...string) ([]*ModelEntry, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	files, err := gguf.DiscoverGGUF(dirs...)
	if err != nil {
		return nil, err
	}
	discovered := make([]*ModelEntry, 0, len(files))
	for _, file := range files {
		entry, addErr := r.AddModel(file)
		if addErr != nil {
			continue
		}
		discovered = append(discovered, entry)
	}
	return discovered, nil
}

func (r *Registry) AddModel(path string) (*ModelEntry, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	absPath = filepath.Clean(absPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat model: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("model path is not a regular file: %s", absPath)
	}
	r.mu.RLock()
	for _, existing := range r.models {
		if existing != nil && existing.Path == absPath {
			result := cloneEntry(existing)
			r.mu.RUnlock()
			return result, nil
		}
	}
	r.mu.RUnlock()
	metadata, err := gguf.Parse(absPath)
	if err != nil {
		return nil, fmt.Errorf("parse GGUF: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.models == nil {
		r.models = make(map[string]*ModelEntry)
	}
	for _, existing := range r.models {
		if existing != nil && existing.Path == absPath {
			return cloneEntry(existing), nil
		}
	}
	id := generateID(absPath)
	for attempt := 0; ; attempt++ {
		if _, exists := r.models[id]; !exists {
			break
		}
		id = generateID(fmt.Sprintf("%s\x00%d\x00%d", absPath, time.Now().UnixNano(), attempt))
	}
	entry := &ModelEntry{ID: id, Name: filepath.Base(absPath), Path: absPath, Status: StatusValidated, Metadata: metadata, AddedAt: time.Now().UTC(), Tags: []string{}}
	r.models[id] = entry
	return cloneEntry(entry), nil
}

func (r *Registry) RemoveModel(id string) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[id]; !ok {
		return fmt.Errorf("model %q not found", id)
	}
	delete(r.models, id)
	return nil
}

func (r *Registry) GetModel(id string) (*ModelEntry, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.models[id]
	if !ok {
		return nil, false
	}
	return cloneEntry(entry), true
}

func (r *Registry) GetModelByPath(path string) (*ModelEntry, bool) {
	if r == nil {
		return nil, false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, false
	}
	absPath = filepath.Clean(absPath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.models {
		if entry != nil && entry.Path == absPath {
			return cloneEntry(entry), true
		}
	}
	return nil, false
}

func (r *Registry) ListModels(filter ...string) []*ModelEntry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	entries := make([]*ModelEntry, 0, len(r.models))
	for _, entry := range r.models {
		if entry == nil || !matchesModel(entry, filter) {
			continue
		}
		entries = append(entries, cloneEntry(entry))
	}
	r.mu.RUnlock()
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if !left.LastUsed.IsZero() && !right.LastUsed.IsZero() {
			if !left.LastUsed.Equal(right.LastUsed) {
				return left.LastUsed.After(right.LastUsed)
			}
		} else if !left.LastUsed.IsZero() != !right.LastUsed.IsZero() {
			return !left.LastUsed.IsZero()
		}
		leftName, rightName := left.GetDisplayName(), right.GetDisplayName()
		if leftName == rightName {
			return left.ID < right.ID
		}
		return leftName < rightName
	})
	return entries
}

func (r *Registry) GetBestModel(purpose string) (*ModelEntry, error) {
	models := r.ListModels()
	if len(models) == 0 {
		return nil, fmt.Errorf("no models in registry")
	}
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if purpose == "coding" || purpose == "code" {
		for _, model := range models {
			name := strings.ToLower(model.GetDisplayName())
			if strings.Contains(name, "code") || strings.Contains(name, "coder") {
				return model, nil
			}
		}
	}
	return models[0], nil
}

func (r *Registry) UpdateModel(id string, opts UpdateOptions) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.models[id]
	if !ok {
		return fmt.Errorf("model %q not found", id)
	}
	if opts.CustomName != "" {
		entry.CustomName = opts.CustomName
	}
	if opts.Notes != "" {
		entry.Notes = opts.Notes
	}
	if opts.Tags != nil {
		entry.Tags = cloneStrings(opts.Tags)
	}
	return nil
}

func (r *Registry) MarkUsed(id string) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.models[id]
	if !ok {
		return fmt.Errorf("model %q not found", id)
	}
	entry.LastUsed = time.Now().UTC()
	if entry.UseCount < 0 {
		entry.UseCount = 0
	}
	if entry.UseCount < int64(^uint64(0)>>1) {
		entry.UseCount++
	}
	return nil
}

func (r *Registry) Validate() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	paths := make(map[string]string, len(r.models))
	for id, entry := range r.models {
		if entry != nil {
			paths[id] = entry.Path
		}
	}
	r.mu.RUnlock()
	type result struct {
		id      string
		entry   *ModelEntry
		missing bool
	}
	results := make([]result, 0, len(paths))
	for id, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			results = append(results, result{id: id, entry: &ModelEntry{Status: statusForStatError(err), Error: statErrorText(err)}, missing: os.IsNotExist(err)})
			continue
		}
		if !info.Mode().IsRegular() {
			results = append(results, result{id: id, entry: &ModelEntry{Status: StatusMissing, Error: "model path is not a regular file"}})
			continue
		}
		metadata, parseErr := gguf.Parse(path)
		if parseErr != nil {
			results = append(results, result{id: id, entry: &ModelEntry{Status: StatusError, Error: parseErr.Error()}})
			continue
		}
		results = append(results, result{id: id, entry: &ModelEntry{Status: StatusValidated, Metadata: metadata}})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	missing := make([]string, 0)
	for _, result := range results {
		entry, ok := r.models[result.id]
		if !ok || entry == nil {
			continue
		}
		entry.Status = result.entry.Status
		entry.Error = result.entry.Error
		if result.entry.Metadata != nil {
			entry.Metadata = cloneMetadata(result.entry.Metadata)
		} else if result.missing {
			entry.Metadata = nil
		}
		if result.missing {
			missing = append(missing, result.id)
		}
	}
	sort.Strings(missing)
	return missing
}

func (r *Registry) GetStats() Stats {
	if r == nil {
		return Stats{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	stats := Stats{TotalModels: len(r.models)}
	var totalSize int64
	for _, entry := range r.models {
		if entry == nil {
			continue
		}
		if entry.Metadata != nil && entry.Metadata.FileSize > 0 {
			if totalSize > int64(^uint64(0)>>1)-entry.Metadata.FileSize {
				totalSize = int64(^uint64(0) >> 1)
			} else {
				totalSize += entry.Metadata.FileSize
			}
		}
		switch entry.Status {
		case StatusValidated, StatusLoaded:
			stats.ValidModels++
		case StatusMissing:
			stats.MissingModels++
		}
	}
	stats.TotalSizeGB = float64(totalSize) / (1024 * 1024 * 1024)
	return stats
}

type Stats struct {
	TotalModels   int     `json:"total_models"`
	ValidModels   int     `json:"valid_models"`
	MissingModels int     `json:"missing_models"`
	TotalSizeGB   float64 `json:"total_size_gb"`
}

type UpdateOptions struct {
	CustomName string
	Notes      string
	Tags       []string
}

func refreshEntry(entry *ModelEntry) {
	if entry == nil {
		return
	}
	info, err := os.Stat(entry.Path)
	if err != nil {
		entry.Status = statusForStatError(err)
		entry.Error = statErrorText(err)
		entry.Metadata = nil
		return
	}
	if !info.Mode().IsRegular() {
		entry.Status = StatusMissing
		entry.Error = "model path is not a regular file"
		entry.Metadata = nil
		return
	}
	metadata, err := gguf.Parse(entry.Path)
	if err != nil {
		entry.Status = StatusError
		entry.Error = err.Error()
		entry.Metadata = nil
		return
	}
	entry.Metadata = metadata
	entry.Status = StatusValidated
	entry.Error = ""
}

func statusForStatError(err error) ModelStatus {
	if os.IsNotExist(err) {
		return StatusMissing
	}
	return StatusError
}

func statErrorText(err error) string {
	if os.IsNotExist(err) {
		return "file not found"
	}
	return err.Error()
}

func matchesModel(entry *ModelEntry, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	name := strings.ToLower(entry.GetDisplayName())
	hasFilter := false
	for _, value := range filter {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		hasFilter = true
		if strings.Contains(name, strings.ToLower(value)) {
			return true
		}
		for _, tag := range entry.Tags {
			if strings.EqualFold(tag, value) {
				return true
			}
		}
	}
	return !hasFilter
}

func cloneEntry(entry *ModelEntry) *ModelEntry {
	if entry == nil {
		return nil
	}
	copy := *entry
	copy.Tags = cloneStrings(entry.Tags)
	copy.Metadata = cloneMetadata(entry.Metadata)
	return &copy
}

func cloneMetadata(metadata *gguf.ModelMetadata) *gguf.ModelMetadata {
	if metadata == nil {
		return nil
	}
	copy := *metadata
	copy.RawMetadata = make(map[string]gguf.MetadataValue, len(metadata.RawMetadata))
	for key, value := range metadata.RawMetadata {
		copy.RawMetadata[key] = cloneMetadataValue(value)
	}
	return &copy
}

func cloneMetadataValue(value gguf.MetadataValue) gguf.MetadataValue {
	if value.Array == nil {
		return value
	}
	value.Array = make([]gguf.MetadataValue, len(value.Array))
	for i := range value.Array {
		value.Array[i] = cloneMetadataValue(value.Array[i])
	}
	return value
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func readRegistryFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("registry path is not a regular file: %s", path)
	}
	if info.Size() > maxRegistryBytes {
		return nil, fmt.Errorf("registry file is too large: %d bytes", info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRegistryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRegistryBytes {
		return nil, fmt.Errorf("registry file is too large")
	}
	return data, nil
}

func generateID(path string) string {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".gguf")
	var clean strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			clean.WriteRune(r)
		} else {
			clean.WriteByte('_')
		}
	}
	base = strings.Trim(clean.String(), "_")
	if base == "" {
		base = "model"
	}
	if len(base) > 80 {
		base = base[:80]
	}
	hash := sha256.Sum256([]byte(path))
	return base + "_" + hex.EncodeToString(hash[:])[:12]
}
