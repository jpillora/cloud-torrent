package roaring

const (
	arrayDefaultMaxSize = 4096 // containers with 4096 or fewer integers should be array containers.
	arrayLazyLowerBound = 1024
	maxCapacity         = 1 << 16
	serialCookie        = 12346
	invalidCardinality  = -1
)

func getSizeInBytesFromCardinality(card int) int {
	if card > arrayDefaultMaxSize {
		return maxCapacity / 8
	}
	return 2 * int(card)
}

// should be replaced with optimized assembly instructions
func numberOfTrailingZeros(i uint64) int {
	if i == 0 {
		return 64
	}
	x := i
	n := int64(63)
	y := x << 32
	if y != 0 {
		n -= 32
		x = y
	}
	y = x << 16
	if y != 0 {
		n -= 16
		x = y
	}
	y = x << 8
	if y != 0 {
		n -= 8
		x = y
	}
	y = x << 4
	if y != 0 {
		n -= 4
		x = y
	}
	y = x << 2
	if y != 0 {
		n -= 2
		x = y
	}
	return int(n - int64(uint64(x<<1)>>63))
}

func fill(arr []uint64, val uint64) {
	for i := range arr {
		arr[i] = val
	}
}
func fillRange(arr []uint64, start, end int, val uint64) {
	for i := start; i < end; i++ {
		arr[i] = val
	}
}

func fillArrayAND(container []uint16, bitmap1, bitmap2 []uint64) {
	if len(bitmap1) != len(bitmap2) {
		panic("array lengths don't match")
	}
	// TODO: rewrite in assembly
	pos := 0
	for k := range bitmap1 {
		bitset := bitmap1[k] & bitmap2[k]
		for bitset != 0 {
			t := bitset & -bitset
			container[pos] = uint16((k*64 + int(popcount(t-1))))
			pos = pos + 1
			bitset ^= t
		}
	}
}

func fillArrayANDNOT(container []uint16, bitmap1, bitmap2 []uint64) {
	if len(bitmap1) != len(bitmap2) {
		panic("array lengths don't match")
	}
	// TODO: rewrite in assembly
	pos := 0
	for k := range bitmap1 {
		bitset := bitmap1[k] &^ bitmap2[k]
		for bitset != 0 {
			t := bitset & -bitset
			container[pos] = uint16((k*64 + int(popcount(t-1))))
			pos = pos + 1
			bitset ^= t
		}
	}
}

func fillArrayXOR(container []uint16, bitmap1, bitmap2 []uint64) {
	if len(bitmap1) != len(bitmap2) {
		panic("array lengths don't match")
	}
	// TODO: rewrite in assembly
	pos := 0
	for k := 0; k < len(bitmap1); k++ {
		bitset := bitmap1[k] ^ bitmap2[k]
		for bitset != 0 {
			t := bitset & -bitset
			container[pos] = uint16((k*64 + int(popcount(t-1))))
			pos = pos + 1
			bitset ^= t
		}
	}
}

func highbits(x uint32) uint16 {
	return uint16(x >> 16)
}
func lowbits(x uint32) uint16 {
	return uint16(x & 0xFFFF)
}

func maxLowBit() uint16 {
	return uint16(0xFFFF)
}

func toIntUnsigned(x uint16) uint32 {
	return uint32(x)
}

func flipBitmapRange(bitmap []uint64, start int, end int) {
	if start >= end {
		return
	}
	firstword := start / 64
	endword := (end - 1) / 64
	bitmap[firstword] ^= ^(^uint64(0) << uint(start%64))
	for i := firstword; i < endword; i++ {
		bitmap[i] = ^bitmap[i]
	}
	bitmap[endword] ^= ^uint64(0) >> (uint(-end) % 64)
}

func resetBitmapRange(bitmap []uint64, start int, end int) {
	if start >= end {
		return
	}
	firstword := start / 64
	endword := (end - 1) / 64
	if firstword == endword {
		bitmap[firstword] &= ^((^uint64(0) << uint(start%64)) & (^uint64(0) >> (uint(-end) % 64)))
		return
	}
	bitmap[firstword] &= ^(^uint64(0) << uint(start%64))
	for i := firstword + 1; i < endword; i++ {
		bitmap[i] = 0
	}
	bitmap[endword] &= ^(^uint64(0) >> (uint(-end) % 64))

}

func setBitmapRange(bitmap []uint64, start int, end int) {
	if start >= end {
		return
	}
	firstword := start / 64
	endword := (end - 1) / 64
	if firstword == endword {
		bitmap[firstword] |= (^uint64(0) << uint(start%64)) & (^uint64(0) >> (uint(-end) % 64))
		return
	}
	bitmap[firstword] |= ^uint64(0) << uint(start%64)
	for i := firstword + 1; i < endword; i++ {
		bitmap[i] = ^uint64(0)
	}
	bitmap[endword] |= ^uint64(0) >> (uint(-end) % 64)
}

func flipBitmapRangeAndCardinalityChange(bitmap []uint64, start int, end int) int {
	before := wordCardinalityForBitmapRange(bitmap, start, end)
	flipBitmapRange(bitmap, start, end)
	after := wordCardinalityForBitmapRange(bitmap, start, end)
	return int(after - before)
}

func resetBitmapRangeAndCardinalityChange(bitmap []uint64, start int, end int) int {
	before := wordCardinalityForBitmapRange(bitmap, start, end)
	resetBitmapRange(bitmap, start, end)
	after := wordCardinalityForBitmapRange(bitmap, start, end)
	return int(after - before)
}

func setBitmapRangeAndCardinalityChange(bitmap []uint64, start int, end int) int {
	before := wordCardinalityForBitmapRange(bitmap, start, end)
	setBitmapRange(bitmap, start, end)
	after := wordCardinalityForBitmapRange(bitmap, start, end)
	return int(after - before)
}

func wordCardinalityForBitmapRange(bitmap []uint64, start int, end int) uint64 {
	answer := uint64(0)
	if start >= end {
		return answer
	}
	firstword := start / 64
	endword := (end - 1) / 64
	for i := firstword; i <= endword; i++ {
		answer += popcount(bitmap[i])
	}
	return answer
}

func selectBitPosition(w uint64, j int) int {
	seen := 0

	// Divide 64bit
	part := w & 0xFFFFFFFF
	n := popcount(part)
	if n <= uint64(j) {
		part = w >> 32
		seen += 32
		j -= int(n)
	}
	w = part

	// Divide 32bit
	part = w & 0xFFFF
	n = popcount(part)
	if n <= uint64(j) {
		part = w >> 16
		seen += 16
		j -= int(n)
	}
	w = part

	// Divide 16bit
	part = w & 0xFF
	n = popcount(part)
	if n <= uint64(j) {
		part = w >> 8
		seen += 8
		j -= int(n)
	}
	w = part

	// Lookup in final byte
	var counter uint
	for counter = 0; counter < 8; counter++ {
		j -= int((w >> counter) & 1)
		if j < 0 {
			break
		}
	}
	return seen + int(counter)

}
