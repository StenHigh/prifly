# Проверки

Дата: 2026-09-13.

- Регрессионный `TestCleanupRemovesOnlyOwnedWorkspace` до исправления: FAIL на отсутствии пути в `unsafe_path` после разрешения тестового Unix-сокета. Первоначальный sandbox-прогон не дошёл до проверки из-за запрета сокета.
- После исправления `go test ./internal/runtime ./cmd/prifly -run 'TestCleanup|TestMonitor(CatalogAndScopedReads|Storage|Maintenance)|TestProblem' -count=1`: PASS (1.417 с / 2.414 с).
- Уточнённая проверка повторного отказа в Cleanup до digest: PASS (1.036 с). Проверяются адресный и общий preview, сохранность Run и внешней цели ссылки, успешная очистка после переноса проб.
- `make build fmt-check refusal-check`, OpenSpec strict validation и `git diff --check`: PASS.

## Локальный стенд

Каталог проб `/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority/.prifly/fp` перенесён в `fp` под тем же authority root. Все исходные bytes сохранены; оригинал изменённого скрипта — `fp/refusal-diff.sh.before-metadata-fix`. Пути `.prifly/fp` заменены на `fp`, `bash -n` прошёл. Сам скрипт отказов не запускался.

Монитор перезапущен проверенной сборкой. Штатный preview для разрешённого владельцем Run `run:f1e421fc0f839be478d5fe611cdd637a0ea31f5dc4f65767c407d3962da0f3a9` вернул HTTP 200 и только этот Run в плане. Штатный delete вернул HTTP 200: `deleted_runs=1`, `deleted_files=40`, `deleted_bytes=186497`, `warnings=[]`.

После удаления API подтвердил сохранность остальных 16 Runs и `not_found` при прямом чтении удалённого Run. Данные внешнего repository не изменялись. Полный release gate не запускался.
