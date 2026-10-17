## Status

Реализованный development read contract для E6; released/production acceptance остаётся открытой. Machine shape задаётся canonical schemas/Go DTO и новыми bundles; этот документ фиксирует решения change, не становится второй независимо редактируемой спецификацией полей после archive. Нормативная семантика — delta `prifly-light`, compatibility — delta `published-contracts`.

## Endpoints

Пути ниже относительны нормализованному `--base-path` Light listener (default `/`). Только GET/HEAD; все reads повторно проверяют доступ к зарегистрированному source. Query не содержит файлового пути.

| Путь | Назначение и ограничения |
|---|---|
| `api/health` | `service=prifly-light/1`, readiness/index coverage, base path; не masquerade как `prifly-local-monitor/1` |
| `api/light/board?q=&project=&source=&lane=&limit=25&cursor=` | Одна глобальная страница компактных карточек по выбранным source generations; ETag, next cursor, filtered count, counters и source coverage |
| `api/light/task?source=&key=&limit=25&cursor=` | Одна task/fallback карточка и страница связанных Run summaries/истории; explicit lookup, без полного RunView |
| `api/light/reviews?source=&run=&invocation=&limit=10&cursor=` | Страница review records с provenance; candidate/round/branch filters, без всех report bodies |
| `api/light/report?source=&run=&review=&kind=report` | Lazy bounded preview exact report/gate/request/candidate из выбранного review record; `kind` allowlist, не произвольный ArtifactRef/path/URL |

`limit` integer 1–50 для board/task, 1–20 для reviews; неверные фильтры/limits — 400 с типизированным Problem, unknown resource — 404, access denial — existing refusal. Обычные reads не создают queue/Run/receipt, не мигрируют storage. Report preview до 64 KiB с total bytes/truncated; весь raw report доступен через существующий full-monitor artifact view с отдельной явной загрузкой и проверкой доступа. Отсутствие historical bytes — unavailable, не пустой passed report.

Board sort: «Нужно участие» первым, далее native ready tasks по `priority desc, ordinal asc`, остальная работа по recorded last observation desc; ties по authority/task-or-run identity. `lane` фильтрует конечную таблицу display groups; поиск по recorded title, project и exact IDs, без сканирования raw bodies. Counters и filtered total относятся ко всему отфильтрованному index generation; число карточек текущей страницы подписано отдельно. UI не изображает counts по 25 карточкам как global totals.

Source key для routing совместим с full monitor: существующий `monitorSourceID` от canonical root. Он отличается от authority identity; ответ включает оба. Physical dedup/access/relocation guards сохраняются. При совпавших IDs в скопированной authority source conflict показывается отдельно, такие источники не сливаются и не считаются одной здоровой authority.

## Response shape

Новые identities: `prifly-light-board/1`, `prifly-light-task/1`, `prifly-light-reviews/1`, `prifly-light-report/1`, `prifly-light-projection/1`. Существующие `local-run-board/1`, `project-task-queue/1` и state/read не расширяются. Shapes закрытые, неизвестная schema version отказывается явно.

| Объект | Поля и смысл |
|---|---|
| Board page | `schema_version`, `mapping_digest` (null без mapping), `generation`, `sources`, `cards`, `page` (`limit`, `next_cursor`, `total`, `complete`), `counters` с coverage |
| Source read | `source`, `authority_id`, `project_id`, `cut`, `observed_at`, `last_success_at`, `freshness` (`fresh/updating/stale/unavailable`), `coverage` (`complete/indexing/unsupported`), `error` nullable |
| Card identity | `key`, `kind` (`queue_task/run`), source/authority/project identity, `task_id` nullable, `task_revision` nullable, `title`, `title_origin` (`queue/brief/task_input/fallback`), project title/origin |
| Card summary | `queue_state` nullable, `lane`, `phase` (`key`, `label`, `mapped`, `mapping_ref`), `status`, `outcome`, `completion_scope`, `reasons`, `round` nullable, `candidate_digest` nullable |
| Card collections | `current_runs` preview+count, `active_branches` preview+count, `reviews` preview+count, `history_count`, `truncated` и links на paginated details |
| Run summary | exact `run_id`, `run_version`, `event_sequence`, workflow ref, bound task revision, status/outcome, ancestry refs, recorded frontier relation; ни graphs, ни trace, ни inputs bodies |
| Review row | source/Run/invocation/stage/Attempt/iteration identities, lens, applicability, candidate/request/round basis, report/gate refs и schemas, gate scope, check/gate status, freshness, display state, findings preview+count/truncation, source cut, mapping digest |

У aggregate task с несколькими несопоставимыми frontier Runs `status=mixed`, `outcome=null`, `round=null`, `candidate_digest=null`; это только read-проекция. Отдельные actual Run status/outcome сохранены в `current_runs`. При однозначном scope round/candidate — только записанные значения, не номер Attempt или timestamp. Все nullable unknowns имеют reason в details. Unknown не заменяется 0.

