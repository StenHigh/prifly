# Проектное измерение внутри repeat

Пример показывает declared extension points. Он использует synthetic replies,
не требует LLM и не запускает прикладные тесты. Он не квалифицирует настоящий
provision, пригодность отчёта либо выпуск продукта.

Нужен binary с capability `project_extension_points`. Baseline v0.13.82 этой
возможности не имеет. До нового owner release проверка candidate не означает
приёмку released binary. Project profile — `/3` либо `/4`; extension — `/4`.
Native минимум: Call WorkflowRevision 1, Repeat 3, blocked 6, resumable 8.
Assisted external_write — StepDefinition 11. Старые сохранённые Runs не меняются.

```text
root produce -> cycle(repeat) -----------------------> done
                  |
round input -> measure-before-read -> read -> clean
                                      |
                                    repair
                                      |
                 fixed <- measure-after-repair (native Call)
                   |
             next iteration carrier -> round input
```

Пакет владеет `extension-points.yaml`, `workflow.yaml` и `workflows/round.yaml`.
Проект меняет `extend.yaml` и `project/**`. Sidecar задаёт exact workflow ID,
anchor, input sources, routes и разрешённые consumer ports. Project entry
задаёт implementation и output для каждого consumer. Compiler не угадывает
имя carrier и не меняет другие bindings.

`measure-before-read` передаёт новый carrier в read, repair и clean.
`measure-after-repair` получает output repair и передаёт новый carrier в fixed.
Native repeat берёт fixed export через `$iteration.carrier`. Следующая iteration
снова выполняет entry measurement. Каждый measurement публикует свежие report/log
refs и сохраняет исходные refs. Reader проверяет именно output текущего measurement.

Пакет может не разрешать Call для конкретного point. Неизвестные points,
неоднозначные edges, чужие components, произвольные bindings, несовместимые schemas
и cycles отказывают до исполнения. Отсутствующий promised output не заменяется
старым carrier. Partial/blocked/fail/no_work не превращаются в passed.

Если extension отсутствует либо имеет `enabled: false`, пакет сохраняет прежние
entry, routes и carrier bindings. Read-only reader сохраняет effects:none.
Measurement объявляет external_write только для synthetic temporary target.
Ни один external service fixture не создаёт. Для process measurement действуют
обычные execution bindings, allowlist и отдельное разрешение исполнения.

## Полный native путь

Запусти из корня checkout:

```sh
go build -o /tmp/prifly-points ./cmd/prifly
python3 -B test/e2e/verify-extension-points.py \
  --binary /tmp/prifly-points --output /tmp/prifly-points-results.json
```

Для released binary укажи его абсолютный путь вместо `/tmp/prifly-points`.
Driver создаёт собственные temporary repository и authority. Он выполняет:

1. `project init`, копирование этого folder и объявление launch `transport`.
2. `project compile --package transport` и проверку manifest в `sources`.
3. `project questionnaire --launch transport --host codex-app --workspace worktree --prepare`.
4. `project start` с теми же аргументами и `--expected-launch-digest` из prepare.
5. `run drive`, `session task`, `session take` и типизированный `session submit`.
6. В case `continue`: `project continue --source-run RUN --launch transport --prepare`, затем start с review digest.
7. Проверку fresh refs, original report/log export после удаления completed producer directory и освобождение claims.

```text
compile -> prepare -> start -> drive/task/submit -> partial
                                                   |
                reviewed continue -> новый Run -> fresh measurement -> reader
```

Cases: success, repair, failed original report, reviewed continue, measurement
blocked/fail/no_work, child partial, missing output, absent и disabled.
Recovery frontier — root cycle; accepted produce переносится. Новый Run
перезапускает child measurement. Source Run и original nested ArtifactRef
сохраняются. Individual nested-stage resume отсутствует. Изменённый accepted
prefix получает native refusal; fixture не обходит его.

Driver проверяет output slots и exact reader input ref. После repair порядок
равен `produce → measure → read → repair → measure → measure → read`.
Raw report applicability остаётся ответственностью проекта и consumer issue #9.
