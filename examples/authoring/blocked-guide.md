# Как обрабатывать `blocked`

Руководство для автора workflow и для ИИ-агента, который пишет или ведёт его.
Поля показаны в [`workflow-authoring-reference.yaml`](workflow-authoring-reference.yaml)
и [`step-authoring-reference.yaml`](step-authoring-reference.yaml), продолжение —
в [`continuation-guide.md`](continuation-guide.md). Рабочий пример, который
можно запустить без ИИ и Git:
[`workflows/blocked-condition/`](../workflows/blocked-condition/README.md).

## Что такое `blocked` и чем он не является

`blocked` — вердикт шага «я не смог сделать работу, потому что нет условия для
неё»: сервис не ответил, документ не пришёл, нет разрешения. Это **не**
суждение о работе.

| Что произошло | Как это выглядит | Что делает движок |
|---|---|---|
| работа сделана | `pass` | маршрут `on.pass` |
| работа сделана плохо | `fail`, `needs_revision` | маршрут автора; повтора нет |
| работу нельзя было сделать | `blocked` | маршрут `on.blocked` автора; повтора нет |
| шаг не смог отчитаться (процесс упал, результат не прошёл контракт) | технический отказ | `technical_retries` по классу повтора шага, затем `on_error` |
| владелец остановил Run | пауза, стоп, отмена | ничего не допускается до снятия; отмена перекрывает любой маршрут |

Движок не выводит `blocked` сам, не превращает его в `fail` и не считает
технической ошибкой. Что значит препятствие и как его устранять — решает автор.

## 1. Передать причину: обещанный выход

Шаг, который может вернуть `blocked`, обещает выход с причиной на этом
вердикте. Схема причины — ваша; например, как в примере:

```yaml
outputs:
  obstacle:
    schema_ref: obstacle_schema      # reason_code, summary, condition_ref, observations, remaining_work
    required_for: [blocked]
result_schema_ref: result            # core:schema/step-result@2.0.0 — контракт, где есть blocked
```

- Ассистируемый шаг (`authoring: prifly-step/2`) спускается к контракту шага
  10, программный (`prifly-step/1`) — к 12. Писать номер не нужно: лестница
  авторинга выводит его из обещания.
- Шаг, вернувший `blocked` без обещанного выхода, отказывается при приёме:
  `invalid_output` с `output_required_missing` в тексте. Молча потерять
  причину нельзя.
- Стадия, к которой ведёт `on.blocked`, может связать обязательный вход только
  с выходом, обещанным на `blocked`; иначе компиляция откажет
  `unavailable_output`.
- Движок проверяет форму выхода и ссылки, а не правдивость сказанного.

## 2. Выбрать путь после `blocked`

На ревизии 6 и выше каждый шаговый узел обязан сказать, куда ведёт `blocked`
(или объявить его невозможным в `impossible_verdicts`). **Назад по графу
нельзя**: граф ацикличен, ребро назад — отказ `cycle`. Три формы:

**Устранить и попробовать снова, ограниченно.** Проверяемый шаг и шаг
устранения — в теле `repeat`:

```yaml
# тело: check → blocked → remedy → finish partial (с obstacle)
stages:
  attempt:
    kind: repeat
    body_workflow_ref: attempt_body
    initial_bindings: {request: $inputs.request}
    next_bindings: {request: $inputs.request}
    continue_on: [partial]               # тело кончилось partial — ещё раз
    until: {op: exists, ref: $iteration.result}
    max_iterations: 3                    # предел повторов
    on_complete: {succeeded: done, rejected: rejected}
    on_limit: stopped                    # куда, если условие так и не выполнилось
```

- `max_iterations` — ваш лимит; проект может сузить его (`limit_configuration`),
  но не поднять. Исчерпание ведёт по `on_limit`, а не в общий бюджет Run.
- `technical_retries` сюда не относится: он повторяет технический сбой и
  никогда не повторяет принятый вердикт.

**Подождать условия.** `on.blocked` → стадия `wait` с `timeout_seconds` и
`on_timeout`; пришедшее событие ведёт дальше, истечение срока — туда, куда
сказал автор. Фонового опроса нет: срок замечается, когда authority снова
смотрит на Run.

**Остановиться и продолжить позже.** `on.blocked` → finish с исходом
(например, `partial`), который выводит причину выходом Run. Дальше два пути,
оба командой `project continue`, см. [`continuation-guide.md`](continuation-guide.md):

- **тем же workflow** — он объявляет `resumable` (ревизия 8), и Run
  возобновляется тем же launch с той стадии, что привела к finish; принятое
  до неё не повторяется;
- **другим workflow** — тот объявляет `continuation` от этого workflow и
  исхода и сам берёт нужные входы.

## 3. Шаг устранения: свои входы, свои права

