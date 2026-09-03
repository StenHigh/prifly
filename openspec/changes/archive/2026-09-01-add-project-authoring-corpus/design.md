## Context

См. `proposal.md`. Existing e2e checks уже используют собранный CLI и Python
standard library; project authoring пока подтверждается главным образом Go
tests. Corpus должен проверять внешний folder route, но не превращаться в
второй parser или пользовательский runtime.

## Goals / Non-Goals

**Goals:**

- Держать author-visible YAML cases в отдельных файлах.
- Проверять accepted folder и каждую удалённую legacy boundary через binary.
- Не требовать сеть, AI Factory, credentials или third-party Python package.

**Non-Goals:**

- Не создавать второй validator или новый authoring format.
- Не дублировать все unit cases либо запускать workflow worker.

## Decisions

### Fixture directory и ожидаемый JSON

Каждый case хранит repository source и маленький `expect.json`. YAML остаётся
предметом corpus, JSON expectation читается Python standard library без
зависимости от YAML parser. Имя case не несёт поведения.

Альтернатива — описывать fixtures в Python — отклонена: тогда author не видит
реальных source files и проверка перестаёт быть независимой.

### Проверка через public CLI

Verifier копирует case во временную directory, initialise local
`core-workflow/1` authority только для accepted compile, затем вызывает
`project workflows` или `project compile`. Он проверяет declared input list,
sealed manifest либо diagnostic и не импортирует package.

Альтернатива — вызвать internal Go parser — отклонена: она не доказала бы
contract CLI и folder discovery.

## Risks / Trade-offs

- [Diagnostic wording изменится без contract] → each negative case ожидает
  stable purpose substring, не полный error JSON.
- [Corpus станет вторым набором unit tests] → в нём остаются только route
  boundaries и один accepted round-trip.

## Migration Plan

1. Добавить files для accepted folder и legacy rejection boundaries.
2. Добавить verifier в current e2e sequence.
3. Запустить only corpus и `make e2e`; сохранить historical evidence без
   изменений.
