# Screenshots Plan Для Главы 4

## 1. Реестр управленческих случаев

- Что видно: таблица или список кейсов, статусы, даты, выбранные действия.
- Зачем нужен: показывает вход в систему и накопление управленческих случаев.
- Подтверждает: frontend-экран реестра, `GET /api/cases`.

## 2. Форма создания события отклонения

- Что видно: поля event_id, product, plan_demand, fact_demand, capacity_load, forecast_error, campaign_flag, alpha.
- Зачем нужен: демонстрирует регистрацию исходного отклонения.
- Подтверждает: frontend-форма создания, `POST /api/workflow/process`.

## 3. Карточка кейса после обработки workflow

- Что видно: case_id, статус `action_selected`, top cause, selected action, risk values.
- Зачем нужен: показывает результат автоматической обработки интеллектуальным контуром.
- Подтверждает: карточка кейса, `GET /api/cases/:id`.

## 4. Блок диагностики причин

- Что видно: гипотезы `demand_spike`, `capacity_overload`, `forecast_error`, `campaign_effect`, confidence и details.
- Зачем нужен: иллюстрирует этап причинной диагностики отклонения.
- Подтверждает: блок diagnostics, `GET /api/cases/:id/diagnostics`.

## 5. Блок сценариев

- Что видно: baseline, demand_growth, stress_capacity, recovery; вероятности и loss_vector.
- Зачем нужен: показывает сценарную основу для риск-ориентированной оценки.
- Подтверждает: блок scenarios, `GET /api/cases/:id/scenarios`.

## 6. Блок корректирующих действий и выбранного действия

- Что видно: список действий, ExpectedLoss, VaR, CVaR, признак selected.
- Зачем нужен: демонстрирует CVaR-ранжирование и выбор корректирующего воздействия.
- Подтверждает: блок actions, `GET /api/cases/:id/actions`.

## 7. Экран экспертной валидации

- Что видно: HITL-кнопки submit-validation, approved, rejected, returned, execute, archive.
- Зачем нужен: показывает участие эксперта в контуре принятия решения.
- Подтверждает: frontend HITL controls, endpoints `/submit-validation`, `/expert-decision`, `/execute`, `/archive`.

## 8. Протокол decision_trace / П

- Что видно: последовательность steps: case_created, event_registered, status_changed, diagnostic_created, cvar_calculated, action_generated, action_selected, expert_decision_recorded.
- Зачем нужен: подтверждает трассируемость принятия решения.
- Подтверждает: trace-блок, `GET /api/cases/:id/trace`.

## 9. Финальный статус archived

- Что видно: статус кейса `archived` после execute/archive или rejected/archive.
- Зачем нужен: показывает завершение жизненного цикла управленческого случая.
- Подтверждает: карточка кейса, `GET /api/cases/:id`.

## 10. Общая схема backend/frontend взаимодействия

- Что видно: frontend `localhost:5173`, backend `localhost:18080`, PostgreSQL, REST API и CORS.
- Зачем нужен: объясняет техническую архитектуру демонстрационного стенда.
- Подтверждает: README/API contract, Docker Compose, CORS preflight.
