[Индекс](index.md) · [Исполнение](hosting.md) · [Восстановление](recovery.md)

# Объявленные решения и вопросы хоста

## До запуска

Questionnaire показывает declared questions. `preflight` собирает ответы до start; `runtime` ожидает конкретного запроса шага.
Значение из standing `extend.yaml` сохраняется, если пользователь явно не выбирает override.
Повторно читайте questionnaire с выбранными profile/policy/answers, чтобы узнать conditional applicability.
`autonomous` выбирает только разрешённые automatic recommendations. Он не разрешает scope-changing ответ без владельца.
Решение Run не создаёт Approval, Grant или новый effect.

## Во время Run

```text
current task -> declared request -> yield -> pending question
                                               |
                                               v
                         answer -> drive -> current task -> work
```

Запрос допустим только для exact declared decision, которую pinned adapter разрешил этому шагу.
Нативный вопрос произвольного skill не становится Decision Bridge автоматически.
Если adapter не сопоставил вопрос ID, используйте обычное общение по разрешённому host protocol.

```sh
"$PRIFLY_BIN" --project "$authority_root" session task --run "$RUN" --all --json
"$PRIFLY_BIN" --project "$authority_root" run decision "$RUN" request \
  --attempt "$ATTEMPT" --envelope-digest "$ENVELOPE_DIGEST" --decision "$DECISION" \
  --expected-run-version "$RUN_VERSION" --yield-execution --json
"$PRIFLY_BIN" --project "$authority_root" run decisions "$RUN" --json
```

Для assisted-session/6+ `--yield-execution` отправляет DecisionRequest/2 и передаёт управление authority.
После принятого yield остановите эту delivery: не пишите файлы, не отправляйте final result и не подтверждайте неизвестный effect.
Это cooperative transfer, а не физическое замораживание внешнего процесса.
Legacy delivery не принимает новую семантику waiting time; для неё абсолютный deadline включает ожидание человека.
CAS `--expected-run-version` берите из task после take. При конфликте перечитайте, а не увеличивайте число наугад.

Ответ может быть уже записан preanswer/policy. Тогда получите текущую redelivery вместо повторного вопроса.
Если action `waiting_decision`, покажите sealed title, description и реальные choices без придуманного scope.
Receipt question tool, preselection, timeout и молчание не являются ответом.

```sh
"$PRIFLY_BIN" --project "$authority_root" run decision "$RUN" answer \
  --decision "$DECISION" --request-digest "$PENDING_REQUEST_DIGEST" \
  --expected-run-version "$RUN_VERSION" --value 'JSON_VALUE' --json
"$PRIFLY_BIN" --project "$authority_root" run drive "$RUN" --json
"$PRIFLY_BIN" --project "$authority_root" run next "$RUN" --json
```

Для ответа используйте `pending_request_digest` и `run_version` текущего `run decisions`.
`request_digest` receipt команды — другое значение.
Сохранённый answer ещё не означает новую delivery. Capacity, stop или resource могут удерживать выполнение.
Получите свежий task только после handoff. Старый envelope после yield не разрешает работу.
Без работающего host/driver нет обещания автоматического пробуждения.

## Отчёт обо всех вопросах Attempt

При `question_report: required` отправьте `answered_questions` на верхнем уровне submission.

```json
{
  "answered_questions": [
    {
      "question": "Какие проверки выполнить?",
      "answer": "Проверки из переданного gate_checks",
      "basis": "decision",
      "decision_id": "gate_checks",
      "asked_by": "pinned skill"
    },
    {
      "question": "Какой формат результата использовать?",
      "answer": "JSON schema входного handoff",
      "basis": "input",
      "port": "handoff"
    }
  ]
}
```

| basis | Когда использовать | Дополнительное поле |
|---|---|---|
| `decision` | Объявленное решение или handed decision_context | `decision_id`, включая handed `core:` key |
| `input` | Значение определил input port | `port` |
| `instructions` | Ответ явно задан pinned инструкциями | Точная формулировка answer |
| `person` | Человек ответил в этой сессии | Фактический ответ |
| `judgement` | Хост выбрал сам в пределах полномочий | Фактическое основание выбора |

`asked_by` и `options` заполняются, когда они были. Сохраняйте текст вопроса, а не только его номер.
Пустой массив допустим только когда вопросов действительно не было.
Report provenance не доказывает отдельную человеческую личность: host и owner могут использовать один OS account.

## Время

`session_limits.active_timeout_ms` — бюджет активной работы Attempt; `null` означает отсутствие конечного work limit по этому контракту.
`decision_wait_timeout_ms:null` — неограниченное ожидание одного declared question, а не бесконечная работа всего Run.
Legacy `legacy_absolute_timeout_ms` включает человеческое ожидание.
Timing различает `host_pickup`, `host_work`, `decision_wait` и `idle` по сохранённым границам.
Не приписывайте всю длительность модели: внешняя работа без записанного take не измерена.

См. также [host report](hosting.md), [launch questionnaire](launch.md), [справочник решений YAML](../authoring/run-decision-authoring-reference.yaml).
