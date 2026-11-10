# Core #22: опубликованный v0.13.90

[Core v0.13.90](https://github.com/StenHigh/prifly/releases/tag/v0.13.90) опубликован и проверен.
Release SHA: `5e7f81cf5e6b53b72b86a937584e18f699f90ff9`.
[Итоговый комментарий](https://github.com/StenHigh/prifly/issues/22#issuecomment-6754299052) фиксирует закрытие #22 с причиной `completed`.

```text
исходники C1–C8 -> exact-SHA verify/qualify -> tag
  -> Linux/native macOS -> owner approval -> signed assets -> published Linux probe
```

## Exact-SHA gates и публикация

| Gate | Результат на release SHA |
|---|---|
| [verify](https://github.com/StenHigh/prifly/actions/runs/42716142852) | PASS: тесты и статические проверки обеих платформ. |
| [qualify](https://github.com/StenHigh/prifly/actions/runs/42716176781) | PASS: race и полный e2e. |
| [release](https://github.com/StenHigh/prifly/actions/runs/42719875397) | PASS: Linux, нативный macOS и публикация после owner approval. |

[Exact-SHA log](exact-sha-gates.jsonl), [tag dry-run](tag-dry-run.txt) и [создание тега](tag-release.txt) сохранены без изменения.
[Owner approval](owner-approval.json) относится только к этому release SHA.
[Release binding](release.json) сохраняет version, SHA, workflow URLs, deployment и результаты проверки поставки.

Первый кандидат `564e4a4e12acbe07ade1261b651372d7e9937cb6` получил отказ e2e из-за отсутствующих новых схем в inventory.
Его race прошёл; весь qualify завершился с отказом.
Commit `5e7f81cf` добавил title новым схемам, inventory и modelines.
Для нового SHA прошли собственные verify и qualify; успех прежнего кандидата не переносился.

## Проверка поставки

Все шесть assets опубликованы.
Обе Ed25519-подписи manifest проверены repository public key.
SHA-256 обоих архивов совпали с подписанным manifest.
Официальный installer установил Linux binary в отдельный временный каталог.
Установленные bytes совпали с binary внутри опубликованного архива.

| Asset | SHA-256 |
|---|---|
| Linux archive | `1a755a7a1134b71a484bedfc347686908eaccd25a81d85d808beddff5da03546` |
| Darwin archive | `0b59dbeeea5bd93d88a48043d435d520fc4cd1fb8ee51385eaf183aac836be9a` |
| Проверенный Linux binary | `db8a158149088e6f9bfb9df3ebe5fc97e1b57942a620c78b65841f5c996bd984` |

Raw [asset verification](asset-verification.json), [manifest](release-manifest.json), [legacy signature](release-manifest.sig) и [JCS signature](release-manifest.jcs.sig) сохранены.
Сохранённый [публичный ключ](release-public-key.hex) повторно проверяет обе подписи без зависимости от будущей ротации repository variable.
Darwin binary прошёл нативные build, minOS и doctor в release workflow.
Локальный probe выполнялся на Linux AMD64.

## Граница приёмки

Полная [приёмка C1–C8 на исходниках](../source-acceptance.md) сохраняет свой исходный checkpoint.
Этот документ добавляет последующую проверку опубликованной поставки.
Он не переписывает прежнее evidence и не объявляет downstream A1–A10 выполненными.

| Проверка опубликованного Linux binary | Результат |
|---|---|
| Версия, capability и две editor schemas | PASS: `0.13.90`, `project_compile_parameters`, непустые title и `$id`. |
| Default и owner | PASS: пределы `12000` и `32000`, разные build keys. |
| Полный task через root/child/worker | PASS: `16557` Unicode code points, принятый synthetic report, Run `completed/succeeded`. |
| Границы и диагностика | PASS: `32000` принимается; `32001` отклоняется в prepare/start с pointer и обеими длинами. |
| Review | PASS: изменение `32000` → `40000` меняет build/review; старый digest получает отказ. |
| Недопустимое owner value | PASS: `1000001` получает типизированный отказ до Run. |

[Raw probe result](probe-result.json) и [исполненный probe](probe.py) сохранены без изменения.
Абсолютный binary path в raw result относится к машине приёмки; для повтора передайте свой путь.
Shared authority, update, pinned Run, continuation и полный набор отказов подтверждены source C1–C8 и exact-SHA gates.
Проверки consumer classic/profiled остаются в [prifly-aif-workflows #3](https://github.com/StenHigh/prifly-aif-workflows/issues/3).

Новый [пример документации](../../../../../../examples/compile-parameters/README.md) проверен тем же опубликованным binary после выпуска.
Все три shell-блока README прошли; YAML-пакет прошёл шесть групп probe с отдельной временной authority.
[Результат и hashes файлов](../documentation-example.json) связывают эту проверку с файлами примера.

## Повторить probe

Команды выполняются из checkout, который содержит этот archived evidence.
Нужны Git, GitHub CLI, Python 3 и Linux AMD64.
Probe использует только стандартную библиотеку Python.
Fixture извлекается из Go test source закреплённого release; Go и LLM не запускаются.

```sh
ISSUE22_PROBE="$PWD/openspec/changes/archive/2026-11-10-add-project-owned-compile-parameters/evidence/released-v0.13.90/probe.py"
ISSUE22_REPLAY=$(mktemp -d)
gh release download v0.13.90 --repo StenHigh/prifly \
  --pattern prifly-linux-amd64.tar.gz --dir "$ISSUE22_REPLAY"
printf '%s  %s\n' \
  '1a755a7a1134b71a484bedfc347686908eaccd25a81d85d808beddff5da03546' \
  "$ISSUE22_REPLAY/prifly-linux-amd64.tar.gz" | sha256sum -c -
tar -xzf "$ISSUE22_REPLAY/prifly-linux-amd64.tar.gz" -C "$ISSUE22_REPLAY"
git clone --depth 1 --branch v0.13.90 https://github.com/StenHigh/prifly.git "$ISSUE22_REPLAY/source"
cd "$ISSUE22_REPLAY/source"
test "$(git rev-parse HEAD)" = 5e7f81cf5e6b53b72b86a937584e18f699f90ff9
python3 -B "$ISSUE22_PROBE" --binary "$ISSUE22_REPLAY/prifly" \
  --version 0.13.90 --output "$ISSUE22_REPLAY/probe-result.json"
```

Ожидаемый итог: `result: passed` и шесть групп проверок.
Probe создаёт отдельную временную authority и синтетические task; временный каталог удаляется после проверки.
Проверка подписей описана в [SECURITY.md](../../../../../../SECURITY.md#проверить-подпись-выпуска-вручную).
