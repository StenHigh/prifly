## 1. Инвентаризация delivery source set

- [x] 1.1 Зафиксировать в `archived-crosswalk.md` все 9 P1 и 18 P2 milestones из `roadmap.md`, их scope, prerequisites, gates и permanent sections; проверить count 27 и отсутствие duplicate/missing milestone.
- [x] 1.2 Инвентаризировать DCL/OSS priorities, RC scope, future workflow catalogue и first-build dependencies с exact source headings; проверить, что каждый record классифицирован как current delivery или historical Git-only.
- [x] 1.3 Инвентаризировать headings и dated status claims `f2-progress.md`/`release.md`; сохранить current snapshot отдельно, а history — с exact path/heading/date в archive; проверить, что historical `passed` не меняет permanent qualification claim.

## 2. Постоянный delivery contract

- [x] 2.1 Расширить candidate до полной P1/P2 milestone matrix и обязательств этапов; проверить 27 readable milestones, phase/gate links и отсутствие legacy authoring IDs в candidate.
- [x] 2.2 Добавить current RC scope, active priority, future queue и first-build inventory с explicit exclusions; проверить, что status/evidence language совпадает с source и не выдаёт narrow RC за full release.
- [x] 2.3 Сверить current snapshot, archive inventory и candidate; выполнить `openspec validate migrate-delivery-roadmap-to-openspec --strict`.

## 3. Переключение источника

- [x] 3.1 Синхронизировать `delivery-roadmap` в permanent specs и проверить strict validation, current snapshot date и отсутствие legacy IDs вне archive.
- [x] 3.2 Переключить ровно одну строку `SOURCE-OF-TRUTH.md`; проверить, что legacy source set и derived roadmap maps остаются byte-identical до final cleanup.

## 4. Защита истории и архивирование

- [x] 4.1 Проверить `git diff --exit-code HEAD -- docs/roadmap docs/f2-progress.md docs/release.md docs/rc-scope.md docs/beyond-phase-two.md docs/dependencies.md docs/evidence file-manifest.json docs/spec/file-manifest.json internal schemas` и `git diff --check`.
- [x] 4.2 Архивировать change после sync; выполнить `openspec validate --specs --strict` и `openspec validate --all --strict --archived`, не выдавая document migration за product qualification.
