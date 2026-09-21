# OPS5 Left-Hand Side (LHS) Pattern Matching Reference

The Left-Hand Side (LHS) of an OPS5 production rule specifies the condition elements (CEs) that must match against Working Memory Elements (WMEs) for the rule to become instantiated and eligible to fire.

```ops5
(p rule-name
    <lhs-condition-element-1>
    <lhs-condition-element-2>
    ...
  -->
    <rhs-action-1>
    <rhs-action-2>
)
```

---

## 1. Positive Condition Elements

A positive condition element tests for the **presence** of a WME matching a specified element class and attribute constraints.

```ops5
(class-name ^attribute-1 value-1 ^attribute-2 value-2 ...)
```

- If all attribute constraints are satisfied by a candidate WME, the condition element matches.
- Attributes not mentioned in the condition element are ignored (wildcard matching).
- Order of attributes in a condition element does not matter.

### Example
```ops5
(person ^name Alice ^age 30)
```
Matches any `person` WME whose `name` is `"Alice"` and whose `age` is `30`, regardless of any other attributes (e.g. `job`, `city`) that the WME might contain.

---

## 2. Element Variables (`<var>`)

An **element variable** binds the entire matching WME (specifically its unique timetag) to a variable name. Element variables are enclosed in angle brackets `<...>` and placed immediately before the condition element:

```ops5
<p> (person ^name Alice ^status pending)
```

### Purpose of Element Variables
1. **Targeting RHS Actions**:
   Allows RHS actions such as `modify` and `remove` to reference and mutate the exact matched WME:
   ```ops5
   (p activate-person
       <p> (person ^status pending)
     -->
       (modify <p> ^status active)
   )
   ```
2. **Referencing Timetags in Computations & Output**:
   Element variables resolve to the WME's integer timetag on the RHS:
   ```ops5
   (write "Updated person with timetag:" <p> (crlf))
   ```

> [!NOTE]
> Element variables can only be attached to **positive** condition elements. Negated condition elements do not correspond to any WME in working memory and cannot have element variables.

---

## 3. Relational Operators & Predicates

By default, an attribute test without an operator tests for **equality** (`=`). OPS5 supports a rich set of relational operators:

| Operator | Meaning | Example | Behavior |
| :--- | :--- | :--- | :--- |
| **`=`** | Equal to (default) | `^status = active`<br>`^count 5` | Matches if attribute equals the target value. |
| **`<>`**, **`!=`** | Not equal to | `^status <> complete`<br>`^type != batch` | Matches if attribute is not equal to target. |
| **`<`** | Strictly less than | `^temperature < 100`<br>`^priority < 5` | Numeric or lexicographical less-than. |
| **`<=`** | Less than or equal | `^balance <= 0.0` | Numeric or lexicographical less-than-or-equal. |
| **`>`** | Strictly greater than | `^retry-count > 3` | Numeric or lexicographical greater-than. |
| **`>=`** | Greater than or equal | `^score >= 70` | Numeric or lexicographical greater-than-or-equal. |

### Multiple Constraints & Conjunction Blocks (`{ ... }`)
An attribute can have multiple constraints in the same condition element, either by repeating the attribute or using a braced conjunction block `{ ... }`:
```ops5
; Repeated attribute syntax
(sensor ^reading >= 50 ^reading <= 100)

; Braced conjunction block syntax
(sensor ^reading { >= 50 <= 100 })
```
All constraints inside `{ ... }` must be satisfied (logical AND).

### Attribute Disjunction Blocks (`<< ... >>`)
To test an attribute against multiple acceptable values or relational conditions where **any single match** is sufficient (logical OR), enclose the alternatives in double angle brackets `<< ... >>`:
```ops5
; Match if status is either 'active' or 'pending'
(task ^status << active pending >>)

; Match extreme sensor values (< 10 OR >= 100)
(sensor ^temp << < 10 >= 100 >>)

; Match if route matches either previously bound primary or backup route
(packet ^route << <primary> <backup> >>)
```
Disjunction blocks can also be used inside conjunction blocks (`^reading { > 0 << 10 20 30 >> }`) and within positional condition elements.

---

## 4. Variable Bindings & Cross-Condition Joins

Variables inside attribute tests are enclosed in angle brackets (`<var>`).

### Binding Variables
When a variable appears for the first time in a positive condition element, it binds to the value of that attribute in the matching WME:
```ops5
(order ^id <order-id> ^total <amt>)
```
Here, `<order-id>` binds to the value of `^id`, and `<amt>` binds to the value of `^total`.

