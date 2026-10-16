## ADDED Requirements

### Requirement: Unbound claim проверяется по полному согласованному ownership evidence
До привязки unbound WorktreeClaim к Run или его release authority SHALL
проверять всё сохранённое ownership evidence на одном ReadCut. Общий объём
восстановленных snapshots больше 64 МиБ или число Run больше 1000 MUST NOT
сами по себе препятствовать завершению этой проверки. Память проверки SHALL
быть ограничена размером одного допустимого snapshot и ограниченным рабочим
буфером, а не объёмом всей истории. Aggregate limits других bounded read API
MUST сохраняться.

Успешное доказательство отсутствия запрещающего держателя MUST требовать
полного окончания чтения и успешных integrity/compatibility checks. Unknown
state, corruption, недоступное evidence, cancellation и незавершённое чтение
MUST NOT считаться доказательством отсутствия владельца. Binding SHALL
сохранять existing запрет повторного использования claim, уже выданного legacy
Run; release SHALL сохранять существующие settlement/terminal условия.

До фиксации binding или release fence authority SHALL повторно проверять
актуальность ownership доказательства, exact claim generation и resource
identity. Конкурентное изменение relevant evidence MUST вызывать отказ или
новую полную проверку до filesystem deletion. Изменение generation после
проверки MUST NOT позволять удалить новое дерево или передать его stale owner.
Bound claim SHALL сохранять обычный direct Run ownership path без full scan.

#### Scenario: Свободный claim на большой authority
- **WHEN** complete packed/unpacked population превышает 64 МиБ или 1000 Run, а запрещающий holder отсутствует
- **THEN** штатный binding/release завершается без усечения проверки, удаления истории, увеличения scan limit или подмены authority

#### Scenario: Держатель находится за прежним лимитом
- **WHEN** запрещающий legacy holder расположен после 64 МиБ либо после тысячного Run
- **THEN** admission/release отказывается, не удаляет его дерево и не считает просмотренный prefix полной проверкой

#### Scenario: Evidence изменилось перед commit
- **WHEN** между чтением и binding/release commit изменилось ownership evidence или поколение claim
- **THEN** прежнее доказательство не допускает операцию; новая проверка либо named conflict предшествует любому удалению

#### Scenario: Последний snapshot не читается
- **WHEN** после корректного prefix обнаружен corrupt, unknown-version или недоступный snapshot
- **THEN** claim остаётся защищённым, а ошибка называет недоказанное ownership вместо успешной проверки
