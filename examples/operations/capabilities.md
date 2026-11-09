[Индекс](index.md) · [Авторинг](authoring.md)

# Индекс объявленных возможностей

Capability сообщает поддержку installed contract, а не разрешение на эффект.
Минимальные revisions и worked YAML fields находятся в [каноническом author index](../README.md#что-умеет-движок-и-где-это-показано).
Для каждого имени ниже указан эксплуатационный маршрут. Проверка покрытия сверяет таблицу с реальным capabilities API в обе стороны.
Capability без public handler описывается как contract boundary; не придумывайте команду по её имени.

| Capability | Где применить | Где объявить поле и минимальную поддержку |
|---|---|---|
| `step` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `finish` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `local_process` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `state_hook` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `event_hook` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `telemetry.catalog` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `telemetry.records` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `telemetry.aggregate` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_call_extensions` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_extension_points` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `workflow_composition` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `workflow_composition_with_points` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_compile_parameters` | [Процедура](launch.md) | [Параметр сборки](../authoring/compile-parameters-authoring-reference.yaml), [значение проекта](../authoring/compile-values-authoring-reference.yaml) |
| `storage_budget_management` | [Процедура](operations.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `on_error` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `json_projection` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `input_configuration` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `choice` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `call` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `repeat` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `partial` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `local_workflow_aliases` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `context_resources` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `full_context` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `source_import` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `context_request` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `automatic_checks` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `assisted_session` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `quality_waivers` | [Процедура](control.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `parallel` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `map` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `wait` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `schedule` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `live_guards` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `reported_cost` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `artifact_publication` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `artifact_publication_checks` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `artifact_close` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `publication_subscription_once` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `publication_subscription_each` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `publication_subscription_new_only` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `publication_subscription_terminal_failure` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `publication_subscription_blob` | [Процедура](artifacts.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `action_intent_proposal` | [Процедура](control.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `action_admission` | [Процедура](control.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `action_grant_admission` | [Процедура](control.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `action_delivery_prepared` | [Процедура](control.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `run_fork` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `workspace_modes` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `workspace_tree_artifacts` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `decision_catalog` | [Процедура](decisions.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `neutral_start` | [Процедура](launch.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `execution_bindings` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `assisted_session_timing` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `exclusive_resources` | [Процедура](operations.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `light_task_board` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_task_queue` | [Процедура](operations.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `program_live_output` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `program_progress` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `declared_technical_retries` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `answered_questions` | [Процедура](decisions.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `routed_session_verdicts` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `declared_impossible_verdicts` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `assisted_effects_enforced` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `assisted_session_unbounded_work` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `materialize_only_workspace_tree` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `run_failure_named` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `stage_work_named` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `execution_environment_source` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `program_environment_named` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `model_profile_declared` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `model_profile_translation` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_title` | [Процедура](launch.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `failed_stage_recovery` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `run_finish_named` | [Процедура](observability.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `blocked_verdict` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `declared_external_write` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `repository_workspace_read` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `next_handoff` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `workflow_continuation` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_worktree_naming` | [Процедура](workspace.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `project_owned_naming_inputs` | [Процедура](launch.md) | [Author reference](../authoring/project-profile-authoring-reference.yaml) |
| `workflow_resume` | [Процедура](recovery.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `next_awaiting_host` | [Процедура](hosting.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |
| `program_external_write` | [Процедура](authoring.md) | [Author index](../README.md#что-умеет-движок-и-где-это-показано) |

| `shared_schema_migration` | [Процедура](schema-migration.md) | Explicit schemas; Run/read 45, authority-packages/2; development change |
