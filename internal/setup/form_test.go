package setup

import "testing"

func TestResolveAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		existingKey string
		want        string
	}{
		{name: "blank input retains existing key", input: "", existingKey: "existing-secret", want: "existing-secret"},
		{name: "whitespace-only input retains existing key", input: "   ", existingKey: "existing-secret", want: "existing-secret"},
		{name: "non-blank input overrides existing key", input: "new-secret", existingKey: "existing-secret", want: "new-secret"},
		{name: "blank input with no existing key stays blank", input: "", existingKey: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAPIKey(tt.input, tt.existingKey); got != tt.want {
				t.Errorf("resolveAPIKey(%q, %q) = %q, want %q", tt.input, tt.existingKey, got, tt.want)
			}
		})
	}
}

func TestValidateModel(t *testing.T) {
	if err := validateModel("translategemma"); err != nil {
		t.Errorf("validateModel() unexpected error: %v", err)
	}
	if err := validateModel(""); err == nil {
		t.Error("validateModel(\"\") = nil error, want error")
	}
	if err := validateModel("   "); err == nil {
		t.Error("validateModel(whitespace) = nil error, want error")
	}
}

func TestValidateAPIKeyInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		existingKey string
		wantErr     bool
	}{
		{name: "blank with no existing key is invalid", input: "", existingKey: "", wantErr: true},
		{name: "blank with existing key is valid", input: "", existingKey: "existing-secret"},
		{name: "non-blank is always valid", input: "new-secret", existingKey: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAPIKeyInput(tt.input, tt.existingKey)
			if tt.wantErr && err == nil {
				t.Fatal("validateAPIKeyInput() = nil error, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateAPIKeyInput() unexpected error: %v", err)
			}
		})
	}
}
