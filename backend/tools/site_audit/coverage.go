package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

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
