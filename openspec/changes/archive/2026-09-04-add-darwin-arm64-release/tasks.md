## 1. Multi-platform release package

- [x] 1.1 Расширить release builder с одного binary до declared набора exact platform binaries и одного signed manifest; проверить unit test на два assets, duplicate platform и отсутствие обязательного asset.
- [x] 1.2 Сохранить installer как единственный asset и проверить, что `darwin/arm64` и `linux/amd64` updater выбирают только свой archive, а несовпадающая platform не меняет binary.

## 2. Protected release pipeline

- [x] 2.1 Добавить protected Apple Silicon macOS build job с native CGO build и короткой qualification; проверить CI topology test и воспроизводимую команду на macOS runner.
- [x] 2.2 Собрать в Linux release job оба binary artifacts в один signed asset set и потребовать оба перед upload; проверить, что missing macOS artifact останавливает pipeline до publication.
- [x] 2.3 Добавить в GitLab Release links оба named archives; проверить `test/e2e/verify-release-ci.py` и отсутствие изменения protected credentials/manual gates.

## 3. Документация и приёмка

- [x] 3.1 Обновить README и release-distribution specification точной matrix `linux/amd64`, `darwin/arm64`; проверить, что текст не обещает поддержку других architectures.
- [x] 3.2 Выполнить `make ci-check`, `make e2e`, `openspec validate add-darwin-arm64-release --strict` и `openspec validate --specs`; затем выполнить native qualification на registered protected Apple Silicon macOS runner.
- [x] 3.3 Проверить `git diff --exit-code -- openspec/changes/archive` и historical release evidence; зафиксировать результат перед отдельным решением о следующем semantic release version.

Для 3.2 локальные `make ci-check`, `make e2e` и обе OpenSpec-проверки прошли;
нативная qualification также воспроизведена локально на Apple Silicon.
Защищённый project runner с тегом `macos-arm64` зарегистрирован и online;
его protected tag job успешно квалифицировал macOS binary для `v0.4.0`.
Для 3.3 архивный diff чист, а опубликованный `v0.3.0` содержит только
`linux/amd64`; следующим Release стал согласованный `v0.4.0`.
