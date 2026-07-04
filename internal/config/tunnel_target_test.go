// SPDX-FileCopyrightText: 2026 Vedran Lebo <vedran@flyingpenguin.tech>
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTunnelTarget_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKind  TunnelTargetKind
		wantLit   string
		wantExp   string
		wantStack string
		wantOut   string
	}{
		{
			name:     "literal string",
			input:    `target: i-0abc123def456789a`,
			wantKind: TunnelTargetKindLiteral,
			wantLit:  "i-0abc123def456789a",
		},
		{
			name:    "export map",
			input:   "target:\n  export: MyStack-BastionInstanceId\n",
			wantKind: TunnelTargetKindExport,
			wantExp: "MyStack-BastionInstanceId",
		},
		{
			name:      "stack+output map",
			input:     "target:\n  stack: my-infra\n  output: BastionInstanceId\n",
			wantKind:  TunnelTargetKindStackOutput,
			wantStack: "my-infra",
			wantOut:   "BastionInstanceId",
		},
		{
			name:     "invalid map (unrecognised keys)",
			input:    "target:\n  bucket: my-bucket\n",
			wantKind: TunnelTargetKindInvalid,
		},
		{
			name:      "stack without output (caught at tunnel start time)",
			input:     "target:\n  stack: my-infra\n",
			wantKind:  TunnelTargetKindStackOutput,
			wantStack: "my-infra",
			wantOut:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out struct {
				Target TunnelTarget `yaml:"target"`
			}
			if err := yaml.Unmarshal([]byte(tt.input), &out); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := out.Target
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Literal != tt.wantLit {
				t.Errorf("Literal = %q, want %q", got.Literal, tt.wantLit)
			}
			if got.Export != tt.wantExp {
				t.Errorf("Export = %q, want %q", got.Export, tt.wantExp)
			}
			if got.Stack != tt.wantStack {
				t.Errorf("Stack = %q, want %q", got.Stack, tt.wantStack)
			}
			if got.Output != tt.wantOut {
				t.Errorf("Output = %q, want %q", got.Output, tt.wantOut)
			}
		})
	}
}

func TestTunnelTarget_MarshalYAML(t *testing.T) {
	tests := []struct {
		name   string
		target TunnelTarget
		want   string
	}{
		{
			name:   "literal round-trips as plain string",
			target: TunnelTarget{Kind: TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"},
			want:   "target: i-0abc123def456789a\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := struct {
				Target TunnelTarget `yaml:"target"`
			}{Target: tt.target}
			out, err := yaml.Marshal(in)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if string(out) != tt.want {
				t.Errorf("got %q, want %q", string(out), tt.want)
			}
		})
	}
}

func TestTunnelTarget_MarshalYAML_ExportRoundTrip(t *testing.T) {
	in := struct {
		Target TunnelTarget `yaml:"target"`
	}{
		Target: TunnelTarget{Kind: TunnelTargetKindExport, Export: "MyStack-BastionInstanceId"},
	}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var roundTrip struct {
		Target TunnelTarget `yaml:"target"`
	}
	if err := yaml.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if roundTrip.Target.Kind != TunnelTargetKindExport {
		t.Errorf("round-trip Kind = %q, want %q", roundTrip.Target.Kind, TunnelTargetKindExport)
	}
	if roundTrip.Target.Export != "MyStack-BastionInstanceId" {
		t.Errorf("round-trip Export = %q, want %q", roundTrip.Target.Export, "MyStack-BastionInstanceId")
	}
}

func TestTunnelTarget_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKind  TunnelTargetKind
		wantLit   string
		wantExp   string
		wantStack string
		wantOut   string
	}{
		{
			name:     "bare string (old state-file format, back-compat)",
			input:    `"i-0abc123def456789a"`,
			wantKind: TunnelTargetKindLiteral,
			wantLit:  "i-0abc123def456789a",
		},
		{
			name:     "export object",
			input:    `{"export":"MyStack-BastionInstanceId"}`,
			wantKind: TunnelTargetKindExport,
			wantExp:  "MyStack-BastionInstanceId",
		},
		{
			name:      "stack+output object",
			input:     `{"stack":"my-infra","output":"BastionInstanceId"}`,
			wantKind:  TunnelTargetKindStackOutput,
			wantStack: "my-infra",
			wantOut:   "BastionInstanceId",
		},
		{
			name:     "invalid object (unrecognised keys)",
			input:    `{"bucket":"my-bucket"}`,
			wantKind: TunnelTargetKindInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got TunnelTarget
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Literal != tt.wantLit {
				t.Errorf("Literal = %q, want %q", got.Literal, tt.wantLit)
			}
			if got.Export != tt.wantExp {
				t.Errorf("Export = %q, want %q", got.Export, tt.wantExp)
			}
			if got.Stack != tt.wantStack {
				t.Errorf("Stack = %q, want %q", got.Stack, tt.wantStack)
			}
			if got.Output != tt.wantOut {
				t.Errorf("Output = %q, want %q", got.Output, tt.wantOut)
			}
		})
	}
}

func TestTunnelTarget_MarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		target TunnelTarget
		want   string
	}{
		{
			name:   "literal round-trips as bare string",
			target: TunnelTarget{Kind: TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"},
			want:   `"i-0abc123def456789a"`,
		},
		{
			name:   "export round-trips as object",
			target: TunnelTarget{Kind: TunnelTargetKindExport, Export: "MyStack-BastionInstanceId"},
			want:   `{"export":"MyStack-BastionInstanceId"}`,
		},
		{
			name:   "stack+output round-trips as object",
			target: TunnelTarget{Kind: TunnelTargetKindStackOutput, Stack: "my-infra", Output: "BastionInstanceId"},
			want:   `{"output":"BastionInstanceId","stack":"my-infra"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := json.Marshal(tt.target)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if string(out) != tt.want {
				t.Errorf("got %q, want %q", string(out), tt.want)
			}
		})
	}
}

func TestTunnelTarget_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		target TunnelTarget
		want   bool
	}{
		{"zero value", TunnelTarget{}, true},
		{"literal", TunnelTarget{Kind: TunnelTargetKindLiteral, Literal: "i-xxx"}, false},
		{"export", TunnelTarget{Kind: TunnelTargetKindExport, Export: "my-export"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
