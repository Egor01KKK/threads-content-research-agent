package research

import (
	"sort"
	"time"
)

func extractSemanticWorkflow(text string, assessment SemanticAssessmentV2, _ AuthorBusinessContext) WorkflowEvidence {
	workflow := WorkflowEvidence{}
	if assessment.NonTarget {
		return workflow
	}
	switch {
	case hasAnySemantic(text, "тендер", "закупк", "техническое задание") ||
		(hasExactSemanticToken(text, "тз") && hasAnySemantic(text, "поставщик", "предложение", "несоответствия")):
		workflow.WorkflowName = "Tender preflight review"
		workflow.WorkflowTrigger = "new tender or procurement document"
		workflow.CurrentProcess = "document is sent to a general-purpose AI assistant to find requirements, supplier questions, deadlines, and blind spots"
		workflow.HumanRoles = []string{"tender specialist"}
		workflow.ManualSteps = []string{"review requirements", "check supplier questions", "collect deadlines", "review execution blind spots"}
		workflow.FailurePoint = "requirements or execution questions may be missed"
		workflow.DesiredOutcome = "submit and execute the tender with fewer overlooked requirements"
		workflow.ToolsUsed = semanticTools(text)
	case semanticOperationalDemand(text) && hasAnySemantic(text, "бухгалтер", "накладн", "взаиморасч", "1с", "касс"):
		workflow.WorkflowName = "Order and back-office processing"
		workflow.WorkflowTrigger = "a customer order or accounting document arrives"
		workflow.CurrentProcess = "operator records orders, primary documents, payments, and daily reporting in business software"
		workflow.HumanRoles = []string{"accounting operator"}
		workflow.ManualSteps = []string{"receive and process orders", "prepare invoices and primary documents", "check payments", "prepare daily reports"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "keep order and back-office records current"
		workflow.ToolsUsed = semanticTools(text)
	case hasAnySemantic(text, "оставить заявку", "купить", "сайт", "лендинг", "конвер") &&
		hasAnySemantic(text, "дизайн", "анимац", "заявк", "продаж"):
		workflow.WorkflowName = "Website conversion"
		workflow.WorkflowTrigger = "a visitor evaluates a website or landing page"
		workflow.CurrentProcess = "visitor moves from page impression to request or purchase"
		workflow.HumanRoles = []string{"business owner", "web designer"}
		workflow.ManualSteps = []string{"review offer clarity", "review page friction", "review request or purchase path"}
		workflow.FailurePoint = "visual design can make the request or purchase path harder"
		workflow.DesiredOutcome = "turn site attention into requests or purchases"
		workflow.ToolsUsed = semanticTools(text)
	case semanticOperationalDemand(text) && hasAnySemantic(text, "координатор", "ассистент", "передавать задачи", "статусы", "сроки", "креатор"):
		workflow.WorkflowName = "Client and production coordination"
		workflow.WorkflowTrigger = "a new client order or production task arrives"
		workflow.CurrentProcess = "coordinator communicates with clients, distributes work, and tracks statuses and deadlines"
		workflow.HumanRoles = []string{"assistant", "production coordinator", "creator"}
		workflow.ManualSteps = []string{"reply to clients", "assign work", "track status", "track deadlines"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "keep client orders and production status synchronized"
		workflow.ToolsUsed = semanticTools(text)
	case semanticOperationalDemand(text) && hasAnySemantic(text, "продаж", "тёплые входящие", "теплые входящие", "доводить", "оплат"):
		workflow.WorkflowName = "Warm inbound sales"
		workflow.WorkflowTrigger = "a warm inbound lead arrives in messages"
		workflow.CurrentProcess = "sales manager replies in messages, consults the lead, and moves the conversation toward payment"
		workflow.HumanRoles = []string{"sales manager"}
		workflow.ManualSteps = []string{"reply in messages", "consult", "send price or offer", "follow up to payment"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "convert warm inbound conversations into payments"
		workflow.ToolsUsed = semanticTools(text)
	case semanticOperationalDemand(text) && hasAnySemantic(text, "директ", "сообщен", "запис", "обработк заяв", "консульт"):
		workflow.WorkflowName = "Inbound lead handling"
		workflow.WorkflowTrigger = "a new message or request arrives"
		workflow.CurrentProcess = "employee replies to the prospect, answers questions, qualifies the request, and books a procedure"
		workflow.HumanRoles = []string{"administrator", "lead-response specialist"}
		workflow.ManualSteps = []string{"reply to messages", "consult prospect", "qualify request", "book procedure"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "respond, qualify, and book inbound prospects consistently"
		workflow.ToolsUsed = semanticTools(text)
	case hasAnySemantic(text, "хаос", "бизнес-партнёр", "бизнес партнер") && hasAnySemantic(text, "chatgpt", "стратег"):
		workflow.WorkflowName = "Business planning and thought structuring"
		workflow.WorkflowTrigger = "unstructured business thoughts or operating context"
		workflow.CurrentProcess = "operator gives messy context to ChatGPT and uses the returned structure as a strategy aid"
		workflow.HumanRoles = []string{"business operator"}
		workflow.ManualSteps = []string{"collect context", "describe the problem", "review generated structure"}
		workflow.FailurePoint = "business context feels chaotic before it is structured"
		workflow.DesiredOutcome = "turn business chaos into a usable strategy"
		workflow.ToolsUsed = semanticTools(text)
	case assessment.ActiveSolutionSeeking && hasAnySemantic(text, "crm", "бот", "сервис", "автоматиз", "интеграц"):
		workflow.WorkflowName = "Business software or automation selection"
		workflow.WorkflowTrigger = "an operating process needs a software or automation solution"
		workflow.CurrentProcess = "operator compares or asks about a tool, bot, CRM, or integration"
		workflow.HumanRoles = []string{"business operator"}
		workflow.ManualSteps = []string{"describe process", "compare solutions", "choose implementation path"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "choose a workable digital solution"
		workflow.ToolsUsed = semanticTools(text)
	case hasAnySemantic(text, "контент-завод", "автоматизац", "n8n", "telegram-бот", "телеграм-бот") &&
		hasAnySemantic(text, "собрал", "собираю", "в работе", "готово"):
		workflow.WorkflowName = "Content or local-service automation build"
		workflow.WorkflowTrigger = "a repeatable content or customer-service workflow is identified"
		workflow.CurrentProcess = "builder assembles an automation with a bot, AI model, workflow nodes, or APIs"
		workflow.HumanRoles = []string{"automation builder"}
		workflow.ManualSteps = []string{"define workflow", "connect tools", "test automation", "review output"}
		workflow.FailurePoint = "not stated"
		workflow.DesiredOutcome = "reduce repeated work in content or customer service"
		workflow.ToolsUsed = semanticTools(text)
	}
	return workflow
}

func semanticTools(text string) []string {
	var tools []string
	checks := []struct {
		label string
		terms []string
	}{
		{"ChatGPT", []string{"chatgpt"}},
		{"Claude", []string{"claude"}},
		{"Telegram", []string{"telegram", "телеграм"}},
		{"WhatsApp", []string{"whatsapp", "ватсап"}},
		{"CRM", []string{"crm", "црм"}},
		{"Bitrix24", []string{"битрикс", "битрикс24"}},
		{"amoCRM", []string{"amocrm", "амоcrm"}},
		{"Excel", []string{"excel", "эксель"}},
		{"Google Sheets", []string{"google sheets", "гугл таблиц"}},
		{"n8n", []string{"n8n"}},
		{"API", []string{"api"}},
		{"Tilda", []string{"tilda", "тильда"}},
		{"Miro", []string{"miro"}},
	}
	for _, check := range checks {
		if hasAnySemantic(text, check.terms...) {
			tools = append(tools, check.label)
		}
	}
	return tools
}

func updateSemanticSummary(summary *SemanticV2Summary, record SemanticV2PostRecord) {
	assessment := record.Assessment
	if summary.PrimaryClassCounts == nil {
		summary.PrimaryClassCounts = map[string]int{}
	}
	if summary.ClassCounts == nil {
		summary.ClassCounts = map[string]int{}
	}
	if summary.ITOpportunityCounts == nil {
		summary.ITOpportunityCounts = map[string]int{}
	}
	summary.PrimaryClassCounts[string(assessment.PrimaryEvidenceClass)]++
	for _, class := range assessment.EvidenceClasses {
		summary.ClassCounts[string(class)]++
	}
	summary.ITOpportunityCounts[string(assessment.ITOpportunityTier)]++
	if assessment.ExplicitBusinessPain {
		summary.ExplicitBusinessPainPosts++
	}
	if assessment.OperationalDemandSignal {
		summary.OperationalDemandPosts++
	}
	if assessment.ActiveSolutionSeeking {
		summary.ActiveSolutionSeekingPosts++
	}
	if assessment.WorkaroundSignal {
		summary.WorkaroundPosts++
	}
	if assessment.SignalTier == SemanticTierStrong {
		summary.StrongSignals++
	}
	if assessment.SignalTier == SemanticTierGold {
		summary.GoldSignals++
	}
}

type semanticWorkflowAccumulator struct {
	name          string
	posts         int
	authors       map[string]bool
	verified      map[string]bool
	businessTypes map[string]bool
	tools         map[string]bool
	manualSteps   map[string]bool
	consequences  map[BusinessConsequence]bool
	urls          map[string]bool
	postIDs       map[string]bool
	relatives     []float64
	early         *time.Time
	late          *time.Time
	painPosts     int
	workloads     int
	solutions     int
	itPosts       int
}

func buildWorkflowClusters(records []SemanticV2PostRecord, contexts []AuthorBusinessContext) []WorkflowCluster {
	contextByAuthor := map[string]AuthorBusinessContext{}
	for _, context := range contexts {
		contextByAuthor[normalizeAuthor(context.Username)] = context
	}
	groups := map[string]*semanticWorkflowAccumulator{}
	for _, record := range records {
		name := record.Assessment.Workflow.WorkflowName
		if name == "" {
			continue
		}
		group := groups[name]
		if group == nil {
			group = &semanticWorkflowAccumulator{
				name:          name,
				authors:       map[string]bool{},
				verified:      map[string]bool{},
				businessTypes: map[string]bool{},
				tools:         map[string]bool{},
				manualSteps:   map[string]bool{},
				consequences:  map[BusinessConsequence]bool{},
				urls:          map[string]bool{},
				postIDs:       map[string]bool{},
			}
			groups[name] = group
		}
		group.posts++
		username := normalizeAuthor(record.Post.AuthorUsername)
		if username != "" {
			group.authors[username] = true
			if contextByAuthor[username].VerifiedOwnerContext {
				group.verified[username] = true
			}
		}
		if record.Assessment.BusinessType != "" {
			group.businessTypes[record.Assessment.BusinessType] = true
		}
		for _, tool := range record.Assessment.Workflow.ToolsUsed {
			group.tools[tool] = true
		}
		for _, step := range record.Assessment.Workflow.ManualSteps {
			group.manualSteps[step] = true
		}
		if record.Assessment.BusinessConsequence != ConsequenceNone {
			group.consequences[record.Assessment.BusinessConsequence] = true
		}
		if record.Assessment.ExplicitBusinessPain {
			group.painPosts++
		}
		if record.Assessment.OperationalDemandSignal {
			group.workloads++
		}
		if record.Assessment.ActiveSolutionSeeking {
			group.solutions++
		}
		if record.Assessment.ITOpportunityTier != ITOpportunityLow {
			group.itPosts++
		}
		if record.Post.URL != "" {
			group.urls[record.Post.URL] = true
		}
		if record.Post.ID != "" {
			group.postIDs[record.Post.ID] = true
		}
		if record.Post.PublishedAt.IsZero() == false {
			date := record.Post.PublishedAt.UTC()
			if group.early == nil || date.Before(*group.early) {
				group.early = &date
			}
			if group.late == nil || date.After(*group.late) {
				group.late = &date
			}
		}
		if record.Performance != nil && record.Performance.BaselineConfidence == baselineUsable && record.Performance.RelativePerformance != nil {
			group.relatives = append(group.relatives, *record.Performance.RelativePerformance)
		}
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	clusters := make([]WorkflowCluster, 0, len(names))
	for _, name := range names {
		group := groups[name]
		verifiedCount := len(group.verified)
		clusters = append(clusters, WorkflowCluster{
			WorkflowName:                name,
			EvidenceStrength:            workflowEvidenceStrength(verifiedCount, len(group.authors)),
			Posts:                       group.posts,
			UniqueAuthors:               len(group.authors),
			UniqueVerifiedOwnerContexts: verifiedCount,
			BusinessTypes:               sortedBoolKeys(group.businessTypes),
			DateCoverage: DeepDateCoverage{
				Earliest: group.early,
				Latest:   group.late,
			},
			ToolsMentioned:         sortedBoolKeys(group.tools),
			ManualSteps:            sortedBoolKeys(group.manualSteps),
			BusinessConsequences:   sortedBusinessConsequences(group.consequences),
			PainPosts:              group.painPosts,
			OperationalDemandPosts: group.workloads,
			SolutionSeekingPosts:   group.solutions,
			ITActionablePosts:      group.itPosts,
			Performance: SemanticPerformanceSummary{
				PostsWithKnownBaseline: len(group.relatives),
				PostsAboveBaseline:     countAboveBaseline(group.relatives),
				MedianRelative:         optionalMedian(group.relatives),
			},
			EvidenceURLs:   sortedLimited(group.urls, 20),
			ExamplePostIDs: sortedLimited(group.postIDs, 20),
		})
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].UniqueVerifiedOwnerContexts != clusters[j].UniqueVerifiedOwnerContexts {
			return clusters[i].UniqueVerifiedOwnerContexts > clusters[j].UniqueVerifiedOwnerContexts
		}
		if clusters[i].Posts != clusters[j].Posts {
			return clusters[i].Posts > clusters[j].Posts
		}
		return clusters[i].WorkflowName < clusters[j].WorkflowName
	})
	return clusters
}

func workflowEvidenceStrength(verifiedOwners, authors int) EvidenceStrength {
	count := verifiedOwners
	if count == 0 {
		count = authors
	}
	switch {
	case count >= 5:
		return EvidenceStrongRepeat
	case count >= 3:
		return EvidenceRepeated
	case count == 2:
		return EvidenceWeakRepeat
	default:
		return EvidenceSingleSource
	}
}

func sortedBusinessConsequences(values map[BusinessConsequence]bool) []BusinessConsequence {
	out := make([]BusinessConsequence, 0, len(values))
	for value := range values {
		if value != ConsequenceNone {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedLimited(values map[string]bool, limit int) []string {
	out := sortedBoolKeys(values)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func countAboveBaseline(values []float64) int {
	count := 0
	for _, value := range values {
		if value > 1 {
			count++
		}
	}
	return count
}
