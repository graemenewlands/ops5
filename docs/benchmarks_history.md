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
| **`v0.2.0`** | 2026-09-22 | Token prefix spine sharing, binary heap agenda, alpha switch nodes, zero-copy bindings | **413 ms** (-76%) | **761 ms** (-76%) | **8.41 s** (-75%) | **15 ms** (-77%) | **52 ms** (-92%) | **0.12 ms** (-37%) |
| **[`v0.1.0`](#v010---2026-09-22-baseline)** | 2026-09-22 | Dual-sided join hashing, Structural beta sharing, Left/Right node unlinking, Rule salience | 1.74 s | 3.23 s | 34.20 s | 65 ms | 662 ms | 0.19 ms |

---

## Version Release Logs

### `v0.2.0` - 2026-09-22 (Token Prefix Spine Sharing, Binary Heap Agenda, Alpha Constant Switch Nodes & Zero-Copy Bindings)

* **Key Optimizations**:
  - **Token Prefix Spine Sharing & Allocation-Free Variable Lookups (`Token.GetBinding`)**: Completely eliminated dynamic map allocation and map copying across beta memory token trees. Condition elements accumulate bindings in stack-allocated slices (`[]Binding`), while variable lookups traverse the token ancestor spine in L1 cache ($O(\text{depth})$) with zero hashing and zero heap allocations. Saved **17.3 GB of heap churn (-84%)** on Manners-64 and cut runtimes by up to **4.3x** across all scales.
  - **Indexed Binary Max-Heap Agenda (`conflict.Set`)**: Transformed conflict set agenda into a container/heap binary max-heap with an auxiliary index map. Dominant instantiation selection is now $O(1)$ and activation addition/refraction/removal is $O(\log K)$, delivering an **11.4x speedup on Waltz-50** (662ms -> 52ms) and reducing Waltz heap allocation from 447 MB to 22 MB.
  - **Alpha Constant Hash / Switch Nodes & Node Sharing (`AlphaSwitchNode`)**: Replaced sequential `ConstantTestNode` chains with immediate $O(1)$ constant dispatch map indexing for intra-condition equality tests. Canonical sorting of condition element tests enables full prefix node sharing across both switch nodes and test nodes, skipping non-matching attribute branches with zero comparisons.
  - **Single-Pass Allocation-Free Timetag & WME Materialization**: Rewrote `Token.Timetags()` and `Token.WMEs()` to pre-count elements and fill slices backwards in condition element order, eliminating intermediate reverse-chain slice allocations.
  - **Single & Double Successor Propagation Fast Paths**: Direct unrolled calls in `JoinNode.propagate`, `BetaMemory.LeftActivation`, and `EvalNode.LeftActivation` when successor count $\le 2$, eliminating millions of transient `[]LeftActivatable` slice allocations during pattern matching.
  - **Token Signature Caching & Index Key Optimizations**: Precomputed `sig` string at token creation via incremental string derivation and direct `strconv.FormatInt` formatting.

#### Benchmark Suite Results

```
========================================================================================================================
Benchmark    |   Cycles | WMEs Assert |   Quiescence |     Cycles/sec |       WMEs/sec |   Heap Alloc
------------------------------------------------------------------------------------------------------------------------
Manners-16   |     2009 |       2744 |        413ms |         4865.0 |         6644.9 |  167793.18 KB (~164 MB)
Manners-32   |     3914 |       4649 |        761ms |         5145.2 |         6111.4 |  322288.75 KB (~315 MB)
Manners-64   |    19381 |      21152 |       8.411s |         2304.4 |         2514.9 | 3321835.27 KB (~3.17 GB)
Waltz-12     |      608 |       1238 |         15ms |        41820.1 |        85153.4 |    5732.71 KB (~5.6 MB)
Waltz-50     |     2268 |       4654 |         52ms |        43695.6 |        89664.6 |   22275.93 KB (~21.8 MB)
Zebra-5      |        7 |         19 |       0.12ms |        57736.7 |       156714.0 |      72.52 KB (~72 KB)
========================================================================================================================
```

#### Go Microbenchmark Metrics (`go test -bench`)

```
BenchmarkSuiteManners16-16    1     433547516 ns/op    4634 cycles/s     6329 wmes/s
BenchmarkSuiteManners32-16    1     757109922 ns/op    5170 cycles/s     6140 wmes/s
BenchmarkSuiteWaltz12-16      1      12806894 ns/op   47487 cycles/s    96693 wmes/s
BenchmarkSuiteWaltz50-16      1      67930482 ns/op   33388 cycles/s    68514 wmes/s
BenchmarkSuiteZebra-16        1        118735 ns/op   59979 cycles/s   162801 wmes/s
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
