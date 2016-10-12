package roaring

import (
	"encoding/binary"
	"io"
)

type container interface {
	clone() container
	and(container) container
	iand(container) container // i stands for inplace
	andNot(container) container
	iandNot(container) container // i stands for inplace
	getCardinality() int
	rank(uint16) int
	add(uint16) container
	//addRange(start, final int) container  // range is [firstOfRange,lastOfRange) (unused)
	iaddRange(start, final int) container // i stands for inplace, range is [firstOfRange,lastOfRange)
	remove(uint16) container
	not(start, final int) container               // range is [firstOfRange,lastOfRange)
	inot(firstOfRange, lastOfRange int) container // i stands for inplace, range is [firstOfRange,lastOfRange)
	xor(r container) container
	getShortIterator() shortIterable
	contains(i uint16) bool
	equals(i interface{}) bool
	fillLeastSignificant16bits(array []uint32, i int, mask uint32)
	or(r container) container
	ior(r container) container   // i stands for inplace
	intersects(r container) bool // whether the two containers intersect
	lazyOR(r container) container
	lazyIOR(r container) container
	getSizeInBytes() int
	//removeRange(start, final int) container  // range is [firstOfRange,lastOfRange) (unused)
	iremoveRange(start, final int) container // i stands for inplace, range is [firstOfRange,lastOfRange)
	selectInt(uint16) int
	serializedSizeInBytes() int
	readFrom(io.Reader) (int, error)
	writeTo(io.Writer) (int, error)
}

// careful: range is [firstOfRange,lastOfRange]
func rangeOfOnes(start, last int) container {
	if (last - start + 1) > arrayDefaultMaxSize {
		return newBitmapContainerwithRange(start, last)
	}

	return newArrayContainerRange(start, last)
}

type roaringArray struct {
	keys            []uint16
	containers      []container
	needCopyOnWrite []bool
	copyOnWrite     bool
}

func newRoaringArray() *roaringArray {
	ra := &roaringArray{}
	ra.clear()
	ra.copyOnWrite = false
	return ra
}

func (ra *roaringArray) appendContainer(key uint16, value container, mustCopyOnWrite bool) {
	ra.keys = append(ra.keys, key)
	ra.containers = append(ra.containers, value)
	ra.needCopyOnWrite = append(ra.needCopyOnWrite, mustCopyOnWrite)
}

func (ra *roaringArray) appendWithoutCopy(sa roaringArray, startingindex int) {
	ra.appendContainer(sa.keys[startingindex], sa.containers[startingindex], false)
}

func (ra *roaringArray) appendCopy(sa roaringArray, startingindex int) {
	ra.appendContainer(sa.keys[startingindex], sa.containers[startingindex], true)
	sa.setNeedsCopyOnWrite(startingindex)
}

func (ra *roaringArray) appendWithoutCopyMany(sa roaringArray, startingindex, end int) {
	for i := startingindex; i < end; i++ {
		ra.appendWithoutCopy(sa, i)
	}
}

func (ra *roaringArray) appendCopyMany(sa roaringArray, startingindex, end int) {
	for i := startingindex; i < end; i++ {
		ra.appendCopy(sa, i)
	}
}

func (ra *roaringArray) appendCopiesUntil(sa roaringArray, stoppingKey uint16) {
	for i := 0; i < sa.size(); i++ {
		if sa.keys[i] >= stoppingKey {
			break
		}
		ra.appendContainer(sa.keys[i], sa.containers[i], true)
		sa.setNeedsCopyOnWrite(i)
	}
}

func (ra *roaringArray) appendCopiesAfter(sa roaringArray, beforeStart uint16) {
	startLocation := sa.getIndex(beforeStart)
	if startLocation >= 0 {
		startLocation++
	} else {
		startLocation = -startLocation - 1
	}

	for i := startLocation; i < sa.size(); i++ {
		ra.appendContainer(sa.keys[i], sa.containers[i], true)
		sa.setNeedsCopyOnWrite(i)
	}
}

func (ra *roaringArray) removeIndexRange(begin, end int) {
	if end <= begin {
		return
	}

	r := end - begin

	copy(ra.keys[begin:], ra.keys[end:])
	copy(ra.containers[begin:], ra.containers[end:])
	copy(ra.needCopyOnWrite[begin:], ra.needCopyOnWrite[end:])

	ra.resize(len(ra.keys) - r)
}

func (ra *roaringArray) resize(newsize int) {
	for k := newsize; k < len(ra.containers); k++ {
		ra.containers[k] = nil
	}

	ra.keys = ra.keys[:newsize]
	ra.containers = ra.containers[:newsize]
	ra.needCopyOnWrite = ra.needCopyOnWrite[:newsize]
}

