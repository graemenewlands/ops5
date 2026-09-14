# OPS5 `ppwm` Directive & Command Reference

The `ppwm` ("pretty-print working memory") command filters, inspects, and prints working memory elements (WMEs) that match a condition element pattern. It is the pattern-matching counterpart to `wm` (which inspects working memory elements by class or in full) and `pm` (which inspects production memory rules).

This command is particularly valuable during interactive development and rule debugging to quickly verify which working memory elements satisfy specific attribute constraints.

---

## 1. Syntax

### 1.1 Source File Directive (`.ops` / `.ops5`)

In OPS5 source files, `ppwm` can be declared as an S-expression at the top level:

```ops5
(ppwm <class> [^<attribute> <value> ...])
(ppwm (<class> [^<attribute> <value> ...]))
(ppwm *)
(ppwm)
```

- When a class and attribute constraints are provided, all matching WMEs currently in working memory are printed in timetag order.
- If `*` is specified or if no arguments are provided, **all** elements in working memory are printed.
- Positional attribute tests are supported if a class schema was declared with `(literalize ...)`.

#### Example
```ops5
(literalize City name state population)

(make City ^name Pittsburgh ^state Pennsylvania ^population 300000)
(make City ^name Philadelphia ^state Pennsylvania ^population 1500000)
(make City ^name Boston ^state Massachusetts ^population 675000)

; Print all City WMEs in Pennsylvania
(ppwm City ^state Pennsylvania)
```

Output:
```text
(1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)
(2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)
```

---

### 1.2 Interactive CLI REPL

The interactive REPL supports both bare-word and S-expression forms:

```text
ops5> ppwm <class> [^<attribute> <value> ...]
ops5> ppwm *
ops5> (ppwm <class> [^<attribute> <value> ...])
ops5> (ppwm (<class> [^<attribute> <value> ...]))
ops5> (ppwm *)
ops5> (ppwm)
```

#### Example Session
```text
ops5> (literalize City name state population)
Declared class schema 'City' with 3 attributes: [name state population]
ops5> make City ^name Pittsburgh ^state Pennsylvania ^population 300000
Asserted: (1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)
ops5> make City ^name Philadelphia ^state Pennsylvania ^population 1500000
Asserted: (2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)
ops5> make City ^name Boston ^state Massachusetts ^population 675000
Asserted: (3: City ^name Boston ^population 675000 ^state Massachusetts)
ops5> ppwm City ^state Pennsylvania
(1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)
(2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)
ops5> (ppwm City ^name Boston)
(3: City ^name Boston ^population 675000 ^state Massachusetts)
ops5> ppwm City ^state Ohio
ops5>
```

---

## 2. Syntactic Constraints & Restrictions

In classic OPS5, `ppwm` is designed for direct constant pattern matching against working memory. To prevent ambiguity with rule LHS variables and complex Rete tests, the pattern **cannot** contain any of the following:

| Disallowed Construct | Reason & Example | Engine Behavior |
| :--- | :--- | :--- |
| **Variables** | Variables such as `<x>` or `<state>` require binding contexts not available in static memory inspection. | Rejected with parse error: `ppwm pattern cannot contain variables (found '<x>')` |
| **Predicates / Operators** | Relational tests like `<`, `<=`, `>`, `>=`, `=`, `<>`, `!=` are not permitted in `ppwm` patterns. Equality is implicit. | Rejected with parse error: `ppwm pattern cannot contain predicates (found '>')` |
| **Quote Operator (`//`)** | The classic OPS5 `//` quote operator is disallowed in `ppwm`. | Rejected with parse error: `ppwm pattern cannot contain the quote operator '//'` |
| **Angle Brackets (`<`, `>`)** | Angle brackets are reserved for variables and predicates. | Rejected with parse error: `ppwm pattern cannot contain angle brackets` |
| **Curly Braces (`{`, `}`)** | Conjunction blocks (e.g. `{ > 0 < 100 }`) are not permitted. | Rejected with parse error: `ppwm pattern cannot contain curly braces` |
| **Negation (`-`)** | Negated condition patterns test absence rather than selecting elements to display. | Rejected with parse error: `ppwm pattern cannot contain negation '-'` |

---

## 3. Supported Pattern Matching Features

1. **Class Filter**: Matches the WME class name (case-insensitive). Specifying `*` matches across all classes.
2. **Named Attribute Tests**: `^attribute value` tests equality against symbol, numeric, string, or boolean values.
3. **`nil` Attribute Matching**: `^attribute nil` matches WMEs where the attribute is either unassigned or explicitly set to `nil`.
4. **Vector Attributes**: If an attribute is declared with `(vector-attribute ...)`, testing a scalar value checks for membership within the vector. Specifying multiple values tests the exact vector sequence.
5. **Positional Attributes**: When a schema has been declared with `(literalize ...)`, attributes may be specified positionally without `^` carets.

---

## 4. Programmatic Go API

The engine exposes methods for pattern-matching working memory elements programmatically:

```go
// FindWMEsMatching returns all active WMEs matching the given condition element pattern, ordered by timetag.
func (e *Engine) FindWMEsMatching(pattern *model.ConditionElement) []*model.WME

// PrintPPWM returns the string representations of all active WMEs matching the pattern.
func (e *Engine) PrintPPWM(pattern *model.ConditionElement) []string

// PPWM parses a ppwm pattern string, validates that it complies with OPS5 ppwm restrictions,
// and returns all matching active working memory elements.
func (e *Engine) PPWM(patternSrc string) ([]*model.WME, error)
```

#### Example
```go
eng := engine.New()
eng.Make("City", map[string]model.Value{
    "name":  model.NewSymbol("Pittsburgh"),
    "state": model.NewSymbol("Pennsylvania"),
})

wmes, err := eng.PPWM("City ^state Pennsylvania")
if err != nil {
    log.Fatal(err)
}
for _, w := range wmes {
    fmt.Println(w.String())
}
```

---

## 5. Memory Inspection Comparison Matrix

| Command | Target Memory | Description | Example Output |
| :--- | :--- | :--- | :--- |
| **`ppwm`** | Working Memory (Filtered) | Prints all active WMEs matching an LHS condition element pattern. | `(1: City ^name Pittsburgh ^state Pennsylvania)` |
| **`wm`** | Working Memory (All / Class) | Lists active WMEs with timetags, classes, and attribute-value pairs. | `(1: Request ^target Homer ^type ancestor)` |
| **`pm`** | Production Memory | Pretty-prints rule LHS conditions and RHS actions in valid OPS5 syntax. | `(p r1 (item ^id <x>) --> (halt))` |
| **`cs`** | Conflict Set | Lists un-fired rule activations in salience order. | `* [1] FindAncestors: (1, 2)` |

---

## 6. Related References

- [OPS5 Syntax & Keyword Reference](keywords.md)
- [Print Production Memory Reference (`pm`)](pm.md)
- [LHS Pattern Matching & Rete Network](lhs_patterns.md)
- [Working Memory Initialization & Testing Tutorial](tutorial_2_5_testing.md)
