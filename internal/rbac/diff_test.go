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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

// Helper function to create bool pointers
func boolPtr(b bool) *bool { return &b }

var _ = Describe("DiffAnalyzer", func() {
	var (
		ctx          context.Context
		fakeClient   client.Client
		folderTree   *rbacv1alpha1.FolderTree
		builder      *RoleBindingBuilder
		diffAnalyzer *DiffAnalyzer
		scheme       *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(rbacv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())

		folderTree = &rbacv1alpha1.FolderTree{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-tree",
			},
		}

		builder = &RoleBindingBuilder{
			FolderTree: folderTree,
			Scheme:     scheme,
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		diffAnalyzer = NewDiffAnalyzer(fakeClient, folderTree, builder)
	})

	Context("when no existing RoleBindings exist", func() {
		It("should generate create operations for all desired RoleBindings", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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
						Namespaces: []string{"test-ns1", "test-ns2"},
					},
				},
			}

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(2)) // 2 namespaces × 1 template = 2 operations

			for _, op := range operations {
				Expect(op.Type).To(Equal(OperationCreate))
				Expect(op.DesiredRoleBinding).NotTo(BeNil())
				Expect(op.ExistingRoleBinding).To(BeNil())
			}
		})
	})

	Context("when existing RoleBindings match desired state", func() {
		It("should generate no operations", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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

			// Create existing RoleBinding that matches desired state
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-admin-template",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":                      "foldertree-controller",
						"foldertree.rbac.kubevirt.io/tree":                  "test-tree",
						"foldertree.rbac.kubevirt.io/role-binding-template": "admin-template",
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
					Name:     "admin",
				},
			}
			Expect(fakeClient.Create(ctx, existingRB)).To(Succeed())

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(BeEmpty()) // No operations needed
		})
	})

	Context("when existing RoleBindings need updates", func() {
		It("should generate update operations", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "updated-user", // Different from existing
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

			// Create existing RoleBinding with different subjects
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-admin-template",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":                      "foldertree-controller",
						"foldertree.rbac.kubevirt.io/tree":                  "test-tree",
						"foldertree.rbac.kubevirt.io/role-binding-template": "admin-template",
					},
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:     "User",
						Name:     "old-user", // Different from desired
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "admin",
				},
			}
			Expect(fakeClient.Create(ctx, existingRB)).To(Succeed())

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1))

			op := operations[0]
			Expect(op.Type).To(Equal(OperationUpdate))
			Expect(op.ExistingRoleBinding).NotTo(BeNil())
			Expect(op.DesiredRoleBinding).NotTo(BeNil())
			Expect(op.DesiredRoleBinding.Subjects[0].Name).To(Equal("updated-user"))
		})
	})

	Context("when existing RoleBindings are no longer needed", func() {
		It("should generate delete operations", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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
						Namespaces: []string{"test-ns"}, // Only one namespace now
					},
				},
			}

			// Create existing RoleBindings in two namespaces
			existingRB1 := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-admin-template",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":                      "foldertree-controller",
						"foldertree.rbac.kubevirt.io/tree":                  "test-tree",
						"foldertree.rbac.kubevirt.io/role-binding-template": "admin-template",
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
					Name:     "admin",
				},
			}
			existingRB2 := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-admin-template",
					Namespace: "old-ns", // This namespace is no longer in the spec
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test-tree",
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
					Name:     "admin",
				},
			}
			Expect(fakeClient.Create(ctx, existingRB1)).To(Succeed())
			Expect(fakeClient.Create(ctx, existingRB2)).To(Succeed())

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1)) // Only delete operation for old-ns

			op := operations[0]
			Expect(op.Type).To(Equal(OperationDelete))
			Expect(op.ExistingRoleBinding).NotTo(BeNil())
			Expect(op.ExistingRoleBinding.Namespace).To(Equal("old-ns"))
			Expect(op.DesiredRoleBinding).To(BeNil())
		})
	})

	Context("with propagate field", func() {
		It("should respect propagate=false and not inherit templates", func() {
			// Helper function to create bool pointer
			boolPtr := func(b bool) *bool { return &b }

			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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
								Name:      "propagating-template",
								Propagate: boolPtr(true), // Explicitly set to true
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
							{
								Name:      "non-propagating-template",
								Propagate: boolPtr(false), // Set to false - should not inherit
								Subjects: []rbacv1.Subject{
									{
										Kind:     "Group",
										Name:     "parent-secrets-group",
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
								Name: "child-template",
								// No propagate field - defaults to true
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

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Should have 2 operations: propagating-template + child-template
			// non-propagating-template should NOT be inherited
			Expect(operations).To(HaveLen(2))

			templateNames := make(map[string]bool)
			for _, op := range operations {
				Expect(op.Type).To(Equal(OperationCreate))
				Expect(op.Namespace).To(Equal("child-ns"))
				templateNames[op.RoleBindingTemplate.Name] = true
			}

			// Should have the propagating template and child's own template
			Expect(templateNames).To(HaveKey("propagating-template"))
			Expect(templateNames).To(HaveKey("child-template"))

			// Should NOT have the non-propagating template
			Expect(templateNames).NotTo(HaveKey("non-propagating-template"))
		})

		It("should handle default propagate behavior (nil means false)", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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
								Name: "default-propagate-template",
								// No Propagate field specified - should default to true
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
						Name:       "child",
						Namespaces: []string{"child-ns"},
						// No role binding templates - should inherit from parent
					},
				},
			}

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(BeEmpty()) // Should NOT inherit the parent template (default is false)

			// No operations should be created since propagate defaults to false
		})
	})

	Context("with tree inheritance", func() {
		It("should generate operations for inherited role binding templates", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
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
								Name:      "parent-template",
								Propagate: boolPtr(true), // Explicitly enable propagation
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

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(2)) // 2 templates × 1 namespace = 2 operations (parent propagates, child is local)

			// Should have operations for both parent and child templates in child namespace
			templateNames := make(map[string]bool)
			for _, op := range operations {
				Expect(op.Type).To(Equal(OperationCreate))
				Expect(op.Namespace).To(Equal("child-ns"))
				templateNames[op.RoleBindingTemplate.Name] = true
			}
			Expect(templateNames).To(HaveKey("parent-template"))
			Expect(templateNames).To(HaveKey("child-template"))
		})
	})

	Context("with mixed operations", func() {
		It("should generate create, update, and delete operations as needed", func() {
			folderTree.Spec = rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "admin-template",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "updated-user", // Will need update
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
								Name: "new-template", // Will need create
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "new-user",
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
						Namespaces: []string{"test-ns"},
					},
				},
			}

			// Create existing RoleBindings
			existingRB1 := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-admin-template",
					Namespace: "test-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test-tree",
					},
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:     "User",
						Name:     "old-user", // Different from desired
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "admin",
				},
			}
			existingRB2 := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-tree-old-template",
					Namespace: "test-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test-tree",
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
			Expect(fakeClient.Create(ctx, existingRB1)).To(Succeed())
			Expect(fakeClient.Create(ctx, existingRB2)).To(Succeed())

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(3)) // 1 update + 1 create + 1 delete

			operationTypes := make(map[OperationType]int)
			for _, op := range operations {
				operationTypes[op.Type]++
			}
			Expect(operationTypes[OperationCreate]).To(Equal(1)) // new-template
			Expect(operationTypes[OperationUpdate]).To(Equal(1)) // admin-template
			Expect(operationTypes[OperationDelete]).To(Equal(1)) // old-template
		})
	})

	Context("RoleBindingOperation String method", func() {
		It("should return correct string representations", func() {
			// Test CREATE operation
			createOp := RoleBindingOperation{
				Type:      OperationCreate,
				Namespace: "test-ns",
				RoleBindingTemplate: rbacv1alpha1.RoleBindingTemplate{
					Name: "test-template",
				},
				DesiredRoleBinding: &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-rb",
					},
				},
			}
			Expect(createOp.String()).To(ContainSubstring("CREATE RoleBinding 'test-rb' in namespace 'test-ns' for template 'test-template'"))

			// Test UPDATE operation
			updateOp := RoleBindingOperation{
				Type:      OperationUpdate,
				Namespace: "test-ns",
				RoleBindingTemplate: rbacv1alpha1.RoleBindingTemplate{
					Name: "test-template",
				},
				ExistingRoleBinding: &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-rb",
					},
				},
			}
			Expect(updateOp.String()).To(ContainSubstring("UPDATE RoleBinding 'test-rb' in namespace 'test-ns' for template 'test-template'"))

			// Test DELETE operation
			deleteOp := RoleBindingOperation{
				Type:      OperationDelete,
				Namespace: "test-ns",
				ExistingRoleBinding: &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-rb",
					},
				},
			}
			Expect(deleteOp.String()).To(ContainSubstring("DELETE RoleBinding 'test-rb' in namespace 'test-ns'"))
		})
	})

	Context("RoleRef Change Scenarios", func() {
		It("should generate DELETE+CREATE operations when roleRef changes", func() {
			// Create existing RoleBinding with 'view' roleRef
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-admin",
					Namespace: "test-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test",
					},
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:     "Group",
						Name:     "test-group",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     "view", // Original roleRef
					APIGroup: "rbac.authorization.k8s.io",
				},
			}

			// Add existing RoleBinding to fake client
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRB).Build()

			// Create FolderTree with changed roleRef
			folderTree = &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "admin",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "test-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										Kind:     "ClusterRole",
										Name:     "edit", // Changed from 'view' to 'edit'
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
							Namespaces: []string{"test-ns"},
						},
					},
				},
			}

			builder = &RoleBindingBuilder{FolderTree: folderTree, Scheme: scheme}
			diffAnalyzer = NewDiffAnalyzer(fakeClient, folderTree, builder)

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Should generate DELETE+CREATE operations (not UPDATE)
			Expect(operations).To(HaveLen(2))

			// Find DELETE and CREATE operations
			var deleteOp, createOp *RoleBindingOperation
			for i := range operations {
				switch operations[i].Type {
				case OperationDelete:
					deleteOp = &operations[i]
				case OperationCreate:
					createOp = &operations[i]
				}
			}

			// Verify DELETE operation
			Expect(deleteOp).NotTo(BeNil())
			Expect(deleteOp.Type).To(Equal(OperationDelete))
			Expect(deleteOp.ExistingRoleBinding.Name).To(Equal("foldertree-test-admin"))
			Expect(deleteOp.ExistingRoleBinding.RoleRef.Name).To(Equal("view"))

			// Verify CREATE operation
			Expect(createOp).NotTo(BeNil())
			Expect(createOp.Type).To(Equal(OperationCreate))
			Expect(createOp.DesiredRoleBinding.Name).To(Equal("foldertree-test-admin"))
			Expect(createOp.DesiredRoleBinding.RoleRef.Name).To(Equal("edit"))
		})

		It("should generate UPDATE operation when only subjects change (roleRef unchanged)", func() {
			// Create existing RoleBinding
			existingRB := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foldertree-test-admin",
					Namespace: "test-ns",
					Labels: map[string]string{
						"foldertree.rbac.kubevirt.io/tree": "test",
					},
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:     "Group",
						Name:     "old-group", // Original subject
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     "view", // Same roleRef
					APIGroup: "rbac.authorization.k8s.io",
				},
			}

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRB).Build()

			// Create FolderTree with changed subjects but same roleRef
			folderTree = &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "test-folder",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "admin",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "new-group", // Changed subject
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										Kind:     "ClusterRole",
										Name:     "view", // Same roleRef
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
							Namespaces: []string{"test-ns"},
						},
					},
				},
			}

			builder = &RoleBindingBuilder{FolderTree: folderTree, Scheme: scheme}
			diffAnalyzer = NewDiffAnalyzer(fakeClient, folderTree, builder)

			operations, err := diffAnalyzer.AnalyzeDiff(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Should generate single UPDATE operation (not DELETE+CREATE)
			Expect(operations).To(HaveLen(1))
			Expect(operations[0].Type).To(Equal(OperationUpdate))
			Expect(operations[0].ExistingRoleBinding.RoleRef.Name).To(Equal("view"))
			Expect(operations[0].DesiredRoleBinding.RoleRef.Name).To(Equal("view"))
			Expect(operations[0].DesiredRoleBinding.Subjects[0].Name).To(Equal("new-group"))
		})
	})
})
