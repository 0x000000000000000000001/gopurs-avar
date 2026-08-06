import (
	"fmt"
	"sync"
	"gopurs/output/gopurs_runtime"
)

type putEntry struct {
	val gopurs_runtime.Value
	cb  gopurs_runtime.Value
}

func unboxAVar(avar gopurs_runtime.Value) *AVarImpl {
	val := avar.AnyVal()
	if av, ok := val.(*AVarImpl); ok {
		return av
	}
	panic(fmt.Sprintf("AVar AnyVal is not AVarImpl: %T (value: %#v)", val, val))
}

type AVarImpl struct {
	mu      sync.Mutex
	isEmpty bool
	killed  bool
	err     gopurs_runtime.Value
	val     gopurs_runtime.Value
	takes   []gopurs_runtime.Value
	reads   []gopurs_runtime.Value
	puts    []putEntry
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
		if av.killed {
			av.mu.Unlock()
			return gopurs_runtime.Any(nil)
		}
		av.killed = true
		av.err = err
		av.isEmpty = true

		takes := av.takes
		reads := av.reads
		puts := av.puts

		av.takes = nil
		av.reads = nil
		av.puts = nil
		av.mu.Unlock()

		for _, cb := range takes {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, err), gopurs_runtime.Any(nil))
		}
		for _, cb := range reads {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, err), gopurs_runtime.Any(nil))
		}
		for _, put := range puts {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(put.cb, err), gopurs_runtime.Any(nil))
		}
		return gopurs_runtime.Any(nil)
	})
}

func _PutVar(left gopurs_runtime.Value, right gopurs_runtime.Value, val gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(left, e)), gopurs_runtime.Any(nil))
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		if !av.isEmpty {
			av.puts = append(av.puts, putEntry{val: val, cb: cb})
			av.mu.Unlock()
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		av.val = val
		av.isEmpty = false

		var takeCb gopurs_runtime.Value
		var hasTakeCb bool
		if len(av.takes) > 0 {
			takeCb = av.takes[0]
			hasTakeCb = true
			av.takes = av.takes[1:]
			av.isEmpty = true
		}

		reads := av.reads
		av.reads = nil
		av.mu.Unlock()

		for _, r := range reads {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(r, gopurs_runtime.Apply(right, val)), gopurs_runtime.Any(nil))
		}

		if hasTakeCb {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(takeCb, gopurs_runtime.Apply(right, val)), gopurs_runtime.Any(nil))
		}
		
		go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(right, gopurs_runtime.Any(nil))), gopurs_runtime.Any(nil))

		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
	})
}

func _TryPutVar(val gopurs_runtime.Value, avar gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()
		if av.killed || !av.isEmpty {
			av.mu.Unlock()
			return gopurs_runtime.Box(false)
		}
		av.val = val
		av.isEmpty = false

		var takeCb gopurs_runtime.Value
		var hasTakeCb bool
		if len(av.takes) > 0 {
			takeCb = av.takes[0]
			hasTakeCb = true
			av.takes = av.takes[1:]
			av.isEmpty = true
		}
		reads := av.reads
		av.reads = nil
		av.mu.Unlock()

		for _, r := range reads {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(r, val), gopurs_runtime.Any(nil))
		}
		if hasTakeCb {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(takeCb, val), gopurs_runtime.Any(nil))
		}
		return gopurs_runtime.Box(true)
	})
}

func _TakeVar(left gopurs_runtime.Value, right gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(left, e)), gopurs_runtime.Any(nil))
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		if av.isEmpty {
			av.takes = append(av.takes, cb)
			av.mu.Unlock()
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		val := av.val
		av.isEmpty = true

		var putCb gopurs_runtime.Value
		var hasPutCb bool
		if len(av.puts) > 0 {
			put := av.puts[0]
			av.puts = av.puts[1:]
			av.val = put.val
			av.isEmpty = false
			putCb = put.cb
			hasPutCb = true
		}

		av.mu.Unlock()

		if hasPutCb {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(putCb, gopurs_runtime.Apply(right, gopurs_runtime.Any(nil))), gopurs_runtime.Any(nil))
		}
		
		go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(right, val)), gopurs_runtime.Any(nil))

		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
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

		var putCb gopurs_runtime.Value
		var hasPutCb bool
		if len(av.puts) > 0 {
			put := av.puts[0]
			av.puts = av.puts[1:]
			av.val = put.val
			av.isEmpty = false
			putCb = put.cb
			hasPutCb = true
		}

		av.mu.Unlock()

		if hasPutCb {
			go gopurs_runtime.Apply(gopurs_runtime.Apply(putCb, gopurs_runtime.Apply(right, gopurs_runtime.Any(nil))), gopurs_runtime.Any(nil))
		}

		return gopurs_runtime.Apply(just, val)
	})
}

func _ReadVar(left gopurs_runtime.Value, right gopurs_runtime.Value, avar gopurs_runtime.Value, cb gopurs_runtime.Value) gopurs_runtime.Value {
	return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value {
		av := unboxAVar(avar)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(left, e)), gopurs_runtime.Any(nil))
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		if av.isEmpty {
			av.reads = append(av.reads, cb)
			av.mu.Unlock()
			return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
		}

		val := av.val
		av.mu.Unlock()
		
		go gopurs_runtime.Apply(gopurs_runtime.Apply(cb, gopurs_runtime.Apply(right, val)), gopurs_runtime.Any(nil))

		return gopurs_runtime.Func(func(_ gopurs_runtime.Value) gopurs_runtime.Value { return gopurs_runtime.Any(nil) })
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