### Cross-Condition Equality Joins
When the same variable is used across multiple condition elements, OPS5 creates a **Join Constraint** in the Rete Beta network. The rule matches only when the attribute values in different WMEs are equal:

```ops5
(p ship-order-item
    (order ^id <oid> ^status approved)
    (line-item ^order-id <oid> ^item-name <item> ^shipped false)
  -->
    (write "Shipping" <item> "for order" <oid> (crlf))
)
```
- The first CE binds `<oid>` to the order's `id`.
- The second CE requires `line-item`'s `^order-id` to match the exact same value.

### Cross-Condition Relational Joins
Variables bound in earlier condition elements can be combined with comparison operators in subsequent condition elements:

```ops5
(p detect-over-limit
    (account-limit ^max-amount <limit>)
    (transaction ^amount > <limit> ^account-id <aid>)
  -->
    (write "Transaction on account" <aid> "exceeds limit" <limit> (crlf))
)
```

---

## 5. Negative Condition Elements (`-(...)`)

A negative (or negated) condition element tests for the **absence** of any matching WME in working memory. Negation is denoted by a leading minus sign `-`:

```ops5
-(class-name ^attribute value)
```

A rule with a negative condition element will only fire if **no WME** in working memory satisfies the negated pattern.

### Basic Absence Testing
```ops5
(p detect-missing-config
    (system-state ^status starting)
    -(config-file ^loaded true)
  -->
    (write "Error: system starting but configuration not loaded!" (crlf))
)
```
Fires only when `system-state` is `starting` AND there is no `config-file` with `^loaded true`.

### Negation with Variable Joins
Negated condition elements can reference variables bound by preceding positive condition elements. The negated test is scoped to the specific bound values:

```ops5
(p all-tasks-complete
    <job> (job ^id <jid> ^status in-progress)
    -(task ^job-id <jid> ^status <> completed)
  -->
    (modify <job> ^status finished)
    (write "All tasks completed for job" <jid> (crlf))
)
```
- `<jid>` is bound from the positive `job` WME.
- The negated condition checks: *There does NOT exist any `task` for this specific `job-id` whose `status` is NOT `completed`.*
- Once all tasks for job `<jid>` reach status `completed`, the negated condition element is satisfied and the rule fires.

> [!IMPORTANT]
> **Variable Binding Rule**: Variables **cannot** be introduced (bound for the first time) inside a negative condition element. Variables used in a negative condition element must be bound by an earlier positive condition element.

---

## 6. Positional Condition Elements

When an element class is defined using `(literalize <class> <attr1> <attr2> ...)`, condition elements can use classic OPS5 positional notation without explicit attribute carets `^`:

```ops5
(literalize Point x y)

; Positional matching
(p origin-detector
    (Point 0 0)
  -->
    (write "Point is at origin" (crlf))
)

; Positional matching with operators and variables
(p diagonal-quadrant-one
    (Point > 0 > 0)
  -->
    (write "Point is in quadrant 1" (crlf))
)
```

Positional tests map directly to schema attributes in declaration order:
- The 1st positional test maps to the 1st attribute (`x`).
- The 2nd positional test maps to the 2nd attribute (`y`).

---

## 7. Expressing Logic: Conjunction, Disjunction, and Universal Quantification

OPS5 condition elements provide clean idioms for standard first-order logic constructs:

### 1. Conjunction (AND)
All condition elements in the LHS are implicitly joined by **logical AND**:
```ops5
; Condition 1 AND Condition 2
(customer ^tier gold)
(order ^amount > 1000)
```

### 2. Disjunction (OR)

#### Attribute-Level Disjunction (`<< ... >>`)
When the disjunction applies to an individual attribute's value, use `<< ... >>` directly inside the condition element. This avoids rule duplication and shares Rete alpha/beta nodes:
```ops5
(p alert-extreme-temp
    (sensor ^id <sid> ^temp << < 10 >= 100 >>)
  -->
    (make extreme-alert ^sensor-id <sid>)
)
```

#### Rule-Level Disjunction
To express logical OR between entirely distinct condition element structures, declare separate production rules with identical RHS actions:
```ops5
(p qualify-discount-gold
    (customer ^tier gold)
  -->
    (make discount ^rate 0.15)
)

(p qualify-discount-volume
    (order ^amount > 5000)
  -->
    (make discount ^rate 0.15)
)
```

### 3. Universal Quantification ($\forall$, "For All")
In classical logic, universal quantification ($\forall x: P(x)$) is equivalent to the negation of an existential counterexample ($\neg \exists x: \neg P(x)$):

