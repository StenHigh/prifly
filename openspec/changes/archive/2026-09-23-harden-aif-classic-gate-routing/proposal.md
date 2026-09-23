## Why

В Run `176bdd…db1ac` verify нашёл исправимые остаточные задачи, но host сообщил
`needs_revision` вместо штатного `pass` с `gate.blocking=true`. Граф принял
буквальный verdict и закончил Run как `partial`, обходя уже объявленный
`aif-fix` round. Такой маршрут зависит от безошибочного прочтения адаптера и
непригоден для слабого или нового host.

## What Changes

- Закрыть аварийный обход repair loop: gate с пригодным `gate` artifact
  направляется к declared choice и `$aif-fix` независимо от того, назвал ли
  host findings verdict-ом `pass` или `needs_revision`.
- Оставить terminal `partial` только для отсутствия пригодного gate result,
  owner-only blocker, отсутствия безопасного исправления либо исчерпанного
  лимита rounds.
- Сделать bridge и README однозначными: `$aif-fix` — следующий declared step,
  а не действие, которое host должен угадывать или запускать вручную; после
  исправления Run снова запускает gate.
- Добавить regression check исходного ошибочного verdict для verify и review.
- Дать Codex host явный control loop: после каждого accepted session report он
  читает `run next`, вызывает `run drive` для control/program и берёт только
  следующую выданную Attempt; subagent используется лишь при доступном
  механизме, иначе Attempt выполняется host и это честно фиксируется.

## Capabilities

### New Capabilities

- `aif-classic-workflow`: устойчивый маршрут canonical AI Factory package от
  gate finding через repair round к повторной проверке.

### Modified Capabilities

_Нет._

## Impact

Меняются versioned authoring source и pinned bridge contexts пакета
`.prifly/workflows/aif-classic`, его README, package-level regression checks и
инструкция host `prifly-run`. Core runtime, публичные JSON contracts и история
уже созданных Run не меняются.
