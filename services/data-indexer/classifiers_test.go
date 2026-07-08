package main

import (
	"testing"
)

func TestClassifyColumnPIIPatterns(t *testing.T) {
	tests := []struct {
		name           string
		columnName     string
		dataType       string
		expectedLabel  string
		expectedMinCfn float64
	}{
		{
			name:           "email pattern",
			columnName:     "email",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "e_mail pattern",
			columnName:     "e_mail",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "emailaddress pattern",
			columnName:     "emailaddress",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "email_addr pattern",
			columnName:     "email_addr",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "phone_number pattern",
			columnName:     "phone_number",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "mobile pattern",
			columnName:     "mobile",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "cell pattern",
			columnName:     "cell",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "telephone pattern",
			columnName:     "telephone",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "phonenumber pattern",
			columnName:     "phonenumber",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "first_name pattern",
			columnName:     "first_name",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "last_name pattern",
			columnName:     "last_name",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "full_name pattern",
			columnName:     "full_name",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "firstname pattern",
			columnName:     "firstname",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "lastname pattern",
			columnName:     "lastname",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "fullname pattern",
			columnName:     "fullname",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "displayname pattern",
			columnName:     "displayname",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "address pattern",
			columnName:     "address",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "street pattern",
			columnName:     "street",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "city pattern",
			columnName:     "city",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "zipcode pattern",
			columnName:     "zipcode",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "zip_code pattern",
			columnName:     "zip_code",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "postal pattern",
			columnName:     "postal",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "ssn pattern",
			columnName:     "ssn",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "social_security pattern",
			columnName:     "social_security",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "sin pattern",
			columnName:     "sin",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "national_id pattern",
			columnName:     "national_id",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "tax_id pattern",
			columnName:     "tax_id",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "dob pattern",
			columnName:     "dob",
			dataType:       "DATE",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "date_of_birth pattern",
			columnName:     "date_of_birth",
			dataType:       "DATE",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "birthdate pattern",
			columnName:     "birthdate",
			dataType:       "DATE",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "birth_date pattern",
			columnName:     "birth_date",
			dataType:       "DATE",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "ip_address pattern",
			columnName:     "ip_address",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "ipaddress pattern",
			columnName:     "ipaddress",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "client_ip pattern",
			columnName:     "client_ip",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "user_ip pattern",
			columnName:     "user_ip",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "source_ip pattern",
			columnName:     "source_ip",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "case insensitive email",
			columnName:     "EMAIL",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
		{
			name:           "mixed case phone",
			columnName:     "PhoneNumber",
			dataType:       "VARCHAR",
			expectedLabel:  "PII",
			expectedMinCfn: 92.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, tc.dataType)
			if len(result) == 0 {
				t.Fatalf("expected PII classification, got none")
			}
			if result[0].Label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, result[0].Label)
			}
			if result[0].Confidence < tc.expectedMinCfn {
				t.Errorf("expected confidence >= %f, got %f", tc.expectedMinCfn, result[0].Confidence)
			}
			if result[0].Reason == "" {
				t.Errorf("expected reason, got empty string")
			}
		})
	}
}

func TestClassifyColumnPCIPatterns(t *testing.T) {
	tests := []struct {
		name          string
		columnName    string
		expectedLabel string
	}{
		{"credit_card pattern", "credit_card_number", "PCI"},
		{"creditcard pattern", "creditcard", "PCI"},
		{"card_number pattern", "card_number", "PCI"},
		{"cardnumber pattern", "cardnumber", "PCI"},
		{"cvv pattern", "cvv", "PCI"},
		{"cvc pattern", "cvc", "PCI"},
		{"card_expiry pattern", "card_expiry", "PCI"},
		{"expiry_date pattern", "expiry_date", "PCI"},
		{"case insensitive", "CVV", "PCI"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, "VARCHAR")
			if len(result) == 0 {
				t.Fatalf("expected PCI classification, got none")
			}
			if result[0].Label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, result[0].Label)
			}
			if result[0].Confidence != 95.0 {
				t.Errorf("expected confidence 95.0, got %f", result[0].Confidence)
			}
		})
	}
}

