## 1. Зафиксировать baseline и новые version boundaries

- [ ] 1.1 Подготовить implementation branch на базе с commit `30131326`; явно зафиксировать отношение к released `38980861`. Проверка: reproducible base refs и отсутствие merge/release утверждений в planning evidence.
- [ ] 1.2 Снять deterministic `/4` baseline на exact published v0.13.83. Проверка: package/build refs, native definition bytes, DTO versions, disabled behavior, effect/consent guards и typed refusals в новых immutable fixtures.
- [ ] 1.3 Зафиксировать protected manifest существующих schemas, archived changes и release evidence относительно согласованной базы. Проверка: повторное сравнение каждого исходного path/hash/mode; новые additive файлы не заменяют старые.
- [ ] 1.4 Добавить closed authoring schemas extension `/5` и folder `/3` с exact selectors и `exports.points`. Проверка: schema positive/negative cases; `/4` и folder `/2` отвергают новые fields; sidecar `/1` byte-identical.
- [ ] 1.5 Добавить новые Project DTO и schemas: compile `/4`, summary/start `/7`, provenance `/3`, composition `/2`. Проверка: `make schemas-check`; generated/public/embedded pairs совпадают, emitted fixtures проверяются своими версиями.

## 2. Выделить owner model и versioned readers

- [ ] 2.1 Добавить общий typed compiler representation с source owner, origin, author identity, attachment и implementation origin. Проверка: colliding ID/kind из разных owners не даёт полномочий после flattening.
- [ ] 2.2 Сохранить старый dispatch без нового root opt-in; добавить явный dispatch `/5` либо root folder `/3`. Проверка: differential `/4` baseline и named refusal нового provider под старым root.
- [ ] 2.3 Нормализовать parent `/3` и `/4` по собственным closed readers. Проверка: `/4` не получает imported target или object selector, даже при unified consumer.
- [ ] 2.4 Разрешать local/inherited, direct-parent и import point selectors без глобального поиска. Проверка: ambiguous names, unknown parent/import, wrong kind и guessed private target дают refusal.
- [ ] 2.5 Проверять исходную sidecar inventory до flattening и сохранять root/child mapping при inheritance. Проверка: derived root points, child/repeat body и foreign/project-owned target substitution.

## 3. Реализовать public point exports и isolated variants

- [ ] 3.1 Разрешить `exports.points` только через local point и public workflow export folder `/3`. Проверка: hook вне reachable closure, private/unowned target и root-launch export отвергаются.
- [ ] 3.2 Добавить scoped selection по import alias и workflow export. Проверка: два aliases одного provider создают configured и passthrough variants без source mutation.
- [ ] 3.3 Реализовать зависимые variants для exports с общим nested child. Проверка: hook одного export не меняет другой; same effective policy внутри сборки даёт один exact variant.
- [ ] 3.4 Разделить source cycles и definition/variant dependency cycles. Проверка: допустим consumer-owned implementation внутри provider point; actual Call cycle, recursive point dependency и repeated-alias budget reset отвергаются.
- [ ] 3.5 Связать programs только с owned selected variants; сохранить source directory и каждый прочитанный supporting path. Проверка: private missing file не читается, foreign binding отказывает, shared exact program сохраняет все paths.
- [ ] 3.6 Ввести общий предел 256 workflow variants и сохранить остальные compile/native budgets. Проверка: 256/257, cached depth 8/9, folders/layers limits и generated-wrapper accounting.

## 4. Реализовать b2 и package mapping

- [ ] 4.1 Определить canonical variant-key record без compiled refs и final package digest. Проверка: fixed vectors для owner, effective settings, point policy и dependency keys; map traversal не меняет key; перестановка parent declarations не разрешает conflicts и сохраняет нормализованный effective graph.
- [ ] 4.2 Добавить b2 build key и compiled-version tuple из design, сохранив b1 code path. Проверка: exact formula vectors, distinct variants получают разные refs, unchanged `/4` b1 vector совпадает с baseline.
- [ ] 4.3 Добавить provenance `/3` writer и reader с mapping source/author/variant/compiled. Проверка: package round-trip, несколько variants одной author identity, forged mapping и swapped owner/ref отвергаются.
- [ ] 4.4 Обновить package sealing снизу вверх без изменения source IDs. Проверка: nested Call/Repeat refs указывают на правильные variants, внешние exact refs и literal data не переписываются.

## 5. Сохранить point data contract и добавить допустимые группы

