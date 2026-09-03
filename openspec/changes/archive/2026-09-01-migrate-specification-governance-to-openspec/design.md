## Context

См. `proposal.md`. Последний partial source set содержит 1 331 строку
словаря, 222 строки development rules, 120 строк agent brief и 1 245 строк
журнала автономных решений. Product capabilities уже перенесены: этот change
не вправе повторять их требования или менять runtime. В финальном release
tree не останется legacy `docs/`, но Git и OpenSpec archive обязаны сохранить
исторические материалы.

## Goals / Non-Goals

**Goals:**

- Сделать `specification-governance` единственным current source set правил
  изменения спецификации и терминологии.
- Сохранить определения, границы терминов и механическую Go/JSON карту так,
  чтобы `TestGlossaryBindings` продолжил проверять тот же договор.
- Сохранить исторические rationale без превращения журнала в новую постоянную
  нормативную спецификацию.

**Non-Goals:**

- Не менять product semantics, JSON names, state versions, roadmap statuses,
  execution commands или qualified release evidence.
- Не переносить старые requirement/acceptance IDs в постоянные specs.
- Не сохранять временную ловушку конкретного worktree как общее правило
  продукта: она останется проверяемым историческим фактом archive.

## Decisions

### Словарь живёт рядом с governance spec как явная часть source set

`spec.md` остаётся стандартной OpenSpec specification с requirements и
scenarios. `terms.md` хранит полный читабельный канонический словарь, включая
перечисленную механическую карту Go/JSON. Он расположен в том же capability
directory, явно назван в requirement и source map; поэтому это не невидимый
соседний документ и не второй source of truth.

При переносе в `terms.md` остаются текущие определения и boundaries. Ссылки на
старые нормативные главы заменяются links на соответствующие OpenSpec
capabilities. История редакций не становится текущим договором: её coverage
остаётся в migration archive вместе с исходным файлом и Git history.

Альтернатива — развернуть тысячу строк терминов внутри `spec.md` — отвергнута:
она смешает нормативные requirements с навигационным справочником и ухудшит
проверку OpenSpec diff.

### Contributor rules переносятся по статусу, а не копируются целиком

Действующие правила об источнике правды, терминологии, совместимости,
честности evidence и границе document validation становятся requirements
governance. Командные рецепты, map путей ядра, generated schema procedure и
продуктовые facts уже принадлежат CLI, quality, architecture, published
contracts либо delivery capability; в новом governance source они заменяются
ссылкой, а не вторым пересказом.

Исторические и machine-specific инструкции (включая конкретный p206
worktree) переносятся в archive crosswalk как контекст, но не выдают за
действующее правило для любого contributor.

### Автономный журнал сохраняется архивом, ADR — отдельной capability

`docs/autonomous-decisions.md` переносится неизменённым в archive change как
исторический журнал. Его решения уже нашли постоянное место в перенесённых
capability specs либо остаются контекстом прошлого выбора. Для нового решения
используется active OpenSpec change; архитектурные решения по-прежнему живут в
`architecture-decisions`.

Это лучше, чем преобразовывать весь журнал в requirements: прошлые развилки
не должны искусственно стать действующими контрактами.

## Risks / Trade-offs

- [Термин потеряется при разбиении] → inventory сверяет каждое glossary
  heading, anchor и Go/JSON row с `terms.md` либо уже перенесённой capability.
- [Словарь станет вторым незаметным документом] → `spec.md` и source map
  называют exact path частью одного source set.
- [Исторический журнал будет принят за текущую норму] → он остаётся только в
  archive и получает явное историческое обозначение.
- [Удаление legacy сломает test или link] → до final cleanup изменить только
  ссылки/paths, затем проверить glossary bindings, all OpenSpec specs и поиск
  legacy `docs/` links.

## Migration Plan

1. Инвентаризировать headings и bindings четырёх legacy files, создать
   `terms.md`, permanent spec additions и archived crosswalk без legacy IDs в
   permanent source.
2. Перенаправить `TestGlossaryBindings` и repository entry pointers на
   OpenSpec, не меняя проверяемую Go/JSON карту.
3. Переключить строку `specification-governance` в source map на completed и
   архивировать focused change; legacy files до final cleanup не переписывать.
4. Выполнить final cleanup только в parent change: удалить legacy documents,
   historical evidence/manifests и custom document tooling после отдельной
   link-and-gate проверки. Rollback до cleanup — возврат source map на прежний
   source set; после cleanup восстановление возможно из Git history.
