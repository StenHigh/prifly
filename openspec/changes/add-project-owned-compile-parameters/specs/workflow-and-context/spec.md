## ADDED Requirements

### Requirement: Пакет разрешает ограниченные параметры сборки
Package SHALL объявлять в `compile-parameters.yaml` (`prifly-compile-parameters/1`) именованные параметры с типом integer, положительными minimum/maximum, default и непустыми bindings. Binding SHALL называть собственную schema через локальный alias и JSON Pointer существующего maxLength строковой schema. Значением поля SHALL быть whole scalar placeholder того же параметра. Compiler MUST отказать неизвестной, чужой, повторной или несовместимой цели. Owner SHALL сохранять значения в `project/compile-values.yaml` (`prifly-compile-values/1`), отдельно от upstream-owned файлов. Неизвестное имя, неправильный тип и значение вне диапазона MUST получать типизированный отказ до Run.

#### Scenario: Default и owner value ограничивают описание
- **WHEN** default равен 12000, а owner value равен 32000
- **THEN** default принимает 12000 и отклоняет 12001; owner value принимает 16557 и 32000, но отклоняет 32001

#### Scenario: Binding не соответствует объявленному полю
- **WHEN** binding называет внешний schema alias, отсутствующий pointer или поле без matching placeholder
- **THEN** compiler отказывает до sealing и не подменяет другую schema

### Requirement: Параметр сборки сохраняет совместимость closure и Run
Compiler MUST применять validated value до проверки task и до sealing refs корня, child workflow и worker. Project-owned consumer MUST получать ту же effective schema через обычный typed binding. Разные значения MUST давать отдельные compiled identities в одной authority. Update MUST сохранять owner file и MUST явно отказать несовместимому оставшемуся значению при последующей compilation. Existing Run MUST сохранять исходные definitions и task bytes. Continue MUST использовать исходный контракт либо явно отказать до эффектов. Source set остаётся `workflow-and-context`; legacy authoring без sidecar сохраняет текущий смысл.

#### Scenario: Два проекта запускают разные пределы
- **WHEN** два проекта с одной author identity выбирают разные допустимые значения в общей authority
- **THEN** оба Run используют свои compiled refs без definition drift и конфликтов shared schemas

#### Scenario: Настройка изменена после запуска
- **WHEN** owner меняет значение либо обновляет пакет
- **THEN** прежний Run сохраняет definitions и полный task, а новая compilation проверяет обновлённую декларацию