`generation`/cursor — opaque token: query digest, выбранные sources и их generations/cuts, sort position и срок жизни. Cursor не выдаёт права. Смена filter без нового cursor или cursor другого владельца отказывается; просроченный — `light_page_expired`, UI сохраняет filters и предлагает начать с новой первой страницы. Continuation страниц использует тот же generation vector. Refresh первой страницы запрашивает новый vector; отменённые/запоздавшие ответы другой query generation не применяются.

ETag покрывает query, mapping digest, source vector/coverage и отображаемые rows. Staleness выводится также по `last_success_at` клиентскими часами, чтобы unchanged 304 не скрывал длительную потерю источника. Изменившаяся доступность/ошибка меняет ETag. Один source response содержит queue version, bindings, selected Runs и admission причины одного cut. Глобальный timestamp не переименовывается в общий ReadCut.

## Task and lane projection

| Сохранённый факт | Представление |
|---|---|
| Native ready без Run | «Очередь», recorded priority/order; нет round/review PASS |
| Claimed без admitted Run | «Ожидание запуска»; pending/unresolved intent с причиной в «Нужно участие»; наличие claim само не означает исполнение |
| Owner decision, active stop, unknown dispatch/effect, partial/failed/cancelled frontier | «Нужно участие» с конкретным типом; resource-only wait остаётся в рабочей группе с «Ожидает ресурс» |
| Готовность, execution, pending acceptance | Явная phase mapping либо «В работе / этап не сопоставлен»; ready/pending не называют запущенным |
| Native task claimed, все frontier Runs succeeded | «Результат получен», scope Run, task complete ещё не подтверждён; не колонка «Готово» задачи |
| Native task done | «Готово», scope «Локальная обработка задачи завершена владельцем»; прежние failed/partial остаются в истории |
| Standalone completed/succeeded | «Готово», scope «Run завершён», явный fallback; external delivery неизвестна |
| Несколько mixed frontier Runs | Видны все исходы/блокировки; приоритет attention, без единственного selected latest |

Приоритет группы: unresolved/control/owner attention → task done (только без противоречащих live данных) → очередь/ожидание запуска → recorded active phases → Run-only result → unknown. `done` с противоречащей live association — integrity warning/attention. Вложенные concurrent phases не сводятся к одному шагу: одна карточка, список phases/branches и count. Колонка выбирается детерминированно по приоритету declared phase list, полный список виден в details. Сам phase label не изменяет outcome.

## Mapping contract

`--light-projection` принимает локальный JSON, прочитанный до сервера, maximum 256 KiB, ≤64 workflow mappings, ≤128 stage rules на mapping, ≤16 lenses. Это observer config, не `.prifly/project.yaml` и не authoring contract. Валидация закрытой формы отказывает при неизвестном operator/version, дубликатах/конфликтах selectors, небезопасных pointers и превышении bounds. Mapping должен однозначно подходить по exact `WorkflowRevision` ref (`id/version/digest`) и schemas refs закреплённого Run. Templates в examples требуют разрешения refs из sealed fixture; wildcards по digest не допускаются.

Разрешённые selectors:

- Exact workflow/invocation path, stage identity, declared input/output port и schema ref; повтор выбирается по recorded iteration identity, не по последнему timestamp.
- JSON Pointer в typed артефакте с заранее объявленной schema и explicit expected enum/type. Conditions: scalar equality/in literal set и all/any bounded list, без произвольных expressions.
- Applicability predicate из sealed configuration/DecisionSheet/typed request либо доказанного recorded routing; mapping literal не может сам объявить, что проверка выполнена или выключена. При неизвестном predicate — unknown.
- Correlation: accepted producer inputs/outputs ArtifactRefs, request exact bytes digest, report candidate tuple и round equality. Digest strategy — `artifact-bytes-sha256` для request по exact consumed ref. Другой producer digest algorithm требует отдельной явной версии declarative contract; не приравниваем hash canonical JSON к hash arbitrary bytes.
- Current basis/invalidation selector: typed candidate producer и recorded repair StageActivation, выбранные mapping. Start нового repair инвалидирует прошлую приёмку до fresh capture. При неоднозначности current basis — unknown/stale, не highest round across Runs.
- Value tables: enum → display state и short labels; literal findings берутся из declared fields, выводятся текстом с truncation. Session/model declaration выводится как unverified claim, не критерий искусственной независимости.

Ни один selector не читает Git, workspace paths, Markdown reports или последнюю строку лога. Произвольные file URLs в evidence — только текст. Отсутствующий port/ref/schema или unsupported version делает соответствующую часть unknown/unavailable с diagnostics, не весь board пустым.

## Review decision table

Сначала проверяются applicability и basis/freshness, затем report checks и gate. Все исходные признаки сохраняются, таблица задаёт краткую подпись.

