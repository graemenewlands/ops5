# OPS5 `substr` RHS Function Reference

The `substr` function extracts a subsequence of values (or a single value) from a specified working memory element (WME). It is primarily used for manipulating multi-valued vector attributes, processing sequences, or extracting attribute values by position.

---

## 1. Syntax

### RHS Function Call
```ops5
(substr <elem-ref> <start> <end>)
```

#### Arguments
1. **`<elem-ref>`**: Specifies the working memory element holding the vector or attributes.
   - **Element variable** (recommended): e.g. `<string-variable>`, bound to a condition element on the LHS.
   - **Condition element index**: 1-based integer corresponding to the CE index in the rule's LHS (e.g. `1`, `2`).
   - In REPL mode, can also be a WME timetag integer.
2. **`<start>`**: Index of the beginning of the subsequence.
   - Attribute name (e.g. `sequence` or `^sequence`).
   - Integer index (1-based WME position, e.g. `2`).
   - Variable evaluating to an attribute name or integer index (e.g. `<start-idx>`).
   - Expression evaluating to an integer (e.g. `(compute (litval sequence) + 1)`).
3. **`<end>`**: Index of the end of the subsequence.
   - Attribute name (e.g. `sequence` or `^sequence`).
   - Integer index (e.g. `4`).
   - Variable evaluating to an attribute name or integer index (e.g. `<end-idx>`).
   - Expression evaluating to an integer.
   - Special symbol **`inf`**: Denotes that the end of the subsequence is the end of the vector attribute containing `<start>`.

---

## 2. Semantics & Positional Layout

### WME Positional Indexing
In OPS5, Working Memory Elements are indexed starting at 1:
- **Position 1**: The class name (e.g. `string`).
- **Position 2**: The first attribute in the element schema.
- **Position $k$**: The $(k-1)$-th attribute in the element schema.
- If an attribute is declared as a `vector-attribute`, its elements occupy consecutive positions starting at the attribute's `litval` index.

For example, given:
```ops5
(literalize string sequence)
(vector-attribute sequence)
(make string ^sequence A B C D)
```
- Position 1: `string`
- Position 2: `A` (first element of `sequence`, `litval sequence = 2`)
- Position 3: `B`
- Position 4: `C`
- Position 5: `D`

### Return Values: Single Element vs. Subsequence
1. **Single Element Access** (`<start> == <end>` and `<end> != inf`):
   - When the beginning and ending arguments specify the exact same position or attribute, `substr` extracts and returns that single scalar value directly.
   - For example:
     - `(substr <str> sequence sequence)` returns scalar `'A'`.
     - `(substr <person> age age)` returns scalar integer `30`.
2. **Subsequence / Range Access** (`<start> < <end>` or `<end> == inf`):
   - Returns a vector (`TypeVector`) containing all elements from position `<start>` to `<end>` inclusive.
   - When `<end>` is `inf`, it extracts from `<start>` through the last element of the vector attribute.
   - For example:
     - `(substr <str> 2 4)` returns `(A B C)`.
     - `(substr <str> 3 inf)` returns `(B C D)`.
3. **Empty or Out-of-Bounds Access**:
   - If `<start> > <end>` or `<start>` exceeds the vector length, `substr` returns an empty vector `()` when evaluating ranges.
   - If single-element access references a non-existent slot, it returns `nil`.

---

## 3. Integration with `litval` and `bind`

In all but the simplest cases, the use of `substr` requires the use of the OPS5 `litval` function and action `bind`.

To extract all elements *after* the first element of a vector attribute:
```ops5
(bind <first> (compute (litval sequence) + 1))
(bind <rest> (substr <string-variable> <first> inf))
```

This pattern can be used inside `modify` actions to incrementally consume or pop elements from a vector:
```ops5
(p Process-Next-Char
   <str> (string ^sequence <first> <second>)
   -->
   (bind <head> (substr <str> sequence sequence))
   (bind <start> (compute (litval sequence) + 1))
   (modify <str> ^sequence (substr <str> <start> inf))
   (write "Processed head:" <head> (crlf))
)
```

---

## 4. REPL Interactive Usage

The `substr` function can also be executed directly within the interactive REPL shell:

```
ops5> (literalize string sequence)
ops5> (vector-attribute sequence)
ops5> make string ^sequence A B C D
Asserted: (1: string ^sequence A B C D)

ops5> (substr 1 sequence sequence)
A

ops5> substr 1 2 4
A B C

ops5> (substr 1 3 inf)
B C D
```

Both bare (`substr 1 3 inf`) and parenthesized (`(substr 1 3 inf)`) command forms are supported.

---

## 5. Summary Table

| Expression | Start | End | Result |
|---|---|---|---|
| `(substr <sVal> sequence sequence)` | `sequence` (2) | `sequence` (2) | First element `'A'` (scalar) |
| `(substr <sVal> 2 2)` | 2 | 2 | Element at position 2 (scalar) |
| `(substr <sVal> 2 4)` | 2 | 4 | Subvector `(A B C)` |
| `(substr <sVal> 3 inf)` | 3 | End of vector | Subvector `(B C D)` |
| `(substr <sVal> (compute (litval sequence) + 1) inf)` | 3 | End of vector | Subvector `(B C D)` |
| `(substr <p> name name)` | `name` | `name` | Value of attribute `name` (scalar) |
