package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSamplingPresetByIDClonesValues(t *testing.T) {
	t.Parallel()

	first, ok := SamplingPresetByID(" focused-82 ")
	require.True(t, ok)
	require.Equal(t, "focused-82", first.ID)
	require.Equal(t, "Focused 82", first.Title)
	require.Equal(t, "high", first.Reasoning)

	first.Temperature = ptrFloat64(0.99)
	first.Sampling.Seed = ptrInt64(99)

	second, ok := SamplingPresetByID("focused-82")
	require.True(t, ok)
	require.InDelta(t, 0.18, *second.Temperature, 0.0001)
	require.Nil(t, second.Sampling.Seed)
	require.NotSame(t, first.Temperature, second.Temperature)
}

func TestSamplingPresetIDsAreDeterministic(t *testing.T) {
	t.Parallel()

	ids := SamplingPresetIDs()
	require.Equal(t, []string{
		"manual",
		"focused-82",
		"math-borg",
		"escalate-88",
		"quiet-surgeon",
		"swarm-matrix",
		"doc-smith",
		"cinematic-local",
		"provider-sniper",
		"godzilla-lab",
	}, ids)
}

func TestApplySamplingPresetReplacesStaleValues(t *testing.T) {
	t.Parallel()

	model := SelectedModel{
		Provider:         "openai",
		Model:            "gpt-5",
		ReasoningEffort:  "low",
		Think:            true,
		MaxTokens:        1,
		Temperature:      ptrFloat64(0.9),
		TopP:             ptrFloat64(0.1),
		TopK:             ptrInt64(1),
		FrequencyPenalty: ptrFloat64(0.2),
		PresencePenalty:  ptrFloat64(0.3),
		ProviderOptions: map[string]any{
			"custom":    "value",
			"seed":      int64(99),
			"min_p":     0.2,
			"verbosity": "maximal",
		},
		Sampling: &SamplingOptions{
			Preset:        "focused-82",
			Verbosity:     SamplingVerbosityMaximal,
			Seed:          ptrInt64(7),
			MinP:          ptrFloat64(0.2),
			RepeatPenalty: ptrFloat64(1.2),
		},
	}

	got, message, err := ApplySamplingPreset(model, "quiet-surgeon")
	require.NoError(t, err)
	require.Equal(t, "Quiet Surgeon preset loaded", message)
	require.Equal(t, "quiet-surgeon", got.SamplingPreset)
	require.EqualValues(t, 4096, got.MaxTokens)
	require.InDelta(t, 0.08, *got.Temperature, 0.0001)
	require.InDelta(t, 0.70, *got.TopP, 0.0001)
	require.EqualValues(t, 24, *got.TopK)
	require.InDelta(t, 0.25, *got.FrequencyPenalty, 0.0001)
	require.Nil(t, got.PresencePenalty)
	require.Empty(t, got.ReasoningEffort)
	require.False(t, got.Think)
	require.Equal(t, "quiet-surgeon", got.Sampling.Preset)
	require.Nil(t, got.Sampling.Seed)
	require.Nil(t, got.Sampling.MinP)
	require.InDelta(t, 1.05, *got.Sampling.RepeatPenalty, 0.0001)
	require.Equal(t, "value", got.ProviderOptions["custom"])
	require.NotContains(t, got.ProviderOptions, "seed")
	require.NotContains(t, got.ProviderOptions, "min_p")
	require.NotContains(t, got.ProviderOptions, "verbosity")

	got.ProviderOptions["custom"] = "changed"
	got.Sampling.Seed = ptrInt64(123)
	require.Equal(t, "value", model.ProviderOptions["custom"])
	require.EqualValues(t, 7, *model.Sampling.Seed)
	require.Equal(t, SamplingVerbosityMaximal, model.Sampling.Verbosity)
}

