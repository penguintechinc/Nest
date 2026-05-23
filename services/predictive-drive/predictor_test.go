package main

import (
	"testing"
)

func TestNewDrivePredictor(t *testing.T) {
	p := NewDrivePredictor()
	if p == nil {
		t.Fatal("NewDrivePredictor returned nil")
	}
	if p.assessments == nil {
		t.Fatal("assessments map is nil")
	}
	if len(p.assessments) != 0 {
		t.Errorf("expected empty assessments, got %d", len(p.assessments))
	}
}

func TestAssessHealthyDrive(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	if assessment.RiskScore != 0.0 {
		t.Errorf("expected risk score 0.0 for healthy drive, got %f", assessment.RiskScore)
	}
	if assessment.FailureLikely {
		t.Errorf("expected FailureLikely=false for healthy drive, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 0 {
		t.Errorf("expected DaysToFailure=0 for low risk, got %d", assessment.DaysToFailure)
	}
	if assessment.Reason != "Normal operation" {
		t.Errorf("expected reason 'Normal operation', got: %s", assessment.Reason)
	}
	if assessment.NodeName != "node1" {
		t.Errorf("expected node1, got %s", assessment.NodeName)
	}
	if assessment.DevicePath != "/dev/sda" {
		t.Errorf("expected /dev/sda, got %s", assessment.DevicePath)
	}
}

func TestAssessReallocatedSectors(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 20,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// 20 sectors * 5.0 = 100.0, but capped at 100.0
	expectedScore := float64(20) * 5.0
	if assessment.RiskScore < expectedScore-0.1 || assessment.RiskScore > 100.0 {
		t.Errorf("expected risk score %.1f (capped at 100), got %f", expectedScore, assessment.RiskScore)
	}
	if !assessment.FailureLikely {
		t.Errorf("expected FailureLikely=true for high risk, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 7 {
		t.Errorf("expected DaysToFailure=7 for critical risk, got %d", assessment.DaysToFailure)
	}
	if assessment.Reason != "Critical: high risk of failure" {
		t.Errorf("expected critical reason, got: %s", assessment.Reason)
	}
}

func TestAssessTemperature(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 80.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// (80 - 50) * 3.0 = 90.0, capped at 100.0
	expectedScore := (80.0 - 50.0) * 3.0
	if assessment.RiskScore < expectedScore-0.1 {
		t.Errorf("expected risk score >= %.1f, got %f", expectedScore, assessment.RiskScore)
	}
	if !assessment.FailureLikely {
		t.Errorf("expected FailureLikely=true for high temperature, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 7 {
		t.Errorf("expected DaysToFailure=7 for critical risk, got %d", assessment.DaysToFailure)
	}
}

func TestAssessTemperatureBelowThreshold(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 45.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// 45 < 50, so no contribution to risk
	if assessment.RiskScore != 0.0 {
		t.Errorf("expected risk score 0.0 for temp < 50, got %f", assessment.RiskScore)
	}
}

func TestAssessSeekErrors(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         500,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// (500 / 100) * 2.0 = 10.0
	expectedScore := float64(500/100) * 2.0
	if assessment.RiskScore < expectedScore-0.1 || assessment.RiskScore > expectedScore+0.1 {
		t.Errorf("expected risk score ~%.1f, got %f", expectedScore, assessment.RiskScore)
	}
	if assessment.FailureLikely {
		t.Errorf("expected FailureLikely=false for ~10 risk score, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 0 {
		t.Errorf("expected DaysToFailure=0 for low risk, got %d", assessment.DaysToFailure)
	}
}

func TestAssessSpinRetryCount(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     25,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// Spin retries > 5, so (25 * 2.0) = 50.0 (score > 40 -> days 30)
	expectedScore := float64(25) * 2.0
	if assessment.RiskScore < expectedScore-0.1 || assessment.RiskScore > expectedScore+0.1 {
		t.Errorf("expected risk score ~%.1f, got %f", expectedScore, assessment.RiskScore)
	}
	if assessment.FailureLikely {
		t.Errorf("expected FailureLikely=false for ~50 risk score, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 30 {
		t.Errorf("expected DaysToFailure=30 for medium risk (40-70), got %d", assessment.DaysToFailure)
	}
	if assessment.Reason != "Warning: elevated risk" {
		t.Errorf("expected warning reason, got: %s", assessment.Reason)
	}
}

func TestAssessSpinRetryCountBelowThreshold(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     3,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// Spin retries <= 5, so no contribution
	if assessment.RiskScore != 0.0 {
		t.Errorf("expected risk score 0.0 for spin retries <= 5, got %f", assessment.RiskScore)
	}
}

func TestAssessMediumRisk(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 60.0,
		ReallocatedSectors: 3,
		SeekErrors:         400,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// 3*5 + (60-50)*3 + (400/100)*2 = 15 + 30 + 8 = 53
	if assessment.RiskScore < 50 || assessment.RiskScore > 60 {
		t.Errorf("expected risk score ~53, got %f", assessment.RiskScore)
	}
	if assessment.FailureLikely {
		t.Errorf("expected FailureLikely=false for medium risk (score ~53), got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 30 {
		t.Errorf("expected DaysToFailure=30 for medium risk (40-70), got %d", assessment.DaysToFailure)
	}
}

func TestAssessHighRisk(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 70.0,
		ReallocatedSectors: 5,
		SeekErrors:         500,
		SpinRetryCount:     10,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// 5*5 + 10*2 + (70-50)*3 + (500/100)*2 = 25 + 20 + 60 + 10 = 115 -> capped at 100
	if assessment.RiskScore != 100.0 {
		t.Errorf("expected capped risk score 100.0, got %f", assessment.RiskScore)
	}
	if !assessment.FailureLikely {
		t.Errorf("expected FailureLikely=true for critical risk, got %v", assessment.FailureLikely)
	}
	if assessment.DaysToFailure != 7 {
		t.Errorf("expected DaysToFailure=7 for critical risk, got %d", assessment.DaysToFailure)
	}
}

func TestGetAssessment(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	p.Assess(m)

	assessment, ok := p.GetAssessment("node1", "/dev/sda")
	if !ok {
		t.Fatal("expected assessment to exist")
	}
	if assessment.NodeName != "node1" {
		t.Errorf("expected node1, got %s", assessment.NodeName)
	}
	if assessment.DevicePath != "/dev/sda" {
		t.Errorf("expected /dev/sda, got %s", assessment.DevicePath)
	}
}

func TestGetAssessmentNotFound(t *testing.T) {
	p := NewDrivePredictor()
	_, ok := p.GetAssessment("nonexistent", "/dev/nonexistent")
	if ok {
		t.Fatal("expected assessment not to exist")
	}
}

func TestListAssementsEmpty(t *testing.T) {
	p := NewDrivePredictor()
	result := p.ListAssessments("")
	if len(result) != 0 {
		t.Errorf("expected empty assessments, got %d", len(result))
	}
}

func TestListAssessmentsAllNodes(t *testing.T) {
	p := NewDrivePredictor()

	// Add assessments for 3 nodes
	for i := 1; i <= 3; i++ {
		nodeName := "node" + string(rune(i+48))
		m := DriveMetrics{
			NodeName:           nodeName,
			DevicePath:         "/dev/sda",
			TemperatureCelsius: 30.0,
			ReallocatedSectors: 0,
			SeekErrors:         0,
			SpinRetryCount:     0,
			PowerOnHours:       1000,
			ReadErrorRate:      0.0,
		}
		p.Assess(m)
	}

	result := p.ListAssessments("")
	if len(result) != 3 {
		t.Errorf("expected 3 assessments, got %d", len(result))
	}
}

func TestListAssessmentsFilterByNode(t *testing.T) {
	p := NewDrivePredictor()

	// Add assessments for 2 different nodes
	for i := 1; i <= 2; i++ {
		nodeName := "node" + string(rune(i+48))
		for j := 1; j <= 2; j++ {
			device := "/dev/sd" + string(rune(97+j))
			m := DriveMetrics{
				NodeName:           nodeName,
				DevicePath:         device,
				TemperatureCelsius: 30.0,
				ReallocatedSectors: 0,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			}
			p.Assess(m)
		}
	}

	result := p.ListAssessments("node1")
	if len(result) != 2 {
		t.Errorf("expected 2 assessments for node1, got %d", len(result))
	}

	for _, a := range result {
		if a.NodeName != "node1" {
			t.Errorf("expected all assessments for node1, got %s", a.NodeName)
		}
	}
}

func TestHighRisk(t *testing.T) {
	p := NewDrivePredictor()

	// Add mixed risk assessments
	// Low risk
	for i := 0; i < 2; i++ {
		m := DriveMetrics{
			NodeName:           "node" + string(rune(i+48)),
			DevicePath:         "/dev/sda",
			TemperatureCelsius: 30.0,
			ReallocatedSectors: 0,
			SeekErrors:         0,
			SpinRetryCount:     0,
			PowerOnHours:       1000,
			ReadErrorRate:      0.0,
		}
		p.Assess(m)
	}

	// High risk
	for i := 2; i < 4; i++ {
		m := DriveMetrics{
			NodeName:           "node" + string(rune(i+48)),
			DevicePath:         "/dev/sda",
			TemperatureCelsius: 30.0,
			ReallocatedSectors: 20,
			SeekErrors:         0,
			SpinRetryCount:     0,
			PowerOnHours:       1000,
			ReadErrorRate:      0.0,
		}
		p.Assess(m)
	}

	result := p.HighRisk()
	if len(result) != 2 {
		t.Errorf("expected 2 high-risk drives, got %d", len(result))
	}

	for _, a := range result {
		if a.RiskScore <= 70 {
			t.Errorf("expected all high-risk drives (>70), got %f", a.RiskScore)
		}
	}
}

func TestHighRiskEmpty(t *testing.T) {
	p := NewDrivePredictor()

	// Add only low-risk drives
	for i := 0; i < 3; i++ {
		m := DriveMetrics{
			NodeName:           "node" + string(rune(i+48)),
			DevicePath:         "/dev/sda",
			TemperatureCelsius: 30.0,
			ReallocatedSectors: 0,
			SeekErrors:         0,
			SpinRetryCount:     0,
			PowerOnHours:       1000,
			ReadErrorRate:      0.0,
		}
		p.Assess(m)
	}

	result := p.HighRisk()
	if len(result) != 0 {
		t.Errorf("expected 0 high-risk drives, got %d", len(result))
	}
}

func TestAssessmentConfidence(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	// Confidence should always be 65% per implementation
	if assessment.Confidence != 65.0 {
		t.Errorf("expected confidence 65.0, got %f", assessment.Confidence)
	}
}

func TestAssessmentTimestamp(t *testing.T) {
	p := NewDrivePredictor()
	m := DriveMetrics{
		NodeName:           "node1",
		DevicePath:         "/dev/sda",
		TemperatureCelsius: 30.0,
		ReallocatedSectors: 0,
		SeekErrors:         0,
		SpinRetryCount:     0,
		PowerOnHours:       1000,
		ReadErrorRate:      0.0,
	}

	assessment := p.Assess(m)

	if assessment.AssessedAt.IsZero() {
		t.Fatal("expected AssessedAt timestamp to be set")
	}
}

func TestConcurrentAssess(t *testing.T) {
	p := NewDrivePredictor()

	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			m := DriveMetrics{
				NodeName:           "node" + string(rune(idx%5+48)),
				DevicePath:         "/dev/sd" + string(rune(97+idx%4)),
				TemperatureCelsius: float64(idx*3 + 30),
				ReallocatedSectors: idx % 10,
				SeekErrors:         idx * 50,
				SpinRetryCount:     idx % 8,
				PowerOnHours:       idx*100 + 1000,
				ReadErrorRate:      float64(idx) * 0.1,
			}
			p.Assess(m)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	if len(p.assessments) == 0 {
		t.Fatal("expected assessments to be stored")
	}
}

func TestRiskScoreBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		fields DriveMetrics
		score  float64
	}{
		{
			name: "score exactly 70",
			fields: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 70.0,
				ReallocatedSectors: 2,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			score: 70.0,
		},
		{
			name: "score just above 70",
			fields: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 71.0,
				ReallocatedSectors: 2,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			score: 73.0,
		},
		{
			name: "score just below 40",
			fields: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 53.0,
				ReallocatedSectors: 0,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			score: 9.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDrivePredictor()
			assessment := p.Assess(tt.fields)
			if assessment.RiskScore < tt.score-2.0 || assessment.RiskScore > tt.score+2.0 {
				t.Errorf("expected risk score ~%.1f, got %.1f", tt.score, assessment.RiskScore)
			}
		})
	}
}

func TestDaysToFailureBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		metrics    DriveMetrics
		daysExpect int
		failExpect bool
	}{
		{
			name: "score 71 - critical",
			metrics: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 71.0,
				ReallocatedSectors: 2,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			daysExpect: 7,
			failExpect: true,
		},
		{
			name: "score 70 - not critical",
			metrics: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 70.0,
				ReallocatedSectors: 2,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			daysExpect: 30,
			failExpect: false,
		},
		{
			name: "score 41 - medium",
			metrics: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 63.67,
				ReallocatedSectors: 0,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			daysExpect: 30,
			failExpect: false,
		},
		{
			name: "score 40 - low",
			metrics: DriveMetrics{
				NodeName:           "n1",
				DevicePath:         "/dev/sda",
				TemperatureCelsius: 53.33,
				ReallocatedSectors: 0,
				SeekErrors:         0,
				SpinRetryCount:     0,
				PowerOnHours:       1000,
				ReadErrorRate:      0.0,
			},
			daysExpect: 0,
			failExpect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDrivePredictor()
			assessment := p.Assess(tt.metrics)
			if assessment.DaysToFailure != tt.daysExpect {
				t.Errorf("expected DaysToFailure %d, got %d", tt.daysExpect, assessment.DaysToFailure)
			}
			if assessment.FailureLikely != tt.failExpect {
				t.Errorf("expected FailureLikely %v, got %v", tt.failExpect, assessment.FailureLikely)
			}
		})
	}
}

func TestMultipleDrivesPerNode(t *testing.T) {
	p := NewDrivePredictor()

	// Add multiple drives for one node
	for i := 0; i < 4; i++ {
		m := DriveMetrics{
			NodeName:           "node1",
			DevicePath:         "/dev/sd" + string(rune(97+i)),
			TemperatureCelsius: 30.0 + float64(i)*10,
			ReallocatedSectors: i * 5,
			SeekErrors:         0,
			SpinRetryCount:     0,
			PowerOnHours:       1000,
			ReadErrorRate:      0.0,
		}
		p.Assess(m)
	}

	result := p.ListAssessments("node1")
	if len(result) != 4 {
		t.Errorf("expected 4 drives for node1, got %d", len(result))
	}

	// Verify key composition (node:device)
	assessment, ok := p.GetAssessment("node1", "/dev/sda")
	if !ok {
		t.Fatal("expected assessment for /dev/sda")
	}
	if assessment.DevicePath != "/dev/sda" {
		t.Errorf("expected /dev/sda, got %s", assessment.DevicePath)
	}
}
