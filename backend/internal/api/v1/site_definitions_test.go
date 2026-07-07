package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestSiteDefinitionsForUIDedupesByHostAndKeepsRicherDefinition(t *testing.T) {
	defs := []*site.SiteMetadata{
		{
			ID:     "pterclub",
			Name:   "Cat",
			Schema: "nexusphp",
			URLs:   []string{"https://pterclub.net"},
		},
		{
			ID:                "pter",
			Name:              "PTerClub",
			Schema:            "nexusphp",
			URLs:              []string{"https://pterclub.net/"},
			LegacyURLs:        []string{"https://pter.club/", "https://www.pterclub.net"},
			AKA:               []string{"Cat"},
			Categories:        []site.CategoryDef{{ID: "401", Name: "Movie"}},
			LevelRequirements: []site.LevelRequirement{{ID: 1, Name: "User"}},
		},
		{
			ID:     "other",
			Name:   "Other",
			Schema: "nexusphp",
			URLs:   []string{"https://other.example"},
		},
	}

	got := siteDefinitionsForUI(defs)
	if len(got) != 2 {
		t.Fatalf("got %d definitions, want 2", len(got))
	}
	var pter *site.SiteMetadata
	for _, d := range got {
		if d.ID == "pter" {
			pter = d
			break
		}
	}
	if pter == nil {
		t.Fatalf("richer pter definition was not kept: %#v", got)
	}
	if len(pter.URLs) != 2 {
		t.Fatalf("merged urls = %#v, want canonical host plus legacy domain", pter.URLs)
	}
	if pter.URLs[0] != "https://pterclub.net/" || pter.URLs[1] != "https://pter.club/" {
		t.Fatalf("merged urls = %#v", pter.URLs)
	}
}

func TestSiteDefinitionsForUIRealDefinitionsDedupKnownDuplicateHosts(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}

	got := siteDefinitionsForUI(reg.Definitions())
	byID := map[string]*site.SiteMetadata{}
	for _, d := range got {
		byID[d.ID] = d
	}

	for _, duplicate := range []string{"ptchdbits", "pterclub", "ssd"} {
		if _, ok := byID[duplicate]; ok {
			t.Fatalf("duplicate placeholder %s should be merged out of UI definitions", duplicate)
		}
	}

	assertDefinitionURL(t, byID, "chdbits", "https://ptchdbits.co/")
	assertDefinitionURL(t, byID, "chdbits", "https://chdbits.co/")
	assertDefinitionURL(t, byID, "pter", "https://pterclub.net/")
	assertDefinitionURL(t, byID, "pter", "https://pter.club/")
	assertDefinitionURL(t, byID, "pthome", "https://pthome.net")
	assertDefinitionName(t, byID, "pthome", "铂金家")
	assertDefinitionAKA(t, byID, "pthome", "PTHome")
	assertDefinitionAKA(t, byID, "pthome", "PT之家")
	assertDefinitionAKA(t, byID, "pthome", "铂金家")
	assertDefinitionAKA(t, byID, "pthome", "PTH")
	assertDefinitionURL(t, byID, "springsunday", "https://springsunday.net/")
	assertDefinitionURL(t, byID, "springsunday", "https://hdcmct.org/")
}

func TestDefinitionsEndpointIncludesPTHomeCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	NewSiteHandler(nil, nil, reg, nil).Definitions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Definitions []struct {
			ID   string   `json:"id"`
			Name string   `json:"name"`
			URLs []string `json:"urls"`
			AKA  []string `json:"aka"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var pthome *struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		URLs []string `json:"urls"`
		AKA  []string `json:"aka"`
	}
	for i := range body.Definitions {
		if body.Definitions[i].ID == "pthome" {
			pthome = &body.Definitions[i]
			break
		}
	}
	if pthome == nil {
		t.Fatalf("pthome missing from /sites/definitions response")
	}
	if pthome.Name != "铂金家" {
		t.Fatalf("pthome name = %q, want 铂金家", pthome.Name)
	}
	assertStringContains(t, pthome.URLs, "https://pthome.net", "pthome urls")
	assertStringContains(t, pthome.AKA, "PTHome", "pthome aka")
	assertStringContains(t, pthome.AKA, "PT之家", "pthome aka")
}

func assertDefinitionName(t *testing.T, byID map[string]*site.SiteMetadata, id, want string) {
	t.Helper()
	def := byID[id]
	if def == nil {
		t.Fatalf("definition %s missing from UI definitions", id)
	}
	if def.Name != want {
		t.Fatalf("definition %s name = %q, want %q", id, def.Name, want)
	}
}

func assertDefinitionURL(t *testing.T, byID map[string]*site.SiteMetadata, id, want string) {
	t.Helper()
	def := byID[id]
	if def == nil {
		t.Fatalf("definition %s missing from UI definitions", id)
	}
	for _, got := range def.URLs {
		if got == want {
			return
		}
	}
	t.Fatalf("definition %s urls = %#v, want %s", id, def.URLs, want)
}

func assertDefinitionAKA(t *testing.T, byID map[string]*site.SiteMetadata, id, want string) {
	t.Helper()
	def := byID[id]
	if def == nil {
		t.Fatalf("definition %s missing from UI definitions", id)
	}
	for _, got := range def.AKA {
		if got == want {
			return
		}
	}
	t.Fatalf("definition %s aka = %#v, want %s", id, def.AKA, want)
}

func assertStringContains(t *testing.T, got []string, want, label string) {
	t.Helper()
	for _, v := range got {
		if v == want {
			return
		}
	}
	t.Fatalf("%s = %#v, want %s", label, got, want)
}
