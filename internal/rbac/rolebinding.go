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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// RoleBindingBuilder provides shared logic for creating RoleBindings
// Used by both the controller (for actual creation) and webhook (for dry-run validation)
type RoleBindingBuilder struct {
	FolderTree *rbacv1alpha1.FolderTree
	Scheme     *runtime.Scheme
}

// BuildRoleBindingFromTemplate creates a RoleBinding for the given namespace and role binding template
// This is the shared logic used by both controller and webhook
func (rb *RoleBindingBuilder) BuildRoleBindingFromTemplate(namespace string, roleBindingTemplate rbacv1alpha1.RoleBindingTemplate) (*rbacv1.RoleBinding, error) {
	// Create RoleBinding name
	roleBindingName := fmt.Sprintf("foldertree-%s-%s", rb.FolderTree.Name, roleBindingTemplate.Name)

	// Define the RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":                      "foldertree-controller",
				"foldertree.rbac.kubevirt.io/tree":                  rb.FolderTree.Name,
				"foldertree.rbac.kubevirt.io/role-binding-template": roleBindingTemplate.Name,
			},
		},
		Subjects: roleBindingTemplate.Subjects,
		RoleRef:  roleBindingTemplate.RoleRef,
	}

	// Set owner reference (only for controller, webhook skips this)
	if rb.Scheme != nil {
		if err := controllerutil.SetControllerReference(rb.FolderTree, roleBinding, rb.Scheme); err != nil {
			return nil, err
		}
	}

	return roleBinding, nil
}

// GenerateRandomRoleBindingName creates a unique name for dry-run validation
// This ensures webhook dry-run attempts don't conflict with real RoleBindings
func GenerateRandomRoleBindingName(folderTreeName, permissionName string) string {
	// Use a timestamp with nanoseconds to ensure uniqueness even for rapid calls
	return fmt.Sprintf("dryrun-foldertree-%s-%s-%d", folderTreeName, permissionName, metav1.Now().UnixNano())
}
