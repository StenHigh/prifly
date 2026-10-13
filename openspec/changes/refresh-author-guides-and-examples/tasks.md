## 1. Подготовка и сверка

- [ ] 1.1 После интеграции предыдущих claims владельцем зафиксировать baseline нового Run, уже внесённые documentation changes и статус stable/development. Проверка: список файлов и версий в implementation-отчёте, отсутствие повторного копирования текстов reuse/recovery.
- [ ] 1.2 Сверить capability-таблицу и authoring references с `prifly capabilities`, кодом и нормативными источниками; исправить конкретные пробелы полей, минимальных ревизий и ссылок. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и смысловое ревью каждой изменённой строки; приёмочная запись перечисляет найденные пробелы и исправления.

## 2. Руководства и навигация

- [ ] 2.1 Обновить индекс `examples/README.md` и короткие ссылки README, включая ревизии 7/8, решения Run, progress/live output и актуальный follow-up Claude. Проверка: локальные ссылки разрешаются, reader path ведёт от задачи к конкретному справочнику или исполняемому примеру; README не дублирует новые полные руководства.
- [ ] 2.2 Добавить `examples/authoring/launch-choice-guide.md` со snippets `title`/`description`, необязательных критериев и отключения совета через `PROJECT.md`, явного выбора и fallback. Связать объявленные preflight/runtime decisions с существующим справочником. Проверка: snippets сверены с `/3` и prifly-run; точный ID, отсутствие задачи, пропуск совета и отсутствие host допустимы; выбор не означает start approval; Claude UI не отмечен PASS.
- [ ] 2.3 Уточнить continuation-guide и troubleshooting: когда применять continuation, resume и recovery, что наследуется, что повторяется, какие отказы защищают повтор, как получить свежий prepared digest. Проверка: команды сверены с CLI интегрированного baseline; статусы development/release truthful, уже внесённые nested recovery тексты сохранены без дублирования.
- [ ] 2.4 Сверить contributor-инструкции с интегрированным verification reuse: local snapshot, fresh обязательные проверки и release gates на том же SHA. Проверка: команды существуют в Makefile/helper, текст не обещает перенос успеха между коммитами и не заменяет CI локальными результатами.

## 3. Исполняемые примеры

- [ ] 3.1 Дополнить CSV-пример демонстрацией program progress и явного live output с ограничением вывода; документировать наблюдение и ожидаемый report. Проверка: compile и обычный запуск без Git/ИИ проходят в disposable проекте, report совпадает с прежним, progress и stdout/stderr наблюдаются через документированные команды; fd 3 и fd 4 не смешаны.
- [ ] 3.2 Добавить минимальный `examples/workflows/resumable-report` с управляемым первым failure и возобновлением тем же workflow ревизии 8. Проверка: на собранном CLI в disposable проекте первый Run завершается ожидаемо, второй завершается успешно, принятый первый результат наследуется и не исполняется заново; README-команды воспроизводят оба Run без Git/ИИ и дают ожидаемые bytes.

## 4. Общая приёмка

- [ ] 4.1 Выполнить `python3 -B test/e2e/test_examples.py`, подходящие `make test-changed`/`make check-fast`, описанную выше сквозную приёмку примеров, `openspec validate refresh-author-guides-and-examples --strict` и `git diff --check`. Предусмотренный полный локальный gate выполнить один раз для итогового снимка через helper интегрированного baseline; reuse — только по его условиям. Проверка: результаты и hashes бинарника/примеров записаны в implementation-отчёт; без dispatch qualify, нового release и живой Claude UI-приёмки.
- [ ] 4.2 Проверить `git diff --name-status BASELINE` на отсутствие изменений runtime, опубликованных schemas, versioned contracts и `openspec/changes/archive/`; отдельно проверить отсутствие правок historical release evidence/manifests. Проверка: итоговый diff ограничен согласованными руководствами, примерами, их необходимыми проверками и текущими planning/report artifacts; текущий Claude follow-up сохраняет DEFERRED.
