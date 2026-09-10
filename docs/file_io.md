# File I/O, Stream Redirection & Input in OPS5

This document details the file stream input/output management, default stream redirection, vertical bar symbols, and input functions supported in this OPS5 implementation.

---

## 1. Overview

OPS5 provides stream-oriented file I/O and standard I/O redirection:
* **`openfile`**: Associates an operating system file path with a logical name in read (`in`), write/truncate (`out`), or write/append (`append`) mode.
* **`closefile`**: Flushes and closes a stream associated with a logical name.
* **`default`**: Directs the default stream for the `accept`, `write`, or `trace` subsystems to a logical file name, or restores terminal standard I/O.
* **`accept`**: RHS function that reads the next whitespace-delimited atom from a stream (or default input).
* **`acceptline`**: RHS function that reads a full line from a stream (or default input), returning a vector or scalar.
* **Vertical Bar Symbols (`|...|`)**: Literal symbol syntax for file specifications with punctuation or spaces (e.g. `|RuleTrace.ops|`).

---

## 2. Vertical Bar Symbols (`|...|`)

In classic OPS5, file names and special symbols containing periods, hyphens, slashes, or whitespace can be enclosed in vertical bars:
```lisp
|RuleTrace.ops|
|/tmp/ops5_data.txt|
|data file.in|
```
The lexer preserves the exact characters within the vertical bars as a single symbol token.

---

## 3. File Actions

### `openfile`
Opens a file and registers it under a logical name.

**Syntax:**
```lisp
(openfile <logical-name> <filespec> <mode>)
```
* `<logical-name>`: An atom identifier representing the file stream.
* `<filespec>`: A symbol, string, or bound variable containing the file path (e.g., `|trace.log|`, `"trace.log"`, `<filepath>`).
* `<mode>`: Stream mode:
  * `in`: Open for reading (read-only).
  * `out`: Open for writing (creates or truncates file).
  * `append`: Open for writing (creates or appends to file).

**Examples:**
```lisp
(openfile ruletrace |RuleTrace.ops| out)
(openfile inlog |data.txt| in)
(openfile audit "audit.log" append)
```

---

### `closefile`
Closes an open file stream.

**Syntax:**
```lisp
(closefile <logical-name>)
```

**Examples:**
```lisp
(closefile ruletrace)
(closefile inlog)
```
If a logical file being closed is currently set as the default stream for `accept`, `write`, or `trace`, the subsystem automatically reverts to standard terminal I/O.

---

### `default`
Redirects the default stream for an I/O subsystem, or restores standard I/O.

**Syntax:**
```lisp
(default <logical-name> <subsystem>)
```
* `<logical-name>`: An active logical file name, or `nil` / `terminal` / `t` / `stdin` / `stdout` to restore standard stream.
* `<subsystem>`: One of:
  * `accept`: Default stream for `(accept)` and `(acceptline)` reading.
  * `write`: Default destination for `(write ...)` actions.
  * `trace`: Destination for rule execution firing logs when tracing is enabled.

**Examples:**
```lisp
; Direct trace output to file
(default ruletrace trace)

; Direct write output to file
(default ruletrace write)

; Direct accept input to read from file
(default inlog accept)

; Restore standard terminal input / output
(default nil accept)
(default nil write)
(default nil trace)
```

---

## 4. Input Functions: `accept` & `acceptline`

`accept` and `acceptline` are RHS value expressions that can be used in `make`, `modify`, `bind`, and `write` actions.

### `accept`
Reads the next whitespace-delimited word from standard input (or the logical stream).
* Converts numeric strings to integer or float values automatically.
* Returns symbol `end-of-file` when EOF is reached.
* Syntax:
  * `(accept)`: Reads from default accept stream (terminal or redirected by `default`).
  * `(accept <logical-name>)`: Reads from the specified logical file.

**Example:**
```lisp
(p prompt-user
   (stage ^name enter-id)
   -->
   (write "Enter user ID:" (crlf))
   (make user ^id (accept))
)
```

---

### `acceptline`
Reads an entire line of input up to `\n`:
* If line is empty: returns symbol `nil`.
* If line has a single word: returns that value (symbol, int, float).
* If line has multiple words: returns a multi-valued vector of values.
* Returns symbol `end-of-file` on EOF.
* Syntax:
  * `(acceptline)`: Reads from default accept stream.
  * `(acceptline <logical-name>)`: Reads from the specified logical file.

**Example:**
```lisp
(p read-tags
   (stage ^name enter-tags)
   -->
   (make metadata ^tags (acceptline inlog))
)
```

---

## 5. REPL Usage

In the interactive shell, file commands can be entered with or without parentheses:

```
ops5> (openfile ruletrace |RuleTrace.ops| out)
Opened file 'RuleTrace.ops' as ruletrace (out)

ops5> (default ruletrace trace)
Default for trace set to 'ruletrace'

ops5> default
Default streams: accept=terminal write=terminal trace=ruletrace

ops5> (closefile ruletrace)
Closed file 'ruletrace'
```

---

## 6. End-to-End Production Rule Example

```lisp
(literalize stage name)
(literalize person name age)

(p export-person
   <st> (stage ^name export)
   <p>  (person ^name <name> ^age <age>)
   -->
   (openfile outf |records.txt| out)
   (default outf write)
   (write "RECORD:" <name> <age> (crlf))
   (closefile outf)
   (default nil write)
   (modify <st> ^name import)
)

(p import-person
   <st> (stage ^name import)
   -->
   (openfile inf |records.txt| in)
   (default inf accept)
   (make imported-record ^data (acceptline))
   (closefile inf)
   (modify <st> ^name done)
   (halt)
)
```
