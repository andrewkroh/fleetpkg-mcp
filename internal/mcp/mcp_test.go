// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package mcp

import "testing"

func TestSplitCamelCaseToken(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ProcessTTYAttached", "Process TTY Attached"},
		{"sourceIP", "source IP"},
		{"TTY", "TTY"},
		{"process", "process"},
		{"HTTPSRequest", "HTTPS Request"},
		{"getHTTPSResponse", "get HTTPS Response"},
		{"snake_case_name", "snake case name"},
		{"camelCase_AND_snake", "camel Case AND snake"},
		{"", ""},
		{"X", "X"},
		{"IP", "IP"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := splitCamelCaseToken(tt.in)
			if got != tt.want {
				t.Errorf("splitCamelCaseToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ProcessTTYAttached fdr", "Process TTY Attached fdr"},
		{"process tty", "process tty"},
		{"sourceIP destinationPort", "source IP destination Port"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := splitCamelCase(tt.in)
			if got != tt.want {
				t.Errorf("splitCamelCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestImplicitOR(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// Plain tokens get OR-joined.
		{"process tty attached", "process OR tty OR attached"},
		// Single token unchanged.
		{"tty", "tty"},
		// Existing operators pass through.
		{"network AND bytes", "network AND bytes"},
		{"process OR tty", "process OR tty"},
		{"NOT deprecated", "NOT deprecated"},
		// Phrases pass through.
		{`"source address"`, `"source address"`},
		// Prefix wildcards pass through.
		{"authent*", "authent*"},
		// Parentheses pass through.
		{"(process OR tty) AND attached", "(process OR tty) AND attached"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := implicitOR(tt.in)
			if got != tt.want {
				t.Errorf("implicitOR(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
