package research

import (
	"sort"
	"strings"
	"unicode"
)

const (
	OwnerLikelihoodHigh    = "HIGH"
	OwnerLikelihoodMedium  = "MEDIUM"
	OwnerLikelihoodLow     = "LOW"
	OwnerLikelihoodUnknown = "UNKNOWN"

	PainStrengthHigh   = "HIGH"
	PainStrengthMedium = "MEDIUM"
	PainStrengthLow    = "LOW"

	Yes = "YES"
	No  = "NO"

	ITActionabilityHigh   = "HIGH"
	ITActionabilityMedium = "MEDIUM"
	ITActionabilityLow    = "LOW"

	BuyerIntentHigh   = "HIGH"
	BuyerIntentMedium = "MEDIUM"
	BuyerIntentLow    = "LOW"
	BuyerIntentNone   = "NONE"
)

var russianPainTypes = []string{
	"LEAD_GENERATION",
	"SALES",
	"CRM_CLIENT_MANAGEMENT",
	"CUSTOMER_COMMUNICATION",
	"BOOKING_NO_SHOW",
	"MANUAL_WORK",
	"AUTOMATION",
	"ANALYTICS_REPORTING",
	"WEBSITE_CONVERSION",
	"ECOMMERCE",
	"PAYMENTS",
	"INTEGRATIONS",
	"CONTENT_MARKETING",
	"TEAM_OPERATIONS",
	"INTERNAL_PROCESSES",
	"DATA_MANAGEMENT",
	"OTHER",
}

// BusinessPainAssessment is a deterministic, Russian-language assessment for
// small-business pain discovery. It is deliberately separate from relevance,
// commercial intent, and performance ranking.
type BusinessPainAssessment struct {
	RussianLanguage      string   `json:"russian_language"`
	OwnerLikelihood      string   `json:"owner_likelihood"`
	PainTypes            []string `json:"pain_types,omitempty"`
	PainStrength         string   `json:"pain_strength"`
	SolutionSeeking      string   `json:"solution_seeking"`
	WorkaroundPresent    string   `json:"workaround_present"`
	ToolsMentioned       []string `json:"tools_mentioned,omitempty"`
	ITActionability      string   `json:"it_actionability"`
	BuyerIntent          string   `json:"buyer_intent"`
	BusinessType         string   `json:"business_type,omitempty"`
	BusinessTypes        []string `json:"business_types,omitempty"`
	BusinessProcess      string   `json:"business_process,omitempty"`
	BusinessProcesses    []string `json:"business_processes,omitempty"`
	BusinessConsequences []string `json:"business_consequences,omitempty"`
	CurrentWorkaround    string   `json:"current_workaround,omitempty"`
	GoldPainSignal       bool     `json:"gold_pain_signal"`
	Reasons              []string `json:"business_pain_reasons,omitempty"`
}

// isRussianSmallBusinessPainTopic identifies the intentionally narrow target
// mode. It does not turn the collector into a universal multilingual searcher.
func isRussianSmallBusinessPainTopic(topic string) bool {
	if !hasCyrillic(topic) {
		return false
	}
	text := normalizeRussian(topic)
	return strings.Contains(text, "бизнес") || strings.Contains(text, "предприним") ||
		strings.Contains(text, "автомат") || strings.Contains(text, "малого") ||
		strings.Contains(text, "малый")
}

func researchLanguage(topic string) string {
	if isRussianSmallBusinessPainTopic(topic) {
		return "ru"
	}
	return "auto"
}

