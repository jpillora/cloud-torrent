package roaring

import (
	"unsafe"
)

type arrayContainer struct {
	content []uint16
}

func (ac *arrayContainer) fillLeastSignificant16bits(x []uint32, i int, mask uint32) {
	for k := 0; k < len(ac.content); k++ {
		x[k+i] = uint32(ac.content[k]) | mask
	}
}

func (ac *arrayContainer) getShortIterator() shortIterable {
	return &shortIterator{ac.content, 0}
}

func (ac *arrayContainer) getSizeInBytes() int {
	// unsafe.Sizeof calculates the memory used by the top level of the slice
	// descriptor - not including the size of the memory referenced by the slice.
	// http://golang.org/pkg/unsafe/#Sizeof
	return ac.getCardinality()*2 + int(unsafe.Sizeof(ac.content))
}

func (ac *arrayContainer) serializedSizeInBytes() int {
	// based on https://golang.org/src/pkg/encoding/binary/binary.go#265
	// there is no serialization overhead for writing an array of fixed size vals
	return ac.getCardinality() * 2
}

// add the values in the range [firstOfRange,lastofRange)
func (ac *arrayContainer) addRange(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac.clone()
	}
	indexstart := binarySearch(ac.content, uint16(firstOfRange))
	if indexstart < 0 {
		indexstart = -indexstart - 1
	}
	indexend := binarySearch(ac.content, uint16(lastOfRange-1))
	if indexend < 0 {
		indexend = -indexend - 1
	} else {
		indexend++
	}
	rangelength := lastOfRange - firstOfRange

	newcardinality := indexstart + (ac.getCardinality() - indexend) + rangelength
	if newcardinality > arrayDefaultMaxSize {
		a := ac.toBitmapContainer()
		return a.iaddRange(firstOfRange, lastOfRange)
	}
	answer := &arrayContainer{make([]uint16, newcardinality)}
	copy(answer.content[:indexstart], ac.content[:indexstart])
	copy(answer.content[indexstart+rangelength:], ac.content[indexend:])
	for k := 0; k < rangelength; k++ {
		answer.content[k+indexstart] = uint16(firstOfRange + k)
	}
	return answer
}

// remove the values in the range [firstOfRange,lastofRange)
func (ac *arrayContainer) removeRange(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac.clone()
	}
	indexstart := binarySearch(ac.content, uint16(firstOfRange))
	if indexstart < 0 {
		indexstart = -indexstart - 1
	}
	indexend := binarySearch(ac.content, uint16(lastOfRange-1))
	if indexend < 0 {
		indexend = -indexend - 1
	} else {
		indexend++
	}
	rangelength := indexend - indexstart
	answer := &arrayContainer{make([]uint16, ac.getCardinality()-rangelength)}
	copy(answer.content[:indexstart], ac.content[:indexstart])
	copy(answer.content[indexstart:], ac.content[indexstart+rangelength:])
	return answer
}

// add the values in the range [firstOfRange,lastofRange)
func (ac *arrayContainer) iaddRange(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac
	}
	indexstart := binarySearch(ac.content, uint16(firstOfRange))
	if indexstart < 0 {
		indexstart = -indexstart - 1
	}
	indexend := binarySearch(ac.content, uint16(lastOfRange-1))
	if indexend < 0 {
		indexend = -indexend - 1
	} else {
		indexend++
	}
	rangelength := lastOfRange - firstOfRange
	newcardinality := indexstart + (ac.getCardinality() - indexend) + rangelength
	if newcardinality > arrayDefaultMaxSize {
		a := ac.toBitmapContainer()
		return a.iaddRange(firstOfRange, lastOfRange)
	}
	if cap(ac.content) < newcardinality {
		tmp := make([]uint16, newcardinality, newcardinality)
		copy(tmp[:indexstart], ac.content[:indexstart])
		copy(tmp[indexstart+rangelength:], ac.content[indexend:])

		ac.content = tmp
	} else {
		ac.content = ac.content[:newcardinality]
		copy(ac.content[indexstart+rangelength:], ac.content[indexend:])

	}
	for k := 0; k < rangelength; k++ {
		ac.content[k+indexstart] = uint16(firstOfRange + k)
	}
	return ac
}

