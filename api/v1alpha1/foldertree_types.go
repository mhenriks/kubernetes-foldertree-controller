/*
Copyright 2025.

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

package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConditionTypeReady indicates that the FolderTree has been successfully processed
	ConditionTypeReady = "Ready"

	// ConditionTypeProcessingFailed indicates that processing the FolderTree failed
	ConditionTypeProcessingFailed = "ProcessingFailed"
)

// FolderTree API implementation for hierarchical namespace organization with RBAC.
// This file defines the core types for the split structure design.

// FolderTree API Design:
// This API uses a split structure approach with inline role binding templates:
// - TreeNode: Defines the tree structure (parent-child relationships) with names only
// - Folder: Contains the actual data (inline role binding templates, namespaces) for each named folder
// - RoleBindingTemplate: Defines inline RBAC templates with subjects and roleRef directly in folders
// - Controller: Maps TreeNode names to Folder data and inherits role binding templates down the hierarchy
//
// Benefits:
// - Reduces OpenAPI v3 recursive schema issues to minimal scope (only TreeNode.subfolders)
// - Allows strict validation for folder data while working around recursive limitations
// - Supports standalone folders not part of any tree structure
// - Clean separation of concerns between hierarchy and data with inline RBAC definitions
// - Eliminates need for separate Permission section and FolderRoleBinding CRD

// TreeNode represents the hierarchical structure without any data.
// TreeNodes define parent-child relationships using names that reference Folder objects.
type TreeNode struct {
	// Name is the unique identifier for this tree node
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Subfolders is a list of child tree nodes
	// +optional
	// +kubebuilder:validation:Schemaless
	// NOTE: Due to limitations in OpenAPI v3 schema generation for recursive types,
	// unknown fields in subfolders will be accepted by the API server but ignored
	// by the controller. This is a known limitation, not a feature.
	Subfolders []TreeNode `json:"subfolders,omitempty"`
}

// RoleBindingTemplate defines an inline RBAC template for a folder.
// RoleBindingTemplates contain the subjects and roleRef needed to create RoleBindings.
type RoleBindingTemplate struct {
	// Name is the unique identifier for this role binding template
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Subjects holds references to the objects the role applies to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Subjects []rbacv1.Subject `json:"subjects"`

	// RoleRef can only reference a ClusterRole in the global namespace.
	// If the RoleRef cannot be resolved, the Authorizer must return an error.
	// +kubebuilder:validation:Required
	RoleRef rbacv1.RoleRef `json:"roleRef"`

	// Propagate determines whether this role binding template should be inherited
	// by child folders in the hierarchy. If true, child folders will inherit this
	// template. If false or unset (default), this template applies only to the current folder.
	// +optional
	// +kubebuilder:default=false
	Propagate *bool `json:"propagate,omitempty"`
}

// Folder represents folder data without hierarchical structure.
// Folders contain the actual role binding templates and namespace assignments.
// Folder names are referenced by TreeNode names to establish relationships.
type Folder struct {
	// Name is the unique identifier for this folder
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// RoleBindingTemplates is a list of inline RBAC templates that apply to this folder
	// +optional
	RoleBindingTemplates []RoleBindingTemplate `json:"roleBindingTemplates,omitempty"`

	// Namespaces is a list of Kubernetes namespaces that belong to this folder
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`
}

// FolderTreeSpec defines the desired state of FolderTree using a split structure approach.
// The spec separates hierarchical relationships (tree) from data (folders) with
// inline RBAC definitions for better schema validation and cleaner separation of concerns.
type FolderTreeSpec struct {
	// Tree defines the hierarchical structure with parent-child relationships.
	// TreeNode names must reference Folder names to establish the data association.
	// +optional
	Tree *TreeNode `json:"tree,omitempty"`

	// Folders is a flat list of folder data containing inline role binding templates and namespace assignments.
	// Folders can exist independently (standalone) or be referenced by the Tree.
	// Folder names must be unique within a FolderTree.
	// +optional
	Folders []Folder `json:"folders,omitempty"`
}

// FolderTreeStatus defines the observed state of FolderTree.
type FolderTreeStatus struct {
	// Conditions represent the latest available observations of the FolderTree's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ProcessedGeneration is the generation of the FolderTree that was last processed
	// +optional
	ProcessedGeneration int64 `json:"processedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// FolderTree is the Schema for the foldertrees API.
// FolderTree allows grouping Kubernetes namespaces into a hierarchical structure
// with inherited RBAC permissions. It uses a split structure design where:
// - spec.tree defines the hierarchy (TreeNode with parent-child relationships)
// - spec.folders[] contains the data (inline role binding templates and namespace assignments)
// The controller creates RoleBindings in namespaces based on folder role binding templates
// and inherits role binding templates from parent folders in the tree structure.
type FolderTree struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of FolderTree
	// +required
	Spec FolderTreeSpec `json:"spec"`

	// status defines the observed state of FolderTree
	// +optional
	Status FolderTreeStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// FolderTreeList contains a list of FolderTree
type FolderTreeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FolderTree `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FolderTree{}, &FolderTreeList{})
}
