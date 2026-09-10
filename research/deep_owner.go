package research

import (
	"sort"
	"strings"
)

// deepOwnerVerification is the collection-time gate used before a profile's
// recent feed is fetched. It is deliberately narrower than a generic
// business-topic classifier: a search hit is only a lead until the profile
// and its seed posts corroborate an owner/operator context.
type deepOwnerVerification struct {
	Verified           bool
	Confidence         string
	Reasons            []string
	NegativeEvidence   []string
	BusinessCategories []string
}

func verifyDeepOwnerProfile(profile Author, seedTexts []string) deepOwnerVerification {
	profileText := normalizeRussian(strings.TrimSpace(strings.Join([]string{profile.Name, profile.Bio}, " ")))
	allTexts := append([]string{profileText}, seedTexts...)
	categories := deepBusinessCategories(allTexts...)

	profileRole := deepOwnerRole(profileText)
	profileIdentity := deepBusinessIdentity(profileText)
	profileCommercial := deepCommercialActivity(profileText)
	profileTeam := deepTeamEvidence(profileText)
	profileOperations := deepOperationalEvidence(profileText)

	seedFirstPerson := 0
	seedOwnership := 0
	seedCommercial := 0
	seedOperational := 0
	for _, seedText := range seedTexts {
		text := normalizeRussian(seedText)
		if text == "" {
			continue
		}
		if hasRussianFirstPerson(relevanceTokens(text)) {
			seedFirstPerson++
		}
		if deepBusinessIdentity(text) || deepOwnerRole(text) {
			seedOwnership++
		}
		if deepCommercialActivity(text) {
			seedCommercial++
		}
		if deepOperationalEvidence(text) {
			seedOperational++
		}
	}

	var reasons []string
	addReason := func(reason string, condition bool) {
		if condition {
			reasons = append(reasons, reason)
		}
	}
	addReason("profile_owner_or_operator_role", profileRole)
	addReason("profile_business_identity", profileIdentity)
	addReason("profile_concrete_business_type", len(categories) > 0)
	addReason("profile_commercial_activity", profileCommercial)
	addReason("profile_team_or_staff_context", profileTeam)
	addReason("profile_operational_responsibility", profileOperations)
	addReason("seed_first_person_business_context", seedFirstPerson > 0)
	addReason("seed_explicit_owner_or_business_identity", seedOwnership > 0)
	addReason("repeated_seed_commercial_context", seedCommercial >= 2)
	addReason("repeated_seed_operational_context", seedOperational >= 2)

	negative := []string{}
	educationOnly := deepEducationOnly(profileText, seedTexts, profileRole, profileTeam, profileOperations)
	if educationOnly {
		negative = append(negative, "education_or_course_only_without_operating_evidence")
	}
	if profileCommercial && !profileRole && !profileIdentity && seedOperational == 0 {
		negative = append(negative, "commercial_service_signal_without_operator_evidence")
	}
	if !profileRole && !profileIdentity && seedOwnership == 0 {
		negative = append(negative, "no_explicit_owner_or_business_identity")
	}

	verified := false
	confidence := ""
	switch {
	case !educationOnly && profileRole && (profileCommercial || profileTeam || profileOperations || len(categories) > 0 || seedCommercial > 0):
		verified = true
		confidence = "HIGH"
		reasons = append(reasons, "accepted_profile_role_with_business_evidence")
	case !educationOnly && profileIdentity && (profileCommercial || profileTeam || profileOperations || seedCommercial > 0 || seedOperational > 0) && (seedFirstPerson > 0 || profileTeam || profileOperations):
		verified = true
		confidence = "HIGH"
		reasons = append(reasons, "accepted_business_identity_with_operating_evidence")
	case !educationOnly && seedOwnership > 0 && seedCommercial > 0 && (seedOperational > 0 || profileCommercial || profileIdentity):
		verified = true
		confidence = "MEDIUM"
		reasons = append(reasons, "accepted_seed_owner_and_commercial_evidence")
	case !educationOnly && seedFirstPerson >= 2 && seedCommercial >= 2 && (profileCommercial || profileIdentity || profileOperations):
		verified = true
		confidence = "MEDIUM"
		reasons = append(reasons, "accepted_repeated_first_person_commercial_context")
	case !educationOnly && len(categories) > 0 && profileCommercial && seedFirstPerson > 0:
		verified = true
		confidence = "MEDIUM"
		reasons = append(reasons, "accepted_business_type_with_first_person_commercial_context")
	}
	if !verified {
		reasons = append(reasons, "insufficient_owner_operator_evidence")
	}

	sort.Strings(reasons)
	sort.Strings(negative)
	return deepOwnerVerification{
		Verified:           verified,
		Confidence:         confidence,
		Reasons:            uniqueStrings(reasons),
		NegativeEvidence:   uniqueStrings(negative),
		BusinessCategories: categories,
	}
}

