package research

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// DeepCorpusExportInput is the small part of complete-readable.json required
// for a local semantic reclassification. Unknown fields are deliberately
// ignored so the command can read future deep export versions.
type DeepCorpusExportInput struct {
	ExportVersion string `json:"export_version"`
	Report        struct {
		Topic string `json:"topic"`
	} `json:"report"`
	SeedProfiles []DeepSeedProfile `json:"seed_profiles"`
	Corpus       []DeepPostRecord  `json:"corpus"`
}

// LoadDeepCorpusExport reads an existing deep export. It never creates a
// Threads client and never performs network I/O.
func LoadDeepCorpusExport(path string) (DeepCorpusExportInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return DeepCorpusExportInput{}, fmt.Errorf("open deep corpus export: %w", err)
	}
	defer func() { _ = file.Close() }()

	var input DeepCorpusExportInput
	if err := json.NewDecoder(file).Decode(&input); err != nil {
		return DeepCorpusExportInput{}, fmt.Errorf("decode deep corpus export: %w", err)
	}
	if len(input.Corpus) == 0 {
		return DeepCorpusExportInput{}, errors.New("deep corpus export contains no posts")
	}
	return input, nil
}

// ReclassifyDeepCorpus performs the v2 semantic pass over an existing JSON
// export. The result keeps every original post and its previous deterministic
// assessment next to the new evidence model.
func ReclassifyDeepCorpus(path string) (SemanticV2Result, error) {
	input, err := LoadDeepCorpusExport(path)
	if err != nil {
		return SemanticV2Result{}, err
	}
	contexts := buildAuthorBusinessContexts(input.Corpus, input.SeedProfiles)
	contextByUsername := make(map[string]AuthorBusinessContext, len(contexts))
	verifiedContexts := 0
	for _, context := range contexts {
		contextByUsername[normalizeAuthor(context.Username)] = context
		if context.VerifiedOwnerContext {
			verifiedContexts++
		}
	}

	result := SemanticV2Result{
		ExportVersion:         SemanticV2Version,
		GeneratedAt:           time.Now().UTC(),
		SourcePath:            path,
		Topic:                 input.Report.Topic,
		AuthorBusinessContext: contexts,
		Summary: SemanticV2Summary{
			CorpusPosts:                len(input.Corpus),
			Authors:                    len(contexts),
			AuthorsWithVerifiedContext: verifiedContexts,
			PrimaryClassCounts:         map[string]int{},
			ClassCounts:                map[string]int{},
			ITOpportunityCounts:        map[string]int{},
		},
		Limitations: []string{
			"This is a deterministic local reclassification of an existing deep export; it does not add collection coverage.",
			"Business context is inferred from visible post/profile evidence and remains an auditable heuristic, not a verified user attribute.",
			"Workflow and IT opportunity fields preserve uncertainty; an automation hypothesis is not buyer intent or a product request.",
			"Post and author counts are not independent market observations when several posts come from one author.",
		},
		Corpus:               make([]SemanticV2PostRecord, 0, len(input.Corpus)),
		VerifiedPains:        make([]SemanticV2PostRecord, 0),
		OperationalWorkloads: make([]SemanticV2PostRecord, 0),
		SolutionSeeking:      make([]SemanticV2PostRecord, 0),
		StrongSignals:        make([]SemanticV2PostRecord, 0),
		GoldSignalsV2:        make([]SemanticV2PostRecord, 0),
		WorkflowClusters:     make([]WorkflowCluster, 0),
	}

	for _, source := range input.Corpus {
		username := normalizeAuthor(source.Post.AuthorUsername)
		context := contextByUsername[username]
		assessment := classifySemanticPost(source.Post.Text, context)
		result.Corpus = append(result.Corpus, SemanticV2PostRecord{
			Post:               source.Post,
			Stages:             append([]string(nil), source.Stages...),
			SearchQueries:      append([]string(nil), source.SearchQueries...),
			Provenance:         append([]DeepProvenance(nil), source.Provenance...),
			PreviousAssessment: source.Assessment,
			Performance:        source.Performance,
			Assessment:         assessment,
		})
	}
	sort.SliceStable(result.Corpus, func(i, j int) bool { return result.Corpus[i].Post.ID < result.Corpus[j].Post.ID })
	for _, record := range result.Corpus {
		updateSemanticSummary(&result.Summary, record)
		switch record.Assessment.PrimaryEvidenceClass {
		case EvidenceExplicitBusinessPain:
			result.VerifiedPains = append(result.VerifiedPains, record)
		case EvidenceOperationalDemand:
			result.OperationalWorkloads = append(result.OperationalWorkloads, record)
		case EvidenceActiveSolutionSeeking:
			result.SolutionSeeking = append(result.SolutionSeeking, record)
		}
		if record.Assessment.SignalTier == SemanticTierStrong {
			result.StrongSignals = append(result.StrongSignals, record)
		}
		if record.Assessment.SignalTier == SemanticTierGold {
			result.GoldSignalsV2 = append(result.GoldSignalsV2, record)
		}
	}
	result.WorkflowClusters = buildWorkflowClusters(result.Corpus, contexts)
	return result, nil
}

