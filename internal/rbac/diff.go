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

package rbac

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// OperationType represents the type of operation needed for a RoleBinding
type OperationType string

const (
	// OperationCreate indicates a new RoleBinding needs to be created
	OperationCreate OperationType = "create"
	// OperationUpdate indicates an existing RoleBinding needs to be updated
	OperationUpdate OperationType = "update"
	// OperationDelete indicates an existing RoleBinding needs to be deleted
	OperationDelete OperationType = "delete"
)

// RoleBindingOperation represents an operation that needs to be performed on a RoleBinding
type RoleBindingOperation struct {
	Type                OperationType
	Namespace           string
	RoleBindingTemplate rbacv1alpha1.RoleBindingTemplate
	ExistingRoleBinding *rbacv1.RoleBinding // nil for create operations
	DesiredRoleBinding  *rbacv1.RoleBinding // nil for delete operations
}

// String returns a human-readable description of the operation
func (op *RoleBindingOperation) String() string {
	switch op.Type {
	case OperationCreate:
		return fmt.Sprintf("CREATE RoleBinding '%s' in namespace '%s' for template '%s'",
			op.DesiredRoleBinding.Name, op.Namespace, op.RoleBindingTemplate.Name)
	case OperationUpdate:
		return fmt.Sprintf("UPDATE RoleBinding '%s' in namespace '%s' for template '%s'",
			op.ExistingRoleBinding.Name, op.Namespace, op.RoleBindingTemplate.Name)
	case OperationDelete:
		return fmt.Sprintf("DELETE RoleBinding '%s' in namespace '%s'",
			op.ExistingRoleBinding.Name, op.Namespace)
	default:
		return fmt.Sprintf("UNKNOWN operation on RoleBinding in namespace '%s'", op.Namespace)
	}
}

// DiffAnalyzer compares the desired state (from FolderTree) with the current state (existing RoleBindings)
// and returns a list of operations needed to synchronize them
type DiffAnalyzer struct {
	Client     client.Client
	FolderTree *rbacv1alpha1.FolderTree
	Builder    *RoleBindingBuilder
}

// NewDiffAnalyzer creates a new DiffAnalyzer instance
func NewDiffAnalyzer(client client.Client, folderTree *rbacv1alpha1.FolderTree, builder *RoleBindingBuilder) *DiffAnalyzer {
	return &DiffAnalyzer{
		Client:     client,
		FolderTree: folderTree,
		Builder:    builder,
	}
}

// AnalyzeDiff compares the desired state with current state and returns required operations
func (da *DiffAnalyzer) AnalyzeDiff(ctx context.Context) ([]RoleBindingOperation, error) {
	// Get all existing RoleBindings managed by this FolderTree
	existingRoleBindings, err := da.getExistingRoleBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing RoleBindings: %v", err)
	}

	// Collect desired RoleBindings from the FolderTree specification
	desiredRoleBindings, err := da.collectDesiredRoleBindings()
	if err != nil {
		return nil, fmt.Errorf("failed to collect desired RoleBindings: %v", err)
	}

	// Compare and generate operations
	operations := da.compareAndGenerateOperations(existingRoleBindings, desiredRoleBindings)

	return operations, nil
}

// getExistingRoleBindings retrieves all RoleBindings managed by this FolderTree
func (da *DiffAnalyzer) getExistingRoleBindings(ctx context.Context) (map[string]*rbacv1.RoleBinding, error) {
	roleBindingList := &rbacv1.RoleBindingList{}
	err := da.Client.List(ctx, roleBindingList, client.MatchingLabels{
		"foldertree.rbac.kubevirt.io/tree": da.FolderTree.Name,
	})
	if err != nil {
		return nil, err
	}

	existing := make(map[string]*rbacv1.RoleBinding)
	for i := range roleBindingList.Items {
		rb := &roleBindingList.Items[i]
		key := fmt.Sprintf("%s/%s", rb.Namespace, rb.Name)
		existing[key] = rb
	}

	return existing, nil
}

