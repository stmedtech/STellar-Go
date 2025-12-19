package utils

import (
	"testing"
)

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		cmdLine string
		command string
		args    []string
	}{
		{
			name:    "simple command",
			cmdLine: "echo hello",
			command: "echo",
			args:    []string{"hello"},
		},
		{
			name:    "command only",
			cmdLine: "ls",
			command: "ls",
			args:    []string{},
		},
		{
			name:    "multiple args",
			cmdLine: "cmd arg1 arg2 arg3",
			command: "cmd",
			args:    []string{"arg1", "arg2", "arg3"},
		},
		{
			name:    "double quoted string",
			cmdLine: `cmd "arg with spaces"`,
			command: "cmd",
			args:    []string{"arg with spaces"},
		},
		{
			name:    "single quoted string",
			cmdLine: `cmd 'arg with spaces'`,
			command: "cmd",
			args:    []string{"arg with spaces"},
		},
		{
			name:    "mixed quotes",
			cmdLine: `cmd 'single' "double"`,
			command: "cmd",
			args:    []string{"single", "double"},
		},
		{
			name:    "nested quotes",
			cmdLine: `cmd "outer 'inner' text"`,
			command: "cmd",
			args:    []string{"outer 'inner' text"},
		},
		{
			name:    "escaped quotes in double quotes",
			cmdLine: `cmd "arg with \"escaped\" quotes"`,
			command: "cmd",
			args:    []string{`arg with "escaped" quotes`},
		},
		{
			name:    "escaped backslash",
			cmdLine: `cmd "path\\to\\file"`,
			command: "cmd",
			args:    []string{`path\to\file`},
		},
		{
			name:    "conda run-python example",
			cmdLine: `__conda run-python base "import os; print(os.getenv('abc'))"`,
			command: "__conda",
			args:    []string{"run-python", "base", "import os; print(os.getenv('abc'))"},
		},
		{
			name:    "multiple spaces",
			cmdLine: "cmd   arg1    arg2",
			command: "cmd",
			args:    []string{"arg1", "arg2"},
		},
		{
			name:    "leading and trailing spaces",
			cmdLine: "  cmd arg1  ",
			command: "cmd",
			args:    []string{"arg1"},
		},
		{
			name:    "empty string",
			cmdLine: "",
			command: "",
			args:    nil,
		},
		{
			name:    "only spaces",
			cmdLine: "   ",
			command: "",
			args:    nil,
		},
		{
			name:    "quoted empty string",
			cmdLine: `cmd "" arg`,
			command: "cmd",
			args:    []string{"", "arg"},
		},
		{
			name:    "backslash escape outside quotes",
			cmdLine: `cmd arg\ with\ spaces`,
			command: "cmd",
			args:    []string{"arg with spaces"},
		},
		{
			name:    "newline in quoted string",
			cmdLine: `cmd "line1\nline2"`,
			command: "cmd",
			args:    []string{"line1\nline2"},
		},
		{
			name:    "tab in quoted string",
			cmdLine: `cmd "arg\twith\ttabs"`,
			command: "cmd",
			args:    []string{"arg\twith\ttabs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, args := ParseCommandLine(tt.cmdLine)
			if command != tt.command {
				t.Errorf("command = %q, want %q", command, tt.command)
			}
			if len(args) != len(tt.args) {
				t.Errorf("args length = %d, want %d", len(args), len(tt.args))
			}
			for i, arg := range args {
				if i >= len(tt.args) {
					t.Errorf("extra arg[%d] = %q", i, arg)
					continue
				}
				if arg != tt.args[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.args[i])
				}
			}
		})
	}
}

func TestQuoteArg(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		expected string
	}{
		{
			name:     "simple arg",
			arg:      "hello",
			expected: "hello",
		},
		{
			name:     "arg with spaces",
			arg:      "arg with spaces",
			expected: `"arg with spaces"`,
		},
		{
			name:     "arg with quotes",
			arg:      `arg"with"quotes`,
			expected: `"arg\"with\"quotes"`,
		},
		{
			name:     "arg with backslash",
			arg:      `path\to\file`,
			expected: `"path\\to\\file"`,
		},
		{
			name:     "empty string",
			arg:      "",
			expected: `""`,
		},
		{
			name:     "arg with tab",
			arg:      "arg\twith\ttabs",
			expected: `"arg	with	tabs"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QuoteArg(tt.arg)
			if result != tt.expected {
				t.Errorf("QuoteArg(%q) = %q, want %q", tt.arg, result, tt.expected)
			}
		})
	}
}

func TestJoinCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		expected string
	}{
		{
			name:     "simple command",
			command:  "echo",
			args:     []string{"hello"},
			expected: `echo hello`,
		},
		{
			name:     "command with quoted arg",
			command:  "cmd",
			args:     []string{"arg with spaces"},
			expected: `cmd "arg with spaces"`,
		},
		{
			name:     "multiple args",
			command:  "cmd",
			args:     []string{"arg1", "arg2", "arg with spaces"},
			expected: `cmd arg1 arg2 "arg with spaces"`,
		},
		{
			name:     "conda example",
			command:  "__conda",
			args:     []string{"run-python", "base", "import os; print(os.getenv('abc'))"},
			expected: `__conda run-python base "import os; print(os.getenv('abc'))"`,
		},
		{
			name:     "empty command",
			command:  "",
			args:     []string{"arg"},
			expected: ``,
		},
		{
			name:     "no args",
			command:  "ls",
			args:     []string{},
			expected: `ls`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinCommandLine(tt.command, tt.args)
			if result != tt.expected {
				t.Errorf("JoinCommandLine(%q, %v) = %q, want %q", tt.command, tt.args, result, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that parsing and joining produces the same result
	testCases := []string{
		`__conda run-python base "import os; print(os.getenv('abc'))"`,
		`cmd "arg with spaces"`,
		`cmd 'single quoted' "double quoted"`,
		`cmd arg1 arg2 arg3`,
		`cmd "arg with \"escaped\" quotes"`,
	}

	for _, cmdLine := range testCases {
		t.Run(cmdLine, func(t *testing.T) {
			command, args := ParseCommandLine(cmdLine)
			reconstructed := JoinCommandLine(command, args)
			// Parse again to verify
			command2, args2 := ParseCommandLine(reconstructed)
			if command != command2 {
				t.Errorf("command mismatch: %q != %q", command, command2)
			}
			if len(args) != len(args2) {
				t.Errorf("args length mismatch: %d != %d", len(args), len(args2))
			}
			for i := range args {
				if args[i] != args2[i] {
					t.Errorf("args[%d] mismatch: %q != %q", i, args[i], args2[i])
				}
			}
		})
	}
}
