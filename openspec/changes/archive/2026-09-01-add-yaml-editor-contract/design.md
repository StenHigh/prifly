## Context

См. `proposal.md`. Source YAML уже строго проверяется compiler, но JSON Schema
для редактора не опубликована. Положить `$schema` как YAML field нельзя: это
изменило бы authoring bytes и потребовало бы нового parser contract.

## Goals / Non-Goals

**Goals:**

- Дать авторам локальные versioned schema и точные mappings для шести YAML
  document kinds.
- Сделать editor feedback доступным без сети и без зависимости от AI Factory
  или конкретного редактора.
- Сохранить compiler единственной semantic authority.

**Non-Goals:**

- Не добавлять runtime dependency, новый CLI command или `$schema` field в
  YAML.
- Не дублировать в JSON Schema вычисление exact refs, graph, permissions,
  bindings или sealing.
- Не копировать editor schema в `.prifly/` project profile.

## Decisions

### Comment-modeline вместо YAML property

Published references начинаются с comment-modeline, который понимает
yaml-language-server и совместимые редакторы. YAML parser игнорирует comment,
поэтому one source с modeline по-прежнему компилируется и seal-ится как тот же
document без modeline. Для других редакторов README даёт те же local path и
glob mapping без привязки Pri-Fly к их configuration format.

Альтернатива — top-level `$schema` — отклонена: это стало бы authoring API и
изменяло бы value, которую принимает compiler.

### Малый static contract рядом с published schemas

`schemas/authoring/` содержит шесть versioned JSON Schema и manifest.
Schema ограничивает document kind, version marker, known top-level fields и
простые shapes, чтобы дать completion и early typo diagnostics. Вложенные
dynamic graph objects допускают дополнительные поля там, где их окончательно
разрешает только compiler.

Альтернатива — повторить весь semantic compiler в JSON Schema — отклонена:
она неизбежно расходилась бы с проверкой exact refs и graph semantics.

### Статическая проверка без нового validator dependency

Python stdlib test читает manifest и JSON documents, сверяет versioned IDs и
modelines в двух full references. Existing Go authoring tests продолжают
компилировать эти references, что доказывает отсутствие влияния comments на
lowering. Это проверяет published contract без third-party validator.

## Risks / Trade-offs

- [Editor поддерживает только proprietary mapping] → README сообщает glob и
  local schema path; source YAML всё равно переносим между редакторами.
- [JSON Schema пропустит semantic ошибку] → README прямо отправляет автора к
  `project compile`; compiler остаётся authoritative validation.
- [Schema отстанет от authoring fields] → manifest/reference test ловит
  сломанный published contract, а code review меняет schema вместе с parser.

## Migration Plan

1. Добавить local schemas, manifest, README и modelines в references.
2. Запустить static contract test, existing authoring tests и relevant gates.
3. Архивировать OpenSpec change; rollback — удалить только editor artifacts,
   так как runtime и sealed package не менялись.
