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
	"context"
	"fmt"
	"regexp"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
	"kubevirt.io/folders/internal/rbac"
)

// nolint:unused
// log is for logging in this package.
var foldertreelog = logf.Log.WithName("foldertree-resource")

// SetupFolderTreeWebhookWithManager registers the webhook for FolderTree in the manager.
func SetupFolderTreeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&rbacv1alpha1.FolderTree{}).
		WithValidator(&FolderTreeCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// Validating admission webhook for FolderTree resources.
// Provides comprehensive validation including business logic, uniqueness constraints,
// and cross-resource validation that cannot be enforced by OpenAPI schema alone.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-rbac-kubevirt-io-v1alpha1-foldertree,mutating=false,failurePolicy=fail,sideEffects=None,groups=rbac.kubevirt.io,resources=foldertrees,verbs=create;update;delete,versions=v1alpha1,name=vfoldertree-v1alpha1.kb.io,admissionReviewVersions=v1

// FolderTreeCustomValidator struct is responsible for validating the FolderTree resource
// when it is created, updated, or deleted. It validates the split structure design where:
// - TreeNodes define hierarchical relationships (with recursive schema handling)
// - Folders contain the actual permission names and namespace data
// For DELETE operations, it validates that the user has permission to delete all RoleBindings
// that would be removed when the FolderTree is deleted, preventing privilege escalation.
// - Permissions define reusable RBAC templates with subjects and roleRef
// - Cross-references between TreeNode names and Folder names are validated
// - Cross-references between Folder permissions and Permission names are validated
// - Global uniqueness constraints are enforced across all FolderTrees
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type FolderTreeCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &FolderTreeCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type FolderTree.
func (v *FolderTreeCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	foldertree, ok := obj.(*rbacv1alpha1.FolderTree)
	if !ok {
		return nil, fmt.Errorf("expected a FolderTree object but got %T", obj)
	}
	foldertreelog.Info("Validation for FolderTree upon creation", "name", foldertree.GetName())

	var allWarnings admission.Warnings

	// Note: We cannot validate unknown fields here because controller-runtime
	// already unmarshaled the object, dropping unknown fields. This is a
	// limitation of the CustomValidator interface.
	// Unknown fields in TreeNode.subfolders are handled by x-kubernetes-preserve-unknown-fields
	// in the CRD schema, which allows them but they're ignored by the Go struct.

	// Validate the split structure: both TreeNodes (hierarchy) and Folders (data)
	if err := v.validateNewStructure(ctx, foldertree); err != nil {
		return nil, err
	}

	// Validate business logic
	if err := v.validateBusinessLogic(ctx, foldertree); err != nil {
		return nil, err
	}

	// Check for conflicts with other FolderTrees
	if err := v.validateGlobalUniqueness(ctx, foldertree); err != nil {
		return nil, err
	}

	// Validate RBAC authorization (privilege escalation check)
	if err := v.validateRBACAuthorization(ctx, foldertree); err != nil {
		return nil, err
	}

	return allWarnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type FolderTree.
func (v *FolderTreeCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldFolderTree, ok := oldObj.(*rbacv1alpha1.FolderTree)
	if !ok {
		return nil, fmt.Errorf("expected a FolderTree object for the oldObj but got %T", oldObj)
	}

	newFolderTree, ok := newObj.(*rbacv1alpha1.FolderTree)
	if !ok {
		return nil, fmt.Errorf("expected a FolderTree object for the newObj but got %T", newObj)
	}

	foldertreelog.Info("Validation for FolderTree upon update", "name", newFolderTree.GetName())

	var allWarnings admission.Warnings

	// Validate the tree structures and folders
	if err := v.validateNewStructure(ctx, newFolderTree); err != nil {
		return nil, err
	}

	// Validate business logic
	if err := v.validateBusinessLogic(ctx, newFolderTree); err != nil {
		return nil, err
	}

	// Check for conflicts with other FolderTrees (excluding this one)
	if err := v.validateGlobalUniqueness(ctx, newFolderTree); err != nil {
		return nil, err
	}

	// No need to validate permission references since role binding templates are now inline

	// Validate RBAC authorization (privilege escalation check) - compare FolderTree states
	if err := v.validateRBACAuthorizationUpdate(ctx, oldFolderTree, newFolderTree); err != nil {
		return nil, err
	}

	return allWarnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type FolderTree.
func (v *FolderTreeCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	foldertree, ok := obj.(*rbacv1alpha1.FolderTree)
	if !ok {
		return nil, fmt.Errorf("expected a FolderTree object but got %T", obj)
	}
	foldertreelog.Info("Validation for FolderTree upon deletion", "name", foldertree.GetName())

	// Validate RBAC authorization - user must have permission to delete all RoleBindings
	// that will be removed when this FolderTree is deleted
	if err := v.validateRBACAuthorizationDelete(ctx, foldertree); err != nil {
		return nil, err
	}

	return nil, nil
}

// validateNewStructure validates the split structure design by:
// 1. Validating the TreeNode structure (hierarchy validation)
// 2. Validating each Folder in the folders array (data validation with inline role binding templates)
// 3. Ensuring proper structure and field constraints for all types
func (v *FolderTreeCustomValidator) validateNewStructure(ctx context.Context, folderTree *rbacv1alpha1.FolderTree) error {
	var allErrors field.ErrorList

	// Validate the tree structure (if it exists)
	if folderTree.Spec.Tree != nil {
		treePath := field.NewPath("spec", "tree")
		if err := v.validateTreeNode(ctx, *folderTree.Spec.Tree, treePath); err != nil {
			allErrors = append(allErrors, field.InternalError(treePath, err))
		}
	}

	// Validate each folder
	for i, folder := range folderTree.Spec.Folders {
		folderPath := field.NewPath("spec", "folders").Index(i)
		if err := v.validateFolder(ctx, folder, folderPath); err != nil {
			allErrors = append(allErrors, field.InternalError(folderPath, err))
		}
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// validateTreeNode validates a single tree node structure
func (v *FolderTreeCustomValidator) validateTreeNode(ctx context.Context, treeNode rbacv1alpha1.TreeNode, fldPath *field.Path) error {
	var allErrors field.ErrorList

	// Validate name
	if len(treeNode.Name) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("name"), "name cannot be empty"))
	} else if !isValidKubernetesName(treeNode.Name) {
		allErrors = append(allErrors, field.Invalid(fldPath.Child("name"), treeNode.Name, "name must be a valid DNS-1123 label"))
	}

	// Recursively validate subfolders
	for i, subfolder := range treeNode.Subfolders {
		subPath := fldPath.Child("subfolders").Index(i)
		if err := v.validateTreeNode(ctx, subfolder, subPath); err != nil {
			allErrors = append(allErrors, field.InternalError(subPath, err))
		}
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// validateFolder validates a single folder data structure
func (v *FolderTreeCustomValidator) validateFolder(ctx context.Context, folder rbacv1alpha1.Folder, fldPath *field.Path) error {
	var allErrors field.ErrorList

	// Validate name
	if len(folder.Name) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("name"), "name cannot be empty"))
	} else if !isValidKubernetesName(folder.Name) {
		allErrors = append(allErrors, field.Invalid(fldPath.Child("name"), folder.Name, "name must be a valid DNS-1123 label"))
	}

	// Validate role binding templates
	for i, roleBindingTemplate := range folder.RoleBindingTemplates {
		roleBindingTemplatePath := fldPath.Child("roleBindingTemplates").Index(i)
		if err := v.validateRoleBindingTemplate(ctx, roleBindingTemplate, roleBindingTemplatePath); err != nil {
			allErrors = append(allErrors, field.InternalError(roleBindingTemplatePath, err))
		}
	}

	// Validate namespaces
	for i, namespace := range folder.Namespaces {
		if len(namespace) == 0 {
			allErrors = append(allErrors, field.Invalid(
				fldPath.Child("namespaces").Index(i), namespace,
				"namespace name cannot be empty string"))
		} else if !isValidKubernetesName(namespace) {
			allErrors = append(allErrors, field.Invalid(
				fldPath.Child("namespaces").Index(i), namespace,
				"namespace must be a valid DNS-1123 label"))
		}
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// validateRoleBindingTemplate validates a single role binding template structure
func (v *FolderTreeCustomValidator) validateRoleBindingTemplate(ctx context.Context, roleBindingTemplate rbacv1alpha1.RoleBindingTemplate, fldPath *field.Path) error {
	var allErrors field.ErrorList

	// Validate name
	if len(roleBindingTemplate.Name) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("name"), "name cannot be empty"))
	} else if !isValidKubernetesName(roleBindingTemplate.Name) {
		allErrors = append(allErrors, field.Invalid(fldPath.Child("name"), roleBindingTemplate.Name, "name must be a valid DNS-1123 label"))
	}

	// Validate subjects (required and must have at least one)
	if len(roleBindingTemplate.Subjects) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("subjects"), "subjects cannot be empty"))
	} else {
		for i, subject := range roleBindingTemplate.Subjects {
			subjectPath := fldPath.Child("subjects").Index(i)

			// Validate subject kind
			if len(subject.Kind) == 0 {
				allErrors = append(allErrors, field.Required(subjectPath.Child("kind"), "kind cannot be empty"))
			}

			// Validate subject name
			if len(subject.Name) == 0 {
				allErrors = append(allErrors, field.Required(subjectPath.Child("name"), "name cannot be empty"))
			}

			// Validate apiGroup for Group and User kinds
			if (subject.Kind == "Group" || subject.Kind == "User") && subject.APIGroup != "rbac.authorization.k8s.io" {
				allErrors = append(allErrors, field.Invalid(subjectPath.Child("apiGroup"), subject.APIGroup, "apiGroup must be 'rbac.authorization.k8s.io' for Group and User kinds"))
			}
		}
	}

	// Validate roleRef (required)
	if len(roleBindingTemplate.RoleRef.Kind) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("roleRef").Child("kind"), "roleRef.kind cannot be empty"))
	}
	if len(roleBindingTemplate.RoleRef.Name) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("roleRef").Child("name"), "roleRef.name cannot be empty"))
	}
	if roleBindingTemplate.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
		allErrors = append(allErrors, field.Invalid(fldPath.Child("roleRef").Child("apiGroup"), roleBindingTemplate.RoleRef.APIGroup, "roleRef.apiGroup must be 'rbac.authorization.k8s.io'"))
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// isValidKubernetesName validates that a name follows DNS-1123 label format
func isValidKubernetesName(name string) bool {
	// DNS-1123 label: lowercase alphanumeric characters or '-',
	// must start and end with alphanumeric character
	if len(name) == 0 || len(name) > 63 {
		return false
	}

	// Regex for DNS-1123 label
	dnsLabelRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	return dnsLabelRegex.MatchString(name)
}

