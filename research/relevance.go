package research

import (
	"sort"
	"strings"
	"unicode"
)

const (
	relevanceScoreRelevant   = 0.90
	relevanceScoreAdjacent   = 0.45
	relevanceScoreUncertain  = 0.20
	relevanceScoreIrrelevant = 0.00
)

// AssessRelevance applies the explainable, local topic gate used before
// profile enrichment and ranking. It intentionally looks only at the topic
// and post text; no provider or profile metadata is consulted.
func AssessRelevance(topic, text string) RelevanceAssessment {
	return assessRelevance(topic, "", text)
}

// AssessRelevanceForQuery is the query-aware form used by the collection
// pipeline. Query text can add an audit reason, but never turns a post into a
// relevant result by itself. This prevents a broad query such as "freelancers"
// from contaminating a client-acquisition topic.
func AssessRelevanceForQuery(topic, query, text string) RelevanceAssessment {
	return assessRelevance(topic, query, text)
}

func assessRelevance(topic, query, text string) RelevanceAssessment {
	postTokens := relevanceTokens(text)
	if len(postTokens) == 0 {
		return RelevanceAssessment{
			Score:   relevanceScoreUncertain,
			Label:   RelevanceUncertain,
			Reasons: []string{"missing_or_nontextual_post"},
		}
	}

	topicTokens := relevanceTopicTokens(topic)
	if len(topicTokens) == 0 {
		return RelevanceAssessment{
			Score:   relevanceScoreUncertain,
			Label:   RelevanceUncertain,
			Reasons: []string{"topic_has_no_meaningful_terms"},
		}
	}

	reasons := make([]string, 0, 8)
	if queryContextMatches(query, postTokens, topicTokens) {
		reasons = append(reasons, "query_context_match")
	}
	phraseMatch := normalizedPhrase(topic) != "" && containsTokenPhrase(postTokens, relevanceTokens(topic))
	if phraseMatch {
		reasons = append(reasons, "topic_phrase_match")
	}

	if isRussianSmallBusinessPainTopic(topic) {
		return assessRussianSmallBusinessPain(text, reasons)
	}
	if isClientAcquisitionTopic(topic) {
		return assessClientAcquisition(text, postTokens, reasons)
	}
	if isSmallBusinessAutomationTopic(topic) {
		return assessSmallBusinessAutomation(postTokens, reasons)
	}
	if phraseMatch {
		return relevantAssessment(reasons)
	}

	matchedTerms, matchedPostTokens := matchTopicTerms(topicTokens, postTokens)
	if len(matchedTerms) >= 2 && len(matchedPostTokens) >= 2 {
		reasons = append(reasons, topicMatchReasons(matchedTerms)...)
		if missing := missingTopicAnchors(topicTokens, postTokens); len(missing) > 0 {
			reasons = append(reasons, topicAnchorReasons(missing)...)
			return adjacentAssessment(reasons)
		}
		return relevantAssessment(reasons)
	}
	if len(matchedTerms) == 1 {
		reasons = append(reasons, topicMatchReasons(matchedTerms)...)
		return adjacentAssessment(reasons)
	}
	return irrelevantAssessment([]string{"no_topic_signal"})
}

