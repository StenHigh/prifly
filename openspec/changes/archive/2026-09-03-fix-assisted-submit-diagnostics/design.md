## Context

`ProblemFor` (`internal/runtime/problem.go`) переводит ошибку в JSON
`Problem`. Типизированные ошибки (`*flow.Problem`, `*local.Rejection`,
sentinel errors) распознаются явно; всё остальное попадает в default-ветку,
которая режет `err.Error()` по первому двоеточию и берёт код только при
наличии двоеточия и валидной форме. Runtime поднимает 29 refusals как
`errors.New("code")` без двоеточия и десятки свободных текстов; путь
`session submit` (`sessions.go`, `driver.go:resultOutputs`,
`workspace_trees.go`) использует обе формы. Остаток после двоеточия сейчас
отбрасывается всегда. `selectedTreeLocation` отвергает любой присланный путь
для binding с `InputManifest`, а `SubmitSession` вызывает захват только при
`assisted-session/4` или непустом `workspace_trees`, поэтому `/5` без
`workspace_trees` пропускает захват. Проверки `resultOutputs` (обязательные
порты, identity slot, digest, schema выхода) выполняются только при
acceptance и вместе с sealing: неверный candidate становится `failed`
Attempt, а при `retry_class: never` — terminal Run. Мотивация — в
proposal.md.

## Goals / Non-Goals

**Goals:**
- Любой stable code, поднятый runtime, доходит до клиента как `code`.
- Предмет отказа (port, path, version) виден в `violations` без раскрытия
  raw input, argv, environment или nested foreign errors.
- Одна форма отправки `workspace_trees` для output-only и input+output
  bindings.

**Non-Goals:**
- Переименование сотен свободных текстов вне пути submit.
- Изменение shape `Problem`, `SessionSubmission`, `NextView` или exit-code
  map.
- Действие `waiting_host` и bump `CoreNextView` (следующий change).

## Decisions

1. **Default-ветка `ProblemFor` принимает голый код.** `strings.Cut` без
   двоеточия даёт весь текст как кандидат кода; валидный кандидат становится
   `code`. Альтернатива — переписать 29 `errors.New("code")` в форму
   `code: message` — лишний churn и та же ловушка для следующего автора.
2. **Остаток после двоеточия сохраняется только для валидного кода.** Он
   уходит в `violations[0].reason` (pointer пустой), обрезка 2048 байт уже
   есть. Свободный текст без кода по-прежнему даёт `invalid_input` с пустыми
   `violations`. Остаток сохраняется только когда у ошибки нет wrapped cause
   (`errors.Unwrap(err) == nil`): три colon-form ошибки runtime оборачивают
   чужую ошибку через `%w` (`driver_already_active`, `unsafe_archive`,
   `invalid_invocation`), и их detail может содержать путь или текст
   драйвера; для них остаётся code без detail. Граница приватности
   `ProblemFor` не меняется.
3. **Отказы пути submit становятся `*flow.Problem` с pointer.**
   `resultOutputs` и shape-проверки submission поднимают
   `&flow.Problem{Code, Path, Message}`: `ProblemFor` уже переводит его в
   `code` + `violations[{pointer, reason}]` с exit 2 (ошибка входа host).
   `local.Reject` (exit 3) остаётся для identity-конфликтов
   (`result_identity_mismatch`). Коды: `output_required_missing`
   (`/result/outputs/<port>`), `output_port_undeclared`,
   `output_identity_mismatch`, `output_digest_mismatch`,
   `output_seal_mismatch`, `submission_shape_invalid`,
   `submission_trees_unsupported`, `submission_cost_unsupported`.
4. **`selectedTreeLocation` принимает путь, равный `InputLocation`.** Для
   binding с `InputManifest` присланный путь сравнивается с declared
   location; равенство — принять, расхождение —
   `workspace_tree_input_location_mismatch`. Старый код
   `..._forbidden` никогда не доходил до клиента (схлопывался), поэтому его
   замена не ломает совместимость. Capture, provenance и confinement не
   меняются.
5. **Захват keyed на bindings шага, а не на версию.** `SubmitSession`
   вызывает `captureWorkspaceTreeOutputs`, когда step объявляет
   `workspace_trees` или submission их прислала; версии без поддержки деревьев
   по-прежнему отклоняются раньше. Альтернатива — требовать от host всегда
   присылать `workspace_trees` — противоречит правилу «runtime capture-ит
   сам».
6. **Location называется только там, где host её выбирает.** Для
   `exact_file` единственное значение, проходящее `selectedTreeLocation`, —
   `policy.Path`, поэтому его отсутствие трактуется как этот путь, а не как
   `workspace_tree_location_missing`; для `direct_child_file` и
   `direct_child_tree` имя выбирает host, и location остаётся обязательной.
   Contract не меняется: `workspace_trees` уже nullable, а
   `WorkspaceTreeLocation.required` остаётся прежним. Альтернатива — сделать
   `path` optional в опубликованной схеме — меняет closed shape ради того же
   результата.
7. **Чистая предпроверка intake, sealing остаётся в acceptance.** Проверки
   `resultOutputs`, не пишущие в registry (обязательные для verdict порты,
   объявленность, identity/revision admitted slot, digest файла slot, JSON
   schema выхода, result schema шага через `p.ValidateJSON`), выделяются в
   функцию без side effects, которую `SubmitSession` вызывает до записи
   `attempt.result_candidate`, а acceptance — перед sealing. Отказ intake
   поднимается `*flow.Problem`; handoff остаётся `awaiting`. Managed путь
   (`driver.go:1072`) не меняется: там кандидат приходит от процесса, а не от
   host, и повторная отправка невозможна.
8. **Проверка через тесты, не через документацию.** Unit-тесты `ProblemFor`
   (голый код, код с остатком, свободный текст, `%w`-обёртка), тест
   `session submit` для input+output binding с повторённым и другим путём,
   тест `resultOutputs` на имя порта, тест `/5` без `workspace_trees`
   (захват выполнен), тест отклонённой отправки (handoff awaiting, второй
   отчёт под той же envelope принят), CLI-тест JSON `Problem`.

## Risks / Trade-offs

- [Бывшие `invalid_input` станут другими кодами и exit 3 для голых кодов,
  содержащих `conflict`/`drift`] → это существующее правило exit-map; host
  runner не ветвится по `invalid_input` для этих случаев, так как они были
  неразличимы.
- [Engine-authored detail может содержать имя порта или путь из sealed plan]
  → это данные, уже выданные host в SessionTask; secrets и foreign payload
  туда не попадают.
- [Предпроверка читает файлы slot дважды: при intake и при acceptance] →
  чтение ограничено `MaxOutputBytes`, а sealing по-прежнему один; acceptance
  сохраняет полный набор проверок на случай изменения фактов между вызовами.
- [Повторённый путь для input binding ослабляет строгость] → принимается
  только точное равенство declared location; другой путь отклоняется как и
  раньше.

## Migration Plan

Изменение только в поведении отказов и admission одной формы отправки;
persisted state, schemas и bundles не меняются. Откат — возврат бинаря.
