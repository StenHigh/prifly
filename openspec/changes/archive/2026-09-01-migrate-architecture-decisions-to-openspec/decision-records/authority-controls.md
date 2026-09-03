# Authority-side controls и управляющие решения

## Контекст

Owner decisions, access, stops, approvals and grants cannot be represented as
untrusted worker fields or synthetic workflow steps.

## Решение

A distinct authority control plane stores authenticated principals, access,
policy decisions, approvals, grants, waivers and scoped stops. Protected
mutation consumes exact control intent atomically with current checks; host,
CLI and library use the same typed command boundary.

## Последствия

Revocation and stop restrict future dispatch without rewriting history.
Approval separation, grant accounting and quality waiver scope are explicit;
they cannot create missing evidence, a fabricated pass or arbitrary tool
permission.

## Пересмотр

Remote identity, webhook, broad RBAC or per-action external admission require
their own authenticated transport, versioned public contract and qualification.

## Не входит

Нет generic identity provider, implicit independent-human quorum, direct host
journal write или blanket execution permission.
