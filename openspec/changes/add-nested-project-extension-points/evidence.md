# Локальная проверка реализации

Native matrix candidate: **11 cases / 12 Runs PASS**. Точный binary hash и
результаты находятся в `native-results.json`. Ответы синтетические. Проверка
не использует LLM, provision или прикладную suite.

Проверены entry и after-repair step/Call, exact reader input slots, fresh
carrier/report/log refs, original refs после удаления producer directory,
success/repair/partial/blocked/fail/no_work, missing output, absent/disabled,
stale prepare digest, reviewed continue на root frontier и сохранность source.
Изменённый accepted prefix и nested-stage resume получают native refusals.
Каждый case освобождает свои claims.

Сравнение с v0.13.82 использует exact consumer baseline fixture. Build key,
package ref и каждый component ref совпадают. Saved runtime schemas не менялись.
Прежние extension-v1/v2 не менялись. Новые closed schemas требуют opt-in.

Verify нашёл ownership defect для package folder с именем `project`.
Repair добавил package-relative prefix и общий cycle graph для legacy Call
и point. Обе регрессии сначала падали, затем прошли. Пять дополнительных
null/double-selector cases также сначала падали, затем прошли. Compiler
использует embedded editor schema для closed intake новых форм.

Целевые Go tests, editor contracts, TestGlossaryBindings, capability index,
schema-gen consistency, diff --check и все 37 OpenSpec items прошли локально.
Первый full batch прерван после подтверждённого semantic blocker. Полный batch с Go 1.27.3 прошёл: tests, vet linux/darwin, staticcheck
linux/darwin, fmt/refusal, vuln-check, schemas, scripts-check и go mod verify.
Verify, security и review приняты текущим Run. Security не нашёл новой
уязвимости; audit.sh остановился на false positive в ignored module cache.
Ограничения проверки сохранены в gate artifact. Итоговый staged-снимок
проверяется через ci-check-once CHECK_FOR_COMMIT=1 перед локальным коммитом.

Task 4.2 остаётся открытым до owner release. После выпуска нужно выполнить
generic example на released binary и передать capability consumer issue #9.
Этот локальный результат не закрывает release gate, consumer acceptance или P1/P2.
