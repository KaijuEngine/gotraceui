package main

import (
	"fmt"
	"slices"
)

type Items[T any] interface {
	Len() int
	At(idx int) T
	AtPtr(idx int) *T
	Slice(start, end int) Items[T]
	// Contiguous reports whether there are no gaps between items.
	Contiguous() bool
	// Subslice reports whether the items are a subslice of a track's items.
	Subslice() bool
	// Container returns the ItemContainer that applies to all items, or false if there is no singular container.
	Container() (ItemContainer, bool)
	ContainerAt(idx int) ItemContainer
}

type SimpleItems[T any] struct {
	items      []T
	container  ItemContainer
	contiguous bool
	subslice   bool
}

func (s SimpleItems[T]) At(idx int) T {
	return s.items[idx]
}

func (s SimpleItems[T]) AtPtr(idx int) *T {
	return &s.items[idx]
}

func (s SimpleItems[T]) Contiguous() bool {
	return s.contiguous
}

func (s SimpleItems[T]) Subslice() bool {
	return s.subslice
}

func (s SimpleItems[T]) Container() (ItemContainer, bool) {
	return s.container, true
}

func (s SimpleItems[T]) ContainerAt(idx int) ItemContainer {
	return s.container
}

func (s SimpleItems[T]) Len() int {
	return len(s.items)
}

func (s SimpleItems[T]) Slice(start int, end int) Items[T] {
	s.items = s.items[start:end]
	return s
}

type MergedItems[T any] struct {
	bases           []Items[T]
	singleContainer ItemContainer
	indices         []int
	start           int
	end             int
}

func MergeItems[T any](items []Items[T], less func(a, b *T) bool) Items[T] {
	if len(items) == 0 {
		return NoItems[T]{}
	} else if len(items) == 1 {
		return items[0]
	}

	var (
		singleContainer    ItemContainer
		hasSingleContainer bool
		first              = true
		bases              = make([]Items[T], 0, len(items))
	)
	for _, ss := range items {
		if ss.Len() == 0 {
			continue
		}
		if first {
			singleContainer, hasSingleContainer = ss.Container()
			first = false
		}

		c, ok := ss.Container()
		if !ok || c != singleContainer {
			hasSingleContainer = false
		}
		if ms, ok := ss.(MergedItems[T]); ok {
			bases = append(bases, ms.bases...)
		} else {
			bases = append(bases, ss)
		}
	}

	var n int
	for _, ss := range bases {
		n += ss.Len()
	}

	if !hasSingleContainer {
		singleContainer = ItemContainer{}
	}

	ms := MergedItems[T]{
		bases:           bases,
		singleContainer: singleContainer,
		start:           0,
		end:             n,
	}
	ms.sort(less)
	return ms
}

func (items *MergedItems[T]) sort(less func(a, b *T) bool) {
	// Each set of items in items.bases is already sorted, so we only need to merge them.
	n := 0
	for _, s := range items.bases {
		n += s.Len()
	}
	items.indices = make([]int, 0, n)
	offsets := make([]int, len(items.bases))

	startOffsets := make([]int, len(items.bases))
	baseLengths := make([]int, len(items.bases))
	for i, b := range items.bases[:len(items.bases)-1] {
		startOffsets[i+1] = startOffsets[i] + b.Len()
	}
	for i, b := range items.bases {
		baseLengths[i] = b.Len()
	}

	for i := 0; i < n; i++ {
		var (
			minBaseIdx int = -1
			minItem    *T
		)
		for j, b := range items.bases {
			if offsets[j] == baseLengths[j] {
				continue
			}
			candidate := b.AtPtr(offsets[j])
			if minBaseIdx == -1 || less(candidate, minItem) {
				minItem = candidate
				minBaseIdx = j
			}
		}

		items.indices = append(items.indices, startOffsets[minBaseIdx]+offsets[minBaseIdx])
		offsets[minBaseIdx]++
	}
}

func (items MergedItems[T]) index(idx int) (int, int) {
	idx += items.start

	if len(items.indices) != 0 {
		idx = items.indices[idx]
	}

	for i, s := range items.bases {
		if s.Len() > idx {
			return i, idx
		} else {
			idx -= s.Len()
		}
	}
	if idx == 0 {
		return len(items.bases) - 1, items.bases[len(items.bases)-1].Len()
	}
	panic(fmt.Sprintf("index %d is out of bounds", idx))
}

func (items MergedItems[T]) At(idx int) T {
	a, b := items.index(idx)
	return items.bases[a].At(b)
}

func (items MergedItems[T]) AtPtr(idx int) *T {
	a, b := items.index(idx)
	return items.bases[a].AtPtr(b)
}

func (items MergedItems[T]) Len() int {
	return items.end - items.start
}

func (items MergedItems[T]) Container() (ItemContainer, bool) {
	if items.singleContainer != (ItemContainer{}) {
		return items.singleContainer, true
	}
	if items.Len() == 1 {
		return items.ContainerAt(0), true
	}
	return ItemContainer{}, false
}

func (items MergedItems[T]) ContainerAt(idx int) ItemContainer {
	a, b := items.index(idx)
	return items.bases[a].ContainerAt(b)
}

