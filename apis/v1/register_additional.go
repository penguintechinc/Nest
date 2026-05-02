package v1

func init() {
	SchemeBuilder.Register(
		&Credential{}, &CredentialList{},
		&DataProtectionPolicy{}, &DataProtectionPolicyList{},
		&Schema{}, &SchemaList{},
		&WebhookSubscription{}, &WebhookSubscriptionList{},
		&Operation{}, &OperationList{},
		&DarkDrive{}, &DarkDriveList{},
		&DataContract{}, &DataContractList{},
		&ResourceLabel{}, &ResourceLabelList{},
		&SearchPool{}, &SearchPoolList{},
	)
}
