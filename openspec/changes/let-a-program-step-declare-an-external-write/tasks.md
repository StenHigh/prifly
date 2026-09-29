## 1. Согласование с предыдущим change

- [x] 1.1 Заархивировать `let-an-assisted-step-declare-an-external-write` как
      выпущенное решение 0.13.64 (без переписывания), затем перевести его
      требования, которые этот change меняет, в MODIFIED. Сделано 2026-09-29:
      архив `2026-09-29-let-an-assisted-step-declare-an-external-write`, его
      5.2 перенесена сюда в 6.3; `openspec validate --strict` чисто.

## 2. Контракт и компиляция

- [x] 2.1 Схема StepDefinition v14: мутаторы до 5, затем v12, затем поле
      `external_write` из той же схемы границы, что у v11 (переиспользование,
      не копия). Регистрация в `schema.go` и `schemas/`. Проверка:
      `make schemas-check`; закреплённые бандлы и контракты 1–13 байт в байт.
- [x] 2.2 Lowering `prifly-step/1`: принимает `external_write`, выбирает v14;
      фиксация ниже 14 при границе отказывает с названием 14; шаг без поля
      понижается в прежние байты. Отказ программе в `prifly-step/2` называет
      `prifly-step/1`. Проверка: тесты authoring на каждый случай, включая
      сравнение байт шага без поля с предыдущей сборкой.
- [x] 2.3 `compile.go`: для `external_write` отказывать только `pure`, с
      текстом из design (решение 2). `idempotent` проходит, а
      `technical_retries` с ним допускаются. Проверка: тест, который отказывает на 0.13.64
      (idempotent) и проходит сейчас; `pure` отказывает с новым текстом.
- [x] 2.4 `start.go:339`: программе допустим `external_write`; текст отказа
      для `destructive` без упоминания «external write belongs to an assisted
      step». Проверка: старт Run с программной границей под политикой 4.0.0
      проходит, под политикой 3.x отказывает, называя класс.

## 3. Исход прерванной программы

- [x] 3.1 `settleWith`: условие design (решение 3) переводит попытку в
      `uncertain` с диагностикой `external_write_unreconciled`, которая называет
      стадию, retry-класс и `run resolve`. Проверка: для каждого из
      `deduplicated`, `reconcile_required`, `never` (сбой после старта, отмена,
      отказ приёма) есть тест, который красный без условия (вырезать условие →
      тест падает).
- [x] 3.2 Путь потери драйвера (восстановление осиротевшей попытки) проходит
      через то же условие. Проверка: тест с убитым драйвером и доказанно пустой
      группой даёт `uncertain` для `reconcile_required` и `failed` для
      `idempotent`.
- [x] 3.3 Состояние: Run с программной границей запечатывается существующим
      состоянием внешней записи; сборка 0.13.64 на его плане отказывает, называя
      контракт 14 новее себя. Проверка: тест достижимости состояния (как для
      34/35/39) и тест текста «новее».
- [x] 3.4 Неизменность: `idempotent` с доказанно пустой группой → `failed`,
      `project continue` исполняет шаг снова без `run resolve`; незапущенная
      программа и принятый вердикт не дают `uncertain`; ассистируемый шаг с
      `idempotent` при потере сессии остаётся `uncertain`. Проверка: по тесту на
      каждый случай.
- [x] 3.5 Сквозной путь: `uncertain` → `project continue` отказывает
      `recover_source_unsettled` → `run resolve --outcome not_applied` →
      `project continue` исполняет шаг снова. Проверка: e2e на собранном CLI.

## 4. Протокол и справка

- [x] 4.1 `compatibility.go`: возможность `program_external_write`, контракт
      шага 14 в документе возможностей. Проверка:
      `TestEveryDeclaredCapabilityIsInTheAuthorIndex` красный без строки README
      и зелёный с ней.
- [x] 4.2 `run next` / `run explain` Run в `uncertain` из-за
      `external_write_unreconciled` называют стадию, retry-класс и `run.resolve`
      с `applied`/`not_applied`. Проверка: тест чтения.
- [x] 4.3 `prifly help`: строка о retry-классе внешней записи (повтор без
      свидетельства только при `idempotent`, заявление автора не проверяется);
      строку «Not yet supported … automatic retry/reconcile» сверить, чтобы она
      не противоречила. Проверка: тест текста справки.

## 5. Документация автора

