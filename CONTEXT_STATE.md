# CONTEXT_STATE

Рабочее состояние репозитория для следующей сессии. Нормативная правда —
в `openspec/` (см. `openspec/SOURCE-OF-TRUTH.md`); этот файл только
ориентирует.

Обновлено: 2026-09-04. Это рабочая копия Pri-Fly (`StenHigh/prifly`),
начатая свежей историей из GitLab-дерева `main` = `27fa58c`. GitLab-проект
`stenhigh/prifly` архивирован (read-only, README указывает на GitHub).

## Переезд на GitHub (change `migrate-hosting-to-github`, заархивирован)

- Module path `github.com/stenhigh/prifly`; updater и `scripts/install.sh`
  читают `https://github.com/StenHigh/prifly/releases/latest/download/`.
- CI — GitHub Actions: `verify.yml` (branches/PR, теги исключены),
  `race.yml` (manual), `release.yml` (tag `vX.Y.Z`; darwin/arm64 нативно на
  hosted `macos-14`; publication job за environment `release` с
  `contents: write`; `gh release create --verify-tag`).
  `test/e2e/verify-release-ci.py` проверяет этот контракт.
- Spec `release-distribution` синхронизирована с delta change; README и
  SECURITY.md указывают на GitHub.
- Репозиторий `StenHigh/prifly` создан (initial `8bb73d0`), `verify` и
  manual `race` на GitHub-runner зелёные. Настроены environment `release`
  (required reviewer StenHigh), variable `PRIFLY_RELEASE_PUBLIC_KEY`
  (восстановлен проверкой подписи манифеста v0.5.0), ruleset `release tags`
  (`v*`, bypass только admin).
- Первый релиз с GitHub: tag `v0.6.0`, run `33769043603` после approve
  владельца; installer с `releases/latest/download` проверен на darwin/arm64
  и linux/amd64, `prifly update` читает signed manifest с GitHub. Каталог и
  `prifly-aif-workflows` (README, installer URL в CI) переведены на GitHub.
- GitLab-проект архивирован (его `main` `117e2ff` с указателем на GitHub);
  релизы `v0.2.0`–`v0.5.0` там остаются доступны. Существующие установки
  `v0.5.0` смотрят на GitLab и обновлений не увидят — переустановка новой
  командой. Рабочая копия — `Pri-Fly/github`; `Pri-Fly/isolated` —
  замороженный GitLab-клон, нужен только как источник `.tools`.
- Полный `make race` на этой машине под нагрузкой (load ≈ 8) даёт
  `schema_timeout`/`database is locked`; доверять GitHub `race.yml`.

## Надёжность и производительность authority (change `harden-authority-reliability-and-performance`, заархивирован)

51/51, все ворота зелёные (`make check` с race, `make e2e`, `--strict`,
`git diff --check`). Delta синхронизирована в `runtime-resources`,
`architecture-decisions`, `cli-protocol`, `control-security-ux`,
`delivery-roadmap`, `quality-and-acceptance`, `release-distribution`;
измерения и не достигнутые цели — в `evidence.md` архива
`openspec/changes/archive/2026-09-04-harden-authority-reliability-and-performance/`.
Roadmap milestone и product gate этим не закрыты.

- Хранилище перешло на версии 5 и 6: `authority.verified_cut`
  (инкрементальная проверка при открытии) и таблица `pinned_bytes`
  (закреплённые байты хранятся один раз по digest). Миграции применяются
  по одной, `open` больше не объявляет базу текущей после первой из них.
- Переходы состояний пишутся событием `state.changed` и восстанавливаются
  из журнала при чтении; форма Run и его digest не изменились.
- Схемы валидируются in-process, если оценка обхода схемы ниже бюджета;
  подпроцесс остался для экспоненциальных схем и за
  `PRIFLY_SCHEMA_WORKER=1`. Это уточнение дизайна: подпроцесс существовал
  не только из-за ReDoS.
- `synchronous=NORMAL` намеренно НЕ включён (4.4): выигрыш 15.6 → 13.7 с
  не оправдывает потерю последних коммитов при потере питания, и
  `runtime-resources` требует FULL.
