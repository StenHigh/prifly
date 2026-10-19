## Context

См. proposal.md. Recovery сейчас блокируется Run cancel flag, всеми active Stops и invocation cancel flag. Resolution сохраняет failed state и `resolved_applied`/`resolved_not_applied` diagnostic. `attempt.resolved` несёт command identity; diagnostic ID детерминированно связан с той же командой, Run, phase и code. Это позволяет проверить уже записанные источники без новой схемы.

## Goals / Non-Goals

**Goals:** открыть только reviewed новый Run с доказанным settled writer; сохранить cancellation в source, original inputs/base, decisions, один claim и native guards.

**Non-Goals:** blind retry/requeue, возобновление source, новый успешный writer result, перенос PASS после frontier, изменение canonical classic, выпуск/закрытие external E3.

## Decisions

1. Проверять eligibility существующим planner. Для cancelled failed source требуется correlated resolution journal/diagnostic конкретного failed writer после cancellation Stop, который покрывает его invocation. Все активные non-cancel Stops остаются барьером. Альтернатива удалить проверки флагов отвергнута: это разрешило бы неподтверждённую работу.
2. Единственную failure chain продолжает проверять existing root/call/repeat traversal. Invocation cancellation failed chain разрешается лишь при доказанной cancellation/resolution; сломанные links, независимые failures и live descendants отвергаются. Для uncertain ProcessOutcome допускается только owner-resolved Attempt, не догадка по exit.
Если global cancel уже завершил ancestor call/repeat как cancelled, source остаётся cancelled и использует существующий declared `resumable.from_cancelled` (rev8); тесты проверяют обе terminal формы.
3. Разрешённый owner resolution всегда ведёт к `execute`; никакой result candidate не переиспользуется даже при `applied`. Старые process facts сохраняются. Вложенная root stage переисполняется целиком, quality после неё также.
4. Workspace guards остаются при native Attempt preparation: prepare не материализует и не очищает дерево. Если producer path занят, start может передать claim новому Run, который остановится named `workspace_tree_output_exists` до выдачи Attempt. Owner сохраняет partial отдельно и освобождает path, затем готовит recovery от этого нового failed Run. Если owner освобождает path до start, первый recovery сразу допускает writer. Это документируется явно, без обещания preflight проверки файлов в PlanRecovery.
5. Public schemas остаются прежними: используются existing event identities и diagnostics; новая capability/authoring revision не нужна. Authoring index дополняется в existing строках recovery/resume, справочник показывает existing `resumable` и минимальную ревизию 8.

## Risks / Trade-offs

- [Похожий diagnostic без резолюции] → проверять command-correlated journal и Stop provenance; тесты удаляют/искажают proof.
- [Повтор после новой отмены] → каждое recovery проверяет текущий source и порядок cancel/resolution, source version и digest.
- [Вложенные scope/оставшиеся obligations] → проверять failure chain и terminal descendants, сохраняя отказ на неподдерживаемую topology.
- [Dirty bytes после writer applied] → existing native materialization/output guards; owner действие отдельно от resolution.
- [Прошлый PASS ошибочно переносится] → перенос только prefix до корневой frontier, execution tests проверяют новый quality Attempt.

## Migration Plan

Миграция хранения не требуется. Добавить регрессии, исправить общий planner, обновить справочники и current source map ссылкой на active delta. Выполнить focused tests, `make test-changed`, `git diff --check`; полные ворота один раз для итогового снимка через `make ci-check-once CHECK_CONTEXT=direct-issue-13`. Старые bundles/archive/evidence не менять. Выпуск и external consumer qualification остаются отдельным действием владельца.
