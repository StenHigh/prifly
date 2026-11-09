## ADDED Requirements

### Requirement: Параметр сборки отличается от настройки входа
Словарь SHALL определять Compile parameter как package-declared типизированное значение compiler с default, диапазоном и точными schema bindings. Project-owned compile value SHALL означать сохранённый выбор проекта для такого параметра. Эти понятия MUST отличаться от Project workflow setting, который меняет default configuration input. Новая терминология MUST сопровождаться исходниками, примерами и проверками. Source set остаётся `specification-governance`; Go/JSON bindings добавляются только для реализованных типов.

#### Scenario: Автор выбирает изменение schema limit
- **WHEN** автору нужен configurable maxLength до task validation
- **THEN** справочник направляет к compile parameter и не обещает такую возможность через runtime configuration input
