[Индекс](index.md) · [Обслуживание](operations.md)

# Диагностика по симптомам

Сначала прочитайте последнюю Problem-запись stderr. Сохраните `code`, version, exact command и нужные `violations`.
Не смешивайте stdout/stderr в один JSON stream. Не публикуйте секретные values.

| Симптом/код | Что прочитать | Следующее действие |
|---|---|---|
| Update network/signature/receipt failure | [Установка](installation.md) | Остановить новый Run; назвать уже применённые обновления |
| `project_runner_conflict` | `runners update --check`, exact managed paths | Сохранить local edits; не перезаписывать custom files |
| `project_workflow_modified` | Paths из diagnostic, origin | Сохранить local source changes; не remove/add автоматически |
| `project_workflow_origin_missing` | Project package declaration | Назвать package локальным; не придумывать upstream |
| `project_local_conflict` | Existing local.yaml | Сохранить machine-local configuration; не выполнять init поверх неё |
| `authority_configuration_incompatible` | authority root и bootstrap history | Project authority создаётся через project init; generic init не эквивалентен |
| Missing host/input/workspace | Questionnaire и declared inputs | Получить недостающее значение, не угадывать |
| Stale catalog/launch digest | Текущий questionnaire и prepare | Новый prepare и review, не удаление expected digest |
| `awaiting_host` | `run next`, его next_command | Взять уже выданный Attempt, не новый Run |
| `idle` | Status, ожидания и graph | Не выдумывать работу; follow declared next action |
| `answered_questions_missing` | Task question_report, [report](hosting.md) | Добавить правдивый top-level answered_questions |
| `workspace_tree_location_missing` | Task workspace_trees, WorkspaceTreeLocation | Передать output_port и относительный path для direct-child capture |
| `effect_not_permitted` | Named changed paths, permitted_effects | Исправить нежелательные writes; не расширять gate effects |
| `workspace_tree_output_exists` | Capture path | Сохранить partial bytes; owner решает очистку exact path |
| `workspace_tree_input_drift` | Input manifest и materialized tree | Восстановить exact input по допустимой процедуре, не принять похожие bytes |
| `model_profile_unanswered` | Task model_profile и schema | Honoured/unavailable/declined по факту, не выдумывать модель |
| `decision_conflict` | Текущий task/run decisions | Fresh Run version после take; exact pending digest для answer |
| `resource_busy` | Holder Run, capacity show | Подождать settlement и drive тот же Run |
| `capacity_conflict` | Capacity show, active attempts | Не повышать лимит без owner решения |
| `claim_conflict` | Claim list, doctor --claim | Проверить actual path/generation; не обходить чужой claim |
| `driver_already_active` | driver_live и process handle | Наблюдать действующий driver; не создавать второй |
| Timeout наблюдения driver | Тот же process/session handle | Re-poll; timeout наблюдения не доказывает завершение процесса |
| Run failed, outcome null | Status failure, next/explain | [Техническое recovery](recovery.md), не business retry |
| Terminal fail/partial/blocked | Explain finish и next arrived_from | Читать accepted outputs и declared continuation |
| `external_write_unreconciled` | Actual external state и obligation | Owner inspection/resolve, затем settlement и reviewed execute |
| `resume_prefix_empty` | Reviewed accepted prefix и dirty tree | Сохранить работу; не reset-ить source |
| `project_continue_active_child` | Named child Run | Продолжить/исследовать его; duplicate только намеренно |
| Нет stdout/stderr | Declared live_output | [Output guide](observability.md); нельзя включить задним числом |
| Нет progress | reported/stale/not_reported/unavailable | Progress диагностический; не считать отсутствующий progress failure |
| `stdout_limit` / `stderr_limit` | Output limit и program bindings | Исправить программу/declared limit для нового package, не sealed Run |
| Storage exhausted | storage budget show, doctor --storage | Reviewed повышение budget или supported cleanup |

Более подробные реальные случаи находятся в [существующем указателе](../troubleshooting.md).
Он охватывает nested recovery, composition, extension routes, model profiles и output limits.

## Если нужной страницы нет

1. Проверьте `references/manual-version.json` и установленный `version`.
2. Прочитайте public `help`, `schema`, `capabilities` и текущий `run next`.
3. Для нового запуска выполните обязательную процедуру updates.
4. При конфликте или отсутствии ответа сообщите documentation gap и остановите зависимое действие.

Missing page существующего Run не разрешает заменить его pinned instructions новой версией workflow.
Для обычной поддержанной операции не скачивайте Core и не исследуйте Go-файлы вместо справочника.
Разработка самого Core допустима как отдельная задача с нужными effects.

См. также [команды](commands.md), [восстановление](recovery.md), [индекс](index.md).
