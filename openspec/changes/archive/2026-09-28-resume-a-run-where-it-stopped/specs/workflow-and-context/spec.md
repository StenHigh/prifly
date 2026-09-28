## ADDED Requirements

### Requirement: Workflow объявляет, что его Run можно возобновить
WorkflowRevision 8 SHALL позволять workflow объявить `resumable`: список
исходов, при которых его Run можно возобновить тем же workflow, и/или
`from_cancelled: true` для отменённых Runs — хотя бы одно из двух.
Объявление MUST NOT содержать маппинг входов: возобновлённый Run получает
входы исходного Run. Технический отказ без исхода восстанавливается без
объявления, как прежде. Объявление в явной ревизии ниже 8 MUST отказываться;
лестница авторинга MUST выводить ревизию 8 по наличию поля. Ревизии 1–7 MUST
компилироваться как прежде.

#### Scenario: Workflow объявляет возобновление
- **WHEN** workflow объявляет `resumable: {from_outcomes: [partial, rejected], from_cancelled: true}`
- **THEN** компилятор запечатывает объявление в WorkflowRevision 8, и оно
  входит в digest workflow

#### Scenario: Пустое объявление
- **WHEN** `resumable` не называет ни исходов, ни `from_cancelled`
- **THEN** компиляция отказывает