// assessRussianSmallBusinessPain is the narrow deterministic relevance gate
// for the Russian owner/operator validation milestone. Query wording alone is
// never sufficient: the post must expose a concrete pain, workaround, active
// solution search, or business consequence in a business context.
func assessRussianSmallBusinessPain(text string, reasons []string) RelevanceAssessment {
	if !isRussianText(text) {
		return uncertainAssessment(append(reasons, "insufficient_russian_text"))
	}
	assessment := AssessBusinessPain("проблемы малого бизнеса автоматизация", text)
	if assessment.RussianLanguage == Yes {
		reasons = append(reasons, "russian_language")
	}
	if assessment.OwnerLikelihood != OwnerLikelihoodUnknown {
		reasons = append(reasons, "owner_likelihood:"+assessment.OwnerLikelihood)
	}
	if len(assessment.PainTypes) > 0 {
		reasons = append(reasons, "business_pain_type")
	}
	if assessment.SolutionSeeking == Yes {
		reasons = append(reasons, "solution_seeking")
	}
	if assessment.WorkaroundPresent == Yes {
		reasons = append(reasons, "workaround_present")
	}
	if len(assessment.BusinessConsequences) > 0 {
		reasons = append(reasons, "business_consequence")
	}
	usefulEvidence := assessment.PainStrength != PainStrengthLow ||
		assessment.SolutionSeeking == Yes || assessment.WorkaroundPresent == Yes ||
		len(assessment.BusinessConsequences) > 0
	businessContext := assessment.OwnerLikelihood != OwnerLikelihoodUnknown ||
		len(assessment.BusinessTypes) > 0 || hasRussianBusinessAnchor(normalizeRussian(text), relevanceTokens(normalizeRussian(text)))
	if usefulEvidence && businessContext {
		return relevantAssessment(reasons)
	}
	if usefulEvidence || businessContext {
		return adjacentAssessment(reasons)
	}
	return irrelevantAssessment(append(reasons, "no_owner_or_business_pain_signal"))
}

func assessClientAcquisition(text string, postTokens []string, reasons []string) RelevanceAssessment {
	clientSignal := hasFamily(postTokens, "client")
	freelanceSignal := hasFamily(postTokens, "freelance")
	buyerSignal := hasBuyerHiringSignal(text, postTokens)
	acquisitionContext := hasClientAcquisitionEvidence(text, postTokens)
	clientTargetingContext := hasClientTargetingContext(postTokens)
	customerServiceContext := hasCustomerServiceContext(postTokens)
	existingClientContext := hasExistingClientContext(postTokens)
	nonClientRecruitment := hasNonClientRecruitmentContext(postTokens)
	genericAcquisitionContext := hasGenericAcquisitionContext(postTokens)
	serviceSignal := hasServiceAnchor(postTokens)
	targetAudienceSeeking := hasClientSeekingTargetAudience(postTokens)

	if clientSignal {
		reasons = append(reasons, "client_family")
	}
	if freelanceSignal {
		reasons = append(reasons, "freelance_family")
	}
	if buyerSignal {
		reasons = append(reasons, "buyer_service_request")
	}
	if acquisitionContext {
		reasons = append(reasons, "client_acquisition_context")
	}
	if serviceSignal {
		reasons = append(reasons, "service_or_domain_anchor")
	}
	if clientTargetingContext {
		reasons = append(reasons, "client_targeting_without_acquisition")
	}
	if targetAudienceSeeking {
		reasons = append(reasons, "client_seeking_target_audience")
	}

	// A post recruiting providers who themselves want clients can be useful for
	// audience discovery, but it is not evidence that the author is seeking
	// clients or asking for an acquisition method.
	if targetAudienceSeeking {
		return adjacentAssessment(reasons)
	}
	// Retention/LTV commentary may mention acquisition while actually discussing
	// existing-customer economics. Keep it reviewable without treating it as
	// core evidence for finding clients.
	if existingClientContext && !hasCoreAcquisitionBehavior(text, postTokens) {
		if genericAcquisitionContext {
			reasons = append(reasons, "retention_dominant_acquisition_context")
			return adjacentAssessment(reasons)
		}
		reasons = append(reasons, "existing_client_or_retention_context")
		return irrelevantAssessment(reasons)
	}

	// A buyer can be discovered without the word "client": a concrete request
	// for a freelancer or service provider is itself topic evidence.
	if buyerSignal {
		return relevantAssessment(reasons)
	}
	// Require an acquisition behavior, pain, question, method, or offer in
	// addition to a broad client/freelancer word. This prevents ordinary client
	// references and customer-service language from entering core evidence.
	if (clientSignal || freelanceSignal) && acquisitionContext {
		return relevantAssessment(reasons)
	}
	// Generic acquisition language is useful for review, but without a client,
	// freelancer, or service-request anchor it is not core evidence.
	if genericAcquisitionContext {
		reasons = append(reasons, "generic_acquisition_without_client_context")
		return adjacentAssessment(reasons)
	}
	if customerServiceContext {
		reasons = append(reasons, "customer_service_context")
		return irrelevantAssessment(reasons)
	}
	if existingClientContext {
		reasons = append(reasons, "existing_client_or_retention_context")
		return irrelevantAssessment(reasons)
	}
	if nonClientRecruitment {
		reasons = append(reasons, "non_client_recruitment")
		return irrelevantAssessment(reasons)
	}
	if clientTargetingContext {
		return adjacentAssessment(reasons)
	}
	if clientSignal || freelanceSignal {
		reasons = append(reasons, "client_reference_without_acquisition")
		return adjacentAssessment(reasons)
	}
	if serviceSignal {
		reasons = append(reasons, "service_without_client_acquisition")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no_topic_signal")
	}
	return irrelevantAssessment(reasons)
}

