package registry

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryPersistsBySafeReparse(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	modelPath := filepath.Join(t.TempDir(), "local.gguf")
	require.NoError(t, os.WriteFile(modelPath, portRegistryMinimalGGUF(t), 0o600))
	registry := NewRegistry(dataDir)
	entry, err := registry.AddModel(modelPath)
	require.NoError(t, err)
	require.NotNil(t, entry.Metadata)
	entry.Name = "caller mutation"
	require.NoError(t, registry.Save())

	loadedRegistry := NewRegistry(dataDir)
	require.NoError(t, loadedRegistry.Load())
	loaded, ok := loadedRegistry.GetModel(entry.ID)
	require.True(t, ok)
	require.NotNil(t, loaded.Metadata)
	require.Equal(t, entry.Path, loaded.Path)
	require.Equal(t, StatusValidated, loaded.Status)
	require.Equal(t, "local.gguf", loaded.Name)

	require.NoError(t, os.Remove(modelPath))
	require.NoError(t, loadedRegistry.Load())
	loaded, ok = loadedRegistry.GetModel(entry.ID)
	require.True(t, ok)
	require.Equal(t, StatusMissing, loaded.Status)
	require.Nil(t, loaded.Metadata)
}

func TestRegistryLoadRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "model_registry.json"), []byte(`{"not":"an-array"}`), 0o600))
	registry := NewRegistry(dataDir)
	require.Error(t, registry.Load())
}

func TestRegistryConcurrentAccessIsSafe(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	modelPath := filepath.Join(t.TempDir(), "local.gguf")
	require.NoError(t, os.WriteFile(modelPath, portRegistryMinimalGGUF(t), 0o600))
	registry := NewRegistry(dataDir)
	_, err := registry.AddModel(modelPath)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errors := make(chan error, 32)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				_ = registry.ListModels()
				_, _ = registry.GetModelByPath(modelPath)
				if err := registry.Save(); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestGenerateIDHandlesShortPaths(t *testing.T) {
	t.Parallel()

	id := generateID("x")
	require.NotEmpty(t, id)
	require.Contains(t, id, "_")
}

func portRegistryMinimalGGUF(t *testing.T) []byte {
	t.Helper()
	data := []byte("GGUF")
	data = append(data, portRegistryUint32Bytes(3)...)
	data = append(data, portRegistryUint64Bytes(1)...)
	data = append(data, portRegistryUint64Bytes(0)...)
	data = append(data, portRegistryStringBytes("weight")...)
	data = append(data, portRegistryUint32Bytes(1)...)
	data = append(data, portRegistryUint64Bytes(2)...)
	data = append(data, portRegistryUint32Bytes(0)...)
	return append(data, portRegistryUint64Bytes(0)...)
}

func portRegistryUint32Bytes(value uint32) []byte {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	return data[:]
}

func portRegistryUint64Bytes(value uint64) []byte {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	return data[:]
}

func portRegistryStringBytes(value string) []byte {
	return append(portRegistryUint64Bytes(uint64(len(value))), []byte(value)...)
}