func TestClassifyColumnPHIPatterns(t *testing.T) {
	tests := []struct {
		name          string
		columnName    string
		expectedLabel string
	}{
		{"diagnosis pattern", "diagnosis_code", "PHI"},
		{"icd pattern", "icd_code", "PHI"},
		{"medication pattern", "medication_list", "PHI"},
		{"prescription pattern", "prescription_id", "PHI"},
		{"mrn pattern", "mrn", "PHI"},
		{"medical_record pattern", "medical_record_number", "PHI"},
		{"patient_id pattern", "patient_id", "PHI"},
		{"health_plan pattern", "health_plan", "PHI"},
		{"insurance_id pattern", "insurance_id", "PHI"},
		{"case insensitive", "DIAGNOSIS", "PHI"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, "VARCHAR")
			if len(result) == 0 {
				t.Fatalf("expected PHI classification, got none")
			}
			if result[0].Label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, result[0].Label)
			}
			if result[0].Confidence != 90.0 {
				t.Errorf("expected confidence 90.0, got %f", result[0].Confidence)
			}
		})
	}
}

func TestClassifyColumnCREDENTIALSPatterns(t *testing.T) {
	tests := []struct {
		name          string
		columnName    string
		expectedLabel string
	}{
		{"api_key pattern", "api_key", "CREDENTIALS"},
		{"apikey pattern", "apikey", "CREDENTIALS"},
		{"secret pattern", "secret", "CREDENTIALS"},
		{"password pattern", "password", "CREDENTIALS"},
		{"passwd pattern", "passwd", "CREDENTIALS"},
		{"token pattern", "token", "CREDENTIALS"},
		{"access_key pattern", "access_key", "CREDENTIALS"},
		{"private_key pattern", "private_key", "CREDENTIALS"},
		{"ssh_key pattern", "ssh_key", "CREDENTIALS"},
		{"auth_token pattern", "auth_token", "CREDENTIALS"},
		{"case insensitive", "PASSWORD", "CREDENTIALS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, "VARCHAR")
			if len(result) == 0 {
				t.Fatalf("expected CREDENTIALS classification, got none")
			}
			if result[0].Label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, result[0].Label)
			}
			if result[0].Confidence != 88.0 {
				t.Errorf("expected confidence 88.0, got %f", result[0].Confidence)
			}
		})
	}
}

func TestClassifyColumnSENSITIVEPatterns(t *testing.T) {
	tests := []struct {
		name          string
		columnName    string
		expectedLabel string
	}{
		{"salary pattern", "salary", "SENSITIVE"},
		{"income pattern", "income", "SENSITIVE"},
		{"revenue pattern", "revenue", "SENSITIVE"},
		{"bank_account pattern", "bank_account", "SENSITIVE"},
		{"routing_number pattern", "routing_number", "SENSITIVE"},
		{"iban pattern", "iban", "SENSITIVE"},
		{"swift pattern", "swift", "SENSITIVE"},
		{"case insensitive", "SALARY", "SENSITIVE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, "VARCHAR")
			if len(result) == 0 {
				t.Fatalf("expected SENSITIVE classification, got none")
			}
			if result[0].Label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, result[0].Label)
			}
			if result[0].Confidence != 80.0 {
				t.Errorf("expected confidence 80.0, got %f", result[0].Confidence)
			}
		})
	}
}

func TestClassifyColumnNeutral(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
	}{
		{"user_id", "user_id"},
		{"created_at", "created_at"},
		{"updated_at", "updated_at"},
		{"id", "id"},
		{"uuid", "uuid"},
		{"timestamp", "timestamp"},
		{"description", "description"},
		{"title", "title"},
		{"status", "status"},
		{"name without PII pattern", "name"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyColumn(tc.columnName, "VARCHAR")
			if len(result) > 0 {
				t.Errorf("expected no classification for neutral column, got %+v", result)
			}
		})
	}
}