// validateBusinessLogic performs additional business logic validation
func (v *FolderTreeCustomValidator) validateBusinessLogic(ctx context.Context, folderTree *rbacv1alpha1.FolderTree) error {
	var allErrors field.ErrorList

	// Validate that at least one namespace is assigned somewhere
	hasNamespaces := false
	for _, folder := range folderTree.Spec.Folders {
		if len(folder.Namespaces) > 0 {
			hasNamespaces = true
			break
		}
	}

	if !hasNamespaces {
		allErrors = append(allErrors, field.Invalid(
			field.NewPath("spec", "folders"),
			folderTree.Spec.Folders,
			"folder tree must contain at least one namespace assignment"))
	}

	// Validate unique folder names
	folderNames := make(map[string]*field.Path)
	for i, folder := range folderTree.Spec.Folders {
		folderPath := field.NewPath("spec", "folders").Index(i)
		if existingPath, exists := folderNames[folder.Name]; exists {
			allErrors = append(allErrors, field.Duplicate(
				folderPath.Child("name"),
				fmt.Sprintf("folder name '%s' already used at %s", folder.Name, existingPath)))
		} else {
			folderNames[folder.Name] = folderPath.Child("name")
		}
	}

	// Validate unique role binding template names within each folder
	for i, folder := range folderTree.Spec.Folders {
		folderPath := field.NewPath("spec", "folders").Index(i)
		roleBindingTemplateNames := make(map[string]*field.Path)
		for j, roleBindingTemplate := range folder.RoleBindingTemplates {
			roleBindingTemplatePath := folderPath.Child("roleBindingTemplates").Index(j)
			if existingPath, exists := roleBindingTemplateNames[roleBindingTemplate.Name]; exists {
				allErrors = append(allErrors, field.Duplicate(
					roleBindingTemplatePath.Child("name"),
					fmt.Sprintf("role binding template name '%s' already used in folder '%s' at %s", roleBindingTemplate.Name, folder.Name, existingPath)))
			} else {
				roleBindingTemplateNames[roleBindingTemplate.Name] = roleBindingTemplatePath.Child("name")
			}
		}
	}

	// Validate unique namespace assignments
	namespaceAssignments := make(map[string]*field.Path)
	for i, folder := range folderTree.Spec.Folders {
		folderPath := field.NewPath("spec", "folders").Index(i)
		for j, namespace := range folder.Namespaces {
			namespacePath := folderPath.Child("namespaces").Index(j)
			if existingPath, exists := namespaceAssignments[namespace]; exists {
				allErrors = append(allErrors, field.Duplicate(
					namespacePath,
					fmt.Sprintf("namespace '%s' already assigned at %s", namespace, existingPath)))
			} else {
				namespaceAssignments[namespace] = namespacePath
			}
		}
	}

	// Validate unique tree node names within the tree
	treeNodeNames := make(map[string]*field.Path)
	if folderTree.Spec.Tree != nil {
		treePath := field.NewPath("spec", "tree")
		v.validateUniqueTreeNodeNames(*folderTree.Spec.Tree, treePath, treeNodeNames, &allErrors)
	}

	// Validate role binding template names don't conflict in inheritance chains
	if err := v.validateInheritanceConflicts(folderTree, &allErrors); err != nil {
		allErrors = append(allErrors, field.InternalError(field.NewPath("spec"), err))
	}

	// Validate that all tree nodes reference declared folders and all folders are used
	if err := v.validateFolderReferences(folderTree, &allErrors); err != nil {
		allErrors = append(allErrors, field.InternalError(field.NewPath("spec"), err))
	}

	// Validate reasonable limits
	totalFolders := len(folderTree.Spec.Folders)
	totalTreeNodes := 0
	totalNamespaces := 0
	totalRoleBindingTemplates := 0

	// Count tree nodes
	var countTreeNodes func(rbacv1alpha1.TreeNode)
	countTreeNodes = func(treeNode rbacv1alpha1.TreeNode) {
		totalTreeNodes++
		for _, subfolder := range treeNode.Subfolders {
			countTreeNodes(subfolder)
		}
	}

	if folderTree.Spec.Tree != nil {
		countTreeNodes(*folderTree.Spec.Tree)
	}

	// Count namespaces and role binding templates
	for _, folder := range folderTree.Spec.Folders {
		totalNamespaces += len(folder.Namespaces)
		totalRoleBindingTemplates += len(folder.RoleBindingTemplates)
	}

	// Apply reasonable limits
	if totalFolders > 100 {
		allErrors = append(allErrors, field.TooMany(
			field.NewPath("spec", "folders"),
			totalFolders,
			100))
	}

	if totalTreeNodes > 100 {
		allErrors = append(allErrors, field.TooMany(
			field.NewPath("spec", "trees"),
			totalTreeNodes,
			100))
	}

	if totalNamespaces > 500 {
		allErrors = append(allErrors, field.TooMany(
			field.NewPath("spec", "folders"),
			totalNamespaces,
			500))
	}

	if totalRoleBindingTemplates > 200 {
		allErrors = append(allErrors, field.TooMany(
			field.NewPath("spec", "folders"),
			totalRoleBindingTemplates,
			200))
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// validateUniqueTreeNodeNames validates that tree node names are unique within the tree structure
func (v *FolderTreeCustomValidator) validateUniqueTreeNodeNames(treeNode rbacv1alpha1.TreeNode, fldPath *field.Path,
	treeNodeNames map[string]*field.Path, allErrors *field.ErrorList) {

	// Check if this tree node name is already used
	if existingPath, exists := treeNodeNames[treeNode.Name]; exists {
		*allErrors = append(*allErrors, field.Duplicate(
			fldPath.Child("name"),
			fmt.Sprintf("tree node name '%s' already used at %s", treeNode.Name, existingPath)))
	} else {
		treeNodeNames[treeNode.Name] = fldPath.Child("name")
	}

	// Recursively check subfolders
	for i, subfolder := range treeNode.Subfolders {
		subPath := fldPath.Child("subfolders").Index(i)
		v.validateUniqueTreeNodeNames(subfolder, subPath, treeNodeNames, allErrors)
	}
}

// validateInheritanceConflicts validates that role binding template names don't conflict
// in inheritance chains. This prevents the issue where a child folder's template
// overwrites a parent folder's template with the same name.
func (v *FolderTreeCustomValidator) validateInheritanceConflicts(folderTree *rbacv1alpha1.FolderTree, allErrors *field.ErrorList) error {
	// Create a map of folder name to folder data for quick lookup
	folderMap := make(map[string]rbacv1alpha1.Folder)
	folderIndexMap := make(map[string]int) // Track folder indices for error reporting
	for i, folder := range folderTree.Spec.Folders {
		folderMap[folder.Name] = folder
		folderIndexMap[folder.Name] = i
	}

	// Check the tree for inheritance conflicts (if it exists)
	if folderTree.Spec.Tree != nil {
		treePath := field.NewPath("spec", "tree")
		v.validateTreeInheritanceConflicts(*folderTree.Spec.Tree, treePath, folderMap, folderIndexMap, []string{}, allErrors)
	}

	return nil
}

// validateTreeInheritanceConflicts recursively validates inheritance conflicts in a tree structure
func (v *FolderTreeCustomValidator) validateTreeInheritanceConflicts(
	treeNode rbacv1alpha1.TreeNode,
	treePath *field.Path,
	folderMap map[string]rbacv1alpha1.Folder,
	folderIndexMap map[string]int,
	inheritedTemplateNames []string,
	allErrors *field.ErrorList) {

	// Get folder data for this tree node
	folder, exists := folderMap[treeNode.Name]
	var currentTemplateNames []string

	if exists {
		// Check for conflicts between inherited templates and this folder's templates
		folderIndex := folderIndexMap[treeNode.Name]
		folderPath := field.NewPath("spec", "folders").Index(folderIndex)

		for j, roleBindingTemplate := range folder.RoleBindingTemplates {
			templatePath := folderPath.Child("roleBindingTemplates").Index(j)

			// Check if this template name conflicts with any inherited template
			for _, inheritedName := range inheritedTemplateNames {
				if roleBindingTemplate.Name == inheritedName {
					*allErrors = append(*allErrors, field.Invalid(
						templatePath.Child("name"),
						roleBindingTemplate.Name,
						fmt.Sprintf("role binding template name '%s' conflicts with inherited template from parent folder in tree hierarchy", roleBindingTemplate.Name)))
				}
			}

			currentTemplateNames = append(currentTemplateNames, roleBindingTemplate.Name)
		}

		// Combine inherited and current template names for child validation
		allTemplateNames := append(inheritedTemplateNames, currentTemplateNames...)

		// Recursively validate subfolders with accumulated template names
		for _, subfolder := range treeNode.Subfolders {
			v.validateTreeInheritanceConflicts(subfolder, treePath, folderMap, folderIndexMap, allTemplateNames, allErrors)
		}
	} else {
		// Tree node exists but no folder data - pass inherited templates to children
		for _, subfolder := range treeNode.Subfolders {
			v.validateTreeInheritanceConflicts(subfolder, treePath, folderMap, folderIndexMap, inheritedTemplateNames, allErrors)
		}
	}
}

// validateFolderReferences validates that all tree nodes reference declared folders
// and that all declared folders are used somewhere (either in trees or as standalone)
func (v *FolderTreeCustomValidator) validateFolderReferences(folderTree *rbacv1alpha1.FolderTree, allErrors *field.ErrorList) error {
	// Create sets for tracking
	declaredFolders := make(map[string]int)    // folder name -> index in folders array
	referencedFolders := make(map[string]bool) // folder names referenced in trees

	// Collect all declared folders
	for i, folder := range folderTree.Spec.Folders {
		declaredFolders[folder.Name] = i
	}

	// Recursively collect all folder names referenced in trees
	var collectReferencedFolders func(rbacv1alpha1.TreeNode, *field.Path)
	collectReferencedFolders = func(treeNode rbacv1alpha1.TreeNode, treePath *field.Path) {
		// Check if this tree node references a declared folder
		if _, exists := declaredFolders[treeNode.Name]; !exists {
			*allErrors = append(*allErrors, field.Invalid(
				treePath.Child("name"),
				treeNode.Name,
				fmt.Sprintf("tree node '%s' references undeclared folder (must be declared in spec.folders)", treeNode.Name)))
		} else {
			referencedFolders[treeNode.Name] = true
		}

		// Recursively check subfolders
		for i, subfolder := range treeNode.Subfolders {
			subPath := treePath.Child("subfolders").Index(i)
			collectReferencedFolders(subfolder, subPath)
		}
	}

	// Check the tree (if it exists)
	if folderTree.Spec.Tree != nil {
		treePath := field.NewPath("spec", "tree")
		collectReferencedFolders(*folderTree.Spec.Tree, treePath)
	}

	// Check that all declared folders are used (either in trees or as standalone)
	for folderName, folderIndex := range declaredFolders {
		isUsedInTree := referencedFolders[folderName]
		isStandalone := !v.isInAnyTreeHelper(folderName, folderTree.Spec.Tree)

		// A folder is valid if it's either used in a tree OR it's standalone (not in any tree)
		// If it's not in any tree, it's considered a standalone folder which is valid
		if !isUsedInTree && !isStandalone {
			// This case shouldn't happen due to the logic above, but kept for completeness
			continue
		}

		// If a folder is declared but never referenced in trees and has no namespaces,
		// it might be a configuration error (though technically valid as an empty standalone folder)
		if !isUsedInTree && isStandalone {
			folder := folderTree.Spec.Folders[folderIndex]
			if len(folder.Namespaces) == 0 && len(folder.RoleBindingTemplates) == 0 {
				// This is just a warning-level issue - empty standalone folders are technically valid
				// but might indicate a configuration mistake
				*allErrors = append(*allErrors, field.Invalid(
					field.NewPath("spec", "folders").Index(folderIndex).Child("name"),
					folderName,
					"folder is declared but not used in any tree and has no namespaces or role binding templates (possible configuration error)"))
			}
		}
	}

	return nil
}

// isInAnyTreeHelper is a helper function for validateFolderReferences
// (separate from the main isInTree to avoid confusion with the diff analyzer)
func (v *FolderTreeCustomValidator) isInAnyTreeHelper(folderName string, tree *rbacv1alpha1.TreeNode) bool {
	return v.isInTreeHelper(folderName, tree)
}

// isInTreeHelper is a helper function for validateFolderReferences
func (v *FolderTreeCustomValidator) isInTreeHelper(folderName string, tree *rbacv1alpha1.TreeNode) bool {
	if tree == nil {
		return false
	}
	return v.isInTreeNodeHelper(folderName, *tree)
}

// isInTreeNodeHelper recursively checks if a folder name appears in a tree node
func (v *FolderTreeCustomValidator) isInTreeNodeHelper(folderName string, node rbacv1alpha1.TreeNode) bool {
	if node.Name == folderName {
		return true
	}
	for _, subfolder := range node.Subfolders {
		if v.isInTreeNodeHelper(folderName, subfolder) {
			return true
		}
	}
	return false
}

// validateGlobalUniqueness checks that folder names and namespaces don't conflict with other FolderTrees
func (v *FolderTreeCustomValidator) validateGlobalUniqueness(ctx context.Context, newTree *rbacv1alpha1.FolderTree) error {
	// Get all existing FolderTrees
	var folderTreeList rbacv1alpha1.FolderTreeList
	if err := v.Client.List(ctx, &folderTreeList); err != nil {
		return fmt.Errorf("failed to list existing FolderTrees: %v", err)
	}

	// Collect folder names and namespaces from the new tree
	newFolderNames := make(map[string]bool)
	newNamespaces := make(map[string]bool)
	newTreeNodeNames := make(map[string]bool)

	// Collect from folders
	for _, folder := range newTree.Spec.Folders {
		newFolderNames[folder.Name] = true
		for _, ns := range folder.Namespaces {
			newNamespaces[ns] = true
		}
	}

	// Collect from tree nodes
	var collectFromTreeNode func(rbacv1alpha1.TreeNode)
	collectFromTreeNode = func(treeNode rbacv1alpha1.TreeNode) {
		newTreeNodeNames[treeNode.Name] = true
		for _, subfolder := range treeNode.Subfolders {
			collectFromTreeNode(subfolder)
		}
	}

	if newTree.Spec.Tree != nil {
		collectFromTreeNode(*newTree.Spec.Tree)
	}

	// Check against existing trees
	var allErrors field.ErrorList
	for _, existingTree := range folderTreeList.Items {
		// Skip self when updating
		if existingTree.Name == newTree.Name {
			continue
		}

		// Check existing folders for conflicts
		for _, folder := range existingTree.Spec.Folders {
			// Check for folder name conflicts
			if newFolderNames[folder.Name] {
				allErrors = append(allErrors, field.Duplicate(
					field.NewPath("spec", "folders"),
					fmt.Sprintf("folder name '%s' already exists in FolderTree '%s'", folder.Name, existingTree.Name)))
			}

			// Check for namespace conflicts
			for _, ns := range folder.Namespaces {
				if newNamespaces[ns] {
					allErrors = append(allErrors, field.Duplicate(
						field.NewPath("spec", "folders"),
						fmt.Sprintf("namespace '%s' is already assigned in FolderTree '%s'", ns, existingTree.Name)))
				}
			}
		}

		// Check existing tree nodes for conflicts
		var checkExistingTreeNode func(rbacv1alpha1.TreeNode)
		checkExistingTreeNode = func(treeNode rbacv1alpha1.TreeNode) {
			if newTreeNodeNames[treeNode.Name] {
				allErrors = append(allErrors, field.Duplicate(
					field.NewPath("spec", "trees"),
					fmt.Sprintf("tree node name '%s' already exists in FolderTree '%s'", treeNode.Name, existingTree.Name)))
			}
			for _, subfolder := range treeNode.Subfolders {
				checkExistingTreeNode(subfolder)
			}
		}

		if existingTree.Spec.Tree != nil {
			checkExistingTreeNode(*existingTree.Spec.Tree)
		}
	}

	if len(allErrors) > 0 {
		return allErrors.ToAggregate()
	}

	return nil
}

// validateRBACAuthorization checks that the user has permissions to perform the specific operations
// that would be required to synchronize the FolderTree. This prevents privilege escalation and
// validates deletion permissions when namespaces or rolebindingtemplates are removed.
func (v *FolderTreeCustomValidator) validateRBACAuthorization(ctx context.Context, folderTree *rbacv1alpha1.FolderTree) error {
	// For CREATE operations, validate against empty old state
	return v.validateRBACAuthorizationUpdate(ctx, nil, folderTree)
}

// validateRBACAuthorizationUpdate performs privilege escalation validation for UPDATE operations
// by comparing old and new FolderTree states to determine actual changes being made.
// This is the correct approach - webhook should compare FolderTree states, not cluster state.
func (v *FolderTreeCustomValidator) validateRBACAuthorizationUpdate(ctx context.Context, oldFolderTree, newFolderTree *rbacv1alpha1.FolderTree) error {
	// Get the user info from the admission request
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		// If we can't get the request, skip authorization check (fail open for system requests)
		foldertreelog.Info("Could not get admission request for RBAC authorization check", "error", err)
		return nil
	}

	// Skip RBAC authorization check for status-only updates
	if req.SubResource == "status" {
		foldertreelog.Info("Skipping RBAC authorization check for status subresource update")
		return nil
	}

	// Use webhook diff analyzer to compare FolderTree states (not cluster state)
	builder := &rbac.RoleBindingBuilder{
		FolderTree: newFolderTree,
		Scheme:     nil, // Don't set owner reference for webhook validation
	}

	webhookDiffAnalyzer := rbac.NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree, builder)

	// Analyze what operations would be performed between FolderTree states
	operations, err := webhookDiffAnalyzer.AnalyzeFolderTreeDiff()
	if err != nil {
		return fmt.Errorf("failed to analyze FolderTree operations: %v", err)
	}

	// Validate user has permission for these specific operations
	if err := v.validateOperationsWithImpersonation(ctx, operations, req.UserInfo); err != nil {
		return fmt.Errorf("privilege escalation prevented: %v", err)
	}

	return nil
}

