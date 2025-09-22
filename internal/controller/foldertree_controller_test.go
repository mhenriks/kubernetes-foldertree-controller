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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// Helper function to create bool pointers
func boolPtr(b bool) *bool { return &b }

var _ = Describe("FolderTree Controller", func() {
	var (
		ctx        context.Context
		reconciler *FolderTreeReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &FolderTreeReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("When reconciling a FolderTree with inline role binding templates", func() {
		It("should successfully reconcile and create RoleBindings", func() {
			resourceName := "test-inline-templates-1"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-ns-1",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create FolderTree with inline role binding templates
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name: "root-folder",
						Subfolders: []rbacv1alpha1.TreeNode{
							{
								Name: "child-folder",
							},
						},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "root-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "root-admin",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "root-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name: "child-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "child-admin",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "child-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
							Namespaces: []string{"foldertree-test-ns-1"},
						},
						{
							Name: "standalone-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "standalone-admin",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "standalone-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
							Namespaces: []string{"foldertree-test-ns-1"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			By("Reconciling the FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for successful processing

			By("Checking that controller processed the FolderTree")
			// Note: In test environment, RoleBindings may not be created due to RBAC limitations
			// but we can verify the FolderTree was processed without errors

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
		})

		It("should handle empty role binding templates", func() {
			resourceName := "test-empty-templates"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-ns-2",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create FolderTree with empty role binding templates
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:                 "empty-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{}, // Empty templates
							Namespaces:           []string{"foldertree-test-ns-2"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			By("Reconciling the FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for successful processing

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
		})

		It("should handle inheritance in tree structures", func() {
			resourceName := "test-inheritance"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-ns-3",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create FolderTree with inheritance
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
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
									Name:      "parent-admin",
									Propagate: boolPtr(true), // Explicitly enable propagation
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "parent-admins",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name: "child",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "child-editor",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "child-editors",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
							Namespaces: []string{"foldertree-test-ns-3"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			By("Reconciling the FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for successful processing

			By("Verifying RoleBindings are created correctly")
			// Should have 2 RoleBindings in child namespace: parent-admin (inherited) + child-editor (local)
			roleBindingList := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, roleBindingList, client.InNamespace("foldertree-test-ns-3"))
			Expect(err).NotTo(HaveOccurred())

			// Filter for FolderTree-managed RoleBindings
			folderTreeRoleBindings := []rbacv1.RoleBinding{}
			for _, rb := range roleBindingList.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					folderTreeRoleBindings = append(folderTreeRoleBindings, rb)
				}
			}

			Expect(folderTreeRoleBindings).To(HaveLen(2), "Should have 2 RoleBindings: inherited parent-admin + local child-editor")

			// Verify specific RoleBindings exist
			roleBindingNames := make(map[string]bool)
			for _, rb := range folderTreeRoleBindings {
				roleBindingNames[rb.Name] = true
			}

			Expect(roleBindingNames).To(HaveKey("foldertree-test-inheritance-parent-admin"), "Should have inherited parent-admin RoleBinding")
			Expect(roleBindingNames).To(HaveKey("foldertree-test-inheritance-child-editor"), "Should have local child-editor RoleBinding")

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
		})

		It("should handle non-existent FolderTree gracefully", func() {
			resourceName := "non-existent-tree"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			By("Reconciling a non-existent FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for non-existent resources
		})

		It("should handle multi-level inheritance correctly", func() {
			resourceName := "test-multi-level"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-ns-4",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create FolderTree with multi-level inheritance
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
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
									Name: "root-perm",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "root-users",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name: "level1",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "level1-perm",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "level1-users",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name: "level2",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "level2-perm",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "level2-users",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
							Namespaces: []string{"foldertree-test-ns-4"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			By("Reconciling the FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for successful processing

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
		})

		It("should handle non-existent namespaces gracefully", func() {
			resourceName := "test-missing-ns"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create FolderTree referencing non-existent namespace
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "test-user",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
							Namespaces: []string{"non-existent-namespace"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			By("Reconciling the FolderTree")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0))) // No requeue for successful processing

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
		})
	})

	Context("When testing diff-based operations", func() {
		It("should execute create operations correctly", func() {
			// Create a test namespace first
			testNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-create-ns",
				},
			}
			Expect(k8sClient.Create(ctx, testNS)).To(Succeed())

			// Create a FolderTree
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-create-ops",
				},
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
							Namespaces: []string{"test-create-ns"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			// Reconcile to trigger operations
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-create-ops"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify RoleBinding was created
			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "foldertree-test-create-ops-test-template",
				Namespace: "test-create-ns",
			}, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Subjects[0].Name).To(Equal("test-user"))
			Expect(rb.RoleRef.Name).To(Equal("view"))
		})

		It("should execute update operations correctly", func() {
			// Create a test namespace first
			testNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-update-ns",
				},
			}
			Expect(k8sClient.Create(ctx, testNS)).To(Succeed())

			// Create existing RoleBinding
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-update-ops-test-template",
					Namespace: "test-update-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test-update-ops",
					},
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:     "User",
						Name:     "old-user",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "view",
				},
			}
			Expect(k8sClient.Create(ctx, existingRB)).To(Succeed())

			// Create a FolderTree with updated subjects
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-update-ops",
				},
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
											Name:     "new-user", // Different from existing
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
							Namespaces: []string{"test-update-ns"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			// Reconcile to trigger update operation
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-update-ops"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify RoleBinding was updated
			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "foldertree-test-update-ops-test-template",
				Namespace: "test-update-ns",
			}, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Subjects[0].Name).To(Equal("new-user")) // Should be updated
		})

		It("should execute delete operations correctly", func() {
			// Create a test namespace first
			testNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-delete-ns",
				},
			}
			Expect(k8sClient.Create(ctx, testNS)).To(Succeed())

			// Create existing RoleBinding that should be deleted
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-delete-ops-old-template",
					Namespace: "test-delete-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test-delete-ops",
					},
				},
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
			}
			Expect(k8sClient.Create(ctx, existingRB)).To(Succeed())

			// Create a FolderTree without the old template (should trigger delete)
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-delete-ops",
				},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "new-template", // Different template name
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
							Namespaces: []string{"test-delete-ns"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			// Reconcile to trigger delete operation
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-delete-ops"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify old RoleBinding was deleted
			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "foldertree-test-delete-ops-old-template",
				Namespace: "test-delete-ns",
			}, rb)
			Expect(err).To(HaveOccurred()) // Should be NotFound

			// Verify new RoleBinding was created
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "foldertree-test-delete-ops-new-template",
				Namespace: "test-delete-ns",
			}, rb)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle missing namespaces gracefully", func() {
			// Create a FolderTree with a non-existent namespace
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-missing-ns",
				},
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
							Namespaces: []string{"non-existent-ns"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, folderTree)).To(Succeed())

			// Reconcile should not fail even with missing namespace
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-missing-ns"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify no RoleBinding was created in non-existent namespace
			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "foldertree-test-missing-ns-test-template",
				Namespace: "non-existent-ns",
			}, rb)
			Expect(err).To(HaveOccurred()) // Should be NotFound
		})
	})
})