func AssessBusinessPain(topic, text string) BusinessPainAssessment {
	if !isRussianSmallBusinessPainTopic(topic) {
		return BusinessPainAssessment{}
	}
	normalized := normalizeRussian(text)
	tokens := relevanceTokens(normalized)
	assessment := BusinessPainAssessment{
		RussianLanguage:   No,
		OwnerLikelihood:   OwnerLikelihoodUnknown,
		PainStrength:      PainStrengthLow,
		SolutionSeeking:   No,
		WorkaroundPresent: No,
		ITActionability:   ITActionabilityLow,
		BuyerIntent:       BuyerIntentNone,
	}
	if !isRussianText(text) {
		assessment.Reasons = []string{"non_russian_or_insufficient_cyrillic_text"}
		return assessment
	}
	assessment.RussianLanguage = Yes
	assessment.OwnerLikelihood = russianOwnerLikelihood(normalized, tokens)
	assessment.BusinessTypes = russianBusinessTypes(normalized, tokens)
	if len(assessment.BusinessTypes) > 0 {
		assessment.BusinessType = assessment.BusinessTypes[0]
	}
	assessment.PainTypes = russianPainTypeMatches(normalized, tokens)
	workaround := hasRussianWorkaround(normalized)
	pain := hasRussianPain(normalized, tokens)
	solution := hasRussianSolutionSearch(normalized)
	consequences := russianBusinessConsequences(normalized, tokens)
	assessment.BusinessConsequences = consequences
	if solution {
		assessment.SolutionSeeking = Yes
	}
	if workaround {
		assessment.WorkaroundPresent = Yes
		assessment.CurrentWorkaround = russianWorkaround(normalized)
	}
	if len(assessment.PainTypes) == 0 && (pain || workaround || solution || len(consequences) > 0) {
		assessment.PainTypes = []string{"OTHER"}
	}
	assessment.ToolsMentioned = russianTools(normalized)
	assessment.BusinessProcesses = russianBusinessProcesses(normalized, tokens)
	if len(assessment.BusinessProcesses) > 0 {
		assessment.BusinessProcess = assessment.BusinessProcesses[0]
	}
	assessment.PainStrength = russianPainStrength(pain, workaround, solution, consequences)
	assessment.ITActionability = russianITActionability(assessment, normalized)
	assessment.BuyerIntent = russianBuyerIntent(normalized)
	assessment.GoldPainSignal = assessment.OwnerLikelihood == OwnerLikelihoodHigh || assessment.OwnerLikelihood == OwnerLikelihoodMedium
	assessment.GoldPainSignal = assessment.GoldPainSignal && pain && workaround && len(consequences) > 0 && solution && assessment.ITActionability == ITActionabilityHigh
	assessment.Reasons = russianPainReasons(assessment, pain, workaround, solution, consequences)
	return assessment
}

func annotateBusinessPain(topic string, posts []RankedPost) []RankedPost {
	out := make([]RankedPost, len(posts))
	copy(out, posts)
	for index := range out {
		assessment := AssessBusinessPain(topic, out[index].Post.Text)
		out[index].RussianLanguage = assessment.RussianLanguage
		out[index].OwnerLikelihood = assessment.OwnerLikelihood
		out[index].PainTypes = append([]string(nil), assessment.PainTypes...)
		out[index].PainStrength = assessment.PainStrength
		out[index].SolutionSeeking = assessment.SolutionSeeking
		out[index].WorkaroundPresent = assessment.WorkaroundPresent
		out[index].ToolsMentioned = append([]string(nil), assessment.ToolsMentioned...)
		out[index].ITActionability = assessment.ITActionability
		out[index].BuyerIntent = assessment.BuyerIntent
		out[index].BusinessType = assessment.BusinessType
		out[index].BusinessTypes = append([]string(nil), assessment.BusinessTypes...)
		out[index].BusinessProcess = assessment.BusinessProcess
		out[index].BusinessProcesses = append([]string(nil), assessment.BusinessProcesses...)
		out[index].BusinessConsequences = append([]string(nil), assessment.BusinessConsequences...)
		out[index].CurrentWorkaround = assessment.CurrentWorkaround
		out[index].GoldPainSignal = assessment.GoldPainSignal
		out[index].BusinessPainReasons = append([]string(nil), assessment.Reasons...)
	}
	return out
}