// validateOperationsWithImpersonation performs privilege escalation validation
// by impersonating the user and attempting to perform the required operations with dry-run.
func (v *FolderTreeCustomValidator) validateOperationsWithImpersonation(ctx context.Context, operations []rbac.RoleBindingOperation, userInfo authenticationv1.UserInfo) error {
	// Create an impersonation client for the requesting user
	impersonationClient, err := v.createImpersonationClient(userInfo)
	if err != nil {
		return fmt.Errorf("failed to create impersonation client: %v", err)
	}

	// Validate each operation with impersonation + dry-run
	for _, operation := range operations {
		if err := v.validateSingleOperation(ctx, impersonationClient, operation); err != nil {
			return fmt.Errorf("failed to validate %s: %v", operation.String(), err)
		}
	}

	return nil
}

// createImpersonationClient creates a Kubernetes client that impersonates the specified user
func (v *FolderTreeCustomValidator) createImpersonationClient(userInfo authenticationv1.UserInfo) (client.Client, error) {
	// Get the current REST config
	config := ctrl.GetConfigOrDie()

	// Set impersonation
	config.Impersonate = rest.ImpersonationConfig{
		UserName: userInfo.Username,
		Groups:   userInfo.Groups,
		UID:      userInfo.UID,
	}

	// Create a new client with impersonation
	impersonationClient, err := client.New(config, client.Options{
		Scheme: v.Client.Scheme(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create impersonation client: %v", err)
	}

	return impersonationClient, nil
}

// validateSingleOperation validates a single RoleBinding operation with impersonation + dry-run
func (v *FolderTreeCustomValidator) validateSingleOperation(ctx context.Context, impersonationClient client.Client, operation rbac.RoleBindingOperation) error {
	switch operation.Type {
	case rbac.OperationCreate:
		return v.validateCreateOperation(ctx, impersonationClient, operation)
	case rbac.OperationUpdate:
		return v.validateUpdateOperation(ctx, impersonationClient, operation)
	case rbac.OperationDelete:
		return v.validateDeleteOperation(ctx, impersonationClient, operation)
	default:
		return fmt.Errorf("unknown operation type: %s", operation.Type)
	}
}

// validateCreateOperation validates that the user can create the RoleBinding
func (v *FolderTreeCustomValidator) validateCreateOperation(ctx context.Context, impersonationClient client.Client, operation rbac.RoleBindingOperation) error {
	// Use a random name to avoid conflicts during dry-run
	testRoleBinding := operation.DesiredRoleBinding.DeepCopy()
	testRoleBinding.Name = rbac.GenerateRandomRoleBindingName(testRoleBinding.Name, operation.RoleBindingTemplate.Name)

	// Attempt to create with dry-run using impersonation
	if err := impersonationClient.Create(ctx, testRoleBinding, client.DryRunAll); err != nil {
		return fmt.Errorf("dry-run creation failed (user lacks required permissions): %v", err)
	}

	return nil
}

// validateUpdateOperation validates that the user can update the RoleBinding
func (v *FolderTreeCustomValidator) validateUpdateOperation(ctx context.Context, impersonationClient client.Client, operation rbac.RoleBindingOperation) error {
	// Create a copy of the existing RoleBinding with the desired changes
	testRoleBinding := operation.ExistingRoleBinding.DeepCopy()
	testRoleBinding.Subjects = operation.DesiredRoleBinding.Subjects
	testRoleBinding.RoleRef = operation.DesiredRoleBinding.RoleRef
	testRoleBinding.Labels = operation.DesiredRoleBinding.Labels

	// Attempt to update with dry-run using impersonation
	if err := impersonationClient.Update(ctx, testRoleBinding, client.DryRunAll); err != nil {
		return fmt.Errorf("dry-run update failed (user lacks required permissions): %v", err)
	}

	return nil
}

// validateRBACAuthorizationDelete performs privilege escalation validation for DELETE operations
// by calculating all RoleBindings that would be deleted and validating user permissions for each.
func (v *FolderTreeCustomValidator) validateRBACAuthorizationDelete(ctx context.Context, folderTree *rbacv1alpha1.FolderTree) error {
	// Get the user info from the admission request
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		// If we can't get the request, skip authorization check (fail open for system requests)
		foldertreelog.Info("Could not get admission request for RBAC authorization check", "error", err)
		return nil
	}

	// Skip validation for system users (controllers, etc.)
	if req.UserInfo.Username == "system:serviceaccount:folders-system:folder-controller-manager" ||
		req.UserInfo.Username == "system:admin" {
		return nil
	}

	// Calculate all RoleBindings that would be deleted when this FolderTree is removed
	builder := &rbac.RoleBindingBuilder{
		FolderTree: folderTree,
		Scheme:     v.Client.Scheme(),
	}

	desiredState, err := rbac.CalculateDesiredRoleBindings(folderTree, builder)
	if err != nil {
		return fmt.Errorf("failed to calculate RoleBindings for deletion validation: %v", err)
	}

	// Create impersonation client
	impersonationClient, err := v.createImpersonationClient(req.UserInfo)
	if err != nil {
		return fmt.Errorf("failed to create impersonation client: %v", err)
	}

	// Validate that the user can delete each RoleBinding that would be removed
	for _, desiredRoleBinding := range desiredState.RoleBindings {
		operation := rbac.RoleBindingOperation{
			Type:                rbac.OperationDelete,
			Namespace:           desiredRoleBinding.Namespace,
			RoleBindingTemplate: desiredRoleBinding.RoleBindingTemplate,
			ExistingRoleBinding: desiredRoleBinding.RoleBinding, // The RoleBinding that would be deleted
		}

		if err := v.validateDeleteOperation(ctx, impersonationClient, operation); err != nil {
			return fmt.Errorf("privilege escalation prevented: failed to validate DELETE RoleBinding '%s' in namespace '%s' for template '%s': %v",
				desiredRoleBinding.RoleBinding.Name,
				desiredRoleBinding.Namespace,
				desiredRoleBinding.RoleBindingTemplate.Name,
				err)
		}
	}

	return nil
}

// validateDeleteOperation validates that the user can delete the RoleBinding
func (v *FolderTreeCustomValidator) validateDeleteOperation(ctx context.Context, impersonationClient client.Client, operation rbac.RoleBindingOperation) error {
	// Attempt to delete with dry-run using impersonation
	if err := impersonationClient.Delete(ctx, operation.ExistingRoleBinding, client.DryRunAll); err != nil {
		return fmt.Errorf("dry-run deletion failed (user lacks required permissions): %v", err)
	}

	return nil
}