func hasClientAcquisitionEvidence(text string, tokens []string) bool {
	if hasClientSeekingTargetAudience(tokens) {
		return false
	}
	if hasBuyerHiringSignal(text, tokens) {
		return true
	}
	if hasActiveClientRequestForService(tokens) {
		return true
	}
	if containsAnyPhrase(tokens,
		"client acquisition", "customer acquisition", "lead generation", "client outreach",
		"find clients", "find a client", "find customers", "get clients", "get customers",
		"looking for clients", "looking for a client", "need clients", "need a client",
		"more clients", "new client", "new clients", "first clients", "grow clientele",
		"find clientele", "attract clients", "attracting clients", "land clients",
		"turn followers into clients", "freelance work", "paid work",
	) && hasClientAcquisitionDetail(text, tokens) {
		return true
	}
	if !hasFamily(tokens, "client") && !hasFamily(tokens, "freelance") {
		return false
	}
	if hasAcquisitionPainSignal(text, tokens) || hasAcquisitionQuestionSignal(text, tokens) ||
		hasAcquisitionMethodSignal(text, tokens) || hasAcquisitionServiceOfferSignal(text, tokens) {
		return true
	}
	return hasClientAcquisitionActionNearClient(tokens)
}

func hasActiveClientRequestForService(tokens []string) bool {
	if hasClientSeekingTargetAudience(tokens) {
		return false
	}
	if !hasFamily(tokens, "client") || !hasServiceAnchor(tokens) {
		return false
	}
	for requestIndex, token := range tokens {
		stem := relevanceStem(token)
		if stem != "need" && stem != "look" && stem != "seek" && stem != "want" {
			continue
		}
		for clientIndex := requestIndex + 1; clientIndex < len(tokens); clientIndex++ {
			if clientIndex-requestIndex > 6 {
				break
			}
			if tokenFamily(tokens[clientIndex]) == "client" {
				return true
			}
		}
	}
	return false
}

func hasClientSeekingTargetAudience(tokens []string) bool {
	return containsAnyPhrase(tokens,
		"who are looking for clients", "who is looking for clients",
		"looking for clients currently", "looking for clients right now",
		"coaches looking for clients", "coaches who are looking for clients",
	)
}

func hasCoreAcquisitionBehavior(text string, tokens []string) bool {
	return hasBuyerHiringSignal(text, tokens) || hasActiveClientRequestForService(tokens) ||
		hasClientSeekingSignal(text, tokens, false) || hasAcquisitionPainSignal(text, tokens) ||
		hasAcquisitionQuestionSignal(text, tokens) || hasAcquisitionMethodSignal(text, tokens) ||
		hasAcquisitionServiceOfferSignal(text, tokens) || containsAnyPhrase(tokens,
		"best client acquisition strategy", "finding clients is easy", "easy finding clients",
		"client acquisition strategy",
	)
}

func hasClientAcquisitionDetail(text string, tokens []string) bool {
	if containsAnyPhrase(tokens,
		"new client", "new clients", "more clients", "first clients", "for my digital products",
		"for web development services", "turn followers into clients", "target clients",
		"best client acquisition strategy", "finding clients is easy", "easy finding clients",
		"goal of customer acquisition", "customer acquisition should", "client acquisition strategy",
	) {
		return true
	}
	if requestedServiceRole(text, tokens) != "" || hasServiceAnchor(tokens) {
		return true
	}
	return hasAcquisitionPainSignal(text, tokens) || hasAcquisitionQuestionSignal(text, tokens) ||
		hasAcquisitionMethodSignal(text, tokens) || hasAcquisitionServiceOfferSignal(text, tokens)
}

