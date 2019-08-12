package common

import "sync"

// Mutex is wrapper for sync.Mutex.
type Mutex struct {
	mutex sync.Mutex
	bcbs  []func()
	acbs  []func()
}

// Lock acquires lock.
func (m *Mutex) Lock() {
	m.mutex.Lock()
}

// Unlock calls scheduled functions and releases lock.
func (m *Mutex) Unlock() {
	for i := 0; i < len(m.bcbs); i++ {
		m.bcbs[i]()
	}
	m.bcbs = nil
	acbs := m.acbs
	m.acbs = nil
	m.mutex.Unlock()
	for _, cb := range acbs {
		cb()
	}
}

// CallBeforeUnlock schedules to call f before next unlock. This function shall
// be called while lock is acquired.
func (m *Mutex) CallBeforeUnlock(f func()) {
	m.bcbs = append(m.bcbs, f)
}

// CallAfterUnlock schedules to call f after next unlock. This function shall be
// called while lock is acquired.
func (m *Mutex) CallAfterUnlock(f func()) {
	m.acbs = append(m.acbs, f)
}

type AutoCallLocker struct {
	locker sync.Locker
	bcbs   []func()
	acbs   []func()
}

// Unlock calls scheduled functions and releases lock.
func (m *AutoCallLocker) Unlock() {
	for i := 0; i < len(m.bcbs); i++ {
		m.bcbs[i]()
	}
	m.bcbs = nil
	acbs := m.acbs
	m.acbs = nil
	m.locker.Unlock()
	for _, cb := range acbs {
		cb()
	}
}

// CallBeforeUnlock schedules to call f before next unlock. This function shall
// be called while lock is acquired.
func (m *AutoCallLocker) CallBeforeUnlock(f func()) {
	m.bcbs = append(m.bcbs, f)
}

// CallAfterUnlock schedules to call f after next unlock. This function shall be
// called while lock is acquired.
func (m *AutoCallLocker) CallAfterUnlock(f func()) {
	m.acbs = append(m.acbs, f)
}

func LockForAutoCall(locker sync.Locker) *AutoCallLocker {
	locker.Lock()
	return &AutoCallLocker{locker: locker}
}

type AutoLock struct {
	locker sync.Locker
	locked bool
}

func NewAutoLock(l sync.Locker) *AutoLock {
	l.Lock()
	return &AutoLock{
		locker: l,
		locked: true,
	}
}

func (l *AutoLock) Unlock() {
	if l.locked {
		l.locked = false
		l.locker.Unlock()
	}
}

func (l *AutoLock) Lock() {
	l.locker.Lock()
	l.locked = true
}
