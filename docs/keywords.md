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
| **`openfile`** | `(openfile <log-name> <filespec> <mode>)` | Opens a file stream for reading, writing, or appending. See [`file_io` Reference](file_io.md). | `(openfile ruletrace \|RuleTrace.ops\| out)` |
| **`closefile`** | `(closefile <log-name>)` | Closes an open file stream. See [`file_io` Reference](file_io.md). | `(closefile ruletrace)` |
| **`default`** | `(default <log-name> <subsystem>)` | Redirects default stream for `accept`, `write`, or `trace`. See [`file_io` Reference](file_io.md). | `(default ruletrace accept)` |
| **`excise`** | `(excise <rule1> ... <ruleN>)` | Evicts production rules from production memory, detaches their terminal nodes from the Rete network, and purges all pending activations and refraction history from the conflict set. Existing WMEs are preserved. See [`excise` Reference](excise.md). | `(excise detect-item cleanup-task)` |
| **`pm`** | `(pm [<rule1> ... <ruleN> \| *])` | Pretty-prints the source text of specified production rule(s) or all rules (`*`) currently held in production memory. See [`pm` Reference](pm.md). | `(pm FindAncestors)`<br>`(pm *)` |
| **`ppwm`** | `(ppwm [<class> [^<attr> <val> ...]] \| *)` | Filters and prints active working memory elements matching an LHS condition pattern. See [`ppwm` Reference](ppwm.md). | `(ppwm City ^state Pennsylvania)` |
| **`remove`** | `(remove [<timetag...> \| *])` | Retracts specific WME(s) by timetag or all WMEs (`*`) from working memory. | `(remove *)`<br>`(remove 1 2)` |
| **`build`** | `(build (p <name> ...))` | Dynamically parses and compiles a production rule into the Rete network at the top level or REPL. | `(build (p rule1 (task) --> (halt)))` |
| **`matches`** | `(matches [<rule1> ... <ruleN> \| *])` | Displays partial Rete matches (alpha condition matches, intermediate beta join tokens, and conflict set activations) for production rules. | `(matches detect-item)`<br>`(matches *)` |
| **`pbreak`** | `(pbreak [<rule1> ... <ruleN>])` | Sets execution breakpoints on specified production rules, suspending `run` execution prior to firing. If called with no arguments, lists active breakpoints. | `(pbreak detect-item)`<br>`(pbreak)` |
| **`unpbreak`** / **`unbreak`** | `(unpbreak [<rule1> ... \| *])` | Removes execution breakpoints from specified rules, or clears all breakpoints (`*` or no arguments). | `(unpbreak detect-item)`<br>`(unpbreak *)` |
| **`watch`** | `(watch [0 \| 1 \| 2])` | Configures or displays the engine trace level (0=silent, 1=rule firings with timetags, 2=rule firings and WM assertions/retractions). Default is 1. | `(watch)`<br>`(watch 2)` |

---

## 2. RHS Action Verbs

These action verbs execute sequentially when a production rule fires.

