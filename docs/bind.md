# OPS5 `bind` and `compute` Arithmetic Reference

OPS5 supports arithmetic computation and local variable assignment on the Right-Hand Side (RHS) of production rules via the `compute` function and the `bind` action directive.

---

## 1. Syntax

### `bind` Action
```ops5
(bind <variable> <value-or-expression>)
```
Assigns the evaluated value or result of a computation to `<variable>` within the local scope of the executing rule cycle. Subsequent RHS actions in the same rule firing can reference `<variable>`.

### `compute` Expression
```ops5
(compute <operand1> <operator> <operand2> [<operator> <operand3> ...])
```
Evaluates an arithmetic expression left-to-right. `(compute ...)` can appear inside a `bind` action, or directly as an attribute value in `make`, `modify`, and `write` actions.

---

## 2. Arithmetic Operators & Precedence

OPS5 supports standard arithmetic operations:

| Operator | Operation | Semantics |
| :--- | :--- | :--- |
| **`+`** | Addition | Sums numbers. |
| **`-`** | Subtraction / Unary Minus | Subtracts or negates numbers. Unary minus is supported (e.g. `(compute - 5)` or `(compute - <x>)`). |
| **`*`** | Multiplication | Multiplies numbers. |
| **`/`**, **`//`**, **`\`** | Division | If both operands are integers, performs truncating integer division. If either operand is a float, performs floating-point division. |
| **`%`**, **`\\`** | Modulo (Remainder) | Computes the integer remainder (`a % b`). |

### Left-to-Right Evaluation Order
In standard OPS5, `compute` expressions are evaluated strictly **left-to-right** without operator precedence:
```ops5
(compute 2 + 3 * 4)   ; Evaluates as (2 + 3) * 4 = 20, NOT 14
```

To enforce standard mathematical precedence or group operations, use nested `(compute ...)` expressions:
```ops5
(compute 2 + (compute 3 * 4))   ; Evaluates to 14
```

---

## 3. Variable Binding & Local Scope

- Variables bound by `bind` are stored in a **cycle-local binding environment**.
- They are immediately available to all subsequent actions in the same rule instantiation.
- `bind` variables do not modify LHS token bindings in the Rete network, preserving token immutability.
- Multiple `bind` actions can build upon each other sequentially:
```ops5
(bind <subtotal> (compute <price> * <qty>))
(bind <tax> (compute <subtotal> * 0.05))
(bind <total> (compute <subtotal> + <tax>))
```

---

## 4. Usage in Other RHS Actions

The `(compute ...)` expression is not restricted to `bind`. It can be used anywhere a value is expected on the RHS:

### In `make`
```ops5
(make item ^id 101 ^total (compute <price> * <qty>))
```

### In `modify`
```ops5
(modify <counter> ^value (compute <current-val> + 1))
```

### In `write`
```ops5
(write "Remaining items:" (compute <total> - <processed>) (crlf))
```

---

## 5. `cbind` Action (Element Variable Binding)

### Definition & Syntax
```ops5
(cbind <element-variable>)
```
The action `cbind` is used to bind a working memory element to an element variable. The element bound is the **last element added to the working memory by `make`, `modify`, or `call`**. The action takes one argument, an element variable (e.g. `<p>` or `p`).

### Key Differences: `bind` vs `cbind`
- **`bind`**: Assigns a scalar literal, variable value, or the result of a `(compute ...)` arithmetic expression to a variable (e.g. `(bind <tax> (compute <p> * 0.05))`).
- **`cbind`**: Binds an element variable to the most recently asserted or modified WME timetag. Subsequent actions in the same rule firing can use that element variable in `modify`, `remove`, or pass it as an attribute reference.

### Common Use Cases

1. **Modifying a Newly Created WME**:
   ```ops5
   (make person ^name "Alice" ^age 30)
   (cbind <p>)
   (modify <p> ^age 31)
   ```

2. **Chaining Multiple Modifications in a Single Rule Firing**:
   ```ops5
   ; Modify existing element matched on LHS
   (modify <item> ^status in-progress)
   ; Rebind <item> to the newly asserted replacement WME
   (cbind <item>)
   ; Safely apply another modification to the new element
   (modify <item> ^attempts 1)
   ```

3. **Establishing Relational References Between WMEs**:
   ```ops5
   (make order ^id 101 ^total 0)
   (cbind <ord>)
   (make log-entry ^order-ref <ord> ^message "Created order")
   ```

---

## 6. Complete Workflow Example

The following production rule program calculates cart item totals, computes subtotal and sales tax, and generates an invoice:

```ops5
(literalize item id name price qty total)
(literalize tax-config rate)
(literalize invoice subtotal tax-amount total-due)
(literalize stage name)

(p compute-item-total
   (stage ^name items)
   <it> (item ^id <id> ^name <name> ^price <p> ^qty <q> ^total pending)
   -->
   (bind <item-total> (compute <p> * <q>))
   (modify <it> ^total <item-total>)
   (write (crlf) (tabto 5) <name> (tabto 20) "qty:" <q> (tabto 30) "price:" <p> (tabto 42) "total:" <item-total>)
)

(p finish-items
   <sVar> (stage ^name items)
   -(item ^total pending)
   -->
   (modify <sVar> ^name summary)
)

(p generate-invoice
   <sVar> (stage ^name summary)
   (tax-config ^rate <r>)
   (item ^id 1 ^total <t1>)
   (item ^id 2 ^total <t2>)
   -->
   (bind <subtotal> (compute <t1> + <t2>))
   (bind <tax> (compute <subtotal> * <r>))
   (bind <grand-total> (compute <subtotal> + <tax>))
   (make invoice ^subtotal <subtotal> ^tax-amount <tax> ^total-due <grand-total>)
   (write (crlf) (tabto 5) "--------------------------------------------------" (crlf)
          (tabto 5) "Subtotal:" (tabto 42) <subtotal> (crlf)
          (tabto 5) "Tax:" (tabto 42) <tax> (crlf)
          (tabto 5) "Total Due:" (tabto 42) <grand-total> (crlf))
   (modify <sVar> ^name done)
   (halt)
)
```

### Output Generated
```text
    Gadget         qty: 2    price: 15   total: 30
    Widget         qty: 3    price: 20   total: 60
    --------------------------------------------------
    Subtotal:                            90
    Tax:                                 4.5
    Total Due:                           94.5
```

---

## 7. Implementation References

- **Action & Expression Model**: [`pkg/model/action.go`](../pkg/model/action.go) (`BindAction`, `CBindAction`), [`pkg/model/value.go`](../pkg/model/value.go) (`TypeCompute`, `ComputeExpr`, `ComputeOp`)
- **Parser**: [`pkg/parser/parser.go`](../pkg/parser/parser.go) (`parseCompute`, `case "bind"`, `case "cbind"`)
- **Execution**: [`pkg/engine/engine.go`](../pkg/engine/engine.go) (`evaluateCompute`, `resolveValue`, `Step`, `lastAddedTimetag`)
- **Test Fixtures**:
  - [`tests/fixtures/bind_compute_workflow.ops`](../tests/fixtures/bind_compute_workflow.ops) & [`tests/fixtures/bind_compute_workflow.json`](../tests/fixtures/bind_compute_workflow.json)
  - [`tests/fixtures/cbind_workflow.ops`](../tests/fixtures/cbind_workflow.ops) & [`tests/fixtures/cbind_workflow.json`](../tests/fixtures/cbind_workflow.json)

