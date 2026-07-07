package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
	"gopkg.in/yaml.v3"
)

type auditDef struct {
	path    string
	header  string
	pending bool
	def     site.SiteMetadata
}

type userProcessRefs struct {
	ids                       map[string]bool
	userProcessFields         map[string][]string
	userFieldNonEmptySelector map[string]map[string]bool
	userFieldEmptySelector    map[string]map[string]bool
	userFieldMissingSelector  map[string]map[string]bool
	userFieldFixedText        map[string]map[string]string
}

type managerRefs = userProcessRefs
type depilerRefs = userProcessRefs

type moviePilotRefs struct {
	root          string
	available     bool
	err           string
	parserSchemas map[string]string
	parserFiles   map[string]string
	spiderParsers map[string]string
}

type moviePilotCoverageRow struct {
	key    string
	value  string
	file   string
	status string
	target string
	note   string
}

func main() {
	defsDir := flag.String("defs", "definitions", "PTPatronus definitions directory")
	ptDepilerDir := flag.String("ptdepiler", `D:\PT-depiler-master\src\packages\site\definitions`, "PT-depiler definitions directory")
	ptManagerDir := flag.String("ptmanager", `D:\pt-manager-source-20260705-220320\pt-manager\android\app\src\main\assets\site_defs`, "PT-manager site_defs directory")
	moviePilotDir := flag.String("moviepilot", `D:\MoviePilot-2`, "MoviePilot checkout root")
	outPath := flag.String("out", "", "optional output markdown file")
	flag.Parse()

	defs, err := loadAuditDefs(*defsDir)
	if err != nil {
		fatal(err)
	}
	depiler := loadDepilerRefs(*ptDepilerDir)
	manager := loadManagerRefs(*ptManagerDir)
	moviepilot := loadMoviePilotRefs(*moviePilotDir)
	var b bytes.Buffer
	writeReport(&b, defs, depiler, manager, moviepilot)
	if *outPath != "" {
		if err := os.WriteFile(*outPath, b.Bytes(), 0o644); err != nil {
			fatal(err)
		}
		return
	}
	_, _ = os.Stdout.Write(b.Bytes())
}

func loadAuditDefs(dir string) ([]auditDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]auditDef, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var def site.SiteMetadata
		if err := yaml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if def.ID == "" {
			continue
		}
		header := leadingComments(string(data))
		out = append(out, auditDef{
			path:    path,
			header:  header,
			pending: strings.Contains(header, "calibration: pending"),
			def:     def,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].def.ID < out[j].def.ID })
	return out, nil
}

func refIDs(dir, ext string) map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		out[strings.ToLower(strings.TrimSuffix(e.Name(), ext))] = true
	}
	return out
}

type userFieldSelectorState string

