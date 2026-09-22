# sppr-prototype frontend

React dashboard для демонстрации веб-прототипа СППР в контуре внутрифирменного планирования.

Frontend показывает реестр управленческих случаев, форму создания события отклонения, карточку кейса, диагностические гипотезы, сценарии, корректирующие действия, протокол `decision_trace` / П и HITL-операции экспертной валидации.

## Технологический Стек

- React
- Vite
- TypeScript
- Tailwind CSS

## Backend Dependency

Backend должен быть запущен на:

```text
http://localhost:18080
```

CORS на backend разрешён для:

```text
http://localhost:5173
```

## Переменные Окружения

```bash
VITE_API_BASE_URL=http://localhost:18080
```

Если переменная не задана, приложение использует backend URL, заданный в коде frontend.

## Запуск Frontend

```bash
npm install
npm run dev
```

Production build:

```bash
npm run build
```

## Основные Экраны

- Реестр кейсов.
- Создание кейса по событию отклонения.
- Карточка кейса.
- Блоки diagnostics, scenarios, actions и trace.
- HITL-кнопки: submit-validation, approved, rejected, returned, execute, archive.

## Полный Demo-Flow Через UI

1. Открыть dashboard на `http://localhost:5173`.
2. Создать кейс через форму события отклонения.
3. Открыть карточку созданного кейса.
4. Проверить diagnostics, scenarios, actions и trace.
5. Отправить кейс на экспертную валидацию.
6. Выбрать `approved`.
7. Выполнить действие через `execute`.
8. Архивировать кейс через `archive`.
9. Проверить, что trace содержит шаги workflow, экспертное решение и финальные status transitions.

## MVP v0.1

Frontend предназначен для демонстрации MVP v0.1. Авторизация, роли, Swagger UI, experiments runner и промышленная интеграция в этой версии не реализованы.
