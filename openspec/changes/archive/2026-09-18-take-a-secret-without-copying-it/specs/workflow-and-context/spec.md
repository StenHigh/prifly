## MODIFIED Requirements

### Requirement: Dynamic context, limits и secrets остаются explicit
Additional reading/search MUST be a declared bounded tool operation with
provenance. Context bytes, refs and token budget MUST be independently finite;
overflow MUST use declared split/summary/refusal, never silent truncation.
Secrets MUST use restricted runtime channels; redacted material MUST retain
its own digest and MUST not remove evidence while claiming full proof.
Machine-local объявление окружения объявленной программы MUST допускать
источник значения вместо самого значения: переменную окружения вызывающего,
файл на этой машине целиком или один названный ключ такого файла. Runtime MUST разрешать источник непосредственно
перед запуском программы, MUST отказывать именованно и до запуска, если
источник отсутствует или пуст, и MUST NOT печатать разрешённое значение в
итогах, задачах, диагностиках или журналах. Литеральная форма сохраняется для
значений, которые секретом не являются.

#### Scenario: Required source does not fit context budget
- **WHEN** renderer exceeds a mandatory limit
- **THEN** Run follows declared overflow handling or refuses before dispatch

#### Scenario: Пароль не копируется в конфигурацию
- **WHEN** объявленная программа требует переменную с секретом, а владелец
  назвал источником переменную окружения своей машины или файл
- **THEN** программа получает значение при запуске, конфигурация его не
  содержит, и ни один вывод его не печатает

#### Scenario: Источник отсутствует в момент запуска
- **WHEN** названный источник пуст или отсутствует
- **THEN** запуск отказывает до старта программы, называя переменную и место,
  где её ждали, а не оставляет программу падать на аутентификации
