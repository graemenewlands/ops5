# OPS5 `write`, `(crlf)`, and `(tabto N)` Formatting Reference

In OPS5, the `write` action emits text to standard output (or the configured output writer). Beyond printing individual values, OPS5 provides formatting directives—specifically `(crlf)` and `(tabto N)`—to construct formatted reports, tables, and aligned grids.

---

## 1. Action Syntax

Inside the Right-Hand Side (RHS) of a production rule:

```ops5
(write <item-1> <item-2> ... <item-N>)
```

Items passed to `write` can be:
- **Literals**: Strings (`"ID:"`), symbols (`active`), integers (`101`), floats (`3.14`), booleans (`true`, `false`).
- **Variables**: Bound variables (e.g. `<id>`, `<name>`) evaluated from LHS condition matching.
- **`(crlf)`** (or bare `crlf`): Emits a newline (carriage-return / line-feed) and resets the current column counter to 1.
- **`(tabto <column>)`**: Moves the output cursor forward to the specified 1-based column position by padding with spaces. `<column>` can be an integer literal or a bound integer variable.

---

## 2. Formatting Mechanics

### Column Counter
- The engine maintains a 1-based horizontal column tracker (`currentCol`) across written characters.
- Column 1 represents the leftmost character on a line.
- Emitting a newline resets `currentCol` to 1.

### `(tabto <column>)`
- If the current column is strictly less than `<column>`, spaces are emitted until column `<column>` is reached (`strings.Repeat(" ", column - currentCol)`).
- If the current column is already greater than or equal to `<column>`, a single space separator is emitted to prevent tokens from colliding.
- Variables can specify the target column dynamically: `(tabto <col-var>)`.

### Newline Rules
- If `(crlf)` is present anywhere within the `(write ...)` action, newlines are emitted exclusively at the designated `(crlf)` points.
- If no `(crlf)` appears in the `(write ...)` action, a trailing newline is automatically appended at the end of the action for convenience and backward compatibility.

---

## 3. Grid & Table Formatting Example

Here is a complete example constructing a formatted report grid with headers, aligned data columns, and footers:

```ops5
(literalize City id name pop)
(literalize stage current)

(p print-header
   <sVar> (stage ^current header)
   -->
   (write (crlf) (tabto 5) "ID" (tabto 15) "CITY" (tabto 30) "POPULATION" (crlf)
          (tabto 5) "--" (tabto 15) "----" (tabto 30) "----------" (crlf))
   (modify <sVar> ^current data)
)

(p print-row
   (stage ^current data)
   <c> (City ^id <id> ^name <name> ^pop <pop>)
   -->
   (write (tabto 5) <id> (tabto 15) <name> (tabto 30) <pop> (crlf))
   (remove <c>)
)

(p print-footer
   <sVar> (stage ^current data)
   -(City)
   -->
   (write (tabto 5) "--" (tabto 15) "----" (tabto 30) "----------" (crlf)
          (tabto 5) "End of Report" (crlf))
   (remove <sVar>)
   (halt)
)
```

### Standard Output Produced

```text
    ID        CITY           POPULATION
    --        ----           ----------
    102       Cambridge      118000
    101       Boston         675000
    --        ----           ----------
    End of Report
```

Notice:
- `ID` is aligned at column 5.
- `CITY` is aligned at column 15.
- `POPULATION` is aligned at column 30.

---

## 4. Implementation References

- **Parser**: [`pkg/parser/parser.go`](../pkg/parser/parser.go) (`parseAction` case `"write"`)
- **Action Model**: [`pkg/model/action.go`](../pkg/model/action.go) (`WriteArg`, `WriteCRLF`, `WriteTabTo`, `WriteAction`)
- **Engine Execution**: [`pkg/engine/engine.go`](../pkg/engine/engine.go) (`case model.WriteAction` with column tracking)
- **Test Fixture**: [`tests/fixtures/grid_formatting.ops`](../tests/fixtures/grid_formatting.ops) & [`tests/fixtures/grid_formatting.json`](../tests/fixtures/grid_formatting.json)