$$\forall x \in S : P(x) \iff \neg \exists x \in S : \neg P(x)$$

In OPS5:
- To test: **"All items in the batch are inspected"**:
- Write: **"There is NO item in the batch that is NOT inspected"**:
```ops5
(batch ^id <bid>)
-(item ^batch-id <bid> ^inspected <> true)
```

---

## 8. Predicate Test Condition Elements (`(test ...)`)

A `(test ...)` condition element evaluates mathematical, boolean, or relational expressions across variables already bound by preceding condition elements without matching against working memory:

```ops5
(test (<val1> <op> <val2>))
(test (<op> <val1> <val2>))
(test (compute <expr> <op> <val>))
```

### Purpose & Syntax
1. **Cross-Variable Comparison**:
   Standard condition elements only test individual WMEs or equality joins. To compare two bound variables with inequality or relational operators, use `(test ...)`:
   ```ops5
   (p detect-overdraw
       (account ^balance <bal>)
       (withdrawal-request ^amount <amt>)
       (test (<amt> > <bal>))
     -->
       (write "Declined: withdrawal" <amt> "exceeds balance" <bal> (crlf))
   )
   ```
2. **Arithmetic with `compute`**:
   Mathematical formulas can be evaluated directly on the LHS:
   ```ops5
   (p discount-bulk
       (item ^price <p> ^qty <q>)
       (test (compute <p> * <q> >= 100))
     -->
       (make discount ^rate 0.10)
   )
   ```
3. **Multiple Conjunction Tests**:
   Multiple comparisons can be included in a single `(test ...)` element:
   ```ops5
   (test (<x> >= 10) (<x> <= 100))
   ```

> [!NOTE]
> All variables referenced in a `(test ...)` condition must be bound by earlier positive condition elements. A `(test ...)` condition cannot be negated with `-` and cannot have an element variable `<var>`.

---

## 9. Existential Condition Elements (`(exists ...)`)

An **existential condition element** tests for the presence of **at least one** matching WME in working memory without causing token multiplication. In relational algebra, this is known as a **semi-join** ($\exists x: P(x)$).

```ops5
(exists (class-name ^attribute value ...))
; Or flat syntax:
(exists class-name ^attribute value ...)
```

### The Cartesian Explosion Problem
With standard positive condition elements, every matching WME creates a distinct token instantiation:
```ops5
(system ^status online)
(order ^status pending)
```
If there are 50 `order` WMEs with `^status pending`, the rule matches **50 times**.

Often, a rule only needs to know **whether** at least one matching item exists (e.g., "notify operator if there are any alerts", "start processing batch if there are pending items"), without firing for every single item.

### Semi-Join Semantics & Behavior
With `(exists ...)`:
```ops5
(p alert-pending-orders
    (system ^status online)
    (exists (order ^status pending))
  -->
    (write "System is online and pending orders exist." (crlf))
)
```
1. **Zero Token Multiplication**: Regardless of whether there are 1, 10, or 10,000 pending orders, the rule produces **exactly one** activation.
2. **Dynamic Transition Tracking**:
   - When the first matching WME is created ($0 \to 1$), the condition becomes satisfied and a token is emitted downstream.
   - When subsequent matching WMEs are added ($1 \to 2, 3, \dots$), propagation is suppressed.
   - When matching WMEs are retracted, the token remains active until the last matching WME is removed ($1 \to 0$), at which point a retraction token is emitted downstream.
3. **Cross-Condition Joins**:
   Existential conditions can join on variables bound by earlier positive conditions:
   ```ops5
   (p customer-has-open-tickets
       (customer ^id <cid> ^name <cname>)
       (exists (ticket ^customer-id <cid> ^status open))
     -->
       (write "Customer" <cname> "has open support tickets." (crlf))
   )
   ```

> [!IMPORTANT]
> **Variable Scoping & RHS Rules**:
> - Variables introduced inside an `(exists ...)` condition element are **locally scoped** and are **not** exported downstream or available on the RHS.
> - An `(exists ...)` condition element **cannot** have an element variable `<var> (exists ...)` because no single WME represents the condition.
> - An `(exists ...)` condition element **cannot** be negated (`-(exists ...)`). To test for non-existence, use standard negative condition elements `-(class ...)`.

---

## 10. Accumulate Condition Elements (`(accumulate ...)` / `(acc ...)`)

An **accumulate condition element** aggregates a collection of matching WMEs and binds the resulting aggregate value to a variable:

```ops5
(accumulate (class-name ^attr val ...) <operation> [<target>] <result-var>)
; Or using alias 'acc':
(acc (class-name ^attr val ...) <operation> [<target>] <result-var>)
```

### Supported Aggregation Operations

| Operation | Syntax Example | Behavior on Empty Set | Description |
| :--- | :--- | :--- | :--- |
| **`:count`** | `:count <cnt>` | Emits `0` | Counts total matching WMEs. Target operand is optional. |
| **`:sum`** | `:sum <p> <total>` | Emits `0` | Sums numeric values (supports integers and floats). |
| **`:avg`**, **`:average`** | `:avg <g> <gpa>` | Suppressed (no emission) | Calculates arithmetic mean as float. Requires count > 0. |
| **`:min`** | `:min <score> <lowest>` | Suppressed (no emission) | Selects minimum value via relational comparison. Requires count > 0. |
| **`:max`** | `:max <score> <highest>`| Suppressed (no emission) | Selects maximum value via relational comparison. Requires count > 0. |
| **`:collect`** | `:collect <id> <list>` | Emits `()` (empty vector) | Collects values into an ordered vector. |

### Target Expressions
The target to accumulate can be:
1. **A Variable**: Bound within the inner pattern (e.g. `^price <p>` with target `<p>`).
2. **An Attribute**: Direct attribute name (e.g. `price`).
3. **An Arithmetic Expression**: Using `(compute ...)`, e.g. `:sum (compute <qty> * <price>) <subtotal>`.

### Examples

#### 1. Calculating Order Total
```ops5
(p summarize-order
    (order ^id <oid> ^customer <cust>)
    (accumulate (order-line ^order-id <oid> ^price <p>) :sum <p> <total>)
  -->
    (write "Order" <oid> "total is:" <total> (crlf))
)
```

#### 2. Chaining with Predicate Tests
An aggregate result can be filtered immediately by a downstream `(test ...)` condition:
```ops5
(p alert-high-balance
    (account ^id <aid> ^owner <name>)
    (accumulate (transaction ^acc-id <aid> ^amount <amt>) :sum <amt> <total>)
    (test (<total> > 10000))
  -->
    (write "Alert: High transaction volume for" <name> "Total:" <total> (crlf))
)
```

#### 3. Counting Pending Tasks
```ops5
(p report-status
    (system ^state online)
    (accumulate (task ^status pending) :count <num-pending>)
  -->
    (write "Online with" <num-pending> "tasks remaining" (crlf))
)
```

### Reactive Invalidation & Lifecycle
- When matching WMEs are added or modified, `AccumulateNode` recalculates the aggregate value and underlying timetags.
- If an earlier activation token was emitted downstream, the engine emits `TagRemove` for the old token and `TagAdd` for the newly calculated token.
- Timetags of the aggregated WMEs are preserved in the token chain, ensuring that conflict resolution recency (LEX / MEA) and refraction mechanics function accurately.

> [!IMPORTANT]
> **Variable Scoping & RHS Rules**:
> - Variables declared purely inside the inner condition pattern (e.g. `<p>` in `^price <p>`) are **locally scoped** to the accumulation.
> - The output variable `<result-var>` (e.g. `<total>`) is **exported downstream** and is fully accessible to subsequent conditions and RHS actions (`make`, `modify`, `write`).
> - An accumulate condition element **cannot** be negated (`-(accumulate ...)`) and **cannot** have an element variable `<var> (accumulate ...)`.

---

## 11. Negated Conjunctive Conditions (`-( (c1) (c2) ... )` / `(ncc ...)`)

A **Negated Conjunctive Condition (NCC)** tests for the **absence of a joint combination** of two or more condition elements.

While an individual negative condition element `-(class ...)` tests for the non-existence of a single WME, an NCC tests for the non-existence of an entire conjunction: $\neg (C_1 \land C_2 \land \dots \land C_k)$.

```ops5
; Standard OPS5 nested parentheses syntax:
-( (class-1 ^attr1 val1 ...) (class-2 ^attr2 val2 ...) ... )

; Alternative braced syntax:
-{ (class-1 ^attr1 val1 ...) (class-2 ^attr2 val2 ...) ... }
-( { (class-1 ^attr1 val1 ...) (class-2 ^attr2 val2 ...) ... } )

; Explicit keyword syntax:
(ncc (class-1 ^attr1 val1 ...) (class-2 ^attr2 val2 ...) ... )
```

### Why NCC is Needed: Absence of Relationships

Consider testing the condition: *"Find every department manager where there is NO employee in the department who earns more than that manager."*