- [x] 5.1 `examples/authoring/step-authoring-reference.yaml`: блок
      `external_write` для обеих линий (`prifly-step/1` → v14, `prifly-step/2` →
      v11); таблица retry-классов «класс → что при прерывании → когда
      выбирать», с развилкой ensure/create; отдельная фраза: ассистируемая
      потерянная сессия `uncertain` при любом классе; граница не песочница и не
      доказательство. Переписать абзац «publishing from a worker declared
      `class: none` is a way around the contract». Без продуктовых имён.
      Проверка: пример компилируется существующим тестом справочников.
- [x] 5.2 `examples/README.md`: строка `program_external_write` (контракт 14,
      `prifly-step/1`, retry-класс решает повтор), строка
      `declared_external_write` без «только ассистируемый шаг». Проверка: 4.1.
- [x] 5.3 `examples/authoring/continuation-guide.md`: раздел «прерванная
      внешняя запись» — что `project continue` делает при каждом классе и как
      выйти через `run resolve`. `blocked-guide.md:101` — упомянуть, что
      объявить может и программа. Проверка: ревью; все упомянутые команды и коды
      есть в сборке (grep по `cmd/prifly` и кодам отказов).
- [x] 5.4 `examples/troubleshooting.md`: запись «Run `uncertain` после
      прерванной программы» с причиной `external_write_unreconciled`, шагами
      `run resolve` и указанием, когда автору стоит сменить класс на
      `idempotent`. Проверка: ревью.

## 6. Ворота и выпуск

- [x] 6.1 `make check` и `make ci-check` зелёные по абсолютному пути
      репозитория; записать счётчики (fmt-check, refusal-check) в этот файл в
      день прогона.
- [ ] 6.2 После push: `gh run list` зелёный на обеих платформах.
- [ ] 6.3 Пилоту: сообщить выпуск; проверить на настоящем Run, что
      `tests.yaml` с `external_write`/`idempotent` компилируется, журнал несёт
      границу, а прерванный шаг возобновляется `project continue` без
      `run resolve`. Приёмка — за пилотом.

## Итог реализации (2026-09-29)

Отклонения от плана:
- 2.4/7: контракт 14 закрепляет `executor.operation: process` в схеме, а не
  отдельной проверкой в `compile.go`.
- 3.3: попутно найден и исправлен в корне дефект: плоский Run с программной
  внешней записью не запускал программу (ключ исполнителя). Исправление —
  `requiresInvocationState` включает `requiresExternalWriteState`.
- 3.5: сквозной путь проверен на уровне runtime
  (`TestAnInterruptedProgramWriteWaitsForTheOwnerUnlessIdempotent`: uncertain →
  `recoveryPointOf` отказывает `recover_source_unsettled` с `run resolve` →
  `ResolveObligation` → `recoveryFrontier` находит шаг) и воспроизведением
  пилота на собранном CLI (`TestProjectCompileSealsAProgramStepsExternalWrite`:
  `project compile --package` проектного шага `tests`). Отдельного e2e
  `project continue` на CLI нет. Приёмка настоящим Run — за пилотом (6.3).
- 4.2: причина видна в `run status` (`failure` теперь есть и у `uncertain`
  Run). `reason_code` в `run next` — опубликованный enum, и его не трогали.
  Отказ возобновления `uncertain` Run теперь `recover_source_unsettled` с
  `run resolve`, а не `recover_source_ineligible`.
- Сверх плана: неизвестный (более новый) контракт шага назван «newer than this
  build» (`unsupported_contract`); отказ старта при политике без
  `external_write` называет 4.0.0; устаревшая запись troubleshooting
  «опубликовать ветку — нельзя» переписана.

Проверки, что тесты умеют падать: вырезанное условие в `settleWith` →
`TestAnInterruptedProgramWriteWaitsForTheOwnerUnlessIdempotent` и
`TestALostDriverLeavesAnUnreconciledProgramWriteForTheOwner` красные;
удалённая строка README → `TestEveryDeclaredCapabilityIsInTheAuthorIndex`
красный с названием `program_external_write`.

Ворота 2026-09-29, `make check` (содержит ci-check и race) — exit 0: `go test`
cmd/prifly 66 с, flow 7 с, runtime 282 с; race: cmd/prifly 347 с, flow 35 с,
runtime 833 с; fmt-check 336 файлов; refusal-check 174 файла; staticcheck 9
пакетов linux + darwin без находок; vuln-check 9 пакетов; schemas-check и
release-ci-check совпадают. `make e2e` 2026-09-29 — exit 0.
