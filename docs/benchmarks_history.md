# OPS5 Benchmark Timing History & Performance Log

This document maintains the historical benchmark timings across versions and optimization releases of the **OPS5** production rule system. 

> [!NOTE]
> The performance metrics listed in the root [`README.md`](../README.md) always reflect the **current library version**. When new optimizations are developed and verified, benchmarks are re-run on the standardized test environment, and a new historical entry is appended to this log.

---

## Benchmark Methodology & Machine Architecture

All benchmarks are executed on a dedicated, bare-metal standardized test environment:

| Component | Specification |
| :--- | :--- |
| **System Host** | Intel NUC (`graeme-nuc`) |
| **Processor (CPU)** | 12th Gen Intel(R) Core(TM) i7-1260P (Alder Lake) |
| **Architecture** | `x86_64` (64-bit, Little Endian, 39-bit physical, 48-bit virtual) |
| **Cores & Threads** | 12 Cores (4 Performance-cores + 8 Efficient-cores), 16 Threads |
| **CPU Frequency** | 400 MHz base / 4,700 MHz (4.70 GHz) max boost |
| **L1 Data Cache** | 448 KiB (12 instances) |
| **L1 Instruction Cache** | 640 KiB (12 instances) |
| **L2 Unified Cache** | 9 MiB (6 instances) |
| **L3 Shared Cache** | 18 MiB (1 instance) |
| **System Memory (RAM)** | 64 GB (62.5 GiB physical, single NUMA node) |
| **Operating System** | Ubuntu 24.04.5 LTS (Noble Numbat) |
| **Linux Kernel** | `6.8.0-139-generic` (SMP PREEMPT_DYNAMIC x86_64) |
| **Go Compiler / Runtime** | `go version go1.23.2 linux/amd64` (`CGO_ENABLED=0`) |
| **Conflict Strategy** | `LEX` (`conflict.StrategyLEX`) with watch level 0 |

### Benchmark Commands
```bash
# Full canonical benchmark suite execution (including Manners-64):
go test -v -run TestBenchmarkSuite ./benchmarks

# Microbenchmark throughput verification:
go test -bench=. -benchtime=1x -run=^$ ./benchmarks
```

---

## Historical Version Comparison

