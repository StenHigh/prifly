# Прежний пробел маршрута verify: fail и no_work

**Статус:** воспроизведён на baseline и candidate. В readability patch не исправлен.

Baseline: repository workflows `v1.53.0`, aif-classic 1.47.0, Pri-Fly stable 0.13.81.
Детерминированная проба возвращает `fail` или `no_work` из шага verify без gate artifact.
Оба verdicts разрешены контрактом; gate для них не требуется.

Наблюдение:

```text
verify fail/no_work
  → verify-once.stopped: no_work
  → verify-batch.unstarted: no_work
  → root.verify: handler отсутствует
  → Run status failed; diagnostic unhandled_child_outcome
```

Root `stages.verify.on` определяет только `succeeded` и `partial`.
Исходник и кандидат дают одинаковый технический сбой, а не объявленный root outcome.
[Evidence](../evidence/routes.json) содержит оба случая и diagnostic code.

Воспроизведение: применить `upstream-route-probe.patch` и выполнить команду из [README](README.md).
Исправление требует отдельного решения владельца внешнего package о terminal outcome.
Текущая работа сохраняет поведение и не выбирает такой outcome вместо владельца.
