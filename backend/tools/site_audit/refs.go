package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func newUserProcessRefs() userProcessRefs {
	return userProcessRefs{
		ids:                       map[string]bool{},
		userProcessFields:         map[string][]string{},
		userFieldNonEmptySelector: map[string]map[string]bool{},
		userFieldEmptySelector:    map[string]map[string]bool{},
		userFieldMissingSelector:  map[string]map[string]bool{},
		userFieldFixedText:        map[string]map[string]string{},
	}
}

func loadDepilerRefs(dir string) depilerRefs {
	out := newUserProcessRefs()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ts") {
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(e.Name(), ".ts"))
		out.ids[id] = true
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		raw := string(data)
		if fields := extractManagerUserProcessFields(raw); len(fields) > 0 {
			out.userProcessFields[id] = fields
			for _, field := range fields {
				noteUserFieldSelectorState(&out, id, field, managerUserFieldSelectorState(raw, field))
				if text, ok := managerUserFieldFixedText(raw, field); ok {
					noteUserFieldFixedText(&out, id, field, text)
				}
			}
		}
	}
	return out
}

func loadManagerRefs(dir string) managerRefs {
	out := newUserProcessRefs()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(e.Name(), ".js"))
		out.ids[id] = true
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		raw := string(data)
		if fields := extractManagerUserProcessFields(raw); len(fields) > 0 {
			out.userProcessFields[id] = fields
			for _, field := range fields {
				noteUserFieldSelectorState(&out, id, field, managerUserFieldSelectorState(raw, field))
				if text, ok := managerUserFieldFixedText(raw, field); ok {
					noteUserFieldFixedText(&out, id, field, text)
				}
			}
		}
	}
	return out
}

func loadMoviePilotRefs(root string) moviePilotRefs {
	out := moviePilotRefs{
		root:          root,
		parserSchemas: map[string]string{},
		parserFiles:   map[string]string{},
		spiderParsers: map[string]string{},
	}
	if strings.TrimSpace(root) == "" {
		out.err = "moviepilot path is empty"
		return out
	}
	parserInit := filepath.Join(root, "app", "modules", "indexer", "parser", "__init__.py")
	data, err := os.ReadFile(parserInit)
	if err != nil {
		out.err = err.Error()
		return out
	}
	out.available = true
	out.parserSchemas = extractMoviePilotSiteSchemas(string(data))

	parserDir := filepath.Dir(parserInit)
	if entries, err := os.ReadDir(parserDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") || e.Name() == "__init__.py" {
				continue
			}
			path := filepath.Join(parserDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			for _, schema := range extractMoviePilotParserSchemaAssignments(string(data)) {
				out.parserFiles[schema] = e.Name()
			}
		}
	}

	indexerInit := filepath.Join(root, "app", "modules", "indexer", "__init__.py")
	if data, err := os.ReadFile(indexerInit); err == nil {
		out.spiderParsers = extractMoviePilotSpiderParsers(string(data))
	}
	return out
}

func extractMoviePilotSiteSchemas(raw string) map[string]string {
	out := map[string]string{}
	body := pythonClassBody(raw, "SiteSchema")
	re := regexp.MustCompile(`(?m)^\s+([A-Za-z][A-Za-z0-9_]*)\s*=\s*["']([^"']+)["']`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		out[m[1]] = m[2]
	}
	return out
}