| Action Verb | Syntax Form | Description | Example |
| :--- | :--- | :--- | :--- |
| **`make`** | `(make <class> [^<attr> <val> ...])` | Asserts a new WME into working memory with a monotonically increasing timetag. | `(make task ^id <new-id> ^status ready)` |
| **`modify`** | `(modify <target> [^<attr> <val> ...])` | Modifies an existing WME (retracts and re-asserts with a new timetag, preserving unmodified attributes). See [`modify` Reference](modify.md). | `(modify <t> ^status complete)`<br>`(modify 1 ^status complete)` |
| **`remove`** | `(remove <target...> \| *)` | Retracts existing WME(s) from working memory. `<target>` is an element variable, 1-based CE index, or `*` to clear all working memory. | `(remove <t>)`<br>`(remove 2)`<br>`(remove *)` |
| **`write`** | `(write <val1> ...)` | Emits values, strings, or resolved variables to output. Supports `(crlf)` and `(tabto N)`. See [`write` Reference](write.md). | `(write (crlf) (tabto 5) <id> (tabto 20) <name> (crlf))` |
| **`crlf`** | `(crlf)` or `crlf` | Directs `(write ...)` to emit a newline and reset horizontal column counter to 1. | `(write (crlf) "Start")` |
| **`tabto`** | `(tabto <col>)` | Directs `(write ...)` to pad output with spaces until column `<col>` is reached. | `(write (tabto 15) "Column 2")` |
| **`bind`** | `(bind <var> <val-or-expr>)` | Evaluates a value or `compute` expression and binds it to a local variable for subsequent actions. See [`bind` Reference](bind.md). | `(bind <total> (compute <subtotal> + <tax>))` |
| **`cbind`** | `(cbind <elem-var>)` | Binds the last element added to working memory (by `make`, `modify`, or `call`) to an element variable. See [`bind` Reference](bind.md#5-cbind-action-element-variable-binding). | `(make item ^id 1)`<br>`(cbind <it>)`<br>`(modify <it> ^status active)` |
| **`compute`** | `(compute <op1> <op> <op2> ...)` | Evaluates arithmetic expressions (`+`, `-`, `*`, `/`, `//`, `\`, `%`). Supports nesting and unary minus. See [`bind` Reference](bind.md). | `(compute <price> * <qty>)`<br>`(compute <p> + (compute <p> * <r>))` |
| **`openfile`** | `(openfile <log-name> <filespec> <mode>)` | Opens a file stream for `in`, `out`, or `append`. See [`file_io` Reference](file_io.md). | `(openfile ruletrace \|RuleTrace.ops\| out)` |
| **`closefile`** | `(closefile <log-name>)` | Closes an open file stream. See [`file_io` Reference](file_io.md). | `(closefile ruletrace)` |
| **`default`** | `(default <log-name> <subsystem>)` | Redirects default stream for `accept`, `write`, or `trace`. See [`file_io` Reference](file_io.md). | `(default ruletrace accept)` |
| **`accept`** | `(accept [<log-name>])` | RHS value function reading the next whitespace-delimited atom from input stream. See [`file_io` Reference](file_io.md). | `(make user ^id (accept))` |
| **`acceptline`** | `(acceptline [<log-name>])` | RHS value function reading a full line of text into a scalar or vector. See [`file_io` Reference](file_io.md). | `(make data ^tokens (acceptline))` |
| **`genatom`** | `(genatom)` | RHS function generating a unique symbolic atom (`atom1`, `atom2`, ...). See [`genatom` Reference](genatom.md). | `(make node ^id (genatom))` |
| **`litval`** | `(litval [<class>] <attr>)` | RHS function returning the numeric index (2, 3, ...) of an attribute. See [`litval` Reference](litval.md). | `(make meta ^slot (litval name))` |
| **`substr`** | `(substr <elem> <start> <end>)` | RHS function extracting a subsequence or single value from a WME. See [`substr` Reference](substr.md). | `(substr <str> sequence sequence)`<br>`(substr <str> 3 inf)` |
| **`build`** | `(build (p <name> ...))` | Dynamically synthesizes and compiles a production rule into Rete at runtime, substituting variables bound in the parent rule. | `(build (p shortcut (traveler ^dest <d>) --> (write "Direct route to" <d>)))` |
| **`halt`** | `(halt)` | Halts the inference engine execution loop immediately. Current cycle completes, but no further rules fire. | `(halt)` |
| **`watch`** | `(watch [0 \| 1 \| 2])` | Modifies or displays the engine trace level dynamically during rule execution. | `(watch 2)`<br>`(watch 0)` |


---

## 3. LHS Condition Elements & Syntactic Operators

These keywords, delimiters, and operators are recognized in rule condition patterns on the Left-Hand Side (LHS) of a production rule. For complete documentation on condition patterns, variable joins, and universal quantification, see the [LHS Pattern Matching Reference](lhs_patterns.md) and [Design Patterns & Idioms](idioms.md).

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
| **`<< ... >>`** | Disjunction block | Matches if attribute satisfies ANY of the enclosed values, operators, or bound variables. | `(item ^status << active pending >>)`<br>`(sensor ^temp << < 10 >= 100 >>)` |
| **`{ ... }`** | Conjunction block | Matches if attribute satisfies ALL enclosed relational constraints and variable bindings. | `(sensor ^reading { >= 50 <= 100 })` |
| **`true`**, **`false`** | Boolean literals | First-class boolean values (case-insensitive) for scalar and vector attributes. | `(sensor ^active true)` |
| **`nil`** | Value literal | Represents unset/null attributes when an attribute is specified without a value. | `(modify <g> ^result nil)` |
| **`;`** | Comment marker | Line comment running to the end of the line. | `; Process next batch item` |

---

## 4. Interactive REPL Commands

The interactive CLI shell (`ops5`) supports both bare words and paren-enclosed command forms:

| Command | Arguments | Description |
| :--- | :--- | :--- |
| **`wm`** | `[class] [--table]` | Prints active Working Memory Elements (optionally filtered by class or in a boxed table). |
| **`cs`** | `[--table]` | Prints the active conflict set (agenda) sorted by salience (optionally in a boxed table). |
| **`schemas`** / **`schema`** | `[class] [--table]` | Prints registered class schemas and their vector attributes (optionally in a boxed table). |
| **`status`** / **`info`** | _none_ | Displays runtime status overview (rules count, WME count, strategy, dominant rule, etc.). |
| **`table`** | `[on\|off]` | Displays or toggles global boxed tabular formatting mode for `wm`, `cs`, and `schemas`. |
| **`clear`** / **`cls`** | _none_ | Clears the terminal screen (also available via `Ctrl-L`). |
| **`step`** | _none_ | Executes a single Match-Resolve-Act cycle. |
| **`run`** | `[N]` | Runs rules until quiescence, `(halt)`, or `N` cycles. |
| **`make`** | `<class> [^attr val ...]` | Asserts a new WME from the REPL prompt with syntax highlighting. |
| **`modify`** | `<timetag> [^attr val ...]` | Modifies an existing WME by its timetag. |
| **`remove`** | `<timetag...> \| *` | Retracts specific WME(s) by timetag or all WMEs (`*`). |
| **`openfile`** | `<logical-name> <filespec> <mode>` | Opens a file stream (`in`, `out`, `append`). Supports `\|...\|`. |
| **`closefile`** | `<logical-name>` | Closes an open file stream. |
| **`default`** | `[<logical-name> <subsystem>]` | Sets or displays default streams for `accept`, `write`, `trace`. |
| **`genatom`** | _none_ | Generates and displays a unique symbolic atom (e.g. `atom1`). |
| **`litval`** | `[<class>] <attr>` | Displays the numeric index assigned to an attribute name. |
| **`substr`** | `<elem> <start> <end>` | Extracts a subsequence or value from a working memory element. See [`substr` Reference](substr.md). |
| **`literalize`** | `<class> <attr1> ...` | Declares a class schema with positional attribute layout. |
| **`vector-attribute`** | `<attr1> ...` | Declares attribute(s) as multi-valued vector attributes. |
| **`vector-attributes`** | _none_ | Lists all registered vector attributes. |
| **`strategy`** | `[lex\|mea]` | Displays or sets conflict resolution strategy (`LEX` or `MEA`). See [Selection Strategy Reference](conflict_resolution.md). |
| **`trace`** | `on\|off` | Toggles rule firing execution traces (legacy alias for `watch 1` / `watch 0`). |
| **`watch`** | `[0\|1\|2]` | Displays or sets trace level (0=silent, 1=firings, 2=firings + WM changes). |
| **`load`** | `<file.ops>` | Loads and parses an external OPS5 source file. |
| **`excise`** | `<rule1> [rule2 ...]` | Evicts production rule(s) by name from production memory, detaches terminal nodes from Rete network, and purges pending activations from the conflict set. See [`excise` Reference](excise.md). |
| **`pm`** | `[<rule1> ... \| *]` | Pretty-prints the source text of specified production rule(s) or all rules (`*`). See [`pm` Reference](pm.md). |
| **`ppwm`** | `[<class> [^attr val...]] \| *` | Prints active working memory elements matching an LHS condition pattern. See [`ppwm` Reference](ppwm.md). |
| **`matches`** | `[<rule1> ... \| *]` | Displays diagnostic partial matches (alpha WMEs, beta join tokens, conflict set activations) for rules. |
| **`pbreak`** | `[<rule1> ...]` | Sets execution breakpoints on production rules, or lists breakpoints if no arguments are given. |
| **`unpbreak`** / **`unbreak`** | `[<rule1> ... \| *]` | Removes rule breakpoints or clears all breakpoints (`*` or no arguments). |
| **`test`** | `<file.json>` | Executes a JSON test harness case. |
| **`reset`** | _none_ | Clears working memory, network state, and conflict set. |
| **`help`** | _none_ | Displays interactive REPL help. |
| **`exit`** / **`quit`** | _none_ | Exits the interactive shell. |

### REPL GUI & Interactive Enhancements
- **Syntax Highlighting & ANSI Colors**: Color-coded prompts, class identifiers, caret attributes (`^attr`), values (numbers, strings, booleans, symbols), and status headers. Supports automatic terminal detection, `NO_COLOR`, and `--color=auto|always|never`.
- **Tab Auto-Completion**: Contextual tab completion for base commands, class schemas, attributes (`^...`), rules for `excise`/`pm`/`matches`/`pbreak`/`unpbreak`, strategies (`lex`/`mea`), trace levels (`0`, `1`, `2`), and file paths (`.ops`, `.json`).
- **Command History**: Persistent command line history saved to `~/.ops5_history` with `Up`/`Down` arrow navigation and duplicate suppression.
- **Readline Line Editor**: Full cursor navigation (`Left`/`Right`/`Home`/`End`), deletion (`Backspace`, `Delete`), and shortcuts (`Ctrl-A`, `Ctrl-E`, `Ctrl-K`, `Ctrl-U`, `Ctrl-L`, `Ctrl-C`, `Ctrl-D`).
- **Boxed Tabular Mode**: Formatted Unicode/ASCII tables for `wm`, `cs`, and `schemas` with exact visual column width calculation (`wm --table`, `cs --table`, `schemas --table`, or global toggle `table on`).

---

## 5. Classic OPS5 Keywords & Roadmap Status

All core language features, RHS action verbs, I/O subsystems, pattern matching constructs (including disjunctions, conjunctions, negative conditions, NCC blocks, and existential quantification), and developer diagnostic tools (`pm`, `ppwm`, `matches`, `pbreak`, `unpbreak`, `watch`, `excise`) from the classic Charles Forgy OPS5 specification are fully implemented.


