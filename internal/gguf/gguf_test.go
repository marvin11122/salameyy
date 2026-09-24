package gguf

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type portTestMetadataValue struct {
	valueType ValueType
	data      []byte
}

func TestParseAndReadTensorInfos(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "model.gguf")
	data := portTestBuildGGUF(t, []portTestMetadataValue{
		{valueType: ValueTypeString, data: portTestEncodeString("Test Model")},
		{valueType: ValueTypeString, data: portTestEncodeString("llama")},
		{valueType: ValueTypeUint64, data: portTestEncodeUint64(32768)},
		{valueType: ValueTypeUint64, data: portTestEncodeUint64(4096)},
		{valueType: ValueTypeUint64, data: portTestEncodeUint64(12)},
		{valueType: ValueTypeUint32, data: portTestEncodeUint32(1)},
	})
	require.NoError(t, os.WriteFile(path, data, 0o600))

	metadata, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, uint32(VersionV3), metadata.Version)
	require.Equal(t, uint64(1), metadata.TensorCount)
	require.Equal(t, "Test Model", metadata.Name)
	require.Equal(t, "llama", metadata.Architecture)
	require.Equal(t, uint64(32768), metadata.ContextLength)
	require.Equal(t, uint64(4096), metadata.EmbeddingLength)
	require.Equal(t, uint64(12), metadata.LayerCount)
	require.Equal(t, uint32(1), metadata.BOS)
	require.True(t, metadata.IsValid())
	require.Equal(t, "98304", metadata.ParameterCountString())

	tensors, err := ReadTensorInfos(path)
	require.NoError(t, err)
	require.Len(t, tensors, 1)
	require.Equal(t, "weight", tensors[0].Name)
	require.Equal(t, []uint64{4096}, tensors[0].Shape)
}

func TestParseRejectsUnboundedCountsAndStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{name: "metadata count", data: portTestHeader(1, ^uint64(0))},
		{name: "string length", data: append(portTestHeader(1, 1), portTestEncodeUint64(maxStringBytes+1)...)},
		{name: "array length", data: func() []byte {
			data := append(portTestHeader(1, 1), portTestEncodeString("array")...)
			data = append(data, portTestEncodeUint32(uint32(ValueTypeArray))...)
			data = append(data, portTestEncodeUint32(uint32(ValueTypeUint32))...)
			return append(data, portTestEncodeUint64(maxArrayElements+1)...)
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.gguf")
			require.NoError(t, os.WriteFile(path, test.data, 0o600))
			_, err := Parse(path)
			require.Error(t, err)
		})
	}
}

func TestParseRejectsInvalidBoolean(t *testing.T) {
	t.Parallel()

	data := append(portTestHeader(1, 1), portTestEncodeString("flag")...)
	data = append(data, portTestEncodeUint32(uint32(ValueTypeBool))...)
	data = append(data, 2)
	path := filepath.Join(t.TempDir(), "bool.gguf")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	_, err := Parse(path)
	require.Error(t, err)
}

func portTestBuildGGUF(t *testing.T, values []portTestMetadataValue) []byte {
	t.Helper()
	keys := []string{"general.name", "general.architecture", "llama.context_length", "llama.embedding_length", "llama.block_count", "tokenizer.ggml.bos_token_id"}
	require.Len(t, values, len(keys))
	data := portTestHeader(1, uint64(len(values)))
	for i, value := range values {
		data = append(data, portTestEncodeString(keys[i])...)
		data = append(data, portTestEncodeUint32(uint32(value.valueType))...)
		data = append(data, value.data...)
	}
	data = append(data, portTestEncodeString("weight")...)
	data = append(data, portTestEncodeUint32(1)...)
	data = append(data, portTestEncodeUint64(4096)...)
	data = append(data, portTestEncodeUint32(0)...)
	data = append(data, portTestEncodeUint64(0)...)
	return data
}

func portTestHeader(tensorCount, metadataCount uint64) []byte {
	data := []byte("GGUF")
	data = append(data, portTestEncodeUint32(VersionV3)...)
	data = append(data, portTestEncodeUint64(tensorCount)...)
	return append(data, portTestEncodeUint64(metadataCount)...)
}

func portTestEncodeString(value string) []byte {
	data := portTestEncodeUint64(uint64(len(value)))
	return append(data, []byte(value)...)
}

func portTestEncodeUint32(value uint32) []byte {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	return data[:]
}

func portTestEncodeUint64(value uint64) []byte {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	return data[:]
}
