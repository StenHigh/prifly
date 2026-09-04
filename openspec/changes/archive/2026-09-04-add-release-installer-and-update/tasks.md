## 1. Release contract and publication path

- [x] 1.1 Добавить versioned release manifest, Ed25519 signing/verification и minimal platform archive builder на Go standard library; проверить unit tests для valid signature, changed manifest, changed archive и unsupported platform.
- [x] 1.2 Сделать inspectable user-space bootstrap script, который скачивает asset в temporary file, не требует `sudo` и не меняет project files; проверить positive и unwritable-destination shell cases во temporary directory.
- [x] 1.3 Добавить protected-tag/manual GitLab release job, который публикует только signed assets, и проверить CI configuration плюс local dry-run без publication.

## 2. Managed update command

- [x] 2.1 Реализовать managed-install receipt и безопасное определение собственного installation path; проверить, что source build и copied binary не могут стать целью update.
- [x] 2.2 Реализовать `prifly update`: signed latest manifest, SemVer no-downgrade, bounded archive extraction, temporary write и atomic replacement; проверить current, successful update, interruption и invalid signature/digest через local HTTP fixture.
- [x] 2.3 Добавить structured CLI result, help и diagnostics для `update`; проверить successful output и `invalid_usage` без network request.

## 3. Документация и qualification

- [x] 3.1 Обновить README и SECURITY policy: exact install command, first-bootstrap trust boundary, supported platform и manual update/recovery; проверить, что README не дублирует OpenSpec contract.
- [x] 3.2 Добавить `release-distribution` в OpenSpec source map и синхронизировать current roadmap/source documentation без изменения historical evidence; проверить `openspec validate add-release-installer-and-update --strict`.
- [x] 3.3 Прогнать `make ci-check`, `make e2e` и targeted updater tests; сохранить точные результаты в change/commit, не выдавая local package за public qualification.

## 4. Первая публикация

- [x] 4.1 Владелец создаёт protected `PRIFLY_RELEASE_SIGNING_KEY`, matching `PRIFLY_RELEASE_PUBLIC_KEY` и отдельный `PRIFLY_RELEASE_PUBLISH_TOKEN`, подтверждает compiled public key и запускает first qualified tag release; проверить install и `prifly update` из опубликованных GitLab assets на named supported platform.
