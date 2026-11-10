// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"strings"
)

func matchingJSDelimiter(raw string, open int, left, right byte) (int, bool) {
	depth := 0
	inString := byte(0)
	escaped := false
	inLineComment := false
	inBlockComment := false
	inRegex := false
	inRegexClass := false
	for i := open; i < len(raw); i++ {
		ch := raw[i]
		next := byte(0)
		if i+1 < len(raw) {
			next = raw[i+1]
		}
		if inLineComment {
			if ch == '\n' || ch == '\r' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if inRegex {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '[':
				inRegexClass = true
			case ']':
				inRegexClass = false
			case '/':
				if !inRegexClass {
					inRegex = false
				}
			}
			continue
		}
		switch ch {
		case '/':
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
			if looksLikeRegexStart(raw, i) {
				inRegex = true
				inRegexClass = false
				continue
			}
		case '"', '\'', '`':
			inString = ch
			continue
		}
		if ch == left {
			depth++
			continue
		}
		if ch == right {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func skipJSWhitespaceAndComments(raw string, i int) int {
	for i < len(raw) {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\n' || raw[i] == '\r') {
			i++
		}
		if i+1 >= len(raw) || raw[i] != '/' {
			return i
		}
		if raw[i+1] == '/' {
			i += 2
			for i < len(raw) && raw[i] != '\n' && raw[i] != '\r' {
				i++
			}
			continue
		}
		if raw[i+1] == '*' {
			i += 2
			for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
				i++
			}
			if i+1 < len(raw) {
				i += 2
			}
			continue
		}
		return i
	}
	return i
}

func atJSPropertyBoundary(raw string, i int) bool {
	if i == 0 {
		return true
	}
	for j := i - 1; j >= 0; j-- {
		ch := raw[j]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			continue
		}
		return ch == ',' || ch == '{' || ch == '['
	}
	return true
}

func readJSPropertyName(raw string, i int) (string, int, bool) {
	if i >= len(raw) {
		return "", i, false
	}
	if raw[i] == '"' || raw[i] == '\'' {
		quote := raw[i]
		var b strings.Builder
		escaped := false
		for j := i + 1; j < len(raw); j++ {
			ch := raw[j]
			if escaped {
				b.WriteByte(ch)
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				return b.String(), j + 1, true
			}
			b.WriteByte(ch)
		}
		return "", i, false
	}
	if !isJSIdentStart(raw[i]) {
		return "", i, false
	}
	j := i + 1
	for j < len(raw) && isJSIdentPart(raw[j]) {
		j++
	}
	return raw[i:j], j, true
}

func isJSIdentStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_' || ch == '$'
}

func isJSIdentPart(ch byte) bool {
	return isJSIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func looksLikeRegexStart(raw string, slash int) bool {
	for i := slash - 1; i >= 0; i-- {
		ch := raw[i]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			continue
		}
		return strings.ContainsRune("([{=,:;!&|?", rune(ch))
	}
	return true
}
