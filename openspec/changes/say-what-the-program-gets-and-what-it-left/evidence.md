# Проверки

Дата: 2026-09-18.

- `make ci-check GO="$(go env GOROOT)/bin/go"`: rc=0. vet прочитал linux и darwin; fmt-check 297 файлов; refusal-check 162 файла; staticcheck 9 пакетов × 2 платформы, без находок; vuln-check 9 пакетов; все bundle'ы совпали, включая новый `program-environment` (236 220 байт, sha256:8c31272537fd20640070fac5ba015f6540fb31841b1cf7227d4872e65dffcda1).
- `go test ./...`: зелёный целиком.
- `openspec validate --all --strict`: 23 passed. `python3 -B test/e2e/test_examples.py`: 6 тестов, OK.
- `make e2e` и `make race` — в общем прогоне окна вместе с `run-a-failed-stage-again`.

## Что доказали тесты

- `TestNextAnswersWhatTheProgramWouldBeGiven`: у готовой программной стадии ответ о следующем действии несёт `program_environment` — имена отсортированы, место названо (`dotenv:<path>:PASSWORD`), значения нет ни в одном байте канонического документа; Run остаётся на своей версии состояния, потому что 33 её не заводит.
- `TestNonzeroExitNamesTheCodeAndTheBoundaryOfWhatIsKept`: сообщение диагностики содержит код выхода из `process_outcome` и фразу о том, что вывод программы не хранится.
- `TestProjectLocalReceiptDescribesTheFile`: заметка «sealed when its Run starts» идёт в stderr и не попадает в документ на stdout.

## Стена, на которую наткнулись

Первая сборка ставила Run'у `core-state/33` и уронила три группы тестов одинаково: `schema_invalid at /profiles/1/state_versions: the contract allows at most 32 items, this value has 33`. Замер: профиль `core-workflow/1` перечисляет ровно 32 версии состояния и 32 версии чтения, а опубликованный документ способностей ограничивает оба списка тридцатью двумя. Следующая версия состояния или чтения сделала бы наш документ невалидным против каждого уже опубликованного bundle.

Поэтому 33 — семейство только ответа: `core-next/33`, никаких новых полей в состоянии, никакой новой версии состояния. Версии ответа в документе способностей не перечисляются, предел не тронут. Правило записано в design: добавление, которое только отвечает, версию состояния не заводит.
