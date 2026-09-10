package research

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type relevanceFixturePost struct {
	ID            string `json:"id"`
	Author        string `json:"author"`
	URL           string `json:"url"`
	Text          string `json:"text"`
	ExpectedLabel string `json:"expected_label"`
}

func TestFindingClientsSyntheticFixtureRelevance(t *testing.T) {
	data, err := os.ReadFile("testdata/finding_clients_relevance.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture []relevanceFixturePost
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}

	counts := map[RelevanceLabel]int{}
	trueRelevant := 0
	truePositive := 0
	falsePositive := 0
	falseNegative := 0
	for _, item := range fixture {
		assessment := AssessRelevanceForQuery("finding clients for freelancers", "clients", item.Text)
		counts[assessment.Label]++
		if item.ExpectedLabel == string(RelevanceRelevant) {
			trueRelevant++
		}
		if assessment.Label == RelevanceRelevant && item.ExpectedLabel == string(RelevanceRelevant) {
			truePositive++
		}
		if assessment.Label == RelevanceRelevant && item.ExpectedLabel != string(RelevanceRelevant) {
			falsePositive++
		}
		if assessment.Label != RelevanceRelevant && item.ExpectedLabel == string(RelevanceRelevant) {
			falseNegative++
		}
		if assessment.Label != RelevanceLabel(item.ExpectedLabel) {
			t.Errorf("%s @%s: got %s (%0.2f, %v), want %s", item.ID, item.Author, assessment.Label, assessment.Score, assessment.Reasons, item.ExpectedLabel)
		}
		if item.URL == "" || item.Text == "" {
			t.Errorf("fixture item %s lost source text or URL", item.ID)
		}
	}

	precision := ratio(truePositive, truePositive+falsePositive)
	recall := ratio(truePositive, truePositive+falseNegative)
	t.Logf("synthetic fixture: labels=%v precision=%.2f recall=%.2f false_positives=%d false_negatives=%d", counts, precision, recall, falsePositive, falseNegative)
	if counts[RelevanceRelevant] != 8 || counts[RelevanceAdjacent] != 2 || counts[RelevanceIrrelevant] != 10 {
		t.Fatalf("fixture label distribution = %v, want relevant=8 adjacent=2 irrelevant=10", counts)
	}
	if precision != 1 || recall != 1 || falsePositive != 0 || falseNegative != 0 {
		t.Fatalf("fixture metrics precision=%.2f recall=%.2f fp=%d fn=%d", precision, recall, falsePositive, falseNegative)
	}
}

func TestRelevanceDoesNotTrustBroadQueryAlone(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		label RelevanceLabel
	}{
		{name: "broad freelancer hit", text: "Fancy Francine.", label: RelevanceIrrelevant},
		{name: "client mention only", text: "My favorite client loved my work", label: RelevanceIrrelevant},
		{name: "specific acquisition", text: "Need clients for web design", label: RelevanceRelevant},
		{name: "missing text", text: "😎", label: RelevanceUncertain},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := AssessRelevanceForQuery("finding clients for freelancers", "freelancers", test.text)
			if got.Label != test.label {
				t.Fatalf("label = %s (%0.2f, %v), want %s", got.Label, got.Score, got.Reasons, test.label)
			}
			if test.label != RelevanceIrrelevant && !strings.Contains(strings.Join(got.Reasons, ","), "family") && test.label != RelevanceUncertain {
				t.Errorf("assessment has no topic-family explanation: %v", got.Reasons)
			}
		})
	}
}

func TestClientAcquisitionRelevanceUsesAcquisitionContext(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		label      RelevanceLabel
		reasonPart string
	}{
		{
			name:       "restaurant customer service complaint",
			text:       "The restaurant customer service was terrible and I asked for a refund.",
			label:      RelevanceIrrelevant,
			reasonPart: "customer_service_context",
		},
		{
			name:       "existing client relationship",
			text:       "My existing client relationship is wonderful and very stable.",
			label:      RelevanceIrrelevant,
			reasonPart: "existing_client_or_retention_context",
		},
		{
			name:       "established clientele without acquisition",
			text:       "I have an established clientele who love my work.",
			label:      RelevanceIrrelevant,
			reasonPart: "existing_client_or_retention_context",
		},
		{
			name:       "freelancer looking for clients",
			text:       "As a freelancer, I am looking for new clients.",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "buyer looking for freelance editor",
			text:       "Looking for a freelance video editor for a paid project.",
			label:      RelevanceRelevant,
			reasonPart: "buyer_service_request",
		},
		{
			name:       "business hiring lead generation help",
			text:       "Our business is looking for a marketer who can generate leads for us.",
			label:      RelevanceRelevant,
			reasonPart: "buyer_service_request",
		},
		{
			name:       "freelancer acquisition question",
			text:       "I am a VA starting out. Where do I get clients?",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "marketing is not producing clients",
			text:       "I tried marketing, but it isn't producing clients.",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "concrete acquisition tactic",
			text:       "I find leads through cold email outreach and referrals.",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "acquisition opinion",
			text:       "The best client acquisition strategy is to be great at what you do.",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "retention only",
			text:       "Client retention is everything; my existing clients keep coming back.",
			label:      RelevanceIrrelevant,
			reasonPart: "existing_client_or_retention_context",
		},
		{
			name:       "retention dominant acquisition commentary",
			text:       "Client acquisition is radically different than client retention. MRR is the result of customers continuing to buy.",
			label:      RelevanceAdjacent,
			reasonPart: "retention_dominant_acquisition_context",
		},
		{
			name:       "providers sought as target audience",
			text:       "I need coaches who are looking for clients currently.",
			label:      RelevanceAdjacent,
			reasonPart: "client_seeking_target_audience",
		},
		{
			name:       "common client typo with acquisition question",
			text:       "How do you find cliets when just starting out?",
			label:      RelevanceRelevant,
			reasonPart: "client_acquisition_context",
		},
		{
			name:       "bare client request remains reviewable",
			text:       "Looking for clients!!",
			label:      RelevanceAdjacent,
			reasonPart: "client_reference_without_acquisition",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := AssessRelevance("finding clients for freelancers", test.text)
			if got.Label != test.label {
				t.Fatalf("label = %s (%0.2f, %v), want %s", got.Label, got.Score, got.Reasons, test.label)
			}
			if test.reasonPart != "" && !strings.Contains(strings.Join(got.Reasons, ","), test.reasonPart) {
				t.Fatalf("reasons = %v, want a reason containing %q", got.Reasons, test.reasonPart)
			}
		})
	}
}