// remove the values in the range [firstOfRange,lastOfRange)
func (ac *arrayContainer) iremoveRange(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac
	}
	indexstart := binarySearch(ac.content, uint16(firstOfRange))
	if indexstart < 0 {
		indexstart = -indexstart - 1
	}
	indexend := binarySearch(ac.content, uint16(lastOfRange-1))
	if indexend < 0 {
		indexend = -indexend - 1
	} else {
		indexend++
	}
	rangelength := indexend - indexstart
	answer := ac
	copy(answer.content[indexstart:], ac.content[indexstart+rangelength:])
	answer.content = answer.content[:ac.getCardinality()-rangelength]
	return answer
}

// flip the values in the range [firstOfRange,lastOfRange)
func (ac *arrayContainer) not(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac.clone()
	}
	return ac.notClose(firstOfRange, lastOfRange-1) // remove everything in [firstOfRange,lastOfRange-1]
}

// flip the values in the range [firstOfRange,lastOfRange]
func (ac *arrayContainer) notClose(firstOfRange, lastOfRange int) container {
	if firstOfRange > lastOfRange { // unlike add and remove, not uses an inclusive range [firstOfRange,lastOfRange]
		return ac.clone()
	}

	// determine the span of array indices to be affected^M
	startIndex := binarySearch(ac.content, uint16(firstOfRange))
	if startIndex < 0 {
		startIndex = -startIndex - 1
	}
	lastIndex := binarySearch(ac.content, uint16(lastOfRange))
	if lastIndex < 0 {
		lastIndex = -lastIndex - 2
	}
	currentValuesInRange := lastIndex - startIndex + 1
	spanToBeFlipped := lastOfRange - firstOfRange + 1
	newValuesInRange := spanToBeFlipped - currentValuesInRange
	cardinalityChange := newValuesInRange - currentValuesInRange
	newCardinality := len(ac.content) + cardinalityChange

	if newCardinality > arrayDefaultMaxSize {
		return ac.toBitmapContainer().not(firstOfRange, lastOfRange+1)
	}
	answer := newArrayContainer()
	answer.content = make([]uint16, newCardinality, newCardinality) //a hack for sure

	copy(answer.content, ac.content[:startIndex])
	outPos := startIndex
	inPos := startIndex
	valInRange := firstOfRange
	for ; valInRange <= lastOfRange && inPos <= lastIndex; valInRange++ {
		if uint16(valInRange) != ac.content[inPos] {
			answer.content[outPos] = uint16(valInRange)
			outPos++
		} else {
			inPos++
		}
	}

	for ; valInRange <= lastOfRange; valInRange++ {
		answer.content[outPos] = uint16(valInRange)
		outPos++
	}

	for i := lastIndex + 1; i < len(ac.content); i++ {
		answer.content[outPos] = ac.content[i]
		outPos++
	}
	answer.content = answer.content[:newCardinality]
	return answer

}

func (ac *arrayContainer) equals(o interface{}) bool {
	srb, ok := o.(*arrayContainer)
	if ok {
		// Check if the containers are the same object.
		if ac == srb {
			return true
		}

		if len(srb.content) != len(ac.content) {
			return false
		}

		for i, v := range ac.content {
			if v != srb.content[i] {
				return false
			}
		}
		return true
	}
	return false
}

func (ac *arrayContainer) toBitmapContainer() *bitmapContainer {
	bc := newBitmapContainer()
	bc.loadData(ac)
	return bc

}
func (ac *arrayContainer) add(x uint16) container {
	// Special case adding to the end of the container.
	l := len(ac.content)
	if l > 0 && l < arrayDefaultMaxSize && ac.content[l-1] < x {
		ac.content = append(ac.content, x)
		return ac
	}

	loc := binarySearch(ac.content, x)

	if loc < 0 {
		if len(ac.content) >= arrayDefaultMaxSize {
			a := ac.toBitmapContainer()
			a.add(x)
			return a
		}
		s := ac.content
		i := -loc - 1
		s = append(s, 0)
		copy(s[i+1:], s[i:])
		s[i] = x
		ac.content = s
	}
	return ac
}