- [ ] 5.1 Разрешить `/5` point leaf с local project implementation либо explicit imported export. Проверка: unknown implementation, wrong kind, provider program override и client routes/inputs/consumers отвергаются.
- [ ] 5.2 Проверять exact schema, availability и каждый declared consumer до sealing. Проверка: reader и repeat next_bindings получают новый carrier; missing/partial output не заменяется прежним.
- [ ] 5.3 Сохранить disabled passthrough и closure-scoped execution consent. Проверка: disabled imported program не читается и не требует allow-execution; active program использует allowlist/review.
- [ ] 5.4 Объединить parents/layers по source declarations и ancestry. Проверка: common ancestor once, explicit duplicate layer refusal, opaque replacement, independent conflicts и consumer resolution.
- [ ] 5.5 Добавить point arrange только для empty-consumer point и compatible step/call routes. Проверка: sequence/parallel/choice, unknown/default, distinct failure routes, missing/mismatched on_error и impossible_verdicts.
- [ ] 5.6 Сохранить origins и inherited groups при следующих уровнях composition. Проверка: каждый active member один раз; duplicate/unknown/disabled/missing selector и nested group depth дают named refusal.
- [ ] 5.7 Сохранять technical failures без outcome до outer point on_error. Проверка: Step и Call failure в sequence/parallel/choice; branches settle; publication после ошибки не происходит.
- [ ] 5.8 Отказать на automatic group с consumer ports и на overlap root/point anchors. Проверка: exact unsupported/conflict codes, absence partial output, отдельный typed Call с carrier продолжает работать.

## 6. Обновить CLI review и авторские входы

- [ ] 6.1 Выдавать новые DTO только при новом root opt-in и сохранить naming/queue/recovery fields. Проверка: real compile/prepare/start/continue fixtures проходят новые closed schemas; старые version paths не меняются.
- [ ] 6.2 Показать owner/variant/point/implementation/consumer mapping и disabled choices в review/explain. Проверка: deterministic relative paths, scoped aliases и отсутствие programs/claims/authority mutation при prepare.
- [ ] 6.3 Включить новые inputs в freshness digest и сохранить source-aware typed Fault. Проверка: изменение export, sidecar, layer, settings или supporting bytes отменяет старый review до claim/dispatch.
- [ ] 6.4 Добавить capability `workflow_composition_with_points`, author index и reference с actual `/5`, folder `/3`, selectors и exports. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, обе стороны таблицы и ручное ревью minimum native revisions.
- [ ] 6.5 Обновить glossary, agent brief и source-of-truth map в scope нового delta. Проверка: `TestGlossaryBindings`; existing terms и saved JSON names сохранены; main `/4` rules и historical evidence не переписаны.
- [ ] 6.6 Подготовить runnable generic example с parent `/4`, imports и public nested hook. Проверка: examples показывают zero-consumer UI+audit group и отдельный typed carrier Call; явно отмечены synthetic и реальные browser/release gates.

## 7. Совместная приёмка source candidate

- [ ] 7.1 Добавить regression-first проверки mixed source contracts и adversarial ownership. Проверка: red на прежнем candidate, green после реализации; новые Go tests используют `t.Parallel()`, кроме process/global mutations.
- [ ] 7.2 Запустить полный unchanged `/4` harness и новый mixed harness на одном exact binary. Проверка: identity baseline, outputs, disabled/effects/consent, applicability required/not-required/unknown, isolated variants, groups и freshness.
- [ ] 7.3 Проверить partial/cancel/continue на native root frontier и fresh nested ArtifactRef. Проверка: source/target Runs и exact package refs записаны; stale carrier reuse и nested-stage resume не объявлены допустимыми.
- [ ] 7.4 Выполнить `make test-changed` либо `make check-fast` по текущему diff и `openspec validate --all --strict --no-interactive`. Проверка: реальные логи без заявления feature qualification по одной document validation.
- [ ] 7.5 Выполнить финальные `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1` на Go проекта и `git diff --check`. Проверка: staged bytes, receipt context, fresh vuln/mod verify и повторный protected-file manifest check.
- [ ] 7.6 Провести security и independent review source/variant ownership и b2 compatibility. Проверка: findings разрешены, docs/evidence называют конкретный candidate; local commit создаётся только по поручению владельца.

## 8. Отдельный выпуск и consumer qualification

- [ ] 8.1 Перед выпуском запустить `python3 scripts/ensure-release-checks.py --sha SHA --ref main --wait` на фиксированном release commit. Проверка: зелёные exact-SHA verify/qualify; публикация требует отдельного поручения.
- [ ] 8.2 Выпустить через `python3 scripts/tag-release.py vX.Y.Z` и повторить оба CLI harness на exact published binary. Проверка: version/SHA256/signatures, unchanged `/4` и mixed `/5` evidence отдельно от source candidate.
- [ ] 8.3 Передать unified authoring downstream workflows и отдельно провести живую consumer приёмку. Проверка: реальные browser lifecycle, permissions, fresh candidate correlation и owner acceptance; Core fixture не закрывает F2/P1/P2/smsplace.