func TestRelevanceTargetDomainTopicsNeedOperationalContext(t *testing.T) {
	if got := AssessRelevance("problems small businesses want to automate", "Small businesses spend hours on repetitive admin tasks and want to automate the work"); got.Label != RelevanceRelevant {
		t.Fatalf("specific SMB automation problem = %s (%v), want relevant", got.Label, got.Reasons)
	}
	if got := AssessRelevance("problems small businesses want to automate", "Small business owners are hiring"); got.Label != RelevanceAdjacent {
		t.Fatalf("business-only anchor = %s (%v), want adjacent", got.Label, got.Reasons)
	}
	if got := AssessRelevance("problems small businesses want to automate", "A recipe for dinner"); got.Label != RelevanceIrrelevant {
		t.Fatalf("no target anchor = %s (%v), want irrelevant", got.Label, got.Reasons)
	}
	if got := AssessRelevance("AI automation for small businesses", "AI automation can help businesses reduce repetitive work"); got.Label != RelevanceRelevant {
		t.Fatalf("business plural anchor = %s (%v), want relevant", got.Label, got.Reasons)
	}
	if got := AssessRelevance("finding clients for freelancers", "How do I find customers as a freelancer?"); got.Label != RelevanceRelevant {
		t.Fatalf("customer/freelancer families = %s (%v), want relevant", got.Label, got.Reasons)
	}
}

func TestRussianSmallBusinessPainRelevanceRequiresUsefulBusinessEvidence(t *testing.T) {
	topic := "проблемы малого бизнеса которые можно решить автоматизацией и IT"
	cases := []struct {
		name  string
		text  string
		label RelevanceLabel
	}{
		{
			name:  "owner with manual lead loss and solution search",
			text:  "У меня свой магазин. Веду заявки в Excel, они теряются, кто посоветует CRM?",
			label: RelevanceRelevant,
		},
		{
			name:  "owner business discussion without pain",
			text:  "У меня свой бизнес, расскажите чем занимаетесь.",
			label: RelevanceAdjacent,
		},
		{
			name:  "personal pain without business context",
			text:  "Мне тяжело всё успевать на работе, но решения пока не ищу.",
			label: RelevanceAdjacent,
		},
		{
			name:  "unrelated Russian text",
			text:  "Сегодня прекрасная погода, гуляем в парке.",
			label: RelevanceIrrelevant,
		},
		{
			name:  "English result is not Russian target evidence",
			text:  "Small business owners lose leads in spreadsheets and need a CRM.",
			label: RelevanceUncertain,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := AssessRelevance(topic, test.text)
			if got.Label != test.label {
				t.Fatalf("label = %s (%0.2f, %v), want %s", got.Label, got.Score, got.Reasons, test.label)
			}
		})
	}
}

func TestAssessBusinessPainKeepsDistinctRussianSignals(t *testing.T) {
	topic := "проблемы малого бизнеса которые можно решить автоматизацией и IT"
	assessment := AssessBusinessPain(topic, "У нас в агентстве все задачи в чатах, сотрудники забывают передавать заявки, поэтому теряем клиентов. Как связать CRM и Telegram?")
	if assessment.OwnerLikelihood != OwnerLikelihoodHigh {
		t.Fatalf("owner likelihood = %s, want HIGH", assessment.OwnerLikelihood)
	}
	for _, want := range []string{"TEAM_OPERATIONS", "LEAD_GENERATION", "CUSTOMER_COMMUNICATION", "CRM_CLIENT_MANAGEMENT", "INTEGRATIONS"} {
		found := false
		for _, got := range assessment.PainTypes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("pain types = %v, missing %s", assessment.PainTypes, want)
		}
	}
	if assessment.WorkaroundPresent != Yes || assessment.SolutionSeeking != Yes || assessment.ITActionability != ITActionabilityHigh {
		t.Errorf("business pain assessment = %+v", assessment)
	}
	if len(assessment.BusinessConsequences) == 0 || !assessment.GoldPainSignal {
		t.Errorf("assessment should be a gold signal: %+v", assessment)
	}
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
