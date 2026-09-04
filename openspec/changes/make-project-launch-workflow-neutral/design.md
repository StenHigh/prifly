## Context

Мотивация и границы — в [proposal.md](proposal.md). Осмотр выполнен на
Pri-Fly `5b5c4ca` / `v0.9.1`, внешнем AIF package `v1.10.0`.

- `project_compile.go` сохраняет author id/version при смене profile/contexts.
  Fast и Full дают разные manifest digests; последовательный import в одной
  authority воспроизвёл `package_identity_conflict`. Это проверка import,
  не доказательство полного живого AIF Run.
- `runtime/packages.go` также запрещает пересечение component id/version
  между installed packages. Исправления только package identity недостаточно.
- `project.go` требует Git root и все три hosts; `project_start.go` требует
  brief/host и делает claim безусловно. Package profiles при этом уже
  произвольны и необязательны: Fast/Full/Ultra в compiler не зашиты.
- `runtime/driver.go` уже не требует claim для assisted `effects:none`;
  local-process уже использует отдельную Attempt workspace.
- `runtime/start.go` берёт executors из общей config по step ID. Готового
  per-Run API для bindings нет. `EffectiveConfiguration` намеренно описывает
  только обычные inputs, не executable/argv/permissions.

## Goals / Non-Goals

**Goals:** один Project route для трёх разных применений, сохранение exact
history, отсутствие отраслевых ветвей в CLI/runtime, небольшие отдельно
проверяемые срезы. Прикладное поведение остаётся в YAML packages.

**Non-Goals:** «универсальность» не означает поддержку всех executors, ОС или
операторов сразу. Первый managed путь использует существующий worker protocol,
а не превращает произвольный shell command в Pri-Fly worker автоматически.
Общая запись в произвольную папку без Git claim не вводится; managed scratch
не объявляется sandbox для недоверенной программы. Остальные исключения —
в proposal; новый plugin framework не нужен.

## Decisions

### 1. Общая compilation identity, не исключение для AIF

Для нового profile `/3` оба входа `project compile` и `project start`
используют одну b1-сборку. `/2` сохраняет legacy compilation с авторскими
id/version, включая известный отказ при конфликте вариантов; устранение
этого ограничения требует явного перехода проекта на `/3`. Profile `/3`
вводится уже в первом срезе, нейтральный init и optional зависимости — во втором.
После
profile/extend/value resolution compiler строит canonical описание эффективного
authoring closure с logical refs, exact external refs и hashes ресурсов.
Оно включает ВСЕ данные, влияющие на output bytes: manifest metadata,
decision catalog, supporting-file bytes и значения полей provenance.
В hash включается версия алгоритма сборки; время, absolute paths, порядок
inventory и сгенерированные из build key refs туда не входят. Host label не
меняет сборку, если выбранные inputs/bytes совпадают.

Build key — SHA-256 этого описания. Package и каждый owned component сохраняют
авторский ID, но получают детерминированную compiled version. Для package
берётся build key; для component — SHA-256 canonical tuple из build key,
kind, author ID и author version. Encoding: `0.0.0-b1.` плюс lower-case
base32 полного SHA-256 без padding (61 символ). Это укладывается в текущий
Version contract; `+metadata` не используется. Нельзя сокращать hash до
нескольких символов или использовать compiled version для выбора «последнего».

Все owned refs перепривязываются до итогового sealing, в том числе context
refs; external dependencies, core adapters и policy refs остаются exact.
`build-provenance.json` — объявленный inert data file в существующем manifest
по образцу decision catalog, не новый runtime объект. Он хранит author
identity, build algorithm/key, effective profile/settings и соответствие
author refs → compiled refs/root; final package digest в собственные bytes
не включается. Формат `prifly-build-provenance/1` имеет закрытую schema;
compiler/CLI сверяют root и каждую mapping с actual manifest exports перед
использованием. Отсутствие файла допустимо только для legacy package, где
применяется прежний exact root contract, а не поиск похожего root.
Source locations не выдаются за подпись. CLI показывает
человеку author version рядом с exact compiled ref. Внешний consumer новой
сборки получает именно этот compiled ref: author `ID@version` не превращается
в alias «последнего варианта». Existing внешние refs старых packages остаются
валидными, пока соответствующие packages доступны.

Альтернатива «поднять только package.version» оставляет component collisions.
Отключение collision checks ломает history/trust. Случайная версия устраняет
повторное использование. Все три отвергаются; registry/lifecycle не заменяются.
Новый вариант проходит отдельное ordinary trust admission exact manifest;
revocation не снимается recompilation. Массовый отзыв author version не вводится.

### 2. Нейтральный Project profile, условные зависимости

