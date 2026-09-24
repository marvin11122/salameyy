package dialog

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

func samplingDialogTestCommon() *common.Common {
	st := styles.DefaultStyles()
	return &common.Common{Styles: &st}
}

func samplingDialogTestKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func samplingDialogTestTextKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text, Code: []rune(text)[0]}
}

func samplingDialogTestBool(value bool) *bool {
	return &value
}

func TestDialogLayerSamplingStateTransitions(t *testing.T) {
	t.Parallel()

	temperature := 0.4
	model := config.SelectedModel{
		Provider:    "provider",
		Model:       "model",
		Temperature: &temperature,
		Sampling: &config.SamplingOptions{
			Verbosity: config.SamplingVerbosityBalance,
		},
	}
	dialog := NewSampling(samplingDialogTestCommon(), model)

	action := dialog.HandleMsg(samplingDialogTestKey(tea.KeyRight))
	updated, ok := action.(ActionUpdateSampling)
	require.True(t, ok)
	require.Equal(t, "focused-82", updated.Model.SamplingPreset)
	require.InDelta(t, 0.18, *updated.Model.Temperature, 0.0001)
	require.InDelta(t, temperature, *model.Temperature, 0.0001)
	require.Equal(t, "focused-82", dialog.Model().SamplingPreset)

	require.True(t, dialog.SelectControl("temperature"))
	action = dialog.HandleMsg(samplingDialogTestKey(tea.KeyLeft))
	updated, ok = action.(ActionUpdateSampling)
	require.True(t, ok)
	require.InDelta(t, 0.08, *updated.Model.Temperature, 0.0001)

	require.True(t, dialog.SelectControl("think"))
	action = dialog.CycleControl(1)
	updated, ok = action.(ActionUpdateSampling)
	require.True(t, ok)
	require.True(t, updated.Model.Think)
}

func TestDialogLayerSamplingValueEditingAndValidation(t *testing.T) {
	t.Parallel()

	dialog := NewSampling(samplingDialogTestCommon(), config.SelectedModel{})
	require.True(t, dialog.SelectControl("temperature"))
	require.True(t, dialog.BeginEdit())
	require.True(t, dialog.IsEditing())

	action := dialog.HandleMsg(samplingDialogTestKey(tea.KeyEnter))
	_, ok := action.(ActionUpdateSampling)
	require.True(t, ok)
	require.False(t, dialog.IsEditing())
	require.InDelta(t, 0.0, *dialog.Model().Temperature, 0.0001)

	before := dialog.Model()
	require.Nil(t, dialog.SetControlValue("temperature", "not-a-number"))
	require.Equal(t, before, dialog.Model())
	require.NotEmpty(t, dialog.Error())
}

func TestDialogLayerDisplaySettingsStateAndActions(t *testing.T) {
	t.Parallel()

	dialog := NewDisplaySettings(samplingDialogTestCommon(), config.TUIOptions{
		PhysicsFace:    samplingDialogTestBool(false),
		AnimationSpeed: "fast",
		ShowMetrics:    samplingDialogTestBool(false),
		Transparent:    samplingDialogTestBool(true),
		CompactMode:    true,
	})

	require.Equal(t, DisplaySettingsState{
		PhysicsFace:    false,
		AnimationSpeed: "fast",
		ShowMetrics:    false,
		Transparent:    true,
		CompactMode:    true,
	}, dialog.State())

	require.True(t, dialog.SelectControl("animation_speed"))
	action := dialog.CycleControl(1)
	updated, ok := action.(ActionUpdateDisplaySettings)
	require.True(t, ok)
	require.Equal(t, "off", updated.AnimationSpeed)
	require.Equal(t, "off", dialog.State().AnimationSpeed)

	require.Nil(t, dialog.SetControl("animation_speed", "turbo"))
	require.NotEmpty(t, dialog.Error())
}

func TestDialogLayerKeybindingNormalizationAndEditing(t *testing.T) {
	t.Parallel()

	require.Equal(t, "ctrl+shift+a", mustNormalizeSamplingDialogKey(t, " Ctrl+Shift+A "))
	require.Equal(t, "alt+enter", mustNormalizeSamplingDialogKey(t, "option+return"))
	require.Error(t, ValidateKeybinding(""))
	require.Error(t, ValidateKeybinding("ctrl+c"))
	require.Error(t, ValidateKeybindings(map[string]string{
		"models":   "ctrl+a",
		"sessions": "ctrl+a",
	}))
	require.NoError(t, ValidateKeybindings(DefaultKeybindings()))

	dialog := NewKeybindings(samplingDialogTestCommon(), map[string]string{
		"models": "alt+m",
	})
	require.Equal(t, "alt+m", dialog.Binding("models"))

	action := dialog.SetBinding("sessions", "ctrl+alt+n")
	updated, ok := action.(ActionUpdateKeybinding)
	require.True(t, ok)
	require.Equal(t, "sessions", updated.Action)
	require.Equal(t, "ctrl+alt+n", updated.Key)
	require.Equal(t, "ctrl+alt+n", dialog.Binding("sessions"))

	before := dialog.Bindings()
	require.Nil(t, dialog.SetBinding("models", "ctrl+g"))
	require.Equal(t, before, dialog.Bindings())

	require.Nil(t, dialog.HandleMsg(samplingDialogTestKey(tea.KeyEnter)))
	require.True(t, dialog.IsEditing())
	action = dialog.HandleMsg(samplingDialogTestTextKey("x"))
	updated, ok = action.(ActionUpdateKeybinding)
	require.True(t, ok)
	require.Equal(t, "models", updated.Action)
	require.Equal(t, "x", updated.Key)
}

func TestDialogLayerDrawWithSnapshots(t *testing.T) {
	t.Parallel()

	com := samplingDialogTestCommon()
	screen := uv.NewScreenBuffer(100, 40)
	area := image.Rect(0, 0, 100, 40)
	NewSampling(com, config.SelectedModel{}).Draw(screen, area)
	NewDisplaySettings(com, config.TUIOptions{}).Draw(screen, area)
	NewKeybindings(com, map[string]string{}).Draw(screen, area)
}

func mustNormalizeSamplingDialogKey(t *testing.T, raw string) string {
	t.Helper()
	value, err := NormalizeKeyString(raw)
	require.NoError(t, err)
	return value
}