func hasClientAcquisitionActionNearClient(tokens []string) bool {
	clientIndexes := make([]int, 0, 2)
	for index, token := range tokens {
		if tokenFamily(token) == "client" {
			clientIndexes = append(clientIndexes, index)
		}
	}
	if len(clientIndexes) == 0 {
		return false
	}
	for index, token := range tokens {
		stem := relevanceStem(token)
		// Bare "looking for clients" / "need clients" remains adjacent
		// unless another detail (role, pain, question, or method) exists.
		if stem == "look" || stem == "need" || stem == "want" || stem == "seek" {
			continue
		}
		if !hasAcquisitionVerbStem(stem) {
			continue
		}
		for _, clientIndex := range clientIndexes {
			if absInt(index-clientIndex) <= 5 {
				return true
			}
		}
	}
	return false
}

func hasAcquisitionVerbStem(stem string) bool {
	for _, prefix := range []string{
		"find", "found", "get", "look", "seek", "need", "want", "acquir", "outreach",
		"contact", "land", "win", "attract", "generat", "convert", "clos", "grow",
		"market", "promot", "refer", "network", "email", "post", "launch", "sell",
	} {
		if strings.HasPrefix(stem, prefix) {
			return true
		}
	}
	return false
}

func hasClientTargetingContext(tokens []string) bool {
	return containsAnyPhrase(tokens,
		"target client", "target clients", "target client type", "potential clients",
		"sell it as a service", "sell services", "service package",
	)
}

func hasCustomerServiceContext(tokens []string) bool {
	if containsAnyPhrase(tokens, "customer service", "customer support", "client support", "support ticket") {
		return true
	}
	if !hasFamily(tokens, "client") {
		return false
	}
	return containsAnyPhrase(tokens, "complaint", "refund", "waiter", "restaurant", "delivery", "order", "bad service")
}

func hasExistingClientContext(tokens []string) bool {
	if !hasFamily(tokens, "client") && !hasFamily(tokens, "customer") {
		return false
	}
	return containsAnyPhrase(tokens,
		"existing client", "existing clients", "established clientele", "long term client",
		"client relationship", "client told me", "my client", "my clients", "favorite client", "retain clients",
		"client retention", "customer retention", "repeat business", "lifetime value", "ltv",
	) || clientPossessiveReference(tokens)
}

func clientPossessiveReference(tokens []string) bool {
	for index, token := range tokens {
		if tokenFamily(token) != "client" {
			continue
		}
		for previous := maxInt(0, index-3); previous < index; previous++ {
			if tokens[previous] == "my" || tokens[previous] == "our" || tokens[previous] == "own" {
				return true
			}
		}
	}
	return false
}

func hasNonClientRecruitmentContext(tokens []string) bool {
	if containsAnyPhrase(tokens, "business partner", "business partnership", "partner", "investment", "invest", "models", "model") {
		return true
	}
	return hasFamily(tokens, "business") && containsAnyPhrase(tokens, "space", "apartment")
}