// DesiredRoleBinding represents a RoleBinding that should exist according to the FolderTree spec
type DesiredRoleBinding struct {
	Namespace           string
	RoleBindingTemplate rbacv1alpha1.RoleBindingTemplate
	RoleBinding         *rbacv1.RoleBinding
}

// collectDesiredRoleBindings uses the shared calculation logic to determine what RoleBindings should exist
func (da *DiffAnalyzer) collectDesiredRoleBindings() (map[string]*DesiredRoleBinding, error) {
	desiredSet, err := CalculateDesiredRoleBindings(da.FolderTree, da.Builder)
	if err != nil {
		return nil, err
	}
	return desiredSet.RoleBindings, nil
}

// Note: collectFromTreeNode logic moved to calculation.go as shared function

// compareAndGenerateOperations compares existing and desired RoleBindings and generates operations
func (da *DiffAnalyzer) compareAndGenerateOperations(existing map[string]*rbacv1.RoleBinding, desired map[string]*DesiredRoleBinding) []RoleBindingOperation {
	var operations []RoleBindingOperation

	// Check for creates and updates
	for key, desiredRB := range desired {
		if existingRB, exists := existing[key]; exists {
			// RoleBinding exists, check if it needs updating
			if da.needsUpdate(existingRB, desiredRB.RoleBinding) {
				operations = append(operations, RoleBindingOperation{
					Type:                OperationUpdate,
					Namespace:           desiredRB.Namespace,
					RoleBindingTemplate: desiredRB.RoleBindingTemplate,
					ExistingRoleBinding: existingRB,
					DesiredRoleBinding:  desiredRB.RoleBinding,
				})
			}
		} else {
			// RoleBinding doesn't exist, needs to be created
			operations = append(operations, RoleBindingOperation{
				Type:                OperationCreate,
				Namespace:           desiredRB.Namespace,
				RoleBindingTemplate: desiredRB.RoleBindingTemplate,
				ExistingRoleBinding: nil,
				DesiredRoleBinding:  desiredRB.RoleBinding,
			})
		}
	}

	// Check for deletes
	for key, existingRB := range existing {
		if _, exists := desired[key]; !exists {
			// RoleBinding exists but is no longer desired, needs to be deleted
			operations = append(operations, RoleBindingOperation{
				Type:                OperationDelete,
				Namespace:           existingRB.Namespace,
				RoleBindingTemplate: rbacv1alpha1.RoleBindingTemplate{}, // Empty for delete operations
				ExistingRoleBinding: existingRB,
				DesiredRoleBinding:  nil,
			})
		}
	}

	return operations
}

// needsUpdate checks if an existing RoleBinding needs to be updated to match the desired state
func (da *DiffAnalyzer) needsUpdate(existing, desired *rbacv1.RoleBinding) bool {
	// Compare subjects
	if !da.subjectsEqual(existing.Subjects, desired.Subjects) {
		return true
	}

	// Compare roleRef
	if existing.RoleRef != desired.RoleRef {
		return true
	}

	// Compare labels (only the ones we manage)
	for key, desiredValue := range desired.Labels {
		if existingValue, exists := existing.Labels[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	return false
}

// subjectsEqual compares two slices of RBAC subjects for equality
func (da *DiffAnalyzer) subjectsEqual(a, b []rbacv1.Subject) bool {
	if len(a) != len(b) {
		return false
	}

	// Create maps for comparison (order shouldn't matter)
	aMap := make(map[string]rbacv1.Subject)
	bMap := make(map[string]rbacv1.Subject)

	for _, subject := range a {
		key := fmt.Sprintf("%s:%s:%s:%s", subject.Kind, subject.Name, subject.Namespace, subject.APIGroup)
		aMap[key] = subject
	}

	for _, subject := range b {
		key := fmt.Sprintf("%s:%s:%s:%s", subject.Kind, subject.Name, subject.Namespace, subject.APIGroup)
		bMap[key] = subject
	}

	// Compare maps
	for key, subjectA := range aMap {
		if subjectB, exists := bMap[key]; !exists || subjectA != subjectB {
			return false
		}
	}

	return true
}