func extractMoviePilotParserSchemaAssignments(raw string) []string {
	re := regexp.MustCompile(`(?m)^\s*schema\s*=\s*SiteSchema\.([A-Za-z][A-Za-z0-9_]*)\b`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func extractMoviePilotSpiderParsers(raw string) map[string]string {
	out := map[string]string{}
	re := regexp.MustCompile(`SPIDER_PARSER_CLASSES\s*=\s*\{`)
	loc := re.FindStringIndex(raw)
	if loc == nil {
		return out
	}
	block, ok := jsObjectBody(raw, loc[1]-1)
	if !ok {
		return out
	}
	pairRe := regexp.MustCompile(`["']([^"']+)["']\s*:\s*([A-Za-z_][A-Za-z0-9_]*)`)
	for _, m := range pairRe.FindAllStringSubmatch(block, -1) {
		if len(m) < 3 {
			continue
		}
		out[m[1]] = m[2]
	}
	return out
}

func pythonClassBody(raw, className string) string {
	lines := strings.Split(raw, "\n")
	start := -1
	prefix := "class " + className
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	var body []string
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasPrefix(trimmed, "class ") {
			break
		}
		body = append(body, line)
	}
	return strings.Join(body, "\n")
}

func extractManagerUserProcessFields(raw string) []string {
	loc := managerUserInfoRe.FindStringIndex(raw)
	if loc == nil {
		return nil
	}
	block, ok := jsObjectBody(raw, loc[1]-1)
	if !ok {
		return nil
	}
	if !strings.Contains(block, "process") {
		return nil
	}
	processBlock := block
	if procLoc := regexp.MustCompile(`(?s)\bprocess\s*:\s*\[`).FindStringIndex(block); procLoc != nil {
		if body, ok := jsArrayBody(block, procLoc[1]-1); ok {
			processBlock = body
		}
	}
	seen := map[string]bool{}
	var out []string
	add := func(field string) {
		if field == "" || seen[field] {
			return
		}
		seen[field] = true
		out = append(out, field)
	}
	for _, arr := range managerFieldsArrayRe.FindAllStringSubmatch(processBlock, -1) {
		if len(arr) < 2 {
			continue
		}
		for _, m := range managerQuotedFieldRe.FindAllStringSubmatch(arr[1], -1) {
			if len(m) < 2 {
				continue
			}
			add(m[1])
		}
	}
	for _, selectors := range jsNamedObjectBodies(processBlock, "selectors") {
		for _, key := range jsObjectPropertyKeys(selectors) {
			add(key)
		}
	}
	sort.Strings(out)
	return out
}

func managerUserFieldHasNonEmptySelector(raw, field string) bool {
	return managerUserFieldSelectorState(raw, field) == userFieldSelectorNonEmpty
}

func managerUserFieldSelectorState(raw, field string) userFieldSelectorState {
	fieldRe := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(field) + `\s*:\s*\{`)
	loc := fieldRe.FindStringIndex(raw)
	if loc == nil {
		return userFieldSelectorMissing
	}
	block, ok := jsObjectBody(raw, loc[1]-1)
	if !ok {
		return userFieldSelectorMissing
	}
	m := regexp.MustCompile(`(?s)\bselector\s*:\s*(\[[\s\S]*?\]|"[^"]*"|'[^']*'|undefined|null)`).FindStringSubmatch(block)
	if len(m) < 2 {
		if regexp.MustCompile(`(?s)\bselector\s*:`).MatchString(block) {
			return userFieldSelectorNonEmpty
		}
		return userFieldSelectorMissing
	}
	value := strings.TrimSpace(m[1])
	if value == "[]" || value == "undefined" || value == "null" {
		return userFieldSelectorEmpty
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")) == "" {
			return userFieldSelectorEmpty
		}
		return userFieldSelectorNonEmpty
	}
	if strings.Trim(strings.TrimSpace(value), `"'`) == "" {
		return userFieldSelectorEmpty
	}
	return userFieldSelectorNonEmpty
}

func managerUserFieldFixedText(raw, field string) (string, bool) {
	fieldRe := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(field) + `\s*:\s*\{`)
	loc := fieldRe.FindStringIndex(raw)
	if loc == nil {
		return "", false
	}
	block, ok := jsObjectBody(raw, loc[1]-1)
	if !ok {
		return "", false
	}
	m := regexp.MustCompile(`(?s)\btext\s*:\s*("([^"\\]|\\.)*"|'([^'\\]|\\.)*'|-?\d+(?:\.\d+)?|true|false|null)`).FindStringSubmatch(block)
	if len(m) < 2 {
		return "", false
	}
	value := strings.TrimSpace(m[1])
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\'`, `'`)
		value = strings.ReplaceAll(value, `\\`, `\`)
	}
	return value, true
}

func noteUserFieldSelectorState(refs *userProcessRefs, id, field string, state userFieldSelectorState) {
	switch state {
	case userFieldSelectorNonEmpty:
		if refs.userFieldNonEmptySelector[id] == nil {
			refs.userFieldNonEmptySelector[id] = map[string]bool{}
		}
		refs.userFieldNonEmptySelector[id][field] = true
	case userFieldSelectorEmpty:
		if refs.userFieldEmptySelector[id] == nil {
			refs.userFieldEmptySelector[id] = map[string]bool{}
		}
		refs.userFieldEmptySelector[id][field] = true
	case userFieldSelectorMissing:
		if refs.userFieldMissingSelector[id] == nil {
			refs.userFieldMissingSelector[id] = map[string]bool{}
		}
		refs.userFieldMissingSelector[id][field] = true
	}
}

func noteUserFieldFixedText(refs *userProcessRefs, id, field, text string) {
	if refs.userFieldFixedText[id] == nil {
		refs.userFieldFixedText[id] = map[string]string{}
	}
	refs.userFieldFixedText[id][field] = text
}
