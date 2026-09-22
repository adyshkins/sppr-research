package fsm

import "testing"

func TestCanTransitionAllowed(t *testing.T) {
	tests := []struct {
		name  string
		from  Status
		event Event
		want  Status
	}{
		{"registered to normalized", StatusRegistered, EventNormalize, StatusNormalized},
		{"normalized to deviation confirmed", StatusNormalized, EventConfirmDeviation, StatusDeviationConfirmed},
		{"deviation confirmed to diagnosed", StatusDeviationConfirmed, EventDiagnose, StatusDiagnosed},
		{"diagnosed to risk assessed", StatusDiagnosed, EventAssessRisk, StatusRiskAssessed},
		{"risk assessed to alternatives generated", StatusRiskAssessed, EventGenerateAlternatives, StatusAlternativesGenerated},
		{"alternatives generated to action selected", StatusAlternativesGenerated, EventSelectAction, StatusActionSelected},
		{"action selected to expert validation", StatusActionSelected, EventSendToExpert, StatusExpertValidation},
		{"expert validation to approved", StatusExpertValidation, EventApproveByExpert, StatusExpertApproved},
		{"expert validation to rejected", StatusExpertValidation, EventRejectByExpert, StatusExpertRejected},
		{"expert validation to returned", StatusExpertValidation, EventReturnByExpert, StatusExpertReturned},
		{"expert approved to executed", StatusExpertApproved, EventExecute, StatusExecuted},
		{"executed to archived", StatusExecuted, EventArchive, StatusArchived},
		{"expert returned to diagnosed", StatusExpertReturned, EventDiagnose, StatusDiagnosed},
		{"expert rejected to archived", StatusExpertRejected, EventArchive, StatusArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CanTransition(tt.from, tt.event)
			if !ok {
				t.Fatalf("expected transition to be allowed")
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestCanTransitionForbidden(t *testing.T) {
	got, ok := CanTransition(StatusRegistered, EventExecute)
	if ok {
		t.Fatalf("expected transition to be forbidden")
	}
	if got != StatusRegistered {
		t.Fatalf("expected original status %q, got %q", StatusRegistered, got)
	}
}

func TestMustTransitionForbidden(t *testing.T) {
	_, err := MustTransition(StatusArchived, EventNormalize)
	if err == nil {
		t.Fatalf("expected error")
	}
}
