## 1. OpenSpec baseline

- [x] 1.1 Уточнить `openspec/config.yaml` правилами Pri-Fly и проверить, что
  `openspec status` видит local repository configuration.
- [x] 1.2 Добавить карту источников и перехода в `openspec/`; проверить, что
  она однозначно называет current source для каждой начальной группы
  capabilities.
- [x] 1.3 Применить baseline `specification-governance` и проверить, что после
  archive её requirements находятся в `openspec/specs/`.

## 2. Входные документы и roadmap

- [x] 2.1 Обновить `AGENTS.md`, `README.md` и `docs/agent-brief.md` с
  OpenSpec-first маршрутом для новых изменений; проверить ссылки и отсутствие
  заявления, что OpenSpec нужен runtime.
- [x] 2.2 Добавить в `docs/roadmap/roadmap.md` высокий приоритет поэтапной
  миграции спецификации и обновить связанный progress record; проверить
  roadmap verifier.

## 3. Проверка и сохранение истории

- [x] 3.1 Проверить OpenSpec change в strict mode и существующие проверки
  документации, затем зафиксировать результаты в progress record.
- [x] 3.2 Убедиться через `git diff`, что `docs/evidence/**`,
  `file-manifest.json` и `docs/spec/file-manifest.json` не менялись.
