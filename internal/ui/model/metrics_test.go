package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatMetricDuration(t *testing.T) {
	require.Equal(t, "9s", formatMetricDuration(9*time.Second))
	require.Equal(t, "2m 3s", formatMetricDuration(2*time.Minute+3*time.Second))
	require.Equal(t, "1h 2m", formatMetricDuration(time.Hour+2*time.Minute))
}

func TestFormatMetricCount(t *testing.T) {
	require.Equal(t, "12", formatMetricCount(12))
	require.Equal(t, "1.2K", formatMetricCount(1200))
	require.Equal(t, "1.2M", formatMetricCount(1200000))
}
