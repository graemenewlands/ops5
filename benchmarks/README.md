# OPS5 Canonical Benchmark Suite

This directory contains the canonical rule-based system benchmarks adapted for the OPS5 runtime:

* **Miss Manners (`manners/`)**: Combinatorial guest seating with gender alternation and common hobbies.
* **Waltz Line Labeling (`waltz/`)**: David Waltz's 2D/3D polyhedral edge labeling algorithm.
* **Zebra Puzzle (`zebra/`)**: Einstein's classic 14-clue logic riddle.

## Documentation & Performance History

* **[Benchmark Timing History & Release Log](../docs/benchmarks_history.md)**: Full historical performance tracking across releases and optimizations.
* **[Engine Comparison & Architectural Evaluation](../docs/engine_comparison.md)**: Comparative analysis against CMU OPS5, NASA CLIPS, and Apache Drools.

## Running Benchmarks

```bash
# Run fast suite (skips Manners-64):
go test -v -short ./...

# Run full suite:
go test -v ./...

# Run microbenchmarks reporting throughput:
go test -bench=. -benchtime=1x -run=^$ ./...
```
