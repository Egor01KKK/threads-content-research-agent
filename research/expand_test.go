package research

import (
	"context"
	"strings"
	"testing"
)

func TestDeterministicExpanderAIWorkflow(t *testing.T) {
	queries, err := (DeterministicExpander{}).Expand(context.Background(), "AI automation for small businesses")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 10 {
		t.Fatalf("got %d queries, want 10: %v", len(queries), queries)
	}
	if queries[0] != "AI automation for small businesses" {
		t.Errorf("first query = %q", queries[0])
	}
	wanted := map[string]bool{
		"AI automation":       true,
		"business automation": true,
		"AI agents":           true,
		"workflow automation": true,
		"n8n":                 true,
	}
	for _, query := range queries {
		delete(wanted, query)
	}
	if len(wanted) != 0 {
		t.Errorf("missing curated queries: %v", wanted)
	}
}

func TestDeterministicExpanderGenericTopicIsUnique(t *testing.T) {
	queries, err := (DeterministicExpander{}).Expand(context.Background(), "  local   SEO for restaurants ")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 10 {
		t.Fatalf("got %d queries: %v", len(queries), queries)
	}
	seen := map[string]bool{}
	for _, query := range queries {
		key := strings.ToLower(query)
		if seen[key] {
			t.Errorf("duplicate query %q", query)
		}
		seen[key] = true
	}
	if queries[0] != "local SEO for restaurants" {
		t.Errorf("topic was not normalized: %q", queries[0])
	}
	for _, want := range []string{"problems and pain points", "questions", "tools and solutions", "how-to", "alternatives", "experiences", "recommendations", "business use cases"} {
		found := false
		for _, query := range queries {
			if strings.Contains(strings.ToLower(query), want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("generic expansion missing intent %q: %v", want, queries)
		}
	}
}

func TestDeterministicExpanderHonorsSmallLimit(t *testing.T) {
	queries, err := (DeterministicExpander{MaxQueries: 3}).Expand(context.Background(), "product analytics")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 {
		t.Fatalf("got %d queries: %v", len(queries), queries)
	}
}

func TestDeterministicExpanderAddsMeaningfulTopicAnchors(t *testing.T) {
	queries, err := (DeterministicExpander{MaxQueries: 20}).Expand(context.Background(), "finding clients for freelancers")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, query := range queries {
		seen[strings.ToLower(query)] = true
	}
	for _, want := range []string{"find clients", "client acquisition", "freelance clients", "where to find clients", "client outreach"} {
		if !seen[want] {
			t.Errorf("missing high-precision intent %q: %v", want, queries)
		}
	}
	for _, noisy := range []string{"clients", "freelancers", "finding"} {
		if seen[noisy] {
			t.Errorf("broad standalone query should not be emitted: %q", noisy)
		}
	}
}

func TestDeterministicExpanderSmallBusinessAutomationProblems(t *testing.T) {
	queries, err := (DeterministicExpander{MaxQueries: 20}).Expand(context.Background(), "problems small businesses want to automate")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 20 {
		t.Fatalf("got %d queries: %v", len(queries), queries)
	}
	seen := map[string]bool{}
	for _, query := range queries {
		seen[strings.ToLower(query)] = true
	}
	for _, want := range []string{
		"business automation",
		"small business automation",
		"business process automation",
		"automate repetitive tasks",
		"small business pain points",
		"what should small businesses automate",
		"tools for small business operations",
		"small business workflow automation",
	} {
		if !seen[want] {
			t.Errorf("missing target-domain query %q: %v", want, queries)
		}
	}
	for _, noisy := range []string{"small", "business", "businesses", "problems", "automate", "tools"} {
		if seen[noisy] {
			t.Errorf("broad standalone query should not be emitted: %q", noisy)
		}
	}
}

func TestDeterministicExpanderRussianSmallBusinessPainPack(t *testing.T) {
	queries, err := (DeterministicExpander{MaxQueries: 20}).Expand(context.Background(), "проблемы малого бизнеса которые можно решить автоматизацией и IT")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 20 {
		t.Fatalf("got %d queries: %v", len(queries), queries)
	}
	for _, query := range queries {
		withoutAllowedProducts := strings.ReplaceAll(strings.ReplaceAll(query, "CRM", ""), "IT", "")
		withoutAllowedProducts = strings.ReplaceAll(withoutAllowedProducts, "Excel", "")
		if strings.ContainsAny(withoutAllowedProducts, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			// English product names are allowed in the target corpus, but the
			// deterministic query pack itself should stay Russian.
			t.Errorf("query pack translated or used an English query: %q", query)
		}
	}
	joined := strings.Join(queries, " | ")
	for _, want := range []string{"свой бизнес", "вручную", "автоматизировать", "таблицах", "заявки", "мессенджеры", "CRM", "сайт", "запись", "окупаемость", "чатах"} {
		if !strings.Contains(strings.ToLower(joined), strings.ToLower(want)) {
			t.Errorf("Russian target query pack missing family signal %q: %v", want, queries)
		}
	}
}
