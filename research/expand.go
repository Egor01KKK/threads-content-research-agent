package research

import (
	"context"
	"errors"
	"strings"
)

// DeterministicExpander is the no-LLM fallback used by the MVP. It combines
// the user's topic with generic research intents and small curated families for
// compound topics, while keeping query generation predictable and testable.
type DeterministicExpander struct {
	MaxQueries int
}

// Expand returns deterministic intent queries plus a few meaningful keyword
// probes. The CLI defaults to ten; callers can raise MaxQueries for broader
// coverage without introducing an LLM into the collection path.
func (e DeterministicExpander) Expand(ctx context.Context, topic string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base := normalizeSpace(topic)
	if base == "" {
		return nil, errors.New("topic is required")
	}

	var candidates []string
	appendCandidate := func(values ...string) {
		candidates = append(candidates, values...)
	}
	appendCandidate(base)

	if isRussianSmallBusinessPainTopic(base) {
		// This is an intentionally Russian, target-domain query pack. It probes
		// owner pain, manual work, client handling, CRM, conversion, booking,
		// analytics, integrations, and team operations without translating the
		// topic into an unrelated English search surface.
		appendCandidate(
			"у кого свой бизнес",
			"все задачи в чатах",
			"что бесит в бизнесе",
			"самая большая проблема в бизнесе",
			"какая рутина съедает время",
			"что делаете вручную",
			"как автоматизировать бизнес",
			"что автоматизировать в бизнесе",
			"все в таблицах",
			"ведем клиентов в Excel",
			"теряются заявки",
			"не успеваем отвечать клиентам",
			"клиенты пишут в разные мессенджеры",
			"какую CRM выбрать",
			"CRM для малого бизнеса",
			"сайт не приносит заявки",
			"клиенты забывают про запись",
			"не понимаю окупаемость рекламы",
			"как связать CRM и сайт",
			"все задачи в чатах",
		)
	} else if isAIAutomationTopic(base) {
		appendCandidate(
			"AI automation",
			"business automation",
			"AI agents",
			"workflow automation",
			"n8n",
			"automate repetitive tasks",
			"lead generation automation",
			"AI for agencies",
			"AI for freelancers",
		)
	} else if isClientAcquisitionTopic(base) {
		// Threads' public search treats short queries as broad lexical probes.
		// For this topic family, standalone "clients"/"freelancers" queries
		// produce too many false positives, so keep the probes anchored to an
		// acquisition intent or to the freelance-work context.
		appendCandidate(
			"find clients",
			"get clients",
			"getting clients",
			"finding customers",
			"client acquisition",
			"freelance clients",
			"freelancer clients",
			"looking for clients",
			"need clients",
			"how to get clients",
			"where to find clients",
			"getting freelance work",
			"finding freelance work",
			"freelance leads",
			"client outreach",
		)
	} else if isSmallBusinessAutomationTopic(base) {
		// This is a target-domain family, not a universal synonym expander.
		// Keep each probe anchored to small-business operations so broad terms
		// such as "automation" or "tools" do not dominate the collection.
		appendCandidate(
			"business automation",
			"business process automation",
			"small business automation",
			"automate business processes",
			"small business workflow automation",
			"automate repetitive tasks",
			"manual processes small business",
			"small business workflows",
			"small business operations",
			"small business admin tasks",
			"small business pain points",
			"small business bottlenecks",
			"what should small businesses automate",
			"automate back office",
			"small business efficiency",
			"tools for small business operations",
			"small business workflow",
			"recurring operational pains small businesses",
			"small business AI tools",
		)
	} else {
		terms := topicTerms(base)
		appendCandidate(
			base+" problems and pain points",
			base+" questions",
			base+" tools and solutions",
			base+" how-to",
			base+" alternatives",
			base+" experiences",
			base+" recommendations",
			base+" business use cases",
		)
		if core := removeStopWords(base); core != "" && !strings.EqualFold(core, base) {
			appendCandidate(core)
		}
		// Threads search often behaves like an AND query. Add a small number
		// of source-derived pairs after the intent budget; unlike a standalone
		// broad token, each pair preserves topic context.
		if len(terms) > 1 {
			appendCandidate(terms[0] + " " + terms[1])
			if len(terms) > 2 {
				appendCandidate(terms[1] + " " + terms[len(terms)-1])
			}
		}
	}

	max := e.MaxQueries
	if max == 0 {
		max = 10
	}
	if max < 0 {
		return nil, errors.New("max queries cannot be negative")
	}
	return uniqueQueries(candidates, max), nil
}

func uniqueQueries(candidates []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		query := normalizeSpace(candidate)
		key := strings.ToLower(query)
		if query == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, query)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func isAIAutomationTopic(topic string) bool {
	words := strings.Fields(strings.ToLower(topic))
	hasAI := false
	hasAutomation := false
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:()[]{}")
		if word == "ai" || strings.HasPrefix(word, "ai-") {
			hasAI = true
		}
		if strings.HasPrefix(word, "automat") || strings.HasPrefix(word, "agent") || strings.HasPrefix(word, "workflow") {
			hasAutomation = true
		}
	}
	return hasAI && hasAutomation
}

func removeStopWords(topic string) string {
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "for": true, "in": true,
		"of": true, "on": true, "the": true, "to": true, "with": true,
	}
	var kept []string
	for _, word := range strings.Fields(topic) {
		if !stop[strings.ToLower(strings.Trim(word, ".,!?;:()[]{}"))] {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

func topicTerms(topic string) []string {
	stop := map[string]bool{
		"build": true, "building": true, "create": true, "creating": true,
		"find": true, "finding": true, "get": true, "getting": true,
		"learn": true, "learning": true, "make": true, "making": true,
		"use": true, "using": true,
	}
	var terms []string
	for _, word := range strings.Fields(topic) {
		term := strings.Trim(word, ".,!?;:()[]{}")
		if len([]rune(term)) < 4 || stop[strings.ToLower(term)] {
			continue
		}
		terms = append(terms, term)
	}
	return terms
}
