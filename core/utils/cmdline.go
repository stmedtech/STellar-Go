package utils

import (
	"strings"
	"unicode"
)

// ParseCommandLine parses a command line string into command and arguments,
// handling quoted strings (single and double quotes) and escaped characters.
//
// Examples:
//   - `cmd arg1 "arg with spaces"` -> ["cmd", "arg1", "arg with spaces"]
//   - `cmd 'single quoted' "double quoted"` -> ["cmd", "single quoted", "double quoted"]
//   - `cmd "arg with \"escaped\" quotes"` -> ["cmd", "arg with \"escaped\" quotes"]
//   - `cmd arg\ with\ spaces` -> ["cmd", "arg with spaces"]
//
// Returns the command (first token) and arguments (remaining tokens).
// If the command line is empty, returns empty strings.
func ParseCommandLine(cmdLine string) (command string, args []string) {
	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return "", nil
	}

	tokens := parseTokens(cmdLine)
	if len(tokens) == 0 {
		return "", nil
	}

	command = tokens[0]
	if len(tokens) > 1 {
		args = tokens[1:]
	} else {
		args = []string{}
	}

	return command, args
}

// parseTokens parses a command line string into tokens, handling quotes and escapes.
func parseTokens(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escapeNext := false

	for i, r := range s {
		if escapeNext {
			// Escaped character - handle escape sequences
			escapeNext = false
			if inSingleQuote {
				// In single quotes, only \' and \\ are special
				if r == '\'' || r == '\\' {
					current.WriteRune(r)
				} else {
					// Other escapes are literal backslash + char
					current.WriteRune('\\')
					current.WriteRune(r)
				}
			} else {
				// In double quotes or outside quotes, handle escape sequences
				switch r {
				case 'n':
					current.WriteRune('\n')
				case 't':
					current.WriteRune('\t')
				case 'r':
					current.WriteRune('\r')
				case '\\':
					current.WriteRune('\\')
				case '"':
					current.WriteRune('"')
				case '\'':
					current.WriteRune('\'')
				default:
					// Unknown escape - treat as literal (e.g., \x -> x)
					current.WriteRune(r)
				}
			}
			continue
		}

		switch r {
		case '\\':
			// Backslash - escape next character
			// Inside single quotes, backslash has no special meaning (except \' and \\)
			if inSingleQuote {
				// Check if next char is a single quote or backslash
				if i+1 < len(s) {
					next := rune(s[i+1])
					if next == '\'' || next == '\\' {
						escapeNext = true
						continue
					}
				}
				// Otherwise, treat backslash literally in single quotes
				current.WriteRune(r)
			} else {
				// Outside quotes or in double quotes, backslash escapes next char
				escapeNext = true
			}

		case '\'':
			// Single quote
			if inDoubleQuote {
				// Inside double quotes, single quote is literal
				current.WriteRune(r)
			} else if inSingleQuote {
				// End of single-quoted string
				inSingleQuote = false
				// Save current token (even if empty) and start new one
				tokens = append(tokens, current.String())
				current.Reset()
			} else {
				// Start of single-quoted string
				inSingleQuote = true
			}

		case '"':
			// Double quote
			if inSingleQuote {
				// Inside single quotes, double quote is literal
				current.WriteRune(r)
			} else if inDoubleQuote {
				// End of double-quoted string
				inDoubleQuote = false
				// Save current token (even if empty) and start new one
				tokens = append(tokens, current.String())
				current.Reset()
			} else {
				// Start of double-quoted string
				inDoubleQuote = true
			}

		case ' ', '\t', '\n', '\r':
			// Whitespace
			if inSingleQuote || inDoubleQuote {
				// Inside quotes, whitespace is literal
				current.WriteRune(r)
			} else {
				// Outside quotes, whitespace separates tokens
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
				// Skip consecutive whitespace
			}

		default:
			// Regular character
			current.WriteRune(r)
		}
	}

	// Add the last token if there's anything left (or if we're still in a quote)
	if current.Len() > 0 || inSingleQuote || inDoubleQuote {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// QuoteArg quotes an argument if it contains spaces or special characters.
// This is useful for displaying commands or reconstructing command lines.
func QuoteArg(arg string) string {
	if arg == "" {
		return `""`
	}

	// Check if quoting is needed
	needsQuoting := false
	for _, r := range arg {
		if unicode.IsSpace(r) || r == '"' || r == '\'' || r == '\\' {
			needsQuoting = true
			break
		}
	}

	if !needsQuoting {
		return arg
	}

	// Use double quotes and escape internal quotes and backslashes
	var result strings.Builder
	result.WriteRune('"')
	for _, r := range arg {
		switch r {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		default:
			result.WriteRune(r)
		}
	}
	result.WriteRune('"')
	return result.String()
}

// JoinCommandLine reconstructs a command line from command and arguments,
// quoting arguments that contain spaces or special characters.
func JoinCommandLine(command string, args []string) string {
	if command == "" {
		return ""
	}

	var result strings.Builder
	result.WriteString(QuoteArg(command))

	for _, arg := range args {
		result.WriteRune(' ')
		result.WriteString(QuoteArg(arg))
	}

	return result.String()
}
