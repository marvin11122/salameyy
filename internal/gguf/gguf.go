package gguf

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var ggufMagic = []byte("GGUF")

const (
	VersionV1 = 1
	VersionV2 = 2
	VersionV3 = 3

	maxStringBytes       = 1 << 20
	maxArrayElements     = 1 << 20
	maxMetadataEntries   = 1 << 20
	maxTensorEntries     = 1 << 20
	maxTensorDimensions  = 1 << 12
	maxMetadataValues    = 1 << 24
	maxNestedValueDepth  = 16
	headerSize           = 24
	minimumMetadataEntry = 12
	minimumTensorEntry   = 24
)

type ValueType uint32

const (
	ValueTypeUint8   ValueType = 0
	ValueTypeInt8    ValueType = 1
	ValueTypeUint16  ValueType = 2
	ValueTypeInt16   ValueType = 3
	ValueTypeUint32  ValueType = 4
	ValueTypeInt32   ValueType = 5
	ValueTypeFloat32 ValueType = 6
	ValueTypeBool    ValueType = 7
	ValueTypeString  ValueType = 8
	ValueTypeArray   ValueType = 9
	ValueTypeUint64  ValueType = 10
	ValueTypeInt64   ValueType = 11
	ValueTypeFloat64 ValueType = 12
)

type MetadataValue struct {
	Type    ValueType
	Uint8   uint8
	Int8    int8
	Uint16  uint16
	Int16   int16
	Uint32  uint32
	Int32   int32
	Float32 float32
	Bool    bool
	String  string
	Array   []MetadataValue
	Uint64  uint64
	Int64   int64
	Float64 float64
}

type TensorInfo struct {
	Name   string
	Shape  []uint64
	Type   uint32
	Offset uint64
}

type Header struct {
	Version       uint32
	TensorCount   uint64
	MetadataCount uint64
}

type ModelMetadata struct {
	FilePath string
	FileSize int64

	Version     uint32
	TensorCount uint64

	Name         string
	Architecture string
	Description  string
	Author       string
	URL          string
	License      string
	TrimLayers   uint32

	ContextLength        uint64
	EmbeddingLength      uint64
	AttentionHeadCount   uint64
	AttentionHeadCountKV uint64
	LayerCount           uint64
	FeedForwardLength    uint64
	RopeDimensionalCount uint64

	QuantizationVersion uint32
	FileType            string

	BOS uint32
	EOS uint32
	UNK uint32
	Sep uint32
	PAD uint32

	AddBOS bool
	AddEOS bool

	SystemPromptTemplate string

	RawMetadata map[string]MetadataValue
}

type Tensor struct {
	Name   string
	Shape  []uint64
	Type   uint32
	Offset uint64
	Size   uint64
}

type fileParser struct {
	r         io.Reader
	remaining int64
	offset    int64
	values    uint64
}

func newFileParser(r io.Reader, size int64) (*fileParser, error) {
	if size < headerSize {
		return nil, fmt.Errorf("file is too small for a GGUF header: %d bytes", size)
	}
	return &fileParser{r: r, remaining: size}, nil
}

func (p *fileParser) read(buf []byte) error {
	if int64(len(buf)) > p.remaining {
		return io.ErrUnexpectedEOF
	}
	n, err := io.ReadFull(p.r, buf)
	p.remaining -= int64(n)
	p.offset += int64(n)
	if err != nil {
		return err
	}
	return nil
}

