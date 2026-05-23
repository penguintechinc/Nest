package v1

// Storage resource type constants for DataResource.Spec.Type
const (
	TypePVCBlock   = "pvc/block"
	TypePVCFile    = "pvc/file"
	TypeFilesystem = "filesystem"
	TypeNFS        = "nfs"
	TypeObject     = "object"
	TypeISCSI      = "iscsi"
	TypePostgres   = "postgres"
	TypeKeyvalue   = "keyvalue"
	TypeKafka      = "kafka"
	TypeSearch     = "search"
	TypeRockFS     = "rockfs"
	TypeVector     = "vector"
	TypeClickhouse = "clickhouse"
	TypeTrino      = "warehouse/trino"
	TypeIceberg    = "lakehouse/iceberg"
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

// WarehouseTypes is the set of analytics warehouse/lakehouse types.
var WarehouseTypes = map[string]bool{
	TypeTrino:   true,
	TypeIceberg: true,
}

// IsWarehouseType returns true if the given type is a warehouse type.
func IsWarehouseType(t string) bool {
	return WarehouseTypes[t]
}

// Cloud-native external storage types (require origination: external)
const (
	TypeEBS          = "ebs"           // AWS EBS volume
	TypeAzureDisk    = "azure-disk"    // Azure Managed Disk
	TypeGCPDisk      = "gcp-disk"      // GCP Persistent Disk
	TypeS3           = "s3"            // AWS S3 bucket
	TypeGCS          = "gcs"           // Google Cloud Storage bucket
	TypeAzureBlob    = "azure-blob"    // Azure Blob Storage container
	TypeDOSpaces     = "do-spaces"     // DigitalOcean Spaces (object)
	TypeDOVolume     = "do-volume"     // DigitalOcean Volumes (block)
	TypeVultrObject  = "vultr-object"  // Vultr Object Storage (object)
	TypeVultrBlock   = "vultr-block"   // Vultr Block Storage (block)
	TypeLinodeObject = "linode-object" // Linode Object Storage (object)
	TypeLinodeBlock  = "linode-block"  // Linode Block Volumes (block)
	TypeS3Compat     = "s3-compat"     // Generic S3-compatible object store
)

// CloudStorageTypes is the set of cloud-native storage types (always external origination).
var CloudStorageTypes = map[string]bool{
	TypeEBS:          true,
	TypeAzureDisk:    true,
	TypeGCPDisk:      true,
	TypeS3:           true,
	TypeGCS:          true,
	TypeAzureBlob:    true,
	TypeDOSpaces:     true,
	TypeDOVolume:     true,
	TypeVultrObject:  true,
	TypeVultrBlock:   true,
	TypeLinodeObject: true,
	TypeLinodeBlock:  true,
	TypeS3Compat:     true,
}

func IsCloudStorageType(t string) bool {
	return CloudStorageTypes[t]
}
