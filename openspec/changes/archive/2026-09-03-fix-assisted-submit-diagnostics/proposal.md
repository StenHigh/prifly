## Why

Живой pilot `aif-classic` (2026-09-03, движок `27fa58c`, host `claude-code`,
режим `checkout`) встал на шаге improve: отправка хоста была отвергнута как
`invalid_input` с пустыми `violations`, и ни один из 14 безымянных отказов
прогона не назвал предмет. Причина двойная и целиком в движке: `ProblemFor`
схлопывает stable refusal codes, поднятые без `code: message` формы, и весь
свободный текст пути submit в `invalid_input`; а binding дерева вход+выход
отвергает ту же форму отправки, которую тот же host уже использовал на
предыдущем шаге, не называя причину, а без `workspace_trees` при
`assisted-session/5` захват дерева не вызывается вовсе, так что рабочей формы
у такого шага нет. Вторая проба pilot была принята, шаг записан `failed`, и
Run стал terminal: три пройденных шага сгорели из-за одной неверной формы. Исполнитель вынужден диагностировать
отказ чтением схем из бинаря и SQLite, что противоречит требованию safe
next action.

## What Changes

- `ProblemFor` распознаёт stable code, поднятый без двоеточия, и для
  распознанного кода сохраняет engine-authored остаток после двоеточия как
  `violations[0].reason`; свободный текст без кода по-прежнему даёт
  `invalid_input` и не утекает в ответ.
- Отказы пути `session submit` (обязательный выход не отчитан, необъявленный
  порт, identity/digest выхода, форма submission, версия submission без
  деревьев) становятся именованными refusals с предметом (порт, версия) в
  message.
- Захват declared деревьев выполняется при каждом assisted submission шага
  с bindings независимо от версии `assisted-session/4` или `/5` и наличия
  `workspace_trees`.
- Assisted intake проверяет до записи candidate то, что сегодня проверяется
  только при acceptance: result schema шага, обязательные для verdict порты,
  объявленность портов, identity admitted slot и digest; неверная отправка
  отклоняется named refusal, handoff остаётся awaiting, Attempt не становится
  `failed`.
- Для binding дерева, объявленного и входом, и выходом, host MAY повторить
  declared `input_location` в `workspace_trees`; расхождение отклоняется
  именованно как `workspace_tree_input_location_mismatch`. Форма отправки
  становится одинаковой для output-only и input+output шагов.
- Для capture policy `exact_file` host MAY не называть location: у неё ровно
  одно допустимое значение, declared `capture.path`, и runtime берёт его сам.
  Location остаётся обязательной только там, где host действительно выбирает
  имя, то есть для `direct_child_file` и `direct_child_tree`.
- Вне scope: действие `waiting_host` для ожидающего host Attempt требует
  versioned bump `CoreNextView` (опубликованный enum уже не содержит
  `waiting_decision`) и уходит в следующий change вместе с discoverability
  схем и справки; источник контекста-каталог — отдельный authoring change.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `cli-protocol`: «Problem и exit code сохраняют safe meaning» — stable code
  runtime refusal MUST доходить до клиента, а engine-authored предмет отказа
  MUST быть в `violations`; «Assisted handoff сообщает versioned declared
  Workspace tree bindings» — host MAY повторить declared input location,
  расхождение отклоняется именованно; «Result intake проверяет exact Attempt
  и sealed output» — отказ по обязательному выходу называет порт.

## Impact

Изменение затрагивает product runtime: JSON Problem, который CLI печатает при
отказе, и admission `session submit` для input+output tree bindings. Versioned
contracts не меняются: shape `Problem` (pattern кода, `violations`) и
`SessionSubmissionV4/V5` остаются прежними; меняются только значения кодов и
содержимое `violations`. Код: `internal/runtime/problem.go`,
`internal/runtime/driver.go` (`resultOutputs`), `internal/runtime/sessions.go`
(shape submission), `internal/runtime/workspace_trees.go`
(`selectedTreeLocation`), `cmd/prifly/project.go` (runner templates) и их
тесты. Normative source ownership ни одной capability не меняется.
