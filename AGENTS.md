# Разработка Pri-Fly

- Перед изменением предметной модели, API или терминологии прочитай [словарь](openspec/specs/specification-governance/terms.md) и [brief для агента](openspec/specs/specification-governance/agent-brief.md).
- Используй канонические понятия словаря. Текущие краткие Go-имена — явные соответствия, не новые сущности; не вводи синонимы без отдельного смысла.
- Новое понятие или изменение значения/имени вноси в словарь вместе с затронутыми исходниками ТЗ, кодом, схемами, примерами и проверками. При противоречии сначала согласуй смысл, не выбирай удобную версию.
- Не переименовывай сохранённые JSON-поля и не меняй смысл закреплённых Runs ради стилистики. Учитывай версии контрактов и совместимость; не переписывай прошлые release evidence.
- Новая возможность движка доходит до автора сценария в том же изменении, что и
  сама возможность: строка в таблице `examples/README.md` («Что умеет движок и
  где это показано») и показанное поле в нужном справочнике `examples/authoring/`.
  `TestEveryDeclaredCapabilityIsInTheAuthorIndex` держит таблицу равной списку
  `prifly capabilities` в обе стороны и валит ворота, но он проверяет только
  наличие строки: что справочник действительно показывает поле и что указана
  минимальная ревизия или контракт шага — предмет ревью, не теста. Справочники
  успевали за контрактами, а страница, которая к ним ведёт, отстала на четыре
  выпуска: `technical_retries` не был показан нигде.
- Выполняй `TestGlossaryBindings` при изменении карты терминов; он входит в `make test` / `make check`. Этот тест проверяет указанные Go/JSON-соответствия, а не заменяет смысловое ревью.
- Отказ несёт свой код в типизированном `runtime.Fault`, а не в тексте ошибки. `make refusal-check` (входит в `make check` и `ci-check`) валит сборку, если код снова оказался внутри `errors.New`/`fmt.Errorf`; тесты строят текстовые ошибки намеренно, чтобы доказать, что такую ошибку всё ещё читают верно.
- Transform команды — чистая функция снимка и своего payload. Не читай файлы и blob'ы и не запускай процессы внутри write-транзакции: всё нужное вычисляй до применения команды. Guard `internal/purity` валит тест на нарушении.
- Драйвер хранения не протекает в runtime и CLI: `make vet` собирает их с `CGO_ENABLED=0`, поэтому пакет с типами driver в собственных контрактах перестанет проходить проверку.
- Фазы и приёмку веди по `openspec/specs/delivery-roadmap/`; наличие определения или пройденная проверка документа не означает реализацию F2 либо закрытие продуктового gate.
- Нормативное изменение начинай с OpenSpec change. Перед правкой сверяйся с [картой источников](openspec/SOURCE-OF-TRUTH.md): до явного переноса capability старый source set остаётся единственной правдой. OpenSpec управляет документацией Pri-Fly, а не входит в его runtime или YAML authoring contract.

## Структура репозитория

Карта для агента; подробности стека и границ — `.ai-factory/DESCRIPTION.md` и
`.ai-factory/ARCHITECTURE.md`, не дублировать их здесь.

```
cmd/prifly/        CLI, проектный профиль (project *), host runners, монитор
internal/flow/     версионная модель workflow, схемы, компиляция YAML
internal/runtime/  authority: admission, lifecycle, session protocol, claims
internal/local/    SQLite store, blob'ы, transform guard, процессы
internal/purity/   guard чистоты transform-команд
internal/release/  контракт публичной поставки
schemas/           опубликованные JSON Schema (core, foundation, authoring)
examples/          справочники YAML-авторинга и troubleshooting.md
test/              e2e и fixtures для собранного CLI
scripts/           установка, релиз, проверка схем
openspec/          спецификации, changes, карта источников
.prifly/           профиль проекта Pri-Fly: aif-classic установлен из каталога
.ai-factory/       контекст AI Factory: config, DESCRIPTION, ARCHITECTURE, RULES
```

| Файл | Назначение |
|---|---|
| `cmd/prifly/main.go` | диспетчер команд, Problem-конверт и exit-коды |
| `internal/flow/protocol.schema.json` | источник истины опубликованного контракта |
| `Makefile` | ворота: `check`, `ci-check`, `race`, `e2e`, `schemas-check` |
| `openspec/SOURCE-OF-TRUTH.md` | где сегодня меняется каждое правило |

AI-контекст: `AGENTS.md` (этот файл, Codex) и `CLAUDE.md` (Claude Code, импортирует
его), `.ai-factory/**` (AI Factory), `.claude/skills/prifly-run/PROJECT.md` и
`.agents/skills/prifly-run/PROJECT.md` (правила репозитория для runner'а).
Сценарий разработки — `prifly-run` → launch `aif-classic`.
