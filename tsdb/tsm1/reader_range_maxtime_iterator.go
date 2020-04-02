package tsm1

import (
	"math"
)

const (
	// InvalidMinNanoTime is an invalid nano timestamp that has an ordinal
	// value lower than models.MinNanoTime.
	InvalidMinNanoTime = math.MinInt64
)

// TimeRangeMaxTimeIterator will iterate over the keys of a TSM file, starting at
// the provided key. It is used to determine if each key has data which exists
// within a specified time interval.
type TimeRangeMaxTimeIterator struct {
	timeRangeBlockReader

	// cached values
	lastWrite int64
	hasData   bool
	isLoaded  bool
}

// Next advances the iterator and reports if it is still valid.
func (b *TimeRangeMaxTimeIterator) Next() bool {
	if b.Err() != nil {
		return false
	}

	b.clearIsLoaded()

	return b.iter.Next()
}

// Seek points the iterator at the smallest key greater than or equal to the
// given key, returning true if it was an exact match. It returns false for
// ok if the key does not exist.
func (b *TimeRangeMaxTimeIterator) Seek(key []byte) (exact, ok bool) {
	if b.Err() != nil {
		return false, false
	}

	b.clearIsLoaded()

	return b.iter.Seek(key)
}

// HasData reports true if the current key has data for the time range.
func (b *TimeRangeMaxTimeIterator) HasData() bool {
	if b.Err() != nil {
		return false
	}

	b.load()

	return b.hasData
}

// MaxTime returns the maximum timestamp for the current key within the
// requested time rage. If an error occurred or there is no data,
// InvalidMinTimeStamp will be returned, which is less than models.MinTimeStamp.
// This property can be leveraged when enumerating keys to find the maximum timestamp,
// as this value will always be lower than any valid timestamp returned.
func (b *TimeRangeMaxTimeIterator) MaxTime() int64 {
	if b.Err() != nil {
		return InvalidMinNanoTime
	}

	b.load()

	return b.lastWrite
}

func (b *TimeRangeMaxTimeIterator) clearIsLoaded() { b.isLoaded = false }

func (b *TimeRangeMaxTimeIterator) load() {
	if b.isLoaded {
		return
	}

	b.isLoaded = true
	b.hasData = false
	b.lastWrite = InvalidMinNanoTime

	e, ts := b.getEntriesAndTombstones()
	if len(e) == 0 {
		return
	}

	if len(ts) == 0 {
		b.lastWrite = e[len(e)-1].MaxTime
		// no tombstones, fast path will avoid decoding blocks
		// if queried time interval intersects with one of the entries
		if intersectsEntry(e, b.tr) {
			b.hasData = true
			return
		}

		for i := range e {
			if !b.readBlock(&e[i]) {
				goto ERROR
			}

			if b.a.Contains(b.tr.Min, b.tr.Max) {
				b.hasData = true
				return
			}
		}
	} else {
		var i int
		for i = range e {
			if !b.readBlock(&e[i]) {
				goto ERROR
			}

			// remove tombstoned timestamps
			for i := range ts {
				b.a.Exclude(ts[i].Min, ts[i].Max)
			}

			if b.a.Len() > 0 {
				b.lastWrite = b.a.MaxTime()

				if b.a.Contains(b.tr.Min, b.tr.Max) {
					b.hasData = true
					break
				}
			}
		}

		// search the remaining entries to find the last write timestamp
		for j := len(e) - 1; j > i; j-- {
			if !b.readBlock(&e[i]) {
				goto ERROR
			}

			// remove tombstoned timestamps
			for i := range ts {
				b.a.Exclude(ts[i].Min, ts[i].Max)
			}

			if b.a.Len() > 0 {
				b.lastWrite = b.a.MaxTime()
				break
			}
		}
	}

	return
ERROR:
	// ERROR ensures cached state is set to invalid values
	b.hasData = false
	b.lastWrite = InvalidMinNanoTime
}
