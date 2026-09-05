## ADDED Requirements

### Requirement: Долгое ожидание проверяется без настоящего многонедельного прогона
Изменение сроков MUST иметь детерминированный regression с управляемыми
Observation и restart: вопрос, две недели ожидания, ответ и результат той же
Attempt с сохранённым остатком. Test MUST наблюдать реальные admitted
переходы и outputs, а не только вычисление новой даты. Отдельный короткий
public CLI mixed Run MUST пройти программу, вопрос, принятый host result и
итоговую программу. Scripted clock/host MUST не выдаваться за живой UI.

Набор MUST включать finite wait expiry на границе, active expiry до question,
повторные команды, rollback, cancel/answer race, stale delivery, late result,
release/reacquire capacity при пределе 1, workflow parallelism, sibling
progress, актуальные Stops/revocation/claims и old-version readers. Само
истечение MUST не выдаваться за доказанный внешний effect или его отсутствие.

Новые сроки MUST не менять старые Runs, sealed bytes и historical evidence.
Managed hour limit, native многонедельное исполнение, lease recovery и UI
qualification MUST иметь явно названные границы доказательства. Текущий
source set — `openspec/specs/quality-and-acceptance/` до применения delta.

#### Scenario: Двухнедельная пауза в regression
- **WHEN** тест переводит управляемое время вперёд и заново открывает authority
- **THEN** без настоящего ожидания проверяются прежний остаток, тот же Run,
  принятый ответ и точные bytes итогового отчёта

#### Scenario: Пройден только короткий живой тест
- **WHEN** mixed Run успешно создал отчёт за несколько минут
- **THEN** это не закрывает проверку недельного ожидания, старых contracts,
  неизвестных эффектов, нескольких hosts либо формальный release gate