Причина приходит в шаг устранения обычным входом. Права шага — только его
собственные `effects`: отчёт `blocked` не даёт никаких прав. Шаг с
`effects.class: none`, который всё же изменил рабочее дерево, отказывается при
приёме (`effect_not_permitted`). Изменить что-то во внешней системе шаг может,
только объявив `external_write` с границей: системой, операциями и целью, —
ассистируемый шаг и программа одинаково. Выбранный им `retry_class` решает,
перезапустится ли прерванный шаг устранения сам (см.
[continuation-guide.md](continuation-guide.md#прерванная-внешняя-запись)).

Устранение не делает исходный шаг успешным: исходный шаг должен снова
вернуть `pass` по своему контракту — поэтому в `repeat` он повторяется, а
продолжение проверяет условие первым шагом.

## 4. Что видит новый исполнитель

Исполнитель с чистым контекстом читает всё из `run next` (версия ответа
`core-next/41`, возможность `next_handoff`):

| Поле | Что в нём |
|---|---|
| `arrived_from` | стадия, чей принятый результат привёл сюда; её вердикт, исход вызова или маршрут `repeat` (`on_limit`); ссылки на её выходы — после `blocked` там причина |
| `repeats` | в каких `repeat` идёт действие: итерация, действующий лимит, куда при исчерпании |
| `checkpoint` | последний принятый checkpoint Run, если workflow его объявил |
| `continuations` | у завершённого или отменённого Run — установленные workflow, объявившие его продолжение, и собственный workflow, если он объявил `resumable` для этого Run; тогда в `safe_next_actions` есть `project.continue` |
| `safe_next_actions` | что можно сделать сейчас |

Задание шагу (`session task`) несёт его входы — в том числе артефакт причины —
и `permitted_effects`. Байты артефакта: `artifact export`. Историю беседы
восстанавливать не нужно и нельзя.

`continuations` строится из установленных пакетов: пакет попадает в authority
при первом запуске его launch, поэтому до первого использования продолжающего
workflow список может быть пуст. Окончательная проверка — `project continue
--prepare`.

Run, созданный проектом, запечатывается не ниже состояния, которое отвечает
этой версией `run next`; Runs, созданные раньше, отвечают как прежде.

## 5. Неопределённый внешний эффект

Если шаг мог изменить что-то снаружи и неизвестно, изменил ли, движок не
повторяет его вслепую: Run останавливается с `recovery_required`, и владелец
говорит `run resolve`, применился эффект или нет. `blocked` этого не
отменяет и не заменяет.

## Чего движок не делает

- не знает, что значит ваше препятствие, и не придумывает словарь причин;
- не проверяет правдивость отчёта шага — только форму и ссылки;
- не будит Run сам и не опрашивает условие в фоне;
- не повторяет шаг после принятого вердикта сам — только в `repeat`, который
  объявил автор;
- не выдаёт устранение за успех исходного шага.

## Отказы

| Код | Что значит | Что делать |
|---|---|---|
| `missing_handler` | узел не сказал, куда ведёт `blocked` | `on.blocked` или `impossible_verdicts` |
| `cycle` | `on.blocked` ведёт назад по графу | `repeat`, `wait` или завершение с продолжением |
| `unavailable_output` | вход на ребре `blocked` связан с выходом, не обещанным на `blocked` | `required_for: [..., blocked]` у выхода |
| `invalid_output` / `output_required_missing` | шаг вернул `blocked` без обещанного выхода | вернуть выход или не обещать его |
| `effect_not_permitted` | шаг изменил то, на что у него нет права | объявить эффект или не менять |
| `recovery_required` | неопределённый внешний эффект | `run resolve` |
| `project_continue_undeclared`, `continuation_source_*` | см. [`continuation-guide.md`](continuation-guide.md) | |

## Приёмка заказчика: где что проверено

| Пункт | Проверка |
|---|---|
| 1. условие выполнено — обычный маршрут | `TestCLIBlockedExampleAcceptance` |
| 2. blocked с обязательным артефактом у обоих исполнителей; без него — отказ | `TestCLIBlockedExampleAcceptance`, `TestAProgramStepHandsOverWhatBlockedIt`, `TestAProgramStepThatPromisedAReportMustGiveIt`, `TestTheEnvelopeCarriesAnOutputPromisedForTheBlockedVerdict` |
| 3. шаг устранения со своими входами и правами; устранение ≠ успех | `TestCLIBlockedExampleAcceptance` |
| 4. продолжение без повтора принятых стадий | `TestCLIBlockedExampleAcceptance` (продолжение другим workflow и возобновление тем же), `TestCLIContinuationTakesOverTheSourceTree`, `TestAStoppedRunResumesWhereItStoppedOnlyWhenItsWorkflowSaysSo` |
| 5. лимит по `on_limit`, не общий бюджет | `TestCLIBlockedExampleAcceptance`, `TestRepeatProjectLimitIsPinnedAndOnlyNarrows` |
| 6. новый исполнитель по публичному интерфейсу | `TestCLIBlockedExampleAcceptance`, `TestNextHandsTheRunToAFreshExecutor` |
| 7. изменившееся условие не используется молча | `TestCLIBlockedExampleAcceptance` (продолжение проверяет условие заново) |
| 8. без прав — нет действия | `TestAReportThatChangedTheWorkspaceWithoutPermissionIsRefused`, `TestAProgramStepIsHandedTheWorkspaceAndHeldToItsEffects`, `TestADeclaredExternalWriteIsRefusedWithoutItsBounds` |
| 9. fail и blocked различимы | `TestCLIBlockedExampleAcceptance` |
| 10. технический сбой, blocked и пауза не смешиваются | `TestCoreKnownFailureUsesErrorTransition`, `TestChoiceCommitHonorsPauseAndCancel`, `TestAStepCanSayItCouldNotJudgeAndTheGraphRoutesIt` |
| 11. неопределённый эффект без слепого повтора | `TestDriverUncertainSettlementRetainsObligationsAndWorkspace`, `TestDriverPendingClockRecoveryAndDeadlineNeverBlindRetry` |
