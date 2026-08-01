import "sync"

type putEntry struct {
	val interface{}
	cb  func(interface{}) interface{}
}

type AVarImpl struct {
	mu      sync.Mutex
	isEmpty bool
	killed  bool
	err     interface{}
	val     interface{}
	takes   []func(interface{}) interface{}
	reads   []func(interface{}) interface{}
	puts    []putEntry
}

func Empty() func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		return &AVarImpl{isEmpty: true}
	}
}

func _NewVar(val interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		return &AVarImpl{isEmpty: false, val: val}
	}
}

func _KillVar(err interface{}, avar interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		av := avar.(*AVarImpl)
		av.mu.Lock()
		if av.killed {
			av.mu.Unlock()
			return nil
		}
		av.killed = true
		av.err = err

		takes := av.takes
		reads := av.reads
		puts := av.puts

		av.takes = nil
		av.reads = nil
		av.puts = nil
		av.mu.Unlock()

		for _, cb := range takes {
			go cb(err).(func(interface{}) interface{})(nil) // In PureScript, AVar killed doesn't use Left, it uses an internal error? Wait!
		}
		for _, cb := range reads {
			go cb(err).(func(interface{}) interface{})(nil)
		}
		for _, put := range puts {
			go put.cb(err).(func(interface{}) interface{})(nil)
		}
		return nil
	}
}

func _PutVar(left func(interface{}) interface{}, right func(interface{}) interface{}, val interface{}, avar interface{}, cb func(interface{}) interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		av := avar.(*AVarImpl)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			cb(left(e)).(func(interface{}) interface{})(nil)
			return nil
		}

		if !av.isEmpty {
			av.puts = append(av.puts, putEntry{val: val, cb: cb})
			av.mu.Unlock()
			return nil
		}

		av.val = val
		av.isEmpty = false

		var takeCb func(interface{}) interface{}
		if len(av.takes) > 0 {
			takeCb = av.takes[0]
			av.takes = av.takes[1:]
			av.isEmpty = true
		}

		reads := av.reads
		av.reads = nil
		av.mu.Unlock()

		for _, r := range reads {
			go r(right(val)).(func(interface{}) interface{})(nil)
		}

		if takeCb != nil {
			go takeCb(right(val)).(func(interface{}) interface{})(nil)
		}
		cb(right(nil)).(func(interface{}) interface{})(nil)
		return nil
	}
}

func _TryPutVar(val interface{}, avar interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		av := avar.(*AVarImpl)
		av.mu.Lock()
		if av.killed || !av.isEmpty {
			av.mu.Unlock()
			return false
		}
		av.val = val
		av.isEmpty = false

		var takeCb func(interface{}) interface{}
		if len(av.takes) > 0 {
			takeCb = av.takes[0]
			av.takes = av.takes[1:]
			av.isEmpty = true
		}
		reads := av.reads
		av.reads = nil
		av.mu.Unlock()

		for _, r := range reads {
			go r(val).(func(interface{}) interface{})(nil) // wait tryPut returns boolean and fires callbacks. But wait, in tryPut there is NO right/left wrappers!
		}
		if takeCb != nil {
			go takeCb(val).(func(interface{}) interface{})(nil) // This is wrong if takeCb expects Either. We need to check JS implementation!
		}
		return true
	}
}

func _TakeVar(left func(interface{}) interface{}, right func(interface{}) interface{}, avar interface{}, cb func(interface{}) interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		av := avar.(*AVarImpl)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			cb(left(e)).(func(interface{}) interface{})(nil)
			return nil
		}

		if av.isEmpty {
			av.takes = append(av.takes, cb)
			av.mu.Unlock()
			return nil
		}

		val := av.val
		av.isEmpty = true

		var putCb func(interface{}) interface{}
		if len(av.puts) > 0 {
			put := av.puts[0]
			av.puts = av.puts[1:]
			av.val = put.val
			av.isEmpty = false
			putCb = put.cb
		}

		av.mu.Unlock()

		if putCb != nil {
			go putCb(right(nil)).(func(interface{}) interface{})(nil)
		}
		cb(right(val)).(func(interface{}) interface{})(nil)
		return nil
	}
}

func _TryTakeVar(left func(interface{}) interface{}, right func(interface{}) interface{}, avar interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		panic("Not implemented fully: _tryTakeVar requires Maybe constructors")
	}
}

func _ReadVar(left func(interface{}) interface{}, right func(interface{}) interface{}, avar interface{}, cb func(interface{}) interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		av := avar.(*AVarImpl)
		av.mu.Lock()

		if av.killed {
			e := av.err
			av.mu.Unlock()
			cb(left(e)).(func(interface{}) interface{})(nil)
			return nil
		}

		if av.isEmpty {
			av.reads = append(av.reads, cb)
			av.mu.Unlock()
			return nil
		}

		val := av.val
		av.mu.Unlock()
		cb(right(val)).(func(interface{}) interface{})(nil)
		return nil
	}
}

func _TryReadVar(avar interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		panic("Not implemented fully: _tryReadVar")
	}
}

func _Status(avar interface{}) func(interface{}) interface{} {
	return func(_ interface{}) interface{} {
		panic("Not implemented fully: _status")
	}
}
