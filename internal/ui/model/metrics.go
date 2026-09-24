package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *UI) resetMetrics() {
	now := time.Now()
	m.metricsStart = now
	m.metricsLastSample = now
	m.metricsLastTokens = 0
	m.metricsTokenRate = 0
}

func (m *UI) observeMetrics() {
	if m.session == nil {
		return
	}
	tokens := m.session.PromptTokens + m.session.CompletionTokens
	now := time.Now()
	if !m.metricsLastSample.IsZero() && now.Sub(m.metricsLastSample) >= 100*time.Millisecond {
		delta := tokens - m.metricsLastTokens
		if delta > 0 {
			m.metricsTokenRate = float64(delta) / now.Sub(m.metricsLastSample).Seconds()
		}
		m.metricsLastSample = now
		m.metricsLastTokens = tokens
	}
}

func (m *UI) metricsDuration() time.Duration {
	if m.metricsStart.IsZero() {
		return 0
	}
	return time.Since(m.metricsStart)
}

func formatMetricDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm %ds", int(duration.Minutes()), int(duration.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(duration.Hours()), int(duration.Minutes())%60)
}

func (m *UI) metricsEnabled() bool {
	if m.com == nil || m.com.App == nil {
		return false
	}
	if m.com.App.Config() == nil || m.com.App.Config().Options == nil || m.com.App.Config().Options.TUI == nil {
		return true
	}
	value := m.com.Config().Options.TUI.ShowMetrics
	return value == nil || *value
}

func (m *UI) renderMetrics(width int) string {
	if !m.metricsEnabled() || width <= 0 {
		return ""
	}

	parts := make([]string, 0, 6)
	parts = append(parts, formatMetricDuration(m.metricsDuration()))
	if m.metricsTokenRate > 0 {
		parts = append(parts, fmt.Sprintf("%.1f tok/s", m.metricsTokenRate))
	}
	if m.session != nil {
		used := m.session.PromptTokens + m.session.CompletionTokens
		contextWindow := int64(0)
		if model := m.selectedLargeModel(); model != nil {
			contextWindow = model.CatwalkCfg.ContextWindow
		}
		if contextWindow > 0 {
			percentage := int(float64(used) / float64(contextWindow) * 100)
			parts = append(parts, fmt.Sprintf("%d%% context", percentage))
		} else if used > 0 {
			parts = append(parts, fmt.Sprintf("%s tokens", formatMetricCount(used)))
		}
		if m.session.Cost > 0 {
			parts = append(parts, fmt.Sprintf("$%.2f", m.session.Cost))
		}
	}
	if m.promptQueue > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", m.promptQueue))
	}
	state := "ready"
	if m.isAgentBusy() {
		state = "working"
	}
	parts = append(parts, state)

	metric := strings.Join(parts, "  ·  ")
	metric = ansi.Truncate(metric, width, "…")
	return m.com.Styles.Experience.Metric.Render(metric)
}

func formatMetricCount(value int64) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.1fK", float64(value)/1_000)
	default:
		return fmt.Sprintf("%d", value)
	}
}

func (m *UI) renderExperienceBadge(width int) string {
	if width <= 0 || m.com == nil || m.com.Styles == nil {
		return ""
	}
	t := m.com.Styles
	label := t.Experience.Accent.Render("SALAMEY")
	descriptor := t.Experience.MetricMuted.Render("LOCAL-FIRST")
	content := lipgloss.JoinHorizontal(lipgloss.Left, label, "  ", descriptor)
	return ansi.Truncate(content, width, "…")
}