type semanticAuthorEvidence struct {
	profile        Author
	profileFetched bool
	profileText    string
	profileReasons map[string]bool
	postReasons    map[string]bool
	commercial     map[string]bool
	team           map[string]bool
	clients        map[string]bool
	transactions   map[string]bool
	texts          []string
}

func buildAuthorBusinessContexts(posts []DeepPostRecord, seeds []DeepSeedProfile) []AuthorBusinessContext {
	byUsername := map[string]*semanticAuthorEvidence{}
	get := func(username string) *semanticAuthorEvidence {
		username = normalizeAuthor(username)
		if username == "" {
			return nil
		}
		if byUsername[username] == nil {
			byUsername[username] = &semanticAuthorEvidence{
				profile:        Author{Username: username},
				profileReasons: map[string]bool{},
				postReasons:    map[string]bool{},
				commercial:     map[string]bool{},
				team:           map[string]bool{},
				clients:        map[string]bool{},
				transactions:   map[string]bool{},
			}
		}
		return byUsername[username]
	}

	for _, source := range posts {
		evidence := get(source.Post.AuthorUsername)
		if evidence == nil {
			continue
		}
		evidence.profile = mergeAuthor(evidence.profile, Author{
			Username:   source.Post.AuthorUsername,
			ProfileURL: profileURLFromPost(source.Post),
		})
		evidence.texts = append(evidence.texts, source.Post.Text)
		postEvidence := semanticPostBusinessContext(source.Post.Text)
		for _, reason := range postEvidence {
			evidence.postReasons[reason] = true
		}
		collectAuthorEvidence(evidence, source.Post.Text, postEvidence, false)
	}
	for _, seed := range seeds {
		if !seed.ProfileFetched {
			continue
		}
		evidence := get(seed.Username)
		if evidence == nil {
			continue
		}
		evidence.profileFetched = true
		evidence.profile = mergeAuthor(evidence.profile, seed.Profile)
		evidence.profileText = strings.TrimSpace(seed.Profile.Bio)
		for _, reason := range semanticProfileBusinessContext(seed.Profile.Bio) {
			evidence.profileReasons[reason] = true
		}
		collectAuthorEvidence(evidence, seed.Profile.Bio, sortedBoolKeys(evidence.profileReasons), true)
	}

	usernames := make([]string, 0, len(byUsername))
	for username := range byUsername {
		usernames = append(usernames, username)
	}
	sort.Strings(usernames)
	result := make([]AuthorBusinessContext, 0, len(usernames))
	for _, username := range usernames {
		result = append(result, finalizeAuthorBusinessContext(username, byUsername[username]))
	}
	return result
}

func profileURLFromPost(post Post) string {
	if post.AuthorUsername == "" {
		return ""
	}
	return "https://www.threads.com/@" + normalizeAuthor(post.AuthorUsername)
}

