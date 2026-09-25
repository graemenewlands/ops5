# OPS5 Go Runtime & Engine Specification

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/graemenewlands/ops5.svg)](https://pkg.go.dev/github.com/graemenewlands/ops5)

A minimal, robust, and high-performance Go implementation of Charles Forgy's classic **OPS5** production rule system using the **Rete** pattern matching algorithm.

This engine provides a complete, modern execution environment for rule-based systems, featuring dynamic working memory, Rete alpha/beta network compilation with structural node sharing, negative condition elements, LEX and MEA conflict resolution strategies with strict refraction, retroactive rule compilation, an interactive CLI REPL, cycle tracing, and an automated JSON test harness.

---

## Table of Contents

1. [Architectural Overview](#architectural-overview)
2. [Language & Syntax Specification](#language--syntax-specification)
   - [Keywords & Syntax Quick Reference](docs/keywords.md)
   - [File I/O, Stream Redirection & Input](docs/file_io.md)
   - [Unique Atom Generation (`genatom`)](docs/genatom.md)
   - [Attribute Index Resolution (`litval`)](docs/litval.md)
   - [Subsequence & Vector Extraction (`substr`)](docs/substr.md)
   - [LHS Condition Elements & Pattern Matching](docs/lhs_patterns.md)
   - [Design Patterns & Programming Idioms](docs/idioms.md)
   - [Lexical Elements & Data Types](#lexical-elements--data-types)
   - [Schema & Vector Declarations (`literalize`, `vector-attribute`)](#schema--vector-declarations-literalize-vector-attribute)
   - [Working Memory Elements (WMEs)](#working-memory-elements-wmes)
   - [Production Rules (`(p ... )`)](#production-rules-p--)
   - [Left-Hand Side (LHS) Condition Elements](#left-hand-side-lhs-condition-elements)
   - [Right-Hand Side (RHS) Actions](#right-hand-side-rhs-actions)
3. [Rete Pattern Matching Engine](#rete-pattern-matching-engine)
   - [Beta Node Types & Architecture Reference](docs/beta_nodes.md)
   - [Beta Node Types & Implementation Matrix](#beta-node-types--implementation-matrix)
   - [Alpha Network Mechanics](#alpha-network-mechanics)
   - [Beta Network & Join Mechanics](#beta-network--join-mechanics)
   - [Hashed Beta & Alpha Join Indexing](#hashed-beta--alpha-join-indexing)
   - [Structural Beta Node Sharing](#structural-beta-node-sharing)
   - [Node Unlinking (Left & Right Unlinking)](#node-unlinking-left--right-unlinking)
   - [Negative Condition Elements](#negative-condition-elements)
   - [Dynamic & Retroactive Compilation](#dynamic--retroactive-compilation)
4. [Conflict Resolution & Execution Lifecycle](#conflict-resolution--execution-lifecycle)
   - [Selection Strategies (LEX & MEA) Reference](docs/conflict_resolution.md)
   - [Match-Resolve-Act Cycle](#match-resolve-act-cycle)
   - [Refraction Semantics](#refraction-semantics)
   - [LEX Strategy (Lexicographic)](#lex-strategy-lexicographic)
   - [MEA Strategy (Means-Ends Analysis)](#mea-strategy-means-ends-analysis)
   - [Rule Specificity Calculation](#rule-specificity-calculation)
5. [CLI & Interactive REPL Guide](#cli--interactive-repl-guide)
   - [Installation & Build](#installation--build)
   - [CLI Commands & Flags](#cli-commands--flags)
   - [Interactive REPL Reference](#interactive-repl-reference)
   - [Tutorial: Ancestors Search in the REPL (Section 2.4.3)](docs/tutorial_2_4_3_ancestors.md)
   - [Tutorial: Working Memory Initialization & Test Harness (Section 2.5)](docs/tutorial_2_5_testing.md)
   - [Tutorial: Monkey & Bananas Problem (Section 3)](docs/tutorial_3_monkey_bananas.md)
   - [Program Termination & Halting Reference](docs/program_termination.md)
   - [Execution Tracing](#execution-tracing)
6. [Test Harness & JSON Test Suite Specification](#test-harness--json-test-suite-specification)
   - [Test Case Schema](#test-case-schema)
   - [Fixture Example](#fixture-example)
   - [Running Test Suites](#running-test-suites)
7. [Programmatic Go API Reference](#programmatic-go-api-reference)
   - [Quick Start Example](#quick-start-example)
   - [Package Breakdown](#package-breakdown)
   - [Partitioned Multi-Core Concurrency (ParaOPS5)](#partitioned-multi-core-concurrency-paraops5)
   - [Extensibility & Custom Actions](#extensibility--custom-actions)
8. [Canonical Benchmark Suite](#canonical-benchmark-suite)
   - [Engine Comparison & Architectural Evaluation](docs/engine_comparison.md)
   - [Benchmark Timing History & Release Log](docs/benchmarks_history.md)
   - [Benchmark Problems (Manners, Waltz, Zebra)](#benchmark-problems-manners-waltz-zebra)
   - [Running the Benchmarks](#running-the-benchmarks)
   - [Benchmark Results & Performance Profile](#benchmark-results--performance-profile)
9. [License](#license)

---

## Architectural Overview

```mermaid
flowchart TD
    subgraph WM["Dynamic Working Memory"]
        Assert["WME Assertion (Make)"]
        Retract["WME Retraction (Remove/Modify)"]
        Timetag["Monotonic Timetag Generator"]
    end

    subgraph Rete["Rete Network"]
        AlphaRoot["Alpha Root Node"]
        TypeNode["TypeNode (Class Filter)"]
        AlphaTest["ConstantTestNode (=, <>, <, <=, >, >=)"]
        AlphaMem["AlphaMemory (Shared Intra-Condition Cache)"]
        BetaRoot["Beta Root Memory (Dummy Token)"]
        JoinNode["JoinNode / NegativeJoinNode"]
        BetaMem["BetaMemory (Partial Matches)"]
        TerminalNode["TerminalNode (Full LHS Match)"]
    end

    subgraph Agenda["Conflict Set (Agenda)"]
        CS["Indexed Binary Heap Agenda"]
        Refraction["Refraction Table (Fired Keys)"]
        Strategy["Strategy Evaluator (LEX / MEA)"]
    end

    subgraph Exec["Match-Resolve-Act Cycle"]
        Select["Select Dominant Instantiation"]
        Fire["Fire Dominant Rule & Mark Refracted"]
        RHS["Precompiled RHS Closures (ActionContext Pool)"]
    end

    Timetag --> Assert
    Assert --> AlphaRoot
    Retract --> AlphaRoot
    AlphaRoot --> TypeNode
    TypeNode --> AlphaTest
    AlphaTest --> AlphaMem

    BetaRoot --> JoinNode
    AlphaMem --> JoinNode
    JoinNode -->|Intermediate Join| BetaMem
    JoinNode -->|"Terminal Join Shortcut (Rete-NT)"| TerminalNode
    BetaMem --> TerminalNode

    TerminalNode -->|Activation Add/Remove| CS
    CS --> Strategy
    Strategy --> Select
    Select --> Fire
    Fire --> Refraction
    Fire --> RHS
    RHS -->|make/modify/remove| WM
```

The runtime strictly decouples the pattern-matching network from the working memory store and agenda manager via event-driven interfaces:
- **`wm.WorkingMemory`**: Maintains active WMEs indexed by immutable 64-bit integer timetags. Dispatches `OnAssert` and `OnRetract` events.
- **`rete.Network`**: Maintains shared alpha chains and beta trees. Transforms WME additions/removals into token streams.
- **`conflict.Set`**: Maintains active instantiations, sorts them according to salience policies, and prevents duplicate firings via refraction.
- **`engine.Engine`**: Drives the execution loop, evaluates precompiled RHS action closures with pooled zero-allocation contexts (`ActionContext`), and invokes working memory actions.

---

## Language & Syntax Specification

> [!TIP]
> For an exhaustive, quick-reference table of all top-level directives, RHS actions, LHS operators, REPL commands, and roadmap items, see the [OPS5 Syntax & Keyword Reference](docs/keywords.md).

### Lexical Elements & Data Types

The tokenizer supports standard OPS5 S-expression syntax. Whitespace and newlines serve as token delimiters. Semicolons denote line comments:
```ops5
; This is an OPS5 comment line
```

The engine supports first-class data types:

| Data Type | Syntax Pattern | Go Representation | Examples |
| :--- | :--- | :--- | :--- |
| **Symbol** | Unquoted alphanumeric sequence | `model.TypeSymbol` (`string`) | `active`, `pending`, `goal`, `item-10` |
| **Integer** | Optional sign with digits | `model.TypeInteger` (`int64`) | `0`, `42`, `-101`, `1000000` |
| **Float** | Floating-point decimal | `model.TypeFloat` (`float64`) | `3.14`, `0.001`, `-12.5` |
| **String** | Double-quoted text | `model.TypeString` (`string`) | `"Hello World"`, `"Batch complete"` |
| **Boolean** | Case-insensitive boolean literals | `model.TypeBoolean` (`bool`) | `true`, `false` |
| **Vector** | Space-separated sequence of values | `model.TypeVector` (`[]model.Value`) | `42.36 -71.05`, `"Beantown" "The Hub"` |
| **Variable** | Delimited by angle brackets `<...>` | `model.TypeVariable` (`string`) | `<x>`, `<id>`, `<goal-ptr>`, `<val>` |

### Schema & Vector Declarations (`literalize`, `vector-attribute`)

#### Class Schema Declarations (`(literalize ...)`)

OPS5 programs can declare class schemas using the top-level `(literalize ...)` directive:

```ops5
(literalize <class-name> <attr-1> <attr-2> ... <attr-n>)
```

Attribute names can be declared as bare symbols or prefixed with a caret `^`:
```ops5
(literalize vector x y z)
(literalize point ^x ^y)
```

##### Positional Attribute Mapping
Declaring a schema activates classic OPS5 positional attribute mapping. In classic OPS5, WMEs were represented internally as fixed-width vectors based on their `literalize` declaration. With a registered schema:
- **Positional `make` actions**: Values provided without an explicit attribute name are mapped sequentially to the declared attribute slots:
  ```ops5
  (make point 10 20)
  ; Automatically maps to: (make point ^x 10 ^y 20)
  ```
- **Positional LHS condition elements**: Condition patterns without explicit `^` attributes test positional values against the corresponding schema attributes:
  ```ops5
  (p match-origin
     (point 0 0)
     -->
     (write "Found origin point")
  )
  ```
- **Mixed named & positional attributes**: Explicit named attributes (`^attr val`) can be combined with positional attributes; positional values populate unassigned schema positions in declared order:
  ```ops5
  (make vector 1.0 ^z 3.0 2.0)
  ; Positional 1.0 -> ^x, ^z -> 3.0, positional 2.0 -> ^y
  ```
- If no schema is declared for a class, attributes default to purely dynamic key-value pairs (`^<attr> <val>`).

#### Multi-Valued Attributes (`(vector-attribute ...)`)

In standard OPS5, attributes are single-valued by default. The `(vector-attribute ...)` directive designates one or more attributes as multi-valued vector attributes capable of holding sequences of values:

```ops5
(vector-attribute <attr-1> <attr-2> ... <attr-n>)
```

##### Example
```ops5
(literalize City name location state country population)
(vector-attribute location)

(make City ^name Boston ^location 42.36 -71.05 ^state MA ^country USA ^population 675000)
```

##### Vector Pattern Matching & Variable Binding
- **Positional segment extraction**: When multiple constraints or variables follow a vector attribute, they bind sequentially to the elements of the vector:
  ```ops5
  (p locate-city
     (City ^name <name> ^location <lat> <long> ^state MA)
     -->
     (write <name> "latitude:" <lat> "longitude:" <long>)
  )
  ; Binds <lat> to 42.36 and <long> to -71.05
  ```
- **Membership testing**: When a single scalar or constraint follows a vector attribute, it matches if ANY element in the vector satisfies the condition:
  ```ops5
  (p find-by-coord
     (City ^name <name> ^location 42.36)
     -->
     (write "Matched city by coordinate:" <name>)
  )
  ```
- **Whole-vector capture**: Binding a single variable to a vector attribute captures the entire sequence:
  ```ops5
  (p copy-location
     (City ^name <name> ^location <loc>)
     -->
     (write <name> "full location:" <loc>)
  )
  ; <loc> contains 42.36 -71.05
  ```
- **Vector modifications**: RHS `modify` and REPL `modify` update vector attributes with new value sequences:
  ```ops5
  (modify <c> ^location 42.0 -71.0)
  ```

### Working Memory Elements (WMEs)

A Working Memory Element (WME) represents a structured record in the system:
- **Timetag**: An immutable positive 64-bit integer assigned monotonically upon assertion.
- **Class**: The categorical identifier of the element (e.g., `goal`, `item`, `process`).
- **Attributes**: Key-value pairs prefixed with `^`. Unset attributes evaluate as undefined.

Textual representation:
```ops5
(12: item ^id 101 ^status pending ^priority 5)
```

### Production Rules (`(p ... )`)

A production rule has a name, a Left-Hand Side (LHS) condition elements list, an arrow separator `-->`, and a Right-Hand Side (RHS) actions list:

```ops5
(p <rule-name>
   <condition-element-1>
   <condition-element-2>
   ...
   -->
   <action-1>
   <action-2>
   ...
)
```

### Left-Hand Side (LHS) Condition Elements

> [!NOTE]
> For complete documentation on condition patterns, variable joins, universal quantification, and first-order logic idioms, see the [OPS5 LHS Pattern Matching Reference](docs/lhs_patterns.md) and [OPS5 Design Patterns & Programming Idioms](docs/idioms.md).

#### 1. Positive Condition Elements
Matches a WME of a specified class whose attributes satisfy all stated constraints:
```ops5
(goal ^type batch ^status start)
```

#### 2. Element Variables
Prefixing a condition element with a variable binds that variable to the **timetag** of the matching WME. This bound variable can be referenced later in RHS `modify` and `remove` actions:
```ops5
<g> (goal ^type batch ^status in-progress)
```

#### 3. Negated Condition Elements
Matches when **no** WME exists in working memory that satisfies the condition:
```ops5
-(item ^status pending)
```
Negated conditions can also test variables bound in earlier positive condition elements (cross-condition negative joins):
```ops5
<it> (item ^id <id> ^status pending)
-(hold ^item-id <id>)
```

#### 4. Relational Operators & Multi-Constraint Tests
Attributes can be matched against literal values or variables using relational operators. When no operator is specified, `=` is assumed:

| Operator | Syntax | Description |
| :--- | :--- | :--- |
| **Equal** | `=`, (or omitted) | Values must be identical in type and content |
| **Not Equal** | `<>`, `!=` | Values must not match |
| **Less Than** | `<` | Numeric or lexicographical order strictly less |
| **Less or Equal** | `<=` | Numeric or lexicographical order less than or equal |
| **Greater Than** | `>` | Numeric or lexicographical order strictly greater |
| **Greater or Equal** | `>=` | Numeric or lexicographical order greater than or equal |

Multiple constraints can be specified for a single attribute:
```ops5
(sensor ^temperature > 32 <= 212 ^status active)
```

#### 5. Attribute Disjunction (`<< ... >>`)
Attributes can be matched against a set of alternative values, relational operators, or bound variables enclosed in `<< ... >>`. The condition matches if any of the alternatives match (logical OR):
```ops5
(task ^status << active pending >>)
(sensor ^temperature << < 32 >= 212 >>)
(packet ^route << <primary> <backup> >>)
```

#### 6. Cross-Condition Variable Binding
Variables bound in earlier condition elements enforce equality (or relational joins) when repeated in subsequent condition elements:
```ops5
(order ^order-id <oid> ^customer-id <cid>)
(customer ^id <cid> ^tier vip)
(item ^order-id <oid> ^status pending)
```

### Right-Hand Side (RHS) Actions

Actions execute sequentially when the rule fires:

#### `(make <class> [^<attr> <val> ...])`
Asserts a new WME into working memory with a fresh timetag. Variable references are substituted with their bound values:
```ops5
(make item ^id <new-id> ^status pending ^attempts 0)
```

#### `(modify <target> [^<attr> <val> ...])`
> [!NOTE]
> For an in-depth architectural breakdown of the two-phase retraction/assertion lifecycle and targeting semantics, see the [OPS5 `modify` Reference](docs/modify.md).

Implements standard OPS5 semantic modification:
1. Retracts the target WME.
2. Asserts a replacement WME with a new timetag, merging new attribute values while **preserving all unmodified attributes**.

The `<target>` can be specified by:
- **Element variable**: `(modify <g> ^status done)`
- **1-based CE index**: `(modify 1 ^status done)`

#### `(remove <target>)`
Retracts the targeted WME from working memory. The `<target>` can be an element variable (e.g., `(remove <it>)`) or a 1-based CE index (e.g., `(remove 2)`).

#### `(write <arg1> <arg2> ...)`
> [!NOTE]
> For report generation and table/grid formatting instructions, see the [OPS5 `write`, `(crlf)`, and `(tabto N)` Reference](docs/write.md).

Emits symbols, numbers, strings, and resolved variables to the engine's configured output writer.
- Supports **`(crlf)`** to output newlines and reset the horizontal column position.
- Supports **`(tabto <column>)`** to move the cursor forward to a 1-based column position by padding spaces, enabling aligned tabular grids and reports:
  ```ops5
  (write (crlf) (tabto 5) "ID" (tabto 20) "STATUS" (tabto 35) "VALUE" (crlf))
  (write (tabto 5) <id> (tabto 20) <status> (tabto 35) <val> (crlf))
  ```
- If no `(crlf)` is present in the `write` action, a trailing newline is appended automatically.

#### `(bind <var> <val-or-compute>)`
> [!NOTE]
> For complete details on arithmetic operators, evaluation order, and nested sub-expressions, see the [OPS5 `bind` and `compute` Reference](docs/bind.md).

Assigns the evaluated value or the result of a `compute` expression to a local variable `<var>` during the rule firing cycle:
```ops5
(bind <item-total> (compute <price> * <qty>))
(modify <it> ^total <item-total>)
```
Variables bound by `bind` are immediately accessible to all subsequent actions in the same rule firing.

#### `(cbind <element-variable>)`
Binds the working memory element most recently added by `make`, `modify`, or `call` to `<element-variable>`. Subsequent actions in the same rule firing can use that element variable to modify, remove, or reference that element:
```ops5
(make person ^name "Alice" ^age 30)
(cbind <p>)
(modify <p> ^age 31)
```

#### `(compute <op1> <operator> <op2> ...)`
Evaluates arithmetic expressions using standard OPS5 left-to-right evaluation:
- Operators: `+`, `-` (including unary minus), `*`, `/` (or `//`, `\`), and `%` (or `\\`).
- Usable inside `bind`, or directly as values in `make`, `modify`, and `write`:
```ops5
(write "Grand Total:" (compute <subtotal> + (compute <subtotal> * <rate>)) (crlf))
```

#### `(halt)`
Halts the engine execution immediately. The current cycle completes, but no further rules fire.

#### `(openfile <logical-name> <filespec> <mode>)`
> [!NOTE]
> For complete documentation on stream management, file modes, and examples, see the [File I/O, Stream Redirection & Input Reference](docs/file_io.md).

Opens a file and registers it under a logical name. Modes include `in` (read), `out` (create/truncate write), and `append`. Supports vertical bar symbol escaping for filenames with punctuation or spaces (e.g. `|RuleTrace.ops|`).

#### `(closefile <logical-name>)`
Closes an open file stream. If the closed file was the default stream for `accept`, `write`, or `trace`, that subsystem automatically reverts to standard terminal I/O.

#### `(default <logical-name> <subsystem>)`
Directs the default I/O stream for `accept`, `write`, or `trace` to `<logical-name>`. Passing `nil` or `terminal` restores the default standard terminal stream.

#### `(accept [<logical-name>])` and `(acceptline [<logical-name>])`
Reads user input from standard input or a redirected logical file stream:
- `(accept)`: Reads the next whitespace-delimited atom (symbol, integer, float).
- `(acceptline)`: Reads an entire line of input into a scalar or vector.

#### `(genatom)`
> [!NOTE]
> For complete documentation and examples, see the [OPS5 `genatom` Reference](docs/genatom.md).

Generates a unique sequential symbolic atom (`atom1`, `atom2`, `atom3`, ...):
```ops5
(bind <id> (genatom))
(make task ^id <id> ^status ready)
```
Can also be used directly as an attribute value in `make` and `modify`, in `write` output, and in top-level `make` declarations.

#### `(litval [<class>] <attr>)`
> [!NOTE]
> For complete documentation and examples, see the [OPS5 `litval` Reference](docs/litval.md).

Returns the 1-based numeric index assigned to an attribute within its element class or WME vector layout (where position 1 is the class name, position 2 is the 1st attribute, etc.):
```ops5
(make City ^name Albuquerque ^state NM)
(bind <name-idx> (litval name))   ; evaluates to 2
(bind <state-idx> (litval state)) ; evaluates to 3
```
Can also be evaluated inside `make`, `modify`, `write`, `compute`, and in the REPL.

#### `(substr <elem-ref> <start> <end>)`
> [!NOTE]
> For complete documentation and examples, see the [OPS5 `substr` Reference](docs/substr.md).

Extracts a subsequence of values or a single element from a working memory element:
- `<elem-ref>`: Element variable (e.g. `<str>`) or 1-based condition element index.
- `<start>`: Attribute name, integer, variable, or expression.
- `<end>`: Attribute name, integer, variable, expression, or the special symbol `inf` (denoting end of vector attribute).

When `<start> == <end>` and `<end> != inf`, it extracts and returns that single scalar value directly:
```ops5
(bind <head> (substr <str> sequence sequence)) ; returns 1st element as scalar
(bind <next> (compute (litval sequence) + 1))
(modify <str> ^sequence (substr <str> <next> inf)) ; pops head and keeps rest
```

#### `(build <rule-spec>)`
Dynamically compiles and adds a production rule to production memory at runtime:
```ops5
(p learn-shortcut
   (discovered-shortcut ^from <src> ^to <dst>)
   -->
   (build (p route-shortcut
             (traveler ^dest <dst> ^location <src>)
             -->
             (write "Taking direct shortcut from" <src> "to" <dst> (crlf))))
)
```
Variable references from the enclosing rule firing are substituted into the new rule definition before compilation. The newly built rule immediately joins the active Rete network and matches against existing working memory elements. Can also be invoked as a top-level directive.

---

## Rete Pattern Matching Engine

> [!NOTE]
> For a comprehensive architectural specification of all Beta node types, lifecycle token propagation, and roadmap features, see the [Rete Beta Network Architecture & Catalog](docs/beta_nodes.md).

The engine compiles production rules into a directed acyclic dataflow graph based on Charles Forgy's Rete algorithm.

```mermaid
flowchart LR
    WME["WME Assert/Retract"] --> AlphaRoot["Alpha Root"]
    AlphaRoot --> TN_Goal["TypeNode: goal"]
    AlphaRoot --> TN_Item["TypeNode: item"]

    TN_Goal --> CT_Goal["ConstantTestNode: ^status == active"]
    CT_Goal --> AM_Goal["AlphaMemory (Shared)"]

    TN_Item --> CT_Item["ConstantTestNode: ^status == pending"]
    CT_Item --> AM_Item["AlphaMemory (Shared)"]

    BetaRoot["BetaRoot (Dummy Token)"] --> JN1["JoinNode 1 (goal)"]
    AM_Goal --> JN1
    JN1 --> BM1["BetaMemory 1"]

    BM1 --> JN2["JoinNode 2 (item ^id == <gid>)"]
    AM_Item --> JN2
    JN2 --> TermNode["TerminalNode: process-order"]
    TermNode --> CS["Conflict Set Activation"]
```

### Beta Node Types & Implementation Matrix

The Beta network processes partial rule instantiations (tokens) across conditions and coordinates multi-condition joins:

| Node Type | Arity | Primary Purpose | State / Storage | Engine Status | Target Construct |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`BetaRootMemory`** | 0 (Seeded) | Network root anchor; seeds evaluation with `DummyRootToken()` | Dummy root token | **Implemented** | Engine initialization |
| **`BetaMemory`** | 1 (Left) | Caches partial match tokens; supports successor sharing and index lookups | Tokens map, `BetaIndex` | **Implemented** | Inter-condition boundary |
| **`JoinNode`** | 2 (Left + Right) | Performs relational joins between left tokens and right WMEs | Stateless (queries memories) | **Implemented** | `(class ^attr <var>)` |
| **`NegativeJoinNode`** | 2 (Left + Right) | Enforces absence of matching WMEs; gates tokens on match count = 0 | Tokens map, match counts, `BetaIndex` | **Implemented** | `-(class ^attr <var>)` |
| **`TerminalNode`** | 1 (Left) | Rule terminus; fires activation additions and removals into Conflict Set | Rule, `ConflictSetListener` | **Implemented** | Rule match completion (`-->`) |
| **`EvalNode`** | 1 (Left) | Evaluates arithmetic/boolean expressions across bound variables | Stateless filter | **Implemented** | `(test (compute <x> + <y> > 100))` |
| **`ExistentialJoinNode`** | 2 (Left + Right) | Semi-join (`exists`): verifies $\ge 1$ matching WME without token duplication | Tokens map, match counts | **Implemented** | `(exists (class ^attr <var>))` |
| **`AccumulateNode`** | 2 (Left + Right) | Aggregates matching WMEs (`count`, `sum`, `average`, `min`, `max`, `collect`) into `<var>` | Tokens map, aggregate accumulators, `BetaIndex` | **Implemented** | `(accumulate ... :sum <v>)` |
| **`NccNode` / `NccPartner`** | 2 (Left + Subnet)| Negated conjunction blocks: tests absence of joint condition groups | Sub-token completion maps, match counts | **Implemented** | `-( (cond-1) (cond-2) )` |

### Alpha Network Mechanics
1. **Root Dispatch**: `AlphaRootNode` categorizes incoming WMEs by class using `TypeNode` instances. A wildcard type node `*` captures cross-class patterns.
2. **Constant Test Sharing**: Linear chains of `ConstantTestNode` instances test attribute existence and literal predicates.
3. **Alpha Memory Sharing**: Identical condition patterns share the exact same `AlphaMemory` node across different rules. A canonical key (e.g. `item|^status=pending`) ensures structural sharing.

### Beta Network & Join Mechanics
1. **Tokens**: Tokens represent partial rule instantiations. A token is a linked list of WMEs and an environment map of bound variables.
2. **Beta Root Memory**: Initialized with a dummy empty token so the first condition element can be processed by a standard two-input `JoinNode`.
3. **Variable Join Constraints**: `JoinNode` tests incoming candidate WMEs against variable bindings established in parent tokens.
4. **Activation Propagation**:
   - Left activations (new tokens from parent beta memory) join with right WMEs stored in alpha memory.
   - Right activations (new WMEs from alpha memory) join with left tokens stored in beta memory.

### Hashed Beta & Alpha Join Indexing
To prevent $O(N_{\alpha} \times N_{\beta})$ linear cross-product scans during join evaluation, the Rete network employs **dual-sided hash indexing** on equality join variables:
- **`BetaIndex` (Left Side)**: Indexes tokens in `BetaMemory` by canonical composite keys derived from bound variable values. When a right WME arrives, candidate tokens are retrieved in $O(1)$ average time.
- **`AlphaIndex` (Right Side)**: Indexes WMEs in `AlphaMemory` by canonical composite keys of the joined attributes. When a left token arrives, matching WMEs are retrieved in $O(1)$ average time.
- **Composite Keys & Type Normalization**: Multi-variable joins form composite keys using unit separators (`\x1f`), while `CanonicalValueKey` normalizes cross-types (e.g. integer `42` and float `42.0` share the same hash bucket `i:42`, and symbol/boolean `'true'` share `b:true`).
- **Residual Filtering**: Non-equality constraints (`<`, `>`, `<=`, `>=`, `<>`) and vector membership tests are evaluated as residual predicates against the bucket candidates.
- **Hashed Negative Joins**: `NegativeJoinNode` indexes tokens and alpha WMEs so that satisfiability checks and blocking invalidations operate strictly on matching hash buckets rather than scanning entire memories.

### Structural Beta Node Sharing
In addition to shared Alpha memories, the Rete engine implements **Structural Beta Node Sharing**:
- **Common Condition Prefix Sharing**: When multiple rules share identical initial condition sequences (e.g. `Rule 1: (A) (B) (C)` and `Rule 2: (A) (B) (D)`), the engine shares both the `JoinNode` and downstream `BetaMemory` across rules.
- **Order-Independent Canonical Signatures**: Attributes within condition elements and join constraints are canonically sorted, guaranteeing structural sharing even if attributes appear in different orders across rules.
- **Single Partial Match Evaluation**: Partial matches (such as `[A, B]`) are computed and stored only once in the shared `BetaMemory`. Multiple downstream nodes (e.g. `JoinNode(C)` and `JoinNode(D)`) fork from the shared memory.
- **Reference-Counted Pruning on Excise**: Shared beta nodes maintain rule reference sets. When a rule is excised, shared nodes remain active for other rules that still depend on them. When the last rule referencing a branch is excised, unreferenced beta nodes are automatically pruned bottom-up to prevent memory leaks.

### Node Unlinking (Left & Right Unlinking)
To avoid unnecessary cross-product evaluations in asymmetric or sparse working memory states, the engine implements classic **Node Unlinking** (Forgy / Doorenbos) using $O(1)$ doubly-linked lists (`LeftLink` and `RightLink`):
- **Right Unlinking**: When an `AlphaMemory` has 0 WMEs, child `JoinNode` instances unlink themselves from their parent `BetaMemory`. Left activations (tokens) bypass the join entirely with zero hashing or cross-product overhead. When the first matching WME arrives ($0 \to 1$), the join node re-links to its `BetaMemory` and performs catch-up.
- **Left Unlinking**: When a `BetaMemory` has 0 tokens, child two-input nodes (`JoinNode`, `NegativeJoinNode`, `ExistentialJoinNode`, `AccumulateNode`) unlink themselves from their parent `AlphaMemory`. Right activations (WMEs) bypass the join in $O(1)$ time until the first token arrives ($0 \to 1$).
- **Negative & Existential Joins**: Special care is taken for `NegativeJoinNode`, `ExistentialJoinNode`, and `AccumulateNode` — because an empty right memory satisfies a negated condition or evaluates default aggregates (e.g. `:count` = 0), these nodes remain permanently left-linked to their `BetaMemory`, while fully participating in Left Unlinking from `AlphaMemory` when their parent `BetaMemory` has 0 tokens.
- **Microbenchmark Gains**: Unlinking delivers a 10% to 25%+ throughput improvement for sparse and asymmetric memory pipelines by bypassing dormant join nodes completely.

### Negative Condition Elements
Negated conditions are implemented via `NegativeJoinNode`:
- For each left token, the node maintains a count of matching right WMEs.
- If the match count is **0**, the token is propagated forward to the child beta node (condition satisfied).
- If an assertion in alpha memory matches a token with count 0, the node emits a retraction (`TagRemove`) downstream (condition blocked).
- If the blocking WME is retracted, the count drops back to 0 and the node re-emits an assertion (`TagAdd`) downstream (condition unblocked).

### Dynamic & Retroactive Compilation
In many classic Rete implementations, all rules had to be declared before asserting facts. This engine supports **dynamic and retroactive rule compilation**:
- When `engine.AddRule()` is invoked at runtime (such as in an interactive REPL session), any newly compiled alpha and beta nodes perform a catch-up pass against existing working memory elements.
- Terminal nodes are immediately populated with instantiations for pre-existing facts, and activations enter the conflict set without requiring working memory to be re-asserted.

---

## Conflict Resolution & Execution Lifecycle

> [!NOTE]
> For an in-depth guide on selection algorithms, priority vectors, goal stack patterns, and side-by-side execution walkthroughs, see the [OPS5 Selection Strategy Reference (LEX & MEA)](docs/conflict_resolution.md).

```mermaid
sequenceDiagram
    participant WM as Working Memory
    participant Rete as Rete Network
    participant CS as Conflict Set
    participant Engine as Engine Runner

    loop Cycle Step
        Engine->>CS: SelectDominant()
        alt Conflict Set is Empty
            CS-->>Engine: Quiescence reached (stop)
        else Instantiation Found
            CS-->>Engine: Dominant Activation
            Engine->>CS: MarkFired(activation) [Refraction]
            Engine->>Engine: Execute RHS Actions (make/modify/remove/write/halt)
            Engine->>WM: Apply WM Mutations
            WM->>Rete: Propagate WME Assertions & Retractions
            Rete->>CS: Add / Remove Candidate Activations
        end
    end
```

### Match-Resolve-Act Cycle
Each cycle of the engine proceeds in three strict phases:
1. **Match**: Working memory changes propagate through Rete. The Conflict Set updates with newly enabled or disabled activations.
2. **Resolve**: The Conflict Set sorts all active candidate instantiations and selects the single dominant activation according to the active strategy (**LEX** or **MEA**).
3. **Act**: The dominant activation is marked as refracted and its RHS actions are evaluated.

### Refraction Semantics
To prevent infinite loops where a single rule continuously matches the same static facts:
- An instantiation is uniquely identified by the tuple `(RulePointer, [Timetag_1, Timetag_2, ...])`.
- Once an instantiation fires, its unique key is stored in the refraction table.
- An identical instantiation cannot fire again unless at least one of its participating WMEs is modified or re-asserted (yielding a new timetag).

### Rule Salience / Priority (`[salience N]`)

Rules can declare an optional **salience** (priority weight) directly following the rule name. Higher salience activations strictly dominate in conflict resolution before recency or specificity heuristics are evaluated:

```ops5
(p emergency-shutdown [salience 1000]
    (sensor ^temp > 500)
  -->
    (halt)
)
```

- **Syntax formats supported**: `[salience N]`, `[salience: N]`, `(salience N)`, `(salience: N)`, and CLIPS-style `(declare (salience N))`.
- **Default salience**: `0`. Supports positive (priority) and negative (deferred/cleanup) integers.
- **Tie-breaking**: When candidate activations have equal salience, the active conflict resolution strategy (**LEX** or **MEA**) determines dominance.

### LEX Strategy (Lexicographic)

The default OPS5 conflict resolution strategy prioritizes explicit salience followed by the most recently asserted or modified facts across the entire LHS:

1. **Rule Salience (Tier 1)**:
   - Higher salience rules strictly dominate.
2. **Recency Vector Comparison**:
   - Collect all participating WME timetags in the instantiation.
   - Sort them descending: `[T_max, T_second, ..., T_min]`.
   - Compare vectors lexicographically against other candidate activations. The activation with the highest timetag wins. If highest timetags are equal, compare second-highest, and so on.
3. **Specificity**:
   - If recency vectors are identical, choose the rule with higher specificity (more condition tests).
4. **Rule Index Tie-Breaker**:
   - If specificity is identical, choose the rule declared earliest in the engine (lowest declaration index).
5. **Alphabetical Tie-Breaker**:
   - Final deterministic tie-breaker by rule name string comparison.

### MEA Strategy (Means-Ends Analysis)

The MEA strategy is designed for goal-directed architectures:

1. **Rule Salience (Tier 1)**:
   - Higher salience rules strictly dominate.
2. **Recency of Condition Element 1**:
   - Compare the timetag of the WME matching the **very first condition element** (the goal CE). The instantiation with the newest goal WME dominates.
3. **Recency Vector of Remaining Condition Elements**:
   - If CE 1 timetags are equal, sort the remaining timetags (`CE_2` through `CE_N`) descending and compare them lexicographically (identical to LEX).
4. **Specificity**:
   - If remaining recencies are equal, choose the rule with higher specificity.
5. **Rule Index & Name Tie-Breakers**:
   - Definition index followed by alphabetical tie-breaker.

#### Strategy Comparison Example

Consider two rules and four asserted facts:
- Fact 1: `(1: goal ^id 1)`
- Fact 2: `(2: fact ^tag x)`
- Fact 3: `(3: goal ^id 2)`
- Fact 4: `(4: fact ^tag y)`

Rule `fire-goal-1` matches Goal 1 (timetag 1) and Fact y (timetag 4). Participating timetags: `[1, 4]`.  
Rule `fire-goal-2` matches Goal 2 (timetag 3) and Fact x (timetag 2). Participating timetags: `[3, 2]`.

- **Under LEX**:
  - `fire-goal-1` vector: `[4, 1]`
  - `fire-goal-2` vector: `[3, 2]`
  - Compare first elements: `4 > 3`. **`fire-goal-1` fires first.**
- **Under MEA**:
  - Compare CE 1 timetags: Goal for rule 1 has timetag `1`; Goal for rule 2 has timetag `3`.
  - Compare CE 1: `3 > 1`. **`fire-goal-2` fires first.**

### Rule Specificity Calculation

Rule specificity measures how constrained a rule is:
$$\text{Specificity} = \sum_{ce \in \text{Conditions}} (1 + \text{Constraints}(ce))$$
- Every condition element contributes `+1` for its class name test.
- Every attribute constraint test (`=`, `<>`, `<`, `<=`, `>`, `>=`) contributes `+1`.
- Element variable definitions and variable bindings do not inflate specificity unless associated with a concrete test.

---

## CLI & Interactive REPL Guide

### Installation & Build

Compile the self-contained binary using the Go toolchain (requires Go 1.21+):

```bash
go build -o ops5 ./cmd/ops5
```

### CLI Commands & Flags

```bash
# Launch the interactive REPL
./ops5

# Execute an OPS5 rule file in batch mode
./ops5 path/to/rules.ops

# Pre-load rules and immediately drop into the REPL
./ops5 -i path/to/rules.ops

# Run with cycle tracing enabled
./ops5 -trace path/to/rules.ops

# Export compiled Rete network topology to Graphviz DOT file
./ops5 --dot network.dot path/to/rules.ops

# Execute with MEA strategy and cycle limit
./ops5 -strategy mea -max-cycles 500 path/to/rules.ops

# Run external JSON test suites
./ops5 test tests/fixtures/simple_workflow.json tests/fixtures/mea_lex.json
```

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-i` | boolean | `false` | Drop into interactive REPL after loading input file |
| `-color` | string | `"auto"` | Terminal color output: `auto` (autodetect), `always`, or `never` |
| `-table` | boolean | `false` | Enable boxed tabular mode by default for `wm`, `cs`, and `schemas` |
| `-dot` | string | `""` | Export compiled Rete network graph to Graphviz `.dot` file |
| `-watch` | integer | `1` | Set trace watch level: `0` (silent), `1` (rule firings), `2` (firings + WM changes) |
| `-trace` | boolean | `false` | Enable cycle-by-cycle execution tracing to stdout (alias for `-watch 1`) |
| `-strategy` | string | `"lex"` | Conflict resolution strategy: `lex` or `mea` |
| `-max-cycles`| integer | `1000` | Maximum number of cycles before terminating batch run |

### Interactive REPL Reference

When running `./ops5` interactively, balanced parenthesis tracking allows rules to be entered across multiple lines:

```ops5
ops5> (p classify-alert
...      (sensor ^id <id> ^temp > 100)
...      -->
...      (make alert ^sensor-id <id> ^level critical)
...      (write "Critical alert triggered for sensor" <id>)
...   )
Defined rule 'classify-alert' (conditions=1, specificity=3)
```

#### REPL Command Summary

| Command | Arguments | Description | Example |
| :--- | :--- | :--- | :--- |
| `(literalize ...)` / `literalize` | `<class> <attrs...>` | Declare attribute schema for positional mapping | `literalize point x y` |
| `(vector-attribute ...)` / `vector-attribute` | `<attrs...>` | Declare multi-valued vector attributes | `vector-attribute location` |
| `vector-attributes` | *none* | Display declared vector attributes | `vector-attributes` |
| `schemas` | `[class] [--table]` | Display registered schemas (optionally in a boxed table) | `schemas point --table` |
| `(p ...)` | `<rule-definition>` | Compile a production rule into the active Rete network | `(p r1 (goal ^status active) --> (halt))` |
| `make` | `<class> [^<attr> <val> ...]` | Assert a new WME (syntax highlighted) | `make goal ^type batch ^status start` |
| `modify` | `<timetag> [^<attr> <val> ...]` | Modify an existing WME by timetag | `modify 1 ^status in-progress` |
| `remove` / `(remove ...)` | `<timetag...> \| *` | Retract WME(s) by timetag or all WMEs (`*`) | `remove *`<br>`(remove 1)` |
| `openfile` | `<log-name> <filespec> <mode>` | Open file stream (`in`, `out`, `append`) | `(openfile ruletrace \|RuleTrace.ops\| out)` |
| `closefile` | `<log-name>` | Close an open file stream | `(closefile ruletrace)` |
| `default` | `[<log-name> <subsystem>]` | View or set default stream for `accept`, `write`, `trace` | `(default ruletrace accept)` |
| `genatom` / `(genatom)` | *none* | Generate and display a unique symbolic atom | `(genatom)` |
| `litval` / `(litval ...)` | `[<class>] <attr>` | Display numeric index assigned to an attribute | `(litval name)` |
| `substr` / `(substr ...)` | `<elem> <start> <end>` | Extract a subsequence or value from a WME | `(substr 1 sequence sequence)` |
| `wm` | `[class] [--table]` | Display active WMEs (optionally filtered by class or in a boxed table) | `wm item --table` |
| `cs` | `[--table]` | Display conflict set agenda (optionally in a boxed table) | `cs --table` |
| `status` / `info` | *none* | Display comprehensive engine status overview | `status` |
| `table` | `[on \| off]` | Display or toggle global boxed tabular formatting mode | `table on` |
| `clear` / `cls` | *none* | Clear the terminal screen (or `Ctrl-L`) | `clear` |
| `step` | *none* | Execute exactly one Match-Resolve-Act cycle | `step` |
| `run` | `[max_cycles]` | Execute until quiescence, halt, or max cycles | `run 10` |
| `strategy` | `[lex \| mea]` | View or switch conflict resolution strategy | `strategy mea` |
| `watch` / `(watch ...)` | `[0 \| 1 \| 2]` | Display or set trace watch level (0=silent, 1=firings, 2=firings+WM) | `watch 2`<br>`(watch)` |
| `trace` | `on \| off` | Toggle cycle execution tracing (alias for `watch 1` / `watch 0`) | `trace on` |
| `load` | `<file.ops>` | Load and compile rules and makes from an external file | `load rules.ops` |
| `excise` / `(excise ...)` | `<rule-name...>` | Evict rule(s) from production memory and detach from Rete | `excise rule-1 rule-2` |
| `pm` / `(pm ...)` | `[<rule-name...> \| *]` | Pretty-print production rule source definitions | `pm FindAncestors` |
| `ppwm` / `(ppwm ...)` | `[<class> [^<attr> <val>...]] \| *` | Print working memory elements matching an LHS condition pattern | `(ppwm City ^state Pennsylvania)` |
| `matches` / `(matches ...)` | `[<rule-name...> \| *]` | Inspect partial Rete matches (alpha WMEs, beta join tokens, conflict set activations) | `matches FindAncestors`<br>`(matches *)` |
| `pbreak` / `(pbreak ...)` | `[<rule-name...>]` | Set rule breakpoints to suspend `run` prior to rule firing, or list breakpoints | `pbreak FindAncestors`<br>`(pbreak)` |
| `unpbreak` / `(unpbreak ...)` | `[<rule-name...> \| *]` | Remove rule breakpoints or clear all breakpoints (`*` or no arguments) | `unpbreak FindAncestors`<br>`(unpbreak *)` |
| `dot` / `(dot ...)` | `[<filepath>]` | Export compiled Rete network in Graphviz `.dot` format to stdout or file | `dot network.dot`<br>`(dot)` |
| `test` | `<file.json>` | Execute an external JSON test suite case | `test fixture.json` |
| `reset` | *none* | Clear working memory and conflict set | `reset` |
| `help` | *none* | Display interactive help menu | `help` |
| `exit` / `quit` | *none* | Terminate the REPL session | `exit` |

#### REPL GUI & Interactive Features
- **ANSI Syntax Highlighting**: Colorizes WME timetags (yellow), element classes (magenta), caret attributes (cyan), values (numbers, strings, booleans, symbols), and status headers. Respects `NO_COLOR` standard and terminal detection.
- **Contextual Tab Completion**: Auto-completes commands, registered class schemas, caret attributes (`^attr`), rule names for `excise`/`pm`/`matches`/`pbreak`/`unpbreak`, strategies (`lex`/`mea`), trace levels (`0`, `1`, `2`), and file paths.
- **Persistent Command History**: Automatically saves session command history to `~/.ops5_history` with Up/Down arrow recall and duplicate filtering.
- **Readline Editing**: In-terminal line editing with left/right cursor navigation, Home (`Ctrl-A`), End (`Ctrl-E`), Kill (`Ctrl-K`), Clear Line (`Ctrl-U`), Clear Screen (`Ctrl-L`), and cancel (`Ctrl-C`).
- **Boxed Table Renderer**: Formatted tables using Unicode box-drawing characters (`┌─┬┐`, `│`, `├─┼┤`, `└─┴┘`) with ANSI-aware visual column alignment for `wm`, `cs`, and `schemas`.

> [!TIP]
> For interactive walkthroughs demonstrating rule development, conflict resolution, testing paradigms, and halting controls, see:
> - **[Ancestors Search REPL Tutorial (Section 2.4.3)](docs/tutorial_2_4_3_ancestors.md)**
> - **[Working Memory Initialization & Parameterized Test Harness (Section 2.5)](docs/tutorial_2_5_testing.md)**
> - **[Monkey & Bananas Problem Tutorial (Section 3)](docs/tutorial_3_monkey_bananas.md)**
> - **[Program Termination & Halting Reference](docs/program_termination.md)**
> - **[Rule Excision Reference](docs/excise.md)**
> - **[Print Production Memory Reference (`pm`)](docs/pm.md)**
> - **[Pretty-Print Working Memory Reference (`ppwm`)](docs/ppwm.md)**

### Execution Tracing (`watch`)

The engine provides fine-grained execution tracing via the `watch` directive, REPL command, and CLI flag:
- **`watch 0`**: Silent mode — suppress all rule firings and working memory change reports.
- **`watch 1`** (default): Report the rule name and matching WME timetags for each instantiation that is fired.
- **`watch 2`**: Report rule firings (as in watch 1) plus every working memory addition (`=>WM: ...`) and deletion (`<=WM: ...`).
- **`watch`**: Query the current watch level without modifying it.

#### Watch 1 Trace Example
```ops5
[Cycle 1] Fired rule 'initialize-processing' with WMEs [1]
[Cycle 2] Fired rule 'process-item' with WMEs [2 4]
Processed item 102
Execution halted by rule action after 2 cycles.
```

#### Watch 2 Working Memory Trace Example
```ops5
=>WM: (1: goal ^type batch ^status start)
[Cycle 1] Fired rule 'initialize-processing' with WMEs [1]
<=WM: (1: goal ^type batch ^status start)
=>WM: (2: goal ^type batch ^status in-progress)
=>WM: (3: item ^id 101 ^status pending)
```

### Rete Network Graph Visualization (`--dot` / Graphviz)

The engine can serialize the entire compiled Rete network topology into Graphviz `.dot` format for architectural inspection, structural sharing validation, and debugging.

#### Node & Edge Styling
- **Alpha Network Cluster (`cluster_alpha`)**:
  - **Alpha Root & Type Nodes**: Blue octagons displaying class names.
  - **Constant Test Nodes**: Light-blue rounded boxes displaying intra-condition predicate tests (e.g. `^temp > 100`, disjunctions `^status << active pending >>`).
  - **Alpha Memories**: Rounded blue boxes displaying canonical key signatures and live WME counts (`Items: N`).
- **Beta Network Cluster (`cluster_beta`)**:
  - **Beta Memories**: Green rounded boxes displaying node IDs (`bm0`, `bm1`, ...) and live token counts.
  - **Join Nodes**: Orange ellipses with join test predicates (e.g. `^sensor-id = <sid>`).
  - **Negative Join Nodes**: Red ellipses indicating negative pattern tests (`-(command)`).
  - **Existential Join Nodes**: Amber ellipses indicating existential quantifiers (`(exists item)`).
  - **Accumulate Nodes**: Gray ellipses displaying aggregation functions (`COUNT`, `SUM`, `MIN`, `MAX`, `AVG`).
  - **Eval Nodes**: Yellow diamonds indicating arbitrary predicate expressions.
  - **NCC Subnetworks**: Red rounded conjunction and partner nodes with dotted inhibit connections.
  - **Terminal Nodes**: Purple double-circles showing rule names, priority/salience (`[salience: N]`), and activation counts.
- **Edge Types**:
  - **Solid Green**: Left token propagation between beta memories and join nodes.
  - **Dashed Blue**: Right WME activation from alpha memories to join nodes.
  - **Dashed Red**: Right negative WME activation to negative join nodes.
  - **Dashed Amber**: Right existential WME activation to existential join nodes.
  - **Dotted Red**: Inhibit edges from NCC partner nodes to NCC nodes.
  - **Solid Purple**: Rule activation edges from beta memories to terminal nodes.

#### Exporting & Rendering

Export via CLI:
```bash
./ops5 --dot network.dot path/to/rules.ops
```

Export via REPL:
```ops5
ops5> dot network.dot
Exported Rete network graph to network.dot

ops5> dot
; Emits DOT graph directly to stdout
```

Export in `.ops` files:
```ops5
(dot "network.dot")
```

Render to SVG or PNG using Graphviz:
```bash
dot -Tsvg network.dot -o network.svg
dot -Tpng network.dot -o network.png
```

---

## Test Harness & JSON Test Suite Specification

The engine includes an automated test harness designed to validate rule execution against external test suites.

### Test Case Schema

Test scenarios can be specified in JSON format:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "OPS5TestCase",
  "type": "object",
  "required": ["name"],
  "properties": {
    "name": { "type": "string", "description": "Unique identifier for the test case" },
    "description": { "type": "string" },
    "strategy": { "type": "string", "enum": ["LEX", "MEA"], "default": "LEX" },
    "source": { "type": "string", "description": "Inline OPS5 rule definitions" },
    "source_file": { "type": "string", "description": "Relative or absolute path to .ops rule file" },
    "initial_wm": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["class"],
        "properties": {
          "class": { "type": "string" },
          "attributes": { "type": "object", "additionalProperties": { "type": "string" } }
        }
      }
    },
    "max_cycles": { "type": "integer", "default": 1000 },
    "expected_cycles": { "type": "integer", "description": "Exact number of expected rule firings" },
    "expected_wm": {
      "type": "array",
      "description": "WMEs that MUST exist in working memory upon completion",
      "items": {
        "type": "object",
        "required": ["class"],
        "properties": {
          "class": { "type": "string" },
          "attributes": { "type": "object", "additionalProperties": { "type": "string" } }
        }
      }
    },
    "forbidden_wm": {
      "type": "array",
      "description": "WME patterns that MUST NOT exist in working memory upon completion",
      "items": {
        "type": "object",
        "required": ["class"],
        "properties": {
          "class": { "type": "string" },
          "attributes": { "type": "object", "additionalProperties": { "type": "string" } }
        }
      }
    },
    "expected_outputs": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Substrings expected in write action stdout"
    }
  }
}
```

### Fixture Example

`tests/fixtures/simple_workflow.json`:
```json
{
  "name": "simple_workflow_test",
  "description": "Validates full workflow using external .ops source file",
  "strategy": "LEX",
  "source_file": "fixtures/simple_workflow.ops",
  "initial_wm": [
    {
      "class": "goal",
      "attributes": { "type": "batch", "status": "start" }
    }
  ],
  "max_cycles": 10,
  "expected_cycles": 4,
  "expected_wm": [
    {
      "class": "goal",
      "attributes": { "type": "batch", "status": "all-done" }
    },
    {
      "class": "item",
      "attributes": { "id": "101", "status": "done" }
    },
    {
      "class": "item",
      "attributes": { "id": "102", "status": "done" }
    }
  ],
  "forbidden_wm": [
    {
      "class": "item",
      "attributes": { "status": "pending" }
    }
  ],
  "expected_outputs": [
    "Processed item 101",
    "Processed item 102",
    "Batch completed successfully"
  ]
}
```

### Running Test Suites

Run via the CLI binary:
```bash
./ops5 test tests/fixtures/*.json
```

Run via Go's native test tool:
```bash
go test -v ./tests
```

---

## Programmatic Go API Reference

### Quick Start Example

```go
package main

import (
	"fmt"
	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

func main() {
	// 1. Initialize engine
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)

	// 2. Parse and register rules
	rules, err := parser.ParseRules(`
		(p process-order
		   <o> (order ^id <oid> ^status pending)
		   -->
		   (modify <o> ^status completed)
		   (write "Order completed:" <oid>)
		   (halt)
		)
	`)
	if err != nil {
		panic(err)
	}
	for _, rule := range rules {
		eng.AddRule(rule)
	}

	// 3. Assert initial working memory
	eng.Make("order", map[string]model.Value{
		"id":     model.NewInt(9001),
		"status": model.NewSymbol("pending"),
	})

	// 4. Run execution loop
	cycles, err := eng.Run(100)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Engine completed in %d cycles. Halted: %v\n", cycles, eng.IsHalted())
}
```

### Package Breakdown

```
pkg/
├── model/        # Domain types: WME, ClassSchema, Value (Symbol, Int, Float, String, Variable), Rule, Action
├── wm/           # Thread-safe WorkingMemory, timetag generation, listener notifications
├── rete/         # Alpha & Beta nodes, Tokens, Alpha/Beta memories, joins, negations, terminal nodes
├── conflict/     # Conflict Set agenda, Instantiations, Refraction, LEX & MEA comparator functions
├── engine/       # Match-Resolve-Act lifecycle coordinator, action dispatch, step/run loop
├── parser/       # OPS5 S-expression lexer and parser
├── cli/          # Interactive REPL and command dispatcher
└── harness/      # JSON test suite loader, assertion checker, and runner
```

### Partitioned Multi-Core Concurrency (ParaOPS5)

For large distributed rule networks or high-throughput domains, `engine.PartitionedEngine` coordinates multiple isolated Rete partitions running concurrently across multi-core systems:

```go
// 1. Create a partitioned engine with a routing strategy
pe := engine.NewPartitionedEngine(func(origin, class string, attrs map[string]model.Value) []string {
    // Broadcast notifications to all partitions
    if class == "broadcast" {
        return []string{"*"}
    }
    // Route domain-specific facts
    if dest, ok := attrs["target_partition"]; ok {
        return []string{dest.String()}
    }
    return nil // Local to originating partition
})

// 2. Add partitions and declare schemas
pe.DeclareClass("task", []string{"id", "status"})
p1, _ := pe.AddPartition("worker-1")
p2, _ := pe.AddPartition("worker-2")

// 3. Register partition-specific or global rules
pe.AddRule(globalMonitoringRule)
pe.AddRuleToPartition("worker-1", worker1Rule)

// 4. Run partitions in parallel until global quiescence
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

totalCycles, err := pe.RunParallel(ctx, 1000)
```

Additionally, single-engine instances support concurrent alpha evaluation for large-scale fact assertions:
```go
eng.SetAlphaWorkers(4)
eng.MakeBatch([]engine.MakeRequest{
    {Class: "item", Attributes: map[string]model.Value{"id": model.NewInt(1)}},
    {Class: "item", Attributes: map[string]model.Value{"id": model.NewInt(2)}},
})
```

### Extensibility & Custom Actions

In addition to standard OPS5 actions (`make`, `modify`, `remove`, `write`, `halt`), Go applications can register `CustomAction` handlers directly on rules:

```go
rule := model.NewRule("notify-external-service")
rule.AddCondition(model.NewPositiveCE("event").AddEqualTest("type", model.NewSymbol("alert")))
rule.AddAction(model.CustomAction{
    Name: "webhook-dispatch",
    Execute: func(ctx any) error {
        // Custom Go execution logic (HTTP call, database transaction, etc.)
        return nil
    },
})
eng.AddRule(rule)
```

---

## Canonical Benchmark Suite

The engine includes classic academic production-system benchmark suites located in [`benchmarks/`](benchmarks/). For an in-depth comparative evaluation against other rule engines (CMU OPS5, NASA CLIPS, Apache Drools, and modern Go rule libraries), see the **[Engine Comparison & Architectural Evaluation Reference](docs/engine_comparison.md)**.

### Benchmark Problems (Manners, Waltz, Zebra)

1. **Miss Manners (`benchmarks/manners/`)**:
   - Solves the dinner guest seating arrangement problem via combinatorial constraint satisfaction (alternating male/female seating where adjacent guests share at least one hobby).
   - Generates reproducible, deterministic test sets for 16, 32, 64, and 128 guests (`manners16.ops`, `manners32.ops`, `manners64.ops`, `manners128.ops`).
   - Stresses cross-product beta joins, deep recursion, and state-space exploration.

2. **Waltz Line Labeling (`benchmarks/waltz/`)**:
   - Implements David Waltz's 2D line-labeling algorithm for 3D polyhedral wireframe scenes.
   - Categorizes edges into convex (`+`), concave (`-`), and boundary occlusions (`>`) using junction constraints (corners, forks, arrows, multijunctions).
   - Datasets: `waltz12.ops` (12-edge wireframe) and `waltz50.ops` (50-edge complex scene).
   - Stresses rapid transitive constraint propagation, edge modifications, and negation satisfaction.

3. **Zebra Puzzle / Einstein's Logic Riddle (`benchmarks/zebra/`)**:
   - Classic 5-house logic puzzle with 14 relational clues across house colors, nationalities, pets, drinks, and cigarette brands.
   - Solves for the water drinker (Norwegian in House 1) and zebra owner (Japanese in House 5).
   - Stresses multi-condition beta joins with cross-element equality and inequality constraints.

### Running the Benchmarks

Run the complete benchmark suite test:
```bash
# Fast run (skips Manners-64):
go test -v -short ./benchmarks

# Full suite with all scales:
go test -v ./benchmarks
```

Run Go microbenchmarks reporting `cycles/s` and `wmes/s`:
```bash
go test -bench=. -benchtime=1x -run=^$ ./benchmarks
```

### Benchmark Results & Performance Profile

Current library performance measured on a dedicated test machine (12th Gen Intel Core i7-1260P, 16 threads, 64 GB RAM, Ubuntu 24.04, Go 1.23). For full machine architecture specifications and historical logs across versions, see **[Benchmark Timing History & Performance Log](docs/benchmarks_history.md)**.

| Benchmark | Cycles | WMEs Asserted | Time to Quiescence | Throughput (Cycles/s) | WME Assertions/s | Total Heap Alloc |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Manners-16** | 2,009 | 2,744 | **316 ms** | 6,364.9 | 8,693.5 | ~117 MB |
| **Manners-32** | 3,914 | 4,649 | **598 ms** | 6,544.4 | 7,773.3 | ~245 MB |
| **Manners-64** | 19,381 | 21,152 | **6.22 s** | 3,116.5 | 3,401.3 | ~2.40 GB |
| **Waltz-12** | 608 | 1,238 | **11 ms** | 53,330.9 | 108,591.6 | ~5.5 MB |
| **Waltz-50** | 2,268 | 4,654 | **52 ms** | 43,720.1 | 89,714.9 | ~21.4 MB |
| **Zebra-5** | 7 | 19 | **0.08 ms** | 83,804.2 | 227,468.6 | ~68 KB |


---

## Verification & Concurrency Model

- **Thread-Safety**: `WorkingMemory`, `Network`, `BetaMemory`, `AlphaMemory`, `ConflictSet`, and `Engine` protect shared mutable state with fine-grained read/write mutexes (`sync.RWMutex`), supporting thread-safe inspection and concurrent listener dispatch.
- **ParaOPS5 Multi-Partition Concurrency**: `PartitionedEngine` enables parallel subnetwork execution with cross-partition routing channels, atomic quiescence tracking, and zero inter-partition lock contention.
- **Unit & Integration Tests**: Comprehensive tests in [`tests/suite_test.go`](tests/suite_test.go), [`pkg/rete/network_test.go`](pkg/rete/network_test.go), [`pkg/conflict/conflict_test.go`](pkg/conflict/conflict_test.go), [`pkg/engine/partition_test.go`](pkg/engine/partition_test.go), and [`pkg/cli/repl_test.go`](pkg/cli/repl_test.go).
- Run the full test suite with test coverage:
  ```bash
  go test -race -cover ./...
  ```

---

## License

This project is licensed under the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for the full license text.