Новый `/3` сохраняет layout workflow folders и единственный YAML graph.
`hosts` можно опустить или перечислить нужные поддержанные hosts. Fresh init
без host не создаёт AI directories. Явное добавление выбранного host
переиспользует существующие templates и safe-upgrade checks. `/2` читается
по прежнему контракту; миграция команды — explicit reviewed edit, без
автоматического rewrite при start/add/update.

Обнаружение `.prifly/project.yaml` отделяется от Git identity. Существующий
параметр пути проекта сохраняет совместимость; отсутствие Git само по себе
не мешает init/list/questionnaire/compile/start. Получение workflow из
Git repository остаётся отдельной операцией и вправе требовать Git.

Требования определяются выбранной скомпилированной конфигурацией, а не именами
skills/stages: host-bound source требует host при compile; assisted task
требует host при start; assisted repository write требует Git claim.
У local-process `workspace_write` относится к Attempt workspace, не означает
автоматически запись в checkout. Для ветвей, потребности которых до Run
неизвестны, проверяется полный потенциально достижимый closure: зависимость
не скрывается ради удобства. Исключение неактивных ветвей возможно только
при доказуемом compile-time выборе, не по эвристике.

RunBrief становится таким же declared input, как другие данные; задача из
трекера не требуется сценарию обработки файла. Это требует versioned
RunStart/state/read/envelope contract, а не удаления одной CLI проверки.
Для нового пути основание и границы задаёт admitted Start с exact workflow,
typed inputs, policy/resources и owner intent; отдельная новая сущность
«задача» не создаётся. Если brief не объявлен, `brief_ref` отсутствует,
фиктивный ArtifactRef/пустой RunBrief не материализуется. Старые contracts
по-прежнему требуют brief и сохраняют digest/reader semantics. Workflow,
объявивший RunBrief, обязан его получить и в новом пути.
В нейтральном пути Git claim
отсутствует, а managed и assisted `effects:none` получают только scratch и
объявленные artifacts. Для Git-записи вопрос worktree/checkout остаётся
обязательным; `/3` CLI также требует выбор, `/2` default сохраняется.

### 3. Bindings локальных программ — отдельный узкий вход

Разведка реализации выявила ещё одну необходимую границу: CheckDefinition уже
исполняется runtime, но опубликованный PackageManifest `/1` не допускает
component kind `check`, а Project compiler не читает `checks/`. Для объявленных
в этом срезе check bindings вводится PackageManifest `/2`: закрытый contract,
выведенный из неизменяемого `/1` с новым marker и единственным добавленным kind
`check`. Compiler `/3` читает полные CheckDefinition YAML из `checks/`, проверяет
их существующим `ParseCheckDefinition` и выдаёт manifest `/2` только для пакета
с такими компонентами. Остальные пакеты сохраняют manifest `/1`; profile `/2`
явно отказывает новому `checks/`, не игнорирует файлы. Import и inspection
выбирают schema по marker, old `/1` по-прежнему отвергает `check`. Новый component
проходит обычные exact digest/identity/trust проверки; проверки не запускаются
при чтении YAML, compile или import. Новый executor либо check authoring sugar
для этого не вводится.

YAML объявляет bindings по локальному component selector, логическое имя
executable, argv и confined supporting files. Local configuration разрешает
использование установленного executable и связывает имя с machine path.
Объявление из скачанного package не создаёт локального разрешения; selected
bindings включаются в launch preview и требуют явного owner допуска.
Это не автозапуск scripts при установке и не механизм установки программ.

Compiler переводит selectors в exact refs выбранного closure. Versioned
execution-bindings payload проходит отдельную validation и передаётся в
Start; не помещается в `EffectiveConfiguration`. Один resolver используется
capability/context validation и pinning. Файлы из package разрешаются confined
и seal-ятся до write-транзакции; существующие `PinnedExecutor`, executable
digest и `snapshot:executors` переиспользуются.

Нельзя сохранять bindings в общем `prifly.json` при каждом start, временно
подменять `e.Config` или разрешать binding только по bare step ID. Обычный
старый Start без payload сохраняет старое resolution. Новый путь допускает
сосуществование двух versions/пакетов с разными executors и проверяет command
dedup по полному закрепляемому payload.

Конкретный authoring среза 2: необязательный `execution_bindings` в
`workflow.yaml` содержит карты `steps` и `checks`. Ключ — полный авторский
component ID внутри этого package, не stage ID; неоднозначная версия
отвергается. Значение объявляет логическое `executable`, `args`, `files`
(target → relative source внутри workflow folder), явные `timeout_ms`,
`grace_ms`, `max_output_bytes` и необязательный `context_profile_ref`.
Environment и абсолютные machine paths не входят в shared YAML.
`checks/` принимает полный CheckDefinition YAML; отдельный сокращённый
язык проверок не вводится. Новые bindings/check sources требуют profile `/3`.

