## Context

См. мотивацию в `proposal.md`. Pri-Fly уже имеет нормативные главы,
roadmap, contracts и неизменяемые evidence. Одновременный перенос всего
содержания сделал бы diff огромным и смешал бы миграцию структуры с изменением
смысла.

## Goals / Non-Goals

**Goals:**

- Внести OpenSpec как стандартную, version-controlled рабочую среду для
  изменений ТЗ.
- Установить одно правило ownership для каждой capability.
- Начать migration с малой capability о самом управлении спецификацией.

**Non-Goals:**

- Не менять поведение Pri-Fly, API, YAML authoring или sealed package.
- Не переносить полный `SPECIFICATION.md` либо исторические evidence одним
  срезом.
- Не объявлять OpenSpec зависимостью готового binary.

## Decisions

### Использовать штатную схему `spec-driven`

OpenSpec configuration и обычные Markdown artifacts остаются в репозитории.
Это даёт знакомый для агентов lifecycle change без нового внутреннего формата.
Кастомная схема не нужна: она добавила бы собственную систему поверх стандарта
до появления реальной потребности.

### Переносить capability по одной

Первая baseline specification описывает governance и правила cutover. Для
каждой продуктовой capability будущий OpenSpec change сначала фиксирует её
границы и current source, затем переносит содержание, links и нужные checks.
Пока это не сделано, старый документ остаётся единственным нормативным
источником.

Альтернатива — скопировать все главы в `openspec/specs/` сейчас — отвергнута:
она немедленно создаст две версии одних требований.

### Сохранять историю отдельно от источников

`docs/evidence/**`, historical manifests и release evidence не являются
редактируемым источником новой семантики. Новые документы ссылаются на
актуальное место, а карта relocation объясняет прошлые пути. Это сохраняет
доказательную ценность старых срезов.

## Risks / Trade-offs

- [Переходный период содержит два дерева документов] → карта ownership
  запрещает считать оба источниками одной capability.
- [Локальная CLI нужна участнику] → стартовые документы называют закреплённую
  версию и однострочную установку; binary Pri-Fly от неё не зависит.
- [OpenSpec по умолчанию разделяет proposal и apply] → change artifacts и
  tasks остаются в репозитории, поэтому работа может быть продолжена другим
  агентом без потери контекста.

## Migration Plan

1. Добавить OpenSpec configuration и его Codex skills в repository.
2. Внести capability `specification-governance` и применить её как baseline.
3. Обновить стартовые документы и roadmap без изменения historical evidence.
4. Проверить OpenSpec change, существующие проверки документации и отсутствие
   diff в evidence/manifests.
5. Архивировать этот change: baseline specification станет частью
   `openspec/specs/`, а plan останется в `openspec/changes/archive/`.