func deepOwnerRole(text string) bool {
	return containsRussianPhrase(text,
		"владелец", "владелица", "основатель", "основала", "сооснователь", "директор",
		"управляющий", "управляющая", "руководитель", "предприниматель", "я ип",
		"самозанятый", "самозанятая", "работаю на себя", "фрилансер", "owner", "founder",
	)
}

func deepBusinessIdentity(text string) bool {
	if text == "" {
		return false
	}
	return containsRussianPhrase(text,
		"мой бизнес", "свой бизнес", "моя студия", "мой салон", "мой магазин",
		"у нас бизнес", "у нас салон", "у нас студия", "у нас агентство", "наша команда",
		"у нас сотрудники", "открыли бизнес", "открыл бизнес", "открыла бизнес", "веду бизнес",
		"владею бизнесом", "наш бизнес", "наш проект",
	) || deepOwnerRole(text)
}

func deepCommercialActivity(text string) bool {
	return hasRussianPrefix(relevanceTokens(text),
		"клиент", "заказ", "продаж", "услуг", "цен", "прайс", "запис", "бронир",
		"оплат", "товар", "выруч", "покупател", "договор", "проект", "заказчик",
	) || containsRussianPhrase(text, "оказываю услуги", "принимаем заказы", "работаем с клиентами", "для клиентов")
}

func deepTeamEvidence(text string) bool {
	return hasRussianPrefix(relevanceTokens(text), "команд", "сотруд", "менеджер", "администратор", "персонал", "штат") ||
		containsRussianPhrase(text, "у нас сотрудники", "нанимаем людей", "расширяем команду", "отдел продаж")
}

func deepOperationalEvidence(text string) bool {
	return hasRussianPrefix(relevanceTokens(text),
		"заяв", "лид", "клиент", "заказ", "запис", "достав", "склад", "постав",
		"сотруд", "команд", "директ", "сообщен", "таблиц", "отчет", "распис",
	) || containsRussianPhrase(text, "вручную", "операционка", "рабочий процесс", "прием заказов", "приём заказов")
}

func deepEducationOnly(profileText string, seedTexts []string, role, team, operations bool) bool {
	all := normalizeRussian(strings.Join(append([]string{profileText}, seedTexts...), " "))
	if !hasRussianPrefix(relevanceTokens(all), "школ", "курс", "обучен", "ученик", "репетитор", "урок", "английск") {
		return false
	}
	if role || team || operations || containsRussianPhrase(all, "принимаем заказы", "у нас сотрудники", "моя школа", "наш бизнес") {
		return false
	}
	return !hasRussianPrefix(relevanceTokens(all), "заказ", "клиент", "запис", "продаж", "оплат", "выруч")
}

func deepBusinessCategories(texts ...string) []string {
	joined := normalizeRussian(strings.Join(texts, " "))
	tokens := relevanceTokens(joined)
	var categories []string
	add := func(category string, match bool) {
		if match {
			categories = append(categories, category)
		}
	}
	add("beauty_wellness", hasRussianPrefix(tokens, "салон", "барбершоп", "косметолог", "маникюр", "массаж", "парикмахер"))
	add("fitness", hasRussianPrefix(tokens, "фитнес", "спортзал", "йог", "тренер"))
	add("retail_ecommerce", hasRussianPrefix(tokens, "магазин", "шоурум", "розниц", "торгов") || containsRussianPhrase(joined, "интернет магазин", "интернет-магазин", "онлайн магазин"))
	add("food_hospitality", hasRussianPrefix(tokens, "кофейн", "кафе", "ресторан", "бар", "пекарн"))
	add("health_clinics", hasRussianPrefix(tokens, "клиник", "стоматолог", "медицин", "ветклиник", "ветеринар"))
	add("agencies", hasRussianPrefix(tokens, "агентств", "продакшн", "smm") || containsRussianPhrase(joined, "веб студия", "дизайн студия", "маркетинговое агентство"))
	add("real_estate", hasRussianPrefix(tokens, "недвижим", "риелтор", "агент"))
	add("auto", hasRussianPrefix(tokens, "автосервис", "автомойк", "сто "))
	add("education", hasRussianPrefix(tokens, "школ", "курс", "обучен", "репетитор", "ученик", "урок"))
	add("professional_services", hasRussianPrefix(tokens, "юрист", "бухгалтер", "консалт", "консульт", "услуг", "специалист"))
	add("local_services", hasRussianPrefix(tokens, "ремонт", "строй", "мастерск", "достав", "цветочн", "сервис"))
	add("creator_business", hasRussianPrefix(tokens, "блогер", "создател", "автор", "креатор"))
	if len(categories) == 0 && (deepOwnerRole(joined) || deepBusinessIdentity(joined) || hasRussianPrefix(tokens, "бизнес", "ип", "самозанят")) {
		categories = append(categories, "other_small_business")
	}
	return uniqueStrings(categories)
}
