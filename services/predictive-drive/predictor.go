package main

import (
	"math"
	"sync"
	"time"
)

type DriveMetrics struct {
	NodeName           string  `json:"nodeName"`
	DevicePath         string  `json:"devicePath"`
	TemperatureCelsius float64 `json:"temperatureCelsius"`
	ReallocatedSectors int     `json:"reallocatedSectors"`
	SeekErrors         int     `json:"seekErrors"`
	SpinRetryCount     int     `json:"spinRetryCount"`
	PowerOnHours       int     `json:"powerOnHours"`
	ReadErrorRate      float64 `json:"readErrorRate"`
}

type DriveRiskAssessment struct {
	NodeName      string    `json:"nodeName"`
	DevicePath    string    `json:"devicePath"`
	RiskScore     float64   `json:"riskScore"`
	FailureLikely bool      `json:"failureLikely"`
	DaysToFailure int       `json:"daysToFailure,omitempty"`
	Confidence    float64   `json:"confidence"`
	Reason        string    `json:"reason"`
	AssessedAt    time.Time `json:"assessedAt"`
}

type DrivePredictor struct {
	mu          sync.RWMutex
	assessments map[string]*DriveRiskAssessment
}

func NewDrivePredictor() *DrivePredictor {
	return &DrivePredictor{
		assessments: make(map[string]*DriveRiskAssessment),
	}
}

func (p *DrivePredictor) Assess(m DriveMetrics) *DriveRiskAssessment {
	score := 0.0

	if m.ReallocatedSectors > 0 {
		score += float64(m.ReallocatedSectors) * 5.0
	}
	if m.SpinRetryCount > 5 {
		score += float64(m.SpinRetryCount) * 2.0
	}
	if m.TemperatureCelsius > 50 {
		score += (m.TemperatureCelsius - 50) * 3.0
	}
	if m.SeekErrors > 100 {
		score += float64(m.SeekErrors/100) * 2.0
	}

	score = math.Min(score, 100.0)

	daysToFailure := 0
	if score > 70 {
		daysToFailure = 7
	} else if score > 40 {
		daysToFailure = 30
	}

	reason := "Normal operation"
	if score > 70 {
		reason = "Critical: high risk of failure"
	} else if score > 40 {
		reason = "Warning: elevated risk"
	}

	assessment := &DriveRiskAssessment{
		NodeName:      m.NodeName,
		DevicePath:    m.DevicePath,
		RiskScore:     score,
		FailureLikely: score > 70,
		DaysToFailure: daysToFailure,
		Confidence:    65.0,
		Reason:        reason,
		AssessedAt:    time.Now(),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	key := m.NodeName + ":" + m.DevicePath
	p.assessments[key] = assessment

	return assessment
}

func (p *DrivePredictor) GetAssessment(node, device string) (*DriveRiskAssessment, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := node + ":" + device
	a, ok := p.assessments[key]
	return a, ok
}

func (p *DrivePredictor) ListAssessments(node string) []*DriveRiskAssessment {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*DriveRiskAssessment
	for _, a := range p.assessments {
		if node == "" || a.NodeName == node {
			result = append(result, a)
		}
	}
	return result
}

func (p *DrivePredictor) HighRisk() []*DriveRiskAssessment {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*DriveRiskAssessment
	for _, a := range p.assessments {
		if a.RiskScore > 70 {
			result = append(result, a)
		}
	}
	return result
}
