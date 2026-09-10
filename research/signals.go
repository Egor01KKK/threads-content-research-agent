package research

import (
	"sort"
	"strings"
)

// SignalType is a deterministic commercial/research signal extracted after a
// post has passed the topic relevance gate. It is intentionally separate from
// both relevance and performance ranking.
type SignalType string

const (
	SignalBuyerHiring         SignalType = "BUYER_HIRING_SIGNAL"
	SignalPaidOpportunity     SignalType = "PAID_OPPORTUNITY"
	SignalClientSeeking       SignalType = "CLIENT_SEEKING"
	SignalAcquisitionPain     SignalType = "ACQUISITION_PAIN"
	SignalAcquisitionQuestion SignalType = "ACQUISITION_QUESTION"
	SignalAcquisitionMethod   SignalType = "ACQUISITION_METHOD"
	SignalServiceOffer        SignalType = "ACQUISITION_SERVICE_OFFER"
	SignalAcquisitionOpinion  SignalType = "ACQUISITION_OPINION"
	SignalOtherRelevant       SignalType = "OTHER_RELEVANT"
)

// signalPriorityOrder is the practical commercial-intent order. Lower
// numbers are higher priority; it never changes the performance ranking.
var signalPriorityOrder = []SignalType{
	SignalBuyerHiring,
	SignalPaidOpportunity,
	SignalAcquisitionPain,
	SignalAcquisitionQuestion,
	SignalClientSeeking,
	SignalAcquisitionMethod,
	SignalServiceOffer,
	SignalAcquisitionOpinion,
	SignalOtherRelevant,
}

// SignalAssessment contains only explainable fields derived from post text.
// RequestedRole, Need, PaidSignal, and CTA are normalized descriptors, while
// the original text remains on the source Post.
type SignalAssessment struct {
	Types                    []SignalType `json:"signal_types,omitempty"`
	Primary                  SignalType   `json:"primary_signal,omitempty"`
	CommercialIntentPriority int          `json:"commercial_intent_priority,omitempty"`
	RequestedRole            string       `json:"requested_role,omitempty"`
	Need                     string       `json:"need,omitempty"`
	PaidSignal               string       `json:"paid_signal,omitempty"`
	CTA                      string       `json:"cta,omitempty"`
	Reasons                  []string     `json:"signal_reasons,omitempty"`
}

// ClassifySignalTypes classifies only the client-acquisition target domain.
// Other target domains can retain the same report shape without inheriting
// client-specific commercial assumptions.
func ClassifySignalTypes(topic, text string) SignalAssessment {
	if !isClientAcquisitionTopic(topic) {
		return SignalAssessment{}
	}
	tokens := relevanceTokens(text)
	if len(tokens) == 0 {
		return SignalAssessment{}
	}

	buyer := hasBuyerHiringSignal(text, tokens)
	paid := buyer && hasPaidOpportunitySignal(text, tokens)
	clientSeeking := hasClientSeekingSignal(text, tokens, buyer)
	pain := hasAcquisitionPainSignal(text, tokens)
	question := hasAcquisitionQuestionSignal(text, tokens)
	// A hiring request mentioning "marketing" or "lead generation" is not
	// itself a tactic. Keep the buyer lane separate unless the post also
	// clearly describes a method in its own right.
	method := !buyer && hasAcquisitionMethodSignal(text, tokens)
	offer := hasAcquisitionServiceOfferSignal(text, tokens)
	opinion := hasAcquisitionOpinionSignal(text, tokens, buyer, clientSeeking, pain, question, method, offer)

	types := make([]SignalType, 0, len(signalPriorityOrder))
	appendIf := func(signal SignalType, enabled bool) {
		if enabled {
			types = append(types, signal)
		}
	}
	appendIf(SignalBuyerHiring, buyer)
	appendIf(SignalPaidOpportunity, paid)
	appendIf(SignalAcquisitionPain, pain)
	appendIf(SignalAcquisitionQuestion, question)
	appendIf(SignalClientSeeking, clientSeeking)
	appendIf(SignalAcquisitionMethod, method)
	appendIf(SignalServiceOffer, offer)
	appendIf(SignalAcquisitionOpinion, opinion)
	if len(types) == 0 && hasClientAcquisitionAnchor(tokens) {
		types = append(types, SignalOtherRelevant)
	}

	assessment := SignalAssessment{
		Types:         types,
		RequestedRole: requestedServiceRole(text, tokens),
		PaidSignal:    paidSignal(text, tokens),
		CTA:           detectedCTA(text, tokens),
	}
	assessment.Primary, assessment.CommercialIntentPriority = primarySignal(types)
	assessment.Need = signalNeed(assessment, text, tokens)
	assessment.Reasons = signalReasons(assessment, buyer, paid, clientSeeking, pain, question, method, offer, opinion)
	return assessment
}