func buildBusinessPainSections(posts []RankedPost) (BusinessPainCounts, []RankedPost, []RankedPost) {
	counts := BusinessPainCounts{
		Language:              "ru",
		OwnerLikelihoodCounts: map[string]int{},
		PainTypeCounts:        map[string]int{},
		BusinessTypeCounts:    map[string]int{},
		BusinessProcessCounts: map[string]int{},
	}
	all := make([]RankedPost, 0, len(posts))
	gold := make([]RankedPost, 0)
	for _, post := range posts {
		if post.RussianLanguage != Yes {
			continue
		}
		all = append(all, post)
		counts.RussianPosts++
		counts.OwnerLikelihoodCounts[post.OwnerLikelihood]++
		if post.OwnerLikelihood == OwnerLikelihoodHigh || post.OwnerLikelihood == OwnerLikelihoodMedium {
			counts.OwnerLikelyPosts++
		}
		if post.PainStrength == PainStrengthHigh || post.PainStrength == PainStrengthMedium {
			counts.GenuinePainPosts++
		}
		if post.ITActionability == ITActionabilityHigh || post.ITActionability == ITActionabilityMedium {
			counts.ITActionablePainPosts++
		}
		if post.SolutionSeeking == Yes {
			counts.SolutionSeekingPosts++
		}
		if post.WorkaroundPresent == Yes {
			counts.WorkaroundPosts++
		}
		for _, painType := range post.PainTypes {
			counts.PainTypeCounts[painType]++
		}
		for _, businessType := range post.BusinessTypes {
			counts.BusinessTypeCounts[businessType]++
		}
		for _, businessProcess := range post.BusinessProcesses {
			counts.BusinessProcessCounts[businessProcess]++
		}
		if post.GoldPainSignal {
			counts.GoldPainSignals++
			gold = append(gold, post)
		}
	}
	return counts, gold, all
}

func russianOwnerLikelihood(text string, tokens []string) string {
	explicit := []string{
		"у меня свой бизнес", "у меня свое дело", "у меня своё дело", "у нас бизнес",
		"мой бизнес", "мой магазин", "у нас салон", "у нас студия", "у нас клиника",
		"у нас агентство", "у нас в агентстве", "у нас кофейня", "у нас школа", "я предприниматель",
		"я ип", "работаю на себя", "открыли бизнес", "веду бизнес", "владею бизнесом",
		"свой бизнес", "свое дело", "своё дело", "самозанятый", "самозанятая",
	}
	if containsRussianPhrase(text, explicit...) {
		return OwnerLikelihoodHigh
	}
	firstPerson := hasRussianFirstPerson(tokens)
	business := hasRussianBusinessAnchor(text, tokens)
	operational := hasRussianOperationalContext(text, tokens)
	if firstPerson && business && operational {
		return OwnerLikelihoodMedium
	}
	if firstPerson && (business || operational) {
		return OwnerLikelihoodMedium
	}
	if business && operational {
		return OwnerLikelihoodLow
	}
	if business {
		return OwnerLikelihoodLow
	}
	return OwnerLikelihoodUnknown
}

func russianBusinessTypes(text string, tokens []string) []string {
	var out []string
	appendIf := func(label string, match bool) {
		if match {
			out = append(out, label)
		}
	}
	appendIf("beauty", hasRussianPrefix(tokens, "салон", "студи", "косметолог", "парикмахер", "маникюр"))
	appendIf("agency", hasRussianPrefix(tokens, "агентств"))
	appendIf("retail", hasRussianPrefix(tokens, "магазин", "торгов", "розниц"))
	appendIf("restaurant", hasRussianPrefix(tokens, "кафе", "ресторан", "кофейн"))
	appendIf("education", hasRussianPrefix(tokens, "школ", "курс", "образован", "обучен"))
	appendIf("healthcare", hasRussianPrefix(tokens, "клиник", "врач", "медицин", "стоматолог"))
	appendIf("ecommerce", containsRussianPhrase(text, "интернет магазин", "интернет-магазин", "онлайн магазин"))
	appendIf("professional_services", hasRussianPrefix(tokens, "услуг", "специалист", "консульт", "юрист", "бухгалтер"))
	appendIf("creator_business", hasRussianPrefix(tokens, "блогер", "создател", "автор", "креатор"))
	appendIf("local_services", hasRussianPrefix(tokens, "мастер", "ремонт", "доставк", "сервис"))
	appendIf("other", hasRussianPrefix(tokens, "бизнес", "предприним", "ип", "самозанят"))
	return uniqueStrings(out)
}