const (
	userFieldSelectorMissing  userFieldSelectorState = "missing"
	userFieldSelectorEmpty    userFieldSelectorState = "empty"
	userFieldSelectorNonEmpty userFieldSelectorState = "non_empty"
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

var (
	managerUserInfoRe     = regexp.MustCompile(`(?s)userInfo\s*:\s*\{`)
	managerFieldsArrayRe  = regexp.MustCompile(`(?s)fields\s*:\s*\[([^\]]*)\]`)
	managerQuotedFieldRe  = regexp.MustCompile(`["']([A-Za-z_][A-Za-z0-9_]*)["']`)
	managerSupportedField = map[string]string{
		"id":                        "id",
		"name":                      "name",
		"username":                  "name",
		"email":                     "email",
		"levelName":                 "level",
		"levelId":                   "level_id",
		"numericId":                 "numeric_user_id",
		"numericUserId":             "numeric_user_id",
		"avatar":                    "avatar",
		"personalAvatar":            "personal_avatar",
		"isDonor":                   "is_donor",
		"uploaded":                  "uploaded",
		"downloaded":                "downloaded",
		"ratio":                     "ratio",
		"trueUploaded":              "true_uploaded",
		"trueDownloaded":            "true_downloaded",
		"trueRatio":                 "true_ratio",
		"seeding":                   "seeding",
		"leeching":                  "leeching",
		"seedingSize":               "seeding_size",
		"seedingTime":               "seeding_time",
		"uploadCount":               "uploads",
		"uploads":                   "uploads",
		"adoptions":                 "adoptions",
		"posts":                     "posts",
		"snatches":                  "snatches",
		"totalTraffic":              "total_traffic",
		"uniqueGroups":              "unique_groups",
		"perfectFLAC":               "perfect_flac",
		"invited":                   "invited",
		"bonus":                     "bonus",
		"bonusPerHour":              "bonus_per_hour",
		"seedingBonus":              "seeding_bonus",
		"seedingBonusPerHour":       "seeding_bonus_per_hour",
		"rescueSeedingBonusPerHour": "rescue_seeding_bonus_per_hour",
		"uploadCountAdd":            "uploads_add",
		"claimCount":                "claim_count",
		"maxClaim":                  "max_claim",
		"joinTime":                  "join_date",
		"joinDate":                  "join_date",
		"lastAccessAt":              "last_access_at",
	}
	managerSpecialCoverage = map[string]map[string]bool{
		"mteam":        setOf("id", "name", "email", "level", "level_id", "avatar", "is_donor", "uploaded", "downloaded", "ratio", "seeding", "leeching", "seeding_size", "uploads", "bonus", "bonus_per_hour", "join_date", "last_access_at"),
		"rousipro":     setOf("id", "name", "email", "level", "level_id", "uploaded", "downloaded", "ratio", "seeding", "leeching", "seeding_size", "bonus", "bonus_per_hour", "seeding_time", "join_date", "last_access_at"),
		"starspace":    setOf("id", "name", "level", "uploaded", "downloaded", "ratio", "true_uploaded", "true_downloaded", "true_ratio", "seeding", "seeding_size", "uploads", "bonus", "bonus_per_hour", "seeding_bonus", "claim_count", "max_claim", "join_date", "last_access_at"),
		"torrentleech": setOf("id", "name", "level", "uploaded", "downloaded", "seeding", "seeding_size", "uploads", "bonus", "join_date", "last_access_at"),
		"zhixing":      setOf("id", "name", "level", "uploaded", "downloaded", "ratio", "seeding", "seeding_size", "uploads", "bonus", "seeding_bonus", "join_date", "last_access_at"),
	}
	managerReferenceFieldClasses = map[string]referenceFieldClass{
		"messageCount": {title: "Existing Capability Fields", note: "covered by the messages capability rather than `site.UserInfo`"},
		"invites":      {title: "Existing Capability Fields", note: "covered by the harem/invite capability rather than `site.UserInfo`"},

		"hnrPreWarning":  {title: "Existing Capability Fields", note: "covered by the H&R capability rather than `site.UserInfo`"},
		"hnrUnsatisfied": {title: "Existing Capability Fields", note: "covered by the H&R capability rather than `site.UserInfo`"},
		"hrCount":        {title: "Existing Capability Fields", note: "covered by the H&R capability rather than `site.UserInfo`"},

		"seedingBonusPerHour": {title: "Deferred UserInfo Fields", note: "reference selector is empty; migrate only site-specific non-empty selectors"},
	}
)

type referenceFieldClass struct {
	title string
	note  string
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

func writeReport(b *bytes.Buffer, defs []auditDef, depiler depilerRefs, manager managerRefs, moviepilot moviePilotRefs) {
	ours := map[string]bool{}
	for _, d := range defs {
		ours[strings.ToLower(d.def.ID)] = true
	}
	pending := filter(defs, func(d auditDef) bool { return d.pending })
	dead := filter(defs, func(d auditDef) bool { return d.def.Dead })
	withUserFields := filter(defs, func(d auditDef) bool {
		return len(d.def.UserInfo.Fields)+len(d.def.UserInfo.DetailFields) > 0
	})
	withListAPI := filter(defs, func(d auditDef) bool {
		return d.def.UserInfo.UserTorrentListAPI != ""
	})

	fmt.Fprintln(b, "# Site Adaptation Audit")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Generated by `go run ./tools/site_audit`.")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "## Summary")
	fmt.Fprintln(b)
	fmt.Fprintf(b, "- PTPatronus definitions: %d\n", len(defs))
	fmt.Fprintf(b, "- PT-depiler reference definitions: %d, missing in PTPatronus: %d\n", len(depiler.ids), countMissing(depiler.ids, ours))
	fmt.Fprintf(b, "- PT-depiler definitions with literal `userInfo.process`: %d\n", len(depiler.userProcessFields))
	fmt.Fprintf(b, "- PT-manager reference definitions: %d, missing in PTPatronus: %d\n", len(manager.ids), countMissing(manager.ids, ours))
	fmt.Fprintf(b, "- PT-manager definitions with `userInfo.process`: %d\n", len(manager.userProcessFields))
	if moviepilot.available {
		parserRows := moviePilotParserCoverageRows(defs, moviepilot)
		spiderRows := moviePilotSpiderCoverageRows(defs, moviepilot)
		pc, pp, pm := moviePilotCoverageCounts(parserRows)
		sc, sp, sm := moviePilotCoverageCounts(spiderRows)
		fmt.Fprintf(b, "- MoviePilot parser schemas: %d, covered: %d, partial: %d, missing: %d\n", len(parserRows), pc, pp, pm)
		fmt.Fprintf(b, "- MoviePilot dedicated spiders: %d, covered: %d, partial: %d, missing: %d\n", len(spiderRows), sc, sp, sm)
	} else {
		fmt.Fprintf(b, "- MoviePilot reference unavailable: %s\n", moviepilot.err)
	}
	fmt.Fprintf(b, "- Dead definitions retained for display/history: %d\n", len(dead))
	fmt.Fprintf(b, "- Definitions with explicit user fields: %d\n", len(withUserFields))
	fmt.Fprintf(b, "- Definitions with `user_torrent_list_api`: %d\n", len(withListAPI))
	fmt.Fprintf(b, "- Definitions still marked `calibration: pending`: %d\n", len(pending))
	fmt.Fprintln(b)

	writeMissing(b, "Missing From PT-depiler", depiler.ids, ours)
	writeMissing(b, "Missing From PT-manager", manager.ids, ours)
	writeExtras(b, defs, depiler.ids, "Extra Versus PT-depiler")
	writeDepilerProcessCoverage(b, defs, depiler)
	writeManagerProcessCoverage(b, defs, manager)
	writeMoviePilotCoverage(b, defs, moviepilot)
	writeSchemaCoverage(b, defs)
	writeDuplicateHosts(b, defs)
	writeMultiDomain(b, defs)
	writeFieldCoverage(b, defs)
	writeSeedingSizeCoverage(b, defs, depiler, manager)
	writePending(b, pending)
}

func writeDepilerProcessCoverage(b *bytes.Buffer, defs []auditDef, depiler depilerRefs) {
	writeReferenceProcessCoverage(b, defs, depiler, "PT-depiler")
}

func writeManagerProcessCoverage(b *bytes.Buffer, defs []auditDef, manager managerRefs) {
	writeReferenceProcessCoverage(b, defs, manager, "PT-manager")
}

func writeMoviePilotCoverage(b *bytes.Buffer, defs []auditDef, refs moviePilotRefs) {
	fmt.Fprintln(b, "## MoviePilot Parser/Spider Coverage")
	fmt.Fprintln(b)
	if !refs.available {
		fmt.Fprintf(b, "- MoviePilot reference unavailable from `%s`: %s\n", refs.root, refs.err)
		fmt.Fprintln(b)
		return
	}
	fmt.Fprintln(b, "MoviePilot does not expose local YAML site definition files in this checkout; its useful reference surface is `app/modules/indexer/parser` for user data and `app/modules/indexer/spider` for dedicated search. This section maps those parser/spider names to PTPatronus executable schemas, definitions, or dedicated adapters.")
	fmt.Fprintln(b)

	parserRows := moviePilotParserCoverageRows(defs, refs)
	pc, pp, pm := moviePilotCoverageCounts(parserRows)
	fmt.Fprintf(b, "- Parser schemas found: %d, covered: %d, partial: %d, missing: %d\n", len(parserRows), pc, pp, pm)
	fmt.Fprintln(b)
	fmt.Fprintln(b, "### Parser Schemas")
	fmt.Fprintln(b)
	if len(parserRows) == 0 {
		fmt.Fprintln(b, "- None found.")
	} else {
		for _, row := range parserRows {
			file := row.file
			if file == "" {
				file = "enum only"
			}
			fmt.Fprintf(b, "- `%s` (`%s`, `%s`): **%s** via %s", row.key, row.value, file, row.status, row.target)
			if row.note != "" {
				fmt.Fprintf(b, " — %s", row.note)
			}
			fmt.Fprintln(b)
		}
	}
	fmt.Fprintln(b)

	spiderRows := moviePilotSpiderCoverageRows(defs, refs)
	sc, sp, sm := moviePilotCoverageCounts(spiderRows)
	fmt.Fprintf(b, "- Dedicated spiders found: %d, covered: %d, partial: %d, missing: %d\n", len(spiderRows), sc, sp, sm)
	fmt.Fprintln(b)
	fmt.Fprintln(b, "### Dedicated Search Spiders")
	fmt.Fprintln(b)
	if len(spiderRows) == 0 {
		fmt.Fprintln(b, "- None found.")
	} else {
		for _, row := range spiderRows {
			fmt.Fprintf(b, "- `%s` (`%s`): **%s** via %s", row.key, row.value, row.status, row.target)
			if row.note != "" {
				fmt.Fprintf(b, " — %s", row.note)
			}
			fmt.Fprintln(b)
		}
	}
	fmt.Fprintln(b)
}

func moviePilotParserCoverageRows(defs []auditDef, refs moviePilotRefs) []moviePilotCoverageRow {
	keys := sortedStringMapKeys(refs.parserSchemas)
	rows := make([]moviePilotCoverageRow, 0, len(keys))
	for _, key := range keys {
		status, target, note := moviePilotParserCoverage(defs, key)
		rows = append(rows, moviePilotCoverageRow{
			key:    key,
			value:  refs.parserSchemas[key],
			file:   refs.parserFiles[key],
			status: status,
			target: target,
			note:   note,
		})
	}
	return rows
}

func moviePilotSpiderCoverageRows(defs []auditDef, refs moviePilotRefs) []moviePilotCoverageRow {
	keys := sortedStringMapKeys(refs.spiderParsers)
	rows := make([]moviePilotCoverageRow, 0, len(keys))
	for _, key := range keys {
		status, target, note := moviePilotSpiderCoverage(defs, key)
		rows = append(rows, moviePilotCoverageRow{
			key:    key,
			value:  refs.spiderParsers[key],
			status: status,
			target: target,
			note:   note,
		})
	}
	return rows
}

func moviePilotCoverageCounts(rows []moviePilotCoverageRow) (covered, partial, missing int) {
	for _, row := range rows {
		switch row.status {
		case "covered":
			covered++
		case "partial":
			partial++
		default:
			missing++
		}
	}
	return covered, partial, missing
}

func moviePilotParserCoverage(defs []auditDef, schema string) (status, target, note string) {
	switch schema {
	case "DiscuzX":
		return coveredBySchema(defs, "discuz", "GenericHTML adapter covers Discuz-style definitions.")
	case "Gazelle":
		return coveredBySchema(defs, "gazelle", "Gazelle adapter covers grouped search, user.php profile fields, and configured pages.")
	case "Ipt":
		return coveredByDefinitions(defs, []string{"iptorrents"}, "IPTorrents has an executable definition with user.php/peers user-data pages.")
	case "NexusPhp":
		return coveredBySchema(defs, "nexusphp", "NexusPHP adapter covers default user discovery, detail page fields, and seeding-size list APIs.")
	case "NexusProject":
		status, target, _ = coveredBySchema(defs, "nexusphp", "")
		return status, target, "NexusProject is NexusPHP-compatible; per-site viewusertorrents.php flows are carried by definitions where needed."
	case "NexusRabbit":
		return coveredByDefinitions(defs, []string{"hares"}, "Hares/NexusRabbit user.php selectors were migrated from MoviePilot and are covered by mock tests.")
	case "NexusHhanclub":
		return coveredByDefinitions(defs, []string{"hhanclub"}, "HhanClub has site-specific user_info fields and bonus/rescue pages.")
	case "NexusAudiences":
		return coveredByDefinitions(defs, []string{"audiences"}, "Audiences custom skin user fields are represented in YAML and tests.")
	case "SmallHorse":
		return "covered", fmt.Sprintf("schema `%s` framework defaults (%d concrete definition(s))", "smallhorse", countDefinitionsBySchema(defs, "smallhorse")), "MoviePilot exposes only the framework parser locally; PTPatronus now has executable smallhorse defaults for user.php stats and table#torrent_table seeding-size pages, while concrete site/domain mapping remains reference-dependent."
	case "Unit3d":
		return coveredBySchema(defs, "unit3d", "Unit3D adapter covers Laravel/Unit3D search and user-data APIs/pages.")
	case "TorrentLeech":
		return coveredByDefinitions(defs, []string{"torrentleech"}, "TorrentLeech has definition-level search plus a dedicated user-info helper.")
	case "FileList":
		return coveredBySchema(defs, "filelist", "FileList is handled by GenericHTML with executable definition selectors.")
	case "TNode":
		return coveredBySchema(defs, "tnode", "TNode/Zhuque JSON search and user-data pages are handled by GenericJSON.")
	case "MTorrent":
		return coveredBySchema(defs, "mteam", "MoviePilot MTorrent maps to the M-Team API; PTPatronus uses the dedicated mteam adapter.")
	case "Yema":
		return coveredBySchema(defs, "yemapt", "YemaPT API flow is handled by the yemapt GenericJSON schema.")
	case "HDDolby":
		return coveredByDefinitions(defs, []string{"hddolby"}, "HDDolby has site-specific NexusPHP selectors and user_torrent_list_api coverage.")
	case "Zhixing":
		return coveredByDefinitions(defs, []string{"zhixing"}, "Zhixing uses the cgbtsource definition plus a dedicated user-info helper.")
	case "Bitpt":
		return coveredByDefinitions(defs, []string{"bitpt"}, "BitPT has executable custom selectors and user_info pages.")
	case "RousiPro":
		return coveredBySchemaAndDefinitions(defs, "rousi", []string{"rousipro"}, "Rousi Pro API has a dedicated adapter and definition.")
	default:
		return "missing", "`unmapped`", "No MoviePilot-to-PTPatronus mapping is defined yet."
	}
}

func moviePilotSpiderCoverage(defs []auditDef, parser string) (status, target, note string) {
	switch parser {
	case "TNodeSpider":
		return coveredBySchemaAndDefinitions(defs, "tnode", []string{"zhuque"}, "MoviePilot TNodeSpider corresponds to Zhuque/TNode JSON search.")
	case "TorrentLeech":
		return coveredByDefinitions(defs, []string{"torrentleech"}, "TorrentLeech search is represented by the executable JSON/HTML definition.")
	case "mTorrent":
		return coveredBySchemaAndDefinitions(defs, "mteam", []string{"mteam"}, "MoviePilot mTorrent spider maps to the dedicated M-Team API adapter.")
	case "Yema":
		return coveredBySchemaAndDefinitions(defs, "yemapt", []string{"yemapt"}, "YemaPT search uses the GenericJSON yemapt schema.")
	case "Haidan":
		return coveredByDefinitions(defs, []string{"haidan"}, "Haidan has custom search selectors and NexusPHP user-info defaults.")
	case "HDDolby":
		return coveredByDefinitions(defs, []string{"hddolby"}, "HDDolby has custom search/user-info selectors.")
	case "RousiPro":
		return coveredBySchemaAndDefinitions(defs, "rousi", []string{"rousipro"}, "Rousi Pro search is handled by the dedicated API adapter.")
	default:
		return "missing", "`unmapped`", "No MoviePilot spider mapping is defined yet."
	}
}

func coveredBySchema(defs []auditDef, schema, note string) (status, target, outNote string) {
	count := countDefinitionsBySchema(defs, schema)
	target = fmt.Sprintf("schema `%s` (%d definition(s))", schema, count)
	if count == 0 {
		return "missing", target, note
	}
	return "covered", target, note
}

func coveredByDefinitions(defs []auditDef, ids []string, note string) (status, target, outNote string) {
	target = "definition " + backtickJoin(ids)
	missing := missingDefinitionIDs(defs, ids)
	if len(missing) > 0 {
		return "missing", target, "missing " + backtickJoin(missing) + noteSuffix(note)
	}
	return "covered", target, note
}

func coveredBySchemaAndDefinitions(defs []auditDef, schema string, ids []string, note string) (status, target, outNote string) {
	schemaCount := countDefinitionsBySchema(defs, schema)
	target = fmt.Sprintf("schema `%s` (%d definition(s)) + definition %s", schema, schemaCount, backtickJoin(ids))
	missing := missingDefinitionIDs(defs, ids)
	if schemaCount == 0 || len(missing) > 0 {
		parts := []string{}
		if schemaCount == 0 {
			parts = append(parts, "schema `"+schema+"`")
		}
		if len(missing) > 0 {
			parts = append(parts, backtickJoin(missing))
		}
		return "missing", target, "missing " + strings.Join(parts, ", ") + noteSuffix(note)
	}
	return "covered", target, note
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return "; " + note
}

func countDefinitionsBySchema(defs []auditDef, schema string) int {
	count := 0
	for _, d := range defs {
		if strings.EqualFold(d.def.Schema, schema) {
			count++
		}
	}
	return count
}

func missingDefinitionIDs(defs []auditDef, ids []string) []string {
	have := map[string]bool{}
	for _, d := range defs {
		have[strings.ToLower(d.def.ID)] = true
	}
	var missing []string
	for _, id := range ids {
		if !have[strings.ToLower(id)] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}

func writeReferenceProcessCoverage(b *bytes.Buffer, defs []auditDef, refs userProcessRefs, source string) {
	processFields := refs.userProcessFields
	defByID := map[string]site.SiteMetadata{}
	for _, d := range defs {
		defByID[strings.ToLower(d.def.ID)] = d.def
	}
	type missingRow struct {
		id     string
		fields []string
	}
	var missing []missingRow
	unsupported := map[string][]string{}
	selectorless := map[string][]string{}
	fixedText := map[string][]string{}
	fixedTextValues := map[string]map[string][]string{}
	special := []string{}
	for id, fields := range processFields {
		if _, ok := managerSpecialCoverage[id]; ok {
			special = append(special, id)
		}
		var miss []string
		for _, refField := range fields {
			internal, supported := managerSupportedField[refField]
			if !supported {
				unsupported[refField] = append(unsupported[refField], id)
				continue
			}
			if managerFieldCovered(defByID[id], id, internal) {
				continue
			}
			if !refs.userFieldNonEmptySelector[id][refField] {
				if text, ok := refs.userFieldFixedText[id][refField]; ok {
					fixedText[refField] = append(fixedText[refField], id)
					if fixedTextValues[refField] == nil {
						fixedTextValues[refField] = map[string][]string{}
					}
					fixedTextValues[refField][text] = append(fixedTextValues[refField][text], id)
					continue
				}
			}
			if !refs.userFieldNonEmptySelector[id][refField] && (refs.userFieldEmptySelector[id][refField] || refs.userFieldMissingSelector[id][refField]) {
				selectorless[refField] = append(selectorless[refField], id)
				continue
			}
			miss = append(miss, refField+"->"+internal)
		}
		if len(miss) > 0 {
			sort.Strings(miss)
			missing = append(missing, missingRow{id: id, fields: miss})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].id < missing[j].id })
	sort.Strings(special)

	fmt.Fprintf(b, "## %s UserInfo Process Coverage\n", source)
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Supported reference fields are mapped to the current `site.UserInfo` JSON fields. YAML fields and schema adapter-derived fields, such as NexusPHP UID discovery and uploaded-torrent counts, are both counted as executable coverage; reference-only fields are tracked separately until the product model stores them.")
	fmt.Fprintln(b)
	fmt.Fprintf(b, "- Definitions with %s `userInfo.process`: %d\n", source, len(processFields))
	fmt.Fprintf(b, "- Dedicated adapter definitions counted as covered: %s\n", backtickJoin(special))
	fmt.Fprintf(b, "- Missing supported process fields: %d\n", len(missing))
	fmt.Fprintf(b, "- Fixed non-selector reference fields: %d site-field(s)\n", countReferenceFieldSites(fixedText))
	fmt.Fprintf(b, "- Reference fields without executable selectors: %d site-field(s)\n", countReferenceFieldSites(selectorless))
	fmt.Fprintln(b)
	fmt.Fprintln(b, "### Missing Supported Process Fields")
	fmt.Fprintln(b)
	if len(missing) == 0 {
		fmt.Fprintln(b, "- None.")
	} else {
		for _, row := range missing {
			fmt.Fprintf(b, "- `%s`: %s\n", row.id, backtickJoin(row.fields))
		}
	}
	fmt.Fprintln(b)

	fmt.Fprintln(b, "### Reference Fields With Fixed Non-Selector Values")
	fmt.Fprintln(b)
	if len(fixedText) == 0 {
		fmt.Fprintln(b, "- None.")
	} else {
		fmt.Fprintf(b, "These fields are named by %s `userInfo.process`, but the reference provides a fixed `text` value instead of a site selector. Numeric zero values are already represented by `site.UserInfo` zero defaults unless a site-specific selector exists; non-data sentinels such as `N/A` are not migrated into numeric fields.\n", source)
		fmt.Fprintln(b)
		for _, field := range sortedStringKeys(fixedText) {
			ids := unique(fixedText[field])
			examples := ids
			if len(examples) > 8 {
				examples = examples[:8]
			}
			suffix := ""
			if len(ids) > len(examples) {
				suffix = fmt.Sprintf(" (+%d more)", len(ids)-len(examples))
			}
			fmt.Fprintf(b, "- `%s`: %d site(s), e.g. %s%s%s\n", field, len(ids), backtickJoin(examples), suffix, fixedTextValueSummary(fixedTextValues[field]))
		}
	}
	fmt.Fprintln(b)

	fmt.Fprintln(b, "### Reference Fields Without Executable Selectors")
	fmt.Fprintln(b)
	if len(selectorless) == 0 {
		fmt.Fprintln(b, "- None.")
	} else {
		fmt.Fprintf(b, "These fields are named by %s `userInfo.process`, but the reference definition either leaves the selector empty or does not define one. They are not actionable YAML migrations until a site-specific selector exists upstream or is live-calibrated.\n", source)
		fmt.Fprintln(b)
		for _, field := range sortedStringKeys(selectorless) {
			ids := unique(selectorless[field])
			examples := ids
			if len(examples) > 8 {
				examples = examples[:8]
			}
			suffix := ""
			if len(ids) > len(examples) {
				suffix = fmt.Sprintf(" (+%d more)", len(ids)-len(examples))
			}
			empty, missing := selectorlessStateCounts(refs, field, ids)
			detail := ""
			if empty+missing > 0 {
				detail = fmt.Sprintf(" (empty selector: %d, missing selector: %d)", empty, missing)
			}
			fmt.Fprintf(b, "- `%s`: %d site(s), e.g. %s%s%s\n", field, len(ids), backtickJoin(examples), suffix, detail)
		}
	}
	fmt.Fprintln(b)

	fmt.Fprintln(b, "### Reference Fields Outside Supported Coverage")
	fmt.Fprintln(b)
	if len(unsupported) == 0 {
		fmt.Fprintln(b, "- None.")
		fmt.Fprintln(b)
		return
	}
	fmt.Fprintf(b, "These fields came from %s `userInfo.process`, but are not counted as supported UserInfo coverage yet. They are grouped by destination so existing capabilities and intentionally deferred fields are not mistaken for missing executable selectors.\n", source)
	fmt.Fprintln(b)
	grouped := groupReferenceFields(unsupported)
	for _, title := range sortedReferenceGroupTitles(grouped) {
		fmt.Fprintf(b, "#### %s\n\n", title)
		for _, field := range sortedStringKeys(grouped[title]) {
			ids := unique(grouped[title][field])
			examples := ids
			if len(examples) > 8 {
				examples = examples[:8]
			}
			suffix := ""
			if len(ids) > len(examples) {
				suffix = fmt.Sprintf(" (+%d more)", len(ids)-len(examples))
			}
			note := referenceFieldNote(field)
			if note != "" {
				note = " — " + note
			}
			fmt.Fprintf(b, "- `%s`: %d site(s), e.g. %s%s%s\n", field, len(ids), backtickJoin(examples), suffix, note)
		}
		fmt.Fprintln(b)
	}
}

func selectorlessStateCounts(refs userProcessRefs, field string, ids []string) (empty int, missing int) {
	for _, id := range ids {
		if refs.userFieldEmptySelector[id][field] {
			empty++
			continue
		}
		if refs.userFieldMissingSelector[id][field] {
			missing++
		}
	}
	return empty, missing
}

func countReferenceFieldSites(fields map[string][]string) int {
	total := 0
	for _, ids := range fields {
		total += len(unique(ids))
	}
	return total
}

func fixedTextValueSummary(values map[string][]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := sortedStringKeys(values)
	parts := make([]string, 0, len(keys))
	for _, value := range keys {
		parts = append(parts, fmt.Sprintf("`%s` x%d", value, len(unique(values[value]))))
	}
	return " (fixed text: " + strings.Join(parts, ", ") + ")"
}

func groupReferenceFields(fields map[string][]string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for field, ids := range fields {
		class := referenceFieldClassFor(field)
		if out[class.title] == nil {
			out[class.title] = map[string][]string{}
		}
		out[class.title][field] = ids
	}
	return out
}

func referenceFieldClassFor(field string) referenceFieldClass {
	if class, ok := managerReferenceFieldClasses[field]; ok {
		return class
	}
	return referenceFieldClass{
		title: "No Current Product Model",
		note:  "not currently represented by `site.UserInfo` or another product capability",
	}
}

func referenceFieldNote(field string) string {
	return referenceFieldClassFor(field).note
}

func managerFieldCovered(def site.SiteMetadata, id, field string) bool {
	if special := managerSpecialCoverage[id]; special[field] {
		return true
	}
	if explicitUserFieldCovered(def, field) {
		return true
	}
	// Empty schema definitions run through GenericHTML at runtime. GenericHTML does
	// not execute NexusPHP's ajax user-torrent fallback, so defaults injected only
	// for backward-compatible selectors must not be counted as process coverage.
	if def.Schema != "" {
		withDefaults := *site.EnsureDefaults(&def)
		if explicitUserFieldCovered(withDefaults, field) {
			return true
		}
	}
	if managerAdapterFieldCovered(def, field) {
		return true
	}
	return false
}

func managerAdapterFieldCovered(def site.SiteMetadata, field string) bool {
	switch def.Schema {
	case "nexusphp":
		switch field {
		case "id":
			// NexusPHP.FetchUserInfo discovers the current user id from userdetails links.
			return true
		case "seeding", "seeding_size":
			// fetchSeeding reads getusertorrentlistajax/getusertorrentlist type=seeding.
			return true
		case "uploads":
			// fetchUploadsCount reads getusertorrentlistajax/getusertorrentlist type=uploaded.
			return true
		case "claim_count", "max_claim":
			// fetchClaimCounts reads claim.php and parses "认领 x/y" style summaries.
			return true
		}
	case "gazelle", "luminance":
		// Gazelle/Luminance FetchUserInfo discovers uid from the index page before
		// parsing user.php?id=<uid> and configured pages.
		return field == "id"
	}
	return false
}

func writeMissing(b *bytes.Buffer, title string, ref, ours map[string]bool) {
	missing := missingIDs(ref, ours)
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(missing) == 0 {
		fmt.Fprintln(b, "- None.")
		fmt.Fprintln(b)
		return
	}
	for _, id := range missing {
		fmt.Fprintf(b, "- `%s`\n", id)
	}
	fmt.Fprintln(b)
}

func writeExtras(b *bytes.Buffer, defs []auditDef, ref map[string]bool, title string) {
	var extra []string
	for _, d := range defs {
		id := strings.ToLower(d.def.ID)
		if !ref[id] {
			extra = append(extra, d.def.ID)
		}
	}
	sort.Strings(extra)
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(extra) == 0 {
		fmt.Fprintln(b, "- None.")
	} else {
		for _, id := range extra {
			fmt.Fprintf(b, "- `%s`\n", id)
		}
	}
	fmt.Fprintln(b)
}

func writeSchemaCoverage(b *bytes.Buffer, defs []auditDef) {
	counts := map[string]int{}
	for _, d := range defs {
		sc := d.def.Schema
		if sc == "" {
			sc = "(empty/generic)"
		}
		counts[sc]++
	}
	keys := sortedKeys(counts)
	fmt.Fprintln(b, "## Schema Coverage")
	fmt.Fprintln(b)
	for _, k := range keys {
		fmt.Fprintf(b, "- `%s`: %d\n", k, counts[k])
	}
	fmt.Fprintln(b)
}

func writeDuplicateHosts(b *bytes.Buffer, defs []auditDef) {
	hosts := map[string][]string{}
	for _, d := range defs {
		for _, h := range defHosts(d.def) {
			hosts[h] = append(hosts[h], d.def.ID)
		}
	}
	var dupHosts []string
	for h, ids := range hosts {
		if len(unique(ids)) > 1 {
			dupHosts = append(dupHosts, h)
		}
	}
	sort.Strings(dupHosts)
	fmt.Fprintln(b, "## Duplicate Host Groups")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Note: `/sites/definitions` merges these host groups for the add-site UI and keeps the richer definition with all known domains.")
	fmt.Fprintln(b)
	if len(dupHosts) == 0 {
		fmt.Fprintln(b, "- None.")
		fmt.Fprintln(b)
		return
	}
	for _, h := range dupHosts {
		fmt.Fprintf(b, "- `%s`: %s\n", h, backtickJoin(unique(hosts[h])))
	}
	fmt.Fprintln(b)
}

func writeMultiDomain(b *bytes.Buffer, defs []auditDef) {
	var rows []string
	for _, d := range defs {
		hosts := unique(defHosts(d.def))
		if len(hosts) > 1 {
			rows = append(rows, fmt.Sprintf("- `%s`: %s", d.def.ID, backtickJoin(hosts)))
		}
	}
	sort.Strings(rows)
	fmt.Fprintln(b, "## Multi-domain Definitions")
	fmt.Fprintln(b)
	if len(rows) == 0 {
		fmt.Fprintln(b, "- None.")
	} else {
		for _, row := range rows {
			fmt.Fprintln(b, row)
		}
	}
	fmt.Fprintln(b)
}

func writeFieldCoverage(b *bytes.Buffer, defs []auditDef) {
	fields := []string{"name", "level", "level_id", "numeric_user_id", "avatar", "personal_avatar", "is_donor", "uploaded", "downloaded", "ratio", "true_uploaded", "true_downloaded", "true_ratio", "seeding", "leeching", "seeding_size", "uploads", "uploads_add", "adoptions", "posts", "snatches", "total_traffic", "unique_groups", "perfect_flac", "invited", "bonus", "bonus_per_hour", "seeding_bonus", "seeding_bonus_per_hour", "rescue_seeding_bonus_per_hour", "seeding_time", "claim_count", "max_claim", "join_date", "last_access_at"}
	counts := map[string]int{}
	for _, d := range defs {
		seen := map[string]bool{}
		for k := range d.def.UserInfo.Fields {
			seen[k] = true
		}
		for k := range d.def.UserInfo.DetailFields {
			seen[k] = true
		}
		for _, page := range d.def.UserInfo.Pages {
			for _, k := range page.UseFields {
				seen[k] = true
			}
			for k := range page.Fields {
				seen[k] = true
			}
		}
		for _, f := range fields {
			if seen[f] {
				counts[f]++
			}
		}
	}
	fmt.Fprintln(b, "## Explicit User Field Coverage")
	fmt.Fprintln(b)
	for _, f := range fields {
		fmt.Fprintf(b, "- `%s`: %d\n", f, counts[f])
	}
	fmt.Fprintln(b)
}

func writeSeedingSizeCoverage(b *bytes.Buffer, defs []auditDef, depiler depilerRefs, manager managerRefs) {
	type row struct {
		id       string
		schema   string
		ref      bool
		refKnown bool
		userInfo bool
	}
	counts := map[string]int{}
	var missingActive []row
	totalActive := 0
	coveredActive := 0
	for _, d := range defs {
		reason := seedingSizeCoverageReason(d.def)
		if d.def.Dead {
			if reason == "" {
				counts["dead/no executable reference"]++
			} else {
				counts["dead/"+reason]++
			}
			continue
		}
		totalActive++
		if reason == "" {
			ref := refsRequestField(depiler, manager, d.def.ID, "seedingSize")
			refKnown := refsKnowSite(depiler, manager, d.def.ID)
			userInfo := hasUserInfoCapability(d.def)
			missingActive = append(missingActive, row{id: d.def.ID, schema: d.def.Schema, ref: ref, refKnown: refKnown, userInfo: userInfo})
			if ref {
				counts["active/no evidence/reference requests seedingSize"]++
			} else if userInfo && refKnown {
				counts["active/no evidence/reference checked without seedingSize source"]++
			} else if userInfo {
				counts["active/no evidence/user_info without checked reference"]++
			} else if refKnown {
				counts["active/no evidence/reference checked without user_info capability"]++
			} else {
				counts["active/no evidence/no user_info capability"]++
			}
			continue
		}
		coveredActive++
		counts[reason]++
	}
	sort.Slice(missingActive, func(i, j int) bool { return missingActive[i].id < missingActive[j].id })

	fmt.Fprintln(b, "## Seeding Size Coverage")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "This section tracks effective evidence for `seeding_size` separately from raw YAML field counts. NexusPHP coverage is counted only for explicit `schema: nexusphp` definitions because empty-schema definitions run through GenericHTML at runtime.")
	fmt.Fprintln(b)
	fmt.Fprintf(b, "- Active definitions with seeding-size evidence: %d / %d\n", coveredActive, totalActive)
	for _, k := range sortedKeys(counts) {
		fmt.Fprintf(b, "- `%s`: %d\n", k, counts[k])
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "### Active Definitions Without Seeding-Size Evidence")
	fmt.Fprintln(b)
	if len(missingActive) == 0 {
		fmt.Fprintln(b, "- None.")
		fmt.Fprintln(b)
		return
	}
	fmt.Fprintln(b, "#### Reference Requests `seedingSize`")
	fmt.Fprintln(b)
	wrote := false
	for _, row := range missingActive {
		if !row.ref {
			continue
		}
		schema := row.schema
		if schema == "" {
			schema = "(empty/generic)"
		}
		fmt.Fprintf(b, "- `%s` (`%s`)\n", row.id, schema)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "- None.")
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "#### Reference Checked Without `seedingSize` Source")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "These definitions have executable user-info pages in PTPatronus, and the matching PT-depiler/PT-manager reference definition does not provide an executable `seedingSize` selector/API. Keep them visible for live calibration, but do not treat them as pending migrations from the checked references.")
	fmt.Fprintln(b)
	wrote = false
	for _, row := range missingActive {
		if row.ref || !row.refKnown || !row.userInfo {
			continue
		}
		schema := row.schema
		if schema == "" {
			schema = "(empty/generic)"
		}
		fmt.Fprintf(b, "- `%s` (`%s`)\n", row.id, schema)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "- None.")
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "#### User-Info Capability Without Checked Reference")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "These definitions expose user-info pages locally, but no matching PT-depiler/PT-manager reference definition was available in this audit. They need live calibration or another reference before `seeding_size` can be added safely.")
	fmt.Fprintln(b)
	wrote = false
	for _, row := range missingActive {
		if row.ref || row.refKnown || !row.userInfo {
			continue
		}
		schema := row.schema
		if schema == "" {
			schema = "(empty/generic)"
		}
		fmt.Fprintf(b, "- `%s` (`%s`)\n", row.id, schema)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "- None.")
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "#### Reference Checked Without User-Info Capability")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "These definitions do not expose executable user-info pages or fields, and the matching PT-depiler/PT-manager reference also does not provide a user-data flow. Most are public/search-only definitions; `seeding_size` is not actionable unless live calibration finds authenticated user pages.")
	fmt.Fprintln(b)
	wrote = false
	for _, row := range missingActive {
		if row.ref || row.userInfo || !row.refKnown {
			continue
		}
		schema := row.schema
		if schema == "" {
			schema = "(empty/generic)"
		}
		fmt.Fprintf(b, "- `%s` (`%s`)\n", row.id, schema)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "- None.")
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "#### No User-Info Capability Configured")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "These definitions do not currently expose executable user-info pages or fields, and no checked PT-depiler/PT-manager reference definition was available in this audit.")
	fmt.Fprintln(b)
	wrote = false
	for _, row := range missingActive {
		if row.ref || row.userInfo || row.refKnown {
			continue
		}
		schema := row.schema
		if schema == "" {
			schema = "(empty/generic)"
		}
		fmt.Fprintf(b, "- `%s` (`%s`)\n", row.id, schema)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "- None.")
	}
	fmt.Fprintln(b)
}