| Version / Tag | Release Date | Key Optimizations / Features | Manners-16 | Manners-32 | Manners-64 | Waltz-12 | Waltz-50 | Zebra-5 |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **`v0.2.0`** | 2026-09-22 | Token signature caching, zero-copy binding sharing, single-key RightActivation fast path | **857 ms** (-51%) | **1.59 s** (-51%) | **17.78 s** (-48%) | **64 ms** (-2%) | **659 ms** (-1%) | **0.16 ms** (-16%) |
| **[`v0.1.0`](#v010---2026-09-22-baseline)** | 2026-09-22 | Dual-sided join hashing, Structural beta sharing, Left/Right node unlinking, Rule salience | 1.74 s | 3.23 s | 34.20 s | 65 ms | 662 ms | 0.19 ms |

---

## Version Release Logs

### `v0.2.0` - 2026-09-22 (Token Signature Caching & Binding Sharing)

* **Key Optimizations**:
  - **Token Signature Caching**: Precomputed `sig` string at token creation via incremental string derivation, eliminating hundreds of thousands of `fmt.Sprintf` calls, slice allocations, and recursive `Timetags()` walks in beta memory indexing and duplicate detection.
  - **Zero-Copy Binding Sharing**: When a condition element introduces no new variable bindings, child tokens share the parent's binding map directly instead of copying, saving thousands of map allocations.
  - **Single-Key RightActivation Fast Path**: Bypasses allocating a `seen map[string]bool` when an incoming WME matches a single join key (the common case).
  - **Index Key Optimizations**: Direct `strconv.FormatInt` formatting in `CanonicalValueKey` and fast-path single-variable key extraction in `KeyForToken`.

#### Benchmark Suite Results

```
========================================================================================================================
Benchmark    |   Cycles | WMEs Assert |   Quiescence |     Cycles/sec |       WMEs/sec |   Heap Alloc
------------------------------------------------------------------------------------------------------------------------
Manners-16   |     2009 |       2744 |        857ms |         2344.3 |         3201.9 |  543049.16 KB (~530 MB)
Manners-32   |     3914 |       4649 |        1.59s |         2461.2 |         2923.4 | 1073987.34 KB (~1.02 GB)
Manners-64   |    19381 |      21152 |      17.781s |         1090.0 |         1189.6 | 11206724.62 KB (~10.7 GB)
Waltz-12     |      608 |       1238 |         64ms |         9543.4 |        19432.0 |   35563.23 KB (~34.7 MB)
Waltz-50     |     2268 |       4654 |        659ms |         3444.0 |         7067.1 |  433674.98 KB (~423 MB)
Zebra-5      |        7 |         19 |       0.16ms |        43171.8 |       117180.5 |      94.70 KB (~95 KB)
========================================================================================================================
```

#### Go Microbenchmark Metrics (`go test -bench`)

```
BenchmarkSuiteManners16-16    1     811539191 ns/op    2476 cycles/s    3381 wmes/s
BenchmarkSuiteManners32-16    1    1616702229 ns/op    2421 cycles/s    2876 wmes/s
BenchmarkSuiteWaltz12-16      1      54773614 ns/op   11101 cycles/s   22604 wmes/s
BenchmarkSuiteWaltz50-16      1     686426072 ns/op    3304 cycles/s    6780 wmes/s
BenchmarkSuiteZebra-16        1        150860 ns/op   47120 cycles/s  127896 wmes/s
```

---

### `v0.1.0` - 2026-09-22 (Baseline)

* **Git Tag**: [`v0.1.0`](https://github.com/graemenewlands/ops5/releases/tag/v0.1.0)
* **Git Commit**: [`5901639`](https://github.com/graemenewlands/ops5/commit/59016394b8be6614938fdc28fb863135d93ccbaf)
* **Description**: Initial baseline release implementing the Charles Forgy OPS5 specification in Go with:
  - Dual-sided composite hash indexing on join nodes (`JoinNode.betaHashIndex` and `alphaHashIndex`).
  - Structural Beta Node Sharing across rules sharing common condition prefixes (`net.betaPool`).
  - Left & Right Node Unlinking (Forgy/Doorenbos) for two-input nodes.
  - Full conflict resolution (LEX & MEA with strict refraction) and rule salience.
  - S-expression parser, interactive REPL, Graphviz `--dot` export, and JSON test harness.

#### Benchmark Suite Results

```
========================================================================================================================
Benchmark    |   Cycles | WMEs Assert |   Quiescence |     Cycles/sec |       WMEs/sec |   Heap Alloc
------------------------------------------------------------------------------------------------------------------------
Manners-16   |     2009 |       2744 |       1.738s |         1156.0 |         1578.9 | 1002075.86 KB (~978 MB)
Manners-32   |     3914 |       4649 |       3.227s |         1213.0 |         1440.8 | 1980060.75 KB (~1.93 GB)
Manners-64   |    19381 |      21152 |      34.204s |          566.6 |          618.4 | 20605734.83 KB (~20.1 GB)
Waltz-12     |      608 |       1238 |         65ms |         9384.5 |        19108.6 |   39169.73 KB (~38.2 MB)
Waltz-50     |     2268 |       4654 |        662ms |         3426.4 |         7031.1 |  447665.09 KB (~437 MB)
Zebra-5      |        7 |         19 |       0.19ms |        36965.3 |       100334.3 |     134.52 KB (~134 KB)
========================================================================================================================
```

#### Go Microbenchmark Metrics (`go test -bench`)

```
BenchmarkSuiteManners16-16    1    1799440989 ns/op    1116 cycles/s    1525 wmes/s
BenchmarkSuiteManners32-16    1    3396780430 ns/op    1152 cycles/s    1369 wmes/s
BenchmarkSuiteWaltz12-16      1      66925083 ns/op    9086 cycles/s   18500 wmes/s
BenchmarkSuiteWaltz50-16      1     692832364 ns/op    3274 cycles/s    6717 wmes/s
BenchmarkSuiteZebra-16        1        176868 ns/op   40092 cycles/s  108820 wmes/s
```

---

## Protocol for Logging New Optimizations

When introducing a new optimization (e.g. Token Prefix Spine Sharing, Binary Heap Agenda, Alpha Switch Nodes):

1. **Implement and Verify**:
   - Ensure all existing unit, harness, and race tests pass: `go test -race ./...`.
2. **Execute Full Suite**:
   - Run the benchmark suite:
     ```bash
     go test -v -run TestBenchmarkSuite ./benchmarks
     go test -bench=. -benchtime=1x -run=^$ ./benchmarks
     ```
3. **Update Documentation**:
   - Append a new section to this file ([`docs/benchmarks_history.md`](benchmarks_history.md)) under **Version Release Logs** with the git tag/commit and updated timing table.
   - Update the current performance table in the root [`README.md`](../README.md) to reflect the new timings of the library.
4. **Tag & Push**:
   - Tag the release in git and trigger module indexing on `proxy.golang.org`.
