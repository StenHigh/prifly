## 1. Regression и compatibility contracts

- [x] 1.1 Добавить synthetic fixture с несколькими shared schemas, двумя последовательными шагами и старым Run. Проверка до fix: clean/remove успешны, revoke даёт `package_not_resolvable` и 0 новых Runs; положительный migration test воспроизводит отсутствие пути.
- [x] 1.2 Добавить closed migration DTO, `authority-packages/2` и следующую свободную opt-in Run/Project boundary. Проверить реальные serialized fixtures новыми schemas; legacy `/1` и Run без binding сохраняют прежние shape/version.

## 2. Planner, lifecycle generations и atomic apply

- [x] 2.1 Реализовать deterministic read-only planner полного consumer closure с явными schema suppliers. Проверить все refs, всех revoked источников, независимые trust/status reasons, supplier bytes и одинаковый digest для эквивалентного порядка ввода.
- [x] 2.2 Добавить bounded scan влияющих текущих и сохранённых Runs, диагностику кандидатов и `applicable=false` при неполном выборе. Проверить отсутствие mutation, Run, claim и исполняемых hooks; превышение scan limit не выдаёт применимый план.
- [x] 2.3 Реализовать переход `/1` → `/2` при первом apply и generations во всех package handlers. Проверить неизменность прежних status/trust facts, увеличение при новом повторном revoke, стабильность exact trusted reimport и сохранение decisions после последующих imports/status changes.
- [x] 2.4 Реализовать owner apply и сохранённый decision одним `ApplyAuthority` с package/control/cut/Run guards. Проверить owner denial, digest mismatch, pure transform и отсутствие частичного разрешения; `internal/purity` и refusal guard должны проходить.

## 3. Start и повторные admissions

- [x] 3.1 Добавить явный выбор decision в runtime Start и `SchemaMigrationBinding` нового Run. Проверить exact consumer/supplier/schemas, scope mismatch, запись достижимой state boundary и отказ без активного нового claim или Run.
- [x] 3.2 Подключить binding к общему admission для следующего шага и вложенной работы. Проверить минимум два admissions после Start, engine restart и неизменность закреплённого supplier.
- [x] 3.3 Сохранить запреты для старых Runs, trusted reuse, executable components и refs вне scope. Проверить old Run с тем же schema Ref, Start без выбора и попытку переноса binding в связанный Run без нового выбора/проверки.
- [x] 3.4 Проверять актуальные generations и полный набор revoked совпадений при каждом admission. Проверить повторный revoke с новым command id, двух revoked поставщиков и появление нового отозванного источника после apply.

## 4. Public CLI и поведенческая матрица

- [x] 4.1 Добавить public prepare/inspect и versioned JSON/human вывод. Native CLI test показывает несколько schema refs, все legacy package refs/reasons, suppliers и последствия для Runs без ручного чтения SQLite.
- [x] 4.2 Добавить public apply, `run start --schema-migration` и Project questionnaire/start integration. Проверить owner reason/receipt, явный ID в review digest, Start mismatch и сохранённый decision после restart.
- [x] 4.3 Выполнить отрицательную scope матрицу: другой consumer/digest, неполный plan, executable ref, новый supplier, непокрытый revoke и старый Run. Каждый случай должен сохранять trust/history и возвращать ожидаемый код отказа.
- [x] 4.4 Проверить stale plan после package state, control policy, closure, supplier bytes и изменения версии влияющего Run без изменения количества Runs. Каждый отказ должен оставлять весь scope неприменённым.
- [x] 4.5 Добавить конкурентные native tests apply/revoke, apply/Run change и Start/revoke. Проверить единый результат commit, отсутствие частичных разрешений, повтор принятого command id после потери ответа и conflict для изменённого payload.
- [x] 4.6 Выполнить compatibility matrix clean/remove/quarantine/restore/dependency trust, old Run history, claims и audit. Проверить тот же public CLI без migration flags на authority `/1` и `/2`.

## 5. Словарь и поставляемая документация

- [x] 5.1 Обновить `terms.md`, PackageStatus, Go/JSON bindings и agent brief вместе с implemented types. Выполнить `TestGlossaryBindings`; убедиться, что прежние термины и saved JSON не переименованы.
- [x] 5.2 Обновить public help, `examples/operations/index.md`, `commands.md` и upgrade guidance. Проверить полный маршрут compile/import → prepare → apply → explicit Start и объяснение remove/revoke с holders/dependents.
- [x] 5.3 Добавить capability с минимальной Core/contract boundary, строку `examples/README.md` и реальное поле binding в подходящий authoring/read reference. Выполнить `TestEveryDeclaredCapabilityIsInTheAuthorIndex`; не добавлять несуществующее YAML-поле миграции.
- [x] 5.4 Обновить embedded operator/runner assets и `openspec/SOURCE-OF-TRUTH.md` ссылкой на active delta. Проверить delivered handbook и одинаковые команды; сохранить пользовательские `PROJECT.md` и ownership main source sets.

## 6. Итоговая проверка и release acceptance

- [x] 6.1 Выполнить focused native tests, `make test-changed`, `make schemas-check`, `make refusal-check`, `make vet`, `openspec validate migrate-revoked-shared-schemas --strict --no-interactive` и `git diff --check`. Для итогового implementation snapshot выполнить один `make ci-check-once CHECK_CONTEXT=issue-21-schema-migration`. Зафиксировать результаты и проверить отсутствие правок `openspec/changes/archive/**`, прежних schema bundles и historical release evidence.
- [ ] 6.2 После release gates на фиксированном SHA выполнить generic public-CLI acceptance released binary. Сохранить version/digest, исходный отказ, несколько schemas, apply, новый Run с последующей работой, old Run refusal, restart и cleanup всех временных authority/monitor registrations. Source tests не закрывают эту задачу.
- [x] 6.3 Подготовить owner handoff для private authority acceptance со списком точных команд и проверок. Отдельно указать её фактический статус; без отдельного участия владельца не выполнять её и не заявлять ремонт живой установки. Проверка завершения — документированный handoff и граница generic/private результатов.
