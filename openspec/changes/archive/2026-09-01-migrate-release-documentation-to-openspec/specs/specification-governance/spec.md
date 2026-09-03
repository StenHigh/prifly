## MODIFIED Requirements

### Requirement: Исторические evidence сохраняют исходное содержание
OpenSpec migration MUST не переписывать historical release evidence и manifests
ради согласования с новой структурой. Пока legacy source set существует,
исторические файлы остаются проверяемой записью прошлого среза. После полного
replacement в OpenSpec отдельный final cleanup change MAY удалить legacy files
из release tree, сохранив их неизменённые bytes в Git history. Он MUST сначала
подтвердить, что карта источников указывает только migrated OpenSpec sources и
что relevant OpenSpec checks прошли.

#### Scenario: Перенос затрагивает материал с исторической ссылкой
- **WHEN** новый документ должен сослаться на материал, который присутствует в
  historical evidence до полного cutover
- **THEN** новая документация указывает актуальный путь без изменения
  исторического файла или его сохранённого отпечатка

#### Scenario: Финальный release cleanup удаляет historical evidence
- **WHEN** все replacement specifications существуют и final cleanup change
  готовится к первому релизу
- **THEN** он удаляет historical evidence без изменения его содержания в
  предыдущих commits и без изменения product semantics