func refsKnowSite(depiler depilerRefs, manager managerRefs, id string) bool {
	id = strings.ToLower(id)
	return depiler.ids[id] || manager.ids[id]
}

func refsRequestField(depiler depilerRefs, manager managerRefs, id, field string) bool {
	id = strings.ToLower(id)
	for _, refs := range []userProcessRefs{depiler, manager} {
		for _, f := range refs.userProcessFields[id] {
			if f == field {
				return true
			}
		}
	}
	return false
}

func hasUserInfoCapability(def site.SiteMetadata) bool {
	ui := def.UserInfo
	return strings.TrimSpace(ui.Path) != "" ||
		strings.TrimSpace(ui.DetailPath) != "" ||
		strings.TrimSpace(ui.UserTorrentListAPI) != "" ||
		len(ui.Fields) > 0 ||
		len(ui.DetailFields) > 0 ||
		len(ui.Pages) > 0
}

func seedingSizeCoverageReason(def site.SiteMetadata) string {
	if explicitUserFieldCovered(def, "seeding_size") {
		return "explicit YAML field/page"
	}
	if dedicatedSeedingSizeHelper(def.ID) {
		return "dedicated helper"
	}
	switch strings.ToLower(def.Schema) {
	case "nexusphp":
		if strings.TrimSpace(def.UserInfo.UserTorrentListAPI) != "" {
			return "nexusphp user_torrent_list_api"
		}
		return "nexusphp default list API"
	case "mteam", "rousi", "avistaz", "gazelle_jsonapi":
		return "schema adapter API/profile"
	case "unit3d", "gazelle", "luminance", "rartracker", "f3nix", "fsm", "filelist", "discuz", "tbsource", "tcg", "xbtit", "aidoruonline", "smallhorse", "cgbtsource", "meantorrent", "tbdev", "tnode", "yemapt":
		if explicitUserFieldCovered(*site.EnsureDefaults(&def), "seeding_size") {
			return "schema default/user_info selectors"
		}
	}
	return ""
}

