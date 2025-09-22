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

var _ = Describe("FolderTree Controller - Propagate Field", func() {
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

	Context("When testing propagate field behavior", func() {
		It("should respect propagate=false (secure by default)", func() {
			resourceName := "test-no-propagation"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespaces
			parentNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-parent",
				},
			}
			childNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foldertree-test-child",
				},
			}
			Expect(k8sClient.Create(ctx, parentNamespace)).To(Succeed())
			Expect(k8sClient.Create(ctx, childNamespace)).To(Succeed())

			// Create FolderTree with non-propagating templates (secure by default)
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
							Name:       "parent",
							Namespaces: []string{"foldertree-test-parent"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "parent-secret-access",
									// No Propagate field - defaults to false (secure by default)
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "secret-admins",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name:       "child",
							Namespaces: []string{"foldertree-test-child"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "child-local-access",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "child-team",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
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

			By("Verifying parent namespace has only its own RoleBinding")
			parentRoleBindings := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, parentRoleBindings, client.InNamespace("foldertree-test-parent"))
			Expect(err).NotTo(HaveOccurred())

			parentFolderTreeRBs := []rbacv1.RoleBinding{}
			for _, rb := range parentRoleBindings.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					parentFolderTreeRBs = append(parentFolderTreeRBs, rb)
				}
			}
			Expect(parentFolderTreeRBs).To(HaveLen(1), "Parent namespace should have only 1 RoleBinding")
			Expect(parentFolderTreeRBs[0].Name).To(Equal("foldertree-test-no-propagation-parent-secret-access"))

			By("Verifying child namespace has only its own RoleBinding (no inheritance)")
			childRoleBindings := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, childRoleBindings, client.InNamespace("foldertree-test-child"))
			Expect(err).NotTo(HaveOccurred())

			childFolderTreeRBs := []rbacv1.RoleBinding{}
			for _, rb := range childRoleBindings.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					childFolderTreeRBs = append(childFolderTreeRBs, rb)
				}
			}

			Expect(childFolderTreeRBs).To(HaveLen(1), "Child namespace should have only 1 RoleBinding (no inheritance)")
			Expect(childFolderTreeRBs[0].Name).To(Equal("foldertree-test-no-propagation-child-local-access"))

			// Verify the parent's secret access did NOT propagate
			for _, rb := range childFolderTreeRBs {
				Expect(rb.Name).NotTo(ContainSubstring("parent-secret-access"), "Parent's secret access should NOT propagate to child")
			}

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, parentNamespace)).To(Succeed())
			Expect(k8sClient.Delete(ctx, childNamespace)).To(Succeed())
		})

		It("should handle mixed propagation scenarios", func() {
			resourceName := "test-mixed-propagation"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespaces
			parentNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "foldertree-mixed-parent"},
			}
			childNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "foldertree-mixed-child"},
			}
			Expect(k8sClient.Create(ctx, parentNS)).To(Succeed())
			Expect(k8sClient.Create(ctx, childNS)).To(Succeed())

			// Create FolderTree with mixed propagation behavior
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name:       "parent",
						Subfolders: []rbacv1alpha1.TreeNode{{Name: "child"}},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "parent",
							Namespaces: []string{"foldertree-mixed-parent"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name:      "shared-platform-access",
									Propagate: boolPtr(true), // This SHOULD propagate
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "platform-team",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
								{
									Name: "parent-only-secrets",
									// No Propagate field - defaults to false, should NOT propagate
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "parent-secrets-team",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
								{
									Name:      "parent-explicit-no-propagate",
									Propagate: boolPtr(false), // Explicitly false, should NOT propagate
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "User",
											Name:     "parent-lead",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name:       "child",
							Namespaces: []string{"foldertree-mixed-child"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "child-team-access",
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "child-team",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
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

			By("Verifying parent namespace has all 3 RoleBindings")
			parentRBs := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, parentRBs, client.InNamespace("foldertree-mixed-parent"))
			Expect(err).NotTo(HaveOccurred())

			parentFTRBs := []rbacv1.RoleBinding{}
			for _, rb := range parentRBs.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					parentFTRBs = append(parentFTRBs, rb)
				}
			}
			Expect(parentFTRBs).To(HaveLen(3), "Parent should have all 3 RoleBindings")

			By("Verifying child namespace has correct RoleBindings (selective inheritance)")
			childRBs := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, childRBs, client.InNamespace("foldertree-mixed-child"))
			Expect(err).NotTo(HaveOccurred())

			childFTRBs := []rbacv1.RoleBinding{}
			childRBNames := make(map[string]bool)
			for _, rb := range childRBs.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					childFTRBs = append(childFTRBs, rb)
					childRBNames[rb.Name] = true
				}
			}

			// Should have 2 RoleBindings: 1 inherited (shared-platform-access) + 1 local (child-team-access)
			Expect(childFTRBs).To(HaveLen(2), "Child should have 2 RoleBindings: 1 inherited + 1 local")

			// Verify the correct RoleBindings exist
			Expect(childRBNames).To(HaveKey("foldertree-test-mixed-propagation-shared-platform-access"), "Should have inherited shared-platform-access")
			Expect(childRBNames).To(HaveKey("foldertree-test-mixed-propagation-child-team-access"), "Should have local child-team-access")

			// Verify the non-propagating RoleBindings do NOT exist
			Expect(childRBNames).NotTo(HaveKey("foldertree-test-mixed-propagation-parent-only-secrets"), "Should NOT have parent-only-secrets (no propagate field)")
			Expect(childRBNames).NotTo(HaveKey("foldertree-test-mixed-propagation-parent-explicit-no-propagate"), "Should NOT have parent-explicit-no-propagate (propagate: false)")

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, parentNS)).To(Succeed())
			Expect(k8sClient.Delete(ctx, childNS)).To(Succeed())
		})

		It("should handle default propagate behavior (nil means false)", func() {
			resourceName := "test-default-behavior"
			typeNamespacedName := types.NamespacedName{Name: resourceName}

			// Create test namespaces
			parentNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "foldertree-default-parent"},
			}
			childNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "foldertree-default-child"},
			}
			Expect(k8sClient.Create(ctx, parentNS)).To(Succeed())
			Expect(k8sClient.Create(ctx, childNS)).To(Succeed())

			// Create FolderTree with no propagate fields specified (should default to false)
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name:       "parent",
						Subfolders: []rbacv1alpha1.TreeNode{{Name: "child"}},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "parent",
							Namespaces: []string{"foldertree-default-parent"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "default-behavior-template",
									// No Propagate field specified - should default to false
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "default-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
								},
							},
						},
						{
							Name:       "child",
							Namespaces: []string{"foldertree-default-child"},
							// No RoleBindingTemplates - should inherit nothing (secure by default)
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

			By("Verifying parent namespace has its RoleBinding")
			parentRBs := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, parentRBs, client.InNamespace("foldertree-default-parent"))
			Expect(err).NotTo(HaveOccurred())

			parentFTRBs := []rbacv1.RoleBinding{}
			for _, rb := range parentRBs.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					parentFTRBs = append(parentFTRBs, rb)
				}
			}
			Expect(parentFTRBs).To(HaveLen(1), "Parent should have 1 RoleBinding")

			By("Verifying child namespace has NO RoleBindings (secure by default)")
			childRBs := &rbacv1.RoleBindingList{}
			err = k8sClient.List(ctx, childRBs, client.InNamespace("foldertree-default-child"))
			Expect(err).NotTo(HaveOccurred())

			childFTRBs := []rbacv1.RoleBinding{}
			for _, rb := range childRBs.Items {
				if managedBy, exists := rb.Labels["app.kubernetes.io/managed-by"]; exists && managedBy == "foldertree-controller" {
					childFTRBs = append(childFTRBs, rb)
				}
			}
			Expect(childFTRBs).To(HaveLen(0), "Child should have NO RoleBindings (secure by default)")

			// Clean up
			Expect(k8sClient.Delete(ctx, folderTree)).To(Succeed())
			Expect(k8sClient.Delete(ctx, parentNS)).To(Succeed())
			Expect(k8sClient.Delete(ctx, childNS)).To(Succeed())
		})
	})
})
