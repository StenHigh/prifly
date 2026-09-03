## Context

См. мотивацию в [proposal.md](proposal.md). `docs/foundation/architecture.md`
смешивает профиль, границы будущих расширений, fixture и список будущей
приёмки. Рядом лежат JSON fixture, его Node checker и generated report.
Постоянная OpenSpec capability должна хранить нормативное поведение, а
historical test assets не должны становиться второй Markdown-спецификацией.

## Goals / Non-Goals

**Goals:**

- Создать один постоянный OpenSpec contract `foundation-profile`, покрывающий
  все 15 содержательных разделов legacy architecture и 24 foundation scenarios.
- Сохранить distinction между specified foundation qualification и фактически
  выполненным evidence.
- Оставить старые identifiers только в точной archived crosswalk и перевести
  ownership-карту после проверки покрытия.

**Non-Goals:**

- Не менять runtime, schemas, YAML, fixtures, checker, generated report или
  historical evidence в этой миграции.
- Не переносить JSON fixture и Node checker в OpenSpec: после переключения они
  остаются test assets, а их последующее физическое размещение решается общим
  final cleanup репозитория.
- Не объявлять `foundation-sequence/1` реализованным или квалифицированным.

## Decisions

### Один contract вместо копии legacy-архитектуры

Постоянный `openspec/specs/foundation-profile/spec.md` получает 15
тестируемых требований: границы профиля, допустимый workflow, frontend,
identities, snapshots, results, admission, stop, recovery, trust, telemetry,
compatibility, extension boundary, fixture scope и qualification catalog.
Он описывает поведение, а не повторяет реализационные варианты или план
roadmap. Другие OpenSpec capabilities остаются владельцами своих подробных
contracts; foundation profile фиксирует только согласованный узкий scope и
инварианты, которые этот scope обязан сохранить.

Альтернатива — оставить architecture.md самостоятельной «верхней»
спецификацией — отвергнута: она сохранила бы два изменяемых источника правила.

### Old IDs только в архивной трассировке

Постоянная спецификация и её scenarios используют смысловые названия без
`FND-AC-*`. В этом change design создаётся единственная точная таблица:

| Historical acceptance | Постоянный scenario |
|---|---|
| `FND-AC-01` | Независимые включения definition |
| `FND-AC-02` | Последовательность ожидает завершения процесса |
| `FND-AC-03` | Предметный отказ идёт только в разрешённый finish |
| `FND-AC-04` | Неподдержанная возможность отвергается заранее |
| `FND-AC-05` | Невалидный граф или binding отвергается заранее |
| `FND-AC-06` | Файлы не дрейфуют после start |
| `FND-AC-07` | Повтор команды сохраняет единственную работу |
| `FND-AC-08` | Конкурирующая мутация не создаёт частичный state |
| `FND-AC-09` | Stale UI не обходит stop |
| `FND-AC-10` | Граница dispatch и stop видна в журнале |
| `FND-AC-11` | Release и resume не снимают новый stop |
| `FND-AC-12` | Crash вокруг spawn не удваивает exec |
| `FND-AC-13` | Crash после commit не запускает шаг снова |
| `FND-AC-14` | Неполный success не становится завершением |
| `FND-AC-15` | Поздний result не переписывает принятый |
| `FND-AC-16` | Изменение Run version не отменяет ownership Attempt |
| `FND-AC-17` | Timeout не изображает terminal cancel |
| `FND-AC-18` | Повреждённый blob не даёт успеха |
| `FND-AC-19` | Replay не повторяет процессы |
| `FND-AC-20` | Exhaustion блокирует новый admission |
| `FND-AC-21` | Неизвестный profile отвергается до dispatch |
| `FND-AC-22` | Cancel проверяется на заявленной OS |
| `FND-AC-23` | Terminal Run не имеет обязательств |
| `FND-AC-24` | Foundation workflow независим от исходного проекта |

Это сознательное исключение из общей подсказки OpenSpec о сохранении IDs:
пользовательский policy Pri-Fly сильнее и требует оставлять legacy IDs только
в архивном crosswalk, а не в permanent specification.

### Coverage проверяется секциями, assets — назначением

Перед sync сравниваются следующие элементы:

| Legacy source | Replacement |
|---|---|
| §1–2 | Границы профиля |
| §3 | Workflow подмножество; YAML и JSON описывают одну модель |
| §4–6 | Definition/исполнение identities; snapshot; результат; admission |
| §7–8 | Durable recovery; stop; cooperative trust |
| §9–10 | Расширение профиля сохраняет основания модели |
| §11–12 | Совместимость; hooks и измерения |
| §13 | Foundation qualification содержит полный заявленный каталог |
| §14–15 | Явные границы scope и правило будущего расширения |
| `two-checks.workflow.json`, `verify-example.mjs` | Требование об ограниченном scope historical fixture; файлы не меняются |
| `verification.json` | Historical generated report; не переписывается и не становится нормативным source |

После этого permanent spec создаётся из delta, ownership row переключается на
`openspec/specs/foundation-profile/`, а legacy files остаются byte-identical
до общего финального cleanup.

### Квалификация не равна документальной проверке

`node docs/foundation/verify-example.mjs` в legacy tree допускается только как
проверка fixture. Его отчёт не используется для успешного закрытия product
gate и не перештамповывается. Для миграции достаточно strict OpenSpec
validation, coverage assertions и `git diff` защищённых historical paths.

## Risks / Trade-offs

- [Foundation пересекается с уже перенесёнными capabilities] → permanent
  contract фиксирует профиль и links, а не конкурирует с их подробной нормой.
- [Historical checker пишет report] → не запускать его в migration gate; report
  сохраняется неизменным.
- [Старые links остаются до final cleanup] → ownership меняется только в карте;
  исторические references не переписываются.

## Migration Plan

1. Проверить 15 requirements, 24 scenarios, section coverage и отсутствие
   FND identifiers вне this archived crosswalk.
2. Синхронизировать `foundation-profile` в permanent specs, не копируя туда
   legacy IDs, fixture или generated report.
3. Переключить только foundation row в `SOURCE-OF-TRUTH.md` после проверки
   bytes legacy source, evidence и manifests.
4. Архивировать change с этой crosswalk; выполнить strict OpenSpec validation.

Rollback до final cleanup — вернуть одну source-map row на `docs/foundation/`;
historical files при этом остаются на месте.
