/*
Original work Copyright (c) 2013 zhenjl
Modified work Copyright (c) 2015 Tyler Treat

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package boom

import (
	"hash"
	"hash/fnv"
	"math"
)

// PartitionedBloomFilter implements a variation of a classic Bloom filter as
// described by Almeida, Baquero, Preguica, and Hutchison in Scalable Bloom
// Filters:
//
// http://gsd.di.uminho.pt/members/cbm/ps/dbloom.pdf
//
// This filter works by partitioning the M-sized bit array into k slices of
// size m = M/k bits. Each hash function produces an index over m for its
// respective slice. Thus, each element is described by exactly k bits, meaning
// the distribution of false positives is uniform across all elements.
type PartitionedBloomFilter struct {
	partitions []*Buckets  // partitioned filter data
	hash       hash.Hash64 // hash function (kernel for all k functions)
	m          uint        // filter size (divided into k partitions)
	k          uint        // number of hash functions (and partitions)
	s          uint        // partition size (m / k)
	count      uint        // number of items added
}

// NewPartitionedBloomFilter creates a new partitioned Bloom filter optimized
// to store n items with a specified target false-positive rate.
func NewPartitionedBloomFilter(n uint, fpRate float64) *PartitionedBloomFilter {
	var (
		m          = OptimalM(n, fpRate)
		k          = OptimalK(fpRate)
		partitions = make([]*Buckets, k)
		s          = uint(math.Ceil(float64(m) / float64(k)))
	)

	for i := uint(0); i < k; i++ {
		partitions[i] = NewBuckets(s, 1)
	}

	return &PartitionedBloomFilter{
		partitions: partitions,
		hash:       fnv.New64(),
		m:          m,
		k:          k,
		s:          s,
	}
}

// Capacity returns the Bloom filter capacity, m.
func (p *PartitionedBloomFilter) Capacity() uint {
	return p.m
}

// K returns the number of hash functions.
func (p *PartitionedBloomFilter) K() uint {
	return p.k
}

// Count returns the number of items added to the filter.
func (p *PartitionedBloomFilter) Count() uint {
	return p.count
}

// EstimatedFillRatio returns the current estimated ratio of set bits.
func (p *PartitionedBloomFilter) EstimatedFillRatio() float64 {
	return 1 - math.Exp(-float64(p.count)/float64(p.s))
}

// FillRatio returns the average ratio of set bits across all partitions.
func (p *PartitionedBloomFilter) FillRatio() float64 {
	t := float64(0)
	for i := uint(0); i < p.k; i++ {
		sum := uint32(0)
		for j := uint(0); j < p.partitions[i].Count(); j++ {
			sum += p.partitions[i].Get(j)
		}
		t += (float64(sum) / float64(p.s))
	}
	return t / float64(p.k)
}

// Test will test for membership of the data and returns true if it is a
// member, false if not. This is a probabilistic test, meaning there is a
// non-zero probability of false positives but a zero probability of false
// negatives. Due to the way the filter is partitioned, the probability of
// false positives is uniformly distributed across all elements.
func (p *PartitionedBloomFilter) Test(data []byte) bool {
	lower, upper := hashKernel(data, p.hash)

	// If any of the K partition bits are not set, then it's not a member.
	for i := uint(0); i < p.k; i++ {
		if p.partitions[i].Get((uint(lower)+uint(upper)*i)%p.s) == 0 {
			return false
		}
	}

	return true
}

// Add will add the data to the Bloom filter. It returns the filter to allow
// for chaining.
func (p *PartitionedBloomFilter) Add(data []byte) Filter {
	lower, upper := hashKernel(data, p.hash)

	// Set the K partition bits.
	for i := uint(0); i < p.k; i++ {
		p.partitions[i].Set((uint(lower)+uint(upper)*i)%p.s, 1)
	}

	p.count++
	return p
}

// TestAndAdd is equivalent to calling Test followed by Add. It returns true if
// the data is a member, false if not.
func (p *PartitionedBloomFilter) TestAndAdd(data []byte) bool {
	lower, upper := hashKernel(data, p.hash)
	member := true

	// If any of the K partition bits are not set, then it's not a member.
	for i := uint(0); i < p.k; i++ {
		idx := (uint(lower) + uint(upper)*i) % p.s
		if p.partitions[i].Get(idx) == 0 {
			member = false
		}
		p.partitions[i].Set(idx, 1)
	}

	p.count++
	return member
}

// Reset restores the Bloom filter to its original state. It returns the filter
// to allow for chaining.
func (p *PartitionedBloomFilter) Reset() *PartitionedBloomFilter {
	for _, partition := range p.partitions {
		partition.Reset()
	}
	return p
}

// SetHash sets the hashing function used in the filter.
// For the effect on false positive rates see: https://github.com/tylertreat/BoomFilters/pull/1
func (p *PartitionedBloomFilter) SetHash(h hash.Hash64) {
	p.hash = h
}
