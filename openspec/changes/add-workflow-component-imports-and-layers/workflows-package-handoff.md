# Поручение репозиторию workflows после выпуска Core

Требуется опубликованный Core с `workflow_composition`. Изменения пакета и пилот
smsplace выполняются отдельно. Этот документ не отправляет внешнее поручение.

1. Переведите необходимые sources на folder `/2` и extension `/3`.
2. Экспортируйте собственный `review-for-change` workflow с явными task/candidate ports.
3. Оставьте standalone запуск UI/UX пригодным для отдельной задачи. При необходимости наследуйте classic через `extends`.
4. Сделайте applicability первой обязательной стадией экспортированного workflow.
5. Проверяйте исходную задачу и фактический candidate. Файловой эвристики недостаточно, включая PHP-rendered UI.
6. Записывайте required, not-required с объяснением либо unknown. Not-required не создаёт browser reader Attempt.
7. При required проверьте UI/UX в согласованной browser environment. Недоступность обязательной проверки блокирует приёмку.
8. Закрепляйте task/candidate correlation, reports и evidence. После repair повторяйте applicability и нужные checks.
9. Опишите browser startup/readiness, ownership, cleanup и interruption. Старый PASS не доказывает результат нового candidate.
10. Выпустите и квалифицируйте package. Затем подготовьте отдельный reviewed pilot smsplace/backend.

```text
Core release → package export + qualification → smsplace reviewed pilot
```

В smsplace сначала перенесите прежние project tests/MR в общий delivery layer.
Затем подключите public export без копии provider stages. Обязательным остаётся
Call применимости. Сам browser reader исполняется по результату provider.
Проверьте required, justified skip, unknown, repair и interruption на настоящем проекте.
