# Проверка выпуска Pri-Fly v0.13.85

Core выпущен: [v0.13.85](https://github.com/StenHigh/prifly/releases/tag/v0.13.85). Единый `/5` доступен через явный root opt-in. Прежний `/4` сохраняет опубликованные контракты.

```text
source -> verify + qualify -> tag -> native build -> signatures -> published replay
```

## Закреплённые входы

- Release SHA: `3e91ba5cbebee382df701a8e8e278a57cd876d16`.
- Implementation commit: `4f9e2669b4790ee73a559c809dbeb8c39ef8069b`.
- Исходная база опубликованного `/4`: `389808611e40f17d5bbcedebf4580e0db93eb06b` (v0.13.83).
- Native Run: `run:6a199b8aacdaa2a11493300d876b3b636b7f61027e493ca0740091a691e1bb20`; `completed/succeeded`.
- Финальные локальные ворота: Go 1.27.3, `CHECK_CONTEXT=codex-app-authorized`, `CHECK_FOR_COMMIT=1`; PASS. Последний batch выполнен полностью без reuse.
- OpenSpec strict validation и diff check: PASS. Protected manifest: 907 файлов без изменений. Сохранённые исходные 53 файла и stash не изменены.

## CI на точном SHA

- [verify](https://github.com/StenHigh/prifly/actions/runs/41337932218): прямой полный batch, PASS.
- [qualify](https://github.com/StenHigh/prifly/actions/runs/41337938588): race detector и полный e2e, PASS.

Первый кандидат остановился на устаревшем expected schema set редакторского e2e-теста. Дополнительный commit обновил этот тест и modeline assertion. Попытка v0.13.84 затем остановилась до публикации из-за [планового отключения macos-14](https://github.com/actions/runner-images/issues/13518). Тег сохранён. Новый release job использует native macos-15. Ворота выше проверили итоговый SHA v0.13.85.

## Опубликованные bytes

Обе Ed25519 подписи manifest проверены ключом `PRIFLY_RELEASE_PUBLIC_KEY`. SHA256 обоих архивов совпал с подписанным manifest. Linux binary сообщил версию `0.13.85`. Mach-O header опубликованного macOS binary подтверждает ARM64 и сохранённый минимум macOS 14.0.

- Linux binary SHA256: `d512311554b31f469c912b3e1a2d65128edf99362d8ab29314c91448134fe954`.
- Linux archive SHA256: `0982fac9136eeaf0e37ff0af38154f3da356c91d1eb29d3e5679820e998729af`.
- macOS archive SHA256: `71821649d8d4e027597ebb7ddcc73ff1b7c75bfdc763c262c266e7b05908fcb1`.

## Проверки опубликованного Linux binary

- Прежний `/4`: 11 сценариев; полный harness, PASS.
- `/4` byte baseline v0.13.83: selected/absent/disabled и четыре typed refusals, PASS.
- Mixed `/5`: 11 сценариев; полный harness, PASS.
- Группы `/5`: 8 сценариев sequence/parallel/choice, failures и cancellation; PASS.
- Прежний `/3`: smoke трёх operators; PASS. Полная матрица `/3` входит в exact-SHA qualify.

Все replay reports закрепляют тот же SHA256 binary. JSON рядом содержит Run IDs, outcomes, refs и границы сценариев. macOS asset собран и проверен нативным release job; этот replay выполнялся на Linux.

## Граница приёмки

StepResult в этих сценариях синтетический. Проверки используют настоящий CLI, runtime и ArtifactRef. Они не подтверждают живой browser lifecycle, LLM, внешнюю публикацию или downstream owner acceptance.

Задача 8.3 остаётся отдельной по утверждённым `plan_constraints`. Этот выпуск не закрывает F2/P1/P2/smsplace.
