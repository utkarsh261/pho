package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    invocation
		wantErr string
	}{
		{
			name: "no args launches dashboard",
			args: []string{},
			want: invocation{RootDir: "."},
		},
		{
			name: "plain flags",
			args: []string{"-debug", "-root", "~/code"},
			want: invocation{Debug: true, RootDir: "~/code"},
		},
		{
			name: "pr subcommand",
			args: []string{"pr", "123"},
			want: invocation{RootDir: ".", PRNumber: 123},
		},
		{
			name: "pr subcommand with hash prefix",
			args: []string{"pr", "#42"},
			want: invocation{RootDir: ".", PRNumber: 42},
		},
		{
			name: "flags before subcommand",
			args: []string{"-debug", "-root", "/tmp/repos", "pr", "7"},
			want: invocation{Debug: true, RootDir: "/tmp/repos", PRNumber: 7},
		},
		{
			name: "flags after subcommand",
			args: []string{"pr", "7", "-debug"},
			want: invocation{RootDir: ".", Debug: true, PRNumber: 7},
		},
		{
			name: "same flag on both sides, later wins",
			args: []string{"-root", "/before", "pr", "7", "-root", "/after"},
			want: invocation{RootDir: "/after", PRNumber: 7},
		},
		{
			name: "version flag",
			args: []string{"-version"},
			want: invocation{Version: true, RootDir: "."},
		},
		{
			name:    "missing number",
			args:    []string{"pr"},
			wantErr: "`pho pr` requires a pull request number",
		},
		{
			name:    "non-numeric number",
			args:    []string{"pr", "abc"},
			wantErr: "invalid pull request number",
		},
		{
			name:    "zero number",
			args:    []string{"pr", "0"},
			wantErr: "invalid pull request number",
		},
		{
			name:    "negative number",
			args:    []string{"pr", "-5"},
			wantErr: "invalid pull request number",
		},
		{
			name:    "unknown command",
			args:    []string{"list"},
			wantErr: "unknown command",
		},
		{
			name:    "extra positional after number",
			args:    []string{"pr", "123", "extra"},
			wantErr: "unexpected argument",
		},
		{
			name:    "unknown flag",
			args:    []string{"-nope"},
			wantErr: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseInvocation(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got invocation %+v", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("invocation mismatch:\ngot:  %+v\nwant: %+v", got, tt.want)
			}
		})
	}
}

func TestParseInvocationHelpReturnsErrHelp(t *testing.T) {
	t.Parallel()

	_, err := parseInvocation([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp for -h, got %v", err)
	}
}