func collectAuthorEvidence(evidence *semanticAuthorEvidence, text string, reasons []string, profile bool) {
	value := normalizeRussian(text)
	for _, reason := range reasons {
		switch {
		case strings.Contains(reason, "commercial") || strings.Contains(reason, "service") || strings.Contains(reason, "business"):
			evidence.commercial[reason] = true
		case strings.Contains(reason, "team") || strings.Contains(reason, "staff") || strings.Contains(reason, "employer"):
			evidence.team[reason] = true
		case strings.Contains(reason, "client") || strings.Contains(reason, "lead") || strings.Contains(reason, "customer"):
			evidence.clients[reason] = true
		case strings.Contains(reason, "transaction") || strings.Contains(reason, "order") || strings.Contains(reason, "payment"):
			evidence.transactions[reason] = true
		}
	}
	if hasAnySemantic(value, "продаж", "заказ", "оплат", "купить", "заяв", "лид") {
		if profile {
			evidence.transactions["profile_transaction_language"] = true
		} else {
			evidence.transactions["post_transaction_language"] = true
		}
	}
}

func finalizeAuthorBusinessContext(username string, evidence *semanticAuthorEvidence) AuthorBusinessContext {
	profileReasons := sortedBoolKeys(evidence.profileReasons)
	postReasons := sortedBoolKeys(evidence.postReasons)
	commercial := sortedBoolKeys(evidence.commercial)
	team := sortedBoolKeys(evidence.team)
	clients := sortedBoolKeys(evidence.clients)
	transactions := sortedBoolKeys(evidence.transactions)

	profileScore := 0
	if len(profileReasons) > 0 {
		profileScore++
	}
	if containsAnySemantic(evidence.profileText, "основал", "основала", "владел", "руковод", "предприним", "ип", "самозан") {
		profileScore += 3
	}
	if len(commercial) > 0 {
		profileScore++
	}
	if len(team) > 0 {
		profileScore++
	}
	if len(clients) > 0 {
		profileScore++
	}
	if len(transactions) > 0 {
		profileScore++
	}
	postScore := len(postReasons)
	if containsAnyReason(postReasons, "explicit_business_identity", "employer_or_vacancy_context", "professional_workflow_first_person") {
		postScore += 2
	}
	ownerScore := profileScore + postScore
	ownerLikelihood := OwnerLikelihoodUnknown
	if ownerScore >= 7 {
		ownerLikelihood = OwnerLikelihoodHigh
	} else if ownerScore >= 3 {
		ownerLikelihood = OwnerLikelihoodMedium
	} else if ownerScore > 0 {
		ownerLikelihood = OwnerLikelihoodLow
	}
	operatorLikelihood := OwnerLikelihoodUnknown
	if len(team) > 0 || len(transactions) > 0 || containsAnyReason(postReasons, "explicit_business_identity", "employer_or_vacancy_context") {
		operatorLikelihood = OwnerLikelihoodHigh
	} else if len(commercial) > 0 || len(clients) > 0 {
		operatorLikelihood = OwnerLikelihoodMedium
	} else if ownerScore > 0 {
		operatorLikelihood = OwnerLikelihoodLow
	}
	confidence := "UNKNOWN"
	if ownerScore >= 7 || (len(profileReasons) > 0 && len(postReasons) > 0) {
		confidence = "HIGH"
	} else if ownerScore >= 3 {
		confidence = "MEDIUM"
	} else if ownerScore > 0 {
		confidence = "LOW"
	}
	businessType := firstBusinessType(append([]string{evidence.profileText}, evidence.texts...))
	verified := ownerScore >= 3 && (len(profileReasons) > 0 || len(postReasons) > 0)
	return AuthorBusinessContext{
		Username:                   username,
		Profile:                    evidence.profile,
		OwnerLikelihood:            ownerLikelihood,
		OperatorLikelihood:         operatorLikelihood,
		BusinessType:               businessType,
		CommercialActivityEvidence: commercial,
		TeamEvidence:               team,
		ClientEvidence:             clients,
		TransactionEvidence:        transactions,
		ProfileEvidenceReasons:     profileReasons,
		PostEvidenceReasons:        postReasons,
		Confidence:                 confidence,
		VerifiedOwnerContext:       verified,
		ProfileFetched:             evidence.profileFetched,
	}
}

