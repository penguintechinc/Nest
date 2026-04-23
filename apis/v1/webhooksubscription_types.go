package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type WebhookSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WebhookSubscriptionSpec   `json:"spec,omitempty"`
	Status            WebhookSubscriptionStatus `json:"status,omitempty"`
}

type WebhookSubscriptionSpec struct {
	// URL is the webhook delivery endpoint
	URL string `json:"url"`
	// Tenant is the owning tenant
	Tenant string `json:"tenant"`
	// Events lists subscribed event types
	Events []string `json:"events"`
	// SecretRef references the HMAC signing key secret
	SecretRef string `json:"secretRef,omitempty"`
}

type WebhookSubscriptionStatus struct {
	// LastDeliveredAt is when the last webhook was successfully delivered
	LastDeliveredAt *metav1.Time `json:"lastDeliveredAt,omitempty"`
	// FailureCount consecutive delivery failures
	FailureCount int32 `json:"failureCount,omitempty"`
	// State: active, suspended
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
type WebhookSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookSubscription `json:"items"`
}