With single negative condition elements, you cannot express this because the comparison requires joining `department`, `employee`, and `salary` simultaneously:
- Testing `-(employee ^salary > <mgr-salary>)` alone would check if *no employee anywhere in any department* makes more than `<mgr-salary>`.
- Testing `-(department ...)` and `-(employee ...)` as separate negative CEs would test each independently.

With an NCC, the sub-conditions are joined together before negation is evaluated:

```ops5
(p find-top-paid-managers
    (manager ^dept <d> ^name <mname> ^salary <msal>)
    -( (employee ^dept <d> ^name <ename> ^salary <esal>)
       (test (<esal> > <msal>)) )
  -->
    (write "Manager" <mname> "is highest paid in department" <d> (crlf))
)
```
The rule matches only if there is **no combination** of `employee` and `test` in department `<d>` with salary greater than `<msal>`.

### Another Example: Multistage Workflows
```ops5
(p order-ready-for-packaging
    (order ^id <oid> ^status processing)
    -( (order-item ^order-id <oid> ^sku <sku>)
       (inventory ^sku <sku> ^available false) )
  -->
    (write "Order" <oid> "has all items in stock and is ready for packaging" (crlf))
)
```
The rule fires when there does **not** exist an item in order `<oid>` that is simultaneously out of stock in `inventory`.

### Variable Scoping & Semantics in NCC
1. **Parent-Variable Inheritance**:
   Sub-conditions within the NCC can reference variables bound by preceding positive conditions (e.g. `<d>` and `<msal>` above). The sub-network is scoped per parent token.
2. **Intra-Conjunction Joins**:
   Sub-conditions inside the NCC can introduce variables and join with one another (e.g. `<sku>` shared between `order-item` and `inventory`).
3. **No Variable Leaking**:
   Variables introduced *inside* the NCC (such as `<ename>` or `<esal>`) are **strictly local** to the NCC. They cannot be referenced downstream or used on the RHS.
4. **No Element Variables**:
   Element variables `<var> -( ... )` cannot be attached to an NCC block because no single WME represents the negated conjunction.
5. **Minimum Conditions**:
   An NCC must contain at least two condition elements (or at least one condition joined with a `(test ...)`). A single condition can simply be written as a standard negated condition `-(class ...)`.

### Rete Architecture: `NccNode` & `NccPartnerNode`
Under the hood, NCC is implemented as an asynchronous sub-pipeline in the Rete network:
- The sub-conditions form a branched Rete Beta pipeline starting from the parent beta memory.
- At the end of the sub-pipeline, an `NccPartnerNode` buffers completed sub-matches and notifies the corresponding `NccNode` on the main pipeline.
- `NccNode` maintains a count of completed sub-matches for each parent token.
- When `count == 0`, the parent token is satisfied and propagated downstream.
- When `count > 0`, downstream propagation is blocked (or retracted if previously active).

---

## 12. Summary of LHS Syntax & Rules

| Construct | Syntax | Notes |
| :--- | :--- | :--- |
| **Positive CE** | `(class ^attr val ...)` | Matches presence of WME. Token emitted per matching WME. |
| **Negative CE** | `-(class ^attr val ...)` | Matches absence of matching WMEs. Single-condition negated test. |
| **Negated Conjunction (NCC)** | `-( (c1) (c2) ... )`<br>`(ncc (c1) (c2) ...)` | Tests absence of a joint combination of WMEs. Sub-variables do not leak downstream. |
| **Existential CE** | `(exists (class ^attr val ...))` | Semi-join ($\exists$). Tests for $\ge 1$ matching WME without token multiplication. |
| **Accumulate CE** | `(accumulate (class ...) :op <v> <res>)` | Aggregates matching WMEs (`:count`, `:sum`, `:avg`, `:min`, `:max`, `:collect`). |
| **Test CE** | `(test (<val1> <op> <val2>))` | Evaluates expression across bound variables via `EvalNode`. |
| **Element Variable** | `<var> (class ...)` | Binds WME timetag for RHS `modify`/`remove`. Positive CEs only. |
| **Value Variable** | `^attr <var>` | Binds attribute value or constrains join across CEs. |
| **Equality Test** | `^attr val` or `^attr = val` | Default test when operator omitted. |
| **Inequality Test** | `^attr <> val` or `^attr != val` | Negated attribute test. |
| **Relational Tests** | `^attr < val`, `<=`, `>`, `>=` | Numeric and string comparisons. |
| **Positional Test** | `(class val1 val2 ...)` | Requires registered `literalize` schema. |