func hasGenericAcquisitionContext(tokens []string) bool {
	return containsAnyPhrase(tokens, "acquisition", "acquire", "lead generation", "customer growth")
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func assessSmallBusinessAutomation(postTokens []string, reasons []string) RelevanceAssessment {
	businessSignal := hasFamily(postTokens, "business")
	automationSignal := hasAutomationProblemAnchor(postTokens)
	painSignal := hasOperationalPainSignal(postTokens)
	workflowSignal := hasSpecificWorkflowContext(postTokens)
	questionOrNeedSignal := hasQuestionOrNeedSignal(postTokens)

	if businessSignal {
		reasons = append(reasons, "business_family")
	}
	if automationSignal {
		reasons = append(reasons, "automation_or_process_anchor")
	}
	if painSignal {
		reasons = append(reasons, "operational_pain")
	}
	if workflowSignal {
		reasons = append(reasons, "specific_workflow_context")
	}
	if questionOrNeedSignal {
		reasons = append(reasons, "question_or_need")
	}

	// Require the target audience and an operational automation signal. A
	// business-only mention is adjacent; a generic automation post is not
	// evidence about small-business problems.
	if businessSignal && automationSignal && (painSignal || workflowSignal || questionOrNeedSignal) {
		return relevantAssessment(reasons)
	}
	if businessSignal || automationSignal || painSignal || workflowSignal {
		return adjacentAssessment(reasons)
	}
	return irrelevantAssessment([]string{"no_topic_signal"})
}

func relevantAssessment(reasons []string) RelevanceAssessment {
	return RelevanceAssessment{
		Score:   relevanceScoreRelevant,
		Label:   RelevanceRelevant,
		Reasons: uniqueSortedStrings(reasons),
	}
}

func adjacentAssessment(reasons []string) RelevanceAssessment {
	return RelevanceAssessment{
		Score:   relevanceScoreAdjacent,
		Label:   RelevanceAdjacent,
		Reasons: uniqueSortedStrings(reasons),
	}
}

func irrelevantAssessment(reasons []string) RelevanceAssessment {
	return RelevanceAssessment{
		Score:   relevanceScoreIrrelevant,
		Label:   RelevanceIrrelevant,
		Reasons: uniqueSortedStrings(reasons),
	}
}

func uncertainAssessment(reasons []string) RelevanceAssessment {
	return RelevanceAssessment{
		Score:   relevanceScoreUncertain,
		Label:   RelevanceUncertain,
		Reasons: uniqueSortedStrings(reasons),
	}
}

func relevanceTopicTokens(topic string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "for": true, "from": true,
		"in": true, "of": true, "on": true, "the": true, "to": true,
		"with": true, "how": true, "what": true, "why": true, "where": true,
		"find": true, "finding": true, "get": true, "getting": true,
		"make": true, "making": true, "build": true, "building": true,
	}
	seen := map[string]bool{}
	terms := make([]string, 0)
	for _, token := range relevanceTokens(topic) {
		stem := relevanceStem(token)
		if stem == "" || stop[token] || stop[stem] || seen[stem] {
			continue
		}
		seen[stem] = true
		terms = append(terms, stem)
	}
	return terms
}

func relevanceTokens(value string) []string {
	tokens := make([]string, 0)
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, builder.String())
		builder.Reset()
	}
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func relevanceStem(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return ""
	}
	if strings.HasSuffix(token, "ies") && len(token) > 4 {
		return token[:len(token)-3] + "y"
	}
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if strings.HasSuffix(token, suffix) && len(token) > len(suffix)+2 {
			if suffix == "s" && strings.HasSuffix(token, "ss") {
				continue
			}
			stem := token[:len(token)-len(suffix)]
			if suffix == "ing" && len(stem) > 2 && stem[len(stem)-1] == stem[len(stem)-2] {
				stem = stem[:len(stem)-1]
			}
			return stem
		}
	}
	return token
}

func normalizedPhrase(value string) string {
	return strings.Join(relevanceTokens(value), " ")
}