func (ac *arrayContainer) remove(x uint16) container {
	loc := binarySearch(ac.content, x)
	if loc >= 0 {
		s := ac.content
		s = append(s[:loc], s[loc+1:]...)
		ac.content = s
	}
	return ac
}

func (ac *arrayContainer) or(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.orArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.or(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) ior(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.orArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.ior(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) lazyIOR(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.orArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.lazyOR(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) lazyOR(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.orArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.lazyOR(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) orArray(value2 *arrayContainer) container {
	value1 := ac
	maxPossibleCardinality := value1.getCardinality() + value2.getCardinality()
	if maxPossibleCardinality > arrayDefaultMaxSize { // it could be a bitmap!^M
		bc := newBitmapContainer()
		for k := 0; k < len(value2.content); k++ {
			v := value2.content[k]
			i := uint(v) >> 6
			mask := uint64(1) << (v % 64)
			bc.bitmap[i] |= mask
		}
		for k := 0; k < len(ac.content); k++ {
			v := ac.content[k]
			i := uint(v) >> 6
			mask := uint64(1) << (v % 64)
			bc.bitmap[i] |= mask
		}
		bc.cardinality = int(popcntSlice(bc.bitmap))
		if bc.cardinality <= arrayDefaultMaxSize {
			return bc.toArrayContainer()
		}
		return bc
	}
	answer := newArrayContainerCapacity(maxPossibleCardinality)
	nl := union2by2(value1.content, value2.content, answer.content)
	answer.content = answer.content[:nl] // reslice to match actual used capacity
	return answer
}

func (ac *arrayContainer) and(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.andArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.and(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) intersects(a container) bool {
	switch a.(type) {
	case *arrayContainer:
		return ac.intersectsArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.intersects(ac)
	}
	return false // should not happen
}

func (ac *arrayContainer) iand(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.iandArray(a.(*arrayContainer))
	case *bitmapContainer:
		return ac.iandBitmap(a.(*bitmapContainer))
	}
	panic("should never happen")
}

func (ac *arrayContainer) iandBitmap(bc *bitmapContainer) *arrayContainer {
	pos := 0
	c := ac.getCardinality()
	for k := 0; k < c; k++ {
		if bc.contains(ac.content[k]) {
			ac.content[pos] = ac.content[k]
			pos++
		}
	}
	ac.content = ac.content[:pos]
	return ac

}

func (ac *arrayContainer) xor(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.xorArray(a.(*arrayContainer))
	case *bitmapContainer:
		return a.xor(ac)
	}
	panic("should never happen")
}

func (ac *arrayContainer) xorArray(value2 *arrayContainer) container {
	value1 := ac
	totalCardinality := value1.getCardinality() + value2.getCardinality()
	if totalCardinality > arrayDefaultMaxSize { // it could be a bitmap!
		bc := newBitmapContainer()
		for k := 0; k < len(value2.content); k++ {
			v := value2.content[k]
			i := uint(v) >> 6
			bc.bitmap[i] ^= (uint64(1) << (v % 64))
		}
		for k := 0; k < len(ac.content); k++ {
			v := ac.content[k]
			i := uint(v) >> 6
			bc.bitmap[i] ^= (uint64(1) << (v % 64))
		}
		bc.computeCardinality()
		if bc.cardinality <= arrayDefaultMaxSize {
			return bc.toArrayContainer()
		}
		return bc
	}
	desiredCapacity := totalCardinality
	answer := newArrayContainerCapacity(desiredCapacity)
	length := exclusiveUnion2by2(value1.content, value2.content, answer.content)
	answer.content = answer.content[:length]
	return answer

}

func (ac *arrayContainer) andNot(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.andNotArray(a.(*arrayContainer))
	case *bitmapContainer:
		return ac.andNotBitmap(a.(*bitmapContainer))
	}
	panic("should never happen")
}

func (ac *arrayContainer) iandNot(a container) container {
	switch a.(type) {
	case *arrayContainer:
		return ac.iandNotArray(a.(*arrayContainer))
	case *bitmapContainer:
		return ac.iandNotBitmap(a.(*bitmapContainer))
	}
	panic("should never happen")
}

func (ac *arrayContainer) andNotArray(value2 *arrayContainer) container {
	value1 := ac
	desiredcapacity := value1.getCardinality()
	answer := newArrayContainerCapacity(desiredcapacity)
	length := difference(value1.content, value2.content, answer.content)
	answer.content = answer.content[:length]
	return answer
}

func (ac *arrayContainer) iandNotArray(value2 *arrayContainer) container {
	length := difference(ac.content, value2.content, ac.content)
	ac.content = ac.content[:length]
	return ac
}

func (ac *arrayContainer) andNotBitmap(value2 *bitmapContainer) container {
	desiredcapacity := ac.getCardinality()
	answer := newArrayContainerCapacity(desiredcapacity)
	answer.content = answer.content[:desiredcapacity]
	pos := 0
	for _, v := range ac.content {
		if !value2.contains(v) {
			answer.content[pos] = v
			pos++
		}
	}
	answer.content = answer.content[:pos]
	return answer
}

func (ac *arrayContainer) andBitmap(value2 *bitmapContainer) container {
	desiredcapacity := ac.getCardinality()
	answer := newArrayContainerCapacity(desiredcapacity)
	answer.content = answer.content[:desiredcapacity]
	pos := 0
	for _, v := range ac.content {
		if value2.contains(v) {
			answer.content[pos] = v
			pos++
		}
	}
	answer.content = answer.content[:pos]
	return answer
}

func (ac *arrayContainer) iandNotBitmap(value2 *bitmapContainer) container {
	pos := 0
	for _, v := range ac.content {
		if !value2.contains(v) {
			ac.content[pos] = v
			pos++
		}
	}
	ac.content = ac.content[:pos]
	return ac
}

func copyOf(array []uint16, size int) []uint16 {
	result := make([]uint16, size)
	for i, x := range array {
		if i == size {
			break
		}
		result[i] = x
	}
	return result
}

// flip the values in the range [firstOfRange,lastOfRange)
func (ac *arrayContainer) inot(firstOfRange, lastOfRange int) container {
	if firstOfRange >= lastOfRange {
		return ac
	}
	return ac.inotClose(firstOfRange, lastOfRange-1) // remove everything in [firstOfRange,lastOfRange-1]
}

// flip the values in the range [firstOfRange,lastOfRange]
func (ac *arrayContainer) inotClose(firstOfRange, lastOfRange int) container {
	if firstOfRange > lastOfRange { // unlike add and remove, not uses an inclusive range [firstOfRange,lastOfRange]
		return ac
	}
	// determine the span of array indices to be affected
	startIndex := binarySearch(ac.content, uint16(firstOfRange))
	if startIndex < 0 {
		startIndex = -startIndex - 1
	}
	lastIndex := binarySearch(ac.content, uint16(lastOfRange))
	if lastIndex < 0 {
		lastIndex = -lastIndex - 1 - 1
	}
	currentValuesInRange := lastIndex - startIndex + 1
	spanToBeFlipped := lastOfRange - firstOfRange + 1

	newValuesInRange := spanToBeFlipped - currentValuesInRange
	buffer := make([]uint16, newValuesInRange)
	cardinalityChange := newValuesInRange - currentValuesInRange
	newCardinality := len(ac.content) + cardinalityChange
	if cardinalityChange > 0 {
		if newCardinality > len(ac.content) {
			if newCardinality > arrayDefaultMaxSize {
				return ac.toBitmapContainer().inot(firstOfRange, lastOfRange+1)
			}
			ac.content = copyOf(ac.content, newCardinality)
		}
		base := lastIndex + 1
		copy(ac.content[lastIndex+1+cardinalityChange:], ac.content[base:base+len(ac.content)-1-lastIndex])

		ac.negateRange(buffer, startIndex, lastIndex, firstOfRange, lastOfRange)
	} else { // no expansion needed
		ac.negateRange(buffer, startIndex, lastIndex, firstOfRange, lastOfRange)
		if cardinalityChange < 0 {

			for i := startIndex + newValuesInRange; i < newCardinality; i++ {
				ac.content[i] = ac.content[i-cardinalityChange]
			}
		}
	}
	ac.content = ac.content[:newCardinality]
	return ac
}

func (ac *arrayContainer) negateRange(buffer []uint16, startIndex, lastIndex, startRange, lastRange int) {
	// compute the negation into buffer

	outPos := 0
	inPos := startIndex // value here always >= valInRange,
	// until it is exhausted
	// n.b., we can start initially exhausted.

	valInRange := startRange
	for ; valInRange <= lastRange && inPos <= lastIndex; valInRange++ {
		if uint16(valInRange) != ac.content[inPos] {
			buffer[outPos] = uint16(valInRange)
			outPos++
		} else {
			inPos++
		}
	}

	// if there are extra items (greater than the biggest
	// pre-existing one in range), buffer them
	for ; valInRange <= lastRange; valInRange++ {
		buffer[outPos] = uint16(valInRange)
		outPos++
	}

	if outPos != len(buffer) {
		//panic("negateRange: outPos " + outPos + " whereas buffer.length=" + len(buffer))
		panic("negateRange: outPos  whereas buffer.length=")
	}

	for i, item := range buffer {
		ac.content[i] = item
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func (ac *arrayContainer) andArray(value2 *arrayContainer) *arrayContainer {
	desiredcapacity := min(ac.getCardinality(), value2.getCardinality())
	answer := newArrayContainerCapacity(desiredcapacity)
	length := intersection2by2(
		ac.content,
		value2.content,
		answer.content)
	answer.content = answer.content[:length]
	return answer
}

func (ac *arrayContainer) intersectsArray(value2 *arrayContainer) bool {
	return intersects2by2(
		ac.content,
		value2.content)
}

func (ac *arrayContainer) iandArray(value2 *arrayContainer) *arrayContainer {
	length := intersection2by2(
		ac.content,
		value2.content,
		ac.content)
	ac.content = ac.content[:length]
	return ac
}

func (ac *arrayContainer) getCardinality() int {
	return len(ac.content)
}

func (ac *arrayContainer) rank(x uint16) int {
	answer := binarySearch(ac.content, x)
	if answer >= 0 {
		return answer + 1
	}
	return -answer - 1

}

func (ac *arrayContainer) selectInt(x uint16) int {
	return int(ac.content[x])
}

func (ac *arrayContainer) clone() container {
	ptr := arrayContainer{make([]uint16, len(ac.content))}
	copy(ptr.content, ac.content[:])
	return &ptr
}

func (ac *arrayContainer) contains(x uint16) bool {
	return binarySearch(ac.content, x) >= 0
}

func (ac *arrayContainer) loadData(bitmapContainer *bitmapContainer) {
	ac.content = make([]uint16, bitmapContainer.cardinality, bitmapContainer.cardinality)
	bitmapContainer.fillArray(ac.content)
}
func newArrayContainer() *arrayContainer {
	p := new(arrayContainer)
	return p
}

func newArrayContainerCapacity(size int) *arrayContainer {
	p := new(arrayContainer)
	p.content = make([]uint16, 0, size)
	return p
}

func newArrayContainerSize(size int) *arrayContainer {
	p := new(arrayContainer)
	p.content = make([]uint16, size, size)
	return p
}

func newArrayContainerRange(firstOfRun, lastOfRun int) *arrayContainer {
	valuesInRange := lastOfRun - firstOfRun + 1
	this := newArrayContainerCapacity(valuesInRange)
	for i := 0; i < valuesInRange; i++ {
		this.content = append(this.content, uint16(firstOfRun+i))
	}
	return this
}
