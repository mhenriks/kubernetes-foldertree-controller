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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
	"kubevirt.io/folders/internal/rbac"
)

// createTestNamespace creates a simple Namespace object for testing
func createTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

var _ = Describe("FolderTree Webhook", func() {
	var (
		ctx       context.Context
		obj       *rbacv1alpha1.FolderTree
		validator FolderTreeCustomValidator
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &rbacv1alpha1.FolderTree{}
		validator = FolderTreeCustomValidator{Client: k8sClient}

		// Create common test namespaces used by multiple tests
		commonNsNames := []string{
			"test-ns", "child-ns", "frontend-ns", "backend-ns",
			"child1-ns", "child2-ns", "tree-ns", "standalone-ns",
		}
		for _, name := range commonNsNames {
			ns := createTestNamespace(name)
			_ = k8sClient.Create(ctx, ns) // Ignore error if already exists
		}
	})

	Context("Inline Role Binding Templates Validation", func() {
		It("should validate correct role binding template structure", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject role binding template with empty name", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "", // Empty name should be rejected
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject role binding template with empty subjects", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name:     "test-template",
								Subjects: []rbacv1.Subject{}, // Empty subjects should be rejected
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate folder with multiple role binding templates", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
							{
								Name: "viewer-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "viewers",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject duplicate role binding template names within a folder", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "duplicate-name",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user1",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
							{
								Name: "duplicate-name", // Duplicate name should be rejected
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user2",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate tree structure with inheritance", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "parent",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "child",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "parent",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "parent-admin",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "parent-admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
					},
					{
						Name: "child",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "child-editor",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "child-editors",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"child-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("Business Logic Validation", func() {
		It("should require at least one namespace assignment", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						// No namespaces assigned anywhere
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject duplicate folder names", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "duplicate-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template1",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user1",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"ns1"},
					},
					{
						Name: "duplicate-folder", // Duplicate name
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template2",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user2",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"ns2"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject duplicate namespace assignments", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "folder1",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template1",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user1",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"duplicate-ns"},
					},
					{
						Name: "folder2",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template2",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user2",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"duplicate-ns"}, // Duplicate namespace
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate tree node names are unique", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "duplicate-node",
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "child1",
								},
							},
						},
						{
							Name: "duplicate-node", // Duplicate tree node name
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "child2",
								},
							},
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate subject apiGroup for User and Group kinds", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "wrong.api.group", // Should be rbac.authorization.k8s.io
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate roleRef apiGroup", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "wrong.api.group", // Should be rbac.authorization.k8s.io
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("Inheritance Conflict Validation", func() {
		It("should allow same template names in different inheritance chains", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "frontend-team",
						},
						{
							Name: "backend-team",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "root",
						// Root folder with no direct templates (just provides structure)
					},
					{
						Name: "frontend-team",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-access", // Same name as backend-team, but different chain
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "frontend-admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"frontend-ns"},
					},
					{
						Name: "backend-team",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-access", // Same name as frontend-team, but different chain
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "backend-admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"backend-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject template names that conflict in inheritance chain", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "parent",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "child",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "parent",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "parent-admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
					},
					{
						Name: "child",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template", // Conflicts with parent template
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "child-admins",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"child-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicts with inherited template"))
			Expect(warnings).To(BeEmpty())
		})

		It("should handle multi-level inheritance conflicts", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "level1",
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "level2",
								},
							},
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "root",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "root-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "root-users",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
					{
						Name: "level1",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "level1-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "level1-users",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
					},
					{
						Name: "level2",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "root-template", // Conflicts with root template
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "level2-users",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"level2-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicts with inherited template"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow templates with same name when no inheritance relationship", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "parent1",
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "child1",
								},
							},
						},
						{
							Name: "parent2",
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "child2",
								},
							},
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "root",
						// Root folder with no direct templates (just provides structure)
					},
					{
						Name: "parent1",
						// Parent folder with no direct namespaces (inheritance only)
					},
					{
						Name: "parent2",
						// Parent folder with no direct namespaces (inheritance only)
					},
					{
						Name: "child1",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "common-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "child1-users",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"child1-ns"},
					},
					{
						Name: "child2",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "common-template", // Same name as child1, but different tree
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "child2-users",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"child2-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("Diff-based Authorization Validation", func() {
		It("should validate operations with diff analyzer", func() {
			// Note: This test validates the structure but cannot test actual RBAC
			// authorization without a real cluster and user impersonation setup
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
				},
			}

			// The validation should pass structural validation
			// RBAC authorization validation is skipped in test environment
			// since we don't have admission request context
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should handle deletion scenarios in validation", func() {
			// Test that the validator can handle scenarios where namespaces
			// or role binding templates are removed (deletion operations)
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "reduced-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view", // Less privileged role
								},
							},
						},
						Namespaces: []string{"test-ns"}, // Reduced from multiple namespaces
					},
				},
			}

			// The validation should pass structural validation
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate fine-grained changes", func() {
			// Test that the validator handles incremental changes properly
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "parent",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "child",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "parent",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "parent-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "parent-group",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
					{
						Name: "child",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "child-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "child-group",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"child-ns"},
					},
				},
			}

			// The validation should pass and handle inheritance properly
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("Folder Reference Validation", func() {
		It("should reject tree nodes that reference undeclared folders", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "parent",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "missing-folder", // This folder is NOT declared
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "parent",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
					// missing-folder is NOT declared here
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("references undeclared folder"))
			Expect(err.Error()).To(ContainSubstring("missing-folder"))
			Expect(warnings).To(BeEmpty())
		})

		It("should warn about empty standalone folders", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "used-folder",
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "used-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"test-ns"},
					},
					{
						Name: "empty-standalone",
						// No namespaces or role binding templates - should trigger warning
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not used in any tree and has no namespaces or role binding templates"))
			Expect(err.Error()).To(ContainSubstring("empty-standalone"))
			Expect(warnings).To(BeEmpty())
		})

		It("should accept valid standalone folders", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "tree-folder",
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "tree-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "tree-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "tree-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"tree-ns"},
					},
					{
						Name: "standalone-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "standalone-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "standalone-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"standalone-ns"},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should accept folders with only role binding templates when used in trees", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "template-only-folder",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "child-with-namespaces",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "template-only-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template-only",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "template-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						// No namespaces - this is valid for inheritance-only folders
					},
					{
						Name: "child-with-namespaces",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "child-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "child-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
						Namespaces: []string{"child-ns"}, // This satisfies the "at least one namespace" requirement
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should handle complex tree structures with multiple levels", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "level1",
							Subfolders: []rbacv1alpha1.TreeNode{
								{
									Name: "level2",
								},
								{
									Name: "missing-level2", // This should cause error
								},
							},
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "root",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "root-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "root-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
					{
						Name: "level1",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "level1-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "level1-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
					},
					{
						Name: "level2",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "level2-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "level2-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
						Namespaces: []string{"level2-ns"},
					},
					// missing-level2 is NOT declared
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("references undeclared folder"))
			Expect(err.Error()).To(ContainSubstring("missing-level2"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("DELETE Validation", func() {
		It("should validate successful DELETE when user has permissions", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name:       "test-folder",
						Namespaces: []string{"test-namespace"},
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
				},
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			// Note: This may fail in test environment due to impersonation client setup
			// In real cluster, this would validate user permissions properly
			Expect(warnings).To(BeEmpty())
			// We expect either success or a specific impersonation error
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("failed to create impersonation client"),
					ContainSubstring("Could not get admission request"),
				))
			}
		})

		It("should validate DELETE with multiple role binding templates", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name:       "multi-template-folder",
						Namespaces: []string{"namespace-1", "namespace-2"},
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template-1",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "user-1",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
							{
								Name: "template-2",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "group-1",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
						},
					},
				},
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(warnings).To(BeEmpty())
			// Should validate deletion of all RoleBindings across all namespaces
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("failed to create impersonation client"),
					ContainSubstring("Could not get admission request"),
				))
			}
		})

		It("should validate DELETE with hierarchical inheritance", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Tree: &rbacv1alpha1.TreeNode{
					Name: "root",
					Subfolders: []rbacv1alpha1.TreeNode{
						{
							Name: "child",
						},
					},
				},
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "root",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name:      "root-template",
								Propagate: &[]bool{true}[0], // Should inherit to child
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "root-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "admin",
								},
							},
						},
					},
					{
						Name:       "child",
						Namespaces: []string{"child-namespace"},
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "child-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "child-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
				},
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(warnings).To(BeEmpty())
			// Should validate deletion of both inherited and direct RoleBindings
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("failed to create impersonation client"),
					ContainSubstring("Could not get admission request"),
				))
			}
		})

		It("should handle empty FolderTree DELETE gracefully", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{}, // No folders
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(warnings).To(BeEmpty())
			Expect(err).NotTo(HaveOccurred()) // Should succeed as there are no RoleBindings to validate
		})

		It("should handle FolderTree with no namespaces DELETE gracefully", func() {
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "folder-without-namespaces",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "template-without-namespaces",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						// No namespaces specified
					},
				},
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(warnings).To(BeEmpty())
			Expect(err).NotTo(HaveOccurred()) // Should succeed as no RoleBindings would be created
		})
	})

	Context("DELETE+CREATE Pair Validation", func() {
		It("should group operations by target correctly", func() {
			validator := FolderTreeCustomValidator{}

			operations := []rbac.RoleBindingOperation{
				{
					Type:      rbac.OperationDelete,
					Namespace: "test-ns",
					ExistingRoleBinding: &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{Name: "test-rb"},
					},
				},
				{
					Type:      rbac.OperationCreate,
					Namespace: "test-ns",
					DesiredRoleBinding: &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{Name: "test-rb"},
					},
				},
				{
					Type:      rbac.OperationCreate,
					Namespace: "other-ns",
					DesiredRoleBinding: &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{Name: "other-rb"},
					},
				},
			}

			groups := validator.groupOperationsByTarget(operations)

			// Should have 2 groups
			Expect(groups).To(HaveLen(2))

			// test-ns/test-rb should have DELETE+CREATE pair
			testNsGroup := groups["test-ns/test-rb"]
			Expect(testNsGroup).To(HaveLen(2))

			var hasDelete, hasCreate bool
			for _, op := range testNsGroup {
				switch op.Type {
				case rbac.OperationDelete:
					hasDelete = true
				case rbac.OperationCreate:
					hasCreate = true
				}
			}
			Expect(hasDelete).To(BeTrue())
			Expect(hasCreate).To(BeTrue())

			// other-ns/other-rb should have single CREATE
			otherNsGroup := groups["other-ns/other-rb"]
			Expect(otherNsGroup).To(HaveLen(1))
			Expect(otherNsGroup[0].Type).To(Equal(rbac.OperationCreate))
		})
	})

	Context("Namespace Existence Validation", func() {
		It("should validate collectNamespaces helper function", func() {
			// Test with nil FolderTree
			namespaces := validator.collectNamespaces(nil)
			Expect(namespaces).To(BeEmpty())

			// Test with FolderTree containing namespaces
			ft := &rbacv1alpha1.FolderTree{
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "folder1",
							Namespaces: []string{"ns1", "ns2"},
						},
						{
							Name:       "folder2",
							Namespaces: []string{"ns3"},
						},
					},
				},
			}

			namespaces = validator.collectNamespaces(ft)
			Expect(namespaces).To(HaveLen(3))
			Expect(namespaces["ns1"]).To(BeTrue())
			Expect(namespaces["ns2"]).To(BeTrue())
			Expect(namespaces["ns3"]).To(BeTrue())
		})

		It("should reject CREATE when namespace doesn't exist", func() {
			obj.ObjectMeta.Name = "test-nonexistent-ns"
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"namespace-that-absolutely-does-not-exist"},
					},
				},
			}

			// Should fail because namespace doesn't exist
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
			Expect(err.Error()).To(ContainSubstring("cannot add non-existent namespace"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow UPDATE when namespace was in old tree (simulating deleted namespace)", func() {
			// Simulate old FolderTree that had a namespace (now deleted)
			oldObj := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "test-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
							Namespaces: []string{"deleted-namespace-from-old-tree"},
						},
					},
				},
			}

			// New FolderTree with same namespace (but namespace was deleted externally)
			newObj := oldObj.DeepCopy()
			newObj.Spec.Folders[0].RoleBindingTemplates = append(
				newObj.Spec.Folders[0].RoleBindingTemplates,
				rbacv1alpha1.RoleBindingTemplate{
					Name: "additional-template",
					Subjects: []rbacv1.Subject{
						{
							Kind:     "Group",
							Name:     "test-group",
							APIGroup: "rbac.authorization.k8s.io",
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "edit",
					},
				},
			)

			// validateNamespacesExist should succeed because namespace was in old tree
			err := validator.validateNamespacesExist(ctx, newObj, oldObj)
			Expect(err).NotTo(HaveOccurred(), "Should allow update with deleted namespace that was in old tree")
		})

		It("should reject UPDATE when adding NEW namespace that doesn't exist", func() {
			// Old FolderTree with one namespace
			oldObj := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "test-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
							Namespaces: []string{"existing-namespace-in-old"},
						},
					},
				},
			}

			// New FolderTree adding a non-existent namespace
			newObj := oldObj.DeepCopy()
			newObj.Spec.Folders[0].Namespaces = append(
				newObj.Spec.Folders[0].Namespaces,
				"brand-new-nonexistent-namespace",
			)

			// validateNamespacesExist should fail because new namespace doesn't exist
			err := validator.validateNamespacesExist(ctx, newObj, oldObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
			Expect(err.Error()).To(ContainSubstring("brand-new-nonexistent-namespace"))
		})

		It("should allow DELETE when namespace was deleted", func() {
			// Simulate FolderTree with a namespace that was deleted
			obj.ObjectMeta.Name = "test-tree-for-deletion"
			obj.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								RoleRef: rbacv1.RoleRef{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
						Namespaces: []string{"deleted-namespace-for-delete-test"},
					},
				},
			}

			// ValidateDelete should succeed even though namespace doesn't exist
			// The validateDeleteOperation() method checks namespace existence and skips if not found
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred(), "Should allow DELETE even when namespace was deleted")
			Expect(warnings).To(BeEmpty())
		})
	})
})