func containsTokenPhrase(haystack, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(haystack) {
		return false
	}
	for start := 0; start <= len(haystack)-len(phrase); start++ {
		match := true
		for offset := range phrase {
			if relevanceStem(haystack[start+offset]) != relevanceStem(phrase[offset]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func queryContextMatches(query string, postTokens, topicTokens []string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	queryTokens := relevanceTokens(query)
	if len(queryTokens) == 0 {
		return false
	}
	for _, queryToken := range queryTokens {
		for _, postToken := range postTokens {
			if relevanceStem(queryToken) == relevanceStem(postToken) {
				for _, topicToken := range topicTokens {
					if relevanceStem(queryToken) == relevanceStem(topicToken) ||
						(tokenFamily(queryToken) != "" && tokenFamily(queryToken) == tokenFamily(topicToken)) {
						return true
					}
				}
			}
		}
	}
	return false
}

func matchTopicTerms(topicTokens, postTokens []string) (terms, matchedPostTokens []string) {
	seenTerms := map[string]bool{}
	seenPostTokens := map[string]bool{}
	for _, topicToken := range topicTokens {
		for _, postToken := range postTokens {
			if relevanceStem(topicToken) == relevanceStem(postToken) ||
				(tokenFamily(topicToken) != "" && tokenFamily(topicToken) == tokenFamily(postToken)) {
				if !seenTerms[topicToken] {
					terms = append(terms, topicToken)
					seenTerms[topicToken] = true
				}
				if !seenPostTokens[postToken] {
					matchedPostTokens = append(matchedPostTokens, postToken)
					seenPostTokens[postToken] = true
				}
				break
			}
		}
	}
	return terms, matchedPostTokens
}

func topicMatchReasons(terms []string) []string {
	reasons := make([]string, 0, len(terms))
	for _, term := range terms {
		reasons = append(reasons, "topic_term:"+term)
	}
	return reasons
}

// missingTopicAnchors keeps a compound topic from collapsing into one broad
// subtopic. For a three-or-more-term topic, the first and last meaningful
// terms act as conservative primary anchors; the full phrase can still win
// without this check.
func missingTopicAnchors(topicTokens, postTokens []string) []string {
	if len(topicTokens) < 3 {
		return nil
	}
	anchors := []string{topicTokens[0], topicTokens[len(topicTokens)-1]}
	missing := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		matched := false
		for _, postToken := range postTokens {
			if relevanceStem(anchor) == relevanceStem(postToken) ||
				(tokenFamily(anchor) != "" && tokenFamily(anchor) == tokenFamily(postToken)) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, anchor)
		}
	}
	return uniqueSortedStrings(missing)
}

func topicAnchorReasons(missing []string) []string {
	reasons := make([]string, 0, len(missing))
	for _, term := range missing {
		reasons = append(reasons, "missing_primary_topic_anchor:"+term)
	}
	return reasons
}

func tokenFamily(token string) string {
	token = relevanceStem(token)
	switch {
	case strings.HasPrefix(token, "client"), strings.HasPrefix(token, "cliet"),
		strings.HasPrefix(token, "customer"),
		strings.HasPrefix(token, "buyer"), strings.HasPrefix(token, "lead"),
		strings.HasPrefix(token, "prospect"):
		return "client"
	case strings.HasPrefix(token, "freelanc"), strings.HasPrefix(token, "contract"),
		strings.HasPrefix(token, "independent"), strings.HasPrefix(token, "solopreneur"):
		return "freelance"
	case strings.HasPrefix(token, "work"):
		return "work"
	case token == "ai" || strings.HasPrefix(token, "artificial") ||
		strings.HasPrefix(token, "gpt") || strings.HasPrefix(token, "agent") ||
		strings.HasPrefix(token, "automat") || strings.HasPrefix(token, "workflow") || token == "n8n":
		return "ai"
	case strings.HasPrefix(token, "content"), strings.HasPrefix(token, "copy"),
		strings.HasPrefix(token, "creator"), strings.HasPrefix(token, "social"),
		strings.HasPrefix(token, "media"), strings.HasPrefix(token, "video"),
		strings.HasPrefix(token, "post"):
		return "content"
	case strings.HasPrefix(token, "creat"), strings.HasPrefix(token, "product"):
		return "creation"
	case strings.HasPrefix(token, "business"), strings.HasPrefix(token, "small"),
		token == "smb" || strings.HasPrefix(token, "entrepreneur"):
		return "business"
	}
	return ""
}

func hasFamily(tokens []string, family string) bool {
	for _, token := range tokens {
		if tokenFamily(token) == family {
			return true
		}
	}
	return false
}

func hasAcquisitionIntent(tokens []string) bool {
	for _, token := range tokens {
		stem := relevanceStem(token)
		for _, prefix := range []string{
			"find", "found", "get", "look", "seek", "need", "want", "acquir", "outreach",
			"contact", "book", "land", "win", "offer", "avail", "connect",
			"give", "provid", "promot", "recommend", "referr", "try",
		} {
			if strings.HasPrefix(stem, prefix) {
				return true
			}
		}
	}
	return false
}

func hasServiceAnchor(tokens []string) bool {
	for _, token := range tokens {
		stem := relevanceStem(token)
		for _, prefix := range []string{
			"design", "web", "site", "app", "social", "media", "smm", "market",
			"content", "video", "edit", "write", "writ", "copy", "brand", "virtual",
			"va", "develop", "architect", "print", "photo", "shopify", "gumroad",
			"canva", "coach", "consult", "product",
		} {
			if strings.HasPrefix(stem, prefix) {
				return true
			}
		}
	}
	return false
}

func hasSpecificContext(tokens []string) bool {
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "am": true, "be": true, "for": true,
		"i": true, "im": true, "me": true, "my": true, "of": true, "on": true,
		"or": true, "the": true, "this": true, "to": true, "we": true, "who": true,
		"how": true, "what": true, "where": true, "you": true, "your": true,
		"anybody": true, "does": true, "has": true, "have": true, "just": true,
		"looking": true, "look": true, "find": true, "finding": true, "get": true,
		"getting": true, "need": true, "want": true, "trying": true, "try": true,
	}
	for _, token := range tokens {
		if stop[token] || tokenFamily(token) == "client" || tokenFamily(token) == "freelance" || hasAcquisitionIntent([]string{token}) {
			continue
		}
		return true
	}
	return false
}

func hasAutomationProblemAnchor(tokens []string) bool {
	for _, token := range tokens {
		stem := relevanceStem(token)
		if tokenFamily(token) == "ai" {
			return true
		}
		for _, prefix := range []string{
			"process", "operat", "admin", "task", "repetit", "manual", "tool", "system",
		} {
			if strings.HasPrefix(stem, prefix) {
				return true
			}
		}
	}
	return false
}

func hasOperationalPainSignal(tokens []string) bool {
	for _, token := range tokens {
		stem := relevanceStem(token)
		for _, prefix := range []string{
			"problem", "pain", "struggl", "hard", "slow", "wast", "bottleneck", "tedious",
			"overwhelm", "busy", "stuck", "expens", "error", "miss", "annoy", "time",
		} {
			if strings.HasPrefix(stem, prefix) {
				return true
			}
		}
	}
	return false
}

func hasSpecificWorkflowContext(tokens []string) bool {
	for _, token := range tokens {
		stem := relevanceStem(token)
		for _, prefix := range []string{
			"lead", "sales", "invoice", "propos", "email", "schedul", "book", "support",
			"onboard", "report", "data", "payroll", "hiring", "market", "content", "social",
			"crm", "calendar", "customer", "client", "order", "inventor", "document",
			"appointment", "financ", "account", "workflow", "operat", "admin", "task",
		} {
			if strings.HasPrefix(stem, prefix) {
				return true
			}
		}
	}
	return false
}

func hasQuestionOrNeedSignal(tokens []string) bool {
	for _, token := range tokens {
		switch relevanceStem(token) {
		case "what", "how", "which", "where", "why", "can", "should", "wish", "need", "want", "try", "look":
			return true
		}
	}
	return false
}

func isClientAcquisitionTopic(topic string) bool {
	tokens := relevanceTokens(topic)
	hasClient := false
	hasFreelance := false
	for _, token := range tokens {
		switch tokenFamily(token) {
		case "client":
			hasClient = true
		case "freelance":
			hasFreelance = true
		}
	}
	return hasClient && hasFreelance
}

func isSmallBusinessAutomationTopic(topic string) bool {
	hasBusiness := false
	hasAutomationOrOperations := false
	hasProblem := false
	for _, token := range relevanceTokens(topic) {
		if tokenFamily(token) == "business" {
			hasBusiness = true
		}
		stem := relevanceStem(token)
		if tokenFamily(token) == "ai" {
			hasAutomationOrOperations = true
		}
		for _, prefix := range []string{
			"process", "operat", "admin", "task", "repetit", "manual", "tool", "system",
		} {
			if strings.HasPrefix(stem, prefix) {
				hasAutomationOrOperations = true
				break
			}
		}
		for _, prefix := range []string{
			"problem", "pain", "struggl", "bottleneck", "hard", "slow", "wast", "tedious",
		} {
			if strings.HasPrefix(stem, prefix) {
				hasProblem = true
				break
			}
		}
	}
	return hasBusiness && (hasAutomationOrOperations || hasProblem)
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