func (ra *roaringArray) clear() {
	ra.keys = make([]uint16, 0)
	ra.containers = make([]container, 0)
	ra.needCopyOnWrite = make([]bool, 0)
}

func (ra *roaringArray) clone() *roaringArray {
	sa := new(roaringArray)
	sa.keys = make([]uint16, len(ra.keys))
	sa.containers = make([]container, len(ra.containers))
	sa.needCopyOnWrite = make([]bool, len(ra.needCopyOnWrite))
	sa.copyOnWrite = ra.copyOnWrite
	copy(sa.keys, ra.keys)
	if sa.copyOnWrite {
		copy(sa.keys, ra.keys)
		copy(sa.containers, ra.containers)
		sa.markAllAsNeedingCopyOnWrite()
		ra.markAllAsNeedingCopyOnWrite()
	} else {
		for i := range sa.needCopyOnWrite {
			sa.needCopyOnWrite[i] = false
		}
		for i := range sa.containers {
			sa.containers[i] = ra.containers[i].clone()
		}
	}
	return sa
}

func (ra *roaringArray) containsKey(x uint16) bool {
	return (ra.binarySearch(0, len(ra.keys), x) >= 0)
}

func (ra *roaringArray) getContainer(x uint16) container {
	i := ra.binarySearch(0, len(ra.keys), x)
	if i < 0 {
		return nil
	}
	return ra.containers[i]
}

func (ra *roaringArray) getWritableContainerContainer(x uint16) container {
	i := ra.binarySearch(0, len(ra.keys), x)
	if i < 0 {
		return nil
	}
	if ra.needCopyOnWrite[i] {
		ra.containers[i] = ra.containers[i].clone()
		ra.needCopyOnWrite[i] = false
	}
	return ra.containers[i]
}

func (ra *roaringArray) getContainerAtIndex(i int) container {
	return ra.containers[i]
}

func (ra *roaringArray) getWritableContainerAtIndex(i int) container {
	if ra.needCopyOnWrite[i] {
		ra.containers[i] = ra.containers[i].clone()
		ra.needCopyOnWrite[i] = false
	}
	return ra.containers[i]
}

func (ra *roaringArray) getIndex(x uint16) int {
	// before the binary search, we optimize for frequent cases
	size := len(ra.keys)
	if (size == 0) || (ra.keys[size-1] == x) {
		return size - 1
	}
	return ra.binarySearch(0, size, x)
}

func (ra *roaringArray) getKeyAtIndex(i int) uint16 {
	return ra.keys[i]
}

func (ra *roaringArray) insertNewKeyValueAt(i int, key uint16, value container) {
	ra.keys = append(ra.keys, 0)
	ra.containers = append(ra.containers, nil)

	copy(ra.keys[i+1:], ra.keys[i:])
	copy(ra.containers[i+1:], ra.containers[i:])

	ra.keys[i] = key
	ra.containers[i] = value

	ra.needCopyOnWrite = append(ra.needCopyOnWrite, false)
	copy(ra.needCopyOnWrite[i+1:], ra.needCopyOnWrite[i:])
	ra.needCopyOnWrite[i] = false
}

func (ra *roaringArray) remove(key uint16) bool {
	i := ra.binarySearch(0, len(ra.keys), key)
	if i >= 0 { // if a new key
		ra.removeAtIndex(i)
		return true
	}
	return false
}

func (ra *roaringArray) removeAtIndex(i int) {
	copy(ra.keys[i:], ra.keys[i+1:])
	copy(ra.containers[i:], ra.containers[i+1:])

	copy(ra.needCopyOnWrite[i:], ra.needCopyOnWrite[i+1:])

	ra.resize(len(ra.keys) - 1)
}

func (ra *roaringArray) setContainerAtIndex(i int, c container) {
	ra.containers[i] = c
}

func (ra *roaringArray) replaceKeyAndContainerAtIndex(i int, key uint16, c container, mustCopyOnWrite bool) {
	ra.keys[i] = key
	ra.containers[i] = c
	ra.needCopyOnWrite[i] = mustCopyOnWrite
}

func (ra *roaringArray) size() int {
	return len(ra.keys)
}

func (ra *roaringArray) binarySearch(begin, end int, ikey uint16) int {
	low := begin
	high := end - 1
	for low+16 <= high {
		middleIndex := int(uint((low + high)) >> 1)
		middleValue := ra.keys[middleIndex]

		if middleValue < ikey {
			low = middleIndex + 1
		} else if middleValue > ikey {
			high = middleIndex - 1
		} else {
			return middleIndex
		}
	}
	for ; low <= high; low++ {
		val := ra.keys[low]
		if val >= ikey {
			if val == ikey {
				return low
			}
			break
		}
	}
	return -(low + 1)
}