func (items MergedItems[T]) Slice(start, end int) Items[T] {
	items.start += start
	items.end = items.start + (end - start)
	return items
}

func (items MergedItems[T]) Contiguous() bool {
	if len(items.bases) == 0 {
		return true
	}
	if len(items.bases) > 1 {
		return false
	}
	return items.bases[0].Contiguous()
}

func (items MergedItems[T]) Subslice() bool {
	if len(items.bases) == 0 {
		return true
	} else if len(items.bases) == 1 {
		return items.bases[0].Subslice()
	} else if items.Len() < 2 {
		return true
	} else {
		return false
	}
}

type NoItems[T any] struct{}

func (NoItems[T]) At(idx int) T {
	panic(fmt.Sprintf("index %d out of bounds", idx))
}

func (NoItems[T]) AtPtr(idx int) *T {
	panic(fmt.Sprintf("index %d out of bounds", idx))
}

func (NoItems[T]) Container() (ItemContainer, bool) {
	return ItemContainer{}, false
}

func (NoItems[T]) ContainerAt(idx int) ItemContainer {
	panic(fmt.Sprintf("index %d out of bounds", idx))
}

func (NoItems[T]) Contiguous() bool {
	return true
}

func (NoItems[T]) Subslice() bool {
	return true
}

func (NoItems[T]) Len() int {
	return 0
}

func (NoItems[T]) Slice(start int, end int) Items[T] {
	if start == 0 && end == 0 {
		return NoItems[T]{}
	} else {
		panic("cannot slice NoItems")
	}
}

type ItemsSubset[T any] struct {
	Base   Items[T]
	Subset []int
}

func (items ItemsSubset[T]) At(idx int) T {
	return items.Base.At(items.Subset[idx])
}

func (items ItemsSubset[T]) AtPtr(idx int) *T {
	return items.Base.AtPtr(items.Subset[idx])
}

func (items ItemsSubset[T]) Len() int {
	return len(items.Subset)
}

func (items ItemsSubset[T]) Slice(start int, end int) Items[T] {
	return ItemsSubset[T]{
		Base:   items.Base,
		Subset: items.Subset[start:end],
	}
}

func (items ItemsSubset[T]) Contiguous() bool {
	return false
}

func (items ItemsSubset[T]) Subslice() bool {
	return false
}

func (items ItemsSubset[T]) Container() (ItemContainer, bool) {
	if items.Len() == 1 {
		return items.ContainerAt(0), true
	} else {
		return items.Base.Container()
	}
}

func (items ItemsSubset[T]) ContainerAt(idx int) ItemContainer {
	return items.Base.ContainerAt(items.Subset[idx])
}

func FilterItems[T any](items Items[T], fn func(item *T) bool) Items[T] {
	var subset []int
	for i := 0; i < items.Len(); i++ {
		if fn(items.AtPtr(i)) {
			subset = append(subset, i)
		}
	}
	if len(subset) == items.Len() {
		return items
	}
	if len(subset) == 0 {
		return NoItems[T]{}
	}

	return ItemsSubset[T]{
		Base:   items,
		Subset: subset,
	}
}

type SortedItems[T any] struct {
	Base  Items[T]
	Order []int
}

func NewSortedItems[T any](items Items[T]) Items[T] {
	order := make([]int, items.Len())
	for i := range order {
		order[i] = i
	}
	return SortedItems[T]{
		Base:  items,
		Order: order,
	}
}

func (s *SortedItems[T]) Reset(items Items[T]) {
	s.Base = items
	if cap(s.Order) >= items.Len() {
		s.Order = s.Order[:items.Len()]
	} else {
		s.Order = make([]int, items.Len())
	}
	for i := range s.Order {
		s.Order[i] = i
	}
}

func (s SortedItems[T]) At(idx int) T {
	return s.Base.At(s.Order[idx])
}

func (s SortedItems[T]) AtPtr(idx int) *T {
	return s.Base.AtPtr(s.Order[idx])
}

func (s SortedItems[T]) Len() int {
	return len(s.Order)
}

func (s SortedItems[T]) Map(idx int) int {
	return s.Order[idx]
}

func (s SortedItems[T]) Sort(cmp func(a, b T) int) {
	slices.SortFunc(s.Order, func(a, b int) int {
		ea := s.Base.At(a)
		eb := s.Base.At(b)
		return cmp(ea, eb)
	})
}

func (s SortedItems[T]) SortIndex(cmp func(a, b int) int) {
	slices.SortFunc(s.Order, cmp)
}

func (items SortedItems[T]) Container() (ItemContainer, bool) {
	if items.Len() == 1 {
		return items.ContainerAt(0), true
	} else {
		return items.Base.Container()
	}
}

func (items SortedItems[T]) ContainerAt(idx int) ItemContainer {
	return items.Base.ContainerAt(items.Map(idx))
}

func (items SortedItems[T]) Contiguous() bool {
	// Generally not contiguous because the items might be sorted.
	return false
}

func (items SortedItems[T]) Slice(start, end int) Items[T] {
	return SortedItems[T]{
		Base:  items.Base,
		Order: items.Order[start:end],
	}
}

func (items SortedItems[T]) Subslice() bool {
	// Generally not a subslice because the items might be sorted.
	return false
}