func TestApplySamplingControlClearsPresetAndMutatesOnlyCopy(t *testing.T) {
	t.Parallel()

	model := SelectedModel{
		SamplingPreset: "focused-82",
		Sampling: &SamplingOptions{
			Preset:    "focused-82",
			Verbosity: SamplingVerbosityBalance,
		},
	}

	got, message, err := ApplySamplingControl(model, "verbosity")
	require.NoError(t, err)
	require.Equal(t, "Verbosity maximal", message)
	require.Equal(t, SamplingVerbosityMaximal, got.Sampling.Verbosity)
	require.Equal(t, "manual", got.SamplingPreset)
	require.Equal(t, "manual", got.Sampling.Preset)
	require.Equal(t, SamplingVerbosityBalance, model.Sampling.Verbosity)
	require.Equal(t, "focused-82", model.SamplingPreset)
}

func TestApplySamplingControlCyclesOptionalValuesToOff(t *testing.T) {
	t.Parallel()

	model := SelectedModel{
		ProviderOptions: map[string]any{"seed": int64(424242)},
		Sampling: &SamplingOptions{
			Seed: ptrInt64(424242),
		},
	}

	got, message, err := ApplySamplingControl(model, "seed")
	require.NoError(t, err)
	require.Equal(t, "Seed off", message)
	require.Nil(t, got.Sampling.Seed)
	require.NotContains(t, got.ProviderOptions, "seed")
	require.EqualValues(t, 424242, *model.Sampling.Seed)
	require.EqualValues(t, 424242, model.ProviderOptions["seed"])
}

func TestSetSamplingControlValuePreservesExplicitZeroValues(t *testing.T) {
	t.Parallel()

	temperature, message, err := SetSamplingControlValue(SelectedModel{}, "temperature", "0")
	require.NoError(t, err)
	require.Equal(t, "Temperature 0.00", message)
	require.NotNil(t, temperature.Temperature)
	require.Equal(t, 0.0, *temperature.Temperature)

	topK, message, err := SetSamplingControlValue(SelectedModel{}, "top_k", "0")
	require.NoError(t, err)
	require.Equal(t, "Top-k 0", message)
	require.NotNil(t, topK.TopK)
	require.EqualValues(t, 0, *topK.TopK)

	minP, _, err := SetSamplingControlValue(SelectedModel{}, "min_p", "0")
	require.NoError(t, err)
	require.NotNil(t, minP.Sampling)
	require.NotNil(t, minP.Sampling.MinP)
	require.Equal(t, 0.0, *minP.Sampling.MinP)
	require.Equal(t, 0.0, minP.EffectiveProviderOptions()["min_p"])
}

func TestSetSamplingControlValueSupportsAutoAndExplicitFalse(t *testing.T) {
	t.Parallel()

	model := SelectedModel{
		ProviderOptions: map[string]any{
			"parallel_tool_calls": true,
			"seed":                int64(9),
		},
		Sampling: &SamplingOptions{
			ParallelToolCalls: ptrBool(true),
			Seed:              ptrInt64(9),
		},
	}

	parallel, message, err := SetSamplingControlValue(model, "parallel_tools", "auto")
	require.NoError(t, err)
	require.Equal(t, "Parallel tools auto", message)
	require.Nil(t, parallel.Sampling.ParallelToolCalls)
	require.NotContains(t, parallel.ProviderOptions, "parallel_tool_calls")

	off, _, err := SetSamplingControlValue(model, "parallel_tools", "off")
	require.NoError(t, err)
	require.NotNil(t, off.Sampling.ParallelToolCalls)
	require.False(t, *off.Sampling.ParallelToolCalls)
	require.Equal(t, false, off.EffectiveProviderOptions()["parallel_tool_calls"])

	seed, _, err := SetSamplingControlValue(model, "seed", "off")
	require.NoError(t, err)
	require.Nil(t, seed.Sampling.Seed)
	require.NotContains(t, seed.ProviderOptions, "seed")
}

func TestEffectiveProviderOptionsMergesWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	providerOptions := map[string]any{
		"existing":            "value",
		"verbosity":           "terse",
		"seed":                int64(1),
		"parallel_tool_calls": true,
	}
	model := SelectedModel{
		ProviderOptions: providerOptions,
		Sampling: &SamplingOptions{
			Verbosity:         SamplingVerbosityMaximal,
			Seed:              ptrInt64(0),
			MinP:              ptrFloat64(0),
			TopA:              ptrFloat64(0),
			RepeatPenalty:     ptrFloat64(1),
			ParallelToolCalls: ptrBool(false),
		},
	}

	got := model.EffectiveProviderOptions()
	require.Equal(t, "value", got["existing"])
	require.Equal(t, SamplingVerbosityMaximal, got["verbosity"])
	require.EqualValues(t, 0, got["seed"])
	require.Equal(t, 0.0, got["min_p"])
	require.Equal(t, 0.0, got["top_a"])
	require.Equal(t, 1.0, got["repeat_penalty"])
	require.Equal(t, false, got["parallel_tool_calls"])
	got["existing"] = "changed"
	require.Equal(t, "value", providerOptions["existing"])

	empty := (SelectedModel{}).EffectiveProviderOptions()
	require.NotNil(t, empty)
}

func TestSamplingControlsAndSummariesUseRuntimeFields(t *testing.T) {
	t.Parallel()

	labels := SamplingControlLabels(SelectedModel{})
	require.Equal(t, SamplingVerbosityBalance, labels["verbosity"])
	require.Equal(t, "0.00", labels["temperature"])
	require.Equal(t, "off", labels["min_p"])
	require.Equal(t, "auto", labels["parallel_tools"])
	require.NotContains(t, labels, "physics_face")

	preset, ok := SamplingPresetByID("focused-82")
	require.True(t, ok)
	require.Equal(t, "max 8192  temp 0.18  top_p 0.82  top_k 48  reason high", SamplingPresetSummary(preset))

	model, _, err := ApplySamplingPreset(SelectedModel{}, "focused-82")
	require.NoError(t, err)
	require.Equal(t, "preset=focused-82 • temp=0.18 • top_p=0.82 • top_k=48 • verbosity=balanced", SamplingSummary(model))
}

func TestPreserveSelectedModelSettings(t *testing.T) {
	previous := SelectedModel{
		Provider:        "local",
		Model:           "model",
		MaxTokens:       4096,
		Temperature:     ptrFloat64(0.17),
		SamplingPreset:  "focused-82",
		Sampling:        &SamplingOptions{Verbosity: SamplingVerbosityTerse},
		ProviderOptions: map[string]any{"custom": true},
	}
	got := PreserveSelectedModelSettings(SelectedModel{
		Provider:  "local",
		Model:     "model",
		MaxTokens: 1000,
	}, previous)
	require.Equal(t, int64(4096), got.MaxTokens)
	require.InDelta(t, 0.17, *got.Temperature, 0.0001)
	require.Equal(t, "focused-82", got.SamplingPreset)
	require.Equal(t, SamplingVerbosityTerse, got.Sampling.Verbosity)
	require.Equal(t, true, got.ProviderOptions["custom"])
}

func TestApplySelectedModelSettingsLoadsPresetOnly(t *testing.T) {
	model := SelectedModel{Provider: "local", Model: "model"}
	applySelectedModelSettings(&model, SelectedModel{SamplingPreset: "escalate-88"})

	require.Equal(t, int64(24576), model.MaxTokens)
	require.True(t, model.Think)
	require.Equal(t, "high", model.ReasoningEffort)
	require.Equal(t, "escalate-88", model.Sampling.Preset)
}

func TestSamplingCoreRejectsUnknownPreset(t *testing.T) {
	t.Parallel()

	_, _, err := ApplySamplingPreset(SelectedModel{}, "does-not-exist")
	require.Error(t, err)
}
