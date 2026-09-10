# OPS5 `litval` Attribute Index Function Reference

The `litval` function returns the numeric index (or vector position) assigned to an attribute name within an element class or working memory element (WME).

---

## 1. Syntax

### RHS Function Call
```ops5
(litval <attribute-name>)
(litval <class-name> <attribute-name>)
```

- `<attribute-name>`: The target attribute identifier. Can be specified with or without a leading caret `^` (e.g. `name` or `^name`), as a symbol, string, or bound variable (e.g. `<attr>`).
- `<class-name>` *(optional)*: The class to which the attribute belongs. If omitted, `litval` searches across all registered schemas and defined attributes.

### REPL Interactive Command
In the interactive REPL shell, `litval` can be invoked directly:
```
ops5> (make City ^name Albuquerque ^state NM)
Asserted: (1: City ^name Albuquerque ^state NM)

ops5> (litval name)
2

ops5> (litval state)
3

ops5> (litval City name)
2
```

Both bare (`litval name`) and parenthesized (`(litval name)`) forms are supported.

---

## 2. Semantics & Indexing Rules

1. **WME Vector Layout**:
   In standard OPS5, Working Memory Elements are internally laid out as fixed-position vectors:
   - **Position 1**: The class name (e.g. `City`).
   - **Position 2**: The first attribute in the element structure.
   - **Position 3**: The second attribute in the element structure.
   - **Position N**: The $(N-1)$-th attribute.

2. **Schema Integration (`literalize`)**:
   When a class is declared using `(literalize <class> <attr1> <attr2> ...)`:
   - `<attr1>` is assigned index **2**.
   - `<attr2>` is assigned index **3**.
   - `<attr3>` is assigned index **4**, and so forth.

3. **Dynamic Elements (`make`)**:
   If a class is asserted via `make` without prior `literalize` declaration:
   ```ops5
   (make City ^name Albuquerque ^state NM)
   ```
   The engine automatically registers an inferred schema based on the attribute order of occurrence:
   - `name` is assigned index **2**.
   - `state` is assigned index **3**.

4. **Return Value**:
   - `litval` evaluates to an integer (`TypeInteger`).
   - If the attribute does not exist in the specified class or schema, `litval` evaluates to `nil` in RHS expressions or reports an unknown attribute error in the REPL.

---

## 3. Usage Contexts

The `(litval ...)` function can appear in:

1. **`bind` Actions**:
   Assigning the slot index to a local rule variable:
   ```ops5
   (bind <slot> (litval name))
   ```

2. **`compute` Expressions**:
   Performing arithmetic with attribute positions:
   ```ops5
   (bind <next-slot> (compute (litval name) + 1))
   ```

3. **`make` & `modify` Actions**:
   Recording attribute schema metadata into working memory:
   ```ops5
   (make schema-info ^class City ^attr name ^index (litval name))
   ```

4. **`write` Actions**:
   Directly outputting attribute indices to standard output or log streams:
   ```ops5
   (write "Attribute 'name' is at index" (litval name) (crlf))
   ```

---

## 4. Examples

### Inspecting Dynamic Classes
```ops5
(make City ^name Albuquerque ^state NM)

(p report-indices
    (City ^name <city>)
  -->
    (write "Field 'name' is index:" (litval name) (crlf))
    (write "Field 'state' is index:" (litval state) (crlf))
)
```

### Inspecting Literalized Classes
```ops5
(literalize item id name price qty total)

(p index-table
    (start)
  -->
    (write "id:" (litval item id) (crlf))       ; prints 2
    (write "name:" (litval item name) (crlf))   ; prints 3
    (write "price:" (litval item price) (crlf)) ; prints 4
    (write "qty:" (litval item qty) (crlf))     ; prints 5
    (write "total:" (litval item total) (crlf)) ; prints 6
)
```
