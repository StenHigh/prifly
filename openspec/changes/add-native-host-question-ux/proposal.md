## Why

`prifly-run` знает, когда нужно остановиться ради решения разработчика, но
сегодня описывает это обычным текстовым вопросом. Codex и Claude Code уже
предоставляют нативный интерфейс выбора; использование его для конечных
вариантов делает запуск понятнее и исключает неявный выбор.

## What Changes

- Генерировать две версии host runner: одну для Codex CLI и Codex app, вторую
  для Claude Code; обе сохраняют свой fixed host ID.
- Предписать runner использовать нативный question tool host для конечных
  решений, чьи варианты он может назвать без догадки: launch, режим Workspace
  и approve/reject. Если tool
  отсутствует, runner ждёт явный текстовый ответ и не выполняет mutation.
- Оставить RunBrief, file path и произвольные JSON/text inputs обычным вводом:
  они не являются набором заранее известных вариантов.
- Описать paging, когда вариантов больше лимита host UI, и запретить скрывать
  варианты или подменять ответ default.
- Сохранить safe migration: existing `prifly-run` не перезаписывается; команды
  обновляют tracked runner отдельным reviewed commit.

Изменение касается только generated host instructions и их проверки. Оно не
меняет Pri-Fly runtime, YAML authoring, command DTO, sealed package или
сохранённые Run.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `cli-protocol`: Project host entry point должен использовать native question
  UI для конечных developer decisions, когда host предоставляет такой tool.
- `workflow-and-context`: interactive Workspace decision и pinned host runner
  должны сохранять explicit ответ без default и без скрытия вариантов.

Ни у одной capability не меняется нормативный source ownership: обе уже
перенесены в соответствующие OpenSpec specs.

## Impact

- `cmd/prifly/project.go`: две генерируемые runner templates и exact runner
  validation.
- `cmd/prifly/main_test.go`: generation и upgrade-safety tests.
- Новые Project profile получают обновлённые runners; существующие profile
  обновляются только явным reviewed изменением в их repository.