func russianBusinessProcesses(text string, tokens []string) []string {
	var out []string
	add := func(label string, match bool) {
		if match {
			out = append(out, label)
		}
	}
	add("lead_capture", hasRussianPrefix(tokens, "заяв", "лид", "обращен", "запрос"))
	add("sales", hasRussianPrefix(tokens, "продаж", "воронк", "конверс", "сделк", "выручк"))
	add("booking", hasRussianPrefix(tokens, "запис", "бронирован", "расписан", "напомин", "неявк"))
	add("follow_up", containsRussianPhrase(text, "повторные продажи", "дожим клиентов", "вернуть клиента", "напомнить клиенту"))
	add("customer_support", hasRussianPrefix(tokens, "отвеч", "мессендж", "телеграм", "ватсап", "сообщен", "поддержк") || containsRussianPhrase(text, "директ", "автоответ"))
	add("reporting", hasRussianPrefix(tokens, "отчет", "отчёт", "показател", "считат"))
	add("payments", hasRussianPrefix(tokens, "оплат", "платеж", "платёж", "касс", "счет", "счёт"))
	add("marketing", hasRussianPrefix(tokens, "маркетинг", "реклам", "продвижен", "рассылк", "seo"))
	add("content", hasRussianPrefix(tokens, "контент", "соцсет", "публиков", "сторис"))
	add("staff_management", hasRussianPrefix(tokens, "сотруд", "задач", "контрол", "делег", "команд"))
	add("inventory", hasRussianPrefix(tokens, "склад", "остатк", "товар", "заказ", "корзин"))
	add("data_entry", hasRussianPrefix(tokens, "данн", "таблиц", "баз") || containsRussianPhrase(text, "копируем руками", "переносим данные"))
	add("analytics", hasRussianPrefix(tokens, "аналит", "окупаем", "прибыл"))
	add("operations", hasRussianPrefix(tokens, "процесс", "операцион", "хаос", "регламент", "рутин", "вручн"))
	if len(out) == 0 && (hasRussianBusinessAnchor(text, tokens) || hasRussianOperationalContext(text, tokens)) {
		out = append(out, "other")
	}
	return uniqueStrings(out)
}

func russianPainTypeMatches(text string, tokens []string) []string {
	var out []string
	add := func(label string, match bool) {
		if match {
			out = append(out, label)
		}
	}
	workaround := hasRussianWorkaround(text)
	add("LEAD_GENERATION", hasRussianPrefix(tokens, "заяв", "лид", "привлеч") || containsRussianPhrase(text, "не хватает клиентов", "клиенты не приходят", "реклама не работает"))
	add("SALES", hasRussianPrefix(tokens, "продаж", "воронк", "конверс", "сделк", "закрыва", "выручк"))
	add("CRM_CLIENT_MANAGEMENT", containsRussianPhrase(text, "crm", "амоcrm", "битрикс", "база клиентов", "вести клиентов", "учет клиентов", "учёт клиентов", "карточки клиентов"))
	add("CUSTOMER_COMMUNICATION", hasRussianPrefix(tokens, "отвеч", "мессендж", "телеграм", "ватсап", "сообщен") || containsRussianPhrase(text, "telegram", "whatsapp", "директ", "автоответ"))
	add("BOOKING_NO_SHOW", hasRussianPrefix(tokens, "запис", "напомин", "неявк") || containsRussianPhrase(text, "не приходят на запись", "не приходят на запис", "отмена записи"))
	add("MANUAL_WORK", workaround)
	add("AUTOMATION", hasRussianPrefix(tokens, "автоматиз", "бот", "робот", "сервис", "систем"))
	add("ANALYTICS_REPORTING", hasRussianPrefix(tokens, "аналит", "окупаем", "показател", "отчет", "отчёт", "считат", "прибыл"))
	add("WEBSITE_CONVERSION", hasRussianPrefix(tokens, "сайт", "лендинг", "страниц") || containsRussianPhrase(text, "не приносит заявки", "нет заявок с сайта", "сайт не продает", "сайт не продаёт"))
	add("ECOMMERCE", containsRussianPhrase(text, "интернет магазин", "интернет-магазин", "брошенные корзины", "оформить заказ", "принимать заказы") || hasRussianPrefix(tokens, "заказ", "корзин"))
	add("PAYMENTS", hasRussianPrefix(tokens, "оплат", "платеж", "платёж", "касс", "счет", "счёт"))
	add("INTEGRATIONS", hasRussianPrefix(tokens, "интеграц", "синхрон") || containsRussianPhrase(text, "как связать", "1с", "amocrm", "amo crm", "битрикс24"))
	add("CONTENT_MARKETING", hasRussianPrefix(tokens, "контент", "соцсет", "публиков", "маркетинг", "реклам", "сторис"))
	add("TEAM_OPERATIONS", hasRussianPrefix(tokens, "сотруд", "задач", "контрол", "делег") || containsRussianPhrase(text, "все задачи в чатах", "задачи теряются"))
	add("INTERNAL_PROCESSES", hasRussianPrefix(tokens, "процесс", "операцион", "хаос", "регламент", "рутин"))
	add("DATA_MANAGEMENT", hasRussianPrefix(tokens, "данн", "таблиц", "баз") || containsRussianPhrase(text, "копируем руками", "переносим данные"))
	return uniqueStrings(out)
}

