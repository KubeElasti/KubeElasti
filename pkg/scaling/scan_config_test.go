package scaling

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestScanConfigFromEnv(t *testing.T) {
	defaults := scanConfig{concurrency: 8, evaluationTimeout: 20 * time.Second, scaleDownConcurrency: 8}
	tests := []struct {
		name        string
		env         map[string]string
		want        scanConfig
		wantWarning bool
	}{
		{name: "unset uses defaults", want: defaults},
		{
			name: "valid values",
			env:  map[string]string{"SCAN_CONCURRENCY": "16", "SCAN_EVALUATION_TIMEOUT": "10s", "SCALE_DOWN_CONCURRENCY": "2"},
			want: scanConfig{concurrency: 16, evaluationTimeout: 10 * time.Second, scaleDownConcurrency: 2},
		},
		{name: "non-numeric concurrency", env: map[string]string{"SCAN_CONCURRENCY": "lots"}, want: defaults, wantWarning: true},
		{name: "zero concurrency", env: map[string]string{"SCAN_CONCURRENCY": "0"}, want: defaults, wantWarning: true},
		{name: "negative scale-down concurrency", env: map[string]string{"SCALE_DOWN_CONCURRENCY": "-1"}, want: defaults, wantWarning: true},
		{name: "unparsable timeout", env: map[string]string{"SCAN_EVALUATION_TIMEOUT": "soon"}, want: defaults, wantWarning: true},
		{name: "non-positive timeout", env: map[string]string{"SCAN_EVALUATION_TIMEOUT": "0s"}, want: defaults, wantWarning: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range []string{"SCAN_CONCURRENCY", "SCAN_EVALUATION_TIMEOUT", "SCALE_DOWN_CONCURRENCY"} {
				t.Setenv(name, tt.env[name])
			}
			core, logs := observer.New(zapcore.WarnLevel)

			got := scanConfigFromEnv(zap.New(core))

			if got != tt.want {
				t.Fatalf("scanConfigFromEnv() = %+v, want %+v", got, tt.want)
			}
			if warned := logs.Len() > 0; warned != tt.wantWarning {
				t.Fatalf("warned = %v, want %v", warned, tt.wantWarning)
			}
		})
	}
}
