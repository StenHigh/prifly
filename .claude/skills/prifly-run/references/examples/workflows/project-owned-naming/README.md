# Входы именования проекта

Project profile `/5` отделяет metadata потребителя от закрытой task schema upstream.
Summary/start `/8`, authority claims `/5` и claim view `/4` поддерживают snapshot до 128 KiB.
[Author reference](../../authoring/project-profile-authoring-reference.yaml) показывает declaration, binding, schema и value.

1. Соберите Core или выберите точный released executable.
2. Выполните generic probe с абсолютным путём binary.
3. Добавьте `--official` для pinned consumer journey `aif-classic v1.57.0`.
4. Сохраните JSON evidence рядом с release evidence выбранного commit.

```
exact binary -> temporary consumer -> prepare -> start
                                         |        |
                                    stale check   synthetic reports
                                                  |
                                            resume -> cleanup
```

```sh
go build -o /tmp/prifly-issue19 ./cmd/prifly
python3 examples/workflows/project-owned-naming/probe.py /tmp/prifly-issue19 \
  --evidence /tmp/issue19-generic.json
python3 examples/workflows/project-owned-naming/probe.py /absolute/path/to/released/prifly \
  --official --evidence /tmp/issue19-released.json
```

Generic probe использует native CLI и synthetic session reports. Он не выполняет продуктовую разработку и не вызывает LLM.
Он проверяет missing/invalid/stale paths до claim, expected worktree, закрытый task input и неизменность upstream.
Resume сохраняет snapshot после удаления profile policy/schema/value file. Probe штатно завершает либо отменяет свои Runs и освобождает claims.

Official journey сначала устанавливает inert `composition-contracts 1.2.0` через documented native installer выбранного релиза.
Он создаёт inert host skill placeholders для compile; эти skills не исполняются.
Затем он устанавливает точный `v1.57.0` из `StenHigh/prifly-aif-workflows` через public CLI.
Он добавляет consumer profile `/5`, naming schema/value, `extend.yaml` и `project/consumer.txt`.
Затем он выполняет update, сравнивает upstream inventory и consumer bytes, выполняет prepare/start и отменяет тестовый Run.
Этот режим требует GitHub access; generic probe работает без сети.

Evidence содержит exact binary path/digest, version, commands, actual claim и upstream inventory.
Source-built success не закрывает released-Core gate. Поле `released_acceptance` всегда false:
владелец отдельно связывает evidence с опубликованным executable и точным release commit.
