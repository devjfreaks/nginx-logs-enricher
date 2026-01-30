package dedupe

import "container/list"

type LRUSet struct {
	capacity int
	ll       *list.List
	index    map[string]*list.Element
}

type entry struct {
	key string
}

func NewLRUSet(capacity int) *LRUSet {
	if capacity <= 0 {
		capacity = 1
	}
	return &LRUSet{
		capacity: capacity,
		ll:       list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

func (s *LRUSet) Seen(key string) bool {
	if el, ok := s.index[key]; ok {
		s.ll.MoveToFront(el)
		return true
	}

	el := s.ll.PushFront(entry{key: key})
	s.index[key] = el

	if s.ll.Len() > s.capacity {
		back := s.ll.Back()
		if back != nil {
			s.ll.Remove(back)
			e := back.Value.(entry)
			delete(s.index, e.key)
		}
	}
	return false
}
