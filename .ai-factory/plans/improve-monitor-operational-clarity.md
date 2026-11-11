# План улучшения Full и Light

Дата: 2026-11-11.
Режим: full, подготовка без реализации.
Текущая ветка: `main`; новую ветку этот план не создаёт. Веткой реализации управляет штатный Pri-Fly claim.

Change: [improve-monitor-operational-clarity](../../openspec/changes/improve-monitor-operational-clarity/proposal.md).
Единственный источник прогресса: [tasks.md](../../openspec/changes/improve-monitor-operational-clarity/tasks.md). Здесь нет второго списка галочек.

## Original Request

Давай подготовим OpenSpec change и план улучшения Full и Light: ясная причина остановки и следующий шаг, исправление ложных подсказок, компактный экран и мобильная версия. Сохрани runtime, JSON-контракты и replay. Без push.

## Settings

- Testing: да. Сначала воспроизвести дефект; затем проверить текст, переходы и поведение в браузере. Существующие эталоны сохраняются.
- Logging: минимальный. Новые production console/telemetry не добавляются; тестовые результаты и screenshots сохраняются в owned output.
- Docs: да. Обновить существующие `examples/operations/observability.md` и `examples/authoring/light-guide.md` вместе с реализацией.
- Workflow: последующая реализация через `prifly-run → aif-classic`; приёмка через повторный `aif-ui-ux` на точном кандидате.
- Ограничения: без изменения runtime, storage, JSON, replay, исторических evidence и без push. На этапе подготовки коммиты не создаются.

## Roadmap Linkage

Milestone: «Понятный путь до результата и очередь внимания владельца» в [delivery-roadmap](../../openspec/specs/delivery-roadmap/spec.md).

Rationale: этот срез улучшает объяснение текущего состояния в двух существующих мониторах. Он не реализует всю очередь внимания и не закрывает направление. Release/production задачи активного Light change остаются отдельными.

## Requirements Reconciliation

Приоритет требований: delta и design этого change → действующие владельцы из [SOURCE-OF-TRUTH](../../openspec/SOURCE-OF-TRUTH.md) → контекст AI Factory и roadmap.

Общие новые нормы принадлежат `control-security-ux`. Full сохраняет контракт `local-run-monitor`. Light сохраняет source set активного `add-prifly-light-task-board`, включая read-only, grouping, mapping и review. Новый baseline Light не создаётся. Прежнее требование изоляции Light не запрещает явно заказанное изменение Full в этом отдельном change.

| Требование | Решение и граница | Проверка |
|---|---|---|
| Понять причину и следующий шаг | Приватные адаптеры уже прочитанных данных; пояснение и безопасный переход к подробностям | Delta: первое требование; acceptance R01–R07 |
| Сохранить достоверность причин | Не выбирать последнюю историческую ошибку; не придумывать owner; показывать неизвестное при нехватке данных | Full list Board и Full details разделены; R03–R05 |
| Не скрывать параллельную работу | Все причины сохраняют свой scope; category, lane, phase и outcome прежние | R02/R03/R06/R07 |
| Исправить ложные подсказки о данных | Успех и снимок относятся к конкретному запросу; ошибка транспорта не меняет Run | Delta: второе требование; R08–R12 |
| Сделать экран компактным | Основной блок до служебных сведений; фильтры и metadata доступны через раскрытие | Delta: третье требование; 390×844 и 1440×900; R13/R14 |
| Сохранить runtime, JSON и replay | Не менять продуктовые Go-файлы и схемы; сопоставить защищённые bytes с actual base | Acceptance E; existing Go guards и полные ворота |
| Подтвердить исправление исходных проблем | Повторить те же 12 checks; добавить сочетания, не удаляя неудобные проверки | Acceptance A/B/D; typed reports и cleanup receipt |

Полная матрица сочетаний, fixture-команды и критерии находятся в [acceptance.md](../../openspec/changes/improve-monitor-operational-clarity/acceptance.md). Решения и отклонённые варианты находятся в [design.md](../../openspec/changes/improve-monitor-operational-clarity/design.md).

```text
Причина и переход → Достоверность чтения → Компактный экран → Проверки и приёмка
        ↓                  ↓                    ↓                  ↓
     1.1–1.4            2.1–2.4               3.1–3.3            4.1–4.3
```

## Tasks

Пункты следуют порядку OpenSpec `tasks.md`. Их ID и зависимости сохраняются. Подробности проверки берутся из acceptance, а не из нового набора ожидаемых результатов.

### Этап 1. Причина и следующий переход

**1.1 — Зафиксировать исходную границу.** Сохранить actual base, dirty scope и защищённые bytes. Проверить реальные формы Full list/details и Light. Проверка: сравнение по acceptance E; активный source set Light и прежняя refactor branch сохранены. Продуктовых изменений нет.

**1.2 — Исправить объяснение Full.** Воспроизвести ложное ожидание отчёта. Добавить чистые адаптеры в `cmd/prifly/monitor.js` и поведенческие примеры R01–R06. Проверка: `node cmd/prifly/monitor_ui_test.cjs`. Зависит от 1.1.

**1.3 — Исправить объяснение Light.** Добавить `cmd/prifly/light_ui_test.cjs` и представление причин из существующего LightCard. Сохранить phase/lane/review. Проверка: `node cmd/prifly/light_ui_test.cjs`, R02–R07. Зависит от 1.1.

