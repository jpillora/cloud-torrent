// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// Service runs a loop for discovery of IGDs (Internet Gateway Devices) and
// setup/renewal of a port mapping.
type Service struct {
	id   protocol.DeviceID
	cfg  *config.Wrapper
	stop chan struct{}

	mappings []*Mapping
	timer    *time.Timer
	mut      sync.RWMutex
}

func NewService(id protocol.DeviceID, cfg *config.Wrapper) *Service {
	return &Service{
		id:  id,
		cfg: cfg,

		timer: time.NewTimer(0),
		mut:   sync.NewRWMutex(),
	}
}

func (s *Service) Serve() {
	announce := stdsync.Once{}

	s.mut.Lock()
	s.timer.Reset(0)
	s.stop = make(chan struct{})
	s.mut.Unlock()

	for {
		select {
		case <-s.timer.C:
			if found := s.process(); found != -1 {
				announce.Do(func() {
					suffix := "s"
					if found == 1 {
						suffix = ""
					}
					l.Infoln("Detected", found, "NAT service"+suffix)
				})
			}
		case <-s.stop:
			s.timer.Stop()
			s.mut.RLock()
			for _, mapping := range s.mappings {
				mapping.clearAddresses()
			}
			s.mut.RUnlock()
			return
		}
	}
}

func (s *Service) process() int {
	// toRenew are mappings which are due for renewal
	// toUpdate are the remaining mappings, which will only be updated if one of
	// the old IGDs has gone away, or a new IGD has appeared, but only if we
	// actually need to perform a renewal.
	var toRenew, toUpdate []*Mapping

	renewIn := time.Duration(s.cfg.Options().NATRenewalM) * time.Minute
	if renewIn == 0 {
		// We always want to do renewal so lets just pick a nice sane number.
		renewIn = 30 * time.Minute
	}

	s.mut.RLock()
	for _, mapping := range s.mappings {
		if mapping.expires.Before(time.Now()) {
			toRenew = append(toRenew, mapping)
		} else {
			toUpdate = append(toUpdate, mapping)
			mappingRenewIn := mapping.expires.Sub(time.Now())
			if mappingRenewIn < renewIn {
				renewIn = mappingRenewIn
			}
		}
	}
	// Reset the timer while holding the lock, because of the following race:
	// T1: process acquires lock
	// T1: process checks the mappings and gets next renewal time in 30m
	// T2: process releases the lock
	// T2: NewMapping acquires the lock
	// T2: NewMapping adds mapping
	// T2: NewMapping releases the lock
	// T2: NewMapping resets timer to 1s
	// T1: process resets timer to 30
	s.timer.Reset(renewIn)
	s.mut.RUnlock()

	// Don't do anything, unless we really need to renew
	if len(toRenew) == 0 {
		return -1
	}

	nats := discoverAll(time.Duration(s.cfg.Options().NATRenewalM)*time.Minute, time.Duration(s.cfg.Options().NATTimeoutS)*time.Second)

	for _, mapping := range toRenew {
		s.updateMapping(mapping, nats, true)
	}

	for _, mapping := range toUpdate {
		s.updateMapping(mapping, nats, false)
	}

	return len(nats)
}

func (s *Service) Stop() {
	s.mut.RLock()
	close(s.stop)
	s.mut.RUnlock()
}

func (s *Service) NewMapping(protocol Protocol, ip net.IP, port int) *Mapping {
	mapping := &Mapping{
		protocol: protocol,
		address: Address{
			IP:   ip,
			Port: port,
		},
		extAddresses: make(map[string]Address),
		mut:          sync.NewRWMutex(),
	}

	s.mut.Lock()
	s.mappings = append(s.mappings, mapping)
	// Reset the timer while holding the lock, see process() for explanation
	s.timer.Reset(time.Second)
	s.mut.Unlock()

	return mapping
}

// RemoveMapping does not actually remove the mapping from the IGD, it just
// internally removes it which stops renewing the mapping. Also, it clears any
// existing mapped addresses from the mapping, which as a result should cause
// discovery to reannounce the new addresses.
func (s *Service) RemoveMapping(mapping *Mapping) {
	s.mut.Lock()
	defer s.mut.Unlock()
	for i, existing := range s.mappings {
		if existing == mapping {
			mapping.clearAddresses()
			last := len(s.mappings) - 1
			s.mappings[i] = s.mappings[last]
			s.mappings[last] = nil
			s.mappings = s.mappings[:last]
			return
		}
	}
}

