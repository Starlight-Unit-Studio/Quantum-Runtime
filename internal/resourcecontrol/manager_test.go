package resourcecontrol

import (
	"context"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/placement"
)

func TestCalibrationIsStored(t *testing.T) {
	m := New()
	_, result := m.Calibrate(context.Background(), 50*time.Millisecond)
	got, ok := m.LastCalibration()
	if !ok {
		t.Fatal("calibration not stored")
	}
	if got.MeasuredAt != result.MeasuredAt {
		t.Fatal("stored calibration differs")
	}
}

func TestPlanRejectsMissingModelSize(t *testing.T) {
	m := New()
	_, _, _, err := m.Plan(placement.Request{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