func hasRussianPain(text string, tokens []string) bool {
	return hasRussianPrefix(tokens, "бесит", "задолб", "сложн", "трудн", "теря", "успева", "долг", "вручн", "дорог", "хаос", "ошиб", "забыва", "неудобн", "надоел", "устал", "злит") ||
		containsRussianPhrase(text, "не получается", "не получаются", "не работает", "не работают", "постоянно проблема", "куча времени", "слишком долго", "не понимаю", "не хватает", "не успеваем")
}

func hasRussianWorkaround(text string) bool {
	return containsRussianPhrase(text,
		"ведем в excel", "ведём в excel", "все ведем вручную", "всё ведем вручную", "все в таблицах",
		"ведем клиентов в таблице", "ведём клиентов в таблице", "веду клиентов в excel", "веду клиентов в эксель",
		"записываем вручную", "пишем каждому",
		"сами переносим", "копируем руками", "проверяем вручную", "вручную собираю", "сам отвечаю всем",
		"несколько таблиц", "в директе", "в чатах", "в блокноте", "на бумаге",
	) || hasRussianPrefix(relevanceTokens(text), "вручн", "копир", "переносим", "таблиц")
}

func hasRussianSolutionSearch(text string) bool {
	return containsRussianPhrase(text,
		"посоветуйте сервис", "посоветуйте crm", "какую crm выбрать", "crm для малого бизнеса",
		"нужна crm", "нужен бот", "есть ли сервис", "чем заменить", "кто может настроить",
		"ищу разработчика", "как автоматизировать", "что автоматизировать", "как убрать ручную работу",
		"как связать", "как настроить", "кто чем пользуется", "как решить", "что делать",
		"кто сталкивался", "кто может внедрить", "нужна онлайн запись", "нужна онлайн-запись",
	)
}

func russianBusinessConsequences(text string, tokens []string) []string {
	var out []string
	add := func(label string, match bool) {
		if match {
			out = append(out, label)
		}
	}
	add("LOST_LEADS_OR_REQUESTS", containsRussianPhrase(text, "теряем заявки", "теряются заявки", "заявки теряются", "лиды теряются", "теряем лиды", "пропадают заявки"))
	add("LOST_CLIENTS", containsRussianPhrase(text, "теряем клиентов", "клиенты уходят"))
	add("MISSED_APPOINTMENTS", containsRussianPhrase(text, "забывают про запись", "не приходят на запись", "неявки клиентов", "клиенты не приходят"))
	add("SLOW_RESPONSES", containsRussianPhrase(text, "не успеваем отвечать", "долго отвечаем", "клиенты долго ждут ответа", "не отвечаем вовремя"))
	add("PROCESS_ERRORS_OR_DUPLICATES", hasRussianPrefix(tokens, "ошиб", "дубли") || containsRussianPhrase(text, "данные не синхронизируются"))
	add("TIME_LOST_TO_ROUTINE", containsRussianPhrase(text, "куча времени", "много времени", "время уходит", "съедает время", "не успеваю"))
	add("UNKNOWN_MARKETING_PAYBACK", containsRussianPhrase(text, "не понимаю окупаемость", "не понимаю откуда клиенты", "не понимаю какая реклама работает"))
	add("WORK_OVERLOAD_OR_CHAOS", containsRussianPhrase(text, "хаос в процессах", "задачи теряются", "операционка съедает время"))
	return uniqueStrings(out)
}