func explicitUserFieldCovered(def site.SiteMetadata, field string) bool {
	if _, ok := def.UserInfo.Fields[field]; ok {
		return true
	}
	if _, ok := def.UserInfo.DetailFields[field]; ok {
		return true
	}
	for _, page := range def.UserInfo.Pages {
		if _, ok := page.Fields[field]; ok {
			return true
		}
		for _, key := range page.UseFields {
			if key == field {
				return true
			}
		}
	}
	return false
}

func dedicatedSeedingSizeHelper(id string) bool {
	switch strings.ToLower(id) {
	case "aidoruonline", "pussytorrents", "skyeysnow", "starspace", "torrentleech", "zhixing":
		return true
	default:
		return false
	}
}

func writePending(b *bytes.Buffer, pending []auditDef) {
	fmt.Fprintln(b, "## Pending Calibration")
	fmt.Fprintln(b)
	if len(pending) == 0 {
		fmt.Fprintln(b, "- None.")
		fmt.Fprintln(b)
		return
	}
	for _, d := range pending {
		todos := headerTodos(d.header)
		if len(todos) == 0 {
			fmt.Fprintf(b, "- `%s`: live smoke required\n", d.def.ID)
			continue
		}
		fmt.Fprintf(b, "- `%s`: %s\n", d.def.ID, strings.Join(todos, "; "))
	}
	fmt.Fprintln(b)
}

