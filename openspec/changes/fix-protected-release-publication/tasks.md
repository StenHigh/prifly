## 1. Контракт публикации

- [x] 1.1 Исправить current release-distribution specification с Developer на Maintainer, добавить release-publish permission preflight и статическую CI-contract проверку; проверить её локально без сети и `openspec validate --strict`.

## 2. Безопасная ротация и выпуск

- [x] 2.1 Создать отдельный project-scoped publisher token роли Maintainer с `api` scope, атомарно заменить protected masked `PRIFLY_RELEASE_PUBLISH_TOKEN` и отозвать прежний publisher token; проверить через GitLab API только metadata token и variable, не выводя secret.
- [x] 2.2 Повторить только `release-publish` для существующего `v0.3.0`, проверить public GitLab Release, tag identity и четыре named asset links без rebuild или повторных product gates.

## 3. Приёмка

- [x] 3.1 Выполнить applicable local gate, `git diff --check`, `openspec validate fix-protected-release-publication --strict`; убедиться, что archive changes и published historical evidence не менялись.