func primarySignal(types []SignalType) (SignalType, int) {
	if len(types) == 0 {
		return "", 0
	}
	priority := map[SignalType]int{}
	for index, signal := range signalPriorityOrder {
		priority[signal] = index + 1
	}
	primary := types[0]
	best := priority[primary]
	for _, signal := range types[1:] {
		if priority[signal] < best {
			primary = signal
			best = priority[signal]
		}
	}
	return primary, best
}

func signalTypesContain(types []SignalType, wanted ...SignalType) bool {
	for _, value := range types {
		for _, target := range wanted {
			if value == target {
				return true
			}
		}
	}
	return false
}

func hasClientAcquisitionAnchor(tokens []string) bool {
	return hasFamily(tokens, "client") || hasFamily(tokens, "freelance") ||
		containsAnyPhrase(tokens,
			"client acquisition", "customer acquisition", "lead generation", "client outreach",
		)
}

func hasActiveRequest(tokens []string) bool {
	return containsAnyPhrase(tokens,
		"looking for", "need", "needs", "hiring", "hire", "seeking", "want", "wanted",
		"anyone available", "who can", "open for", "available for",
	)
}

type serviceRole struct {
	phrase string
	label  string
}

var serviceRoles = []serviceRole{
	{phrase: "freelance professional makeup", label: "freelance makeup artist"},
	{phrase: "social media manager", label: "social media manager"},
	{phrase: "marketing person", label: "marketing / lead-generation specialist"},
	{phrase: "video editor", label: "video editor"},
	{phrase: "lead generator", label: "lead-generation specialist"},
	{phrase: "virtual assistant", label: "virtual assistant"},
	{phrase: "service provider", label: "service provider"},
	{phrase: "hairstylist", label: "hairstylist"},
	{phrase: "hair stylist", label: "hairstylist"},
	{phrase: "braider", label: "braider"},
	{phrase: "nail technician", label: "nail technician"},
	{phrase: "nail tech", label: "nail technician"},
	{phrase: "makeup artist", label: "makeup artist"},
	{phrase: "marketer", label: "marketer"},
	{phrase: "designer", label: "designer"},
	{phrase: "developer", label: "developer"},
	{phrase: "copywriter", label: "copywriter"},
	{phrase: "photographer", label: "photographer"},
	{phrase: "consultant", label: "consultant"},
	{phrase: "coach", label: "coach"},
	{phrase: "contractor", label: "contractor"},
	{phrase: "freelancer", label: "freelancer"},
	{phrase: "freelance", label: "freelancer / contractor"},
	{phrase: "agency", label: "agency"},
	{phrase: "specialist", label: "specialist"},
	{phrase: "writer", label: "writer"},
	{phrase: "editor", label: "editor"},
	{phrase: "hair", label: "hair professional"},
	{phrase: "nails", label: "nail technician"},
}

func requestedServiceRole(text string, tokens []string) string {
	_ = text
	for _, role := range serviceRoles {
		if containsAnyPhrase(tokens, role.phrase) {
			return role.label
		}
	}
	return ""
}

func hasBuyerHiringSignal(text string, tokens []string) bool {
	if !hasActiveRequest(tokens) {
		return false
	}
	// These are recruitment/partnership requests, not a buyer hiring a
	// freelancer or service provider for client work.
	if containsAnyPhrase(tokens, "business partner", "partner", "investment", "invest", "models", "model") {
		return false
	}
	// "I need coaches who are looking for clients" is a seller seeking coaches
	// as potential customers, not a buyer hiring a coach.
	if containsAnyPhrase(tokens, "who are looking for clients", "who is looking for clients", "clients who") {
		return false
	}
	if requestedServiceRole(text, tokens) == "" {
		return false
	}
	return true
}

func hasPaidOpportunitySignal(text string, tokens []string) bool {
	if !hasBuyerHiringSignal(text, tokens) {
		return false
	}
	return paidSignal(text, tokens) != ""
}

func paidSignal(text string, tokens []string) string {
	if strings.Contains(text, "$") || strings.Contains(text, "€") || strings.Contains(text, "£") || strings.Contains(text, "₹") {
		return "currency_amount"
	}
	if containsAnyPhrase(tokens, "paid work", "paid opportunity", "compensation", "budget", "fee", "rate", "commission", "salary") {
		return "explicit_compensation_term"
	}
	return ""
}

