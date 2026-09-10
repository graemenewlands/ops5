# OPS5 `genatom` Unique Atom Generation Reference

OPS5 provides the `(genatom)` function to dynamically generate unique symbolic atoms at runtime.

---

## 1. Syntax

### RHS Function Call
```ops5
(genatom)
```

The `genatom` function takes no arguments and evaluates to a unique symbolic atom each time it is invoked.

### REPL Command
In the interactive REPL, `genatom` can be executed as a standalone command:
```
ops5> (genatom)
atom1
ops5> genatom
atom2
```

---

## 2. Semantics & Generation Behavior

- **Sequential Generation**: Atoms are generated in ascending sequence: `atom1`, `atom2`, `atom3`, ..., `atomN`.
- **Type**: The generated value is a symbolic atom (`TypeSymbol`), equivalent in type and behavior to any literal symbol identifier.
- **Monotonic Sequence**: The atom sequence counter is maintained per engine instance and increments atomically each time `(genatom)` is evaluated.
- **Reset**: Resetting the engine state (via `engine.ResetGenatom()` or the REPL `reset` command) resets the sequence back to `atom1`.
- **Deterministic Resolution**: When multiple attributes in a `make` or `modify` action use `(genatom)`, attributes are evaluated in schema attribute order (or alphabetical order if not schematized), ensuring deterministic atom numbering.

---

## 3. Usage Contexts

The `(genatom)` function can be used in:

1. **`make` Actions**:
   Generating unique IDs or keys when asserting new working memory elements.
   ```ops5
   (make order ^id (genatom) ^status pending)
   ```

2. **`modify` Actions**:
   Assigning a new unique atom to an existing WME's attribute.
   ```ops5
   (modify <item> ^tx-id (genatom))
   ```

3. **`bind` Actions**:
   Binding a generated atom to a local variable for reuse across multiple actions in the same rule firing.
   ```ops5
   (bind <new-id> (genatom))
   (make task ^id <new-id> ^status ready)
   (make log ^task-id <new-id> ^message "created")
   ```

4. **`write` Actions**:
   Directly outputting unique generated atoms to terminal or file streams.
   ```ops5
   (write "Assigned token: " (genatom) (crlf))
   ```

5. **Top-Level `make` Statements**:
   Within `.ops` files or REPL assertions.
   ```ops5
   (make session ^token (genatom))
   ```

---

## 4. Examples

### Generating Unique Transaction IDs

```ops5
(literalize transaction id customer amount status)

(p process-deposit
    (deposit-request ^customer <cust> ^amount <amt>)
  -->
    (bind <tx-id> (genatom))
    (make transaction ^id <tx-id> ^customer <cust> ^amount <amt> ^status approved)
    (write "Created transaction " <tx-id> " for customer " <cust> (crlf))
)
```

### Correlating Entities in Multi-Step Workflows

```ops5
(literalize job id step)
(literalize step-log job-id action)

(p initialize-job
    (job-request ^name <name>)
  -->
    (bind <jid> (genatom))
    (make job ^id <jid> ^step 1)
    (make step-log ^job-id <jid> ^action "job initialized")
)
```
