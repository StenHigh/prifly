## 1. Архивируемая трассировка

- [x] 1.1 Создать `archived-crosswalk.md` с 44 exact legacy rule IDs, понятными permanent requirement headings и foundation/completion stage; проверить counts 24/6/14 и отсутствие пропусков/duplicates.
- [x] 1.2 Добавить в crosswalk 88 exact acceptance IDs, titles, owner stages, requirement links и permanent subject headings; проверить partition 45 OBS, 14 PUB, 29 REA.
- [x] 1.3 Сохранить в change byte-identical extract 88 строк `docs/roadmap/acceptance-map.csv`; проверить content digest либо `cmp` с фильтром исходной карты и не трактовать строки как executed evidence.

## 2. Полный contract candidate

- [x] 2.1 Раскрыть 24 semantic observability requirements и привести их scenarios к 45 individual Given/When/Then cases из crosswalk; проверить exact title coverage и отсутствие `OBS-[0-9]` в candidate.
- [x] 2.2 Раскрыть 6 publication requirements и привести их scenarios к 14 individual Given/When/Then cases из crosswalk; проверить exact title coverage и отсутствие `PUB-[0-9]` в candidate.
- [x] 2.3 Раскрыть 14 reaction requirements и привести их scenarios к 29 individual Given/When/Then cases из crosswalk; проверить exact title coverage и отсутствие `REA-[0-9]` в candidate.
- [x] 2.4 Сверить ровно 44 requirements и 88 scenarios с crosswalk; проверить `openspec validate migrate-observability-publication-reactions-to-openspec --strict`.

## 3. Переключение единственного источника

- [x] 3.1 Синхронизировать candidate в `openspec/specs/observability-publication-reactions/spec.md`; проверить 44 requirements, 88 scenarios, strict validation и отсутствие legacy IDs в permanent spec.
- [x] 3.2 Разделить ownership-карту: добавить отдельную migrated row для `state-and-telemetry.md` и исключить его из delivery source set; проверить, что до final cleanup legacy source остаётся current historical copy без изменений bytes.

## 4. Защита истории и архивирование

- [x] 4.1 Проверить защищённые source/evidence/contracts командой `git diff --exit-code HEAD -- docs/roadmap/state-and-telemetry.md docs/evidence file-manifest.json docs/spec/file-manifest.json internal schemas` и `git diff --check`.
- [x] 4.2 Архивировать change после sync; выполнить `openspec validate --specs --strict`, `openspec validate --all --strict --archived` и подтвердить, что новая capability не меняет runtime status или F2 qualification.
