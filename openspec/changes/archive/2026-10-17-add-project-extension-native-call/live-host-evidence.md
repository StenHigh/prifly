# Native Call: завершение live-приёмки 7.2

Дата решения и завершения: 2026-10-17 (локальная дата); native timestamps ниже — UTC.
Это дополнение к неизменным implementation/release evidence, отражающим более ранние срезы.

## Бинарник и граница проверки

Использован exact released **Pri-Fly 0.13.78**, linux/amd64, Go 1.27.2,
source SHA `201d4428b60e6afc9aef84fc5197a3185cf2f20b`, binary
`sha256:60a6dd1ca0524b4927418a64311207b4d6739080a8f518374d1889726d68ab5f`.
Происхождение и signature — [release evidence](release-0.13.78.md).

Профиль: disposable Project `/3`, worktree, core-local authority, cooperative
native sessions и отдельная local policy fixture. Generated runner взят из
released CLI. Успешный leaf получает тот же Run/claim generation/candidate и
реальные `request` bytes; он читает pinned context с уникальным nonce, пишет
`report/gate`, получает native submit template, хеширует bytes и отправляет
результат. Collector проверяет accepted artifact identities/digests и exact
exports downstream. Producer, downstream reader и завершающий commit-step
исполняются синтетическим fixture host; **живым является только assigned leaf**.
Проверка не утверждает качество review/UIUX, browser lifecycle или W6 policy.

Control leaf read-only (`effects.class: none`), без workspace-write и task claim
fields. Controller ждёт реальный native take, отменяет deepest child и наблюдает
`blocked_child`; живой host читает это состояние, не пишет outputs и не submit-ит
результат. Затем collector отменяет весь disposable Run и проверяет settlement
без active Attempts. Native cancel не доказывает остановку произвольного внешнего
actor. В обоих сценариях source hashes и dirty candidate bytes сохранены.

## Результаты

| Host | Версия | Success | Control |
|---|---|---|---|
| Codex app | execution runtime `0.162.1`, текущий app thread | PASS: accepted `report/gate`, 2 exports, Run succeeded | PASS: child cancelled, blocked_child, exports/report отсутствуют, Run settled cancelled |
| Codex CLI | `codex-cli 0.162.1` | PASS: настоящий CLI process, exit 0, accepted 2 exports, Run succeeded | PASS: настоящий CLI process, exit 0, native cancellation прочитана, без submit, Run settled cancelled |
| Claude Code | `2.1.287`, `loggedIn=false` | **Исключён из scope 0.13.78 решением владельца** | **Исключён из scope 0.13.78 решением владельца** |

App UI build не предоставлен окружением: `0.162.1` — версия execution runtime
из `CODEX_VERSION`, а не утверждение о версии UI. App-проверки выполнены текущим
живым thread `01a14484-2ae5-7140-89b0-795b3e73e416`, без fork/subagent. Запись
`actor-exit` для app означает завершение tool actions, не exit desktop process.
CLI запускался через `codex exec --json --approve-for-me --ephemeral --cd …
--add-dir … -`, с обычным workspace-write/approval review, существующим login,
изолированным fixture HOME. Model override не запрашивался. Секреты не копировались.

Полные Run/Attempt/child/claim identities, native take/settlement, accepted refs,
host commands/exits, компактные observations и SHA256 исходных traces закреплены
в [live-host-results.json](live-host-results.json). Raw JSONL, task/input/output,
controller и final native views сохранены по указанным там temporary paths;
их долговечность не гарантируется, поэтому существенные факты включены в JSON.
Успешные native Runs имеют по 4 invocations; отдельного Run для child нет.

Одна начальная control preparation остановилась на assertion самого coordinator:
readonly task закономерно не содержит claim fields. Assertion ограничен writable
success-case, первая disposable fixture отменена и settled; она не считается
host PASS. Повторная fixture прошла реальный control-сценарий.

## Решение владельца и follow-up

Claude Code доступен как executable, но подписки и авторизации сейчас нет.
Владелец явно выбрал: «Исключить live Claude Code из приёмки 0.13.78, записать
отдельный follow-up и архивировать после остальных проверок».

Это release-scope исключение только для live Claude Code **0.13.78**, а не PASS
и не исключение будущих проверок. Synthetic generated-runner cases всех трёх
hosts уже PASS в released E5 51/51; они не заменяют live Claude. Отдельная работа:
[qualify-claude-native-call-host](../../../../.ai-factory/plans/qualify-claude-native-call-host.md).
Приёмка 7.2 завершена в явно согласованном scope.

## Воспроизведение

Coordinator `test/e2e/verify-call-extension-live.py` проверяет native факты,
но сам не подменяет живого host. Из корня Core:

```sh
python3 -B test/e2e/verify-call-extension-live.py prepare \
  --binary /absolute/path/to/released/prifly --root /tmp/EMPTY-LIVE-PASS \
  --host codex-cli --mode pass
```

Передать `host-prompt.txt` настоящему выбранному host, дождаться его завершения,
затем выполнить `collect --state /tmp/EMPTY-LIVE-PASS/live-state.json` тем же
скриптом. Для `--mode cancel` параллельно с настоящим host запустить
`cancel-after-take --state …/live-state.json`; после выхода host выполнить
`collect`. Для каждого host/mode нужен отдельный пустой root. Generated runner,
pinned task/context и настоящие input files должны быть прочитаны самим host.
