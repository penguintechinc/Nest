package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDataResourceDeepCopy verifies deep copy produces independent copies
func TestDataResourceDeepCopy(t *testing.T) {
	original := &DataResource{
		TypeMeta: metav1.TypeMeta{Kind: "DataResource", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dr",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: DataResourceSpec{
			Type:   "postgres",
			Class:  "ssd-class",
			Tenant: "tenant-1",
			Protocols: []Protocol{
				ProtocolNative,
				ProtocolGRPC,
			},
			Size: &ResourceSize{
				Storage: "100Gi",
				IOPS:    1000,
			},
			Annotations: map[string]string{
				"key1": "value1",
			},
		},
		Status: DataResourceStatus{
			Phase: PhaseReady,
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	copy := original.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy returned nil")
	}

	// Verify fields are copied
	if copy.Name != original.Name {
		t.Errorf("Name mismatch: got %s, want %s", copy.Name, original.Name)
	}

	// Modify copy and verify original is unchanged
	copy.Name = "modified-dr"
	copy.Spec.Type = "mysql"
	copy.Spec.Protocols[0] = ProtocolREST
	copy.Spec.Annotations["key1"] = "modified"
	copy.Status.Phase = PhaseFailed

	if original.Name != "test-dr" {
		t.Error("Original Name was modified by copy modification")
	}
	if original.Spec.Type != "postgres" {
		t.Error("Original Spec.Type was modified by copy modification")
	}
	if original.Spec.Protocols[0] != ProtocolNative {
		t.Error("Original Spec.Protocols[0] was modified by copy modification")
	}
	if original.Spec.Annotations["key1"] != "value1" {
		t.Error("Original Spec.Annotations was modified by copy modification")
	}
	if original.Status.Phase != PhaseReady {
		t.Error("Original Status.Phase was modified by copy modification")
	}
}

// TestDataResourceDeepCopyInto verifies DeepCopyInto produces independent copies
func TestDataResourceDeepCopyInto(t *testing.T) {
	original := &DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "dr-1", Namespace: "ns1"},
		Spec: DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-1",
		},
	}

	copy := &DataResource{}
	original.DeepCopyInto(copy)

	copy.Name = "dr-2"
	copy.Spec.Type = "mysql"

	if original.Name != "dr-1" {
		t.Error("Original was modified")
	}
	if original.Spec.Type != "postgres" {
		t.Error("Original Spec was modified")
	}
}

// TestDataResourceDeepCopyObject verifies DeepCopyObject returns runtime.Object
func TestDataResourceDeepCopyObject(t *testing.T) {
	original := &DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr"},
		Spec:       DataResourceSpec{Type: "postgres"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataResource)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataResource")
	}

	copy.Name = "modified"

	if original.Name != "test-dr" {
		t.Error("Original was modified")
	}
}

// TestDataResourceListDeepCopy verifies list deep copy
func TestDataResourceListDeepCopy(t *testing.T) {
	original := &DataResourceList{
		TypeMeta: metav1.TypeMeta{Kind: "DataResourceList", APIVersion: "v1"},
		Items: []DataResource{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "dr-1"},
				Spec:       DataResourceSpec{Type: "postgres"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "dr-2"},
				Spec:       DataResourceSpec{Type: "mysql"},
			},
		},
	}

	copy := original.DeepCopy()

	if len(copy.Items) != len(original.Items) {
		t.Fatalf("Items length mismatch: got %d, want %d", len(copy.Items), len(original.Items))
	}

	copy.Items[0].Name = "modified"
	copy.Items = append(copy.Items, DataResource{})

	if original.Items[0].Name != "dr-1" {
		t.Error("Original item was modified")
	}
	if len(original.Items) != 2 {
		t.Error("Original Items slice was modified")
	}
}