func (p *fileParser) readUint32() (uint32, error) {
	var buf [4]byte
	if err := p.read(buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (p *fileParser) readUint64() (uint64, error) {
	var buf [8]byte
	if err := p.read(buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

func (p *fileParser) readString() (string, error) {
	length, err := p.readUint64()
	if err != nil {
		return "", fmt.Errorf("read string length: %w", err)
	}
	if length > maxStringBytes {
		return "", fmt.Errorf("string too long: %d", length)
	}
	if length > uint64(p.remaining) {
		return "", io.ErrUnexpectedEOF
	}
	if uint64(int(length)) != length {
		return "", fmt.Errorf("string length overflows int: %d", length)
	}
	buf := make([]byte, int(length))
	if err := p.read(buf); err != nil {
		return "", fmt.Errorf("read string data: %w", err)
	}
	return string(buf), nil
}

func (p *fileParser) header() (*Header, error) {
	var buf [headerSize]byte
	if err := p.read(buf[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if string(buf[:4]) != string(ggufMagic) {
		return nil, fmt.Errorf("invalid GGUF magic: %q", buf[:4])
	}
	version := binary.LittleEndian.Uint32(buf[4:8])
	if version < VersionV1 || version > VersionV3 {
		return nil, fmt.Errorf("unsupported GGUF version: %d", version)
	}
	tensorCount := binary.LittleEndian.Uint64(buf[8:16])
	metadataCount := binary.LittleEndian.Uint64(buf[16:24])
	if tensorCount > maxTensorEntries {
		return nil, fmt.Errorf("tensor count too large: %d", tensorCount)
	}
	if metadataCount > maxMetadataEntries {
		return nil, fmt.Errorf("metadata count too large: %d", metadataCount)
	}
	return &Header{Version: version, TensorCount: tensorCount, MetadataCount: metadataCount}, nil
}

func (p *fileParser) metadata(count uint64, out map[string]MetadataValue) error {
	if count > uint64(p.remaining/minimumMetadataEntry) {
		return fmt.Errorf("metadata count %d cannot fit in %d bytes", count, p.remaining)
	}
	for i := uint64(0); i < count; i++ {
		key, err := p.readString()
		if err != nil {
			return fmt.Errorf("metadata key %d: %w", i, err)
		}
		if key == "" {
			return fmt.Errorf("metadata key %d is empty", i)
		}
		if _, exists := out[key]; exists {
			return fmt.Errorf("duplicate metadata key %q", key)
		}
		valueType, err := p.readUint32()
		if err != nil {
			return fmt.Errorf("metadata type for %q: %w", key, err)
		}
		value, err := p.value(ValueType(valueType), 0)
		if err != nil {
			return fmt.Errorf("metadata value for %q: %w", key, err)
		}
		out[key] = value
	}
	return nil
}

func (p *fileParser) value(vt ValueType, depth int) (MetadataValue, error) {
	if depth > maxNestedValueDepth {
		return MetadataValue{}, fmt.Errorf("metadata nesting exceeds %d", maxNestedValueDepth)
	}
	if p.values >= maxMetadataValues {
		return MetadataValue{}, fmt.Errorf("metadata value count exceeds %d", maxMetadataValues)
	}
	p.values++
	value := MetadataValue{Type: vt}
	var err error
	switch vt {
	case ValueTypeUint8:
		var b [1]byte
		err = p.read(b[:])
		value.Uint8 = b[0]
	case ValueTypeInt8:
		var b [1]byte
		err = p.read(b[:])
		value.Int8 = int8(b[0])
	case ValueTypeUint16:
		var b [2]byte
		err = p.read(b[:])
		value.Uint16 = binary.LittleEndian.Uint16(b[:])
	case ValueTypeInt16:
		var b [2]byte
		err = p.read(b[:])
		value.Int16 = int16(binary.LittleEndian.Uint16(b[:]))
	case ValueTypeUint32:
		value.Uint32, err = p.readUint32()
	case ValueTypeInt32:
		var v uint32
		v, err = p.readUint32()
		value.Int32 = int32(v)
	case ValueTypeFloat32:
		var v uint32
		v, err = p.readUint32()
		value.Float32 = math.Float32frombits(v)
	case ValueTypeBool:
		var b [1]byte
		err = p.read(b[:])
		if err == nil && b[0] > 1 {
			err = fmt.Errorf("invalid boolean value %d", b[0])
		}
		value.Bool = b[0] != 0
	case ValueTypeString:
		value.String, err = p.readString()
	case ValueTypeUint64:
		value.Uint64, err = p.readUint64()
	case ValueTypeInt64:
		var v uint64
		v, err = p.readUint64()
		value.Int64 = int64(v)
	case ValueTypeFloat64:
		var v uint64
		v, err = p.readUint64()
		value.Float64 = math.Float64frombits(v)
	case ValueTypeArray:
		var elementTypeValue uint32
		elementTypeValue, err = p.readUint32()
		if err != nil {
			break
		}
		elementType := ValueType(elementTypeValue)
		if !validValueType(elementType) {
			return value, fmt.Errorf("unknown array element type: %d", elementType)
		}
		var lengthValue uint64
		lengthValue, err = p.readUint64()
		if err != nil {
			break
		}
		if lengthValue > maxArrayElements {
			return value, fmt.Errorf("array too long: %d", lengthValue)
		}
		minimum := minimumValueSize(elementType)
		if minimum == 0 || lengthValue > uint64(p.remaining)/minimum {
			return value, io.ErrUnexpectedEOF
		}
		if uint64(int(lengthValue)) != lengthValue {
			return value, fmt.Errorf("array length overflows int: %d", lengthValue)
		}
		value.Array = make([]MetadataValue, int(lengthValue))
		for i := range value.Array {
			value.Array[i], err = p.value(elementType, depth+1)
			if err != nil {
				return value, err
			}
		}
	default:
		return value, fmt.Errorf("unknown value type: %d", vt)
	}
	if err != nil {
		return value, err
	}
	return value, nil
}

func validValueType(vt ValueType) bool {
	return vt <= ValueTypeFloat64
}

func minimumValueSize(vt ValueType) uint64 {
	switch vt {
	case ValueTypeUint8, ValueTypeInt8, ValueTypeBool:
		return 1
	case ValueTypeUint16, ValueTypeInt16:
		return 2
	case ValueTypeUint32, ValueTypeInt32, ValueTypeFloat32:
		return 4
	case ValueTypeUint64, ValueTypeInt64, ValueTypeFloat64:
		return 8
	case ValueTypeString, ValueTypeArray:
		return 8
	default:
		return 0
	}
}

func Parse(path string) (*ModelMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GGUF file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat GGUF file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("GGUF path is not a regular file: %s", path)
	}
	if info.Size() < headerSize {
		return nil, fmt.Errorf("GGUF file is too small: %d bytes", info.Size())
	}
	p, err := newFileParser(f, info.Size())
	if err != nil {
		return nil, err
	}
	header, err := p.header()
	if err != nil {
		return nil, fmt.Errorf("read GGUF header: %w", err)
	}
	if header.MetadataCount > uint64(p.remaining/minimumMetadataEntry) {
		return nil, fmt.Errorf("metadata count %d cannot fit in %d bytes", header.MetadataCount, p.remaining)
	}
	meta := &ModelMetadata{
		FilePath:    path,
		FileSize:    info.Size(),
		Version:     header.Version,
		TensorCount: header.TensorCount,
		RawMetadata: make(map[string]MetadataValue, int(header.MetadataCount)),
	}
	if err := p.metadata(header.MetadataCount, meta.RawMetadata); err != nil {
		return nil, fmt.Errorf("read GGUF metadata: %w", err)
	}
	parseCommonMetadata(meta)
	return meta, nil
}

func readHeader(r io.Reader) (*Header, error) {
	var buf [headerSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return nil, err
	}
	if string(buf[:4]) != string(ggufMagic) {
		return nil, fmt.Errorf("invalid GGUF magic: %q", buf[:4])
	}
	version := binary.LittleEndian.Uint32(buf[4:8])
	if version < VersionV1 || version > VersionV3 {
		return nil, fmt.Errorf("unsupported GGUF version: %d", version)
	}
	tensorCount := binary.LittleEndian.Uint64(buf[8:16])
	metadataCount := binary.LittleEndian.Uint64(buf[16:24])
	if tensorCount > maxTensorEntries {
		return nil, fmt.Errorf("tensor count too large: %d", tensorCount)
	}
	if metadataCount > maxMetadataEntries {
		return nil, fmt.Errorf("metadata count too large: %d", metadataCount)
	}
	return &Header{Version: version, TensorCount: tensorCount, MetadataCount: metadataCount}, nil
}

func readMetadata(r io.Reader, count uint64, meta *ModelMetadata) error {
	if meta == nil {
		return fmt.Errorf("metadata destination is nil")
	}
	if meta.RawMetadata == nil {
		meta.RawMetadata = make(map[string]MetadataValue)
	}
	if count > maxMetadataEntries {
		return fmt.Errorf("metadata count too large: %d", count)
	}
	p := &fileParser{r: r, remaining: math.MaxInt64}
	if count > uint64(p.remaining/minimumMetadataEntry) {
		return fmt.Errorf("metadata count %d is too large", count)
	}
	for i := uint64(0); i < count; i++ {
		key, err := p.readString()
		if err != nil {
			return fmt.Errorf("metadata key %d: %w", i, err)
		}
		if _, exists := meta.RawMetadata[key]; exists {
			return fmt.Errorf("duplicate metadata key %q", key)
		}
		valueType, err := p.readUint32()
		if err != nil {
			return fmt.Errorf("metadata type for %q: %w", key, err)
		}
		value, err := p.value(ValueType(valueType), 0)
		if err != nil {
			return fmt.Errorf("metadata value for %q: %w", key, err)
		}
		meta.RawMetadata[key] = value
	}
	return nil
}

func readString(r io.Reader) (string, error) {
	var length uint64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length > maxStringBytes || uint64(int(length)) != length {
		return "", fmt.Errorf("string too long: %d", length)
	}
	buf := make([]byte, int(length))
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readValue(r io.Reader, valueType ValueType) (MetadataValue, error) {
	p := &fileParser{r: r, remaining: math.MaxInt64}
	return p.value(valueType, 0)
}

func parseCommonMetadata(meta *ModelMetadata) {
	if meta == nil {
		return
	}
	if meta.RawMetadata == nil {
		meta.RawMetadata = make(map[string]MetadataValue)
	}
	stringMetadata := func(key string) string {
		value, ok := meta.RawMetadata[key]
		if !ok || value.Type != ValueTypeString {
			return ""
		}
		return value.String
	}
	uintMetadata := func(key string) uint64 {
		value, ok := meta.RawMetadata[key]
		if !ok {
			return 0
		}
		return unsignedValue(value)
	}
	uint32Metadata := func(key string) uint32 {
		value := uintMetadata(key)
		if value > math.MaxUint32 {
			return 0
		}
		return uint32(value)
	}

	meta.Name = stringMetadata("general.name")
	meta.Architecture = stringMetadata("general.architecture")
	meta.Description = stringMetadata("general.description")
	meta.Author = stringMetadata("general.author")
	meta.URL = stringMetadata("general.url")
	meta.License = stringMetadata("general.license")
	meta.TrimLayers = uint32Metadata("general.trim_layers")

	arch := meta.Architecture
	if arch == "" {
		arch = "llama"
	}
	meta.ContextLength = uintMetadata(arch + ".context_length")
	meta.EmbeddingLength = uintMetadata(arch + ".embedding_length")
	meta.AttentionHeadCount = uintMetadata(arch + ".attention.head_count")
	meta.AttentionHeadCountKV = uintMetadata(arch + ".attention.head_count_kv")
	meta.LayerCount = uintMetadata(arch + ".block_count")
	meta.FeedForwardLength = uintMetadata(arch + ".feed_forward_length")
	meta.RopeDimensionalCount = uintMetadata(arch + ".rope.dimension_count")
	meta.QuantizationVersion = uint32Metadata("general.quantization_version")
	if value, ok := meta.RawMetadata["general.file_type"]; ok {
		if value.Type == ValueTypeString {
			meta.FileType = value.String
		} else {
			meta.FileType = strconv.FormatUint(unsignedValue(value), 10)
		}
	}

	if _, ok := meta.RawMetadata["tokenizer.ggml.bos_token_id"]; ok {
		meta.BOS = uint32Metadata("tokenizer.ggml.bos_token_id")
		meta.AddBOS = true
	}
	if _, ok := meta.RawMetadata["tokenizer.ggml.eos_token_id"]; ok {
		meta.EOS = uint32Metadata("tokenizer.ggml.eos_token_id")
		meta.AddEOS = true
	}
	meta.UNK = uint32Metadata("tokenizer.ggml.unknown_token_id")
	meta.Sep = uint32Metadata("tokenizer.ggml.separator_token_id")
	meta.PAD = uint32Metadata("tokenizer.ggml.padding_token_id")
	meta.SystemPromptTemplate = stringMetadata("tokenizer.chat_template")
}

func unsignedValue(value MetadataValue) uint64 {
	switch value.Type {
	case ValueTypeUint8:
		return uint64(value.Uint8)
	case ValueTypeInt8:
		if value.Int8 < 0 {
			return 0
		}
		return uint64(value.Int8)
	case ValueTypeUint16:
		return uint64(value.Uint16)
	case ValueTypeInt16:
		if value.Int16 < 0 {
			return 0
		}
		return uint64(value.Int16)
	case ValueTypeUint32:
		return uint64(value.Uint32)
	case ValueTypeInt32:
		if value.Int32 < 0 {
			return 0
		}
		return uint64(value.Int32)
	case ValueTypeUint64:
		return value.Uint64
	case ValueTypeInt64:
		if value.Int64 < 0 {
			return 0
		}
		return uint64(value.Int64)
	default:
		return 0
	}
}

func (m *ModelMetadata) IsValid() bool {
	return m != nil && m.Version >= VersionV1 && m.Version <= VersionV3 && m.TensorCount > 0
}

func (m *ModelMetadata) ParameterCount() uint64 {
	if m == nil || m.EmbeddingLength == 0 || m.LayerCount == 0 {
		return 0
	}
	if m.EmbeddingLength > math.MaxUint64/2 {
		return math.MaxUint64
	}
	product := m.EmbeddingLength * 2
	if product > math.MaxUint64/m.LayerCount {
		return math.MaxUint64
	}
	return product * m.LayerCount
}

func (m *ModelMetadata) ParameterCountString() string {
	if m == nil {
		return "unknown"
	}
	params := m.ParameterCount()
	if params == 0 {
		return "unknown"
	}
	if params >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(params)/1_000_000_000)
	}
	if params >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(params)/1_000_000)
	}
	return strconv.FormatUint(params, 10)
}

func (m *ModelMetadata) EstimatedMemoryGB() float64 {
	if m == nil || m.FileSize <= 0 {
		return 0
	}
	return float64(m.FileSize) / (1024 * 1024 * 1024) * 1.1
}

func (m *ModelMetadata) SupportsContext(length uint64) bool {
	if m == nil {
		return false
	}
	return m.ContextLength == 0 || m.ContextLength >= length
}

func (m *ModelMetadata) QuantType() string {
	if m == nil {
		return "unknown"
	}
	name := strings.ToLower(filepath.Base(m.FilePath))
	types := []string{
		"q2_k", "q3_k_s", "q3_k_m", "q3_k_l", "q4_0", "q4_1", "q4_k_s", "q4_k_m",
		"q5_0", "q5_1", "q5_k_s", "q5_k_m", "q6_k", "q8_0", "f16", "f32", "bf16",
		"iq1_s", "iq1_m", "iq2_xxs", "iq2_xs", "iq2_s", "iq3_xxs", "iq3_s", "iq4_nl", "iq4_xs",
	}
	for _, quantType := range types {
		if strings.Contains(name, quantType) {
			return strings.ToUpper(quantType)
		}
	}
	if m.FileType != "" {
		return m.FileType
	}
	if value, ok := m.RawMetadata["general.file_type"]; ok {
		if value.String != "" {
			return value.String
		}
		return strconv.FormatUint(unsignedValue(value), 10)
	}
	return "unknown"
}

func (m *ModelMetadata) String() string {
	if m == nil {
		return ""
	}
	name := m.Name
	if name == "" {
		name = filepath.Base(m.FilePath)
	}
	return fmt.Sprintf("Model: %s\n  Architecture: %s\n  Parameters: %s\n  Context: %d\n  Layers: %d\n  Embedding: %d\n  Heads: %d (KV: %d)\n  Quantization: %s\n  File size: %.2f GB\n  Est. memory: %.2f GB", name, m.Architecture, m.ParameterCountString(), m.ContextLength, m.LayerCount, m.EmbeddingLength, m.AttentionHeadCount, m.AttentionHeadCountKV, m.QuantType(), float64(m.FileSize)/(1024*1024*1024), m.EstimatedMemoryGB())
}

func ReadTensorInfos(path string) ([]TensorInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("GGUF path is not a regular file: %s", path)
	}
	p, err := newFileParser(f, info.Size())
	if err != nil {
		return nil, err
	}
	header, err := p.header()
	if err != nil {
		return nil, err
	}
	if header.MetadataCount > uint64(p.remaining/minimumMetadataEntry) {
		return nil, fmt.Errorf("metadata count %d cannot fit in %d bytes", header.MetadataCount, p.remaining)
	}
	for i := uint64(0); i < header.MetadataCount; i++ {
		if _, err := p.readString(); err != nil {
			return nil, fmt.Errorf("metadata key %d: %w", i, err)
		}
		valueType, err := p.readUint32()
		if err != nil {
			return nil, fmt.Errorf("metadata type %d: %w", i, err)
		}
		if err := p.skipValue(ValueType(valueType), 0); err != nil {
			return nil, fmt.Errorf("metadata value %d: %w", i, err)
		}
	}
	if header.TensorCount > uint64(p.remaining/minimumTensorEntry) {
		return nil, fmt.Errorf("tensor count %d cannot fit in %d bytes", header.TensorCount, p.remaining)
	}
	if uint64(int(header.TensorCount)) != header.TensorCount {
		return nil, fmt.Errorf("tensor count overflows int: %d", header.TensorCount)
	}
	tensors := make([]TensorInfo, 0, int(header.TensorCount))
	maxOffset := uint64(0)
	for i := uint64(0); i < header.TensorCount; i++ {
		name, err := p.readString()
		if err != nil {
			return nil, fmt.Errorf("tensor %d name: %w", i, err)
		}
		var dimensionsCount uint32
		dimensionsCount, err = p.readUint32()
		if err != nil {
			return nil, fmt.Errorf("tensor %d dimensions: %w", i, err)
		}
		if dimensionsCount > maxTensorDimensions || uint64(dimensionsCount) > uint64(p.remaining/8) {
			return nil, fmt.Errorf("tensor %d has too many dimensions: %d", i, dimensionsCount)
		}
		shape := make([]uint64, int(dimensionsCount))
		for j := range shape {
			shape[j], err = p.readUint64()
			if err != nil {
				return nil, fmt.Errorf("tensor %d dimension %d: %w", i, j, err)
			}
		}
		dtype, err := p.readUint32()
		if err != nil {
			return nil, fmt.Errorf("tensor %d type: %w", i, err)
		}
		offset, err := p.readUint64()
		if err != nil {
			return nil, fmt.Errorf("tensor %d offset: %w", i, err)
		}
		if offset > maxOffset {
			maxOffset = offset
		}
		tensors = append(tensors, TensorInfo{Name: name, Shape: shape, Type: dtype, Offset: offset})
	}
	if p.remaining < 0 {
		return nil, fmt.Errorf("parser position exceeded file size")
	}
	if maxOffset > uint64(p.remaining) {
		return nil, fmt.Errorf("tensor offset %d exceeds remaining file bytes %d", maxOffset, p.remaining)
	}
	return tensors, nil
}

func readHeaderAt(r io.ReaderAt, offset int64) (*Header, error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative header offset: %d", offset)
	}
	return readHeader(io.NewSectionReader(r, offset, headerSize))
}