**1.4 — Подключить точные безопасные переходы.** Сохранить scope причин и раскрытие остальных препятствий. При неоднозначности Run открыть задачу. Проверка: оба Node suite, R03/R06/R07/R15. Зависит от 1.2–1.3.

### Этап 2. Достоверность чтения

**2.1 — Разделить состояния чтения Light.** Воспроизвести первый сбой и ложный кеш нового запроса. Учитывать query, cursor, объект, ETag и поздние ответы. Проверка: Light Node suite и browser R08–R11 по acceptance C. Зависит от 1.3.

**2.2 — Согласовать состояния чтения Full.** Отделить первый сбой, устаревший снимок и восстановление от состояния Run. Учитывать все параметры запроса, включая page. Проверка: Full Node suite и browser R08–R11 по acceptance C. Зависит от 1.2.

**2.3 — Обновлять признаки наблюдения.** Сначала воспроизвести изменение driver_live/freshness без новой версии Run. Затем исправить invalidation, сохранив focus и scroll. Проверка: Full Node suite, browser R12 и `TestMonitorBoardRefreshesAuthorityFactsAtTheSameRunRevision`. Зависит от 2.2.

**2.4 — Включить UI-проверки в обычные ворота.** Добавить `make ui-check`, оба suite в gates/CI и выбор затронутых тестов. Учесть unknown/all fallback и inputs evidence. Проверка: `make ui-check`, `python3 -B scripts/test_test_changed.py`, `make scripts-check`. Зависит от 1.2–1.3.

### Этап 3. Компактная компоновка и навигация

**3.1 — Уплотнить Full.** Сократить шапку; раскрывать служебные сведения; сделать мобильную строку вертикальной. Сохранить default view. Проверка: Full Node suite, `verify-monitor-board.py` по acceptance C и первый экран в обоих viewport. Зависит от 1.4 и 2.3.

**3.2 — Уплотнить Light.** Вывести основное объяснение перед metadata. Убрать большие пустые группы перед найденной карточкой, сохранив фильтры и counters. Проверка: Light Node suite, `verify-light.py` по acceptance C. Зависит от 1.4 и 2.1.

**3.3 — Проверить сохранение навигации.** Расширить существующие browser scripts для keyboard, back, popup, polling, filters и proxy prefix. Добавить в `verify-light.py/.cjs` режим `--scenario stops` с отдельным native export и общим owned setup. Проверка: Full replay и оба режима Light по acceptance C, R01–R15, обе темы и размеры экрана. Зависит от 3.1–3.2.

### Этап 4. Документация и приёмка

**4.1 — Обновить справочники.** Описать причины, безопасные переходы, свежесть, раскрытие фильтров и пределы наблюдения в двух существующих страницах. Проверка: `openspec validate improve-monitor-operational-clarity --strict`, `git diff --check` и сверка с browser replay. Зависит от 3.3.

**4.2 — Повторить штатный аудит.** Защитить implementation candidate перед новым claim. Выполнить `aif-ui-ux` с теми же 12 IDs и матрицей сочетаний. Проверка: native terminal status, typed reports, required checks и точный release receipt по acceptance D. Зависит от 2.4 и 4.1.

**4.3 — Проверить итоговый снимок.** Сопоставить protected history, JSON/replay и actual base. После последних содержательных правок выполнить `make test-changed` либо `make check-fast`. Перед коммитом выполнить `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1` и `git diff --check`. Зависит от 4.2. Push и закрытие Light E6/roadmap не входят.

## Commit Plan

Группы ниже обозначают границы будущего ревью. Они не разрешают коммит на этапе подготовки. Перед каждым фактическим коммитом выполняются ворота для его итогового снимка; повторное evidence допустимо только по правилам `ci-check-once`.

| Группа | Пункты | Предлагаемое сообщение |
|---|---|---|
| Причины и переходы | 1.1–1.4 | `fix(monitor): explain blockers and preserve scoped next steps` |
| Достоверность чтения и проверки | 2.1–2.4 | `fix(monitor): scope read state and refresh observation facts` |
| Компактный экран | 3.1–3.3 | `feat(monitor): improve compact and mobile layouts` |
| Справочники и приёмка | 4.1–4.3 | `docs(monitor): document state explanations and acceptance` |

Если проектный workflow требует один implementation commit, эти группы остаются границами ревью внутри него. Не создавать отдельные коммиты только ради таблицы. Предыдущий рефакторинг и установка audit workflow не включаются автоматически.

## Условие завершения

План готов к исполнению, когда OpenSpec проходит strict validation, все task IDs связаны с проверками, а независимое ревью не оставляет блокирующих противоречий.

Реализация готова к приёмке только после задач 1.1–4.3. Проверка документа сама по себе не доказывает качество интерфейса. Race и полный e2e остаются проверками перед релизом.

## Проверено при подготовке

Два агента независимо проверили ограничения Full и Light. Ревью уточнило identity страницы, текущую ошибку другой ветви и отдельный набор причин остановки Light. После правок блокирующих замечаний в проверенном объёме нет.

`openspec validate improve-monitor-operational-clarity --strict` проходит. Ссылки, порядок 14 task IDs и формат новых файлов проверены. Delta содержит 3 требования и 22 сценария. Существующие 127 изменённых файлов сохранили исходные bytes; продуктовый код не менялся.

Go/Node/browser проверки реализации и полные ворота на этом этапе не запускались. Их команды в acceptance являются планом будущей проверки. Коммит и push не выполнялись.
