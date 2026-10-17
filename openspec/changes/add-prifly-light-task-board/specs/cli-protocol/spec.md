## ADDED Requirements

### Requirement: Light выбирается явно и изолируется от default monitor

Команда `prifly monitor` SHALL поддерживать `--ui full|light` с default `full`. Выбор `light` SHALL запускать отдельный frontend и строго read-only mux на заданном `--addr`; implicit auto-start SHALL сохранять full mode. Existing `--scan-root` SHALL оставаться выбором discovery и MUST NOT переключать UI. Light SHALL работать по зарегистрированным authorities без обязательного scan, storage sizing или migration.

Light options SHALL включать `--full-monitor-url` (absolute HTTP(S) URL без credentials/fragment), optional `--light-projection` (явный локальный файл observer mapping) и `--base-path` (default `/`, нормализованный абсолютный URL path). Для Light отсутствие full-monitor URL SHALL давать документированный local default `http://127.0.0.1:7777/`; за remote proxy operator SHALL задавать внешний URL явно. Custom base path SHALL обслуживать HTML/assets/API только в согласованном prefix, с relative same-origin URLs. Некорректные mode/URL/path/mapping и Light-only options в full mode SHALL отказывать до listener. Безопасность loopback/Host SHALL сохраняться независимо от proxy forwarding headers.

Light health SHALL иметь отдельную service identity `prifly-light/1` и mode/coverage; existing full health identity/shape SHALL сохраняться. Auto-start probe MUST NOT принимать Light service за полный monitor. Документация SHALL отличать новые release options от baseline: до реализации/выпуска этот синтаксис MUST NOT объявляться работающим.

Source set — `openspec/specs/cli-protocol/spec.md`; source ownership не переносится. Saved Run и published historical contract identifiers сохраняются; новые Light read versions публикуются отдельно.

#### Scenario: Default и ошибочный mode
- **WHEN** пользователь запускает monitor без `--ui` либо задаёт неизвестный mode
- **THEN** первый случай сохраняет полный monitor, второй отказывает до bind без изменения authority

#### Scenario: Local Light и proxy prefix
- **WHEN** выпущенный CLI запускается с `--ui light --addr 127.0.0.1:7782 --base-path /light/ --full-monitor-url https://monitor.example/full/?view=related`
- **THEN** Light HTML/assets/API доступны в `/light/`, ссылки сохраняют `/full/?view=related` и добавляют exact encoded source/run в hash route; health различает Light и full

#### Scenario: Light занял адрес full probe
- **WHEN** auto-start обнаружил отвечающий Light service на ожидаемом full порту
- **THEN** он не сообщает готовность полного monitor по этому health, Run продолжает existing execution path с обычным предупреждением при конфликте адреса