func russianTools(text string) []string {
	tools := []struct {
		label string
		terms []string
	}{
		{"CRM", []string{"crm", "црм"}},
		{"amoCRM", []string{"amocrm", "amo crm", "амоcrm", "амо crm"}},
		{"Bitrix24", []string{"битрикс", "битрикс24"}},
		{"Telegram", []string{"telegram", "телеграм"}},
		{"WhatsApp", []string{"whatsapp", "ватсап"}},
		{"Excel", []string{"excel", "эксель"}},
		{"Google Sheets", []string{"google sheets", "гугл таблиц"}},
		{"Notion", []string{"notion"}},
		{"1C", []string{"1с"}},
		{"Instagram Direct", []string{"директ", "instagram"}},
		{"Tilda", []string{"tilda", "тильда"}},
		{"YClients", []string{"yclients", "yclients"}},
	}
	var out []string
	for _, tool := range tools {
		if containsRussianPhrase(text, tool.terms...) {
			out = append(out, tool.label)
		}
	}
	return out
}

func russianPainStrength(pain, workaround, solution bool, consequences []string) string {
	if pain && (workaround || len(consequences) > 0) {
		return PainStrengthHigh
	}
	if pain || workaround || len(consequences) > 0 {
		return PainStrengthMedium
	}
	if solution {
		return PainStrengthLow
	}
	return PainStrengthLow
}

func russianITActionability(assessment BusinessPainAssessment, text string) string {
	digital := len(assessment.ToolsMentioned) > 0 || hasRussianPrefix(relevanceTokens(text), "автоматиз", "бот", "интеграц", "систем", "сайт", "сервис") ||
		len(assessment.PainTypes) > 0 && !onlyLowActionabilityPain(assessment.PainTypes)
	if digital && (assessment.WorkaroundPresent == Yes || assessment.SolutionSeeking == Yes || len(assessment.BusinessConsequences) > 0) {
		return ITActionabilityHigh
	}
	if digital {
		return ITActionabilityMedium
	}
	return ITActionabilityLow
}

func onlyLowActionabilityPain(types []string) bool {
	if len(types) == 0 {
		return true
	}
	for _, painType := range types {
		switch painType {
		case "LEAD_GENERATION", "SALES", "CRM_CLIENT_MANAGEMENT", "CUSTOMER_COMMUNICATION", "BOOKING_NO_SHOW",
			"MANUAL_WORK", "AUTOMATION", "ANALYTICS_REPORTING", "WEBSITE_CONVERSION", "ECOMMERCE", "PAYMENTS",
			"INTEGRATIONS", "CONTENT_MARKETING", "TEAM_OPERATIONS", "INTERNAL_PROCESSES", "DATA_MANAGEMENT":
			return false
		}
	}
	return true
}

func russianBuyerIntent(text string) string {
	if containsRussianPhrase(text, "ищу разработчика", "кто может настроить", "кто может внедрить", "нужен бот", "нужна crm", "заказать", "оплатить") {
		return BuyerIntentHigh
	}
	if containsRussianPhrase(text, "посоветуйте сервис", "какую crm выбрать", "какую crm", "есть ли сервис", "чем заменить", "кто чем пользуется") {
		return BuyerIntentMedium
	}
	if containsRussianPhrase(text, "как автоматизировать", "что автоматизировать", "хочу автоматизировать", "надо автоматизировать") {
		return BuyerIntentLow
	}
	return BuyerIntentNone
}

