package fsm

type Event string

const (
	EventNormalize            Event = "normalize"
	EventConfirmDeviation     Event = "confirm_deviation"
	EventDiagnose             Event = "diagnose"
	EventAssessRisk           Event = "assess_risk"
	EventGenerateAlternatives Event = "generate_alternatives"
	EventSelectAction         Event = "select_action"
	EventSendToExpert         Event = "send_to_expert"
	EventApproveByExpert      Event = "approve_by_expert"
	EventRejectByExpert       Event = "reject_by_expert"
	EventReturnByExpert       Event = "return_by_expert"
	EventExecute              Event = "execute"
	EventArchive              Event = "archive"
)