- Отказы теперь типизированы (`runtime.Fault`), ворота `make refusal-check`
  и `CGO_ENABLED=0 go vet ./...` добавлены в `ci-check`.

## Открытые changes (`openspec list`)

- `add-run-decision-catalog` — 18/20; открыты 4.2 (ручное наблюдение в
  Codex и Claude Code) и 6.3 (bounded live pilot).
- `add-native-host-question-ux` — 6/7 (task 2.3 — ручное наблюдение UI).
- Завершённые, но не заархивированные: `consolidate-current-delivery-backlog`,
  `add-darwin-arm64-release`, `add-project-workspace-mode`,
  `fix-protected-release-publication`, `add-project-workflow-options`,
  `add-release-installer-and-update` — перед удалением любого их требования
  архивировать без sync.

## Следующие шаги

- Следующий релиз включает новую версию хранилища (миграция
  v4→v5→v6 при первом открытии) и новый asset релиза
  `release-manifest.jcs.sig`; обновление старым `prifly update` продолжает
  работать по прежней подписи.

- Релиз `v0.8.0` (2026-09-04) выпущен на коммите `2f8a8a1`: читаемость
  инструмента (справка подкоманд, `--version`, `help TOPIC`, индекс из 362
  контрактов с восемью авторскими документами, `project local set`), отказы
  с предметом (`authority_not_found`, `no_active_handoff`, известные имена
  компонентов, запечатанный package с причиной, `received:` в usage), stable
  code в диагностике драйвера, read view отдаёт reported summary. Change
  `improve-cli-discoverability` заархивирован, delta синхронизированы в
  `cli-protocol` и `observability-publication-reactions`.
- Пакет AI Factory: `aif-classic` v1.1.0 (удаление `.ai-factory/PLAN.md` при
  финальном коммите) и v1.2.0 (у разогрева выход `handoff`, планирование берёт
  его входом); каталог переведён на v1.2.0. Дальнейшую работу по
  `prifly-aif-workflows` ведёт одноимённая сессия — сюда её не тянуть.
- Открыто по отчётам пилота: `run status --run <id>` говорит «нужен ID» вместо
  «ID позиционный»; `package_not_resolvable` приходит с пустыми `violations`;
  двойная ссылка в корне `SessionSubmissionV5`; ответ `session submit` отдаёт
  состояние Run во вложенной квитанции и читается как «отправка в обработке».
- Из завершённого прогона (2026-09-04, все шесть шагов приняты с первой
  попытки): текстовый `run status` не печатает вердикты шагов, из-за чего
  исполнитель читал `state.sqlite3`, хотя `--json` несёт
  `run.steps[<id>].verdict`; `required` у runtime-решения не энфорсится нигде
  и не может — движок не заставит исполнителя задать вопрос, слово вводит в
  заблуждение; публикация не-древесного выхода (байты в объявленный слот,
  `artifact_id`/`revision` из `context.json` плюс digest) нигде не записана и
  трижды выводилась догадкой; схема, объявленная внутри package, не
  разрешается командой `schema` — только `package inspect` или файлом; дамп
  одной формы тянет 311 КБ, нужен селектор `--def`. Не дефекты, проверено:
  `permitted_effects: [write_inside_declared_output_slot]` выдаётся любому
  assisted-шагу и класс `none` объявлен верно — класс описывает воздействие
  вне рабочей области попытки.
