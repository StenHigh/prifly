## Context

Мотивация и scope описаны в [proposal.md](proposal.md).
Текущий `project_start.go` читает только явный `--schema-migration`.
Он проверяет записанное решение и exact consumer package до формирования review digest.
`internal/runtime/schema_migration.go` проверяет trust и sealed bytes, затем создаёт SchemaMigrationBinding для нового Run.

Текущие migration boundaries: `authority-packages/2`, `core-state/45`, `core-read/45` и Project summary `/9`.
Новая настройка меняет источник выбора. Семантика существующего Binding остаётся прежней.

## Goals / Non-Goals

**Goals:**

- Хранить owner выбор локально в authority и переживать перезапуск процесса.
- Проверять ту же точную сборку общим путём compile/prepare/start.
- Отказывать при изменении выбора между review и принятием Run.

**Non-Goals:**

- Изменение upstream YAML или package profile ради локального решения владельца.
- Новый механизм trust, другая семантика revoke либо новое Run state поле.
- Автоматическая настройка private authority при разработке Core.

## Decisions

### 1. Отдельный authority объект выбора

Канонический термин — LaunchSchemaMigrationChoice: сохранённый выбор решения владельцем для project launch.
Не использовать SchemaMigrationSelection: этот термин уже обозначает пару schema и supplier.

Хранить новый объект отдельно от package registry, с собственной closed schema `/1`.
Ключ включает authority ID, существующий Project ID, канонический repository root и launch ID.
Использовать действующее разрешение project root, включая нормализацию symlink, а не введённый пользователем путь.
Это не передаёт выбор другому проекту при явном использовании той же authority.

Запись содержит generation, decision ID, decision review digest, build key, consumer Ref, workflow Ref и provenance владельца.
Provenance содержит actor, reason и command identity.
Build key включает effective compile values, composition, host-selected resources и package profile по существующему алгоритму.
Не включать task bytes и прочие per-Run inputs в область настройки.
Выбор разрешает повторные Runs с другими inputs в пределах той же сборки.

После clear сохранять generation и отсутствие effective выбора.
Это позволяет обнаружить последовательность clear/set с прежним decision ID.
Первое отсутствие объекта имеет generation 0. Read-only команды не материализуют объект.

Отдельный объект не требует изменения `authority-packages/2`.
Альтернатива с полем в `project.yaml` переносит локальный выбор через Git и требует изменения profile contract.
Альтернатива с последним Decision не сохраняет точное намерение владельца.

### 2. Public команды inspect, set и clear

Все команды используют глобальный `--project AUTHORITY`, `--repository` и `--launch`.
Inspect и set принимают `--host` и `--package-profile` для той же сборки, что prepare/start.
Inspect показывает сохранённую запись, текущую generation, состояние сборки и причины недействительности.
При повреждённых исходниках inspect сохраняет доступ к записанным данным и сообщает ошибку текущей сборки отдельно.
Дополнительный `--id ID` в inspect проверяет предложенное решение без записи.

```text
project schema-migration inspect --repository ROOT --launch LAUNCH --host HOST [--id ID]
project schema-migration set --repository ROOT --launch LAUNCH --host HOST --id ID
  --expected-build-key KEY --expected-generation N --reason TEXT [--command-id ID]
project schema-migration clear --repository ROOT --launch LAUNCH
  --expected-generation N --reason TEXT [--command-id ID]
```

Set создаёт либо заменяет один выбор для reviewed Project profile `/3` или новее.
Legacy profile получает существующий profile refusal. Clear не требует поддерживаемого launch profile. Он не импортирует package и не применяет migration plan.
При отсутствии установленного exact consumer команда сообщает необходимость завершить существующий migration workflow.
Set проверяет, что decision действителен для текущих build key, consumer и workflow.
Clear не компилирует launch и работает после исчезновения launch из profile.
Он проверяет scope проекта, owner, generation и наличие записи.
Clear отсутствующей записи с правильной generation возвращает успешный no-op receipt.

Set и clear используют существующие authority commands, owner checks и command deduplication.
Одинаковые command identity и payload возвращают прежний receipt.
Иная payload той же команды возвращает command conflict.
Тяжёлое чтение sealed bytes и compile выполняются перед write-транзакцией.
Внутри transform проверяются подготовленные preconditions, control state и generation записи.

Альтернатива с прямой записью CLI в SQLite обходит authority и не используется.

### 3. Единый выбор перед launch review

Разрешать источник после получения compiled package и workflow Ref, до формирования summary.
Использовать один resolver для questionnaire prepare и project start.
Continuation и recovery проходят тот же resolver для выбранного нового launch.
Они сохраняют все существующие запреты на source Run и не меняют его Binding.

