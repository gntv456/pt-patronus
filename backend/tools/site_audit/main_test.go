// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

func TestExtractManagerUserProcessFields(t *testing.T) {
	got := extractManagerUserProcessFields(`
g.__ptd_definitions.demo = {
  userInfo: {
    selectors: {},
    process: [
      { fields: ["id", "name", "uploadCount"] },
      { fields: ['seedingSize', 'bonusPerHour'] }
    ]
  },
  inviteInfo: {}
}`)
	want := []string{"bonusPerHour", "id", "name", "seedingSize", "uploadCount"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestExtractManagerUserProcessFieldsStopsAtUserInfoObject(t *testing.T) {
	got := extractManagerUserProcessFields(`
g.__ptd_definitions.demo = {
    userInfo: {
      selectors: {
        avatar: { selector: "img[title='{avatar}']" },
        weird: { elementProcess: function(el) { return el ? "{ok}" : ""; } }
      },
      process: [
        { fields: ["id", "name", "messageCount"] }
      ]
    },
  inviteInfo: {
    process: [
      { fields: ["inviteCount", "invitees"] }
    ]
  }
}`)
	want := []string{"id", "messageCount", "name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestExtractManagerUserProcessFieldsIncludesProcessSelectors(t *testing.T) {
	got := extractManagerUserProcessFields(`
export const siteMetadata = {
  userInfo: {
    selectors: {
      unusedGlobal: { selector: "#global" }
    },
    process: [
      {
        requestConfig: { url: "/index.php" },
        selectors: {
          id: { selector: "a[href*='user.php']" },
          "name": { selector: ".name" },
          seedingSize: { text: "N/A" }
        }
      },
      { fields: ["uploaded"] }
    ]
  }
}`)
	want := []string{"id", "name", "seedingSize", "uploaded"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestManagerUserFieldHasNonEmptySelector(t *testing.T) {
	raw := `
g.__ptd_definitions.demo = {
  userInfo: {
    selectors: {
      empty: { text: 0, selector: [] },
      blank: { selector: "" },
      missing: { text: 0 },
      nilish: { selector: null },
      fixed: { selector: ":self", case: "html" },
      sentinel: { text: "N/A" },
      expression: { selector: labelTrans.map((x) => "dt:contains('" + x + "') + dd") },
      list: { selector: ["tbody tr:first td:nth-child(9)"] }
    }
  }
}`
	if managerUserFieldHasNonEmptySelector(raw, "empty") {
		t.Fatal("empty selector should not count as migratable coverage")
	}
	if managerUserFieldSelectorState(raw, "blank") != userFieldSelectorEmpty {
		t.Fatal("blank selector should be classified as empty")
	}
	if managerUserFieldSelectorState(raw, "missing") != userFieldSelectorMissing {
		t.Fatal("missing selector should be classified as missing")
	}
	if managerUserFieldSelectorState(raw, "nilish") != userFieldSelectorEmpty {
		t.Fatal("null selector should be classified as empty")
	}
	if !managerUserFieldHasNonEmptySelector(raw, "fixed") {
		t.Fatal(":self selector should count as non-empty")
	}
	if !managerUserFieldHasNonEmptySelector(raw, "expression") {
		t.Fatal("selector expression should count as non-empty")
	}
	if !managerUserFieldHasNonEmptySelector(raw, "list") {
		t.Fatal("selector array should count as non-empty")
	}
	if got, ok := managerUserFieldFixedText(raw, "sentinel"); !ok || got != "N/A" {
		t.Fatalf("fixed text = %q, %v; want N/A, true", got, ok)
	}
	if got, ok := managerUserFieldFixedText(raw, "empty"); !ok || got != "0" {
		t.Fatalf("zero fixed text = %q, %v; want 0, true", got, ok)
	}
}

func TestReferenceProcessCoverageSeparatesSelectorlessFields(t *testing.T) {
	defs := []auditDef{{def: site.SiteMetadata{ID: "demo", Schema: "nexusphp"}}}
	refs := newUserProcessRefs()
	refs.userProcessFields["demo"] = []string{"seedingBonusPerHour"}
	refs.userFieldEmptySelector["demo"] = map[string]bool{"seedingBonusPerHour": true}

	var buf bytes.Buffer
	writeReferenceProcessCoverage(&buf, defs, refs, "PT-manager")
	report := buf.String()
	if !strings.Contains(report, "- Missing supported process fields: 0") {
		t.Fatalf("selectorless field should not be counted missing:\n%s", report)
	}
	if !strings.Contains(report, "- Reference fields without executable selectors: 1 site-field(s)") {
		t.Fatalf("selectorless summary missing:\n%s", report)
	}
	if !strings.Contains(report, "### Reference Fields Without Executable Selectors") {
		t.Fatalf("selectorless section missing:\n%s", report)
	}
	if !strings.Contains(report, "`seedingBonusPerHour`: 1 site(s), e.g. `demo`") {
		t.Fatalf("selectorless field row missing:\n%s", report)
	}
	if strings.Contains(report, "`demo`: `seedingBonusPerHour->seeding_bonus_per_hour`") {
		t.Fatalf("selectorless field should not be listed as an actionable missing selector:\n%s", report)
	}
}

func TestReferenceProcessCoverageKeepsNonEmptySelectorsActionable(t *testing.T) {
	defs := []auditDef{{def: site.SiteMetadata{ID: "demo", Schema: "nexusphp"}}}
	refs := newUserProcessRefs()
	refs.userProcessFields["demo"] = []string{"seedingBonusPerHour"}
	refs.userFieldNonEmptySelector["demo"] = map[string]bool{"seedingBonusPerHour": true}

	var buf bytes.Buffer
	writeReferenceProcessCoverage(&buf, defs, refs, "PT-manager")
	report := buf.String()
	if !strings.Contains(report, "- Missing supported process fields: 1") {
		t.Fatalf("non-empty selector should remain actionable missing coverage:\n%s", report)
	}
	if !strings.Contains(report, "- Reference fields without executable selectors: 0 site-field(s)") {
		t.Fatalf("non-empty selector should not count as selectorless:\n%s", report)
	}
	if !strings.Contains(report, "`demo`: `seedingBonusPerHour->seeding_bonus_per_hour`") {
		t.Fatalf("non-empty selector should be listed as missing coverage:\n%s", report)
	}
	if strings.Contains(report, "`seedingBonusPerHour`: 1 site(s), e.g. `demo`") {
		t.Fatalf("non-empty selector should not be listed as selectorless:\n%s", report)
	}
}

func TestReferenceProcessCoverageSeparatesFixedTextFields(t *testing.T) {
	defs := []auditDef{{def: site.SiteMetadata{ID: "demo", Schema: ""}}}
	refs := newUserProcessRefs()
	refs.userProcessFields["demo"] = []string{"bonus"}
	refs.userFieldMissingSelector["demo"] = map[string]bool{"bonus": true}
	refs.userFieldFixedText["demo"] = map[string]string{"bonus": "N/A"}

	var buf bytes.Buffer
	writeReferenceProcessCoverage(&buf, defs, refs, "PT-depiler")
	report := buf.String()
	if !strings.Contains(report, "### Reference Fields With Fixed Non-Selector Values") {
		t.Fatalf("fixed-text section missing:\n%s", report)
	}
	if !strings.Contains(report, "- Fixed non-selector reference fields: 1 site-field(s)") {
		t.Fatalf("fixed-text summary missing:\n%s", report)
	}
	if !strings.Contains(report, "`bonus`: 1 site(s), e.g. `demo` (fixed text: `N/A` x1)") {
		t.Fatalf("fixed-text row missing:\n%s", report)
	}
	if strings.Contains(report, "empty selector: 0, missing selector: 1") {
		t.Fatalf("fixed-text field should not be listed as selectorless:\n%s", report)
	}
}

func TestManagerFieldCoveredCountsAdapterDerivedFields(t *testing.T) {
	nexus := site.SiteMetadata{ID: "nexus", Schema: "nexusphp"}
	for _, field := range []string{"id", "seeding", "seeding_size", "uploads"} {
		if !managerFieldCovered(nexus, nexus.ID, field) {
			t.Fatalf("nexus adapter field %q should be covered", field)
		}
	}

	gazelle := site.SiteMetadata{ID: "gazelle", Schema: "gazelle"}
	if !managerFieldCovered(gazelle, gazelle.ID, "id") {
		t.Fatal("gazelle adapter id should be covered")
	}
	if managerFieldCovered(gazelle, gazelle.ID, "leeching") {
		t.Fatal("gazelle leeching should require YAML/pages coverage")
	}

	generic := site.SiteMetadata{ID: "generic", Schema: ""}
	if managerFieldCovered(generic, generic.ID, "seeding_size") {
		t.Fatal("empty schema uses GenericHTML at runtime and should require YAML/pages or a dedicated helper")
	}

	for id, fields := range map[string][]string{
		"rousipro":     {"id", "level_id", "last_access_at", "seeding_time", "bonus_per_hour"},
		"starspace":    {"id", "last_access_at", "seeding_bonus", "seeding_size", "claim_count", "max_claim"},
		"torrentleech": {"id", "last_access_at", "uploads"},
		"zhixing":      {"id", "last_access_at", "seeding_bonus", "seeding_size"},
	} {
		def := site.SiteMetadata{ID: id}
		for _, field := range fields {
			if !managerFieldCovered(def, id, field) {
				t.Fatalf("%s dedicated helper field %q should be covered", id, field)
			}
		}
	}
}

func TestSeedingSizeCoverageReasonMatchesRuntimeEvidence(t *testing.T) {
	if got := seedingSizeCoverageReason(site.SiteMetadata{ID: "nexus", Schema: "nexusphp"}); got != "nexusphp default list API" {
		t.Fatalf("nexus reason = %q", got)
	}
	if got := seedingSizeCoverageReason(site.SiteMetadata{ID: "blank", Schema: ""}); got != "" {
		t.Fatalf("empty schema should not count as NexusPHP runtime coverage, got %q", got)
	}
	if got := seedingSizeCoverageReason(site.SiteMetadata{ID: "torrentleech", Schema: ""}); got != "dedicated helper" {
		t.Fatalf("dedicated helper reason = %q", got)
	}
	if got := seedingSizeCoverageReason(site.SiteMetadata{
		ID:     "page",
		Schema: "filelist",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
			Fields: map[string]site.ElementQuery{"seeding_size": {Selector: ".size"}},
		}}},
	}); got != "explicit YAML field/page" {
		t.Fatalf("explicit page reason = %q", got)
	}
}

func TestSeedingSizeCoverageSeparatesUserInfoCapability(t *testing.T) {
	defs := []auditDef{
		{def: site.SiteMetadata{ID: "public", Schema: ""}},
		{def: site.SiteMetadata{ID: "publicref", Schema: ""}},
		{def: site.SiteMetadata{
			ID: "profile",
			UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
				Fields: map[string]site.ElementQuery{"uploaded": {Selector: ".uploaded"}},
			}}},
		}},
		{def: site.SiteMetadata{
			ID: "localonly",
			UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
				Fields: map[string]site.ElementQuery{"uploaded": {Selector: ".uploaded"}},
			}}},
		}},
		{def: site.SiteMetadata{ID: "refsite", Schema: ""}},
		{def: site.SiteMetadata{ID: "covered", Schema: "nexusphp"}},
	}
	depiler := newUserProcessRefs()
	depiler.ids["profile"] = true
	depiler.ids["publicref"] = true
	depiler.userProcessFields["refsite"] = []string{"seedingSize"}

	var buf bytes.Buffer
	writeSeedingSizeCoverage(&buf, defs, depiler, newUserProcessRefs())
	report := buf.String()

	for _, want := range []string{
		"`active/no evidence/reference requests seedingSize`: 1",
		"`active/no evidence/reference checked without seedingSize source`: 1",
		"`active/no evidence/user_info without checked reference`: 1",
		"`active/no evidence/reference checked without user_info capability`: 1",
		"`active/no evidence/no user_info capability`: 1",
		"#### Reference Requests `seedingSize`",
		"- `refsite` (`(empty/generic)`)",
		"#### Reference Checked Without `seedingSize` Source",
		"- `profile` (`(empty/generic)`)",
		"#### User-Info Capability Without Checked Reference",
		"- `localonly` (`(empty/generic)`)",
		"#### Reference Checked Without User-Info Capability",
		"- `publicref` (`(empty/generic)`)",
		"#### No User-Info Capability Configured",
		"- `public` (`(empty/generic)`)",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestReferenceFieldClassification(t *testing.T) {
	if got := managerSupportedField["username"]; got != "name" {
		t.Fatalf("username maps to %q, want name", got)
	}
	if got := managerSupportedField["levelId"]; got != "level_id" {
		t.Fatalf("levelId maps to %q, want level_id", got)
	}
	if got := managerSupportedField["numericUserId"]; got != "numeric_user_id" {
		t.Fatalf("numericUserId maps to %q, want numeric_user_id", got)
	}
	if got := managerSupportedField["avatar"]; got != "avatar" {
		t.Fatalf("avatar maps to %q, want avatar", got)
	}
	if got := managerSupportedField["personalAvatar"]; got != "personal_avatar" {
		t.Fatalf("personalAvatar maps to %q, want personal_avatar", got)
	}
	if got := managerSupportedField["isDonor"]; got != "is_donor" {
		t.Fatalf("isDonor maps to %q, want is_donor", got)
	}
	if got := managerSupportedField["trueUploaded"]; got != "true_uploaded" {
		t.Fatalf("trueUploaded maps to %q, want true_uploaded", got)
	}
	if got := managerSupportedField["seedingBonus"]; got != "seeding_bonus" {
		t.Fatalf("seedingBonus maps to %q, want seeding_bonus", got)
	}
	if got := managerSupportedField["seedingBonusPerHour"]; got != "seeding_bonus_per_hour" {
		t.Fatalf("seedingBonusPerHour maps to %q, want seeding_bonus_per_hour", got)
	}
	if got := managerSupportedField["claimCount"]; got != "claim_count" {
		t.Fatalf("claimCount maps to %q, want claim_count", got)
	}
	if got := managerSupportedField["maxClaim"]; got != "max_claim" {
		t.Fatalf("maxClaim maps to %q, want max_claim", got)
	}
	for _, field := range []string{"messageCount", "invites", "hnrUnsatisfied"} {
		if got := referenceFieldClassFor(field).title; got != "Existing Capability Fields" {
			t.Fatalf("%s class = %q, want Existing Capability Fields", field, got)
		}
	}
}

func TestExtractMoviePilotSiteSchemas(t *testing.T) {
	got := extractMoviePilotSiteSchemas(`
class SiteSchema(Enum):
    DiscuzX = "DiscuzX"
    Ipt = "IPTorrents"
    SmallHorse = "Small Horse"


class SiteParserBase(metaclass=ABCMeta):
    schema = None
    request_mode = "cookie"
`)
	want := map[string]string{
		"DiscuzX":    "DiscuzX",
		"Ipt":        "IPTorrents",
		"SmallHorse": "Small Horse",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schemas = %#v, want %#v", got, want)
	}
}

func TestExtractMoviePilotParserSchemaAssignments(t *testing.T) {
	got := extractMoviePilotParserSchemaAssignments(`
class Demo:
    schema = SiteSchema.NexusPhp

class Demo2:
    schema = SiteSchema.RousiPro
    schema = SiteSchema.RousiPro
`)
	want := []string{"NexusPhp", "RousiPro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assignments = %#v, want %#v", got, want)
	}
}

