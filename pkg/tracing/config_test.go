package tracing

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if config.ServiceName == "" {
		t.Error("ServiceName should not be empty")
	}

	if config.ServiceVersion == "" {
		t.Error("ServiceVersion should not be empty")
	}

	if config.Sampling.SamplingRate != 1.0 {
		t.Errorf("SamplingRate = %f, want 1.0", config.Sampling.SamplingRate)
	}

	if config.Exporter.Type != ExporterTypeOTLP {
		t.Errorf("Exporter.Type = %s, want %s", config.Exporter.Type, ExporterTypeOTLP)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config with OTLP",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Environment:    "test",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeOTLP,
					OTLP: OTLPConfig{
						Endpoint: "localhost:4317",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: &Config{
				Enabled:        true,
				ServiceVersion: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "invalid sampling rate - negative",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: -0.1,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid sampling rate - too high",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.5,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid exporter type",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: "invalid-exporter",
				},
			},
			wantErr: true,
		},
		{
			name: "OTLP without endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeOTLP,
					OTLP: OTLPConfig{
						Endpoint: "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Jaeger without endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type:   ExporterTypeJaeger,
					Jaeger: JaegerConfig{},
				},
			},
			wantErr: true,
		},
		{
			name: "Jaeger with agent endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeJaeger,
					Jaeger: JaegerConfig{
						AgentEndpoint: "localhost:6831",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Jaeger with collector endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeJaeger,
					Jaeger: JaegerConfig{
						CollectorEndpoint: "http://localhost:14268/api/traces",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Zipkin without endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type:   ExporterTypeZipkin,
					Zipkin: ZipkinConfig{},
				},
			},
			wantErr: true,
		},
		{
			name: "Zipkin with endpoint",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeZipkin,
					Zipkin: ZipkinConfig{
						Endpoint: "http://localhost:9411/api/v2/spans",
						Timeout:  10 * time.Second,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Stdout exporter",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeStdout,
				},
			},
			wantErr: false,
		},
		{
			name: "None exporter",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
				Exporter: ExporterConfig{
					Type: ExporterTypeNone,
				},
			},
			wantErr: false,
		},
		{
			name: "disabled config",
			config: &Config{
				Enabled:     false,
				ServiceName: "test-service",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ToResourceAttributes(t *testing.T) {
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		ResourceAttributes: map[string]string{
			"host.name":  "broker-01",
			"datacenter": "us-west-2",
		},
	}

	attrs := config.ToResourceAttributes()

	if len(attrs) == 0 {
		t.Error("ToResourceAttributes() returned empty slice")
	}

	// Verify we have at least the basic service attributes
	if len(attrs) < 3 {
		t.Errorf("Expected at least 3 attributes, got %d", len(attrs))
	}

	// Convert to map for easier checking
	attrMap := make(map[string]string)
	for _, attr := range attrs {
		attrMap[string(attr.Key)] = attr.Value.AsString()
	}

	// Verify service attributes
	if attrMap["service.name"] != "test-service" {
		t.Errorf("service.name = %v, want test-service", attrMap["service.name"])
	}

	if attrMap["service.version"] != "1.0.0" {
		t.Errorf("service.version = %v, want 1.0.0", attrMap["service.version"])
	}

	if attrMap["deployment.environment"] != "production" {
		t.Errorf("deployment.environment = %v, want production", attrMap["deployment.environment"])
	}

	// Verify custom attributes
	if attrMap["host.name"] != "broker-01" {
		t.Errorf("host.name = %v, want broker-01", attrMap["host.name"])
	}

	if attrMap["datacenter"] != "us-west-2" {
		t.Errorf("datacenter = %v, want us-west-2", attrMap["datacenter"])
	}
}

func TestConfig_ToResourceAttributes_MinimalConfig(t *testing.T) {
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
	}

	attrs := config.ToResourceAttributes()

	if len(attrs) < 3 {
		t.Errorf("Expected at least 3 attributes, got %d", len(attrs))
	}

	// Convert to map for easier checking
	attrMap := make(map[string]string)
	for _, attr := range attrs {
		attrMap[string(attr.Key)] = attr.Value.AsString()
	}

	// Should have at least service name, version, and environment
	if attrMap["service.name"] != "test-service" {
		t.Error("Missing service.name attribute")
	}

	if attrMap["service.version"] != "1.0.0" {
		t.Error("Missing service.version attribute")
	}

	if attrMap["deployment.environment"] != "test" {
		t.Error("Missing deployment.environment attribute")
	}
}
