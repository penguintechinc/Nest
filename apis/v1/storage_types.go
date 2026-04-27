package v1

// Storage resource type constants for DataResource.Spec.Type
const (
	TypePVCBlock    = "pvc/block"
	TypePVCFile     = "pvc/file"
	TypeFilesystem  = "filesystem"
	TypeNFS         = "nfs"
	TypeObject      = "object"
	TypeISCSI       = "iscsi"
	TypePostgres    = "postgres"
	TypeKeyvalue    = "keyvalue"
	TypeKafka       = "kafka"
	TypeSearch      = "search"
	TypeRockFS      = "rockfs"
	TypeVector      = "vector"
	TypeClickhouse  = "clickhouse"
	TypeTrino       = "warehouse/trino"
	TypeIceberg     = "lakehouse/iceberg"
)

// StorageTypes is the set of all storage-layer types (non-database types).
var StorageTypes = map[string]bool{
	TypePVCBlock:   true,
	TypePVCFile:    true,
	TypeFilesystem: true,
	TypeNFS:        true,
	TypeObject:     true,
	TypeISCSI:      true,
}

// IsStorageType returns true if the given type is a raw storage type (not a database).
func IsStorageType(t string) bool {
	return StorageTypes[t]
}
