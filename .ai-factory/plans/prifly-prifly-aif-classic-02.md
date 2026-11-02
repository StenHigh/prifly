# Эксплуатационный справочник и обновление перед Run

## Original Request

Я заметил что во время запусков run иногда не хватает только skill для выполнения работы ИИ. И агент начинает искать возможности prifly изучая его код и это выходит очень дорого. Нужна досконально подробная документация со ссылкой в существующем skill. В skill обязательно проверять новую версию prifly и workflows и обновляться перед запуском. Пользователь подтвердил локальную доставку справочника вместе с runner, поручил довести change до конца и влить изменения в GitHub, выпустить релиз при необходимости. Работать только в отдельном worktree.

## Requirements Reconciliation

Источник поведения: openspec/changes/add-agent-operations-handbook/specs/** и design.md.
Источник задач: openspec/changes/add-agent-operations-handbook/tasks.md.
Карта источников: openspec/SOURCE-OF-TRUTH.md. Существующие runtime contracts сохраняются.
Публикация, merge и необходимый релиз прямо разрешены владельцем в текущем поручении.
Это исключение из default owner-only publishing; права shell и exact-SHA release gates сохраняются.
Нового skill и каталога docs/ не создаём. Изменяем существующий prifly-run и доставку его references.

## Roadmap Linkage

none. Roadmap не меняется; documentation coverage не закрывает product qualification.

## Research Context

Source: warmup handoff текущего Run; formal RESEARCH.md отсутствует.
Active Summary: binary update, workflow update и runner update имеют разные границы и должны выполняться до prepare нового Run.

## Задачи и порядок

Все пункты выполняются по tasks.md, без второго списка галочек.

1. Пункты 1.1–1.4: inventory команд/flags/capabilities, protected baseline, fixture и измеряемый baseline чтения.
2. Пункты 2.1–2.14: индекс и полные тематические страницы, authoring gaps и проверяемые команды.
3. Пункты 3.1–3.6: embed assets, согласованная доставка и безопасное обновление всех hosts.
4. Пункты 4.1–4.7: frozen recognition, короткий runner, mandatory update, сохранение overlays.
5. Пункты 5.1–5.4: независимое coverage, ссылки canonical/installed, entry points и ownership map.
6. Пункты 6.1–6.7: практическая проверка H01–H16, измерения, targeted/full checks и защищённые historical paths.

Каждый пункт сохраняет свой id, критерий и проверку из tasks.md.
Состав изменения определяется из actual diff; заранее переписанные числа файлов не являются acceptance evidence.

## Проверки

Проверены на исходном дереве: openspec validation данного change и git whitespace check.
Процедуры и точные команды дальнейших проверок принадлежат каждому пункту tasks.md.
Новый guard сначала должен дать отказ на потерянной записи либо повреждённой ссылке.
Token/cost metrics не оцениваются выдуманным процентом: при отсутствии измерителя сообщаем bytes/calls.

## Commit Plan

План уже сохранён коммитом 709791df. Реализация сохраняется conventional commit на claim branch.
После принятого Run выполняются публикация и merge через GitHub из отдельного worktree.
При archive+sync этот временный план удаляется в том же коммите, что архивирует change.
Релиз выполняется только после exact-SHA verify/qualify стандартными repository helpers.

## Уточнения после проверки интерфейса

Для 2.4 и H03 показываем top-level workspace_trees с output_port и относительным path.
Для direct_child_file указываем путь выбранного файла; отсутствие location даёт workspace_tree_location_missing.
Ссылки на ArtifactRef workspace-tree output хост не заполняет: это делает движок после capture.
Для 2.4 и 2.10 используем точные имена из schema index: SessionSubmissionV7 и WorkspaceTreeLocation.
Нужное поле читаем через schema WorkspaceTreeLocation --def runtime_WorkspaceTreeLocation.
Отдельно показываем получение свежей Run version после session take перед DecisionRequest.
Эти уточнения одобрены пользователем и сохранены в improve_apply=all.