func firstBusinessType(texts []string) string {
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			continue
		}
		types := russianBusinessTypes(normalizeRussian(text), relevanceTokens(normalizeRussian(text)))
		if len(types) > 0 {
			return types[0]
		}
	}
	return ""
}

func classifySemanticPost(text string, author AuthorBusinessContext) SemanticAssessmentV2 {
	normalized := normalizeRussian(text)
	assessment := SemanticAssessmentV2{
		OwnerLikelihood:        author.OwnerLikelihood,
		OperatorLikelihood:     author.OperatorLikelihood,
		BusinessType:           author.BusinessType,
		BusinessNameIfExplicit: author.BusinessNameIfExplicit,
		BusinessConsequence:    ConsequenceNone,
		ITOpportunityTier:      ITOpportunityLow,
		SignalTier:             SemanticTierInformational,
		Confidence:             "LOW",
	}
	assessment.BusinessContextPostEvidence = semanticPostBusinessContext(text)
	assessment.BusinessContextProfileEvidence = append([]string(nil), author.ProfileEvidenceReasons...)
	assessment.BusinessContextPresent = len(assessment.BusinessContextPostEvidence) > 0 || author.VerifiedOwnerContext
	assessment.NonTarget = semanticNonTarget(normalized)
	operationalDemand := semanticOperationalDemand(normalized) && !assessment.NonTarget
	assessment.OperationalDemandSignal = operationalDemand
	assessment.ServiceOffer = semanticServiceOffer(normalized)
	assessment.WorkaroundSignal = semanticWorkaround(normalized, assessment.BusinessContextPresent) &&
		!assessment.NonTarget && !assessment.ServiceOffer
	assessment.ActiveSolutionSeeking = semanticSolutionSeeking(normalized, assessment.BusinessContextPresent) &&
		!assessment.NonTarget && !assessment.ServiceOffer
	assessment.BusinessOpinion = semanticBusinessOpinion(normalized, assessment.BusinessContextPresent)
	assessment.Workflow = extractSemanticWorkflow(normalized, assessment, author)
	assessment.BusinessConsequence, assessment.ConsequenceExplicit = semanticConsequence(normalized, assessment.Workflow)
	if assessment.NonTarget {
		assessment.BusinessConsequence = ConsequenceNone
		assessment.ConsequenceExplicit = false
	}
	assessment.Workflow.BusinessConsequence = string(assessment.BusinessConsequence)
	assessment.ExplicitBusinessPain = semanticExplicitPain(normalized, assessment, author)
	assessment.HumanWorkflowLoad = semanticWorkflowLoad(normalized, assessment.Workflow, operationalDemand)
	assessment.ITOpportunityScore, assessment.ITOpportunityTier = semanticITOpportunity(normalized, assessment)
	assessment.AutomationHypothesis = semanticAutomationHypothesis(assessment)
	assessment.GoldScore = semanticGoldScore(assessment)
	assessment.SignalTier = semanticSignalTier(assessment)
	assessment.Confidence = semanticConfidence(assessment)
	assessment.EvidenceClasses = semanticEvidenceClasses(assessment)
	assessment.PrimaryEvidenceClass = semanticPrimaryClass(assessment)
	assessment.Uncertain = assessment.PrimaryEvidenceClass == EvidenceUncertain
	assessment.Reasons = semanticReasons(assessment)
	return assessment
}

