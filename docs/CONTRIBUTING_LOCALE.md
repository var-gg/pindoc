# Contributing a locale to Pindoc

Pindoc keeps language-neutral META rules in code (PINDOC.md harness,
preflight contract) and language-specific DATA (length bounds, jargon
token sets, reading speeds) as small data points the operator and OSS
contributors can extend. Adding a new locale to the title-quality
preflight is a JSON-shaped change against
`internal/pindoc/titleguide/locale_data.go`; you should not need to
touch any other file unless you are deliberately changing META rules.

## What is in a locale entry

```go
"ja": {
    Locale:   "ja",
    MinRunes: 10,   // titles below this fire TITLE_TOO_SHORT
    MaxRunes: 70,   // titles above this fire TITLE_TOO_LONG
    JargonTokens: []string{
        "その他", "一般", "雑多", "色々", "仮",
    },
},
```

| Field | What it controls | How to pick a value |
|---|---|---|
| `Locale` | Lookup key. Lowercase BCP47 language subtag (`ko`, `ja`, `zh`). Region/script subtags fall back to the language entry, so `ko-KR` resolves to `ko`. |
| `MinRunes` | Lower bound for title length. Triggers `TITLE_TOO_SHORT`. | A title that is too short to disambiguate later. Korean / Japanese text is denser than English so the floor is lower. |
| `MaxRunes` | Upper bound. Triggers `TITLE_TOO_LONG`. | A title long enough to feel like a sentence rather than a name. Latin-script languages tolerate higher values; CJK languages get pruned earlier. |
| `JargonTokens` | Substring match (case-insensitive) that triggers `TITLE_GENERIC_TOKENS`. | Words that show up in low-quality titles in your language — generic verbs ("처리"), filler nouns ("기타"), placeholder words ("仮"). Don't include words that are project-specific jargon — those go in the operator override below. |

## How to add a locale

1. Open `internal/pindoc/titleguide/locale_data.go`.
2. Add a new entry to `localeDataFor` keyed by the lowercase language
   subtag.
3. Pick `MinRunes` and `MaxRunes`. Reference points:
   `en` = 15 / 80, `ko` = 8 / 60, `ja` = 10 / 70. Calibrate by hand-
   checking a dozen real artifacts in your language — if the in-band
   ones feel right and the out-of-band ones obviously sprawl, the
   numbers are good.
4. Compile a `JargonTokens` list. Keep it short (≤ 15 entries) and
   conservative. False positives are worse than misses here — every
   token you add is a warning every contributor in that language sees.
5. Add a test case to `evaluate_test.go` under
   `TestEvaluateTitle_GenericTokens` covering one realistic
   over-generic title in your language.
6. Update the table in this file with the new locale so reviewers
   see the bounds at a glance.

## Project- vs locale-level jargon

Some words look generic but are core vocabulary inside a specific
project. `Layer N` is meaningless in most projects but a binding
concept in Pindoc itself. Don't add such tokens to the locale
baseline — they belong in the operator override that the
`server_settings.title_jargon_tokens` field will land. The follow-up
that wires that field through `Deps.Settings` is tracked separately;
until then the override path is a no-op and the embedded baseline is
the only signal.

## Fallback chain

`titleguide.Resolve(locale)` resolves in this order:

1. Exact match (`ko-KR` → `ko-KR` if present).
2. Language subtag (`ko-KR` → `ko`).
3. `en` baseline.

Your locale entry only needs to handle the second tier. Region- and
script-specific overrides are out of scope until a contributor brings
a use case.

## What stays in code, not data

These are META rules that apply to every locale and live in Go
(`titleguide/evaluate.go`, harness `Title quality` section):

- "Specific over generic"
- "Distinctive keywords up front"
- "Avoid duplicating Type prefix"
- "Unique within the project"

If you want to change one of these, that is a META change — open an
RFC against PINDOC.md and the harness install template instead of
adding a locale entry.
