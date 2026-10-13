# Повторное использование проверок неизменённых входов

Ветка: prifly/993e5ca3bcf37e7e70b21db03d90e8c88e8eddc19ba0ba7740e9cafab141c19f
Дата: 2026-10-12

## Original Request

Давай реализуем OpenSpec change reuse-unchanged-verification-results, сохранив обязательные релизные проверки.

## Настройки

Профиль full; тесты включены; logging minimal; документация README/OpenSpec, без docs checkpoint. Runtime/schema изменений нет. Gate composition из sealed decision_context: ci-check-once, OpenSpec validation и diff check. Верификация команд остаётся в задачах change; новые helper команды появятся при реализации и не объявлены прошедшими заранее.

## Roadmap Linkage

Milestone: none. Change улучшает процесс репозитория, не закрывает product roadmap milestone.

## Requirements Reconciliation

Источник — openspec/changes/reuse-unchanged-verification-results/specs/** и design.md. Приоритет над текущими spec сохраняется до sync. Все 13 задач tasks.md входят в реализацию. Проверки CI привязаны к exact SHA; локальное evidence не допускает release. Upstream AIF package и общий generated runner не меняются. Claude UI follow-up остаётся отложенным.

## Задачи

Прогресс ведётся только в openspec/changes/reuse-unchanged-verification-results/tasks.md. Эта секция ссылается на IDs, без второго checkbox ledger.

1.1 — audit всех ci-check inputs и policy; scripts/ensure-ci-check.py и audit.md change.
1.2 — локальный snapshot/evidence/lock; scripts/ensure-ci-check.py и test_ensure_ci_check.py.
1.3 — negative cases, fresh vuln scan, staged bytes; те же helper tests.
1.4 — target ci-check-once и scripts-check в Makefile.
2.1 — общий exact-SHA selector и API fixtures, scripts/release_checks.py.
2.2 — ensure-release-checks.py и fake gh сценарии.
2.3 — serialization/preflight verify/qualify, expected SHA, source summaries.
2.4 — tag-release.py и release CI guard, полнота product gates.
3.1 — standing composition extend.yaml; новые Runs без изменения sealed прежних.
3.2 — AGENTS/README/PROJECT overlays; команды reuse/force/retry.
4.1 — disposable pipeline scenarios, counted command fixtures.
4.2 — живой CI на готовом опубликованном candidate после разрешения владельца.
4.3 — итоговые проверки и protected historical paths.

Каждый ID сохраняет критерий проверки из tasks.md. Helper reports печатают причину execute/reuse/wait и ссылки/fingerprint; environment secrets не печатают. Зависимости: 1.1→1.2→1.3→1.4; 2.1→2.2→2.3→2.4; helpers→3.1/3.2→4.1/4.3; готовый commit→4.2.

## План коммита

Один итоговый commit implementation сохраняет единый проверяемый candidate. Промежуточные checkpoints читаются по IDs 1.4, 2.4, 3.2; дополнительные commits не обязательны, поскольку полные ворота относятся к итоговому снимку. Claim branch не push-ится и не вливается автоматически.

## Проверяемая область

Изменения выводятся командой `git diff e4487d7e89391241e9bef8d690a41df1ac7c8ad0 --stat` в target worktree; она выполнена до записи плана и не содержит pinned moving counts. Runtime/internal/schemas и archived evidence защищены.
