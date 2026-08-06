package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

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
