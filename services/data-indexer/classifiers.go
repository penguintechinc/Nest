package main

import "strings"

type ClassificationResult struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func ClassifyColumn(columnName, dataType string) []ClassificationResult {
	name := strings.ToLower(columnName)
	results := []ClassificationResult{}

	// PII patterns
	piiPatterns := map[string][]string{
		"email":   {"email", "e_mail", "emailaddress", "email_addr"},
		"phone":   {"phone", "mobile", "cell", "tel", "telephone", "phonenumber"},
		"name":    {"first_name", "last_name", "full_name", "firstname", "lastname", "fullname", "displayname"},
		"address": {"address", "street", "city", "zipcode", "zip_code", "postal"},
		"ssn":     {"ssn", "social_security", "sin", "national_id", "tax_id"},
		"dob":     {"dob", "date_of_birth", "birthdate", "birth_date"},
		"ip":      {"ip_address", "ipaddress", "client_ip", "user_ip", "source_ip"},
	}
	for reason, patterns := range piiPatterns {
		for _, p := range patterns {
			if strings.Contains(name, p) {
				results = append(results, ClassificationResult{
					Label:      "PII",
					Confidence: 92.0,
					Reason:     "column name matches " + reason + " pattern",
				})
				goto nextCheck
			}
		}
	}
nextCheck:

	// PCI patterns
	pciPatterns := []string{"credit_card", "creditcard", "card_number", "cardnumber", "cvv", "cvc", "card_expiry", "expiry_date"}
	for _, p := range pciPatterns {
		if strings.Contains(name, p) {
			results = append(results, ClassificationResult{
				Label:      "PCI",
				Confidence: 95.0,
				Reason:     "column name matches payment card pattern",
			})
			break
		}
	}

	// PHI patterns
	phiPatterns := []string{"diagnosis", "icd", "medication", "prescription", "mrn", "medical_record", "patient_id", "health_plan", "insurance_id"}
	for _, p := range phiPatterns {
		if strings.Contains(name, p) {
			results = append(results, ClassificationResult{
				Label:      "PHI",
				Confidence: 90.0,
				Reason:     "column name matches health info pattern",
			})
			break
		}
	}

	// CREDENTIALS patterns
	credPatterns := []string{"api_key", "apikey", "secret", "password", "passwd", "token", "access_key", "private_key", "ssh_key", "auth_token"}
	for _, p := range credPatterns {
		if strings.Contains(name, p) {
			results = append(results, ClassificationResult{
				Label:      "CREDENTIALS",
				Confidence: 88.0,
				Reason:     "column name matches credentials pattern",
			})
			break
		}
	}

	// SENSITIVE patterns
	sensitivePatterns := []string{"salary", "income", "revenue", "bank_account", "routing_number", "iban", "swift"}
	for _, p := range sensitivePatterns {
		if strings.Contains(name, p) {
			results = append(results, ClassificationResult{
				Label:      "SENSITIVE",
				Confidence: 80.0,
				Reason:     "column name matches financial pattern",
			})
			break
		}
	}

	return results
}
