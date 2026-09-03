## 1. Проверка coverage переноса

- [x] 1.1 Сверить 15 sections `docs/foundation/architecture.md` с 15 replacement requirements; проверить `openspec validate migrate-foundation-profile-to-openspec --strict`.
- [x] 1.2 Сверить 24 foundation acceptance rows с archived crosswalk и 24 смысловыми qualification scenarios; проверить count и отсутствие `FND-AC-*` в candidate spec.
- [x] 1.3 Зафиксировать назначение fixture, checker и generated report как historical test assets; проверить `git diff --exit-code HEAD -- docs/foundation docs/evidence file-manifest.json docs/spec/file-manifest.json`.

## 2. Постоянный источник и переключение ownership

- [x] 2.1 Синхронизировать candidate `foundation-profile` в `openspec/specs/foundation-profile/spec.md`; проверить совпадение requirements, strict validation и отсутствие legacy IDs в permanent docs.
- [x] 2.2 Переключить только foundation row в `openspec/SOURCE-OF-TRUTH.md`; проверить, что legacy architecture, fixture, checker и report остались byte-identical.

## 3. Архив и финальные проверки change

- [x] 3.1 Архивировать change вместе с exact crosswalk; выполнить `openspec validate --all --strict --archived`, `openspec validate --specs --strict` и `git diff --check`.
- [x] 3.2 Проверить защищённые code, schemas, historical evidence и manifests через `git diff --exit-code HEAD -- internal schemas docs/evidence file-manifest.json docs/spec/file-manifest.json` перед коммитом.
