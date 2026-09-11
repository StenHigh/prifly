# Выпуск Pri-Fly 0.13.22

Одна починка текста отказа, найденная пилотом на живом хосте и
воспроизведённая пакетчиком на 0.13.21.

## Что было

```
project_start_invalid_decision_answer: gate_checks: schema_invalid at : value does not satisfy the declared contract
```

Ни предела, ни фактической длины. Пилот нашёл причину чтением схемы пакета
(`maxLength: 4000`, состав гейта 4277), а не по сообщению. `declaredExpectation`
печатал сторону контракта для `enum`, `const`, `type`, `required` — и молчал
для всех числовых границ, хотя у `kind.MaxLength` оба числа (`Got`, `Want`)
уже лежали в листе ошибки. Та же форма, что «enum из четырёх имён стоил
пилоту трёх попыток», только число вместо имени.

## Что стало

Каждая граница называет себя, а счётная — ещё и счёт значения (длина или
количество — не значение, оно по-прежнему не печатается; отвечает на
единственный оставшийся вопрос «на сколько ужать»):

```
…; the contract allows at most 4000 characters, this value has 4277
…; the contract requires at least 1 items, this value has 0
…; the contract allows at most 1 fields, this value has 2
```

Числовая граница называет только границу — число, не прошедшее её, и есть
значение; pattern — только выражение:

```
…; the contract allows at most 10
…; the contract requires a value below 4
…; the contract requires a value matching ^[a-z]+$
```

Покрыты `maxLength`, `minLength`, `maxItems`, `minItems`, `maxProperties`,
`minProperties`, `maximum`, `minimum`, `exclusiveMaximum`, `exclusiveMinimum`,
`pattern`. Разрез пакетчика в тесте: 16001 при `maxLength: 16000` называет
16000 и 16001; с вырезанным случаем — прежний немой текст.

## Пакетная сторона (не этот выпуск)

В `gate_checks` предел 4000 противоречил собственному совету пакета «a
command is best»; в 1.36.0 (дерево, не выпущено — слово владельца) предел
16000 и назван в описании решения. Инженерного потолка на `session_context`
у движка нет.

## Ворота

`make check` (race), `make e2e`, CI `verify`.
