package config

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

const (
	SamplingVerbosityTerse   = "terse"
	SamplingVerbosityBalance = "balanced"
	SamplingVerbosityMaximal = "maximal"
)

type SamplingOptions struct {
	Preset            string   `json:"preset,omitempty" jsonschema:"description=Named SALAMEY sampling preset,example=focused-82"`
	Verbosity         string   `json:"verbosity,omitempty" jsonschema:"description=Output verbosity preset,enum=terse,enum=balanced,enum=maximal"`
	Seed              *int64   `json:"seed,omitempty" jsonschema:"description=Deterministic sampling seed when supported"`
	MinP              *float64 `json:"min_p,omitempty" jsonschema:"description=Min-p sampling threshold when supported,minimum=0,maximum=1"`
	TopA              *float64 `json:"top_a,omitempty" jsonschema:"description=Top-a sampling threshold when supported,minimum=0,maximum=1"`
	RepeatPenalty     *float64 `json:"repeat_penalty,omitempty" jsonschema:"description=Repeat penalty when supported,minimum=1,maximum=2"`
	ParallelToolCalls *bool    `json:"parallel_tool_calls,omitempty" jsonschema:"description=Allow parallel tool calls when supported"`
}

type SamplingPreset struct {
	ID          string
	Title       string
	Description string
	MaxTokens   int64
	Temperature *float64
	TopP        *float64
	TopK        *int64
	Frequency   *float64
	Presence    *float64
	Reasoning   string
	Think       *bool
	Sampling    SamplingOptions
}

var samplingPresets = []SamplingPreset{
	{
		ID:          "focused-82",
		Title:       "Focused 82",
		Description: "Tight reasoning, low entropy, sharp edits.",
		MaxTokens:   8192,
		Temperature: ptrFloat64(0.18),
		TopP:        ptrFloat64(0.82),
		TopK:        ptrInt64(48),
		Frequency:   ptrFloat64(0.15),
		Reasoning:   "high",
		Sampling: SamplingOptions{
			Preset:    "focused-82",
			Verbosity: SamplingVerbosityBalance,
		},
	},
	{
		ID:          "math-borg",
		Title:       "Math Borg",
		Description: "Local deterministic pressure with low waste, fast tool convergence, and compact reasoning.",
		MaxTokens:   8192,
		Temperature: ptrFloat64(0.14),
		TopP:        ptrFloat64(0.78),
		TopK:        ptrInt64(40),
		Frequency:   ptrFloat64(0.10),
		Sampling: SamplingOptions{
			Preset:        "math-borg",
			Verbosity:     SamplingVerbosityBalance,
			RepeatPenalty: ptrFloat64(1.05),
		},
	},
	{
		ID:          "escalate-88",
		Title:       "Escalate 88",
		Description: "Maximal depth, broader branch search, heavier tool bias.",
		MaxTokens:   24576,
		Temperature: ptrFloat64(0.68),
		TopP:        ptrFloat64(0.96),
		TopK:        ptrInt64(72),
		Presence:    ptrFloat64(0.20),
		Reasoning:   "high",
		Think:       ptrBool(true),
		Sampling: SamplingOptions{
			Preset:            "escalate-88",
			Verbosity:         SamplingVerbosityMaximal,
			ParallelToolCalls: ptrBool(true),
		},
	},
	{
		ID:          "quiet-surgeon",
		Title:       "Quiet Surgeon",
		Description: "Minimal prose, deterministic patches, low variance.",
		MaxTokens:   4096,
		Temperature: ptrFloat64(0.08),
		TopP:        ptrFloat64(0.70),
		TopK:        ptrInt64(24),
		Frequency:   ptrFloat64(0.25),
		Sampling: SamplingOptions{
			Preset:        "quiet-surgeon",
			Verbosity:     SamplingVerbosityTerse,
			RepeatPenalty: ptrFloat64(1.05),
		},
	},
	{
		ID:          "swarm-matrix",
		Title:       "Swarm Matrix",
		Description: "Parallel scouting, fallback lanes, branch-heavy orchestration.",
		MaxTokens:   12288,
		Temperature: ptrFloat64(0.36),
		TopP:        ptrFloat64(0.90),
		TopK:        ptrInt64(80),
		Sampling: SamplingOptions{
			Preset:            "swarm-matrix",
			Verbosity:         SamplingVerbosityMaximal,
			ParallelToolCalls: ptrBool(true),
		},
	},
	{
		ID:          "doc-smith",
		Title:       "Doc Smith",
		Description: "Markdown-heavy synthesis with richer explanations and lower drift.",
		MaxTokens:   12288,
		Temperature: ptrFloat64(0.24),
		TopP:        ptrFloat64(0.86),
		TopK:        ptrInt64(60),
		Sampling: SamplingOptions{
			Preset:    "doc-smith",
			Verbosity: SamplingVerbosityMaximal,
		},
	},
	{
		ID:          "cinematic-local",
		Title:       "Cinematic Local",
		Description: "Local-LLM bias, expressive output, animated persona surfaces.",
		MaxTokens:   8192,
		Temperature: ptrFloat64(0.58),
		TopP:        ptrFloat64(0.92),
		TopK:        ptrInt64(120),
		Sampling: SamplingOptions{
			Preset:    "cinematic-local",
			Verbosity: SamplingVerbosityMaximal,
		},
	},
	{
		ID:          "provider-sniper",
		Title:       "Provider Sniper",
		Description: "Cost-aware precision with fallback disabled.",
		MaxTokens:   6144,
		Temperature: ptrFloat64(0.12),
		TopP:        ptrFloat64(0.80),
		TopK:        ptrInt64(32),
		Frequency:   ptrFloat64(0.10),
		Sampling: SamplingOptions{
			Preset:    "provider-sniper",
			Verbosity: SamplingVerbosityBalance,
		},
	},
	{
		ID:          "godzilla-lab",
		Title:       "Godzilla Lab",
		Description: "Maximum branch pressure, speculative patching, loud orchestration.",
		MaxTokens:   32768,
		Temperature: ptrFloat64(0.66),
		TopP:        ptrFloat64(0.95),
		TopK:        ptrInt64(160),
		Presence:    ptrFloat64(0.35),
		Reasoning:   "high",
		Think:       ptrBool(true),
		Sampling: SamplingOptions{
			Preset:            "godzilla-lab",
			Verbosity:         SamplingVerbosityMaximal,
			ParallelToolCalls: ptrBool(true),
		},
	},
}

