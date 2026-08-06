package skills

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCatalogContainsEightUniqueSkills(t *testing.T) {
	t.Parallel()

	wantCodes := []string{
		"general",
		"pitch_deck",
		"weekly_report",
		"sales_proposal",
		"training",
		"product_launch",
		"consulting",
		"meeting_summary",
	}

	summaries := List()
	if len(summaries) != len(wantCodes) {
		t.Fatalf("List() returned %d skills, want %d", len(summaries), len(wantCodes))
	}

	gotCodes := make([]string, 0, len(summaries))
	seen := make(map[string]struct{}, len(summaries))
	for _, summary := range summaries {
		if _, exists := seen[summary.Code]; exists {
			t.Fatalf("List() returned duplicate skill code %q", summary.Code)
		}
		seen[summary.Code] = struct{}{}
		gotCodes = append(gotCodes, summary.Code)

		if summary.Name == "" || summary.Description == "" {
			t.Fatalf("summary %q must have a name and description", summary.Code)
		}
		if len(summary.PreferredLayouts) == 0 {
			t.Fatalf("summary %q must have a preferred layout", summary.Code)
		}
		if summary.MaxSlides <= 0 {
			t.Fatalf("summary %q MaxSlides = %d, want positive", summary.Code, summary.MaxSlides)
		}

		skill, ok := Resolve(summary.Code)
		if !ok {
			t.Fatalf("Resolve(%q) reported not found", summary.Code)
		}
		if skill.Code != summary.Code {
			t.Fatalf("Resolve(%q) returned code %q", summary.Code, skill.Code)
		}
		if skill.SystemPrompt == "" || skill.OutlineSchema == "" {
			t.Fatalf("skill %q must have a system prompt and outline schema", summary.Code)
		}
	}

	sort.Strings(gotCodes)
	sort.Strings(wantCodes)
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("List() codes = %v, want %v", gotCodes, wantCodes)
	}
}

func TestPublicSummariesDoNotExposeSystemPrompt(t *testing.T) {
	t.Parallel()

	summaries := List()
	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatalf("marshal public summaries: %v", err)
	}
	var publicSummaries []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &publicSummaries); err != nil {
		t.Fatalf("unmarshal public summaries: %v", err)
	}
	if len(publicSummaries) != len(summaries) {
		t.Fatalf("public summary JSON count = %d, want %d", len(publicSummaries), len(summaries))
	}
	for index, summary := range publicSummaries {
		assertExactJSONKeys(t, "public summary", index, summary, "code", "name", "description", "preferredLayouts", "maxSlides")
	}
	for _, summary := range summaries {
		skill, ok := Resolve(summary.Code)
		if !ok {
			t.Fatalf("Resolve(%q) reported not found", summary.Code)
		}
		if strings.Contains(string(encoded), skill.SystemPrompt) || strings.Contains(string(encoded), skill.OutlineSchema) {
			t.Fatalf("public summaries expose server-only configuration for %q", summary.Code)
		}
	}
	for _, fieldName := range []string{"SystemPrompt", "systemPrompt", "OutlineSchema", "outlineSchema"} {
		if strings.Contains(string(encoded), fieldName) {
			t.Fatalf("public summaries expose server-only field name %q", fieldName)
		}
	}
}

func TestResolvedSkillOutlineSchemasMatchOutlineContract(t *testing.T) {
	t.Parallel()

	for _, summary := range List() {
		skill, ok := Resolve(summary.Code)
		if !ok {
			t.Fatalf("Resolve(%q) reported not found", summary.Code)
		}
		if !json.Valid([]byte(skill.OutlineSchema)) {
			t.Fatalf("skill %q has invalid outline schema JSON: %s", summary.Code, skill.OutlineSchema)
		}

		var schema map[string]any
		if err := json.Unmarshal([]byte(skill.OutlineSchema), &schema); err != nil {
			t.Fatalf("unmarshal outline schema for %q: %v", summary.Code, err)
		}
		assertSchemaString(t, summary.Code, schema, "type", "object")
		assertSchemaRequired(t, summary.Code, schema, "title", "pages")

		properties := assertSchemaMap(t, summary.Code, schema, "properties")
		title := assertSchemaMap(t, summary.Code, properties, "title")
		assertSchemaString(t, summary.Code, title, "type", "string")
		pages := assertSchemaMap(t, summary.Code, properties, "pages")
		assertSchemaString(t, summary.Code, pages, "type", "array")

		page := assertSchemaMap(t, summary.Code, pages, "items")
		assertSchemaString(t, summary.Code, page, "type", "object")
		assertSchemaRequired(t, summary.Code, page, "title", "summary", "bullets")
		pageProperties := assertSchemaMap(t, summary.Code, page, "properties")
		for _, key := range []string{"title", "summary"} {
			property := assertSchemaMap(t, summary.Code, pageProperties, key)
			assertSchemaString(t, summary.Code, property, "type", "string")
		}
		bullets := assertSchemaMap(t, summary.Code, pageProperties, "bullets")
		assertSchemaString(t, summary.Code, bullets, "type", "array")
		bullet := assertSchemaMap(t, summary.Code, bullets, "items")
		assertSchemaString(t, summary.Code, bullet, "type", "string")
	}
}