- Ночной автономный прогон `aif-classic` 1.7.1 (отчёт пилота, 2026-09-04).
  Движковые пункты закрыты в v0.9.0, change
  `2026-09-04-report-unanswerable-autonomous-decisions` заархивирован с sync:
  - Ответ на решение был недоступен по документированному пути: `run decisions`
    не отдавал производный `DecisionRequestDigest(pending)`, а `local.Receipt`
    сериализует digest полезной нагрузки команды под тем же именем
    `request_digest`. Теперь read печатает `pending_request_digest`, а отказ
    разделён на четыре с указанием текущего и полученного.
  - `project start --runtime-answer ID=JSON` запечатывает ответ владельца на
    runtime-решение до старта; мост применяет его раньше автополитики и пишет
    источником `actor`. Это и есть путь к ночному заходу без останова.
  - Под `--decision-policy autonomous` результат launch несёт
    `autonomy_unanswered` — что политика взять не сможет и почему; решения с
    запечатанным ответом в перечень не попадают. Условие автоответа живёт одно
    (`autonomousBlock`), поэтому перечень и мост разойтись не могут.
  - Отдельно найдено и починено то, чего в отчёте не было: read-only открытие не
    мигрирует, а чтения безусловно выбирали v6-колонки `state_packed`/
    `snapshot_packed` — любая authority прежних версий отвечала бы
    `persistence_unavailable` на первое же `run status` после обновления.
  - Продуктовое, остаётся: `sensitivity` в autonomous — объявление, а не
    граница. Исполнитель ответил себе сам через `run decision answer`, потому
    что это тот же OS-principal. Согласуется с trust model F1, но вслух нигде
    не сказано.
  - Не наше (передано в `prifly-aif-workflows`): мост планирования молчит про
    вход `handoff` и теряет объявленный `gate_warnings`; выжимка не перечисляет
    проектные оверлеи, включая MANDATORY `skill-context/aif-plan/SKILL.md`;
    `handoff` не объявлен `format: json`, поэтому байты выхода не проверяются
    (движок проверяет их ровно при таком объявлении). Автору пакета отдельно
    сказано НЕ переобъявлять `improve_apply` как `automatic: true,
    sensitivity: ordinary` — автономность теперь берётся предответом.
- Замер выжимки (тот, о котором договаривались): корпус 919 687 знаков,
  прочитано ≈125 000, `handoff` 10 518 — сжатие ≈12×, а не 58×. Из прочитанного
  44 500 знаков ушло на обвязку самого Pri-Fly, то есть больше трети — на
  понимание инструмента, а не проекта. Вывод про форк сессии не меняется.
- Приёмка Decision Bridge: зелёный прогон пилота его НЕ проверил — условие
  вопроса о группировке не наступило, потому что предыдущий шаг закоммитил
  всё сам. Живой пилот моста требует дерева с незакоммиченными изменениями на
  входе в шаг фиксации; задачу `add-run-decision-catalog` 6.3 таким прогоном
  не закрывать.
- Замер, о котором договорились с владельцем: на живом прогоне сравнить размер
  выжимки разогрева с объёмом, который он прочитал, и суммарную стоимость
  шагов. Если выжимка сопоставима с сырьём — вернуться к форку сессии хоста
  (сегодня отклонён: протокол его не запрашивает и не проверяет, а
  унаследованный контекст ломает воспроизводимость).
- Релиз `v0.7.0` (2026-09-03) выпущен из GitHub Actions на коммите `d19d537`:
  именованные отказы приёма отчёта, предпроверка до записи candidate, захват
  деревьев по объявленным bindings. Change `fix-assisted-submit-diagnostics`
  заархивирован, spec `cli-protocol` синхронизирована. Обе pilot-сессии
  оповещены.
- Следующий change — `improve-cli-discoverability` по очереди ниже.
- Ручные наблюдения владельца: `add-run-decision-catalog` 4.2 и 6.3,
  `add-native-host-question-ux` 2.3; после них — архив с sync.
- Установки `v0.5.0` не обновятся сами: переустановка командой из README;
  GitLab-проект владелец скоро удалит, заметок о переходе не делать.
- Очередь после `fix-assisted-submit-diagnostics` (по отчётам pilot-сессий,
  2026-09-03): `improve-cli-discoverability` — `prifly schema` без аргумента
  выдаёт список имён, `submission_schema_ref` в SessionTask, алиасы
  `core:schema/...`; `authority_not_found` вместо `not_found` при пустом
  `--project`; `session task` без удерживаемой передачи отвечает отдельным
  кодом (`no_active_handoff`) с `run.explain`/`run.drive`, а не `not_found`;
  `--help`/`-h` на подкомандах, `--version`, `help <topic>`; `invalid_usage`
  показывает полученное значение (`received: …`, пути с пробелами);
  `project local set --executable`; `prifly schema` печатает список имён;
  описание пары `result_schema_ref.id` против `schema_version: const "1"`;
  `description` для
  `WorkspaceTreeLocation.path` и `SessionSubmission.result` в generated
  schemas; именованные каталоги вместо `context/skills/{00,01}`; не создавать
  пустой `outputs/` в claim; `run status` считает выходы Run и шагов
  раздельно; `waiting_host` через versioned bump `CoreNextView` (enum уже без
  `waiting_decision`); `prifly update` печатает адрес проверенного manifest.
  Затем `pin-skill-reference-trees`: источник контекста-каталог, закрепляемый
  как tree manifest, плюс отказ при запечатывании навыка с неразрешимой
  ссылкой. Обходной путь до него: каждый `references/*.md` объявляется
  отдельным `context_refs` (список).