func TestExtractMoviePilotSpiderParsers(t *testing.T) {
	got := extractMoviePilotSpiderParsers(`
SPIDER_PARSER_CLASSES = {
    "TNodeSpider": TNodeSpider,
    "mTorrent": MTorrentSpider,
    "RousiPro": RousiSpider,
}
`)
	want := map[string]string{
		"TNodeSpider": "TNodeSpider",
		"mTorrent":    "MTorrentSpider",
		"RousiPro":    "RousiSpider",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spiders = %#v, want %#v", got, want)
	}
}

func TestMoviePilotCoverageMapping(t *testing.T) {
	defs := []auditDef{
		{def: site.SiteMetadata{ID: "mteam", Schema: "mteam"}},
		{def: site.SiteMetadata{ID: "hares", Schema: "nexusphp"}},
		{def: site.SiteMetadata{ID: "luminarr", Schema: "luminance"}},
	}
	status, target, _ := moviePilotParserCoverage(defs, "MTorrent")
	if status != "covered" || !strings.Contains(target, "mteam") {
		t.Fatalf("MTorrent coverage = %q via %q, want covered mteam", status, target)
	}
	status, target, _ = moviePilotParserCoverage(defs, "NexusRabbit")
	if status != "covered" || !strings.Contains(target, "hares") {
		t.Fatalf("NexusRabbit coverage = %q via %q, want covered hares", status, target)
	}
	status, target, note := moviePilotParserCoverage(defs, "SmallHorse")
	if status != "covered" || !strings.Contains(target, "smallhorse") || !strings.Contains(note, "executable") {
		t.Fatalf("SmallHorse coverage = %q via %q note %q, want covered smallhorse framework defaults", status, target, note)
	}
	status, _, _ = moviePilotParserCoverage(defs, "UnknownSchema")
	if status != "missing" {
		t.Fatalf("UnknownSchema coverage = %q, want missing", status)
	}
}