func TestSkillJSONKeepsServerOnlyFieldsPrivate(t *testing.T) {
	t.Parallel()

	for _, summary := range List() {
		skill, ok := Resolve(summary.Code)
		if !ok {
			t.Fatalf("Resolve(%q) reported not found", summary.Code)
		}
		encoded, err := json.Marshal(skill)
		if err != nil {
			t.Fatalf("marshal skill %q: %v", summary.Code, err)
		}
		var encodedSkill map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &encodedSkill); err != nil {
			t.Fatalf("unmarshal skill %q JSON: %v", summary.Code, err)
		}
		assertNoJSONKeys(t, "skill "+summary.Code, encodedSkill, "SystemPrompt", "systemPrompt", "OutlineSchema", "outlineSchema")
		if strings.Contains(string(encoded), skill.SystemPrompt) || strings.Contains(string(encoded), skill.OutlineSchema) {
			t.Fatalf("skill %q JSON exposes server-only configuration", summary.Code)
		}
	}
}

func TestUnknownSkillDoesNotFallback(t *testing.T) {
	t.Parallel()

	skill, ok := Resolve("does_not_exist")
	if ok {
		t.Fatalf("Resolve returned fallback skill %#v for an unknown code", skill)
	}
	if !reflect.DeepEqual(skill, Skill{}) {
		t.Fatalf("Resolve returned %#v for an unknown code, want zero Skill", skill)
	}
}

func TestResolvedSkillsAndListedSummariesCannotMutateCatalog(t *testing.T) {
	t.Parallel()

	firstSummary := List()[0]
	firstSummary.PreferredLayouts[0] = "mutated-layout"
	if List()[0].PreferredLayouts[0] == "mutated-layout" {
		t.Fatal("List() exposed a mutable catalog layout slice")
	}

	firstSkill, ok := Resolve(firstSummary.Code)
	if !ok {
		t.Fatalf("Resolve(%q) reported not found", firstSummary.Code)
	}
	firstSkill.PreferredLayouts[0] = "mutated-layout"
	resolvedAgain, ok := Resolve(firstSummary.Code)
	if !ok {
		t.Fatalf("Resolve(%q) reported not found", firstSummary.Code)
	}
	if resolvedAgain.PreferredLayouts[0] == "mutated-layout" {
		t.Fatal("Resolve() exposed a mutable catalog layout slice")
	}
}

func assertSchemaMap(t *testing.T, code string, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key]
	if !ok {
		t.Fatalf("skill %q schema is missing %q", code, key)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("skill %q schema %q = %T, want object", code, key, value)
	}
	return object
}

func assertSchemaRequired(t *testing.T, code string, schema map[string]any, values ...string) {
	t.Helper()
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("skill %q schema required = %T, want array", code, schema["required"])
	}
	for _, want := range values {
		found := false
		for _, value := range required {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("skill %q schema required = %v, want %q", code, required, want)
		}
	}
}

func assertSchemaString(t *testing.T, code string, schema map[string]any, key, want string) {
	t.Helper()
	if got, ok := schema[key].(string); !ok || got != want {
		t.Fatalf("skill %q schema %q = %v, want %q", code, key, schema[key], want)
	}
}

func assertExactJSONKeys(t *testing.T, label string, index int, object map[string]json.RawMessage, allowed ...string) {
	t.Helper()
	if len(object) != len(allowed) {
		t.Fatalf("%s %d has JSON keys %v, want only %v", label, index, jsonKeys(object), allowed)
	}
	for _, key := range allowed {
		if _, ok := object[key]; !ok {
			t.Fatalf("%s %d is missing JSON key %q", label, index, key)
		}
	}
}

func assertNoJSONKeys(t *testing.T, label string, object map[string]json.RawMessage, forbidden ...string) {
	t.Helper()
	for _, key := range forbidden {
		if _, ok := object[key]; ok {
			t.Fatalf("%s exposes server-only JSON key %q", label, key)
		}
	}
}

func jsonKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