| Условия | Chip / вывод |
|---|---|
| Explicit disabled или not applicable evidence | «Выключено» / «Не применяется», раздельные applicability values, без PASS |
| Enabled, producer ещё не выдал результат и есть recorded pending work | `waiting`: «Ожидает ревью» |
| Producer завершён без обязательного report, отсутствует mapping или корреляция | `unknown`: «Нет данных», причина и refs |
| Typed invalid, mismatch candidate/round/request, previous basis invalidated | `unknown` с `freshness=invalid/stale`, «Нужна новая проверка»; прежний raw report исторический |
| Valid fresh report показывает executed failed required check | `defect`: «Есть дефект», даже если другой lens passed |
| Required check unavailable/not_run или relevant gate blocked | `unavailable`: «Приёмка заблокирована», причина/owner-only отдельно |
| Relevant gate failed при отсутствии проверенной lens детализации | `defect` только для scope этой gate, не выдумывает defect всех lens |
| Все требуемые lens checks выполнены/passed, связь с принимающей gate доказана, gate passed, нет blockers | `passed`: «Пройдено» |
| Report/gate warnings без blocking | `warnings`: «Есть замечания»; не скрываем под PASS, не считаем автоматически отказом |
| Optional excluded/not_applicable checks с разрешённым producer gate | Показываем coverage и пропуски; allowed skip не становится executed PASS |

`blocking=true` или `blocking_owner_only=true` запрещают aggregate acceptance даже при противоречивом status passed; такой противоречивый contract также помечается invalid. У aggregate gate blocked и одной fresh passed lens обе записи видимы: «Code: проверка пройдена» и «Приёмка: заблокирована». У слова «проверка» есть scope, зелёная общая приёмка запрещена. Raw `StepResult.verdict` остаётся отдельным полем provenance, таблица его не использует как review gate.

## Consumer fixture mappings

Источник проверенных schema/graphs: [consumer tree на exact SHA](https://github.com/StenHigh/prifly-aif-workflows/tree/3499115956903abd264fbde2be2f432d07bc2c43). Это source fixture, не доказательство пригодности всех его versions и не runtime dependency. W1/W2/W5 names следуют issue; qualification закрепляет конкретные package refs/digests. Optional observer data и fixtures располагаются в Core `examples/`/`test/fixtures/`; consumer repo и установленные workflow packages не меняются.

| Fixture | Явный producer/field mapping | Ограничения |
|---|---|---|
| W1 `aif-review-evidence@1.0.0` | `aif-evidence:workflow/review-evidence`, outputs `report`, `gate`; report schema `aif-evidence:schema/report@1.0.0`, gate `aif-evidence:schema/gate@1.0.0`; `/round`, `/request_digest`, `/candidate`, `/checks`; gate `/status`, `/blocking`, `/blocking_owner_only`, `/findings` | Lens label и check set задаёт explicit request mapping; у report нет lens field. Request digest проверяется по exact bytes consumed request. Нужны capture/evaluate invocation refs для current candidate и gate correlation, а не только finish exports |
| W2 `aif-ui-ux@1.0.0` | `aif-product:workflow/ui-ux`, outputs `ui_report`, `ux_report`, `gate`, `environment`, `release`; UI/UX используют тот же typed report; evaluate consumed inputs `ui_report`, `ux_report`, `candidate`, `ui`, `ux`, затем finalize связывает gate/environment/release | UI и UX — отдельные lens из declared producer ports; итоговая gate aggregate. Cleanup/environment blocker виден отдельно и не объявляет успешную общую приёмку по двум reports |
| W5 `aif-feature@1.2.3` | `aif-feature:workflow/feature`, nested `quality`/`quality-round`; outer `gate` schema `aif-feature:schema/feature-gate@1.0.0`, `evidence`; текущий candidate из typed capture и round из repeat inputs/outputs. `report-bundle` branches `security/review`, `uiux-bundle` branches `ui/ux`; их `ref` и raw_base64 сверяются с recorded ArtifactRef/digest | `surface`, `review_mode`, requests/route decisions объявляют applicability; backend не требует UI/UX. Inner security/review gates имеют lens scope, feature gate aggregate. Sequence и parallel mappings раздельны. Начальный feature round может быть 0, report round ≥1: не прибавляем единицу эвристически, используем фактический request/report текущего quality round |

Report bundle `status=unavailable` — отсутствие evidence, `raw_base64` не переносится на board. У W5 outer gate нет candidate/round: без достоверного inner invocation binding показывается только aggregate статус с неизвестной свежестью, lens chips unknown. У legacy classic/profiled нет обещания нового structured review contract: title/stage/status видны, unsupported review — «нет данных». Новые readers в workflow и package version bumps ради Light запрещены.

## Evidence gaps

Изучены исходники, но не запускались actual consumer fixtures, браузер, released binary или production listener. До выполнения задач apply ни mapping qualification, ни budgets, ни E6-A1–A10 не считаются пройденными. Нехватка producer данных — видимый contract limitation; нельзя чинить её молча дополнительным reviewer или prose parser.