func defHosts(def site.SiteMetadata) []string {
	var out []string
	for _, raw := range append(append([]string{}, def.URLs...), def.LegacyURLs...) {
		h := normalizedHost(raw)
		if h != "" {
			out = append(out, h)
		}
	}
	return unique(out)
}

func normalizedHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func leadingComments(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			lines = append(lines, line)
			continue
		}
		break
	}
	return strings.Join(lines, "\n")
}

func headerTodos(header string) []string {
	var out []string
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if strings.HasPrefix(line, "- ") {
			out = append(out, strings.TrimPrefix(line, "- "))
		}
	}
	return out
}

func missingIDs(ref, ours map[string]bool) []string {
	var out []string
	for id := range ref {
		if !ours[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func countMissing(ref, ours map[string]bool) int {
	return len(missingIDs(ref, ours))
}

func filter(in []auditDef, fn func(auditDef) bool) []auditDef {
	var out []auditDef
	for _, d := range in {
		if fn(d) {
			out = append(out, d)
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedReferenceGroupTitles(m map[string]map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func setOf(vals ...string) map[string]bool {
	out := map[string]bool{}
	for _, v := range vals {
		out[v] = true
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func backtickJoin(in []string) string {
	parts := make([]string, 0, len(in))
	for _, s := range in {
		parts = append(parts, "`"+s+"`")
	}
	return strings.Join(parts, ", ")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
