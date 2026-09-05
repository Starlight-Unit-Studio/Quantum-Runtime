package resourcecontrol

import (
	"context"
	"sync"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/calibration"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/placement"
)

type Manager struct {
	calibrationMu sync.Mutex
	stateMu       sync.RWMutex
	last          *calibration.Result
}

func New() *Manager {
	return &Manager{}
}

func (m *Manager) Host() hostprofile.Profile {
	return hostprofile.Discover()
}

func (m *Manager) Calibrate(ctx context.Context, budget time.Duration) (hostprofile.Profile, calibration.Result) {
	m.calibrationMu.Lock()
	defer m.calibrationMu.Unlock()
	host := hostprofile.Discover()
	result := calibration.Run(ctx, host, budget)
	m.stateMu.Lock()
	copy := result
	m.last = &copy
	m.stateMu.Unlock()
	return host, result
}

func (m *Manager) LastCalibration() (calibration.Result, bool) {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	if m.last == nil {
		return calibration.Result{}, false
	}
	return *m.last, true
}

func (m *Manager) Plan(req placement.Request) (hostprofile.Profile, *calibration.Result, placement.Plan, error) {
	host := hostprofile.Discover()
	var last *calibration.Result
	if result, ok := m.LastCalibration(); ok {
		copy := result
		last = &copy
	}
	plan, err := placement.Build(host, last, req)
	return host, last, plan, err
}
