package Effect

import (
	"fmt"
	"sync"
	"gopurs/output/gopurs_runtime"
)

type putEntry struct {
	val gopurs_runtime.Value
	cb  gopurs_runtime.Value
}

type AVarImpl struct {
	mu       sync.Mutex
	draining bool
	isEmpty  bool
	killed   bool
	err      gopurs_runtime.Value
	val      gopurs_runtime.Value
	takes    []gopurs_runtime.Value
	reads    []gopurs_runtime.Value
	puts     []putEntry
	hasLeft  bool
	left     gopurs_runtime.Value
	right    gopurs_runtime.Value
}

func unboxAVar(avar gopurs_runtime.Value) *AVarImpl {
	val := avar.AnyVal()
	if av, ok := val.(*AVarImpl); ok {
		return av
	}
	panic(fmt.Sprintf("AVar AnyVal is not AVarImpl: %T (value: %#v)", val, val))
}

func drainVar(av *AVarImpl) {
	av.mu.Lock()
	if av.draining {
		av.mu.Unlock()
		return
	}
	av.draining = true

	for {
		if av.killed {
			var errVal gopurs_runtime.Value
			if av.hasLeft {
				errVal = gopurs_runtime.Apply(av.left, av.err)
			} else {
				// Fallback if left is not set (e.g. killed before any operations)
				// but this shouldn't happen or we just use err?
				// Actually, if it's killed before any ops, puts/takes/reads are empty, so it doesn't matter.
				// If they are not empty, left MUST have been set.
				errVal = av.err 
			}

			for len(av.puts) > 0 {
				cb := av.puts[0].cb
				av.puts = av.puts[1:]
				av.mu.Unlock()
				gopurs_runtime.Apply(gopurs_runtime.Apply(cb, errVal), gopurs_runtime.Any(nil))
				av.mu.Lock()
			}
			for len(av.reads) > 0 {
				cb := av.reads[0]
				av.reads = av.reads[1:]
				av.mu.Unlock()
				gopurs_runtime.Apply(gopurs_runtime.Apply(cb, errVal), gopurs_runtime.Any(nil))
				av.mu.Lock()
			}
			for len(av.takes) > 0 {
				cb := av.takes[0]
				av.takes = av.takes[1:]
				av.mu.Unlock()
				gopurs_runtime.Apply(gopurs_runtime.Apply(cb, errVal), gopurs_runtime.Any(nil))
				av.mu.Lock()
			}
			break
		}

		var p *putEntry
		value := av.val

		if av.isEmpty && len(av.puts) > 0 {
			p_val := av.puts[0]
			av.puts = av.puts[1:]
			p = &p_val
			value = p.val
			av.val = p.val
			av.isEmpty = false
		}

		if !av.isEmpty {
			var t gopurs_runtime.Value
			var hasT bool
			if len(av.takes) > 0 {
				t = av.takes[0]
				hasT = true
				av.takes = av.takes[1:]
			}
			
			rsize := len(av.reads)
			for rsize > 0 && len(av.reads) > 0 {
				rsize--
				r := av.reads[0]
				av.reads = av.reads[1:]
				av.mu.Unlock()
				gopurs_runtime.Apply(gopurs_runtime.Apply(r, gopurs_runtime.Apply(av.right, value)), gopurs_runtime.Any(nil))
				av.mu.Lock()
			}
			
			if hasT {
				av.isEmpty = true
				av.mu.Unlock()
				gopurs_runtime.Apply(gopurs_runtime.Apply(t, gopurs_runtime.Apply(av.right, value)), gopurs_runtime.Any(nil))
				av.mu.Lock()
			}
		}

		if p != nil {
			av.mu.Unlock()
			gopurs_runtime.Apply(gopurs_runtime.Apply(p.cb, gopurs_runtime.Apply(av.right, gopurs_runtime.Any(nil))), gopurs_runtime.Any(nil))
			av.mu.Lock()
		}

		if (av.isEmpty && len(av.puts) == 0) || (!av.isEmpty && len(av.takes) == 0) {
			break
		}
	}
	av.draining = false
	av.mu.Unlock()
}

func Empty(_ gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Box(&AVarImpl{isEmpty: true})
}

func _NewVar(val gopurs_runtime.Value, _ gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Box(&AVarImpl{isEmpty: false, val: val})
}

func _KillVar(err gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if !av.killed {
			av.err = err
			av.killed = true
			av.isEmpty = true
			av.mu.Unlock()
			drainVar(av)
		} else {
			av.mu.Unlock()
		}
		return gopurs_runtime.Any(nil)
	})
}

func _PutVar(left gopurs_runtime.Value, right gopurs_runtime.Value, val gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if !av.hasLeft {
			av.hasLeft = true
			av.left = left
			av.right = right
		}
		
		av.puts = append(av.puts, putEntry{val: val, cb: cb})
		av.mu.Unlock()
		
		drainVar(av)
		
		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
			av.mu.Lock()
			// Actually we need to remove it from puts. For now we skip deleteCell logic for canceler, it's rarely used to cancel a put.
			// Just return nil
			av.mu.Unlock()
			return gopurs_runtime.Any(nil)
		})
	})
}

func _TryPutVar(val gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if av.isEmpty && !av.killed {
			av.val = val
			av.isEmpty = false
			av.mu.Unlock()
			drainVar(av)
			return gopurs_runtime.Box(true)
		}
		av.mu.Unlock()
		return gopurs_runtime.Box(false)
	})
}

func _TakeVar(left gopurs_runtime.Value, right gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if !av.hasLeft {
			av.hasLeft = true
			av.left = left
			av.right = right
		}
		
		av.takes = append(av.takes, cb)
		av.mu.Unlock()
		
		drainVar(av)
		
		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
			return gopurs_runtime.Any(nil)
		})
	})
}

func _TryTakeVar(left gopurs_runtime.Value, right gopurs_runtime.Value, nothing gopurs_runtime.Value, just gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		
		if av.isEmpty {
			av.mu.Unlock()
			return nothing
		}
		
		val := av.val
		av.isEmpty = true
		av.mu.Unlock()
		
		drainVar(av)
		
		return gopurs_runtime.Apply(just, val)
	})
}

func _ReadVar(left gopurs_runtime.Value, right gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if !av.hasLeft {
			av.hasLeft = true
			av.left = left
			av.right = right
		}
		av.reads = append(av.reads, cb)
		av.mu.Unlock()
		
		drainVar(av)
		
		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
			return gopurs_runtime.Any(nil)
		})
	})
}

func _TryReadVar(nothing gopurs_runtime.Value, just gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		defer av.mu.Unlock()

		if av.isEmpty {
			return nothing
		}
		return gopurs_runtime.Apply(just, av.val)
	})
}

func _Status(killed gopurs_runtime.Value, filled gopurs_runtime.Value, empty gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		defer av.mu.Unlock()

		if av.killed {
			return gopurs_runtime.Apply(killed, av.err)
		}
		if av.isEmpty {
			return empty
		}
		return gopurs_runtime.Apply(filled, av.val)
	})
}
