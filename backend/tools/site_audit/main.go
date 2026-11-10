// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"bytes"
	"flag"
	"fmt"
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

type userFieldSelectorState string

const (
	userFieldSelectorMissing  userFieldSelectorState = "missing"
	userFieldSelectorEmpty    userFieldSelectorState = "empty"
	userFieldSelectorNonEmpty userFieldSelectorState = "non_empty"
)

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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
