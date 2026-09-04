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