func hasClientSeekingSignal(text string, tokens []string, buyer bool) bool {
	if buyer {
		return false
	}
	if hasClientSeekingTargetAudience(tokens) {
		return false
	}
	if containsAnyPhrase(tokens,
		"looking for clients", "looking for a client", "looking for new client", "looking for new clients",
		"need clients", "need a client", "open for clients", "taking clients",
	) {
		return hasFirstPerson(tokens) || requestedServiceRole(text, tokens) != "" || hasServiceAnchor(tokens) ||
			containsAnyPhrase(tokens,
				"book me", "book now", "available", "dm if interested", "lmk", "accepting bookings",
				"if you are in search of", "open for booking",
			)
	}
	if hasFirstPerson(tokens) && containsAnyPhrase(tokens,
		"trying to find clients", "trying to find a client", "trying to get clients",
		"trying to find customers",
	) {
		return true
	}
	if !hasFamily(tokens, "client") || requestedServiceRole(text, tokens) == "" {
		return false
	}
	return containsAnyPhrase(tokens,
		"book me", "book now", "available", "dm if interested", "lmk", "accepting bookings",
		"if you are in search of", "open for booking",
	)
}

func hasAcquisitionPainSignal(text string, tokens []string) bool {
	if !hasClientAcquisitionAnchor(tokens) {
		return false
	}
	return containsAnyPhrase(tokens,
		"struggling", "struggle", "hard", "difficult", "can't", "cannot", "no luck",
		"nothing happened", "not working", "not attracting", "nobody", "stuck", "overwhelmed",
		"doing everything",
	) || strings.Contains(strings.ToLower(text), "nothing happened") ||
		strings.Contains(strings.ToLower(text), "isn't producing") ||
		strings.Contains(strings.ToLower(text), "is not producing")
}

func hasAcquisitionQuestionSignal(text string, tokens []string) bool {
	if !hasClientAcquisitionAnchor(tokens) {
		return false
	}
	if strings.ContainsAny(text, "?？") {
		return true
	}
	return containsAnyPhrase(tokens,
		"what advice", "any advice", "how do", "how to", "where to", "which channel",
		"does anybody know", "what is the best way", "how are you getting", "advice",
	)
}

func hasAcquisitionMethodSignal(text string, tokens []string) bool {
	if !hasClientAcquisitionAnchor(tokens) {
		return false
	}
	if containsAnyPhrase(tokens,
		"facebook", "fb", "rfp", "cold email", "email outreach", "dm outreach", "direct message",
		"networking", "posting", "content", "promotions", "promotion",
		"marketing", "launching", "outreach", "commenting", "free channels",
		"playbook", "script", "30 day plan", "system", "funnel", "process", "breakdown",
	) {
		return true
	}
	return containsAnyPhrase(tokens,
		"through referrals", "via referrals", "from referrals", "by referral",
		"ask for referrals", "referral program",
	)
}

func hasAcquisitionServiceOfferSignal(text string, tokens []string) bool {
	if !hasClientAcquisitionAnchor(tokens) {
		return false
	}
	return containsAnyPhrase(tokens,
		"client acquisition system", "client acquisition funnel", "acquisition funnel",
		"lead generation service", "build your client acquisition", "show you how the client acquisition",
		"teach you the process", "exact playbook", "help you get clients",
	) || (containsAnyPhrase(tokens, "build", "show", "teach", "help") &&
		containsAnyPhrase(tokens, "client acquisition", "get clients", "lead generation") &&
		containsAnyPhrase(tokens, "dm", "connect", "comment", "send", "apply", "book", "interested"))
}

func hasAcquisitionOpinionSignal(text string, tokens []string, buyer, clientSeeking, pain, question, method, offer bool) bool {
	if buyer || clientSeeking || pain || question || offer {
		return false
	}
	if !hasClientAcquisitionAnchor(tokens) {
		return false
	}
	opinion := containsAnyPhrase(tokens,
		"best client acquisition strategy", "finding clients is easy", "easy finding clients",
		"goal of customer acquisition", "customer acquisition should", "client acquisition strategy",
	)
	if method && !opinion {
		return false
	}
	_ = text
	return opinion
}

func containsAnyPhrase(tokens []string, phrases ...string) bool {
	for _, phrase := range phrases {
		if containsTokenPhrase(tokens, relevanceTokens(phrase)) {
			return true
		}
	}
	return false
}

func hasFirstPerson(tokens []string) bool {
	for _, token := range tokens {
		switch token {
		case "i", "im", "i'm", "ive", "i’ve", "me", "my", "mine", "we", "were", "we’re", "our", "ours", "us":
			return true
		}
	}
	return false
}

