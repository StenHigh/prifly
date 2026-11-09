# Приёмка Core issue #22

Проверяется synthetic task; private input, private authority и LLM не используются.
Изменение: `add-project-owned-compile-parameters`.

| Критерий | Проверка | Наблюдаемый результат |
|---|---|---|
| C1 | `TestProjectCompileParameterPrepareStartUnicode` | Default 12000 принимает 12000; 12001 получает отказ. Owner 32000 принимает 16557 и 32000; 32001 получает отказ. |
| C2 | `TestProjectCompileParameterRefusals`, Unicode lifecycle | Неверные типы, дробь, отрицательное/нулевое значение и превышение диапазона отвергаются до Run. Кириллица и emoji считаются Unicode code points. |
| C3 | Unicode lifecycle, prepare и start | `schema_invalid` содержит `/description`, effective limit и actual length. |
| C4 | Unicode lifecycle | Root передаёт полный task child workflow; child передаёт его project-owned worker. Worker получает полное JSON-значение и завершает Run принятым report. |
| C5 | `TestProjectCompileParameterStaleReview` | Изменение 32000 → 40000 меняет build key и review digest; старый digest получает `project_start_stale_launch` без Run. |
| C6 | `TestProjectCompileParameterSharedAuthorityAndPinnedRun` | Два проекта с одной author identity используют 32000 и 40000 в общей authority без collision/definition drift. |
| C7 | Shared authority/pinned Run; `TestProjectCompileParameterUpdateKeepsOwnerValues` | Прежние definitions, lock и task bytes остаются прежними. Update сохраняет owner file. Новый диапазон 1..20000 отвергает оставшееся 32000. Declared continuation с более узкой схемой отклоняет carried task до нового Run и effects. |
| C8 | Refusals; `TestProjectCompileParameterConsumerBindingMismatch`; `TestProjectCompileParameterProtectsInstanceDataAndLegacy` | Неизвестный параметр, чужая schema, отсутствующий pointer, несовместимый consumer binding, instance-data target, symlink и parameter flag conflict получают явный отказ. Project-owned worker — положительный consumer. |

`TestProjectCompileParameterCompositionScope` проверяет folder /2 и /3: одинаковые имена параметров двух source owners дают независимые 32000 и 40000. Source inventory включает оба sidecar.
`TestProjectUnifiedPublishedFourBytes` проверяет прежние exact compiled bytes без opt-in sidecars.

Команда точечной проверки:

```sh
GOTOOLCHAIN=local GOTELEMETRY=off GOCACHE="$PWD/.cache/go-build" \
  GOMODCACHE="$PWD/.cache/go-mod" .tools/go/bin/go test ./cmd/prifly \
  -run '^TestProjectCompileParameter|^TestNeutralAuthoringReferences|^TestProjectUnifiedPublishedFourBytes' -count=1
```

Результат точечной проверки: PASS. Первоначальный regression check падал с `project_compile_unresolved_values`; тот же набор после реализации проходит.
OpenSpec validation: `openspec validate add-project-owned-compile-parameters --strict` — PASS.
Полный `go test -timeout 20m ./...` проходит после добавления operational capability route. `vet` читает Linux и macOS; fmt/refusal guards проходят.
Для полных ворот используйте установленный Go 1.27.3 также в PATH дочерних анализаторов:

```sh
PATH="$PWD/.tools/go/bin:$PATH" make ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1
```

Без этого PATH базовый Go 1.27.2 при `GOTOOLCHAIN=local` останавливает staticcheck до анализа проекта. Source files не требуют исправления toolchain version.
Локальный helper исполняет полный batch; unsupported symlink в tool context отключает reuse. Итоговые логи находятся в `.cache/verification/`; это не released-binary qualification.

Wire: новые authoring sidecars `/1`; Project profile `/3`–`/5`; существующие folder/extension markers, compiled b1/b2 и Run wire contracts сохраняются.
Сводка запуска связывает effective contract через exact package ref и build key. Отдельных полей в прежние closed DTO изменение не добавляет.

Проверки подтверждают source implementation. Released-binary qualification и downstream classic/profiled A1–A10 не заявляются. Workflows issue #3 сохраняет отдельный статус.

## Итог публикации

Реализация опубликована в [bb2dbad5](https://github.com/StenHigh/prifly/commit/bb2dbad52e18cf614ad6d4efb967161ef77f280a).
Полные локальные ворота с Go 1.27.3 в PATH завершились exit 0 на staged bytes этой реализации; все modules verified.
[Issue #22](https://github.com/StenHigh/prifly/issues/22) закрыт как completed после публикации решения.
[Комментарий с evidence](https://github.com/StenHigh/prifly/issues/22#issuecomment-6753001335) сохраняет границу source acceptance и downstream qualification.