var samplingProviderOptionKeys = [...]string{
	"verbosity",
	"seed",
	"min_p",
	"top_a",
	"repeat_penalty",
	"parallel_tool_calls",
}

func (m SelectedModel) EffectiveProviderOptions() map[string]any {
	out := maps.Clone(m.ProviderOptions)
	if out == nil {
		out = map[string]any{}
	}
	if m.Sampling == nil {
		return out
	}
	if m.Sampling.Verbosity != "" {
		out["verbosity"] = m.Sampling.Verbosity
	}
	if m.Sampling.Seed != nil {
		out["seed"] = *m.Sampling.Seed
	}
	if m.Sampling.MinP != nil {
		out["min_p"] = *m.Sampling.MinP
	}
	if m.Sampling.TopA != nil {
		out["top_a"] = *m.Sampling.TopA
	}
	if m.Sampling.RepeatPenalty != nil {
		out["repeat_penalty"] = *m.Sampling.RepeatPenalty
	}
	if m.Sampling.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *m.Sampling.ParallelToolCalls
	}
	return out
}

func SamplingPresets() []SamplingPreset {
	out := make([]SamplingPreset, 0, len(samplingPresets))
	for _, preset := range samplingPresets {
		out = append(out, cloneSamplingPreset(preset))
	}
	return out
}

func SamplingPresetIDs() []string {
	out := make([]string, 0, len(samplingPresets)+1)
	out = append(out, "manual")
	for _, preset := range samplingPresets {
		out = append(out, preset.ID)
	}
	return out
}

func SamplingPresetByID(id string) (SamplingPreset, bool) {
	id = strings.TrimSpace(id)
	for _, preset := range samplingPresets {
		if preset.ID == id {
			return cloneSamplingPreset(preset), true
		}
	}
	return SamplingPreset{}, false
}

func PreserveSelectedModelSettings(next, previous SelectedModel) SelectedModel {
	if next.Provider != previous.Provider || next.Model != previous.Model {
		return next
	}
	applySelectedModelSettings(&next, previous)
	return next
}

