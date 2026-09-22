package fsm

import "fmt"

var transitions = map[Status]map[Event]Status{
	StatusRegistered: {
		EventNormalize: StatusNormalized,
	},
	StatusNormalized: {
		EventConfirmDeviation: StatusDeviationConfirmed,
	},
	StatusDeviationConfirmed: {
		EventDiagnose: StatusDiagnosed,
	},
	StatusDiagnosed: {
		EventAssessRisk: StatusRiskAssessed,
	},
	StatusRiskAssessed: {
		EventGenerateAlternatives: StatusAlternativesGenerated,
	},
	StatusAlternativesGenerated: {
		EventSelectAction: StatusActionSelected,
	},
	StatusActionSelected: {
		EventSendToExpert: StatusExpertValidation,
	},
	StatusExpertValidation: {
		EventApproveByExpert: StatusExpertApproved,
		EventRejectByExpert:  StatusExpertRejected,
		EventReturnByExpert:  StatusExpertReturned,
	},
	StatusExpertApproved: {
		EventExecute: StatusExecuted,
	},
	StatusExecuted: {
		EventArchive: StatusArchived,
	},
	StatusExpertReturned: {
		EventDiagnose: StatusDiagnosed,
	},
	StatusExpertRejected: {
		EventArchive: StatusArchived,
	},
}

func CanTransition(from Status, event Event) (Status, bool) {
	events, ok := transitions[from]
	if !ok {
		return from, false
	}

	to, ok := events[event]
	if !ok {
		return from, false
	}

	return to, true
}

func MustTransition(from Status, event Event) (Status, error) {
	to, ok := CanTransition(from, event)
	if !ok {
		return from, fmt.Errorf("transition from %q by event %q is not allowed", from, event)
	}

	return to, nil
}