// TestDataResourceClassDeepCopy verifies class deep copy
func TestDataResourceClassDeepCopy(t *testing.T) {
	original := &DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "ssd-class"},
		Spec: ClassSpec{
			Backend: "postgres",
			Placement: &PlacementSpec{
				Prefer: []string{"nvme-hot", "ssd-warm"},
				Allow:  []string{"ssd-warm"},
				Forbid: []string{"tape"},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Placement.Prefer[0] = "modified"
	copy.Spec.Placement.Allow = append(copy.Spec.Placement.Allow, "sata-bulk")

	if original.Spec.Placement.Prefer[0] != "nvme-hot" {
		t.Error("Original Placement.Prefer was modified")
	}
	if len(original.Spec.Placement.Allow) != 1 {
		t.Error("Original Placement.Allow slice was modified")
	}
}

// TestHardwarePoolDeepCopy verifies pool deep copy
func TestHardwarePoolDeepCopy(t *testing.T) {
	original := &HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
		Spec: HardwarePoolSpec{
			Class: "ssd-warm",
			Nodes: []string{"node-1", "node-2"},
			NodeSelector: map[string]string{
				"type": "ssd",
			},
		},
		Status: HardwarePoolStatus{
			CapacityBytes: 1000,
			UsedBytes:     500,
			FreeBytes:     500,
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Nodes[0] = "modified-node"
	copy.Spec.NodeSelector["type"] = "modified"
	copy.Status.FreeBytes = 0

	if original.Spec.Nodes[0] != "node-1" {
		t.Error("Original Nodes was modified")
	}
	if original.Spec.NodeSelector["type"] != "ssd" {
		t.Error("Original NodeSelector was modified")
	}
	if original.Status.FreeBytes != 500 {
		t.Error("Original Status was modified")
	}
}

// TestTenantDeepCopy verifies tenant deep copy
func TestTenantDeepCopy(t *testing.T) {
	original := &Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-1"},
		Spec: TenantSpec{
			DisplayName: "Tenant One",
		},
		Status: TenantStatus{
			Conditions: []metav1.Condition{
				{Type: "Active", Status: metav1.ConditionTrue},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Spec.DisplayName = "Tenant One Modified"
	copy.Status.Conditions[0].Status = metav1.ConditionFalse

	if original.Spec.DisplayName != "Tenant One" {
		t.Error("Original Spec was modified")
	}
	if original.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Error("Original Status.Conditions was modified")
	}
}

// TestCredentialDeepCopy verifies credential deep copy
func TestCredentialDeepCopy(t *testing.T) {
	original := &Credential{
		ObjectMeta: metav1.ObjectMeta{Name: "cred-1"},
		Spec: CredentialSpec{
			Resource:  "dr-1",
			Type:      "db-user",
			SecretRef: "secret-1",
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Type = "s3-key"

	if original.Spec.Type != "db-user" {
		t.Error("Original Spec was modified")
	}
}

// TestDataProtectionPolicyDeepCopy verifies policy deep copy
func TestDataProtectionPolicyDeepCopy(t *testing.T) {
	original := &DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-1"},
		Spec: DataProtectionPolicySpec{
			Backups: &BackupConfig{
				Schedule: "0 2 * * *",
				Retention: &RetentionPolicy{
					Daily:   30,
					Monthly: 12,
				},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Backups.Retention.Daily = 60

	if original.Spec.Backups.Retention.Daily != 30 {
		t.Error("Original Spec.Backups was modified")
	}
}

// TestSchemaDeepCopy verifies schema deep copy
func TestSchemaDeepCopy(t *testing.T) {
	original := &Schema{
		ObjectMeta: metav1.ObjectMeta{Name: "schema-1"},
		Spec: SchemaSpec{
			Fields: []SchemaField{
				{Name: "id", Type: "string"},
				{Name: "value", Type: "int"},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Fields[0].Type = "modified"
	copy.Spec.Fields = append(copy.Spec.Fields, SchemaField{})

	if original.Spec.Fields[0].Type != "string" {
		t.Error("Original Spec.Fields was modified")
	}
	if len(original.Spec.Fields) != 2 {
		t.Error("Original Spec.Fields slice was modified")
	}
}

// TestOperationDeepCopy verifies operation deep copy
func TestOperationDeepCopy(t *testing.T) {
	original := &Operation{
		ObjectMeta: metav1.ObjectMeta{Name: "op-1"},
		Spec: OperationSpec{
			Tenant:        "tenant-1",
			OperationType: "backup",
			ResourceRef:   "dr-1",
		},
		Status: OperationStatus{
			Phase:   OperationPhaseRunning,
			Message: "In progress",
		},
	}

	copy := original.DeepCopy()

	copy.Spec.OperationType = "restore"
	copy.Status.Phase = OperationPhaseSucceeded

	if original.Spec.OperationType != "backup" {
		t.Error("Original Spec was modified")
	}
	if original.Status.Phase != OperationPhaseRunning {
		t.Error("Original Status was modified")
	}
}

// TestWebhookSubscriptionDeepCopy verifies webhook subscription deep copy
func TestWebhookSubscriptionDeepCopy(t *testing.T) {
	original := &WebhookSubscription{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-1"},
		Spec: WebhookSubscriptionSpec{
			URL: "https://example.com/webhook",
		},
	}

	copy := original.DeepCopy()

	copy.Spec.URL = "https://modified.com/webhook"

	if original.Spec.URL != "https://example.com/webhook" {
		t.Error("Original Spec was modified")
	}
}

// TestHardwareInventoryDeepCopy verifies hardware inventory deep copy
func TestHardwareInventoryDeepCopy(t *testing.T) {
	original := &HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "inv-1"},
		Spec: HardwareInventorySpec{
			Node: "node-1",
			Devices: []DeviceSpec{
				{Name: "/dev/nvme0n1", CapacityBytes: 1000000000000},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Spec.Devices[0].CapacityBytes = 2000000000000

	if original.Spec.Devices[0].CapacityBytes != 1000000000000 {
		t.Error("Original Spec.Devices was modified")
	}
}

// TestHardwarePoolListDeepCopy verifies pool list deep copy
func TestHardwarePoolListDeepCopy(t *testing.T) {
	original := &HardwarePoolList{
		Items: []HardwarePool{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
				Spec:       HardwarePoolSpec{Class: "ssd-warm"},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Spec.Class = "nvme-hot"

	if original.Items[0].Spec.Class != "ssd-warm" {
		t.Error("Original Items was modified")
	}
}

// TestTenantListDeepCopy verifies tenant list deep copy
func TestTenantListDeepCopy(t *testing.T) {
	original := &TenantList{
		Items: []Tenant{
			{ObjectMeta: metav1.ObjectMeta{Name: "tenant-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "tenant-modified"

	if original.Items[0].Name != "tenant-1" {
		t.Error("Original Items was modified")
	}
}

// TestNilDeepCopy verifies nil pointers are handled
func TestNilDeepCopy(t *testing.T) {
	var original *DataResource = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil should return nil")
	}
}

// TestNilDeepCopyObject verifies nil DeepCopyObject returns nil
func TestNilDeepCopyObject(t *testing.T) {
	var original *DataResource = nil
	obj := original.DeepCopyObject()

	if obj != nil {
		t.Error("DeepCopyObject of nil should return nil")
	}
}

// TestEmptySpecDeepCopy verifies empty specs are properly copied
func TestEmptySpecDeepCopy(t *testing.T) {
	original := &DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "dr-empty"},
		Spec:       DataResourceSpec{},
	}

	copy := original.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy returned nil")
	}

	copy.Name = "modified"

	if original.Name != "dr-empty" {
		t.Error("Original was modified")
	}
}

// TestReplicaConfigDeepCopy verifies replica config deep copy
func TestReplicaConfigDeepCopy(t *testing.T) {
	original := &ReplicaConfig{
		Write: &ReplicaCountSpec{Min: 1, Max: 3, Default: 2, Count: 2},
		Read:  &ReplicaCountSpec{Min: 2, Max: 5, Default: 3, Count: 3},
	}

	copy := &ReplicaConfig{}
	original.DeepCopyInto(copy)

	copy.Write.Count = 5
	copy.Read.Count = 10

	if original.Write.Count != 2 {
		t.Error("Original Write was modified")
	}
	if original.Read.Count != 3 {
		t.Error("Original Read was modified")
	}
}

// TestPlacementSpecDeepCopy verifies placement spec deep copy
func TestPlacementSpecDeepCopy(t *testing.T) {
	original := &PlacementSpec{
		Prefer: []string{"nvme-hot"},
		Allow:  []string{"ssd-warm", "sata-bulk"},
		Forbid: []string{"tape"},
	}

	copy := &PlacementSpec{}
	original.DeepCopyInto(copy)

	copy.Prefer[0] = "modified"
	copy.Allow = append(copy.Allow, "new")
	copy.Forbid = nil

	if original.Prefer[0] != "nvme-hot" {
		t.Error("Original Prefer was modified")
	}
	if len(original.Allow) != 2 {
		t.Error("Original Allow slice was modified")
	}
	if original.Forbid == nil {
		t.Error("Original Forbid was modified to nil")
	}
}

// TestClassSpecDeepCopy verifies class spec deep copy with nested structures
func TestClassSpecDeepCopy(t *testing.T) {
	original := &ClassSpec{
		Backend: "postgres",
		Placement: &PlacementSpec{
			Prefer: []string{"nvme-hot"},
		},
		Replicas: &ReplicaConfig{
			Write: &ReplicaCountSpec{Min: 1, Max: 3},
		},
	}

	copy := &ClassSpec{}
	original.DeepCopyInto(copy)

	copy.Backend = "mysql"
	copy.Placement.Prefer[0] = "modified"
	copy.Replicas.Write.Min = 5

	if original.Backend != "postgres" {
		t.Error("Original Backend was modified")
	}
	if original.Placement.Prefer[0] != "nvme-hot" {
		t.Error("Original Placement was modified")
	}
	if original.Replicas.Write.Min != 1 {
		t.Error("Original Replicas was modified")
	}
}

// TestDataResourceListDeepCopyObject verifies list DeepCopyObject
func TestDataResourceListDeepCopyObject(t *testing.T) {
	original := &DataResourceList{
		Items: []DataResource{
			{ObjectMeta: metav1.ObjectMeta{Name: "dr-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataResourceList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataResourceList")
	}

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "dr-1" {
		t.Error("Original was modified")
	}
}

// TestCredentialListDeepCopy verifies credential list deep copy
func TestCredentialListDeepCopy(t *testing.T) {
	original := &CredentialList{
		Items: []Credential{
			{ObjectMeta: metav1.ObjectMeta{Name: "cred-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "cred-1" {
		t.Error("Original Items was modified")
	}
}

// TestDataProtectionPolicyListDeepCopy verifies policy list deep copy
func TestDataProtectionPolicyListDeepCopy(t *testing.T) {
	original := &DataProtectionPolicyList{
		Items: []DataProtectionPolicy{
			{ObjectMeta: metav1.ObjectMeta{Name: "policy-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "policy-1" {
		t.Error("Original Items was modified")
	}
}

// TestSchemaListDeepCopy verifies schema list deep copy
func TestSchemaListDeepCopy(t *testing.T) {
	original := &SchemaList{
		Items: []Schema{
			{ObjectMeta: metav1.ObjectMeta{Name: "schema-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "schema-1" {
		t.Error("Original Items was modified")
	}
}

// TestOperationListDeepCopy verifies operation list deep copy
func TestOperationListDeepCopy(t *testing.T) {
	original := &OperationList{
		Items: []Operation{
			{ObjectMeta: metav1.ObjectMeta{Name: "op-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "op-1" {
		t.Error("Original Items was modified")
	}
}

// TestWebhookSubscriptionListDeepCopy verifies webhook subscription list deep copy
func TestWebhookSubscriptionListDeepCopy(t *testing.T) {
	original := &WebhookSubscriptionList{
		Items: []WebhookSubscription{
			{ObjectMeta: metav1.ObjectMeta{Name: "webhook-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "webhook-1" {
		t.Error("Original Items was modified")
	}
}

// TestHardwareInventoryListDeepCopy verifies hardware inventory list deep copy
func TestHardwareInventoryListDeepCopy(t *testing.T) {
	original := &HardwareInventoryList{
		Items: []HardwareInventory{
			{ObjectMeta: metav1.ObjectMeta{Name: "inv-1"}},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Name = "modified"

	if original.Items[0].Name != "inv-1" {
		t.Error("Original Items was modified")
	}
}

// TestDataResourceSpecWithNilPointers verifies nil pointer fields are properly handled
func TestDataResourceSpecWithNilPointers(t *testing.T) {
	original := &DataResourceSpec{
		Type:           "postgres",
		Size:           nil,
		TLS:            nil,
		SecretsBackend: nil,
		Replicas:       nil,
	}

	copy := &DataResourceSpec{}
	original.DeepCopyInto(copy)

	if copy.Size != nil {
		t.Error("Nil Size should remain nil")
	}
	if copy.TLS != nil {
		t.Error("Nil TLS should remain nil")
	}
	if copy.SecretsBackend != nil {
		t.Error("Nil SecretsBackend should remain nil")
	}
	if copy.Replicas != nil {
		t.Error("Nil Replicas should remain nil")
	}
}

// TestDataResourceStatusDeepCopy verifies status deep copy
func TestDataResourceStatusDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &DataResourceStatus{
		Phase:            PhaseReady,
		Conditions:       []metav1.Condition{{Type: "Ready"}},
		Endpoints:        &ResourceEndpoints{Native: "postgres://host:5432"},
		Health:           &HealthSignal{State: HealthHealthy},
		CurrentOperation: "none",
		ProvisionedAt:    &now,
	}

	copy := &DataResourceStatus{}
	original.DeepCopyInto(copy)

	copy.Phase = PhaseFailed
	copy.Conditions[0].Type = "Modified"
	copy.Endpoints.Native = "modified"
	copy.Health.State = HealthDown

	if original.Phase != PhaseReady {
		t.Error("Original Phase was modified")
	}
	if original.Conditions[0].Type != "Ready" {
		t.Error("Original Conditions was modified")
	}
	if original.Endpoints.Native != "postgres://host:5432" {
		t.Error("Original Endpoints was modified")
	}
	if original.Health.State != HealthHealthy {
		t.Error("Original Health was modified")
	}
}

// TestHardwarePoolStatusDeepCopy verifies pool status deep copy
func TestHardwarePoolStatusDeepCopy(t *testing.T) {
	original := &HardwarePoolStatus{
		CapacityBytes: 1000,
		UsedBytes:     500,
		FreeBytes:     500,
		DriveCount:    10,
		Conditions:    []metav1.Condition{{Type: "Ready"}},
	}

	copy := &HardwarePoolStatus{}
	original.DeepCopyInto(copy)

	copy.FreeBytes = 0
	copy.Conditions[0].Type = "Modified"

	if original.FreeBytes != 500 {
		t.Error("Original FreeBytes was modified")
	}
	if original.Conditions[0].Type != "Ready" {
		t.Error("Original Conditions was modified")
	}
}

// TestDataResourceClassListDeepCopy verifies class list deep copy
func TestDataResourceClassListDeepCopy(t *testing.T) {
	original := &DataResourceClassList{
		Items: []DataResourceClass{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "class-1"},
				Spec: ClassSpec{Backend: "postgres"},
			},
		},
	}

	copy := original.DeepCopy()

	copy.Items[0].Spec.Backend = "mysql"

	if original.Items[0].Spec.Backend != "postgres" {
		t.Error("Original Items was modified")
	}
}

// TestHardwarePoolDeepCopyObject verifies pool DeepCopyObject
func TestHardwarePoolDeepCopyObject(t *testing.T) {
	original := &HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
		Spec:       HardwarePoolSpec{Class: "ssd-warm"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*HardwarePool)
	if !ok {
		t.Fatal("DeepCopyObject did not return *HardwarePool")
	}

	copy.Spec.Class = "nvme-hot"

	if original.Spec.Class != "ssd-warm" {
		t.Error("Original was modified")
	}
}

// TestTenantDeepCopyObject verifies tenant DeepCopyObject
func TestTenantDeepCopyObject(t *testing.T) {
	original := &Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*Tenant)
	if !ok {
		t.Fatal("DeepCopyObject did not return *Tenant")
	}

	copy.Name = "tenant-2"

	if original.Name != "tenant-1" {
		t.Error("Original was modified")
	}
}

// TestDataResourceClassDeepCopyObject verifies class DeepCopyObject
func TestDataResourceClassDeepCopyObject(t *testing.T) {
	original := &DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataResourceClass)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataResourceClass")
	}

	copy.Name = "class-2"

	if original.Name != "class-1" {
		t.Error("Original was modified")
	}
}

// TestHardwareInventoryDeepCopyObject verifies inventory DeepCopyObject
func TestHardwareInventoryDeepCopyObject(t *testing.T) {
	original := &HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "inv-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*HardwareInventory)
	if !ok {
		t.Fatal("DeepCopyObject did not return *HardwareInventory")
	}

	copy.Name = "inv-2"

	if original.Name != "inv-1" {
		t.Error("Original was modified")
	}
}

// TestCredentialDeepCopyObject verifies credential DeepCopyObject
func TestCredentialDeepCopyObject(t *testing.T) {
	original := &Credential{
		ObjectMeta: metav1.ObjectMeta{Name: "cred-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*Credential)
	if !ok {
		t.Fatal("DeepCopyObject did not return *Credential")
	}

	copy.Name = "cred-2"

	if original.Name != "cred-1" {
		t.Error("Original was modified")
	}
}

// TestDataProtectionPolicyDeepCopyObject verifies policy DeepCopyObject
func TestDataProtectionPolicyDeepCopyObject(t *testing.T) {
	original := &DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataProtectionPolicy)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataProtectionPolicy")
	}

	copy.Name = "policy-2"

	if original.Name != "policy-1" {
		t.Error("Original was modified")
	}
}

// TestSchemaDeepCopyObject verifies schema DeepCopyObject
func TestSchemaDeepCopyObject(t *testing.T) {
	original := &Schema{
		ObjectMeta: metav1.ObjectMeta{Name: "schema-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*Schema)
	if !ok {
		t.Fatal("DeepCopyObject did not return *Schema")
	}

	copy.Name = "schema-2"

	if original.Name != "schema-1" {
		t.Error("Original was modified")
	}
}

// TestWebhookSubscriptionDeepCopyObject verifies webhook DeepCopyObject
func TestWebhookSubscriptionDeepCopyObject(t *testing.T) {
	original := &WebhookSubscription{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*WebhookSubscription)
	if !ok {
		t.Fatal("DeepCopyObject did not return *WebhookSubscription")
	}

	copy.Name = "webhook-2"

	if original.Name != "webhook-1" {
		t.Error("Original was modified")
	}
}

// TestOperationDeepCopyObject verifies operation DeepCopyObject
func TestOperationDeepCopyObject(t *testing.T) {
	original := &Operation{
		ObjectMeta: metav1.ObjectMeta{Name: "op-1"},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*Operation)
	if !ok {
		t.Fatal("DeepCopyObject did not return *Operation")
	}

	copy.Name = "op-2"

	if original.Name != "op-1" {
		t.Error("Original was modified")
	}
}

// TestTenantSpecDeepCopy verifies tenant spec deep copy
func TestTenantSpecDeepCopy(t *testing.T) {
	original := &TenantSpec{
		DisplayName: "Tenant One",
		Quota: &QuotaSpec{
			MaxStorageBytes: 1000000,
		},
	}

	copy := &TenantSpec{}
	original.DeepCopyInto(copy)

	copy.DisplayName = "Tenant Two"
	copy.Quota.MaxStorageBytes = 2000000

	if original.DisplayName != "Tenant One" {
		t.Error("Original DisplayName was modified")
	}
	if original.Quota.MaxStorageBytes != 1000000 {
		t.Error("Original Quota was modified")
	}
}

// TestClassSpecWithNilFields verifies class spec deep copy with nil fields
func TestClassSpecWithNilFields(t *testing.T) {
	original := &ClassSpec{
		Backend:      "postgres",
		Placement:    nil,
		Replication:  nil,
		Encryption:   nil,
		SLO:          nil,
		Cache:        nil,
		Replicas:     nil,
	}

	copy := &ClassSpec{}
	original.DeepCopyInto(copy)

	if copy.Placement != nil {
		t.Error("Nil Placement should remain nil")
	}
	if copy.Replication != nil {
		t.Error("Nil Replication should remain nil")
	}
	if copy.SLO != nil {
		t.Error("Nil SLO should remain nil")
	}
}

// TestDeviceSpecDeepCopy verifies device spec deep copy
func TestDeviceSpecDeepCopy(t *testing.T) {
	original := &DeviceSpec{
		Name:          "/dev/nvme0n1",
		CapacityBytes: 1000000000000,
		State:         DeviceStateActive,
		SMART: &SMARTData{
			WearPercent: 10,
		},
	}

	copy := &DeviceSpec{}
	original.DeepCopyInto(copy)

	copy.Name = "/dev/nvme1n1"
	copy.SMART.WearPercent = 20

	if original.Name != "/dev/nvme0n1" {
		t.Error("Original Name was modified")
	}
	if original.SMART.WearPercent != 10 {
		t.Error("Original SMART was modified")
	}
}

// TestSchemaSpecDeepCopy verifies schema spec deep copy
func TestSchemaSpecDeepCopy(t *testing.T) {
	original := &SchemaSpec{
		Fields: []SchemaField{
			{Name: "id", Type: "string"},
			{Name: "value", Type: "int"},
		},
		Indexes: []SchemaIndex{
			{Name: "idx_id", Fields: []string{"id"}},
		},
	}

	copy := &SchemaSpec{}
	original.DeepCopyInto(copy)

	copy.Fields[0].Type = "modified"
	copy.Indexes[0].Name = "modified"

	if original.Fields[0].Type != "string" {
		t.Error("Original Fields was modified")
	}
	if original.Indexes[0].Name != "idx_id" {
		t.Error("Original Indexes was modified")
	}
}

// TestSchemaFieldDeepCopy verifies schema field deep copy
func TestSchemaFieldDeepCopy(t *testing.T) {
	original := &SchemaField{
		Name:       "column1",
		Type:       "string",
		Required:   true,
		PrimaryKey: false,
		Labels:     []string{"pii.email"},
	}

	copy := &SchemaField{}
	original.DeepCopyInto(copy)

	copy.Type = "int"
	copy.Labels[0] = "modified"

	if original.Type != "string" {
		t.Error("Original Type was modified")
	}
	if original.Labels[0] != "pii.email" {
		t.Error("Original Labels was modified")
	}
}

// TestSchemaIndexDeepCopy verifies schema index deep copy
func TestSchemaIndexDeepCopy(t *testing.T) {
	original := &SchemaIndex{
		Name:   "idx_primary",
		Fields: []string{"id"},
		Unique: true,
	}

	copy := &SchemaIndex{}
	original.DeepCopyInto(copy)

	copy.Fields[0] = "modified"
	copy.Unique = false

	if original.Fields[0] != "id" {
		t.Error("Original Fields was modified")
	}
	if original.Unique != true {
		t.Error("Original Unique was modified")
	}
}

// TestWebhookSubscriptionSpecDeepCopy verifies webhook spec deep copy
func TestWebhookSubscriptionSpecDeepCopy(t *testing.T) {
	original := &WebhookSubscriptionSpec{
		URL:       "https://example.com/hook",
		Tenant:    "tenant-1",
		Events:    []string{"created", "updated"},
		SecretRef: "secret-1",
	}

	copy := &WebhookSubscriptionSpec{}
	original.DeepCopyInto(copy)

	copy.URL = "https://modified.com/hook"
	copy.Events[0] = "deleted"

	if original.URL != "https://example.com/hook" {
		t.Error("Original URL was modified")
	}
	if original.Events[0] != "created" {
		t.Error("Original Events was modified")
	}
}

// TestWebhookSubscriptionStatusDeepCopy verifies webhook status deep copy
func TestWebhookSubscriptionStatusDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &WebhookSubscriptionStatus{
		State:           "active",
		LastDeliveredAt: &now,
		FailureCount:    0,
	}

	copy := &WebhookSubscriptionStatus{}
	original.DeepCopyInto(copy)

	copy.State = "suspended"
	copy.FailureCount = 1

	if original.State != "active" {
		t.Error("Original State was modified")
	}
	if original.FailureCount != 0 {
		t.Error("Original FailureCount was modified")
	}
}

// TestDataResourceSpecDeepCopyWithAllFields verifies all fields are properly deep copied
func TestDataResourceSpecDeepCopyWithAllFields(t *testing.T) {
	original := &DataResourceSpec{
		Type:   "postgres",
		Class:  "ssd-class",
		Tenant: "tenant-1",
		Protocols: []Protocol{ProtocolNative, ProtocolGRPC},
		Size: &ResourceSize{Storage: "100Gi", IOPS: 1000},
		TLS: &TLSConfig{Mode: "required", MinVersion: "1.3"},
		SecretsBackend: &SecretsBackendRef{Kind: "vault"},
		Annotations: map[string]string{"key": "value"},
		Replicas: &ReplicaConfig{
			Write: &ReplicaCountSpec{Min: 1, Max: 3},
			Read:  &ReplicaCountSpec{Min: 2, Max: 5},
		},
	}

	copy := &DataResourceSpec{}
	original.DeepCopyInto(copy)

	// Modify all fields in copy
	copy.Type = "mysql"
	copy.Protocols[0] = ProtocolREST
	copy.Size.Storage = "200Gi"
	copy.TLS.Mode = "disabled"
	copy.SecretsBackend.Kind = "aws-sm"
	copy.Annotations["key"] = "modified"
	copy.Replicas.Write.Min = 5

	if original.Type != "postgres" {
		t.Error("Original Type was modified")
	}
	if original.Protocols[0] != ProtocolNative {
		t.Error("Original Protocols was modified")
	}
	if original.Size.Storage != "100Gi" {
		t.Error("Original Size was modified")
	}
	if original.TLS.Mode != "required" {
		t.Error("Original TLS was modified")
	}
	if original.SecretsBackend.Kind != "vault" {
		t.Error("Original SecretsBackend was modified")
	}
	if original.Annotations["key"] != "value" {
		t.Error("Original Annotations was modified")
	}
	if original.Replicas.Write.Min != 1 {
		t.Error("Original Replicas was modified")
	}
}

// TestHardwareInventorySpecDeepCopy verifies inventory spec deep copy
func TestHardwareInventorySpecDeepCopy(t *testing.T) {
	original := &HardwareInventorySpec{
		Node: "node-1",
		Devices: []DeviceSpec{
			{
				Name:          "/dev/nvme0n1",
				CapacityBytes: 1000000000000,
				SMART: &SMARTData{WearPercent: 10},
			},
		},
	}

	copy := &HardwareInventorySpec{}
	original.DeepCopyInto(copy)

	copy.Node = "node-2"
	copy.Devices[0].SMART.WearPercent = 50

	if original.Node != "node-1" {
		t.Error("Original Node was modified")
	}
	if original.Devices[0].SMART.WearPercent != 10 {
		t.Error("Original Devices was modified")
	}
}

// TestHardwareInventoryStatusDeepCopy verifies inventory status deep copy
func TestHardwareInventoryStatusDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &HardwareInventoryStatus{
		LastScanTime:   &now,
		DarkDriveCount: 5,
		ActiveDriveCount: 10,
		Conditions:     []metav1.Condition{{Type: "Ready"}},
	}

	copy := &HardwareInventoryStatus{}
	original.DeepCopyInto(copy)

	copy.DarkDriveCount = 0
	copy.Conditions[0].Type = "Modified"

	if original.DarkDriveCount != 5 {
		t.Error("Original DarkDriveCount was modified")
	}
	if original.Conditions[0].Type != "Ready" {
		t.Error("Original Conditions was modified")
	}
}

// TestOperationStatusDeepCopy verifies operation status deep copy
func TestOperationStatusDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &OperationStatus{
		Phase:      OperationPhaseRunning,
		StartedAt:  &now,
		Message:    "In progress",
		Result:     "none",
		Conditions: []metav1.Condition{{Type: "Running"}},
	}

	copy := &OperationStatus{}
	original.DeepCopyInto(copy)

	copy.Phase = OperationPhaseSucceeded
	copy.Message = "Completed"

	if original.Phase != OperationPhaseRunning {
		t.Error("Original Phase was modified")
	}
	if original.Message != "In progress" {
		t.Error("Original Message was modified")
	}
}

// TestCredentialSpecDeepCopy verifies credential spec deep copy
func TestCredentialSpecDeepCopy(t *testing.T) {
	original := &CredentialSpec{
		Resource:       "dr-1",
		Tenant:         "tenant-1",
		Type:           "db-user",
		Username:       "admin",
		SecretRef:      "secret-1",
		RotationPolicy: &RotationPolicy{IntervalDays: 30},
	}

	copy := &CredentialSpec{}
	original.DeepCopyInto(copy)

	copy.Type = "s3-key"
	copy.RotationPolicy.IntervalDays = 90

	if original.Type != "db-user" {
		t.Error("Original Type was modified")
	}
	if original.RotationPolicy.IntervalDays != 30 {
		t.Error("Original RotationPolicy was modified")
	}
}

// TestDataProtectionPolicySpecDeepCopy verifies policy spec deep copy
func TestDataProtectionPolicySpecDeepCopy(t *testing.T) {
	original := &DataProtectionPolicySpec{
		Snapshots: &SnapshotConfig{
			Schedule: "0 2 * * *",
			Retention: &RetentionPolicy{Daily: 7},
		},
		Backups: &BackupConfig{
			Schedule: "0 3 * * *",
			Retention: &RetentionPolicy{Monthly: 12},
		},
		PITR: &PITRConfig{
			Enabled:    true,
			WindowDays: 7,
		},
		Verify: &VerifyConfig{
			RestoreTest: &RestoreTestConfig{Schedule: "0 4 * * 0"},
		},
	}

	copy := &DataProtectionPolicySpec{}
	original.DeepCopyInto(copy)

	copy.Snapshots.Retention.Daily = 30
	copy.Backups.Retention.Monthly = 24
	copy.PITR.WindowDays = 30

	if original.Snapshots.Retention.Daily != 7 {
		t.Error("Original Snapshots was modified")
	}
	if original.Backups.Retention.Monthly != 12 {
		t.Error("Original Backups was modified")
	}
	if original.PITR.WindowDays != 7 {
		t.Error("Original PITR was modified")
	}
}

// TestSnapshotConfigDeepCopy verifies snapshot config deep copy
func TestSnapshotConfigDeepCopy(t *testing.T) {
	original := &SnapshotConfig{
		Schedule: "0 2 * * *",
		Retention: &RetentionPolicy{Daily: 7, Monthly: 12},
	}

	copy := &SnapshotConfig{}
	original.DeepCopyInto(copy)

	copy.Retention.Daily = 30

	if original.Retention.Daily != 7 {
		t.Error("Original Retention was modified")
	}
}

// TestBackupConfigDeepCopy verifies backup config deep copy
func TestBackupConfigDeepCopy(t *testing.T) {
	original := &BackupConfig{
		Schedule: "0 3 * * *",
		Destination: &BackupDestination{
			Kind:     "object",
			Resource: "backup-bucket",
		},
		Retention: &RetentionPolicy{Monthly: 12},
	}

	copy := &BackupConfig{}
	original.DeepCopyInto(copy)

	copy.Schedule = "0 4 * * *"
	copy.Destination.Resource = "modified"

	if original.Schedule != "0 3 * * *" {
		t.Error("Original Schedule was modified")
	}
	if original.Destination.Resource != "backup-bucket" {
		t.Error("Original Destination was modified")
	}
}

// TestSchemaStatusDeepCopy verifies schema status deep copy
func TestSchemaStatusDeepCopy(t *testing.T) {
	original := &SchemaStatus{
		AppliedVersion: 1,
		TargetVersion:  2,
		Drift:          "detected",
		Conditions:     []metav1.Condition{{Type: "Ready"}},
	}

	copy := &SchemaStatus{}
	original.DeepCopyInto(copy)

	copy.AppliedVersion = 2
	copy.Conditions[0].Type = "Modified"

	if original.AppliedVersion != 1 {
		t.Error("Original AppliedVersion was modified")
	}
	if original.Conditions[0].Type != "Ready" {
		t.Error("Original Conditions was modified")
	}
}

// TestTenantStatusDeepCopy verifies tenant status deep copy
func TestTenantStatusDeepCopy(t *testing.T) {
	original := &TenantStatus{
		DataResourceCount:    5,
		OperatorAccountCount: 2,
		ResourceAccountCount: 3,
		Conditions:           []metav1.Condition{{Type: "Active"}},
	}

	copy := &TenantStatus{}
	original.DeepCopyInto(copy)

	copy.DataResourceCount = 10
	copy.Conditions[0].Type = "Inactive"

	if original.DataResourceCount != 5 {
		t.Error("Original DataResourceCount was modified")
	}
	if original.Conditions[0].Type != "Active" {
		t.Error("Original Conditions was modified")
	}
}

// TestCredentialStatusDeepCopy verifies credential status deep copy
func TestCredentialStatusDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &CredentialStatus{
		LastRotatedAt: &now,
		ExpiresAt:     &now,
		State:         "active",
	}

	copy := &CredentialStatus{}
	original.DeepCopyInto(copy)

	copy.State = "revoked"

	if original.State != "active" {
		t.Error("Original State was modified")
	}
}


// TestHardwarePoolSpecDeepCopy verifies pool spec deep copy
func TestHardwarePoolSpecDeepCopy(t *testing.T) {
	original := &HardwarePoolSpec{
		Class: "ssd-warm",
		Nodes: []string{"node-1", "node-2"},
		NodeSelector: map[string]string{
			"type": "ssd",
		},
	}

	copy := &HardwarePoolSpec{}
	original.DeepCopyInto(copy)

	copy.Class = "nvme-hot"
	copy.Nodes[0] = "modified"
	copy.NodeSelector["type"] = "nvme"

	if original.Class != "ssd-warm" {
		t.Error("Original Class was modified")
	}
	if original.Nodes[0] != "node-1" {
		t.Error("Original Nodes was modified")
	}
	if original.NodeSelector["type"] != "ssd" {
		t.Error("Original NodeSelector was modified")
	}
}


// TestDataResourceClassListDeepCopyObject verifies class list DeepCopyObject
func TestDataResourceClassListDeepCopyObject(t *testing.T) {
	original := &DataResourceClassList{
		Items: []DataResourceClass{
			{ObjectMeta: metav1.ObjectMeta{Name: "class-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataResourceClassList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataResourceClassList")
	}

	copy.Items[0].Name = "class-modified"

	if original.Items[0].Name != "class-1" {
		t.Error("Original was modified")
	}
}

// TestTenantListDeepCopyObject verifies tenant list DeepCopyObject
func TestTenantListDeepCopyObject(t *testing.T) {
	original := &TenantList{
		Items: []Tenant{
			{ObjectMeta: metav1.ObjectMeta{Name: "tenant-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*TenantList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *TenantList")
	}

	copy.Items[0].Name = "tenant-modified"

	if original.Items[0].Name != "tenant-1" {
		t.Error("Original was modified")
	}
}

// TestHardwarePoolListDeepCopyObject verifies pool list DeepCopyObject
func TestHardwarePoolListDeepCopyObject(t *testing.T) {
	original := &HardwarePoolList{
		Items: []HardwarePool{
			{ObjectMeta: metav1.ObjectMeta{Name: "pool-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*HardwarePoolList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *HardwarePoolList")
	}

	copy.Items[0].Name = "pool-modified"

	if original.Items[0].Name != "pool-1" {
		t.Error("Original was modified")
	}
}

// TestHardwareInventoryListDeepCopyObject verifies inventory list DeepCopyObject
func TestHardwareInventoryListDeepCopyObject(t *testing.T) {
	original := &HardwareInventoryList{
		Items: []HardwareInventory{
			{ObjectMeta: metav1.ObjectMeta{Name: "inv-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*HardwareInventoryList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *HardwareInventoryList")
	}

	copy.Items[0].Name = "inv-modified"

	if original.Items[0].Name != "inv-1" {
		t.Error("Original was modified")
	}
}

// TestCredentialListDeepCopyObject verifies credential list DeepCopyObject
func TestCredentialListDeepCopyObject(t *testing.T) {
	original := &CredentialList{
		Items: []Credential{
			{ObjectMeta: metav1.ObjectMeta{Name: "cred-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*CredentialList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *CredentialList")
	}

	copy.Items[0].Name = "cred-modified"

	if original.Items[0].Name != "cred-1" {
		t.Error("Original was modified")
	}
}

// TestDataProtectionPolicyListDeepCopyObject verifies policy list DeepCopyObject
func TestDataProtectionPolicyListDeepCopyObject(t *testing.T) {
	original := &DataProtectionPolicyList{
		Items: []DataProtectionPolicy{
			{ObjectMeta: metav1.ObjectMeta{Name: "policy-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*DataProtectionPolicyList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *DataProtectionPolicyList")
	}

	copy.Items[0].Name = "policy-modified"

	if original.Items[0].Name != "policy-1" {
		t.Error("Original was modified")
	}
}

// TestSchemaListDeepCopyObject verifies schema list DeepCopyObject
func TestSchemaListDeepCopyObject(t *testing.T) {
	original := &SchemaList{
		Items: []Schema{
			{ObjectMeta: metav1.ObjectMeta{Name: "schema-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*SchemaList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *SchemaList")
	}

	copy.Items[0].Name = "schema-modified"

	if original.Items[0].Name != "schema-1" {
		t.Error("Original was modified")
	}
}

// TestWebhookSubscriptionListDeepCopyObject verifies webhook list DeepCopyObject
func TestWebhookSubscriptionListDeepCopyObject(t *testing.T) {
	original := &WebhookSubscriptionList{
		Items: []WebhookSubscription{
			{ObjectMeta: metav1.ObjectMeta{Name: "webhook-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*WebhookSubscriptionList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *WebhookSubscriptionList")
	}

	copy.Items[0].Name = "webhook-modified"

	if original.Items[0].Name != "webhook-1" {
		t.Error("Original was modified")
	}
}

// TestOperationListDeepCopyObject verifies operation list DeepCopyObject
func TestOperationListDeepCopyObject(t *testing.T) {
	original := &OperationList{
		Items: []Operation{
			{ObjectMeta: metav1.ObjectMeta{Name: "op-1"}},
		},
	}

	obj := original.DeepCopyObject()

	copy, ok := obj.(*OperationList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *OperationList")
	}

	copy.Items[0].Name = "op-modified"

	if original.Items[0].Name != "op-1" {
		t.Error("Original was modified")
	}
}

// TestNilDataResourceClassListDeepCopy verifies nil handling
func TestNilDataResourceClassListDeepCopy(t *testing.T) {
	var original *DataResourceClassList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil DataResourceClassList should return nil")
	}
}

// TestNilTenantListDeepCopy verifies nil handling
func TestNilTenantListDeepCopy(t *testing.T) {
	var original *TenantList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil TenantList should return nil")
	}
}

// TestNilHardwarePoolListDeepCopy verifies nil handling
func TestNilHardwarePoolListDeepCopy(t *testing.T) {
	var original *HardwarePoolList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil HardwarePoolList should return nil")
	}
}

// TestNilHardwareInventoryListDeepCopy verifies nil handling
func TestNilHardwareInventoryListDeepCopy(t *testing.T) {
	var original *HardwareInventoryList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil HardwareInventoryList should return nil")
	}
}

// TestNilCredentialListDeepCopy verifies nil handling
func TestNilCredentialListDeepCopy(t *testing.T) {
	var original *CredentialList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil CredentialList should return nil")
	}
}

// TestNilDataProtectionPolicyListDeepCopy verifies nil handling
func TestNilDataProtectionPolicyListDeepCopy(t *testing.T) {
	var original *DataProtectionPolicyList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil DataProtectionPolicyList should return nil")
	}
}

// TestNilSchemaListDeepCopy verifies nil handling
func TestNilSchemaListDeepCopy(t *testing.T) {
	var original *SchemaList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil SchemaList should return nil")
	}
}

// TestNilWebhookSubscriptionListDeepCopy verifies nil handling
func TestNilWebhookSubscriptionListDeepCopy(t *testing.T) {
	var original *WebhookSubscriptionList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil WebhookSubscriptionList should return nil")
	}
}

// TestNilOperationListDeepCopy verifies nil handling
func TestNilOperationListDeepCopy(t *testing.T) {
	var original *OperationList = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil OperationList should return nil")
	}
}

// TestNilHardwarePoolDeepCopy verifies nil handling
func TestNilHardwarePoolDeepCopy(t *testing.T) {
	var original *HardwarePool = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil HardwarePool should return nil")
	}
}

// TestNilHardwareInventoryDeepCopy verifies nil handling
func TestNilHardwareInventoryDeepCopy(t *testing.T) {
	var original *HardwareInventory = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil HardwareInventory should return nil")
	}
}

// TestNilTenantDeepCopy verifies nil handling
func TestNilTenantDeepCopy(t *testing.T) {
	var original *Tenant = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil Tenant should return nil")
	}
}

// TestNilDataResourceClassDeepCopy verifies nil handling
func TestNilDataResourceClassDeepCopy(t *testing.T) {
	var original *DataResourceClass = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil DataResourceClass should return nil")
	}
}

// TestNilCredentialDeepCopy verifies nil handling
func TestNilCredentialDeepCopy(t *testing.T) {
	var original *Credential = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil Credential should return nil")
	}
}

// TestNilDataProtectionPolicyDeepCopy verifies nil handling
func TestNilDataProtectionPolicyDeepCopy(t *testing.T) {
	var original *DataProtectionPolicy = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil DataProtectionPolicy should return nil")
	}
}

// TestNilSchemaDeepCopy verifies nil handling
func TestNilSchemaDeepCopy(t *testing.T) {
	var original *Schema = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil Schema should return nil")
	}
}

// TestNilWebhookSubscriptionDeepCopy verifies nil handling
func TestNilWebhookSubscriptionDeepCopy(t *testing.T) {
	var original *WebhookSubscription = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil WebhookSubscription should return nil")
	}
}

// TestNilOperationDeepCopy verifies nil handling
func TestNilOperationDeepCopy(t *testing.T) {
	var original *Operation = nil
	copy := original.DeepCopy()

	if copy != nil {
		t.Error("DeepCopy of nil Operation should return nil")
	}
}
