# Архивная карта переноса governance

Это проверяемая запись cutover 2026-09-01. Она связывает legacy source set с
текущими OpenSpec источниками, не создавая новый формат идентификаторов в
постоянной спецификации. Исторические файлы ниже сохранены в этом change и
Git; они не являются действующей нормой.

## Инвентаризация

| Legacy source | Содержимое на cutover | Current OpenSpec replacement | Проверка |
|---|---|---|---|
| `docs/glossary.md` | 120 definition headings, 256 Go/JSON binding rows, одна marker pair | `openspec/specs/specification-governance/terms.md` | headings, binding rows и marker pair сопоставлены; `TestGlossaryBindings` читает новый путь |
| `docs/glossary.md`, «История словаря» | 33 записи редакций, включая прежние внутренние обозначения | `historical-glossary-history.md` | отдельный archive материал; не входит в current terms |
| `docs/development.md`, «Изменение спецификации» и «Термины и именование» | source map, OpenSpec workflow, compatibility и граница механической проверки словаря | `specification-governance/spec.md`, `terms.md`, `agent-brief.md`, `AGENTS.md` | current pointers не называют legacy source нормой |
| `docs/development.md`, «Команды», «Структура ядра», «Версии и generated files» | команды, ownership кода и public contract process | `cli-protocol`, `quality-and-acceptance`, `architecture-decisions`, `published-contracts` | capability specs уже перенесены; полный исходник сохранён как `historical-development.md` |
| `docs/development.md`, «Факты, выявленные в работе» и «Документы и evidence» | historical investigations и правила честного evidence | `specification-governance/spec.md`; `historical-development.md` | current requirement запрещает выдавать document check за product gate |
| `docs/agent-brief.md` | порядок чтения, границы фактов, правила среза, запреты и язык общения | `openspec/specs/specification-governance/agent-brief.md` | current brief не ссылается на удаляемый `docs/` source; прежний текст сохранён как `historical-agent-brief.md` |
| `docs/autonomous-decisions.md` | 65 рабочих развилок с context, decision, rejected alternative и review condition | `historical-autonomous-decisions.md`; постоянные architectural rules — `architecture-decisions` | exact archive copy; журнал не объявлен current source |

## Границы

- Public JSON Schema `$id`, names CLI, storage/profile versions и Go/JSON
  bindings не были переименованы.
- `docs/evidence/**`, root `file-manifest.json` и
  `docs/spec/file-manifest.json` не перенесены и не переписаны: это historical
  records, которые final cleanup удалит только из release tree.
- Ссылки в current `terms.md` ведут на capability specs или runtime artifacts;
  они не используют удаляемый `docs/` source как норму.