func semanticPostBusinessContext(text string) []string {
	normalized := normalizeRussian(text)
	var reasons []string
	if containsAnySemantic(normalized,
		"у меня свой бизнес", "у меня свое дело", "мой бизнес", "мой магазин", "мой салон", "моя студия",
		"у нас бизнес", "у нас салон", "у нас студия", "у нас агентство", "в моем агентстве", "в моей студии",
		"я предприниматель", "я ип", "работаю на себя", "веду бизнес", "владею бизнесом", "руководитель",
		"основал", "основала", "расширяем команду", "моя команда") {
		reasons = append(reasons, "explicit_business_identity")
	}
	if containsAnySemantic(normalized,
		"наши клиенты", "клиентам", "отвечать клиентам", "принимаем заявки", "обрабатываем заявки",
		"тёплые входящие заявки", "теплые входящие заявки", "работа с действующей клиентской базой",
		"передавать задачи", "контролировать статусы", "сроки", "заказы клиентов") {
		reasons = append(reasons, "client_or_workflow_context")
	}
	if semanticOperationalDemand(normalized) && containsAnySemantic(normalized, "в компанию", "в салон", "в студию", "в агентство", "требуется", "ищем", "ищу") {
		reasons = append(reasons, "employer_or_vacancy_context")
	}
	if hasSemanticFirstPerson(relevanceTokens(normalized)) && hasAnySemantic(normalized,
		"тендер", "закупк", "продаж", "заявк", "клиент", "лид", "сайт", "студию", "контент") {
		reasons = append(reasons, "professional_workflow_first_person")
	}
	if hasSemanticFirstPerson(relevanceTokens(normalized)) && containsAnySemantic(normalized,
		"помогаю", "настрою", "создаю", "продаю", "набираю учеников", "делаю сайты", "оказываю услуги") {
		reasons = append(reasons, "first_person_commercial_activity")
	}
	return uniqueStrings(reasons)
}

func semanticProfileBusinessContext(bio string) []string {
	normalized := normalizeRussian(bio)
	if normalized == "" {
		return nil
	}
	var reasons []string
	if containsAnySemantic(normalized,
		"предприниматель", "основал", "основала", "владелец", "руководитель", "управляю",
		"мой бизнес", "свой бизнес", "ип", "самозанятый", "агентств", "магазин", "салон",
		"студия", "клиника", "школа", "кофейн", "бизнесу нужен") {
		reasons = append(reasons, "profile_business_identity")
	}
	if containsAnySemantic(normalized,
		"помогаю", "увеличиваю продажи", "аудит", "консульт", "создаю", "произвожу",
		"занятия онлайн", "услуг", "научу", "продаю", "портфолио") {
		reasons = append(reasons, "profile_commercial_activity")
	}
	if containsAnySemantic(normalized, "клиент", "лид", "продаж", "заявк", "тендер", "заказ") {
		reasons = append(reasons, "profile_client_or_transaction_context")
	}
	if containsAnySemantic(normalized, "команд", "сотруд", "основал", "основала", "руковод") {
		reasons = append(reasons, "profile_team_or_operator_context")
	}
	return uniqueStrings(reasons)
}

func semanticNonTarget(text string) bool {
	if containsAnySemantic(text,
		"маммолог", "грудью", "гв ", "ребенок", "ребёнок", "дети", "коляск", "садик",
		"грудн", "вскарм",
		"студент", "диплом", "антиплагиат", "научрук", "курсов", "огэ", "егэ", "экзамен",
		"урок", "ученик", "учениц", "английск", "репетитор", "набираю учеников", "ищу учеников",
		"индивидуальные занятия", "пробный урок",
		"банкротств", "автокредит", "коммунал", "каспи", "курьеру", "15 кг", "войн",
		"курьер", "доставк", "карго", "закупкам в китае", "переехать", "уехать в германи", "ausbildung", "виза", "кредит", "аборт",
		"любовь не заслуживается", "не обжираться") {
		return true
	}
	if containsAnySemantic(text, "гдз", "тест") {
		return true
	}
	return false
}

func semanticOperationalDemand(text string) bool {
	return containsAnySemantic(text,
		"вакансия", "требуется", "ищем сотруд", "ищу сотруд", "ищу ассистент", "ищу менеджера",
		"ищу координатор", "нужен администратор", "обязанности:", "зп ", "зарплат") &&
		containsAnySemantic(text, "клиент", "заявк", "продаж", "директ", "сообщен", "запис", "задач", "заказ", "сотруд")
}

func semanticWorkaround(text string, businessContext bool) bool {
	if !businessContext {
		return false
	}
	if !hasAnySemantic(text,
		"тендер", "закупк", "техническое задание", "поставщик", "заявк", "лид",
		"клиент", "продаж", "сайт", "лендинг", "бизнес", "стратег", "контент",
		"автоматиз", "бот", "сервис") && !hasExactSemanticToken(text, "тз") {
		return false
	}
	return containsAnySemantic(text,
		"excel", "эксель", "google sheets", "в таблицах", "вручную", "пишем каждому", "сам отвечаю всем",
		"копируем руками", "переносим данные", "чатах", "chatgpt", "claude", "miro")
}