```text
compile launch -> explicit ID? -> проверить явный ID --------+
                       |                                   |
                       no                                  v
                       |                             summary + digest
                       v                                   |
                 read saved choice                         v
                       |                           reviewed project start
              +--------+---------+                         |
              |                  |                         v
          отсутствует         присутствует          атомарный admission
              |                  |                         |
       прежний admission   exact build + trust              v
                                 |                 новый Run + Binding
                           ошибка -> refusal
```

Приоритет: непустой явный flag, затем сохранённая запись, затем прежнее поведение без миграции.
Пустое значение flag означает отсутствие разового выбора, как в текущем разборе CLI.
Оно не отключает сохранённый выбор.
Недействительный явный ID не вызывает fallback.
Разовый flag не обновляет запись и не зависит от generation неиспользуемого saved выбора.
Standalone `run start` сохраняет существующий явный интерфейс.

Для saved запись должна совпасть по каждому exact Ref и build key.
При несовпадении вернуть `schema_migration_scope_mismatch` с сохранёнными и текущими идентичностями.
Отсутствующее или недействительное Decision использует `schema_migration_decision_invalid`.
Сохранить существующие integrity и access-denied коды.
Не удалять недействительную запись автоматически: owner должен видеть причину.

### 4. Review и защита от гонок

Новый summary показывает effective ID, source `explicit` либо `saved` и exact scope.
Для saved он также показывает generation записи.
Включить эти данные в review digest.
Не включать общий authority cut: посторонний Run не должен инвалидировать выбор одной сборки.

Saved start требует `--expected-launch-digest`.
Это использует обычный reviewed путь Project CLI и исключает отдельный скрытый режим запуска.
Явный flag сохраняет прежние требования к digest.
Если summary использует другой ID, source, generation или сборку, вернуть `project_start_stale_launch` до регистрации и постоянного claim.

Повторное чтение CLI перед Run недостаточно: владелец может изменить запись после этого чтения.
Передать runtime проверенные scope и generation как precondition принятия Run.
Проверить их в той же authority транзакции, которая принимает новый Run.
Там же сохранить текущие migration lifecycle проверки.
При конфликте очистить временные preparation artifacts по существующему пути отказа; постоянный claim не остаётся.

Принятая команда start при повторе возвращает прежний результат по существующему deduplication контракту.
Изменение выбора после принятия Run не перенаправляет retry на новый decision.
Непринятая stale команда не получает права на повторный запуск со старым review.

### 5. Совместимость и публикация

Публиковать новые closed schemas объекта, inspection и command result рядом с Go types.
Дополнительные поля summary/start требуют следующих свободных DTO revisions; существующий migration summary использует `/9`.
Во время реализации подтвердить свободные номера по актуальному реестру схем.
Ответ без effective migration сохраняет прежнюю подходящую boundary.

Run продолжает использовать SchemaMigrationBinding и migration state/read `/45`.
Источник выбора и generation остаются в launch review и receipt, не расширяя существующий Run contract.
Reader без поддержки нового объекта не получает из него разрешение.
Без явного ID прежний binary сохраняет revocation refusal.
Не обещать совместимость нового ответа со старым closed DTO reader.

Обновить glossary, agent brief, public help, operations manual и authoring schema migration guide в реализации.
Если добавляется advertised capability, одновременно обновить `examples/README.md`, operations capability index и точный authoring reference.
Ownership source sets остаётся прежним; `SOURCE-OF-TRUTH.md` получит ссылку на активную delta без нового владельца capability.

## Risks / Trade-offs

- Смена host или compile value меняет сборку → inspect показывает различие; новый выбор требует owner set.
- Один launch имеет несколько вариантов сборки → хранится один текущий выбор; возврат к варианту требует явной замены.
- Выбор меняется во время start → runtime проверяет generation атомарно при принятии Run.
- Owner отзывает supplier после set → обычные migration checks блокируют prepare, start и последующие admissions.
- Повреждённые project files мешают inspection → recorded view и clear остаются доступны без успешной компиляции.
- Доверенный package остаётся revoked → regression проверяет status, generation, старые Runs и executable refs отдельно.

## Migration Plan

1. После review proposal зафиксировать planning artifacts по правилам репозитория.
2. Выполнить реализацию через `prifly-run` и launch `aif-classic`, ссылаясь на задачи этого change.
3. Добавить closed schemas и reader отсутствующего объекта; не менять существующие authorities при чтении.
4. Проверить новый CLI путь во временной authority и обновить поставляемые справочники.
5. Владелец отдельно проверяет и сохраняет выбор в рабочей authority после выпуска поддерживающего Core.

Для rollback owner выполняет clear перед использованием прежнего binary.
Старые Runs сохраняют прежние migration contracts. Старые release evidence не переписываются.
Source checks доказывают реализацию; released-binary acceptance и private acceptance остаются отдельными результатами.
