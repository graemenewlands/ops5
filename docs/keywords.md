# OPS5 Syntax & Keyword Reference

This document provides a comprehensive reference of all keywords, directives, RHS action verbs, LHS condition operators, and REPL commands supported by this OPS5 implementation.

---

## 1. Top-Level Directives & Statements

These keywords appear at the root level of `.ops` source files or directly within the interactive REPL.

| Keyword | Syntax Form | Description | Example |
| :--- | :--- | :--- | :--- |
| **`p`** / **`P`** | `(p <rule-name> <LHS> --> <RHS>)` | Defines a production rule with conditions and actions. | `(p detect-item (item ^status pending) --> (write "Found item"))` |
| **`literalize`** | `(literalize <class> <attr1> ... <attrN>)` | Declares a class schema and its positional attribute layout. | `(literalize City name location state country population)` |
| **`vector-attribute`** | `(vector-attribute <attr1> ... <attrN>)` | Designates attributes as multi-valued vectors capable of holding sequences of values. | `(vector-attribute location coords)` |
| **`make`** | `(make <class> [^<attr> <val> ...] [val1 ...])` | Asserts a working memory element (WME) into initial working memory. | `(make City ^name Boston ^location 42.36 -71.05)` |

---

## 2. RHS Action Verbs

These action verbs execute sequentially when a production rule fires.

