// Package wm provides the dynamic working memory storage and timetag coordination for the OPS5 production system.
//
// Working Memory (WM) maintains the active pool of facts (Working Memory Elements / WMEs) asserted into
// the system. Each WME is assigned an immutable, monotonically increasing integer timetag that uniquely
// identifies the fact and determines its recency during conflict resolution (LEX and MEA strategies).
//
// # Key Semantics
//
//   - Monotonic Timetags: Every asserted WME receives a strictly sequential timetag (1, 2, 3, ...).
//     Timetags never repeat and are never reused during an engine's lifecycle.
//
//   - Modification Semantics: In classic OPS5 semantics, modifying a WME via (modify ...) does not mutate
//     the existing WME in place. Instead, the existing WME is retracted, and a new WME is asserted with
//     a fresh timetag and updated attribute values, preserving unmodified attributes. This guarantees
//     proper refraction and updates the fact's recency score.
//
//   - Observer Pattern (Listener): The Rete discrimination network registers as a Listener on working
//     memory to receive OnAssert and OnRetract notifications whenever facts enter or leave memory.
//
//   - Batch Ingestion: MakeWithoutNotify allows high-throughput assertion of WMEs into working memory
//     without immediately notifying listeners, facilitating concurrent alpha evaluation across worker pools.
//
//   - Thread Safety: All read and write operations on WorkingMemory are synchronized using an internal
//     sync.RWMutex, enabling safe concurrent inspection, batch assertions, and partition routing.
//
// # Example Usage
//
//	// Create working memory instance
//	memory := wm.New()
//
//	// Assert a new WME
//	wme1 := memory.Make("person", map[string]model.Value{
//	    "name": model.NewSymbol("alice"),
//	    "age":  model.NewInt(30),
//	})
//	fmt.Printf("Asserted WME timetag=%d, class=%s\n", wme1.Timetag, wme1.Class)
//
//	// Modify the WME (retracts wme1 and creates wme2 with new timetag)
//	wme2, err := memory.Modify(wme1.Timetag, map[string]model.Value{
//	    "age": model.NewInt(31),
//	})
//
//	// Retract the WME
//	_, err = memory.Remove(wme2.Timetag)
package wm