func russianWorkaround(text string) string {
	switch {
	case containsRussianPhrase(text, "excel", "эксель", "таблиц"):
		return "ведут данные в Excel/таблицах"
	case containsRussianPhrase(text, "директ", "telegram", "телеграм", "whatsapp", "ватсап", "мессендж"):
		return "обрабатывают обращения вручную в чатах/мессенджерах"
	case containsRussianPhrase(text, "пишем каждому", "сам отвечаю всем"):
		return "пишут или отвечают каждому вручную"
	case containsRussianPhrase(text, "копируем", "переносим"):
		return "копируют или переносят данные руками"
	case containsRussianPhrase(text, "записываем вручную", "вручную"):
		return "ведут процесс вручную"
	default:
		return "ручной или разрозненный процесс"
	}
}

func russianPainReasons(assessment BusinessPainAssessment, pain, workaround, solution bool, consequences []string) []string {
	var reasons []string
	if assessment.RussianLanguage == Yes {
		reasons = append(reasons, "russian_language_context")
	}
	if assessment.OwnerLikelihood == OwnerLikelihoodHigh || assessment.OwnerLikelihood == OwnerLikelihoodMedium {
		reasons = append(reasons, "owner_or_operator_context")
	} else if assessment.OwnerLikelihood == OwnerLikelihoodLow {
		reasons = append(reasons, "weak_owner_context")
	}
	if pain {
		reasons = append(reasons, "explicit_pain_language")
	}
	if workaround {
		reasons = append(reasons, "manual_workaround_language")
	}
	if solution {
		reasons = append(reasons, "solution_search_language")
	}
	if len(consequences) > 0 {
		reasons = append(reasons, "business_consequence_language")
	}
	if len(assessment.ToolsMentioned) > 0 {
		reasons = append(reasons, "named_tool_or_service")
	}
	if assessment.ITActionability == ITActionabilityHigh {
		reasons = append(reasons, "high_it_actionability")
	} else if assessment.ITActionability == ITActionabilityMedium {
		reasons = append(reasons, "medium_it_actionability")
	}
	if assessment.BuyerIntent != BuyerIntentNone {
		reasons = append(reasons, "buyer_intent:"+assessment.BuyerIntent)
	}
	if assessment.GoldPainSignal {
		reasons = append(reasons, "gold_pain_signal")
	}
	sort.Strings(reasons)
	return reasons
}

func hasRussianBusinessAnchor(text string, tokens []string) bool {
	return containsRussianPhrase(text, "свой бизнес", "свое дело", "своё дело", "мой бизнес", "мой магазин", "у нас", "я ип") ||
		hasRussianPrefix(tokens, "бизнес", "магазин", "салон", "студи", "клиник", "агентств", "кофейн", "кафе", "ресторан", "школ", "предприним", "самозанят", "ип", "владел", "основател", "директор", "управля", "руководител")
}

func hasRussianOperationalContext(text string, tokens []string) bool {
	return hasRussianPrefix(tokens, "клиент", "заяв", "заказ", "сотруд", "продаж", "запис", "реклам", "процесс", "таблиц", "мессендж", "операцион") ||
		containsRussianPhrase(text, "откуда приходят клиенты", "как вести клиентов", "обрабатывать заявки")
}

func hasRussianFirstPerson(tokens []string) bool {
	for _, token := range tokens {
		switch token {
		case "я", "мне", "мой", "моя", "мое", "моё", "мы", "у", "наш", "наша", "наше", "сами", "сам", "сама":
			return true
		}
	}
	return false
}

func hasRussianPrefix(tokens []string, prefixes ...string) bool {
	for _, token := range tokens {
		for _, prefix := range prefixes {
			if strings.HasPrefix(token, prefix) {
				return true
			}
		}
	}
	return false
}

func containsRussianPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, normalizeRussian(phrase)) {
			return true
		}
	}
	return false
}

func normalizeRussian(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(value, "ё", "е"))), " ")
}

func hasCyrillic(value string) bool {
	for _, r := range value {
		if (r >= 'а' && r <= 'я') || r == 'ё' || (r >= 'А' && r <= 'Я') || r == 'Ё' {
			return true
		}
	}
	return false
}

func isRussianText(value string) bool {
	var cyrillic, letters int
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if (r >= 'а' && r <= 'я') || r == 'ё' || (r >= 'А' && r <= 'Я') || r == 'Ё' {
			cyrillic++
		}
	}
	return cyrillic >= 3 && letters > 0 && float64(cyrillic)/float64(letters) >= 0.25
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