- Ответы пилотам 2026-09-03: состояние совместимо между релизами (v0.7.0 не
  менял ни одну опубликованную схему, ci-check сверяет байты); `external_write`
  отказывает как граница профиля (`start.go` и контракт assisted-шага), P2-09
  не имеет горизонта — её предпосылка P2-08 тоже не принята; ActionIntent и
  ActionAdmission существуют, доставки нет, и класс эффекта шага обязан
  совпадать с намерением, поэтому промежуточной формы публикации сегодня нет;
  подтверждение человеком делается через Decision Bridge с объявленным
  runtime-решением, отдельного порта вопросов не будет.
- Рекомендация пилоту 2026-09-03: запечатанное описание публикации строится по
  опубликованному `ActionIntent` (`targets` как `canonical_id`/`provider_ref`/
  `scope`, `preconditions` с `expected_version`, `arguments` со своей схемой,
  `operation` и `effect_class`); `tool_ref`, `dispatch_not_after` и идентичности
  прогона проставит движок, когда появится доставка. Совместимость не обещана,
  совпадение имён сводит будущую правку к отображению.
- В очередь: отказ при запечатывании шага с недопустимым `effects.class` должен
  называть обе границы (квалификация профиля и контракт assisted-шага) — из
  объявления они не видны.
- Для бэклога `prifly-aif-workflows` (отчёт об `extend.yaml`, 2026-09-03):
  добавить комментарий к `extensions` рядом с существующим комментарием к
  `exclude` в поставляемом `extend.yaml`; абзац в README пакета — нынешний
  текст читается как «`extend.yaml` умеет только вычитать», хотя он и
  добавляет шаг в ребро графа. Ответы на их открытые вопросы: `input_bindings`
  у расширения не бывает вовсе (вставляется шаг без входов, всё сложнее
  требует явного графа), существование шага на этапе разбора формы не
  проверяется — ссылки разрешаются позже.
- Для бэклога `prifly-aif-workflows`: шаги 5-6 `aif-improve` должны объявить
  runtime-решение и вызывать мост; `references/**` закрепить отдельными
  `context_refs`.
- В шаблон runner-навыка добавить: `decision_bridge: true` — способность
  поднять запрос, а не открытый вопрос.

## Нюансы

- GitLab `stenhigh/prifly` (id 85838592) архивирован: push туда невозможен,
  `glab` остаётся залогинен (unarchive — `glab api -X POST
  projects/85838592/unarchive`). В `Pri-Fly/isolated` ничего не менять.

- Гейты запускать по абсолютному пути: `make -C /Users/sh/PhpstormProjects/Pri-Fly/github …`;
  `.tools` — symlink на toolchain соседней GitLab-копии.
- Параллельный запуск `make ci-check` и `make race` на холодном кэше даёт
  ложный `schema_timeout` в `internal/runtime`; гонять последовательно.
- `aif-classic/workflow.yaml` (внешний repo) ссылается на
  `.prifly/workflows/aif-classic/…`; staged copy валидируется под финальным
  именем папки.
- macOS `rename` не заменяет пустой каталог; swap в `update` резервирует имя.
- `git ls-remote URL tag` возвращает tag object; peel через паттерн `tag^{}`.
- Тот же `id@version` с другими bytes = `project_start_package_identity_conflict`.
- Таблица glossary bindings принимает только `internal/` источники.
- `gh` account StenHigh (scope `workflow` есть); SSH-ключ — `dzianis-87`,
  push StenHigh-репозиториев по HTTPS с `!gh auth git-credential`.