| Action Verb | Syntax Form | Description | Example |
| :--- | :--- | :--- | :--- |
| **`make`** | `(make <class> [^<attr> <val> ...])` | Asserts a new WME into working memory with a monotonically increasing timetag. | `(make task ^id <new-id> ^status ready)` |
| **`modify`** | `(modify <target> [^<attr> <val> ...])` | Modifies an existing WME (retracts and re-asserts with a new timetag, preserving unmodified attributes). See [`modify` Reference](modify.md). | `(modify <t> ^status complete)`<br>`(modify 1 ^status complete)` |
| **`remove`** | `(remove <target>)` | Retracts an existing WME from working memory. `<target>` is an element variable or 1-based CE index. | `(remove <t>)`<br>`(remove 2)` |
| **`write`** | `(write <val1> ...)` | Emits values, strings, or resolved variables to output. Supports `(crlf)` and `(tabto N)`. See [`write` Reference](write.md). | `(write (crlf) (tabto 5) <id> (tabto 20) <name> (crlf))` |
| **`crlf`** | `(crlf)` or `crlf` | Directs `(write ...)` to emit a newline and reset horizontal column counter to 1. | `(write (crlf) "Start")` |
| **`tabto`** | `(tabto <col>)` | Directs `(write ...)` to pad output with spaces until column `<col>` is reached. | `(write (tabto 15) "Column 2")` |
| **`bind`** | `(bind <var> <val-or-expr>)` | Evaluates a value or `compute` expression and binds it to a local variable for subsequent actions. See [`bind` Reference](bind.md). | `(bind <total> (compute <subtotal> + <tax>))` |
| **`cbind`** | `(cbind <elem-var>)` | Binds the last element added to working memory (by `make`, `modify`, or `call`) to an element variable. See [`bind` Reference](bind.md#5-cbind-action-element-variable-binding). | `(make item ^id 1)`<br>`(cbind <it>)`<br>`(modify <it> ^status active)` |
| **`compute`** | `(compute <op1> <op> <op2> ...)` | Evaluates arithmetic expressions (`+`, `-`, `*`, `/`, `//`, `\`, `%`). Supports nesting and unary minus. See [`bind` Reference](bind.md). | `(compute <price> * <qty>)`<br>`(compute <p> + (compute <p> * <r>))` |
| **`halt`** | `(halt)` | Halts the inference engine execution loop immediately. Current cycle completes, but no further rules fire. | `(halt)` |

---

## 3. LHS Condition Elements & Syntactic Operators

These keywords, delimiters, and operators are recognized in rule condition patterns on the Left-Hand Side (LHS) of a production rule.

| Symbol / Keyword | Role / Context | Description | Example |
| :--- | :--- | :--- | :--- |
| **`-->`** | Rule separator | Delimits the Left-Hand Side (LHS) conditions from the Right-Hand Side (RHS) actions. | `(p sample (goal) --> (halt))` |
| **`-`** | Condition negation | Prefixes a condition element to test for the **absence** of matching WMEs. | `-(goal ^status pending)` |
| **`^`** | Attribute identifier | Prefixes attribute names within condition elements and actions. | `^name`, `^status`, `^priority` |
| **`<...>`** | Variable delimiter | Denotes variable bindings in attribute tests, or element variable bindings prefixing condition elements. | `<id>`, `<g> (goal ^status active)` |
| **`=`** | Relational test | Equality comparison (default test if operator is omitted). | `(item ^count = 5)` |
| **`<>`**, **`!=`** | Relational test | Inequality / not-equal comparison. | `(item ^status <> finished)` |
| **`<`** | Relational test | Strictly less than (numeric or lexicographical). | `(sensor ^temp < 100)` |
| **`<=`** | Relational test | Less than or equal to. | `(sensor ^temp <= 100)` |
| **`>`** | Relational test | Strictly greater than. | `(priority ^level > 1)` |
| **`>=`** | Relational test | Greater than or equal to. | `(priority ^level >= 1)` |
| **`true`**, **`false`** | Boolean literals | First-class boolean values (case-insensitive) for scalar and vector attributes. | `(sensor ^active true)` |
| **`nil`** | Value literal | Represents unset/null attributes when an attribute is specified without a value. | `(modify <g> ^result nil)` |
| **`;`** | Comment marker | Line comment running to the end of the line. | `; Process next batch item` |

---

## 4. Interactive REPL Commands

The interactive CLI shell (`ops5`) supports both bare words and paren-enclosed command forms:

| Command | Arguments | Description |
| :--- | :--- | :--- |
| **`wm`** | `[class]` | Prints active Working Memory Elements (optionally filtered by class). |
| **`cs`** | _none_ | Prints the active conflict set (agenda) sorted by salience. |
| **`step`** | _none_ | Executes a single Match-Resolve-Act cycle. |
| **`run`** | `[N]` | Runs rules until quiescence, `(halt)`, or `N` cycles. |
| **`make`** | `<class> [^attr val ...]` | Asserts a new WME from the REPL prompt. |
| **`modify`** | `<timetag> [^attr val ...]` | Modifies an existing WME by its timetag. |
| **`remove`** | `<timetag>` | Retracts a WME by its timetag. |
| **`literalize`** | `<class> <attr1> ...` | Declares a class schema with positional attribute layout. |
| **`schemas`** / **`schema`** | `[class]` | Prints registered class schemas and their vector attributes. |
| **`vector-attribute`** | `<attr1> ...` | Declares attribute(s) as multi-valued vector attributes. |
| **`vector-attributes`** | _none_ | Lists all registered vector attributes. |
| **`strategy`** | `[lex\|mea]` | Displays or sets the conflict resolution strategy (`LEX` or `MEA`). |
| **`trace`** | `on\|off` | Toggles rule firing execution traces. |
| **`load`** | `<file.ops>` | Loads and parses an external OPS5 source file. |
| **`test`** | `<file.json>` | Executes a JSON test harness case. |
| **`reset`** | _none_ | Clears working memory, network state, and conflict set. |
| **`help`** | _none_ | Displays interactive REPL help. |
| **`exit`** / **`quit`** | _none_ | Exits the interactive shell. |

---

## 5. Classic OPS5 Keywords Not Yet Implemented (Roadmap)

For reference and future engine development, the following standard OPS5 constructs from the classic Charles Forgy specification are planned or tracked for future implementation:

### RHS Functions & Computations
- **`substr`**: Extracts a sub-vector or substring from a value, e.g. `(substr <vec> 1 2)`.
- **`genatom`**: Generates a unique symbol atom (e.g., `atom1`, `atom2`).
- **`accept`** / **`acceptline`**: Reads user input from standard input on the RHS.

### LHS Compound Matchers
- **`{ ... }`**: Conjunction block restricting an attribute to multiple bounds, e.g. `^val { > 0 < 100 }`.
- **`<< ... >>`**: Disjunction block matching any listed symbol or value, e.g. `^status << active pending >>`.

### Control & Diagnostic Directives
- **`watch`**: Configures fine-grained tracing levels (e.g. `(watch 0)`, `(watch 1)`, `(watch 2)`).
- **`matches`**: Displays partial Rete matches for a specific rule.
- **`pm`**: Prints productions matching a pattern.
- **`pbreak`**: Sets a breakpoint on a production rule.
