# Backlog

---

## P1 - Critical Issues

| ID  | Topic | Description | Reason |
| --- | ----- | ----------- | ------ |

---

## P2 - Important Issues

| ID  | Topic | Description | Reason |
| --- | ----- | ----------- | ------ |
| B-1 | Package moved detection | Add package_moved pass to astdiff DiffExports (same name+sig, different package). Isolated pass, no deps on other passes. | Deferred from v1 AST diff engine — no downstream consumer (scriptgen) yet |

---

## P3 - Minor Issues

| ID  | Topic | Description | Reason |
| --- | ----- | ----------- | ------ |
| B-2 | Promoted method tracking | Flatten embedded type method sets in ParseExports for deeper change detection | v1 records embedding declaration only, does not track promoted methods |
| B-3 | Changelog hints for renames | Parse CHANGELOG.md for explicit rename/move documentation to boost confidence | Could feed into Pass 3 with HIGH confidence without heuristics |
| B-4 | Structured generic diffing | Compare type constraints structurally instead of opaque string comparison | v1 renders generics as part of signature string |
| B-5 | Configurable diff thresholds | CLI flags for MinNameSimilarity, MinParamOverlap | Hardcoded named constants in v1 |