func (ra *roaringArray) equals(o interface{}) bool {
	srb, ok := o.(roaringArray)
	if ok {

		if srb.size() != ra.size() {
			return false
		}
		for i, k := range ra.keys {
			if k != srb.keys[i] {
				return false
			}
		}

		for i, c := range ra.containers {
			if !c.equals(srb.containers[i]) {
				return false
			}
		}
		return true
	}
	return false
}

func (ra *roaringArray) serializedSizeInBytes() uint64 {
	count := uint64(4 + 4)
	for _, c := range ra.containers {
		count = count + 4 + 4
		count = count + uint64(c.serializedSizeInBytes())
	}
	return count
}

func (ra *roaringArray) writeTo(stream io.Writer) (int64, error) {
	preambleSize := 4 + 4 + 4*len(ra.keys)
	buf := make([]byte, preambleSize+4*len(ra.keys))
	binary.LittleEndian.PutUint32(buf[0:], uint32(serialCookie))
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(ra.keys)))

	for i, key := range ra.keys {
		off := 8 + i*4
		binary.LittleEndian.PutUint16(buf[off:], uint16(key))

		c := ra.containers[i]
		binary.LittleEndian.PutUint16(buf[off+2:], uint16(c.getCardinality()-1))
	}

	startOffset := int64(preambleSize + 4*len(ra.keys))
	for i, c := range ra.containers {
		binary.LittleEndian.PutUint32(buf[preambleSize+i*4:], uint32(startOffset))
		startOffset += int64(getSizeInBytesFromCardinality(c.getCardinality()))
	}

	_, err := stream.Write(buf)
	if err != nil {
		return 0, err
	}

	for _, c := range ra.containers {
		_, err := c.writeTo(stream)
		if err != nil {
			return 0, err
		}
	}
	return startOffset, nil
}

func (ra *roaringArray) readFrom(stream io.Reader) (int64, error) {
	var cookie uint32
	err := binary.Read(stream, binary.LittleEndian, &cookie)
	if err != nil {
		return 0, err
	}
	if cookie != serialCookie {
		return 0, err
	}
	var size uint32
	err = binary.Read(stream, binary.LittleEndian, &size)
	if err != nil {
		return 0, err
	}
	keycard := make([]uint16, 2*size, 2*size)
	err = binary.Read(stream, binary.LittleEndian, keycard)
	if err != nil {
		return 0, err
	}
	offsets := make([]uint32, size, size)
	err = binary.Read(stream, binary.LittleEndian, offsets)
	if err != nil {
		return 0, err
	}
	offset := int64(4 + 4 + 8*size)
	for i := uint32(0); i < size; i++ {
		c := int(keycard[2*i+1]) + 1
		offset += int64(getSizeInBytesFromCardinality(c))
		if c > arrayDefaultMaxSize {
			nb := newBitmapContainer()
			nb.readFrom(stream)
			nb.cardinality = int(c)
			ra.appendContainer(keycard[2*i], nb, false)
		} else {
			nb := newArrayContainerSize(int(c))
			nb.readFrom(stream)
			ra.appendContainer(keycard[2*i], nb, false)
		}
	}
	return offset, nil
}

func (ra *roaringArray) advanceUntil(min uint16, pos int) int {
	lower := pos + 1

	if lower >= len(ra.keys) || ra.keys[lower] >= min {
		return lower
	}

	spansize := 1

	for lower+spansize < len(ra.keys) && ra.keys[lower+spansize] < min {
		spansize *= 2
	}
	var upper int
	if lower+spansize < len(ra.keys) {
		upper = lower + spansize
	} else {
		upper = len(ra.keys) - 1
	}

	if ra.keys[upper] == min {
		return upper
	}

	if ra.keys[upper] < min {
		// means
		// array
		// has no
		// item
		// >= min
		// pos = array.length;
		return len(ra.keys)
	}

	// we know that the next-smallest span was too small
	lower += (spansize / 2)

	mid := 0
	for lower+1 != upper {
		mid = (lower + upper) / 2
		if ra.keys[mid] == min {
			return mid
		} else if ra.keys[mid] < min {
			lower = mid
		} else {
			upper = mid
		}
	}
	return upper
}

func (ra *roaringArray) markAllAsNeedingCopyOnWrite() {
	needCopyOnWrite := make([]bool, len(ra.keys))
	for i := range needCopyOnWrite {
		needCopyOnWrite[i] = true
	}
	ra.needCopyOnWrite = needCopyOnWrite
}

func (ra *roaringArray) needsCopyOnWrite(i int) bool {
	return ra.needCopyOnWrite[i]
}

func (ra *roaringArray) setNeedsCopyOnWrite(i int) {
	ra.needCopyOnWrite[i] = true
}
