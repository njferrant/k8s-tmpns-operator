/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Important: Run "make" to regenerate code after modifying this file

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceOwner defines the unique user that owns the TempNamespace resource
type ResourceOwner struct {
	Username string `json:"username,omitempty"`
	UID      string `json:"uid,omitempty"`
}

// TODO(nick): Make fields owner, expiresAt, createdAt required fields. Note that doing so
// will break the BeforeEach and AfterEatch functions in the controller/webhook test suites.
// TempNamespaceSpec defines the desired state of TempNamespace
type TempNamespaceSpec struct {
	// owner is the user who created this TempNamespace and the subject granted access to
	// its namespace. A mutating webhook sets it from the authenticated request; any value
	// supplied by the client is overwritten. Service accounts are recorded with kind
	// ServiceAccount, all other users with kind User. Immutable after creation.
	// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="owner is immutable"
	// +optional
	Owner *ResourceOwner `json:"owner,omitempty"`

	// duration is how long the namespace should live, counted from creation. It is a number
	// followed by a unit: h (hours), d (days), w (weeks) or m (months). For example "4h",
	// "7d", "2w" or "3m". Required when creating a TempNamespace.
	//
	// This field is input only. When the object is created, a mutating webhook converts it
	// into expiresAt and removes it, so it does not appear on stored objects. To change the
	// lifetime afterwards, edit expiresAt; a duration sent on update is ignored with a warning.
	//
	// +kubebuilder:validation:Pattern=^[0-9]([0-9][0-9]?)?[hwdm]$
	// +kubebuilder:validation:MaxLength=4
	// +optional
	Duration string `json:"duration,omitempty"`

	// createdAt is the date time the TempNamespace resource was created, in RFC 3339 date-time
	// format. Set by a mutating webook during resource creation.
	// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="createdAt is immutable"
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// expiresAt is the date-time the TempNamespace resource expires, in RFC 3339 date-time
	// format. When the resource expires, the controller deletes the namespace and all resources
	// in it. Initially set by a mutating webhook during resource creation, the owner may UPDATE
	// this field to extend or reduce date-time that the TempNamespace resource expires.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// TempNamespaceStatus defines the observed state of TempNamespace.
type TempNamespaceStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// namespace is the name of the namespace created for this resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// conditions represent the current state of the TempNamespace resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// TempNamespace is the Schema for the tempnamespaces API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=tmpns
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner.username`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Expires",type=string,JSONPath=`.spec.expiresAt`
type TempNamespace struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of TempNamespace
	// +required
	Spec TempNamespaceSpec `json:"spec"`

	// status defines the observed state of TempNamespace
	// +optional
	Status TempNamespaceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TempNamespaceList contains a list of TempNamespace
type TempNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []TempNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &TempNamespace{}, &TempNamespaceList{})
		return nil
	})
}
