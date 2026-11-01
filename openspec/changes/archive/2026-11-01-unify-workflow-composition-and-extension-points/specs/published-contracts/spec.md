## ADDED Requirements

### Requirement: Unified authoring uses additive closed schemas
Новые authoring contracts SHALL публиковаться как `prifly-extension/5` и `prifly-project-workflow/3`. Existing sidecar `prifly-extension-points/1`, `/4` schema и исторические bundles MUST сохранять bytes и meaning.

Новые CLI DTO SHALL иметь отдельные closed версии: compile `/4`, launch summary `/7`, start `/7`, build provenance `/3` и composition `/2`. Additive bundle SHALL называться `workflow-composition-points.schema.json`. Unknown fields MUST отказываться в соответствующей version boundary.

Новая metadata SHALL храниться в provenance и Project DTO. Она MUST NOT добавлять поля в прежние Run/state/read contracts. Native operators SHALL сохранять свои фактические minimum revisions.

#### Scenario: New field is submitted under extension four
- **WHEN** imports, layers, object point selector либо new public export shape поданы под прежним marker
- **THEN** соответствующая old schema отвергает input; новый binary не расширяет старую closed форму

#### Scenario: Unified result reaches the public boundary
- **WHEN** новый root компилируется, подготавливается и запускается
- **THEN** реальные emitted DTO проходят новые schemas; старые schemas и saved Runs остаются неизменными

### Requirement: Variant identity has an explicit b2 algorithm boundary
Unified root SHALL использовать algorithm b2 в `prifly-build-provenance/3`. Variant key SHALL учитывать exact source owner/content, author identity, effective point contract, settings и dependency variant keys до sealing. Он MUST NOT включать compiled refs или final package digest.

Package build key SHALL учитывать effective profile/values, declaration provenance и hashes влияющих source inputs. Compiled package version SHALL быть `0.0.0-b2.` с lower-case unpadded base32 build-key digest. Compiled component version SHALL использовать hash canonical tuple `[build_key, kind, author_id, author_version, variant_key]` с тем же prefix.

Existing b1 path MUST сохранять прежний tuple, inputs и meaning. Различные point policies MUST давать разные exact compiled refs. Одинаковый effective variant одного source owner внутри одной сборки SHALL иметь один compiled ref.

#### Scenario: Configured and passthrough variants coexist
- **WHEN** два uses одной author identity имеют разные point policies
- **THEN** provenance связывает их с разными b2 compiled versions; b1 fixtures прежнего `/4` остаются byte-identical

#### Scenario: Hash dependency would become recursive
- **WHEN** compiler формирует variant, build и package identities
- **THEN** variant keys вычисляются по source/dependency identities до compiled refs; final package digest не входит в собственные inputs

### Requirement: Unified capability is distinct from the two prior capabilities
Capability manifest SHALL объявлять `workflow_composition_with_points` только после реализации совместного поведения. Existing `workflow_composition` и `project_extension_points` MUST сохранять meaning. Их совместное наличие MUST NOT считаться доказательством нового mixed contract.

Новая возможность SHALL иметь author index row, reference с действительными fields и runnable example. Qualification SHALL отдельно проверять source candidate, exact published binary и consumer acceptance.

#### Scenario: Binary has both old capabilities
- **WHEN** binary сообщает оба прежних tokens, но не сообщает combined capability
- **THEN** tooling не считает его поддерживающим `/5` или public point exports

#### Scenario: Published binary is qualified
- **WHEN** exact release SHA прошёл verify/qualify и опубликован новый binary
- **THEN** replay повторяет mixed workflow и unchanged `/4` checks, сохраняя отдельный статус продуктовой приёмки
