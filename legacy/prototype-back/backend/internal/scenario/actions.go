package scenario

import "sppr-prototype/backend/internal/cvar"

func GenerateActions(input GenerationInput) ([]cvar.Action, error) {
	metadata := func() map[string]any {
		return map[string]any{
			"product":    input.Product,
			"cause_code": input.CauseCode,
			"event_id":   input.EventID,
		}
	}

	return []cvar.Action{
		{
			Code:             "keep_plan",
			Description:      "Сохранить текущий план и продолжить мониторинг",
			ActionType:       "no_change",
			StabilityPenalty: 0,
			Metadata:         metadata(),
		},
		{
			Code:             "increase_capacity",
			Description:      "Увеличить доступную мощность за счёт перераспределения смен или ресурсов",
			ActionType:       "capacity",
			StabilityPenalty: 8,
			Metadata:         metadata(),
		},
		{
			Code:             "expedite_supply",
			Description:      "Ускорить снабжение или логистику для снижения риска невыполнения спроса",
			ActionType:       "supply",
			StabilityPenalty: 12,
			Metadata:         metadata(),
		},
		{
			Code:             "rebalance_plan",
			Description:      "Перераспределить производственный план и изменить приоритеты заказов",
			ActionType:       "planning",
			StabilityPenalty: 10,
			Metadata:         metadata(),
		},
	}, nil
}
