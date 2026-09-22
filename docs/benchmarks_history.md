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
| **`v0.2.0`** | 2026-09-22 | Token prefix spine sharing, binary heap agenda, zero-copy bindings, single-pass timetags | **442 ms** (-75%) | **744 ms** (-77%) | **8.56 s** (-75%) | **17 ms** (-74%) | **58 ms** (-91%) | **0.12 ms** (-37%) |
| **[`v0.1.0`](#v010---2026-09-22-baseline)** | 2026-09-22 | Dual-sided join hashing, Structural beta sharing, Left/Right node unlinking, Rule salience | 1.74 s | 3.23 s | 34.20 s | 65 ms | 662 ms | 0.19 ms |

---

## Version Release Logs

### `v0.2.0` - 2026-09-22 (Token Prefix Spine Sharing, Binary Heap Agenda & Zero-Copy Bindings)

* **Key Optimizations**:
  - **Token Prefix Spine Sharing & Allocation-Free Variable Lookups (`Token.GetBinding`)**: Completely eliminated dynamic map allocation and map copying across beta memory token trees. Condition elements accumulate bindings in stack-allocated slices (`[]Binding`), while variable lookups traverse the token ancestor spine in L1 cache ($O(\text{depth})$) with zero hashing and zero heap allocations. Saved **17.3 GB of heap churn (-84%)** on Manners-64 and cut runtimes by up to **4.3x** across all scales.
  - **Indexed Binary Max-Heap Agenda (`conflict.Set`)**: Transformed conflict set agenda into a container/heap binary max-heap with an auxiliary index map. Dominant instantiation selection is now $O(1)$ and activation addition/refraction/removal is $O(\log K)$, delivering an **11.4x speedup on Waltz-50** (662ms -> 58ms) and reducing Waltz heap allocation from 447 MB to 22 MB.
  - **Single-Pass Allocation-Free Timetag & WME Materialization**: Rewrote `Token.Timetags()` and `Token.WMEs()` to pre-count elements and fill slices backwards in condition element order, eliminating intermediate reverse-chain slice allocations.
  - **Single & Double Successor Propagation Fast Paths**: Direct unrolled calls in `JoinNode.propagate`, `BetaMemory.LeftActivation`, and `EvalNode.LeftActivation` when successor count $\le 2$, eliminating millions of transient `[]LeftActivatable` slice allocations during pattern matching.
  - **Token Signature Caching & Index Key Optimizations**: Precomputed `sig` string at token creation via incremental string derivation and direct `strconv.FormatInt` formatting.

#### Benchmark Suite Results

```
========================================================================================================================
Benchmark    |   Cycles | WMEs Assert |   Quiescence |     Cycles/sec |       WMEs/sec |   Heap Alloc
------------------------------------------------------------------------------------------------------------------------
Manners-16   |     2009 |       2744 |        442ms |         4542.4 |         6204.2 |  167786.16 KB (~164 MB)
Manners-32   |     3914 |       4649 |        744ms |         5259.5 |         6247.2 |  322188.17 KB (~315 MB)
Manners-64   |    19381 |      21152 |       8.563s |         2263.5 |         2470.3 | 3321618.89 KB (~3.17 GB)
Waltz-12     |      608 |       1238 |         17ms |        35827.9 |        72952.2 |    5707.52 KB (~5.6 MB)
Waltz-50     |     2268 |       4654 |         58ms |        38818.0 |        79655.7 |   22167.38 KB (~21.6 MB)
Zebra-5      |        7 |         19 |       0.12ms |        25360.9 |        68836.8 |      72.44 KB (~72 KB)
========================================================================================================================
```

#### Go Microbenchmark Metrics (`go test -bench`)

```
BenchmarkSuiteManners16-16    1     427775686 ns/op    4696 cycles/s     6415 wmes/s
BenchmarkSuiteManners32-16    1     765413524 ns/op    5114 cycles/s     6074 wmes/s
BenchmarkSuiteWaltz12-16      1      13583807 ns/op   44771 cycles/s    91162 wmes/s
BenchmarkSuiteWaltz50-16      1      57755099 ns/op   39272 cycles/s    80586 wmes/s
BenchmarkSuiteZebra-16        1        116198 ns/op   61350 cycles/s   166522 wmes/s
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