func semanticSolutionSeeking(text string, businessContext bool) bool {
	if !businessContext {
		return false
	}
	return containsAnySemantic(text,
		"посоветуйте сервис", "посоветуйте crm", "какую crm", "нужна crm", "нужен бот",
		"есть ли сервис", "чем заменить", "кто может настроить", "ищу разработчика",
		"как автоматизировать", "что автоматизировать", "как убрать ручную работу",
		"как связать", "кто может внедрить", "каким сервисом", "какую систему выбрать")
}

func semanticServiceOffer(text string) bool {
	if containsAnySemantic(text,
		"помогаю", "настрою", "сделаю", "создаю", "предлагаю", "аудит", "разбор аккаунта",
		"первый пробный урок", "бесплатный аудит", "набираю учеников", "ищу учеников",
		"ищу клиентов", "можно заказать", "портфолио") {
		return true
	}
	return containsAnySemantic(text, "пишите в", "пишите мне") && hasAnySemantic(text,
		"услуг", "сайт", "бот", "crm", "разбор", "занят", "проект", "партнер", "партнеров")
}

func semanticBusinessOpinion(text string, businessContext bool) bool {
	if !businessContext {
		return false
	}
	return containsAnySemantic(text,
		"согласны или я не прав", "главный миф", "не всегда", "нужно быть", "как вы выбираете темы",
		"давайте соберем", "расскажите, какие", "у кого получается")
}

func semanticExplicitPain(text string, assessment SemanticAssessmentV2, author AuthorBusinessContext) bool {
	if assessment.NonTarget || assessment.ServiceOffer || !assessment.BusinessContextPresent || assessment.OperationalDemandSignal {
		return false
	}
	if len(assessment.Workflow.WorkflowName) == 0 {
		return false
	}
	failure := containsAnySemantic(text,
		"не работает", "не работают", "теряются", "теряем", "пропадают", "сложнее оставить",
		"сложнее купить", "ошибиться", "ошибки", "хаос", "не успева", "вручную", "долго",
		"не понимаю", "не хватает", "плохо конвер", "задолбал", "бесит", "неудоб")
	if !failure {
		return false
	}
	if author.VerifiedOwnerContext || len(assessment.BusinessContextPostEvidence) > 0 {
		return true
	}
	return false
}

func semanticWorkflowLoad(text string, workflow WorkflowEvidence, operational bool) HumanWorkflowLoad {
	if !operational {
		return ""
	}
	if containsAnySemantic(text,
		"отвечать клиентам", "обрабатывать заявки", "записывать на процедуру", "продавать в переписке",
		"координировать", "контролировать статусы", "передавать задачи", "прием и оформление заказов") {
		return WorkflowLoadHigh
	}
	if workflow.WorkflowName != "" {
		return WorkflowLoadMedium
	}
	return WorkflowLoadLow
}

func semanticConsequence(text string, workflow WorkflowEvidence) (BusinessConsequence, bool) {
	if workflow.WorkflowName == "" {
		return ConsequenceNone, false
	}
	switch {
	case containsAnySemantic(text, "заявки теряются", "теряются заявки", "теряем заявки", "пропадают заявки", "лиды теряются") ||
		(hasAnySemantic(text, "заявк", "лид") && hasAnySemantic(text, "теря", "пропада")):
		return ConsequenceLostLeads, true
	case containsAnySemantic(text, "сложнее оставить заявку", "10 заявок", "не приносит заяв"):
		return ConsequencePoorConversion, true
	case containsAnySemantic(text, "сложнее купить", "клиенту", "мешает получить") && containsAnySemantic(text, "купить", "заявк", "продаж"):
		return ConsequenceCustomerFriction, true
	case containsAnySemantic(text, "ошибиться", "ошибка", "ошибки", "легко пропустить", "слепые зоны"):
		return ConsequenceErrorRisk, true
	case containsAnySemantic(text, "хаос", "кашу", "всё одновременно"):
		return ConsequenceProcessChaos, true
	case (containsAnySemantic(text, "не успева", "куча времени", "много времени", "вручную") || hasExactSemanticToken(text, "долго")) && workflow.WorkflowName != "":
		return ConsequenceTimeCost, true
	case containsAnySemantic(text, "не отвечаем", "долго отвечаем", "ждут ответа"):
		return ConsequenceSlowResponse, true
	default:
		return ConsequenceNone, false
	}
}

