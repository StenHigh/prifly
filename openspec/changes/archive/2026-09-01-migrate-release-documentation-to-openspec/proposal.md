## Why

Pri-Fly сейчас хранит одну и ту же нормативную информацию в большом
`SPECIFICATION.md`, множестве custom Markdown/CSV/JSON documents и historical
evidence. Это усложняет работу человека и агента: неясно, какой файл править,
а собственные document builders поддерживают старую форму вместо продукта.

К первому релизу OpenSpec должен быть единственным источником нормативной
документации. Вне него остаётся только аккуратный GitHub README — вход в
продукт, а не вторая спецификация.

## What Changes

- Перенести нормативные capabilities Pri-Fly из `docs/` в
  `openspec/specs/`, сохранив их смысл, acceptance scenarios и ссылки на
  versioned contracts. Старые documentation IDs остаются только в archived
  migration crosswalk, а не в постоянных specs.
- Ввести release documentation layout: после полного переноса release tree не
  содержит `SPECIFICATION.md`, `docs/evidence/`, historical manifests,
  custom document builders и их reports.
- **BREAKING** Удалить старые documentation source sets только после того, как
  их OpenSpec replacements проверены и карта источников переключена.
- Оставить `README.md` короткой, красиво оформленной GitHub-витриной: описание
  продукта, быстрый старт и ссылки на OpenSpec; без дублирования норматива.
- Удалить необходимость в custom process для ведения документации; OpenSpec
  standard workflow становится единственным способом менять спецификацию.

## Capabilities

### New Capabilities

- `release-documentation-layout`: требования к составу repository перед первым
  релизом, включая OpenSpec-only нормативные документы и роль README.

### Modified Capabilities

- `specification-governance`: условия финального cutover и удаления legacy
  documentation source sets без переписывания их исторического содержания.

## Impact

Изменяется только documentation/process layout репозитория. Go runtime,
пользовательский YAML, sealed packages, JSON wire contracts и сохранённые Runs
не меняют поведение. Перенос затронет `docs/`, `SPECIFICATION.md`,
`tools/docs/`, root/`docs` manifests, entry-point links и custom document
checks; Git history сохранит удалённые historical files.
