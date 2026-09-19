## 1. Объявление в запечатанном плане

- [x] 1.1 `internal/flow`: `StepDefinition` получает необязательное
  `model_profile` — `requested` (имя профиля работы) и `reason` (зачем он
  этому шагу). Новая авторская версия шага; пакет без объявления компилируется
  и ведёт себя как прежде. Профиль едет в `WorkflowRevision`, как
  `technical_retries` в 0.13.34: объявление — свойство шага, а не Run, и в
  состоянии ему делать нечего.
  `TestStepDeclaresTheModelProfileItWants`: с объявлением шаг опускается в v9
  и поле доезжает до плана, v8 его не принимает, а шаг без объявления
  по-прежнему опускается в v7 — байты, которые он запечатывал до появления
  v9, не сдвинулись.

  Побочно: `TestSessionLimitsAuthoringRefusesInvalidContracts` брал
  `schema_version: '9'` как «редакцию, которой сборка не знает». Девятка
  перестала ею быть, фикстура переведена на '10'.

- [x] 1.2 **Переписана по ходу реализации.** Исходно задача просила отказ на
  значении, «похожем на имя площадки или модели». Такой проверки не
  существует: `claude-opus-5` и `deep-reasoning-5` различает смысл, а не
  форма, и любое правило здесь — догадка, одетая в контракт. Проверка,
  которая выглядит доказательством, не будучи им, — ровно тот класс дефекта,
  который правили 0.13.38 и 0.13.40.

  Сделано вместо этого: контракт проверяет форму идентификатора
  (`^[a-z][a-z0-9-]{1,63}$`) и требует `reason` вместе с `requested` — запрос
  без причины читается как предпочтение, и следующий автор не знает, можно ли
  его убрать. Разницу между профилем работы и именем площадки объясняет
  authoring reference (5.1), а не отказ, который притворяется, что умеет её
  различать.

## 2. Задача

- [x] 2.1 `SessionTask.ModelProfile` проецируется из запечатанного плана в
  `sessionTaskFrom`, рядом с `routed_verdicts`.
  `TestATaskCarriesTheModelProfileItsStepDeclared`: шаг без объявления отдаёт
  документ прежней формы, шаг с объявлением — называет его. Проверено, что
  тест держит именно проекцию: со снятой строкой `task.ModelProfile =
  step.ModelProfile` он падает «the task did not carry what the step
  declared: <nil>».

- [x] 2.2 Объявление не дублируется в `SessionHandoff`: тот же тест
  сериализует сохранённый handoff и падает, если в нём появились `model_profile`
  или значение профиля. Форма состояния от объявления не меняется — растёт она
  только там, где появляется новый факт (раздел 3).

## 3. Отчёт

- [x] 3.1 `SessionSubmission.ModelProfile` + `checkModelProfileStatement` на
  приёме. `TestADeclaredModelProfileIsAnsweredOrTheReportIsRefused` — девять
  случаев: три принимаемых (honoured/unavailable/declined) и шесть
  отказываемых, каждый своим кодом — `model_profile_unanswered`,
  `_outcome_invalid`, `_model_unnamed`, `_reason_unexpected`,
  `_model_unexpected`, `_reason_missing`. Плюс обе стороны для шага без
  объявления: молчание принимается, ответ на несуществующее объявление
  отказывается `model_profile_not_declared`.

- [x] 3.2 `Attempt.ModelProfileReport` (`model-profile-report/1`): исход,
  названная модель, причина, наблюдение и копия `requested` — чтобы запись
  говорила, на что она отвечала, даже читателю, у которого есть Run, но нет
  плана. Тексты отказов и описания полей нигде не называют это подтверждённым
  выбором: каждое говорит, что это заявление хоста о себе.

- [x] 3.3 `TestWhatTheHostSaidAboutTheProfileIsStoredAndCountable`: три
  полных круга через запечатанный план — объявление доезжает до задачи, отчёт
  принимается, запись сходится с заявлением по всем полям — и три ответа
  считаются как три исхода, а не три строки текста.

## 4. Контракты

- [x] 4.1 Семейство `model-profile` опубликовано рядом со старыми:
  `urn:prifly:core-model-profile:34`, 237323 байт,
  `sha256:2f090ec6ffe30a0802e05009161137b688f3e5c420652465a16201d2d96d88d4`.
  Ни один прежний бандл не сдвинулся — `git diff` по всем `*.schema.json` и
  `schemas/**` пуст, появились только два новых файла. `modelProfileField`
  прячет поле от каждого бандла, опубликованного до него.

  **Найдено тем, что я этого не сделал сразу.** Поле `SessionTask.ModelProfile`
  было закоммичено и запушено до семейства: `make schemas-check` ответил
  «Schema drift: internal/runtime/sessions.schema.json», CI покраснел на
  `19fc08e`. Перед коммитом были прогнаны только точечные тесты, а не ворота.
  Урок ценой одного красного CI: новое поле опубликованного DTO и его
  семейство — один коммит, не два.

  Измерено заранее (остаётся в силе): правка `maxItems` на месте даёт
  «Immutable invocations schema hash changed; version the new contract
  separately» — guard работает и должен остаться.

- [x] 4.2 Потолок поднят до 64 **в новом семействе**: `model-profile` даёт
  `maxItems: 64`, `program-environment` и всё прежнее — `32`, байт-в-байт.
  Capability-документ теперь перечисляет **33** state- и **33** read-версии.
  `TestTheNewBundleAllowsWhatEveryOlderOneRefuses` держит обе стороны: текущий
  бандл документ принимает, `CoreCapabilitiesV31` его отвергает, и тест
  проверяет, что отвергает именно по потолку, а не по чему-то ещё.

- [x] 4.3 Пять перенаправлены на `CoreCapabilitiesV34`, ни одно не удалено.
  Предсказание сошлось с фактом: упали ровно те пять —
  `TestArtifactClosureSealsExactManifestBeforeProducerSettlement`,
  `TestArtifactPublicationSealsBeforeProducerSettlementAndDoesNotReread`,
  `TestCallCapabilitiesDeclareBothStateContracts`,
  `TestArtifactPublicationChecksAcceptSealedItemBeforeProducerSettles`,
  `TestEachPublicationKeepsIndependentCursorsAndPendingAssignments`.
  То, что они охраняли, отозвано сознательно; то, что осталось — «документ
  вообще валиден» — они охраняют по-прежнему.

  Побочно сработал guard из 5.4 прошлого change'а:
  `TestEveryStateNamesItsNextContract` упал на том, что новая state-версия не
  назвала свой next-контракт в руками написанной таблице. Ровно для этого он
  и писался.

## 5. Документы и ворота

- [ ] 5.1 `examples/authoring/step-authoring-reference.yaml` и
  `examples/troubleshooting.md` называют поле, три исхода заявления и то, чего
  движок не обещает. Проверка: `python3 -B test/e2e/test_examples.py`.

- [ ] 5.2 Раннер называет объявление и три исхода: что делать, если площадка
  не даёт выбрать, — сказать честно, а не промолчать. Пины обновлены.

- [ ] 5.3 `openspec/specs/delivery-roadmap/spec.md`: запись очереди и
  требование о model profile обновлены по факту, без заявления product
  qualification. `aif-fanout` остаётся future acceptance fixture: успешная
  компиляция по-прежнему не доказывает выбор модели.

- [ ] 5.4 `make ci-check`, `make e2e`, `make race` зелёные; счётчики записаны
  здесь.
