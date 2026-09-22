# API Contract MVP v0.1

Base URL: `http://localhost:18080`

Frontend origin: `http://localhost:5173`

Error response:

```json
{
  "error": "message"
}
```

## Endpoints

- `GET /health`
- `GET /ready`
- `POST /api/workflow/process`
- `GET /api/cases`
- `GET /api/cases/:id`
- `GET /api/cases/:id/trace`
- `GET /api/cases/:id/diagnostics`
- `GET /api/cases/:id/scenarios`
- `GET /api/cases/:id/actions`
- `POST /api/cases/:id/submit-validation`
- `POST /api/cases/:id/expert-decision`
- `POST /api/cases/:id/execute`
- `POST /api/cases/:id/archive`

## POST /api/workflow/process

Request:

```json
{
  "event_id": "evt-hitl-011",
  "product": "A100",
  "plan_demand": 1000,
  "fact_demand": 1350,
  "capacity_load": 0.93,
  "forecast_error": 0.18,
  "campaign_flag": true,
  "alpha": 0.95
}
```

Response `201`:

```json
{
  "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "final_status": "action_selected",
  "top_cause_code": "capacity_overload",
  "selected_action_code": "keep_plan",
  "expected_loss": 50.961,
  "var_value": 88.72,
  "cvar_value": 88.72
}
```

## GET /api/cases

Response `200`:

```json
[
  {
    "id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
    "status": "action_selected",
    "risk_level": "unknown",
    "selected_action_id": "950ac51f-2df6-4567-aea5-ed833c060a9c",
    "created_at": "2026-05-19T12:38:09Z",
    "updated_at": "2026-05-19T12:38:09Z"
  }
]
```

Optional filter: `GET /api/cases?status=action_selected`.

## GET /api/cases/:id

Response `200`:

```json
{
  "id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "status": "action_selected",
  "risk_level": "unknown",
  "selected_action_id": "950ac51f-2df6-4567-aea5-ed833c060a9c",
  "created_at": "2026-05-19T12:38:09Z",
  "updated_at": "2026-05-19T12:38:09Z"
}
```

## GET /api/cases/:id/trace

Response `200`:

```json
[
  {
    "id": "96881d76-b549-43c9-9e17-52d0ea4570ad",
    "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
    "step": "cvar_calculated",
    "timestamp": "2026-05-19T12:38:09Z",
    "data_snapshot": {
      "expected_loss": 50.961,
      "selected_action_code": "keep_plan"
    },
    "parameters": {},
    "risk": {
      "alpha": 0.95,
      "tail_probability": 0.05,
      "var_value": 88.72,
      "cvar_value": 88.72
    },
    "expert_decision": {},
    "created_at": "2026-05-19T12:38:09Z"
  }
]
```

## GET /api/cases/:id/diagnostics

Response `200`:

```json
[
  {
    "id": "5c795ab3-e9ae-430d-acbc-864dd2306d5c",
    "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
    "hypothesis": "capacity_overload",
    "confidence": 0.93,
    "details": {
      "capacity_load": 0.93
    },
    "created_at": "2026-05-19T12:38:09Z"
  }
]
```

## GET /api/cases/:id/scenarios

Response `200`:

```json
[
  {
    "id": "fec455cf-a801-4d55-8c76-0e7bdc755c2c",
    "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
    "name": "baseline",
    "probability": 0.45,
    "loss_vector": {
      "kpi_values": {
        "service_level": 0.8265,
        "cost_index": 1.054,
        "capacity_load": 0.93
      },
      "constraints": {
        "capacity_over": 0
      }
    },
    "created_at": "2026-05-19T12:38:09Z"
  }
]
```

## GET /api/cases/:id/actions

Response `200`:

```json
[
  {
    "id": "950ac51f-2df6-4567-aea5-ed833c060a9c",
    "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
    "code": "keep_plan",
    "description": "Сохранить текущий план и продолжить мониторинг",
    "action_type": "no_change",
    "expected_loss": 50.961,
    "var_value": 88.72,
    "cvar_value": 88.72,
    "constraints_violated": {
      "has_critical_constraint_violation": false
    },
    "is_selected": true,
    "created_at": "2026-05-19T12:38:09Z"
  }
]
```

## POST /api/cases/:id/submit-validation

Response `200`:

```json
{
  "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "status": "expert_validation"
}
```

## POST /api/cases/:id/expert-decision

Request:

```json
{
  "outcome": "approved",
  "comment": "Решение подтверждено экспертом",
  "expert_id": "demo-expert"
}
```

Response `200`:

```json
{
  "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "status": "expert_approved",
  "outcome": "approved",
  "comment": "Решение подтверждено экспертом",
  "expert_id": "demo-expert"
}
```

Allowed outcomes: `approved`, `rejected`, `returned`.

## POST /api/cases/:id/execute

Response `200`:

```json
{
  "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "status": "executed"
}
```

## POST /api/cases/:id/archive

Response `200`:

```json
{
  "case_id": "83ee95db-e780-46ac-bcbc-4ad1a353d7db",
  "status": "archived"
}
```
