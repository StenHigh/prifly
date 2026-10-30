## Context

Мотивация — `proposal.md`. Единственный затронутый нормативный source set —
`openspec/specs/delivery-roadmap/spec.md`. Требования runtime и authoring
из `workflow-and-context` здесь используются как ограничения, без изменения.

После review владелец подтвердил сохранение всех предложений в основном roadmap
и HTML в repository. Он также попросил расширить анализ за пределы YAML.
Итоговый scope — 14 направлений, два аналитических HTML-снимка и нормативные
ссылки. Читаемость остаётся первым направлением, но не заменяет развитие
исполнителей, среды, событий, API, прав и оценки качества.

В локально установленном `aif-classic` 60 YAML-файлов. Корневой граф содержит
158 строк, `steps/verify.yaml` — 22 строки, его context bridge — 111 строк.
Это снимок checkout `37d714f`, а не универсальная метрика всех packages.
Проблема состоит в плотных mappings, переходах между файлами и длинном
Markdown внутри YAML, а не только в размере графа.

Warp разделяет `factory.yaml`, файлы агентов, runners, skills и automations.
В `agents/implementer/agent.md` несколько настроек находятся в YAML frontmatter,
а процедура — в обычном Markdown. Связанный человекочитаемый объект виден
в одном месте. Но default маршрут распределяет foreman по инструкциям;
малый файл не эквивалентен нашему явному графу с каждым verdict и лимитом.

Источники сравнения:

