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

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// DesiredRoleBindingSet represents the complete set of RoleBindings that should exist
// for a given FolderTree. This is shared logic used by both controller and webhook.
type DesiredRoleBindingSet struct {
	RoleBindings map[string]*DesiredRoleBinding // key: namespace/name
}

// CalculateDesiredRoleBindings calculates what RoleBindings should exist for a given FolderTree.
// This is the shared logic used by both controller (for cluster state comparison) and
// webhook (for FolderTree state comparison).
func CalculateDesiredRoleBindings(folderTree *rbacv1alpha1.FolderTree, builder *RoleBindingBuilder) (*DesiredRoleBindingSet, error) {
	desired := make(map[string]*DesiredRoleBinding)

	// Create a map of folder name to folder data for quick lookup
	folderMap := make(map[string]rbacv1alpha1.Folder)
	for _, folder := range folderTree.Spec.Folders {
		folderMap[folder.Name] = folder
	}

	// Process the tree structure (if it exists)
	if folderTree.Spec.Tree != nil {
		if err := calculateFromTreeNode(*folderTree.Spec.Tree, folderMap, []rbacv1alpha1.RoleBindingTemplate{}, desired, builder); err != nil {
			return nil, err
		}
	}

	// Process standalone folders (not in the tree)
	for _, folder := range folderTree.Spec.Folders {
		if !isInTree(folder.Name, folderTree.Spec.Tree) {
			for _, namespace := range folder.Namespaces {
				for _, roleBindingTemplate := range folder.RoleBindingTemplates {
					roleBinding, err := builder.BuildRoleBindingFromTemplate(namespace, roleBindingTemplate)
					if err != nil {
						return nil, fmt.Errorf("failed to build RoleBinding for standalone folder '%s': %v", folder.Name, err)
					}

					key := fmt.Sprintf("%s/%s", namespace, roleBinding.Name)
					desired[key] = &DesiredRoleBinding{
						Namespace:           namespace,
						RoleBindingTemplate: roleBindingTemplate,
						RoleBinding:         roleBinding,
					}
				}
			}
		}
	}

	return &DesiredRoleBindingSet{RoleBindings: desired}, nil
}

// calculateFromTreeNode recursively calculates desired RoleBindings from tree structure
func calculateFromTreeNode(node rbacv1alpha1.TreeNode, folderMap map[string]rbacv1alpha1.Folder, inheritedRoleBindingTemplates []rbacv1alpha1.RoleBindingTemplate, desired map[string]*DesiredRoleBinding, builder *RoleBindingBuilder) error {
	// Get folder data for this node
	folder, exists := folderMap[node.Name]
	var allRoleBindingTemplates []rbacv1alpha1.RoleBindingTemplate
	var templatesToInherit []rbacv1alpha1.RoleBindingTemplate

	if exists {
		// Combine inherited role binding templates with this folder's role binding templates
		allRoleBindingTemplates = append(inheritedRoleBindingTemplates, folder.RoleBindingTemplates...)

		// Create desired RoleBindings for this folder's namespaces
		for _, namespace := range folder.Namespaces {
			for _, roleBindingTemplate := range allRoleBindingTemplates {
				roleBinding, err := builder.BuildRoleBindingFromTemplate(namespace, roleBindingTemplate)
				if err != nil {
					return fmt.Errorf("failed to build RoleBinding for folder '%s': %v", folder.Name, err)
				}

				key := fmt.Sprintf("%s/%s", namespace, roleBinding.Name)
				desired[key] = &DesiredRoleBinding{
					Namespace:           namespace,
					RoleBindingTemplate: roleBindingTemplate,
					RoleBinding:         roleBinding,
				}
			}
		}

		// Determine which templates should be inherited by child folders
		// Start with inherited templates (they already passed propagation checks from ancestors)
		templatesToInherit = append(templatesToInherit, inheritedRoleBindingTemplates...)

		// Add this folder's templates that should propagate
		for _, template := range folder.RoleBindingTemplates {
			// Check propagate field (defaults to false if nil)
			shouldPropagate := template.Propagate != nil && *template.Propagate
			if shouldPropagate {
				templatesToInherit = append(templatesToInherit, template)
			}
		}
	} else {
		// Tree node exists but no folder data - only pass inherited role binding templates
		templatesToInherit = inheritedRoleBindingTemplates
	}

	// Recurse into subfolders with templates that should be inherited
	for _, subfolder := range node.Subfolders {
		if err := calculateFromTreeNode(subfolder, folderMap, templatesToInherit, desired, builder); err != nil {
			return err
		}
	}

	return nil
}

// isInTree checks if a folder name appears in the tree structure
func isInTree(folderName string, tree *rbacv1alpha1.TreeNode) bool {
	if tree == nil {
		return false
	}
	return isInTreeNode(folderName, *tree)
}

// isInTreeNode recursively checks if a folder name appears in a tree node
func isInTreeNode(folderName string, node rbacv1alpha1.TreeNode) bool {
	if node.Name == folderName {
		return true
	}
	for _, subfolder := range node.Subfolders {
		if isInTreeNode(folderName, subfolder) {
			return true
		}
	}
	return false
}