func skipString(r io.Reader) error {
	var length uint64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return err
	}
	if length > maxStringBytes {
		return fmt.Errorf("string too long: %d", length)
	}
	if uint64(int(length)) != length {
		return fmt.Errorf("string length overflows int: %d", length)
	}
	_, err := io.CopyN(io.Discard, r, int64(length))
	return err
}

func skipValue(r io.Reader, vt ValueType) error {
	p := &fileParser{r: r, remaining: math.MaxInt64}
	return p.skipValue(vt, 0)
}

func (p *fileParser) skipValue(vt ValueType, depth int) error {
	if depth > maxNestedValueDepth {
		return fmt.Errorf("metadata nesting exceeds %d", maxNestedValueDepth)
	}
	if p.values >= maxMetadataValues {
		return fmt.Errorf("metadata value count exceeds %d", maxMetadataValues)
	}
	p.values++
	switch vt {
	case ValueTypeUint8, ValueTypeInt8, ValueTypeBool:
		return p.discard(1)
	case ValueTypeUint16, ValueTypeInt16:
		return p.discard(2)
	case ValueTypeUint32, ValueTypeInt32, ValueTypeFloat32:
		return p.discard(4)
	case ValueTypeUint64, ValueTypeInt64, ValueTypeFloat64:
		return p.discard(8)
	case ValueTypeString:
		return p.skipString()
	case ValueTypeArray:
		elementTypeValue, err := p.readUint32()
		if err != nil {
			return err
		}
		elementType := ValueType(elementTypeValue)
		if !validValueType(elementType) {
			return fmt.Errorf("unknown array element type: %d", elementType)
		}
		length, err := p.readUint64()
		if err != nil {
			return err
		}
		if length > maxArrayElements {
			return fmt.Errorf("array too long: %d", length)
		}
		minimum := minimumValueSize(elementType)
		if minimum == 0 || length > uint64(p.remaining)/minimum {
			return io.ErrUnexpectedEOF
		}
		for i := uint64(0); i < length; i++ {
			if err := p.skipValue(elementType, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown value type: %d", vt)
	}
}

func (p *fileParser) discard(length int64) error {
	if length < 0 || length > p.remaining {
		return io.ErrUnexpectedEOF
	}
	if _, err := io.CopyN(io.Discard, p.r, length); err != nil {
		return err
	}
	p.remaining -= length
	p.offset += length
	return nil
}

func (p *fileParser) skipString() error {
	length, err := p.readUint64()
	if err != nil {
		return err
	}
	if length > maxStringBytes {
		return fmt.Errorf("string too long: %d", length)
	}
	if length > uint64(p.remaining) {
		return io.ErrUnexpectedEOF
	}
	return p.discard(int64(length))
}

func readUint32(r io.Reader) (uint32, error) {
	var value uint32
	err := binary.Read(r, binary.LittleEndian, &value)
	return value, err
}

func DiscoverGGUF(dirs ...string) ([]string, error) {
	found := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(path string) {
		path = filepath.Clean(path)
		key := path
		if absolute, err := filepath.Abs(path); err == nil {
			key = absolute
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		found = append(found, path)
	}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err == nil && !info.IsDir() {
			if strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
				add(dir)
			}
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", dir, err)
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", dir, err)
		}
		err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry != nil && !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk directory %s: %w", dir, err)
		}
	}
	sort.Strings(found)
	return found, nil
}

func GetModelName(meta *ModelMetadata) string {
	if meta == nil {
		return ""
	}
	if strings.TrimSpace(meta.Name) != "" {
		return meta.Name
	}
	base := filepath.Base(meta.FilePath)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return strings.TrimSpace(base)
}