func semanticITOpportunity(text string, assessment SemanticAssessmentV2) (int, ITOpportunityTier) {
	if assessment.NonTarget || assessment.ServiceOffer && !assessment.BusinessContextPresent {
		return 0, ITOpportunityLow
	}
	workflow := assessment.Workflow
	score := 0
	if workflow.WorkflowName != "" {
		score++ // repeatable workflow is visible
	}
	if hasAnySemantic(text, "telegram", "телеграм", "директ", "мессендж", "сайт", "тендер", "документ", "заявк", "лид", "онлайн") {
		score++ // digital input
	}
	if hasAnySemantic(text, "требован", "чек-лист", "чеклист", "срок", "статус", "запис", "отчет", "отчёт", "ответ", "вопрос") {
		score++ // rule-based or structured step
	}
	if assessment.HumanWorkflowLoad == WorkflowLoadHigh {
		score += 2
	} else if assessment.HumanWorkflowLoad == WorkflowLoadMedium {
		score++
	}
	if hasAnySemantic(text, "документ", "тз", "техническое задание", "заявк", "заказ", "данн", "таблиц", "срок") {
		score++ // structured data
	}
	if assessment.WorkaroundSignal || hasAnySemantic(text, "chatgpt", "claude", "crm", "битрикс", "amocrm", "n8n", "api") {
		score++
	}
	if hasAnySemantic(text, "клиент", "менеджер", "администратор", "ассистент", "координатор", "креатор", "сотруд") {
		score++ // handoff
	}
	if assessment.BusinessConsequence != ConsequenceNone {
		score++
	}
	if len(assessment.Workflow.ToolsUsed) > 0 {
		score++
	}
	if score > 10 {
		score = 10
	}
	tier := ITOpportunityLow
	if score >= 7 {
		tier = ITOpportunityHigh
	} else if score >= 4 {
		tier = ITOpportunityMedium
	}
	return score, tier
}

func semanticAutomationHypothesis(assessment SemanticAssessmentV2) string {
	if assessment.ITOpportunityScore >= 4 || assessment.HumanWorkflowLoad == WorkflowLoadHigh {
		return "parts_of_this_workflow_may_be_automatable"
	}
	return ""
}

func semanticGoldScore(assessment SemanticAssessmentV2) int {
	score := 0
	if assessment.BusinessContextPresent {
		score += 2
	}
	if assessment.Workflow.WorkflowName != "" {
		score += 2
	}
	if assessment.ExplicitBusinessPain {
		score += 2
	}
	if assessment.WorkaroundSignal {
		score += 2
	}
	if assessment.ConsequenceExplicit {
		score += 2
	}
	if assessment.ActiveSolutionSeeking {
		score += 2
	}
	if len(assessment.Workflow.ToolsUsed) > 0 {
		score++
	}
	if assessment.ITOpportunityTier == ITOpportunityHigh {
		score++
	}
	return score
}

func semanticSignalTier(assessment SemanticAssessmentV2) SemanticSignalTier {
	if assessment.ExplicitBusinessPain && assessment.GoldScore >= 10 {
		return SemanticTierGold
	}
	if (assessment.ExplicitBusinessPain || assessment.ActiveSolutionSeeking || assessment.WorkaroundSignal) && assessment.GoldScore >= 7 {
		return SemanticTierStrong
	}
	if assessment.GoldScore >= 4 {
		return SemanticTierWeak
	}
	return SemanticTierInformational
}