func CurrentSamplingPreset(model SelectedModel) string {
	if model.Sampling != nil {
		if id := strings.TrimSpace(model.Sampling.Preset); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(model.SamplingPreset); id != "" {
		return id
	}
	return "manual"
}

func ApplySamplingPreset(model SelectedModel, presetID string) (SelectedModel, string, error) {
	preset, ok := SamplingPresetByID(presetID)
	if !ok {
		return model, "", fmt.Errorf("unknown sampling preset %q", presetID)
	}

	updated := cloneSelectedModel(model)
	updated.SamplingPreset = preset.ID
	updated.MaxTokens = preset.MaxTokens
	updated.Temperature = cloneFloat64Ptr(preset.Temperature)
	updated.TopP = cloneFloat64Ptr(preset.TopP)
	updated.TopK = cloneInt64Ptr(preset.TopK)
	updated.FrequencyPenalty = cloneFloat64Ptr(preset.Frequency)
	updated.PresencePenalty = cloneFloat64Ptr(preset.Presence)
	updated.ReasoningEffort = preset.Reasoning
	updated.Think = preset.Think != nil && *preset.Think
	for _, key := range samplingProviderOptionKeys {
		delete(updated.ProviderOptions, key)
	}
	sampling := cloneSampling(&preset.Sampling)
	sampling.Preset = preset.ID
	updated.Sampling = sampling
	return updated, preset.Title + " preset loaded", nil
}

func ApplySamplingPresetWithMessage(model SelectedModel, presetID string) (SelectedModel, string, error) {
	return ApplySamplingPreset(model, presetID)
}

func ApplySamplingControl(model SelectedModel, controlID string) (SelectedModel, string, error) {
	updated := cloneSelectedModel(model)
	sampling := ensureSampling(updated.Sampling)
	sampling.Preset = "manual"
	updated.SamplingPreset = "manual"

	switch controlID {
	case "verbosity":
		sampling.Verbosity = nextString(sampling.Verbosity, SamplingVerbosityBalance, SamplingVerbosityTerse, SamplingVerbosityBalance, SamplingVerbosityMaximal)
		return withSampling(updated, sampling), "Verbosity " + sampling.Verbosity, nil
	case "temperature":
		updated.Temperature = nextFloat(updated.Temperature, []float64{0.08, 0.18, 0.32, 0.48, 0.66, 0.90})
		return withSampling(updated, sampling), fmt.Sprintf("Temperature %.2f", derefFloat64(updated.Temperature)), nil
	case "top_p":
		updated.TopP = nextFloat(updated.TopP, []float64{0.70, 0.82, 0.88, 0.92, 0.95, 1.00})
		return withSampling(updated, sampling), fmt.Sprintf("Top-p %.2f", derefFloat64(updated.TopP)), nil
	case "top_k":
		updated.TopK = nextInt64(updated.TopK, []int64{24, 40, 64, 96, 160, 0})
		return withSampling(updated, sampling), fmt.Sprintf("Top-k %d", derefInt64(updated.TopK)), nil
	case "max_tokens":
		updated.MaxTokens = nextScalar(updated.MaxTokens, 4096, []int64{4096, 8192, 12288, 16384, 32768, 65536})
		return withSampling(updated, sampling), fmt.Sprintf("Max tokens %d", updated.MaxTokens), nil
	case "repeat_penalty":
		sampling.RepeatPenalty = nextFloatOff(sampling.RepeatPenalty, []float64{1.00, 1.05, 1.10, 1.15, 1.20})
		clearProviderOptionIfNil(updated.ProviderOptions, "repeat_penalty")
		return withSampling(updated, sampling), fmt.Sprintf("Repeat penalty %.2f", derefFloat64(sampling.RepeatPenalty)), nil
	case "min_p":
		sampling.MinP = nextFloatOff(sampling.MinP, []float64{0.05, 0.10, 0.15, 0.20})
		clearProviderOptionIfNil(updated.ProviderOptions, "min_p")
		return withSampling(updated, sampling), "Min-p " + floatLabel(sampling.MinP), nil
	case "top_a":
		sampling.TopA = nextFloatOff(sampling.TopA, []float64{0.10, 0.20, 0.35, 0.50})
		clearProviderOptionIfNil(updated.ProviderOptions, "top_a")
		return withSampling(updated, sampling), "Top-a " + floatLabel(sampling.TopA), nil
	case "seed":
		sampling.Seed = nextInt64Off(sampling.Seed, []int64{7, 42, 314159, 424242})
		clearProviderOptionIfNil(updated.ProviderOptions, "seed")
		return withSampling(updated, sampling), "Seed " + intLabel(sampling.Seed), nil
	case "parallel_tools":
		sampling.ParallelToolCalls = nextBoolPtr(sampling.ParallelToolCalls)
		clearProviderOptionIfNil(updated.ProviderOptions, "parallel_tool_calls")
		return withSampling(updated, sampling), "Parallel tools " + boolLabel(sampling.ParallelToolCalls), nil
	case "think":
		updated.Think = !updated.Think
		return withSampling(updated, sampling), "Thinking mode " + onOff(updated.Think), nil
	default:
		return model, "", fmt.Errorf("unknown sampling control %q", controlID)
	}
}

func SetSamplingControlValue(model SelectedModel, controlID, raw string) (SelectedModel, string, error) {
	updated := cloneSelectedModel(model)
	sampling := ensureSampling(updated.Sampling)

	switch controlID {
	case "preset":
		presetID := strings.TrimSpace(raw)
		if presetID == "" || strings.EqualFold(presetID, "manual") {
			sampling.Preset = "manual"
			updated.SamplingPreset = "manual"
			updated.Sampling = sampling
			return updated, "Sampling preset manual", nil
		}
		return ApplySamplingPreset(updated, presetID)
	case "verbosity":
		value, err := parseEnum(raw, SamplingVerbosityTerse, SamplingVerbosityBalance, SamplingVerbosityMaximal)
		if err != nil {
			return model, "", err
		}
		sampling.Verbosity = value
	case "temperature":
		value, err := parseFloat64(raw, 0, 2)
		if err != nil {
			return model, "", err
		}
		updated.Temperature = ptrFloat64(value)
	case "top_p":
		value, err := parseFloat64(raw, 0, 1)
		if err != nil {
			return model, "", err
		}
		updated.TopP = ptrFloat64(value)
	case "top_k":
		value, err := parseInt64(raw, 0, 1000000)
		if err != nil {
			return model, "", err
		}
		updated.TopK = ptrInt64(value)
	case "max_tokens":
		value, err := parseInt64(raw, 1, 2000000)
		if err != nil {
			return model, "", err
		}
		updated.MaxTokens = value
	case "repeat_penalty":
		value, err := parseOptionalFloat64(raw, 1, 2)
		if err != nil {
			return model, "", err
		}
		sampling.RepeatPenalty = value
		clearProviderOptionIfNil(updated.ProviderOptions, "repeat_penalty")
	case "min_p":
		value, err := parseOptionalFloat64(raw, 0, 1)
		if err != nil {
			return model, "", err
		}
		sampling.MinP = value
		clearProviderOptionIfNil(updated.ProviderOptions, "min_p")
	case "top_a":
		value, err := parseOptionalFloat64(raw, 0, 1)
		if err != nil {
			return model, "", err
		}
		sampling.TopA = value
		clearProviderOptionIfNil(updated.ProviderOptions, "top_a")
	case "seed":
		value, err := parseOptionalInt64(raw, -1<<62, 1<<62-1)
		if err != nil {
			return model, "", err
		}
		sampling.Seed = value
		clearProviderOptionIfNil(updated.ProviderOptions, "seed")
	case "parallel_tools":
		value, err := parseOptionalBool(raw)
		if err != nil {
			return model, "", err
		}
		sampling.ParallelToolCalls = value
		clearProviderOptionIfNil(updated.ProviderOptions, "parallel_tool_calls")
	case "think":
		value, err := parseBool(raw)
		if err != nil {
			return model, "", err
		}
		updated.Think = value
	default:
		return model, "", fmt.Errorf("unknown sampling control %q", controlID)
	}

	sampling.Preset = "manual"
	updated.SamplingPreset = "manual"
	updated.Sampling = sampling
	if controlID == "think" {
		return updated, "Thinking mode " + onOff(updated.Think), nil
	}
	labels := SamplingControlLabels(updated)
	return updated, fmt.Sprintf("%s %s", samplingControlTitle(controlID), labels[controlID]), nil
}

func SamplingControlLabels(model SelectedModel) map[string]string {
	sampling := ensureSampling(model.Sampling)
	return map[string]string{
		"verbosity":      cmp.Or(sampling.Verbosity, SamplingVerbosityBalance),
		"temperature":    fmt.Sprintf("%.2f", derefFloat64(model.Temperature)),
		"top_p":          fmt.Sprintf("%.2f", derefFloat64(model.TopP)),
		"top_k":          fmt.Sprintf("%d", derefInt64(model.TopK)),
		"max_tokens":     fmt.Sprintf("%d", model.MaxTokens),
		"repeat_penalty": floatLabel(sampling.RepeatPenalty),
		"min_p":          floatLabel(sampling.MinP),
		"top_a":          floatLabel(sampling.TopA),
		"seed":           intLabel(sampling.Seed),
		"parallel_tools": boolLabel(sampling.ParallelToolCalls),
		"think":          onOff(model.Think),
	}
}

func SamplingPresetSummary(preset SamplingPreset) string {
	parts := []string{fmt.Sprintf("max %d", preset.MaxTokens)}
	if preset.Temperature != nil {
		parts = append(parts, fmt.Sprintf("temp %.2f", *preset.Temperature))
	}
	if preset.TopP != nil {
		parts = append(parts, fmt.Sprintf("top_p %.2f", *preset.TopP))
	}
	if preset.TopK != nil {
		parts = append(parts, fmt.Sprintf("top_k %d", *preset.TopK))
	}
	if preset.Reasoning != "" {
		parts = append(parts, "reason "+preset.Reasoning)
	}
	if preset.Think != nil && *preset.Think {
		parts = append(parts, "think on")
	}
	return strings.Join(parts, "  ")
}

func SamplingSummary(model SelectedModel) string {
	labels := SamplingControlLabels(model)
	return strings.Join([]string{
		"preset=" + CurrentSamplingPreset(model),
		"temp=" + labels["temperature"],
		"top_p=" + labels["top_p"],
		"top_k=" + labels["top_k"],
		"verbosity=" + labels["verbosity"],
	}, " • ")
}

func clearProviderOptionIfNil(options map[string]any, key string) {
	if options != nil {
		delete(options, key)
	}
}

func cloneSamplingPreset(preset SamplingPreset) SamplingPreset {
	preset.Temperature = cloneFloat64Ptr(preset.Temperature)
	preset.TopP = cloneFloat64Ptr(preset.TopP)
	preset.TopK = cloneInt64Ptr(preset.TopK)
	preset.Frequency = cloneFloat64Ptr(preset.Frequency)
	preset.Presence = cloneFloat64Ptr(preset.Presence)
	preset.Think = cloneBoolPtr(preset.Think)
	preset.Sampling = *cloneSampling(&preset.Sampling)
	return preset
}

func cloneSelectedModel(model SelectedModel) SelectedModel {
	out := model
	out.ProviderOptions = maps.Clone(model.ProviderOptions)
	out.Temperature = cloneFloat64Ptr(model.Temperature)
	out.TopP = cloneFloat64Ptr(model.TopP)
	out.TopK = cloneInt64Ptr(model.TopK)
	out.FrequencyPenalty = cloneFloat64Ptr(model.FrequencyPenalty)
	out.PresencePenalty = cloneFloat64Ptr(model.PresencePenalty)
	out.Sampling = cloneSampling(model.Sampling)
	return out
}

func cloneSampling(in *SamplingOptions) *SamplingOptions {
	if in == nil {
		return nil
	}
	out := *in
	out.Seed = cloneInt64Ptr(in.Seed)
	out.MinP = cloneFloat64Ptr(in.MinP)
	out.TopA = cloneFloat64Ptr(in.TopA)
	out.RepeatPenalty = cloneFloat64Ptr(in.RepeatPenalty)
	out.ParallelToolCalls = cloneBoolPtr(in.ParallelToolCalls)
	return &out
}

func ensureSampling(in *SamplingOptions) *SamplingOptions {
	if in != nil {
		return cloneSampling(in)
	}
	return &SamplingOptions{}
}

func withSampling(model SelectedModel, sampling *SamplingOptions) SelectedModel {
	model.Sampling = cloneSampling(sampling)
	return model
}

func nextString(current, fallback string, values ...string) string {
	if current == "" {
		current = fallback
	}
	idx := slices.Index(values, current)
	if idx == -1 {
		return values[0]
	}
	return values[(idx+1)%len(values)]
}

func nextFloat(current *float64, values []float64) *float64 {
	cur := derefFloat64(current)
	for i, value := range values {
		if value == cur {
			return ptrFloat64(values[(i+1)%len(values)])
		}
	}
	return ptrFloat64(values[0])
}

func nextFloatOff(current *float64, values []float64) *float64 {
	if current == nil {
		return ptrFloat64(values[0])
	}
	cur := *current
	for i, value := range values {
		if value == cur {
			if i == len(values)-1 {
				return nil
			}
			return ptrFloat64(values[i+1])
		}
	}
	return ptrFloat64(values[0])
}

func nextInt64(current *int64, values []int64) *int64 {
	cur := derefInt64(current)
	for i, value := range values {
		if value == cur {
			return ptrInt64(values[(i+1)%len(values)])
		}
	}
	return ptrInt64(values[0])
}

func nextInt64Off(current *int64, values []int64) *int64 {
	if current == nil {
		return ptrInt64(values[0])
	}
	cur := *current
	for i, value := range values {
		if value == cur {
			if i == len(values)-1 {
				return nil
			}
			return ptrInt64(values[i+1])
		}
	}
	return ptrInt64(values[0])
}

func nextBoolPtr(current *bool) *bool {
	switch {
	case current == nil:
		return ptrBool(true)
	case *current:
		return ptrBool(false)
	default:
		return nil
	}
}

func nextScalar[T comparable](current, fallback T, values []T) T {
	if current == fallback && !slices.Contains(values, current) {
		return values[0]
	}
	idx := slices.Index(values, current)
	if idx == -1 {
		return values[0]
	}
	return values[(idx+1)%len(values)]
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt64(v int64) *int64       { return &v }
func ptrBool(v bool) *bool          { return &v }

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func floatLabel(v *float64) string {
	if v == nil {
		return "off"
	}
	return fmt.Sprintf("%.2f", *v)
}

func intLabel(v *int64) string {
	if v == nil {
		return "off"
	}
	return fmt.Sprintf("%d", *v)
}

func boolLabel(v *bool) string {
	switch {
	case v == nil:
		return "auto"
	case *v:
		return "on"
	default:
		return "off"
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func samplingControlTitle(controlID string) string {
	switch controlID {
	case "verbosity":
		return "Verbosity"
	case "temperature":
		return "Temperature"
	case "top_p":
		return "Top-p"
	case "top_k":
		return "Top-k"
	case "max_tokens":
		return "Max tokens"
	case "repeat_penalty":
		return "Repeat penalty"
	case "min_p":
		return "Min-p"
	case "top_a":
		return "Top-a"
	case "seed":
		return "Seed"
	case "parallel_tools":
		return "Parallel tools"
	case "think":
		return "Thinking mode"
	default:
		return controlID
	}
}

func parseEnum(raw string, allowed ...string) (string, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid value %q; allowed: %s", raw, strings.Join(allowed, ", "))
}

func parseBool(raw string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "1", "true", "on", "yes", "y":
		return true, nil
	case "0", "false", "off", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q; use on/off or true/false", raw)
	}
}

func parseOptionalBool(raw string) (*bool, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "auto", "default":
		return nil, nil
	default:
		parsed, err := parseBool(value)
		if err != nil {
			return nil, err
		}
		return ptrBool(parsed), nil
	}
}

func parseFloat64(raw string, minValue, maxValue float64) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("value %.4f out of range [%.2f, %.2f]", value, minValue, maxValue)
	}
	return value, nil
}

func parseOptionalFloat64(raw string, minValue, maxValue float64) (*float64, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" || value == "off" || value == "none" {
		return nil, nil
	}
	parsed, err := parseFloat64(value, minValue, maxValue)
	if err != nil {
		return nil, err
	}
	return ptrFloat64(parsed), nil
}

func parseInt64(raw string, minValue, maxValue int64) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("value %d out of range [%d, %d]", value, minValue, maxValue)
	}
	return value, nil
}

func parseOptionalInt64(raw string, minValue, maxValue int64) (*int64, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" || value == "off" || value == "none" || value == "auto" {
		return nil, nil
	}
	parsed, err := parseInt64(value, minValue, maxValue)
	if err != nil {
		return nil, err
	}
	return ptrInt64(parsed), nil
}
