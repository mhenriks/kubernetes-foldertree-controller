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
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// WebhookDiffAnalyzer compares two FolderTree states to determine what RoleBinding operations
// would be needed. This is used by the webhook to validate permissions for the actual changes
// being made, independent of the current cluster state.
type WebhookDiffAnalyzer struct {
	OldFolderTree *rbacv1alpha1.FolderTree // Previous state (can be nil for create)
	NewFolderTree *rbacv1alpha1.FolderTree // Desired state
	Builder       *RoleBindingBuilder
}

// NewWebhookDiffAnalyzer creates a new webhook diff analyzer for comparing FolderTree states
func NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree *rbacv1alpha1.FolderTree, builder *RoleBindingBuilder) *WebhookDiffAnalyzer {
	return &WebhookDiffAnalyzer{
		OldFolderTree: oldFolderTree,
		NewFolderTree: newFolderTree,
		Builder:       builder,
	}
}

// AnalyzeFolderTreeDiff calculates the operations needed to transition from old to new FolderTree state.
// This is the webhook-specific logic that compares FolderTree states rather than cluster state.
func (w *WebhookDiffAnalyzer) AnalyzeFolderTreeDiff() ([]RoleBindingOperation, error) {
	// Calculate what RoleBindings the old FolderTree would create (empty if nil)
	var oldDesired *DesiredRoleBindingSet
	var err error

	if w.OldFolderTree != nil {
		oldDesired, err = CalculateDesiredRoleBindings(w.OldFolderTree, w.Builder)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate old desired state: %v", err)
		}
	} else {
		// Create operation - no old state
		oldDesired = &DesiredRoleBindingSet{RoleBindings: make(map[string]*DesiredRoleBinding)}
	}

	// Calculate what RoleBindings the new FolderTree would create
	newDesired, err := CalculateDesiredRoleBindings(w.NewFolderTree, w.Builder)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate new desired state: %v", err)
	}

	// Compare the two sets to determine operations
	return w.compareDesiredStates(oldDesired.RoleBindings, newDesired.RoleBindings), nil
}

// compareDesiredStates compares old and new desired states to generate operations
func (w *WebhookDiffAnalyzer) compareDesiredStates(oldDesired, newDesired map[string]*DesiredRoleBinding) []RoleBindingOperation {
	var operations []RoleBindingOperation

	// Find creates and updates
	for key, newRB := range newDesired {
		if oldRB, exists := oldDesired[key]; exists {
			// RoleBinding existed before - check if it needs updating
			if w.needsUpdate(oldRB.RoleBinding, newRB.RoleBinding) {
				operations = append(operations, RoleBindingOperation{
					Type:                OperationUpdate,
					Namespace:           newRB.Namespace,
					RoleBindingTemplate: newRB.RoleBindingTemplate,
					ExistingRoleBinding: oldRB.RoleBinding,
					DesiredRoleBinding:  newRB.RoleBinding,
				})
			}
		} else {
			// New RoleBinding - create operation
			operations = append(operations, RoleBindingOperation{
				Type:                OperationCreate,
				Namespace:           newRB.Namespace,
				RoleBindingTemplate: newRB.RoleBindingTemplate,
				DesiredRoleBinding:  newRB.RoleBinding,
			})
		}
	}

	// Find deletes
	for key, oldRB := range oldDesired {
		if _, exists := newDesired[key]; !exists {
			// RoleBinding was removed - delete operation
			operations = append(operations, RoleBindingOperation{
				Type:                OperationDelete,
				Namespace:           oldRB.Namespace,
				RoleBindingTemplate: oldRB.RoleBindingTemplate,
				ExistingRoleBinding: oldRB.RoleBinding,
			})
		}
	}

	return operations
}

// needsUpdate checks if a RoleBinding needs to be updated (reused from diff.go logic)
func (w *WebhookDiffAnalyzer) needsUpdate(existing, desired *rbacv1.RoleBinding) bool {
	// Compare subjects
	if !w.subjectsEqual(existing.Subjects, desired.Subjects) {
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

// subjectsEqual compares two slices of RBAC subjects for equality (reused from diff.go logic)
func (w *WebhookDiffAnalyzer) subjectsEqual(a, b []rbacv1.Subject) bool {
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
