## MODIFIED Requirements

### Requirement: Stop и cancel сохраняют обязательства

Durable stop acknowledgement MUST запрещать новые ordinary admissions и
перечислять ранее admitted work, cancellation, known и unknown effects.
Pause переходит в waiting после safe stop; cancel становится terminal только
при known судьбе admitted actions, иначе Run uncertain. Probe, cancel и
reconcile действуют по separate recovery permissions. Managed executor
прекращает dequeue и перепроверяет admission перед dispatch; assisted host
честно сообщает, удалось ли остановить worker. Scope cancellation не отменяет
siblings неявно. Наблюдатель попытки MUST заканчиваться вместе с вызовом
драйвера, который его создал, на любом выходе этого вызова; он MUST NOT
запрашивать cancellation Run после того, как его попытка завершена.

#### Scenario: Cancel не может проверить remote target

- **WHEN** target недоступен после cancellation request
- **THEN** пользователь видит uncertain obligation, а не ложный terminal cancel

#### Scenario: Вызов драйвера отказал до запуска программы

- **WHEN** вызов драйвера завершает попытку неначатой после того, как запустил
  наблюдателя этой попытки
- **THEN** наблюдатель заканчивается вместе с вызовом и не запрашивает
  cancellation Run, который к этому моменту уже идёт дальше