Compiler включает inert `execution-bindings.json` в manifest: закрытый
`prifly-package-execution/1` содержит compiled exact refs, логическое имя
программы, config и supporting bytes. Эти данные участвуют в b1; без поля
bindings старый b1 input и его ключ не меняются. Компиляция ничего не запускает.
Machine-local `project local set --allow-executable NAME=/absolute/path`
записывает разрешённую программу в `local.yaml`, не в shared profile.
`project start --allow-execution` — явное разрешение выбранных программ,
аргументов и supporting files этого запуска; одного скачивания workflow
недостаточно. Перед import/claim CLI разрешает только bindings выбранного
compiled closure и проверяет inputs, capabilities и workspace requirements.
Для `/3` CLI по умолчанию берёт authority из local.yaml; явный `--project`
имеет прежний смысл и приоритет. Полный пользовательский summary до dispatch
остаётся следующим срезом 3, это разрешение его не подменяет.

### 4. Общий runner и вопросы, а не оркестратор AIF

Убрать прикладные правила только из текущего generated template. Frozen
templates и их hashes остаются для upgrade. Сложность графа, число reviewers,
improve limits и fix suggestions задаёт package; generic runner следует
ready tasks и effects. Модели и reasoning level не выбираются этим изменением.

Расширить существующий `project questionnaire`, не создавать вторую анкету:
preflight и optional runtime предответы, условия, итог policy и причины
ожидания. Использовать те же catalog digest/validation, что Start.
Финальный summary вычисляется и показывается до первого Drive/dispatch;
при устаревших sources/ответах пересчитывается до эффектов. JSON-клиент может
получить preview и передать explicit answers без чата или LLM.

Dynamic bridge остаётся для объявленных runtime requests. Неизвестный native
вопрос не «перехвачен автоматически». Attended/autonomous можно продолжить
с известной возможностью ожидания; unattended нельзя обещать по одному
пустому списку вопросов. Actor в local-owner profile не доказывает отдельного
человека: sensitivity не превращается в security isolation.

### 5. Три приёмочных пути, один механизм

| Путь | Где проверяем | Что считается результатом |
|---|---|---|
| CSV → validation → report, без Git/ИИ | Core public CLI corpus | Реальный managed worker, проверенный output, read после restart |
| command → assisted `none` → command, без Git | Core; scripted host для protocol, отдельное UI observation | Реальная передача task, durable decision wait, ответ и завершение того же Run |
| AIF Classic/Fanout | Внешний AIF repository против exact candidate | Same-authority profiles/extend, корректные skills, bounded live task |

Core regression для variants использует нейтральные profiles A/B, не AIF
fixtures. Во внешней проверке Fast → Full → Ultra → default → Fast идут
в одной authority, а не в пяти чистых проектах. Changing settings/exclude,
owned context bytes, повтор идентичной сборки, restart старого Run и tamper/
revocation — отдельные assertions в общем lifecycle check.

## Risks / Trade-offs

- Новые compiled refs отличаются от прежних → старые packages не удалять,
  не reseal-ить; сохранять их lock и readers. Больше variants занимают место;
  отдельный GC этим срезом не вводится.
- Новый `/3` profile и execution payload → обновить schemas/editor/CLI docs
  вместе; старый binary должен честно отказать, не трактовать их как `/2`.
- Условные зависимости могут быть избыточными → показывать причину заранее;
  динамическая ленивая выдача прав не входит в этот план.
- Trust package не равен разрешению executable → local allow mapping и
  ordinary admission сохраняются; в общей config ничего не переписывается.
- Скриптовый host доказывает protocol, но не поведение Codex/Claude или AIF →
  живые наблюдения записывать отдельно, отсутствие доступа не считать успехом.

## Migration Plan

Четыре последовательных среза перечислены в tasks. Каждый имеет свой
focused check и commit; длинные полные gates — один раз на собранном candidate,
не после каждого документа и не с повторной CI-публикацией одинакового кода.

`add-run-decision-catalog` tasks 4.2/6.3 и `add-native-host-question-ux` task 2.3
остаются открытыми до их собственного evidence. Не дублировать их реализацию;
результаты общей анкеты и наблюдений связываются с этими changes при приёмке.
AIF code/fixtures меняются в его repository; Core change не архивируется
под видом полной интеграции до записи результата внешнего gate.

До применения main specs сохраняют текущий baseline. При реализации обновить
glossary, editor schemas, опубликованные command contracts и entry docs по
соответствующему срезу, затем синхронизировать delta. Historical archive и
release manifests не менять. Возврат binary не откатывает созданные Runs:
для нового contract нужен совместимый reader; старые Runs и `/2` profiles
сохраняют прежние bytes и поведение.