func semanticConfidence(assessment SemanticAssessmentV2) string {
	if assessment.ExplicitBusinessPain && assessment.Workflow.WorkflowName != "" && assessment.ConsequenceExplicit {
		return "HIGH"
	}
	if assessment.BusinessContextPresent && (assessment.OperationalDemandSignal || assessment.ActiveSolutionSeeking || assessment.WorkaroundSignal || assessment.ServiceOffer) {
		return "MEDIUM"
	}
	if assessment.BusinessContextPresent {
		return "LOW"
	}
	return "UNKNOWN"
}

func semanticEvidenceClasses(assessment SemanticAssessmentV2) []BusinessEvidenceClass {
	var classes []BusinessEvidenceClass
	if assessment.ExplicitBusinessPain {
		classes = append(classes, EvidenceExplicitBusinessPain)
	}
	if assessment.OperationalDemandSignal {
		classes = append(classes, EvidenceOperationalDemand)
	}
	if assessment.ActiveSolutionSeeking {
		classes = append(classes, EvidenceActiveSolutionSeeking)
	}
	if assessment.WorkaroundSignal {
		classes = append(classes, EvidenceWorkaround)
	}
	if assessment.BusinessOpinion {
		classes = append(classes, EvidenceBusinessOpinion)
	}
	if assessment.ServiceOffer {
		classes = append(classes, EvidenceServiceOffer)
	}
	if assessment.NonTarget {
		classes = append(classes, EvidenceNonTarget)
	}
	if len(classes) == 0 {
		classes = append(classes, EvidenceUncertain)
	}
	return classes
}

func semanticPrimaryClass(assessment SemanticAssessmentV2) BusinessEvidenceClass {
	switch {
	case assessment.NonTarget:
		return EvidenceNonTarget
	case assessment.ExplicitBusinessPain:
		return EvidenceExplicitBusinessPain
	case assessment.ActiveSolutionSeeking:
		return EvidenceActiveSolutionSeeking
	case assessment.OperationalDemandSignal:
		return EvidenceOperationalDemand
	case assessment.WorkaroundSignal:
		return EvidenceWorkaround
	case assessment.ServiceOffer:
		return EvidenceServiceOffer
	case assessment.BusinessOpinion:
		return EvidenceBusinessOpinion
	default:
		return EvidenceUncertain
	}
}

func semanticReasons(assessment SemanticAssessmentV2) []string {
	var reasons []string
	if assessment.BusinessContextPresent {
		reasons = append(reasons, "business_context_gate_passed")
	} else {
		reasons = append(reasons, "business_context_gate_not_met")
	}
	if assessment.ExplicitBusinessPain {
		reasons = append(reasons, "specific_workflow_and_failure_language")
	}
	if assessment.OperationalDemandSignal {
		reasons = append(reasons, "human_role_and_repeatable_workload")
	}
	if assessment.ActiveSolutionSeeking {
		reasons = append(reasons, "explicit_solution_or_automation_request")
	}
	if assessment.WorkaroundSignal {
		reasons = append(reasons, "current_workaround_language")
	}
	if assessment.ServiceOffer {
		reasons = append(reasons, "supply_side_offer_language")
	}
	if assessment.NonTarget {
		reasons = append(reasons, "off_target_personal_consumer_or_education_context")
	}
	if assessment.AutomationHypothesis != "" {
		reasons = append(reasons, assessment.AutomationHypothesis)
	}
	return uniqueStrings(reasons)
}

func containsAnyReason(reasons []string, values ...string) bool {
	for _, reason := range reasons {
		for _, value := range values {
			if reason == value {
				return true
			}
		}
	}
	return false
}

func hasAnySemantic(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, normalizeRussian(term)) {
			return true
		}
	}
	return false
}

func containsAnySemantic(value string, terms ...string) bool {
	return hasAnySemantic(value, terms...)
}

func hasExactSemanticToken(value, token string) bool {
	return containsTokenPhrase(relevanceTokens(value), relevanceTokens(token))
}

func hasSemanticFirstPerson(tokens []string) bool {
	for _, token := range tokens {
		switch token {
		case "я", "мне", "меня", "мой", "моя", "мое", "мы", "наш", "наша", "наше":
			return true
		}
	}
	return false
}