func signalNeed(assessment SignalAssessment, text string, tokens []string) string {
	if assessment.RequestedRole != "" && signalTypesContain(assessment.Types, SignalBuyerHiring, SignalPaidOpportunity) {
		return "hire or book a " + assessment.RequestedRole
	}
	if signalTypesContain(assessment.Types, SignalClientSeeking) {
		if assessment.RequestedRole != "" {
			return "new clients for " + assessment.RequestedRole
		}
		return "new clients or customers"
	}
	if signalTypesContain(assessment.Types, SignalAcquisitionPain) {
		return "a workable way to acquire clients"
	}
	if signalTypesContain(assessment.Types, SignalServiceOffer) {
		return "client-acquisition help, a system, or a funnel"
	}
	if signalTypesContain(assessment.Types, SignalAcquisitionQuestion) {
		return "advice on where/how to find clients"
	}
	if signalTypesContain(assessment.Types, SignalAcquisitionMethod) {
		return "a repeatable client-acquisition tactic"
	}
	if signalTypesContain(assessment.Types, SignalAcquisitionOpinion) {
		return "an opinion about client acquisition"
	}
	_ = text
	_ = tokens
	return "relevant client-acquisition context"
}

func detectedCTA(text string, tokens []string) string {
	switch {
	case containsAnyPhrase(tokens, "comment"):
		return "comment"
	case containsAnyPhrase(tokens, "dm asap"):
		return "DM asap"
	case containsAnyPhrase(tokens, "dm", "direct message"):
		return "DM"
	case containsAnyPhrase(tokens, "book me", "book now"):
		return "book"
	case containsAnyPhrase(tokens, "apply", "applying"):
		return "apply"
	case containsAnyPhrase(tokens, "connect"):
		return "connect"
	case containsAnyPhrase(tokens, "lmk"):
		return "let me know"
	case containsAnyPhrase(tokens, "save this", "use it", "study it"):
		return "save/use"
	case strings.Contains(strings.ToLower(text), "where") || strings.Contains(strings.ToLower(text), "how"):
		return "ask the community"
	default:
		return ""
	}
}

func signalReasons(assessment SignalAssessment, buyer, paid, clientSeeking, pain, question, method, offer, opinion bool) []string {
	reasons := make([]string, 0, 9)
	if buyer {
		reasons = append(reasons, "active_service_request")
	}
	if paid {
		reasons = append(reasons, "explicit_paid_marker")
	}
	if clientSeeking {
		reasons = append(reasons, "provider_seeking_clients")
	}
	if pain {
		reasons = append(reasons, "acquisition_pain_language")
	}
	if question {
		reasons = append(reasons, "acquisition_question_language")
	}
	if method {
		reasons = append(reasons, "concrete_acquisition_method")
	}
	if offer {
		reasons = append(reasons, "acquisition_service_offer_language")
	}
	if opinion {
		reasons = append(reasons, "acquisition_opinion_language")
	}
	if assessment.RequestedRole != "" {
		reasons = append(reasons, "requested_role:"+assessment.RequestedRole)
	}
	sort.Strings(reasons)
	return reasons
}

func annotateRankedSignals(topic string, posts []RankedPost) []RankedPost {
	out := make([]RankedPost, len(posts))
	copy(out, posts)
	for index := range out {
		assessment := ClassifySignalTypes(topic, out[index].Post.Text)
		out[index].SignalTypes = append([]SignalType(nil), assessment.Types...)
		out[index].PrimarySignal = assessment.Primary
		out[index].CommercialIntentPriority = assessment.CommercialIntentPriority
		out[index].RequestedRole = assessment.RequestedRole
		out[index].Need = assessment.Need
		out[index].PaidSignal = assessment.PaidSignal
		out[index].CTA = assessment.CTA
		out[index].SignalReasons = append([]string(nil), assessment.Reasons...)
	}
	return out
}

func buildSignalSections(posts []RankedPost) (SignalCounts, []RankedPost, []RankedPost) {
	counts := SignalCounts{RelevantPosts: len(posts)}
	lead := make([]RankedPost, 0)
	content := make([]RankedPost, 0)
	for _, post := range posts {
		if signalTypesContain(post.SignalTypes, SignalBuyerHiring, SignalPaidOpportunity) {
			lead = append(lead, post)
		}
		if len(post.SignalTypes) > 0 && !signalTypesContain(post.SignalTypes, SignalBuyerHiring, SignalPaidOpportunity) {
			content = append(content, post)
		}
		for _, signal := range post.SignalTypes {
			switch signal {
			case SignalBuyerHiring:
				counts.BuyerHiringSignals++
			case SignalPaidOpportunity:
				counts.PaidOpportunities++
			case SignalClientSeeking:
				counts.ClientSeeking++
			case SignalAcquisitionPain:
				counts.AcquisitionPain++
			case SignalAcquisitionQuestion:
				counts.AcquisitionQuestions++
			case SignalAcquisitionMethod:
				counts.AcquisitionMethods++
			case SignalServiceOffer:
				counts.AcquisitionServiceOffers++
			case SignalAcquisitionOpinion:
				counts.AcquisitionOpinions++
			case SignalOtherRelevant:
				counts.OtherRelevant++
			}
		}
	}
	counts.PotentialBuyerSignals = len(lead)
	return counts, lead, content
}
