# Проверки Pri-Fly

Go unit и integration tests лежат рядом с пакетом в `internal/`. Здесь —
внешние проверки, которые запускают собранный CLI и локальные fixture-процессы.
Они не являются пользовательскими сценариями и не используют credentials,
AI Factory или сеть.

| Путь | Назначение |
|---|---|
| `e2e/verify-install.sh` | bootstrap installer против локального стенда release: архив с совпавшим SHA-256 ставится, подменённый отклоняется и binary не появляется |
| `e2e/test_examples.py` | контракт foundation wrappers и локальных editor schemas |
| `e2e/verify-authoring.py` | независимый black-box corpus Project YAML authoring, public workspace launch и установка workflow folder из локального Git-репозитория и каталога |
| `e2e/verify-cli.py` | F1 workflow через настоящий binary |
| `e2e/verify-core.py` | реализованные Core operators |
| `e2e/verify-context.py` | context и обязательные checks |
| `fixtures/` | локальные workers только для проверок |
| `fixtures/foundation/` | legacy F1 generator и Python/shell workers для проверок compatibility |

```sh
make e2e
```

Старый `make examples` оставлен как совместимый alias. Пользовательские
материалы находятся в [`examples/`](../examples/README.md).
Python и shell files в `fixtures/foundation/` не описывают workflow и не нужны
автору YAML-сценария.

`fixtures/project-authoring/` содержит реальные `.prifly` YAML sources и
ожидания для одного accepted folder и legacy rejection boundaries.
`verify-authoring.py` только копирует case и вызывает public CLI; он не
использует Go parser, сеть, AI Factory или execution worker.
`fixtures/project-launch/` содержит отдельный public `project start` fixture:
оба workspace режима доходят до honest host handoff, а dirty checkout получает
отказ до прямой работы в repository.

`fixtures/project-launch-advice/` — local-only setup для отдельных живых host
observations. Он создаёт общий /3 профиль с JSON/blob inputs и вариантами списка;
не запускает модели и не входит в release e2e. Native Codex/Claude диалог
проверяется в реальном host и фиксируется отдельно от Go assertions и setup.
Подробности — [fixture guide](fixtures/project-launch-advice/README.md).

## Pri-Fly Light

Точечные Go проверки: `go test ./internal/local ./internal/runtime ./cmd/prifly -run '^TestLight'`.
Native queue continuation также покрывает `TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory`.
`python3 -B scripts/test_light_projection.py` проверяет pinned consumer bytes и exact-ref resolver.

Воспроизводимый local browser/performance replay (Playwright/Chromium должны уже быть установлены):

```sh
make build
mkdir -p .cache/light-replay/authorities
PRIFLY_LIGHT_PERF_FIXTURE_DIR="$PWD/.cache/light-replay/authorities" \
  .tools/go/bin/go test ./internal/runtime -run '^TestLightExportPerformanceFixtures$|^TestLightNativeTypedReviewRun$|^TestLightExportLongHistoryFixture$' -count=1
PRIFLY_LIGHT_FIXTURE_DIR="$PWD/.cache/light-replay/authorities" \
  .tools/go/bin/go test ./cmd/prifly -run '^TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory$' -count=1
python3 test/e2e/verify-light.py --binary bin/prifly --target .cache/light-replay \
  --playwright-module /path/to/node_modules/playwright --chromium /path/to/chrome
```

Fixture queue bootstrap проходит штатный native validator/authority transaction;
обычный queue CLI проверяется отдельно. Стенд содержит 1000 Runs и 1001 задач,
реальную continuation chain и принятые через session protocol typed report/gate.
Результат: screenshots, browser requests/metrics и runtime decode/read counters в
целевом каталоге. Это local engineering evidence, не released/production gate.

Для phase replay shipped aif-feature используйте неизменённый каталог package из
consumer SHA `3499115956903abd264fbde2be2f432d07bc2c43` (см. manifest fixtures):

```sh
PRIFLY_LIGHT_FEATURE_SOURCE=/path/to/immutable/aif-feature \
PRIFLY_LIGHT_PERF_FIXTURE_DIR="$PWD/.cache/light-replay/authorities" \
  .tools/go/bin/go test ./cmd/prifly -run '^TestLightShippedFeaturePhaseFixture$' -count=1
```

Он компилирует копию package в `/3`, записывает настоящий Run без Drive/LLM и
сопоставляет sealed refs через compiler provenance. Browser runner автоматически
добавляет эту authority и exact mapping, если они экспортированы. Для повторной
генерации очистите только ранее экспортированные fixture-каталоги: resolver
намеренно не перезаписывает существующий mapping.
