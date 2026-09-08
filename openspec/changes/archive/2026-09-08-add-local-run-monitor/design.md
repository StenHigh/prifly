## Context

См. proposal.md. Работа ведётся от актуального GitHub HEAD b1c8e93 в отдельном Git worktree `codex/local-run-monitor`; архив GitLab и незакоммиченные правки соседнего checkout не входят в изменение. Существуют embed HTML monitor, Runtime RunView/TimingTree, artifact reads и bounded полный scan. Данные чатов не нужны.

## Goals / Non-Goals

**Goals:** объединить локальные authority без новой управляющей области; вынести обычное чтение истории из ограничения полного telemetry scan; сделать сохранённые facts доступными через понятные экраны.

**Non-Goals:** сбор чатов/stdio, новый executor, изменение workflow authoring и pinned state, изменение лимитов исполнения, управление через web, diff, remote/SaaS, release или закрытие P2 gate.

## Decisions

1. **Существующий Go binary и встроенные HTML/CSS/JS.** Без frontend dependency/build chain и CDN. Схема — SVG с доступным списком узлов рядом; layout из закреплённых stage edges, включая циклы, с отдельным выбором WorkflowInvocation. Граф показывает определения, дерево показывает реальные instances.
2. **Локальный каталог ссылок на authority roots.** В user config Pri-Fly/monitor хранится отдельная атомарная запись на canonical root. Это служебные настройки наблюдения, не новая domain identity. Создание Run через CLI добавляет root; запись вне authority transaction. Default search проверяет стандартное расположение, home/temp и доступные локальные каталоги, включая нестандартные roots; явный scan-root позволяет ограничить или расширить область. Поиск асинхронный, без чтения содержимого посторонних документов, только поиск конфигурации/installation; ошибки и исключения считаются и видны. Символические aliases дедуплицируются по физическому каталогу; owner проверяется до включения.
3. **Короткие read transactions и отдельный каталог UI.** Страницы snapshots используют keyset cursor, digest verification и текущий read access. Полный telemetry scan сохраняет свои ограничения. HTTP содержит source identity и run ID, а не произвольный filesystem path. Кэш списка обновляется из проверенных источников; незавершённый обход отмечается. Неисправный источник не обрушает общий ответ.
4. **Автозапуск через CLI после успешного создания.** Повторно используется тот же executable с monitor в отдельной process session, stdio отвязан; loopback bind обеспечивает единственный listener. Регистрация и запуск происходят до drive и вне write transaction. Тесты подменяют side-effect entry point, интеграционная проверка использует временный user config и отдельный порт. Ошибки не проходят через управляющий process observer и не отменяют Run.
5. **Чтение имеющихся фактов.** Обзор включает Brief, outcomes, stops/pending checks/decisions, diagnostics, timings, artifacts, accepted/candidate result. Инструкции берутся из pinned context/envelope. Ни строка ERROR, ни статус completed сами не обозначают предметный успех. Files — сохранённые workspace manifests с указанием границы доступности. Model/reasoning/tokens показываются только при реальном наличии; отдельный сбор не добавляется.
6. **Устойчивый UI.** hash navigation хранит source/run/node. Polling 1500 ms не накладывает запросы, stale responses не заменяют новый выбор. Перерисовка сохраняет раскрытие и позицию чтения. Все строки из storage проходят textContent/escaping; артефакты не загружают внешнее содержимое. Светлая/тёмная темы — prefers-color-scheme.

## Risks / Trade-offs

- Большой filesystem/история → фоновый проход, страницы и видимый progress; обещание полноты относится к проверенной доступной области, недоступность не скрывается.
- Удаление/move хранилища → ошибка либо исчезновение записей после подтверждённого удаления; старый кэш не становится текущим состоянием authority.
- Копия authority с той же identity → явный конфликт источника, без молчаливого смешивания расходящихся историй.
- Недостаточные historical file/model facts → честное «не записано»; не добавлять несогласованную интеграцию ради заполнения поля.
- Занятый порт и запрет фонового spawn → предупреждение, сохранение исполнения и явный ручной monitor.
- Большие вложенные Run → загрузка подробностей по запросу и явная ошибка лимита; не возвращать ложный пустой Run.

## Migration Plan

Добавляются служебный user каталог и read surface; миграции базы Run нет. Старые authority обнаруживаются и читаются существующей проверкой версий. Откат binary оставляет служебный каталог неиспользуемым и не меняет историю. После реализации обновляются entry-point docs, SOURCE-OF-TRUTH и current backlog; historical evidence и public bundles защищены проверкой diff.