// updateMapping compares the addresses of the existing mapping versus the natds
// discovered, and removes any addresses of natds that do not exist, or tries to
// acquire mappings for natds which the mapping was unaware of before.
// Optionally takes renew flag which indicates whether or not we should renew
// mappings with existing natds
func (s *Service) updateMapping(mapping *Mapping, nats map[string]Device, renew bool) {
	var added, removed []Address

	renewalTime := time.Duration(s.cfg.Options().NATRenewalM) * time.Minute
	mapping.expires = time.Now().Add(renewalTime)

	newAdded, newRemoved := s.verifyExistingMappings(mapping, nats, renew)
	added = append(added, newAdded...)
	removed = append(removed, newRemoved...)

	newAdded, newRemoved = s.acquireNewMappings(mapping, nats)
	added = append(added, newAdded...)
	removed = append(removed, newRemoved...)

	if len(added) > 0 || len(removed) > 0 {
		mapping.notify(added, removed)
	}
}

func (s *Service) verifyExistingMappings(mapping *Mapping, nats map[string]Device, renew bool) ([]Address, []Address) {
	var added, removed []Address

	leaseTime := time.Duration(s.cfg.Options().NATLeaseM) * time.Minute

	for id, address := range mapping.addressMap() {
		// Delete addresses for NATDevice's that do not exist anymore
		nat, ok := nats[id]
		if !ok {
			mapping.removeAddress(id)
			removed = append(removed, address)
			continue
		} else if renew {
			// Only perform renewals on the nat's that have the right local IP
			// address
			localIP := nat.GetLocalIPAddress()
			if !mapping.validGateway(localIP) {
				l.Debugf("Skipping %s for %s because of IP mismatch. %s != %s", id, mapping, mapping.address.IP, localIP)
				continue
			}

			l.Debugf("Renewing %s -> %s mapping on %s", mapping, address, id)

			addr, err := s.tryNATDevice(nat, mapping.address.Port, address.Port, leaseTime)
			if err != nil {
				l.Debugf("Failed to renew %s -> mapping on %s", mapping, address, id)
				mapping.removeAddress(id)
				removed = append(removed, address)
				continue
			}

			l.Debugf("Renewed %s -> %s mapping on %s", mapping, address, id)

			if !addr.Equal(address) {
				mapping.removeAddress(id)
				mapping.setAddress(id, addr)
				removed = append(removed, address)
				added = append(added, address)
			}
		}
	}

	return added, removed
}

func (s *Service) acquireNewMappings(mapping *Mapping, nats map[string]Device) ([]Address, []Address) {
	var added, removed []Address

	leaseTime := time.Duration(s.cfg.Options().NATLeaseM) * time.Minute
	addrMap := mapping.addressMap()

	for id, nat := range nats {
		if _, ok := addrMap[id]; ok {
			continue
		}

		// Only perform mappings on the nat's that have the right local IP
		// address
		localIP := nat.GetLocalIPAddress()
		if !mapping.validGateway(localIP) {
			l.Debugf("Skipping %s for %s because of IP mismatch. %s != %s", id, mapping, mapping.address.IP, localIP)
			continue
		}

		l.Debugf("Acquiring %s mapping on %s", mapping, id)

		addr, err := s.tryNATDevice(nat, mapping.address.Port, 0, leaseTime)
		if err != nil {
			l.Debugf("Failed to acquire %s mapping on %s", mapping, id)
			continue
		}

		l.Debugf("Acquired %s -> %s mapping on %s", mapping, addr, id)

		mapping.setAddress(id, addr)
		added = append(added, addr)
	}

	return added, removed
}

// tryNATDevice tries to acquire a port mapping for the given internal address to
// the given external port. If external port is 0, picks a pseudo-random port.
func (s *Service) tryNATDevice(natd Device, intPort, extPort int, leaseTime time.Duration) (Address, error) {
	var err error
	var port int

	// Generate a predictable random which is based on device ID + local port + hash of the device ID
	// number so that the ports we'd try to acquire for the mapping would always be the same for the
	// same device trying to get the same internal port.
	predictableRand := rand.New(rand.NewSource(int64(s.id.Short()) + int64(intPort) + hash(natd.ID())))

	if extPort != 0 {
		// First try renewing our existing mapping, if we have one.
		name := fmt.Sprintf("syncthing-%d", extPort)
		port, err = natd.AddPortMapping(TCP, intPort, extPort, name, leaseTime)
		if err == nil {
			extPort = port
			goto findIP
		}
		l.Debugln("Error extending lease on", natd.ID(), err)
	}

	for i := 0; i < 10; i++ {
		// Then try up to ten random ports.
		extPort = 1024 + predictableRand.Intn(65535-1024)
		name := fmt.Sprintf("syncthing-%d", extPort)
		port, err = natd.AddPortMapping(TCP, intPort, extPort, name, leaseTime)
		if err == nil {
			extPort = port
			goto findIP
		}
		l.Debugln("Error getting new lease on", natd.ID(), err)
	}

	return Address{}, err

findIP:
	ip, err := natd.GetExternalIPAddress()
	if err != nil {
		l.Debugln("Error getting external ip on", natd.ID(), err)
		ip = nil
	}
	return Address{
		IP:   ip,
		Port: extPort,
	}, nil
}

func hash(input string) int64 {
	h := fnv.New64a()
	h.Write([]byte(input))
	return int64(h.Sum64())
}
