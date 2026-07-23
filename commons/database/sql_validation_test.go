package commons

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateAdminPasswordUsesExplicitSID(t *testing.T) {
	script := fmt.Sprintf(ValidateAdminPassword, "Secret123!", "ORCL1")
	if !strings.Contains(script, `@ORCL1 as sysdba`) {
		t.Fatalf("expected explicit SID in validation script, got %q", script)
	}
	if strings.Contains(script, "${ORACLE_SID}") {
		t.Fatalf("validation script must not contain an unexpanded ORACLE_SID placeholder: %q", script)
	}
}

func TestValidateOracleSQLPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "plain password", password: "Secret123!", wantErr: false},
		{name: "double quote", password: "bad\"pw", wantErr: true},
		{name: "newline", password: "bad\npw", wantErr: true},
		{name: "carriage return", password: "bad\rpw", wantErr: true},
		{name: "nul", password: "bad\x00pw", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOracleSQLPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOracleSQLPassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestParseOracleBinarySizeLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		want      string
		wantBytes int64
		wantErr   bool
	}{
		{name: "oracle literal", value: "50G", want: "50G", wantBytes: 50 * 1024 * 1024 * 1024},
		{name: "kubernetes binary quantity", value: "120Gi", want: "120G", wantBytes: 120 * 1024 * 1024 * 1024},
		{name: "lowercase quantity", value: "8mi", want: "8M", wantBytes: 8 * 1024 * 1024},
		{name: "missing unit", value: "50", wantErr: true},
		{name: "decimal quantity", value: "1.5Gi", wantErr: true},
		{name: "injection payload", value: "50G scope=both; alter system set open_cursors=9999; --", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOracleBinarySizeLiteral(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseOracleBinarySizeLiteral(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Canonical != tt.want {
				t.Fatalf("ParseOracleBinarySizeLiteral(%q) canonical = %q, want %q", tt.value, got.Canonical, tt.want)
			}
			if got.Quantity.Value() != tt.wantBytes {
				t.Fatalf("ParseOracleBinarySizeLiteral(%q) bytes = %d, want %d", tt.value, got.Quantity.Value(), tt.wantBytes)
			}
		})
	}
}
