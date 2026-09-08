# Проверки изменения монитора

Дата: 2026-09-08. Worktree: `/private/tmp/prifly-monitor-storage`.
Ветка: `codex/monitor-storage`, база: `a5a2202`.

## Выполнено

- `go test -race ./cmd/prifly ./internal/runtime ./internal/local -run 'Monitor|Cleanup|Storage|Fork|Start|Neutral|Grant|Approval|Claim|Glossary' -count=1`: PASS во всех трёх пакетах (102.707 / 164.691 / 4.987 с). Go 1.27, darwin/arm64.
- Точечные новые tests без race: PASS. Проверены запрет незавершённых Run и открытого Engine; сохранение общего артефакта другого Run и независимого import; удаление orphan blob/upload; stale preview; HTTP Origin/token/method и успешное удаление; повтор; восстановление служебного workspace из audit после ошибки файлового удаления; сохранность внешней цели symlink; `Store.Verify` после очистки.
- `TestMonitorStorageCountsHardlinksAndSkipsSymlinks`: PASS; выделенные блоки, hardlinks и общий итог без повторного учёта inode, отсутствие обхода внешнего symlink.
- `make vet fmt-check refusal-check schemas-check build`: PASS, включая CGO=0 vet. Публичные schema bundles не изменены.
- `node --check cmd/prifly/monitor.js` и `node --test cmd/prifly/monitor_ui_test.cjs`: PASS. Существующие UI tests проверяют escaping, timing, графы и file evidence; ручной проверки новых экранов они не заменяют.
- Статическая проверка уникальности HTML id, связности JS/DOM и структуры CSS: PASS.
- `openspec validate improve-monitor-navigation-storage --strict` и strict validation трёх затронутых specs: PASS.
- `git diff --check`: PASS. Исторические release evidence и manifests не менялись.

## Непроверенные границы

- Browser Use дважды отказал в создании вкладки из-за недоступной проверки административной политики. Визуальная проверка длинного списка, Back/возврата, размеров и диалога подтверждения остаётся открытой в tasks.md. Защита браузера не обходилась.
- Новые HTTP/runtime tests удаляют только данные во временных test fixtures. Реальные пользовательские хранилища не очищались.
- Полный release gate / `make check`, нагрузка на очень больших хранилищах, смешанная работа со старыми бинарниками и уникальное физическое потребление APFS clones/snapshots не квалифицированы.
- Блокировка действует между обновлёнными клиентами. Перед использованием очистки следует обновить все клиенты и закрыть старые процессы. Открытый Engine/driver удерживает хранилище; очистка отказывает и допускает повтор после его закрытия.
- Сканирование диска фоновое; показанный итог может быть неполным при ошибках и не является точным обещанием освобождённых физических блоков. SQLite VACUUM требует дополнительного места. Audit/receipts, настройки, пакеты, независимые imports и используемые данные сохраняются.