func TestClassifyColumnMultipleClassifications(t *testing.T) {
	// Test column names that could match multiple patterns
	// (e.g., if a name contains both PCI and other patterns)
	// In the current implementation, once PII is matched, it stops (goto nextCheck)
	// PCI, PHI, CREDENTIALS, SENSITIVE use break, so only first match applies

	result := ClassifyColumn("credit_card_number", "VARCHAR")
	if len(result) != 1 {
		t.Errorf("expected single classification for credit_card_number, got %d", len(result))
	}
	if result[0].Label != "PCI" {
		t.Errorf("expected PCI for credit_card_number, got %s", result[0].Label)
	}
}

func TestClassifyColumnEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
		dataType   string
	}{
		{"empty column name", "", "VARCHAR"},
		{"empty data type", "email", ""},
		{"both empty", "", ""},
		{"single character", "a", "VARCHAR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			_ = ClassifyColumn(tc.columnName, tc.dataType)
		})
	}
}

func TestClassifyColumnDataTypeIgnored(t *testing.T) {
	// Verify data type doesn't affect classification logic
	result1 := ClassifyColumn("email", "VARCHAR")
	result2 := ClassifyColumn("email", "NUMERIC")
	result3 := ClassifyColumn("email", "")

	if len(result1) != len(result2) || len(result1) != len(result3) {
		t.Errorf("expected data type to be ignored in classification")
	}
	if result1[0].Label != result2[0].Label || result1[0].Label != result3[0].Label {
		t.Errorf("expected same label regardless of data type")
	}
}

func TestClassifyColumnReasonField(t *testing.T) {
	result := ClassifyColumn("email", "VARCHAR")
	if len(result) == 0 {
		t.Fatalf("expected classification result")
	}
	if result[0].Reason == "" {
		t.Errorf("expected non-empty reason field")
	}
	if len(result[0].Reason) < 5 {
		t.Errorf("expected descriptive reason, got: %s", result[0].Reason)
	}
}

func TestClassifyColumnSubstringMatching(t *testing.T) {
	// Test that substring matching works correctly
	result := ClassifyColumn("user_email_address", "VARCHAR")
	if len(result) == 0 {
		t.Errorf("expected PII classification for substring match")
	}

	result2 := ClassifyColumn("my_phone_number", "VARCHAR")
	if len(result2) == 0 {
		t.Errorf("expected PII classification for phone in compound name")
	}

	result3 := ClassifyColumn("api_secret_key", "VARCHAR")
	if len(result3) == 0 {
		t.Errorf("expected CREDENTIALS classification for secret in compound name")
	}
}

func TestClassifyColumnAllConfidenceLevels(t *testing.T) {
	tests := []struct {
		columnName  string
		expectedCfn float64
	}{
		{"email", 92.0},     // PII
		{"cvv", 95.0},       // PCI
		{"diagnosis", 90.0}, // PHI
		{"api_key", 88.0},   // CREDENTIALS
		{"salary", 80.0},    // SENSITIVE
	}

	for _, tc := range tests {
		result := ClassifyColumn(tc.columnName, "VARCHAR")
		if len(result) == 0 {
			t.Fatalf("expected classification for %s", tc.columnName)
		}
		if result[0].Confidence != tc.expectedCfn {
			t.Errorf("expected confidence %f for %s, got %f", tc.expectedCfn, tc.columnName, result[0].Confidence)
		}
	}
}

func TestClassifyColumnStructure(t *testing.T) {
	result := ClassifyColumn("email", "VARCHAR")
	if len(result) == 0 {
		t.Fatalf("expected classification result")
	}

	cr := result[0]
	if cr.Label == "" {
		t.Error("expected non-empty Label field")
	}
	if cr.Confidence == 0 {
		t.Error("expected non-zero Confidence field")
	}
	if cr.Reason == "" {
		t.Error("expected non-empty Reason field")
	}
}
