# Проверенный baseline

Источник fixture: consumer repository commit `95ea2cb77b69570c24891694e8e23bcf2d9102cb`.
Binary: released `v0.13.82`, SHA-256 `3b073126921bca3e9d78b7649b51bd20527fd70cf4de44e0d440b5428b7712e5`.

```sh
python3 -B tests/probes/report_transport.py --binary /tmp/prifly-issue14-stable/prifly --cases success repair --output /tmp/prifly-issue14-baseline.json
```

Оба cases PASS как baseline; nested refusal здесь ожидается, не доказывает новую возможность.
Success: produce → measure → read → consume.
Repair: produce → measure → read → repair → read → consume.
Nested attempt: `project_extension_unknown_workflow: round (known: transport)`.
Исходные report/log refs экспортируются после удаления producer Attempt directory. Unknown ref, missing revision и substituted digest отказывают.

Проверка existing primitives: static workflow_ref/body_workflow_ref нельзя выбрать через runtime input. Settings меняют только defaults. Unresolved alias проверяется и за disabled Choice. Package refs и generated aliases нельзя подменить. Root extension не предоставляет downstream rebinding. Требуется authoring возможность, transport уже существует.

Граница: synthetic assisted replies; без AI Factory, LLM и настоящего provision/release.
