package research

import "testing"

func verifiedBusinessContext() AuthorBusinessContext {
	return AuthorBusinessContext{
		Username:               "owner",
		OwnerLikelihood:        OwnerLikelihoodHigh,
		OperatorLikelihood:     OwnerLikelihoodHigh,
		BusinessType:           "professional_services",
		ProfileEvidenceReasons: []string{"profile_business_identity"},
		VerifiedOwnerContext:   true,
		Confidence:             "HIGH",
	}
}

func TestSemanticV2RegressionPatterns(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		author          AuthorBusinessContext
		pain            bool
		operational     bool
		solution        bool
		workaround      bool
		nonTarget       bool
		serviceOffer    bool
		businessOpinion bool
		consequence     BusinessConsequence
		workflow        string
	}{
		{
			name:        "explicit lead loss and solution request",
			text:        "У меня свой бизнес: заявки приходят в Telegram, менеджер переносит их вручную, часть теряется. Какую CRM выбрать?",
			author:      verifiedBusinessContext(),
			pain:        true,
			solution:    true,
			workaround:  true,
			consequence: ConsequenceLostLeads,
			workflow:    "Business software or automation selection",
		},
		{
			name:        "tender workaround and error risk",
			text:        "Я дала ChatGPT один тендер и попросила найти места, где новичок может ошибиться. Получился полезный чек-лист.",
			author:      verifiedBusinessContext(),
			pain:        true,
			workaround:  true,
			consequence: ConsequenceErrorRisk,
			workflow:    "Tender preflight review",
		},
		{
			name:        "website conversion friction",
			text:        "Иногда из-за красивого дизайна человеку сложнее оставить заявку или купить. Это реальная проблема конверсии сайта.",
			author:      verifiedBusinessContext(),
			pain:        true,
			consequence: ConsequencePoorConversion,
			workflow:    "Website conversion",
		},
		{
			name:        "vacancy is workload not pain",
			text:        "В салон требуется администратор: отвечать клиентам в Директе, консультировать и записывать на процедуру.",
			operational: true,
			workflow:    "Inbound lead handling",
		},
		{
			name:      "personal medical question",
			text:      "Как завершить грудное вскармливание без боли?",
			nonTarget: true,
		},
		{
			name:      "education error is not business pain",
			text:      "Ученик снова допустил ошибки в тесте по английскому, как объяснить ему тему?",
			nonTarget: true,
		},
		{
			name:         "service offer is supply side",
			text:         "Настрою CRM для бизнеса и автоматизирую заявки. Пишите в личку.",
			author:       verifiedBusinessContext(),
			serviceOffer: true,
		},
		{
			name:      "generic error without context",
			text:      "Ошибка в тесте, кто знает почему?",
			nonTarget: true,
		},
		{
			name:         "course promotion",
			text:         "Набираю учеников на индивидуальные занятия онлайн. Первый пробный урок бесплатно.",
			nonTarget:    true,
			serviceOffer: true,
		},
		{
			name:   "generic AI content",
			text:   "ChatGPT меняет рынок, сохраняйте этот пост и следите за новостями.",
			author: verifiedBusinessContext(),
		},
		{
			name:      "consumer shipping question",
			text:      "Хочу заказать товар весом около 15 кг. Курьеру будет тяжело или нормально?",
			nonTarget: true,
		},
		{
			name:            "generic business opinion",
			text:            "Красивый сайт не всегда продает. Согласны?",
			author:          verifiedBusinessContext(),
			businessOpinion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := classifySemanticPost(tt.text, tt.author)
			if assessment.ExplicitBusinessPain != tt.pain {
				t.Fatalf("explicit pain = %t, want %t; assessment=%+v", assessment.ExplicitBusinessPain, tt.pain, assessment)
			}
			if assessment.OperationalDemandSignal != tt.operational {
				t.Errorf("operational demand = %t, want %t", assessment.OperationalDemandSignal, tt.operational)
			}
			if assessment.ActiveSolutionSeeking != tt.solution {
				t.Errorf("solution seeking = %t, want %t", assessment.ActiveSolutionSeeking, tt.solution)
			}
			if assessment.WorkaroundSignal != tt.workaround {
				t.Errorf("workaround = %t, want %t", assessment.WorkaroundSignal, tt.workaround)
			}
			if assessment.NonTarget != tt.nonTarget {
				t.Errorf("non-target = %t, want %t", assessment.NonTarget, tt.nonTarget)
			}
			if assessment.ServiceOffer != tt.serviceOffer {
				t.Errorf("service offer = %t, want %t", assessment.ServiceOffer, tt.serviceOffer)
			}
			if assessment.BusinessOpinion != tt.businessOpinion {
				t.Errorf("business opinion = %t, want %t", assessment.BusinessOpinion, tt.businessOpinion)
			}
			wantConsequence := tt.consequence
			if wantConsequence == "" {
				wantConsequence = ConsequenceNone
			}
			if assessment.BusinessConsequence != wantConsequence {
				t.Errorf("consequence = %q, want %q", assessment.BusinessConsequence, wantConsequence)
			}
			if assessment.Workflow.WorkflowName != tt.workflow {
				t.Errorf("workflow = %q, want %q", assessment.Workflow.WorkflowName, tt.workflow)
			}
			if tt.operational && assessment.ExplicitBusinessPain {
				t.Error("operational demand must not be promoted to explicit pain")
			}
		})
	}
}

func TestSemanticV2GoldRequiresExplicitPain(t *testing.T) {
	assessment := classifySemanticPost(
		"У меня свой бизнес, как автоматизировать заявки в Telegram? Посоветуйте CRM.",
		verifiedBusinessContext(),
	)
	if assessment.ExplicitBusinessPain {
		t.Fatal("solution request without an explicit failure must not become pain")
	}
	if assessment.SignalTier == SemanticTierGold {
		t.Fatal("Gold signal must require explicit business pain")
	}
}
