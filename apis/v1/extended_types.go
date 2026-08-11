package v1

const (
	// Extended engine types (P5+)
	TypeTimeseries = "timeseries" // VictoriaMetrics
)

// ExtendedTypes is the set of extended engine types (not raw storage, not core DB).
var ExtendedTypes = map[string]bool{
	TypeKafka:      true,
	TypeRockFS:     true,
	TypeClickhouse: true,
	TypeSearch:     true,
	TypeVector:     true,
	TypeTimeseries: true,
}

// IsExtendedType returns true if the given type is an extended engine type.
func IsExtendedType(t string) bool {
	return ExtendedTypes[t]
}
