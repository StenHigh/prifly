## Context

См. `proposal.md`. После завершения document cutover roadmap всё ещё называет
совместимость со старыми authoring forms, хотя Pri-Fly не выпускался публично.
В repository нет GitLab CI; действующие product gates определены Makefile.

## Goals / Non-Goals

**Goals:**

- Сделать порядок четырёх contributor-ready changes недвусмысленным.
- Убрать дорелизную совместимость authoring из committed очереди.
- Не допустить повторного появления document-only проверок в CI.

**Non-Goals:**

- Не реализовывать CI, YAML cleanup, corpus или editor integration в этом
  change.
- Не менять P1/P2 milestones, runtime, sealed packages или versioned contracts.

## Decisions

### Стабилизировать authoring до независимого corpus и editor support

Сначала CI защищает уже существующий RC path. Затем repository оставляет одну
YAML форму; только после этого corpus и editor contract закрепляют её для
внешних contributor. Иначе два последующих слоя должны будут тестировать и
объяснять форму, подлежащую удалению.

Альтернатива — сохранить legacy compatibility до первого внешнего пользователя
— отклонена: публичного contract ещё нет, а удержание двух путей увеличивает
матрицу проверок без защищаемого пользователя.

### CI использует продуктовые, а не исторические document gates

`make check` и `make e2e` уже выражают current Go, schema и end-to-end
границы. Historical document checks были удалены вместе с предыдущим source
set и не могут быть условием современного CI.

## Risks / Trade-offs

- [Удаление authoring forms затронет локальный незакреплённый prototype] → до
  начала cleanup change явно перечисляет формы и проверяет отсутствие public
  release compatibility obligation.
- [Corpus начнут до стабилизации authoring] → roadmap делает YAML-only cleanup
  обязательным предшественником.
- [CI станет заменой product qualification] → roadmap прямо сохраняет границу
  между зелёным pipeline и P1/P2 qualification.

## Migration Plan

1. Принять этот порядок в delivery roadmap.
2. Создать и завершить GitLab CI change.
3. Создать и завершить YAML-only authoring change.
4. Создать и завершить validator corpus change, затем editor contract change.
