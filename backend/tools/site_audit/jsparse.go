// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"regexp"
	"sort"
)

func jsObjectBody(raw string, openBrace int) (string, bool) {
	if openBrace < 0 || openBrace >= len(raw) || raw[openBrace] != '{' {
		return "", false
	}
	depth := 0
	inString := byte(0)
	escaped := false
	inLineComment := false
	inBlockComment := false
	inRegex := false
	inRegexClass := false
	for i := openBrace; i < len(raw); i++ {
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
		case '/', '#':
			if ch == '/' && next == '/' {
				inLineComment = true
				i++
				continue
			}
			if ch == '/' && next == '*' {
				inBlockComment = true
				i++
				continue
			}
			if ch == '/' && looksLikeRegexStart(raw, i) {
				inRegex = true
				inRegexClass = false
				continue
			}
		case '"', '\'', '`':
			inString = ch
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[openBrace+1 : i], true
			}
		}
	}
	return "", false
}

func jsArrayBody(raw string, openBracket int) (string, bool) {
	if openBracket < 0 || openBracket >= len(raw) || raw[openBracket] != '[' {
		return "", false
	}
	end, ok := matchingJSDelimiter(raw, openBracket, '[', ']')
	if !ok {
		return "", false
	}
	return raw[openBracket+1 : end], true
}

func jsNamedObjectBodies(raw, name string) []string {
	re := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(name) + `\s*:\s*\{`)
	var out []string
	for _, loc := range re.FindAllStringIndex(raw, -1) {
		if body, ok := jsObjectBody(raw, loc[1]-1); ok {
			out = append(out, body)
		}
	}
	return out
}

func jsObjectPropertyKeys(body string) []string {
	seen := map[string]bool{}
	var keys []string
	depth := 0
	inString := byte(0)
	escaped := false
	inLineComment := false
	inBlockComment := false
	inRegex := false
	inRegexClass := false
	for i := 0; i < len(body); i++ {
		ch := body[i]
		next := byte(0)
		if i+1 < len(body) {
			next = body[i+1]
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

		if depth == 0 {
			skipped := skipJSWhitespaceAndComments(body, i)
			if skipped >= len(body) {
				break
			}
			if skipped != i {
				i = skipped
				ch = body[i]
				next = 0
				if i+1 < len(body) {
					next = body[i+1]
				}
			}
			if atJSPropertyBoundary(body, i) {
				key, after, ok := readJSPropertyName(body, i)
				if ok {
					after = skipJSWhitespaceAndComments(body, after)
					if after < len(body) && body[after] == ':' {
						if !seen[key] {
							seen[key] = true
							keys = append(keys, key)
						}
						i = after
						continue
					}
				}
			}
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
			if looksLikeRegexStart(body, i) {
				inRegex = true
				inRegexClass = false
				continue
			}
		case '"', '\'', '`':
			inString = ch
			continue
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		}
	}
	sort.Strings(keys)
	return keys
}