- [Warp definition syntax](https://docs.warp.dev/factories/factory-as-code/).
- [Warp implementer](https://github.com/warpdotdev/warp-factory-examples/blob/84e7c952f1506c4bd2784d96fd40a6f4d6398cad/examples/02-sdlc-issue-to-pr/agents/implementer/agent.md).
- [Warp foreman](https://github.com/warpdotdev/warp-factory-examples/blob/84e7c952f1506c4bd2784d96fd40a6f4d6398cad/examples/02-sdlc-issue-to-pr/agents/foreman/agent.md).
- `cmd/prifly/project_compile.go`: `compileProjectComponent`,
  `projectContextSourcePathValue` и `projectContextSourcePath` уже читают
  строковый source внутри `.prifly/` и сохраняют его raw bytes.
- `internal/flow/authoring.go`: компактный YAML опускается в существующие
  definitions до исполнения.
- `examples/authoring/workflow-authoring-reference.yaml` — полный справочник,
  который нельзя оценивать как минимальный учебный workflow.

При подготовке предложения эквивалентность compact/block mappings проверена
на текущем `steps/verify.yaml` через `yaml.safe_load`: объекты совпали.
Сквозная CLI-проба отдельного Markdown source не завершена: disposable
`project init` вернул `forbidden`, а compile без инициализированной authority —
`authority_not_found`. Поддержка source подтверждена чтением compiler, не
успешным новым native compile или исполнением; эти границы не смешиваются.

## Goals / Non-Goals

**Goals:**

- Сохранить решение о развитии собственных инструментов как направление roadmap.
- Превратить полезные наблюдения в приоритеты с prerequisites и приёмкой.
- Разделить работу Pri-Fly и работу репозитория workflows.
- Дать конкретную отправную точку для улучшения читаемости без выбора нового языка.
- Показать инженерный путь от локального ядра к полезной фабрике, не объявляя
  все направления текущим release scope.
- Сохранить основания выбора рядом с roadmap, доступными после клонирования.

**Non-Goals:**

- Реализовать перечисленные возможности в этом change.
- Менять установленные packages или синтаксис на основании сходства с Warp.
- Переносить engine control flow в свободные инструкции агента.
- Реализовать benchmark platform, облачную фабрику или вызов модели этим
  документационным изменением.

## Decisions

1. **Одна добавленная секция roadmap, без новой параллельной очереди.**
   Delta добавляет три требования: 14 направлений, критерии authoring
   исследования и доступность аналитических отчётов. Они синхронизируются
   в `delivery-roadmap` в этом ходе по подтверждённому решению владельца.
   Активные исправления и P1/P2 сохраняют порядок и статусы. Эта секция
   описывает порядок выбора новой работы, а не отдельный release profile.
   Альтернатива — отдельный Markdown backlog — нарушила бы правило единой очереди.

2. **Читаемость начинается с доступного представления.** Длинный `text: |`
   можно вынести в `.md` и сослаться на него из context YAML. Существенные
   effects, verdict routes и limits записываются блоками; короткий список
   значений не требует механического разворачивания. В начало README package
   помещается короткая карта пути и назначение файлов. Она не становится
   вторым редактируемым графом; при изменении маршрута её актуальность проверяется.

   Пример существующей формы context, не новый формат:

   ```yaml
   id: example:context/verify
   version: 1.0.0
   media_type: text/markdown
   source: .prifly/workflows/example/instructions/verify.md
   ```

   `instructions/` здесь — место raw text под `.prifly/`, а не новая категория
   компонентов. Этот путь не участвует в выводе control flow. Отдельный source
   не сокращает число файлов, но даёт обычный Markdown preview и читаемый diff.

3. **Новая структура — проверяемый кандидат, не уже выбранная архитектура.**
   В следующем authoring исследовании сравниваются: текущая раскладка по типам,
   группировка связанных файлов на разрешённой глубине и объединённый Markdown
   шага с frontmatter. Для последнего нужен отдельный contract change: сейчас
   Project components остаются single-document YAML в известных каталогах.
   YAML anchors, merge keys и произвольное templating не используются как
   обход explicit refs или versioning.

   ```text
   Существующая форма → оформление и Markdown source → проверка человеком
                                                        ↓
                                             Осталась сложность?
                                               да → отдельный prototype
                                               нет → сохранить формат
   ```

4. **Сравниваются равные контракты.** Для простого program workflow и сложного
   assisted workflow фиксируются маршруты успеха/отказа, ports, bindings,
   лимиты, эффекты, retries и контекст. Readability tasks одинаковы до/после.
   Опускание должно сохранять эти факты; build provenance может измениться,
   поскольку учитывает authoring closure. Сравнение прежних и новых compiled
   references учитывает этот mapping и не требует ложного равенства digest.

5. **Существующие инициативы не дублируются.** Навигация и примеры остаются
   в `refresh-author-guides-and-examples`. Улучшения запуска и monitor сначала
   учитывают их активные changes. Выбор actual model остаётся отдельной задачей
   roadmap; новое сравнение конфигураций не объявляет этот выбор реализованным.
   Общее timing/telemetry переиспользуется, а отсутствующее измерение явно
   помечается. Scoring моделью не заменяет обязательный content check или gate.

6. **Улучшение процесса оставляет решение человеку.** Сначала берётся один
   повторный отказ. Предложение правки ссылается на неизменяемое evidence,
   затем проходит сравнение на закреплённых задачах. Автоматический merge,
   переписывание прошлых Runs и фоновое самопробуждение не следуют из этой идеи.
   Изменения prompts/packages ведутся в их repository; здесь появляется
   отдельная engine-задача только при обнаруженной потребности.

7. **Развитие исполнителей и внешнего контура получает конкретный вход.**
   Managed worker начинается с одного локального backend, explicit owner,
   limits и recovery; remote execution требует отдельной квалификации.
   Среда и browser evidence связываются с candidate и session. Schedule и
   webhook требуют живущего вне чата service loop, а не обещания, что срок
   сам продвинет CLI. API/MCP используют существующие controlled handlers,
   а модельный SDK и vendor-dependent prompts не входят в runtime.

   У Warp документированы отдельные per-agent harness/model, runners,
   self-hosted backends, automations, Factory MCP и credentials boundaries.
   Это существенные продуктовые преимущества над нашим текущим local path.
   Они не отменяются наличием у нас строгих contracts. Memory у Warp пока
   Research Preview; перенос идеи в roadmap означает исследование.

   Дополнительные первичные источники:

   - [Agent configuration](https://docs.warp.dev/factories/factory-agents/configuration/).
   - [Runners](https://docs.warp.dev/factories/runners/).
   - [Self-hosting](https://docs.warp.dev/factories/self-hosting/).
   - [Infrastructure and credential boundaries](https://docs.warp.dev/factories/infrastructure-and-security/).
   - [Factory inbox](https://docs.warp.dev/factories/factory-inbox/).
   - [Scorers](https://docs.warp.dev/factories/measure-and-improve/scorers/).
   - [Self-improvement](https://docs.warp.dev/factories/measure-and-improve/self-improvement/).
   - [Automations](https://docs.warp.dev/factories/automations/).
   - [Factory MCP](https://docs.warp.dev/factories/factory-mcp/).
   - [Memory preview status](https://docs.warp.dev/support-and-community/plans-and-billing/pricing-faqs).

8. **HTML хранится внутри delivery-roadmap как ненормативный снимок.**
   `reports/2026-10-30-readable-workflows.html` сохраняет прежний анализ YAML;
   его вводная отмечает, что approval и синхронизация относятся к последующему
   решению. `reports/2026-10-30-warp-lessons.html` даёт расширенный обзор.
   Оба отчёта ведут к `../spec.md`; их repository links относительны.
   Исходные absolute file paths не переносятся в repository.

   Отчёты не служат вторым backlog, не исполняются monitor и не требуют
   repo-specific builder. `answer-me-with-html` используется вне проектного
   build pipeline; в repository сохраняется только standalone HTML.
   Правило `release-documentation-layout` о единственном нормативном source
   сохраняется. Карта источников лишь поясняет ненормативный статус отчётов;
   ownership не меняется. Новая иерархия `docs/` не создаётся.

## Risks / Trade-offs

- [Лаконичный prompt принят за равный строгому workflow] → сравнивать одинаковые
  ports, routes, permissions и limits, включая отказ и исчерпание repair.
- [Новая структура выбрана до понимания причины сложности] → сначала проверить
  имеющийся source и блочную запись; формат выбирать по задачам читателя.
- [Число файлов выросло после выноса Markdown] → оценивать поиск и редактирование,
  а не обещать уменьшение файлов или строк.
- [Оформление принято за прежний build] → различать семантику context и новые
  compiled identities от изменившегося authoring closure.
- [Дублирование пакетов и backlog] → сохранять ответственность двух репозиториев;
  не создавать здесь задачи на изменение внешних workflows.
- [План принят за поставку] → отдельные capability changes и назначенное
  evidence для реализации; формальная приёмка P1/P2 остаётся прежней.
- [Отчёт становится второй спецификацией] → dated snapshot, явная ссылка
  на roadmap и отсутствие самостоятельного актуального task status в HTML.
- [Облачная инфраструктура вытесняет пользу текущего инструмента] → три группы
  развития, минимальный local worker прежде remote service, отдельная приёмка.

## Migration Plan

По подтверждённому решению владельца обновить delta, proposal и tasks,
сохранить оба HTML-снимка и применить delta к `delivery-roadmap`. Проверить
переносимость ссылок, согласованность направлений и protected paths. Runtime,
public schemas, словарные bindings и установленные workflow folders не меняются.
Коммит, push и архивирование этим запросом не требуются. Последующие работы
выбираются отдельными changes из новой секции roadmap.
