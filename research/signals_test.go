package research

import "testing"

func TestClientAcquisitionSignalTypesAndPriority(t *testing.T) {
	cases := []struct {
		name          string
		text          string
		want          []SignalType
		primary       SignalType
		priority      int
		requestedRole string
	}{
		{
			name:          "buyer paid opportunity",
			text:          "Looking for a freelance video editor for paid work. DM if available.",
			want:          []SignalType{SignalBuyerHiring, SignalPaidOpportunity},
			primary:       SignalBuyerHiring,
			priority:      1,
			requestedRole: "video editor",
		},
		{
			name:     "provider pain and question",
			text:     "I am struggling to get clients. How do you find them?",
			want:     []SignalType{SignalAcquisitionPain, SignalAcquisitionQuestion},
			primary:  SignalAcquisitionPain,
			priority: 3,
		},
		{
			name:     "concrete method",
			text:     "Consultants, where are you finding clients? I use Facebook RFPs.",
			want:     []SignalType{SignalAcquisitionQuestion, SignalAcquisitionMethod},
			primary:  SignalAcquisitionQuestion,
			priority: 4,
		},
		{
			name:     "service offer",
			text:     "I build client acquisition funnels for agencies. DM to connect.",
			want:     []SignalType{SignalAcquisitionMethod, SignalServiceOffer},
			primary:  SignalAcquisitionMethod,
			priority: 6,
		},
		{
			name:     "acquisition opinion",
			text:     "The best client acquisition strategy is to be great at what you do.",
			want:     []SignalType{SignalAcquisitionOpinion},
			primary:  SignalAcquisitionOpinion,
			priority: 8,
		},
		{
			name:     "retention commentary is not a method",
			text:     "The goal of customer acquisition should be LTV, not just today's transaction. Retain customers and earn repeat business.",
			want:     []SignalType{SignalAcquisitionOpinion},
			primary:  SignalAcquisitionOpinion,
			priority: 8,
		},
		{
			name:          "provider advertises availability",
			text:          "It is so hard to find clientele. If you are in search of a braider, book me now!",
			want:          []SignalType{SignalClientSeeking, SignalAcquisitionPain},
			primary:       SignalAcquisitionPain,
			priority:      3,
			requestedRole: "braider",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifySignalTypes("finding clients for freelancers", test.text)
			for _, wanted := range test.want {
				if !signalTypesContain(got.Types, wanted) {
					t.Errorf("signal types = %v, want %s", got.Types, wanted)
				}
			}
			if got.Primary != test.primary || got.CommercialIntentPriority != test.priority {
				t.Errorf("primary/priority = %s/%d, want %s/%d (types=%v)", got.Primary, got.CommercialIntentPriority, test.primary, test.priority, got.Types)
			}
			if test.requestedRole != "" && got.RequestedRole != test.requestedRole {
				t.Errorf("requested role = %q, want %q", got.RequestedRole, test.requestedRole)
			}
		})
	}
}

func TestSignalTypesAreNotProducedForUnrelatedDomains(t *testing.T) {
	got := ClassifySignalTypes("problems small businesses want to automate", "Looking for a marketer to get clients")
	if len(got.Types) != 0 || got.CommercialIntentPriority != 0 {
		t.Fatalf("signals for unrelated topic = %+v, want none", got)
	}
}

func TestTargetAudienceSeekingIsNotProviderClientSeeking(t *testing.T) {
	got := ClassifySignalTypes("finding clients for freelancers", "I need coaches who are looking for clients currently.")
	if signalTypesContain(got.Types, SignalClientSeeking) {
		t.Fatalf("target-audience post was classified as provider client seeking: %v", got.Types)
	}
}
