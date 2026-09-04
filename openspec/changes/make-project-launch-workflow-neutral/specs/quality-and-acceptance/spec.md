## ADDED Requirements

### Requirement: Project launch проходит нейтральный и отраслевой пути отдельно
Изменения Project authoring, launch и decision UX SHALL иметь воспроизводимую
приёмку через public CLI: command-only workflow в папке без Git/ИИ; mixed
command → assisted task → command; несколько настроек одного package в одной
authority с сохранением старого Run после restart. Проверка MUST проверять
фактические outputs, import/start и продолжение, не только compile/schema.
Core corpus MUST не зависеть от AIF files, credentials, сети или model quality.
Внешние AIF packages MUST отдельно проверять совместимость с exact binary.

#### Scenario: Нейтральный путь завершён
- **WHEN** пользователь запускает YAML CSV → validation → report без Git и host
- **THEN** реальная команда создаёт проверенный typed результат без скрытого
  task/AI scaffolding и результат можно прочитать после restart

#### Scenario: Mixed путь остановился на вопросе
- **WHEN** command завершён, assisted task запросил решение и процесс перезапущен
- **THEN** тот же Run сохраняет ожидание, принимает ответ и исполняет завершающую
  команду ровно после принятого результата assisted task

#### Scenario: Проверен только AIF compile
- **WHEN** внешняя проверка успешно seal-ит AIF
- **THEN** это не закрывает neutral launch, живой AIF pilot, host UI observation
  или формальную приёмку продукта
