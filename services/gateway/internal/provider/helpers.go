package provider

import (
	"crypto/rand"
	"math/big"
)

const passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const passwordCharsetComplex = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"

// generateRandomPassword returns a cryptographically random password of the given length
// using the standard alphanumeric charset.
func generateRandomPassword(length int) string {
	return generateRandomPasswordFromCharset(length, passwordCharset)
}

// generateRandomPasswordComplex returns a cryptographically random password including symbols.
func generateRandomPasswordComplex(length int) string {
	return generateRandomPasswordFromCharset(length, passwordCharsetComplex)
}

func generateRandomPasswordFromCharset(length int, charset string) string {
	result := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range result {
		n, _ := rand.Int(rand.Reader, max)
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
