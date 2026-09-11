# OPS5 `pm` Directive & Command Reference

The `pm` ("print production memory") command inspects and pretty-prints the source representation of production rules currently registered in production memory. It is the production memory counterpart to `wm` (which inspects working memory elements) and `cs` (which inspects the conflict set agenda).

---

## 1. Syntax

### 1.1 Source File Directive (`.ops` / `.ops5`)

In rule files, `pm` can be declared as an S-expression at the top level:

```ops5
(pm <rule-name-1> [<rule-name-2> ...])
(pm *)
(pm)
```

- If rule names are specified, each specified rule is printed in definition order.
- If `*` is specified or if no arguments are provided, **all** rules in production memory are printed.

#### Example
```ops5
(p FindAncestors
   (Request ^type ancestor ^target <p>)
   (Person ^name <p> ^father <f>)
   -->
   (make Request ^type ancestor ^target <f>)
   (write (crlf) <f> "is an ancestor")
)

; Print the definition of FindAncestors to output
(pm FindAncestors)
```

---

### 1.2 Interactive CLI REPL

The interactive REPL supports both bare-word and S-expression forms:

```text
ops5> pm <rule-name-1> [<rule-name-2> ...]
ops5> pm *
ops5> (pm <rule-name-1> ...)
ops5> (pm *)
```

#### Example Session
```text
ops5> (p detect-leaf (Person ^name <n>) -(Person ^father <n>) --> (write (crlf) <n> "is a leaf"))
Defined rule 'detect-leaf' (conditions=2, specificity=3)
ops5> pm detect-leaf
(p detect-leaf
   (Person ^name <n>)
   -(Person ^father <n>)
   -->
   (write (crlf) <n> "is a leaf")
)
ops5> pm *
(p detect-leaf
   (Person ^name <n>)
   -(Person ^father <n>)
   -->
   (write (crlf) <n> "is a leaf")
)
ops5> pm unknown-rule
Rule 'unknown-rule' not found
```

---

### 1.3 Programmatic Go API

The engine exposes methods for inspecting and pretty-printing rules directly:

```go
// PrintRule returns the pretty-printed OPS5 source text of a production rule by name.
// Returns (text, true) if found, or ("", false) if not found.
func (e *Engine) PrintRule(name string) (string, bool)

// PrintRules returns the pretty-printed OPS5 source text of the specified rules.
// If names is empty or contains "*", all production rules currently in production memory are returned.
func (e *Engine) PrintRules(names ...string) []string
```

#### Example
```go
eng := engine.New()
// ... load or define rules ...

if text, ok := eng.PrintRule("FindAncestors"); ok {
    fmt.Println(text)
}
```

---

## 2. Memory Inspection Comparison Matrix

| Command | Target Memory | Description | Example Output |
| :--- | :--- | :--- | :--- |
| **`pm`** | Production Memory | Pretty-prints rule LHS conditions and RHS actions in valid OPS5 syntax. | `(p r1 (item ^id <x>) --> (halt))` |
| **`wm`** | Working Memory | Lists active WMEs with timetags, classes, and attribute-value pairs. | `(1: Request ^target Homer ^type ancestor)` |
| **`cs`** | Conflict Set | Lists un-fired rule activations in salience order. | `* [1] FindAncestors: (1, 2)` |

---

## 3. Related References

- [OPS5 Syntax & Keyword Reference](keywords.md)
- [Rule Excision Reference (`excise`)](excise.md)
- [LHS Pattern Matching & Rete Network](lhs_patterns.md)
- [Conflict Resolution Strategies (LEX & MEA)](conflict_resolution.md)
