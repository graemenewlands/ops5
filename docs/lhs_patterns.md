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

### Multiple Constraints on a Single Attribute
An attribute can have multiple constraints in the same condition element:
```ops5
(sensor ^reading >= 50 ^reading <= 100)
```
Matches any `sensor` whose `reading` is between 50 and 100 (inclusive).

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
To express logical OR between condition patterns, write separate production rules with identical RHS actions, or declare multiple rules matching different alternatives:
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

## 8. Summary of LHS Syntax & Rules

| Construct | Syntax | Notes |
| :--- | :--- | :--- |
| **Positive CE** | `(class ^attr val ...)` | Matches presence of WME. |
| **Negative CE** | `-(class ^attr val ...)` | Matches absence of matching WMEs. |
| **Element Variable** | `<var> (class ...)` | Binds WME timetag for RHS `modify`/`remove`. Positive CEs only. |
| **Value Variable** | `^attr <var>` | Binds attribute value or constrains join across CEs. |
| **Equality Test** | `^attr val` or `^attr = val` | Default test when operator omitted. |
| **Inequality Test** | `^attr <> val` or `^attr != val` | Negated attribute test. |
| **Relational Tests** | `^attr < val`, `<=`, `>`, `>=` | Numeric and string comparisons. |
| **Positional Test** | `(class val1 val2 ...)` | Requires registered `literalize` schema. |
