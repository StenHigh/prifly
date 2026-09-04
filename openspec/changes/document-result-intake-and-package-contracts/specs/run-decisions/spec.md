Authoritative source set: `openspec/specs/run-decisions/spec.md` (перенесено).
Compatibility path: shape каталога не меняется; `required` перестаёт
запечатываться там, где он ничего не обещает, и ни один поставляемый package
его в этой позиции не объявляет.

## MODIFIED Requirements

### Requirement: Каталог решений является явным sealed контрактом Run
Project workflow package MUST объявлять каждое поддерживаемое решение через
stable ID, вопрос, конечное множество или schema ответов, область действия,
допустимый default/recommendation и правило автоматического выбора. Compiler
MUST validate-ить каталог вместе с выбранным package profile и включать exact
definition в sealed Run inputs. Он MUST NOT извлекать решение из prose skill,
создавать новое решение по имени step или разрешать неизвестный ID.

Каталог MUST объявлять обязательность только там, где она enforced. Ответ на
решение фазы `preflight` MUST требоваться до старта Run. Решение фазы
`runtime` MUST NOT объявляться обязательным: authority не может обязать
executor поднять запрос и принимает отчёт без него, поэтому такая пометка
обещала бы gate, которого нет. Compiler MUST отказывать такому каталогу до
sealing.

#### Scenario: Package объявляет выбор уровня планирования
- **WHEN** workflow package объявляет решения `plan_profile` со значениями
  `fast`, `full` и `ultra`
- **THEN** предзапусковая форма показывает только эти значения и sealed Run
  сохраняет exact catalog definition

#### Scenario: Milestone зависит от выбора linkage
- **WHEN** package объявляет `roadmap_milestone` с predicate
  `roadmap_linkage = "link"`
- **THEN** questionnaire показывает milestone только после exact выбранного
  linkage value, а predicate не создаёт stage, route или permission

#### Scenario: В authoring source есть неописанный вопрос
- **WHEN** compiler встречает ссылку на absent decision ID
- **THEN** он отказывает до sealing и не создаёт Run

#### Scenario: Runtime-решение объявлено обязательным
- **WHEN** каталог помечает решение фазы `runtime` как required
- **THEN** compiler отказывает до sealing и называет, что обязательность
  доступна только фазе `preflight`
